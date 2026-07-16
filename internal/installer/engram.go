// Engram install support.
//
// v0.1 wrote a hand-rolled durable MCP entry at <ClaudeHome>/mcp/engram.json, pointing at an
// absolute, pinned Engram binary path. Slice 3's Step 0 spike
// (documentacion/spikes/spike-e-engram-install.md) proved that path is never actually read by
// Claude Code: `claude mcp list` and ~/.claude.json's top-level mcpServers only ever show the
// engram MCP server sourced from the engram PLUGIN's own bundled .mcp.json
// (`plugin:engram:engram`, launching a bare, PATH-resolved `command: "engram"`). A real developer
// machine was found with both files present — the hand-rolled mcp/engram.json AND the plugin —
// and only the plugin-sourced server showed up as connected. So this file no longer writes an MCP
// config at all: it registers the engram Claude Code PLUGIN (the only mechanism that actually
// wires Engram's tools into a session) and tracks click's own bookkeeping about that install.
package installer

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/manifest"
)

const (
	engramBinaryPathEnvOverride = "CLICK_ENGRAM_BINARY_PATH"
	engramMarketplaceName       = "engram"
	engramPluginName            = "engram"

	// EngramPluginID is the plugin@marketplace identifier Claude Code assigns once the Engram
	// marketplace and plugin are registered. Verified against the real `claude` CLI in Step 0:
	// `claude plugin marketplace add https://github.com/Gentleman-Programming/engram` derives the
	// marketplace name "engram" from the repo, then `claude plugin install engram@engram`
	// registers it under exactly this id. This is also what makes the memory-guard hook's matcher
	// (mcp__plugin_engram_engram__mem_save) correct: Claude Code's plugin-provided MCP tool
	// naming is mcp__plugin_<plugin>_<server>__<tool>, and both the plugin name and its bundled
	// MCP server key are "engram" — confirmed by this session's own available tool names.
	EngramPluginID = engramPluginName + "@" + engramMarketplaceName
)

var engramMarketplaceSource = defaultEngramMarketplaceSource()

func defaultEngramMarketplaceSource() string {
	return "https://github.com/Gentleman-Programming/engram"
}

// SetEngramMarketplaceSourceForTests overrides the Engram marketplace source for tests and
// returns a restore function.
func SetEngramMarketplaceSourceForTests(source string) func() {
	old := engramMarketplaceSource
	engramMarketplaceSource = source
	return func() { engramMarketplaceSource = old }
}

// engramState is click's own bookkeeping about the Engram install: the pinned version/source this
// click-ai-devkit release expects (D8), the binary path resolved the last time click touched this
// machine, and whether click itself installed the engram@engram plugin — as opposed to finding
// one a developer had already set up independently. RemoveEngramPlugin reads InstalledByClick to
// decide whether it is safe to remove the plugin: click only ever reverses what it added.
type engramState struct {
	Version          string `json:"version"`
	BinaryPath       string `json:"binary_path"`
	Source           string `json:"source"`
	InstalledByClick bool   `json:"installed_by_click"`
}

// SyncEngram registers the Engram marketplace and installs engram@engram through the native
// `claude plugin` CLI — unless Engram is already installed, in which case it is left completely
// untouched (see SyncEngramPlugin). It always (re)writes click's own state bookkeeping file so
// `click doctor` and `click uninstall` can report on, and respect, Engram's install ownership.
// Returns alreadyInstalled=true when Engram was already installed before this call.
//
// Ownership (InstalledByClick) is decided exactly once — the first time SyncEngram ever runs
// against a given ClaudeHome — and preserved on every later call. This matters because SyncEngram
// itself is meant to be idempotent (`click install`/`click update` call it every run): by the
// second run, engram@engram is already installed — by click, from the first run — so
// alreadyInstalled is true again. Naively deriving ownership from "!alreadyInstalled" on every
// call would flip a click-owned install to "pre-existing" the moment click's OWN prior install
// makes it look already-there, and RemoveEngramPlugin would then wrongly refuse to remove
// something click actually added (caught by a real end-to-end run in Step 0, not just the fakes).
//
// pathWarning (Phase 4 / D-5, sdd/engram-mcp-resolution obs #1436) is forwarded unchanged from
// EnsureEngramBinary: non-empty ONLY when a PATH-persistence attempt was made and failed or
// partly failed. It is never an error on its own — the binary is still resolvable regardless of
// whether the persisted-PATH write succeeded — so callers surface it (e.g. via ui.Renderer.Warn),
// they don't treat it as a reason to fail install/update.
func SyncEngram(cfg Config, m *manifest.Manifest) (alreadyInstalled bool, pathWarning string, err error) {
	alreadyInstalled, err = SyncEngramPlugin(cfg)
	if err != nil {
		return false, "", err
	}
	// EnsureEngramBinary (Slice 3b) both resolves the binary path AND, when it's missing and Go is
	// available, attempts to provision it via `go install`. It never fails this call — a missing
	// binary/toolchain is reported via remediation, not an error — so its own error return here is
	// reserved for genuine I/O failures resolving state, not "binary not found".
	binaryPath, _, _, pathWarning, err := EnsureEngramBinary(cfg, m.Engram.Version)
	if err != nil {
		return alreadyInstalled, "", err
	}

	installedByClick := !alreadyInstalled
	existing, found, err := loadEngramState(cfg)
	if err != nil {
		return alreadyInstalled, pathWarning, err
	}
	if found {
		installedByClick = existing.InstalledByClick
	}

	state := engramState{
		Version:          m.Engram.Version,
		BinaryPath:       binaryPath,
		Source:           m.Engram.Source,
		InstalledByClick: installedByClick,
	}
	if err := writeJSONFile(cfg.EngramStatePath(), state); err != nil {
		return alreadyInstalled, pathWarning, fmt.Errorf("installer: write engram state: %w", err)
	}
	return alreadyInstalled, pathWarning, nil
}

// SyncEngramPlugin is the plugin-registration half of SyncEngram, split out so callers/tests can
// exercise the exact `claude plugin ...` command sequence (and its idempotent skip) without
// needing a manifest.Manifest. It is deliberately respectful: many developers already run Engram
// independently of click (verified on a real machine during Step 0), so click must never reinstall
// or clobber a working setup — just detect it, skip, and report that back to the caller.
func SyncEngramPlugin(cfg Config) (alreadyInstalled bool, err error) {
	installed, err := HasInstalledPluginID(cfg, EngramPluginID)
	if err != nil {
		return false, err
	}
	if installed {
		return true, nil
	}
	runner := commandRunnerFactory()
	// No --sparse: unlike click-ai-devkit's own marketplace, engram's repo has no plugins/
	// directory (its plugin lives at plugin/claude-code/) — a plugins/-scoped sparse checkout
	// would silently miss it. Confirmed against the real CLI in Step 0.
	if err := addMarketplace(runner, engramMarketplaceSource, nil); err != nil {
		return false, err
	}
	if err := installPluginID(runner, engramPluginName, engramMarketplaceName); err != nil {
		return false, err
	}
	return false, nil
}

// RemoveEngramPlugin reverses SyncEngram, but ONLY when click's own state says click installed
// Engram in the first place. If a developer already had Engram working before running `click
// install`, click uninstall leaves it running untouched — click only ever removes what it added.
// It is idempotent: safe to call when Engram was never touched by click, or has already been
// removed.
func RemoveEngramPlugin(cfg Config) error {
	state, found, err := loadEngramState(cfg)
	if err != nil {
		return err
	}
	if !found {
		// click's Install() never ran against this home (or ran before this feature existed) —
		// nothing click-managed to reverse.
		return nil
	}
	if !state.InstalledByClick {
		// click never owned this install; leave Engram alone, just drop click's own bookkeeping.
		return removeEngramState(cfg)
	}
	installed, err := HasInstalledPluginID(cfg, EngramPluginID)
	if err != nil {
		return err
	}
	if installed {
		runner := commandRunnerFactory()
		if err := uninstallPluginID(runner, engramPluginName, engramMarketplaceName); err != nil {
			return err
		}
		if err := removeMarketplace(runner, engramMarketplaceName); err != nil {
			return err
		}
	}
	return removeEngramState(cfg)
}

func loadEngramState(cfg Config) (engramState, bool, error) {
	data, err := os.ReadFile(cfg.EngramStatePath())
	if err != nil {
		if os.IsNotExist(err) {
			return engramState{}, false, nil
		}
		return engramState{}, false, fmt.Errorf("installer: read engram state: %w", err)
	}
	var state engramState
	if err := json.Unmarshal(data, &state); err != nil {
		return engramState{}, false, fmt.Errorf("installer: parse engram state: %w", err)
	}
	return state, true, nil
}

func removeEngramState(cfg Config) error {
	if err := removeIfExists(cfg.EngramStatePath()); err != nil {
		return fmt.Errorf("installer: remove engram state: %w", err)
	}
	return nil
}

func removeIfExists(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// ResolveEngramBinaryPath resolves the absolute path to the pinned Engram binary: a test/
// deployment override, then a PATH-resolved binary, and finally a Click-managed default path
// where a release-installed binary is expected to land. This matters purely for `click doctor` to
// predict whether the engram plugin's bundled MCP server (bare `command: "engram"`, PATH-resolved
// — confirmed in Step 0) will actually be able to launch; click does not write an MCP config
// itself (see the package doc comment above).
func ResolveEngramBinaryPath(cfg Config) (string, error) {
	if override := os.Getenv(engramBinaryPathEnvOverride); override != "" {
		return filepath.Abs(override)
	}
	if path, err := binaryLookupFactory().LookPath(engramBinaryName()); err == nil {
		return filepath.Abs(path)
	}
	return cfg.DefaultEngramBinaryPath(), nil
}

// EngramBinaryResolvable reports whether the resolved Engram binary path actually exists on disk
// — `click doctor`'s way of predicting whether the plugin's bundled MCP server will actually be
// able to launch, since a bare PATH-resolved command silently fails to connect (not a build/install
// error) when the binary is missing.
func EngramBinaryResolvable(cfg Config) (path string, ok bool, err error) {
	path, err = ResolveEngramBinaryPath(cfg)
	if err != nil {
		return "", false, err
	}
	info, statErr := os.Stat(path)
	if statErr != nil || info.IsDir() {
		return path, false, nil
	}
	return path, true, nil
}

func engramBinaryName() string {
	if runtime.GOOS == "windows" {
		return "engram.exe"
	}
	return "engram"
}

// BinaryLookup abstracts resolving a binary name on PATH (exec.LookPath), mirroring the same
// factory-injected pattern CommandRunner already uses for `claude plugin ...` execution (plugins.go).
// This lets unit tests fake PATH resolution deterministically — including simulating a `go install`
// run making a previously-missing binary newly resolvable — without ever touching a real developer
// machine's PATH or its `go` toolchain.
type BinaryLookup interface {
	LookPath(file string) (string, error)
}

type execBinaryLookup struct{}

func (execBinaryLookup) LookPath(file string) (string, error) { return exec.LookPath(file) }

var binaryLookupFactory = func() BinaryLookup { return execBinaryLookup{} }

// SetBinaryLookupFactoryForTests overrides the binary lookup factory for tests and returns a
// restore function.
func SetBinaryLookupFactoryForTests(factory func() BinaryLookup) func() {
	old := binaryLookupFactory
	binaryLookupFactory = factory
	return func() { binaryLookupFactory = old }
}

// engramBinaryModulePath is the Go module path for Engram's CLI/MCP binary. Confirmed resolvable
// via `go install` in this slice's Step 0 spike (documentacion/spikes/spike-e-engram-install.md):
// both `@latest` and the manifest-pinned `@v1.15.3` resolve and produce a binary that actually runs.
const engramBinaryModulePath = "github.com/Gentleman-Programming/engram/cmd/engram"

// EngramInstallCommand is the exact `go install` command line click recommends — and, when the Go
// toolchain is available, runs itself via EnsureEngramBinary — to provision the Engram binary
// pinned by this click-ai-devkit release.
func EngramInstallCommand(version string) string {
	return fmt.Sprintf("go install %s@%s", engramBinaryModulePath, version)
}

// EngramBinaryRemediationMessage is the single source of truth for the text shown when the Engram
// binary cannot be provisioned automatically — either because the Go toolchain isn't on PATH, or
// because `go install` ran but the binary still isn't resolvable afterward (e.g. GOPATH/bin itself
// isn't on PATH). Both `click install`'s non-fatal fallback and `click doctor`'s checkEngramBinary
// share this exact text, so the two call sites never drift apart.
func EngramBinaryRemediationMessage(version string) string {
	return fmt.Sprintf(
		"El binario de engram no se encuentra en el PATH. Instalalo manualmente con:\n"+
			"  %s\n"+
			"Después asegurate de que tu directorio de binarios de Go (GOPATH/bin, o GOBIN si lo definiste) esté en el PATH.\n"+
			"En macOS también podés usar: brew install gentleman-programming/tap/engram",
		EngramInstallCommand(version),
	)
}

// goAvailable reports whether the Go toolchain is resolvable on PATH, via the same injectable
// BinaryLookup used for the Engram binary itself — click never invokes a second, ad hoc PATH lookup
// mechanism.
func goAvailable() bool {
	_, err := binaryLookupFactory().LookPath("go")
	return err == nil
}

// EnsureEngramBinary checks whether the Engram binary the plugin's bundled MCP server needs (bare,
// PATH-resolved `command: "engram"`, confirmed in Slice 3's Step 0) is already resolvable, and — if
// not — attempts to provision it via `go install <engramBinaryModulePath>@<version>` when the Go
// toolchain is itself available on PATH. It never downloads a release zip, and never fails the
// caller for a provisioning problem: when the binary genuinely can't be provisioned (no Go, or `go
// install` ran but the binary is still not resolvable afterward), it returns a non-empty
// remediation message for the caller to surface, and the overall install/update flow continues
// regardless — a missing dev dependency must never brick `click install`.
//
// Idempotent: once the binary is resolvable (whether it always was, or a previous call's `go
// install` already made it so), a later call issues no `go install` command at all.
//
// pathWarning (Phase 4 / D-5, sdd/engram-mcp-resolution obs #1436): once the binary is confirmed
// resolvable, this also attempts to persist its containing Go bin dir onto the user's *persisted*
// PATH via persistPathToBinaryDir — closing the original bug's root failure mode, where a fresh
// `go install` makes the binary resolvable for the CURRENT shell only, and a brand-new terminal
// session silently can't find it (the plugin's bundled MCP server then fails to connect with no
// clear signal why). That persistence attempt can itself fail (no write permission, an
// unrecognized shell, etc.); such a failure is captured into pathWarning, NOT propagated as err —
// the binary IS still provisioned and resolvable, so a PATH-persistence hiccup must never make
// EnsureEngramBinary itself report failure.
func EnsureEngramBinary(cfg Config, version string) (path string, resolvable bool, remediation string, pathWarning string, err error) {
	path, resolvable, err = EngramBinaryResolvable(cfg)
	if err != nil {
		return "", false, "", "", err
	}
	if resolvable {
		return path, true, "", persistPathToBinaryDir(cfg, path), nil
	}
	if !goAvailable() {
		return path, false, EngramBinaryRemediationMessage(version), "", nil
	}

	runner := commandRunnerFactory()
	// Best-effort: any failure here (network, module-proxy hiccup, etc.) is folded into the same
	// "still not resolvable" remediation path below rather than propagated as a hard error.
	_ = runner.Run("go", "install", engramBinaryModulePath+"@"+version)

	path, resolvable, err = EngramBinaryResolvable(cfg)
	if err != nil {
		return "", false, "", "", err
	}
	if !resolvable {
		// JD-001 fix: `resolvable` reflects THIS process's own LookPath-based prediction of whether
		// a bare `command: "engram"` MCP launch would succeed right now — and a child `go install`
		// process can never mutate that. So on a genuinely fresh install (the exact scenario this
		// whole feature exists to fix), resolvable legitimately stays false even after `go install`
		// has verifiably written the binary to disk. PATH persistence must therefore be attempted
		// independently of `resolvable`: check directly whether the binary now exists on disk at
		// GoBinDir(cfg), and if so, persist that directory regardless of what LookPath reports.
		return path, false, EngramBinaryRemediationMessage(version), pathWarningAfterGoInstall(cfg), nil
	}
	return path, true, "", persistPathToBinaryDir(cfg, path), nil
}

// pathWarningAfterGoInstall independently checks whether the Engram binary now exists on disk at
// GoBinDir(cfg) — regardless of what the LookPath-based EngramBinaryResolvable reports — and, if
// so, attempts to persist that directory onto the user's PATH via persistPathToBinaryDir (JD-001).
// It never errors itself: GoBinDir failing to resolve, or the binary simply not existing there
// (e.g. no Go toolchain, or `go install` itself failed), both mean "not attempted" (empty
// pathWarning) — matching persistPathToBinaryDir's own no-error contract.
func pathWarningAfterGoInstall(cfg Config) (pathWarning string) {
	gobin, err := GoBinDir(cfg)
	if err != nil {
		return ""
	}
	candidate := filepath.Join(gobin, engramBinaryName())
	info, statErr := os.Stat(candidate)
	if statErr != nil || info.IsDir() {
		return ""
	}
	return persistPathToBinaryDir(cfg, candidate)
}

// persistPathToBinaryDir attempts to persist the resolved Engram binary's containing directory
// onto the user's persisted PATH, but ONLY when that directory is actually GoBinDir(cfg) itself —
// a binary resolved via brew, a test/deployment env override, or click's own
// DefaultEngramBinaryPath fallback is not something pathStore has any business touching (D-5
// closes JD-B-008's residual gap by applying this same rule from BOTH the already-resolvable
// early-return path AND the post-`go install` path in EnsureEngramBinary above).
//
// It never returns an error itself: GoBinDir failing to resolve, or the binary living outside
// GoBinDir, both mean "not attempted" (empty pathWarning) — not a warning. Only an actual
// pathStoreFactory().EnsureOnPath failure produces a non-empty, wrapped pathWarning.
// pathStoreFactory().EnsureOnPath is already idempotent (a no-op, changed=false, on a directory
// already present in the persisted PATH), so calling it directly here — rather than pre-checking
// PersistedPathContains first — never produces redundant writes on its own.
func persistPathToBinaryDir(cfg Config, binaryPath string) (pathWarning string) {
	gobin, err := GoBinDir(cfg)
	if err != nil {
		// No `go env`-resolvable GOBIN/GOPATH at all — nothing to compare binaryPath against.
		return ""
	}
	if !sameDir(filepath.Dir(binaryPath), gobin) {
		// Resolved from somewhere other than GoBinDir — pathStore has nothing to do here.
		return ""
	}
	if pathStoreFactory == nil {
		// No platform pathStore wired in (e.g. a build with neither pathenv_windows.go nor
		// pathenv_unix.go compiled in, which never happens in a real release build, only in some
		// narrowly-scoped unit tests) — nothing to attempt.
		return ""
	}
	if _, err := pathStoreFactory().EnsureOnPath(gobin); err != nil {
		return fmt.Sprintf("no se pudo agregar %s al PATH persistente: %v", gobin, err)
	}
	return ""
}

// sameDir reports whether a and b refer to the same directory, comparing case-insensitively on
// Windows (where PATH entries and the filesystem itself are case-insensitive) and case-sensitively
// everywhere else, after filepath.Clean normalizes both.
func sameDir(a, b string) bool {
	a, b = filepath.Clean(a), filepath.Clean(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}
