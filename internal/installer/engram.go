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
	"errors"
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
//
// PathMutatedByClick/PathDir (D-9, T4-1) are the same "only reverse what click added" ownership
// tracking, but for the PATH-persistence side effect of EnsureEngramBinary's own
// persistPathToBinaryDir step — a SEPARATE mutation from the plugin install above, gated
// independently: a developer can have InstalledByClick==false (Engram was already installed) while
// still having PathMutatedByClick==true (their pre-existing binary happened to resolve from inside
// GoBinDir, and click's own EnsureOnPath call — which runs unconditionally, regardless of plugin
// ownership — actually added that dir to their PATH). RemoveEngramPlugin therefore checks these two
// ownership flags independently, not as a single combined gate.
type engramState struct {
	Version            string `json:"version"`
	BinaryPath         string `json:"binary_path"`
	Source             string `json:"source"`
	InstalledByClick   bool   `json:"installed_by_click"`
	PathMutatedByClick bool   `json:"path_mutated_by_click"`
	// PathDir is the LATEST directory a PATH-mutating SyncEngram run persisted. Kept ONLY for
	// backward-compatible JSON reads of state files written by v0.4.3 (before PathDirs existed) and
	// for human-readability in the on-disk JSON — new logic must read PathDirs instead, never this
	// field directly. See PathDirs' own doc comment for why a single latest-dir field is not enough.
	PathDir string `json:"path_dir"`
	// PathDirs (T4-1 follow-up) is EVERY directory a PATH-mutating SyncEngram run has ever persisted,
	// in first-added order, deduped. PathDir alone only ever tracked the latest such directory — if
	// GoBinDir moved TWICE across two separate SyncEngram runs (e.g. Go reinstalled with a different
	// GOPATH between two `click update` runs), the FIRST directory click added became untracked and
	// RemoveEngramPlugin could never reverse it, even though nothing incorrect was ever removed. This
	// is the source of truth removeClickOwnedPath iterates. A state file written before this field
	// existed (only PathDir set) is migrated in-memory by loadEngramState the first time it's read
	// after this change ships — see that function's own doc comment.
	PathDirs []string `json:"path_dirs"`
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
//
// PathMutatedByClick/PathDir ownership (D-9, T4-1): merged with the previously-persisted state the
// same way InstalledByClick already is — but NOT via the identical "found ? always keep existing"
// rule, because the two have different lifecycles. InstalledByClick is decided EXACTLY ONCE, the
// very first time SyncEngram ever runs against a given ClaudeHome (see that field's own doc
// comment) — every later run's derived "!alreadyInstalled" is meaningless noise and must always be
// discarded in favor of whatever was first recorded. PathMutatedByClick has no such single
// decisive-first-run moment: EnsureOnPath can legitimately fail (or never even be attempted — e.g.
// the binary hadn't yet started resolving from inside GoBinDir) on an earlier run and only succeed
// on a later one, so "this run mutated the PATH" must be able to flip false -> true on ANY run, not
// only the first. It is therefore OR-merged with the previously-persisted value — which still
// satisfies the one hard invariant that actually matters (mirroring InstalledByClick's own "never
// flip back" contract): once true, it can never flip back to false on a later idempotent
// (EnsureOnPath changed==false) run. PathDir mirrors this: preserved from the existing state on a
// non-mutating run, and only overwritten with THIS run's freshly-resolved dir when THIS run itself
// actually mutated the PATH (so a later GoBinDir move is still tracked correctly).
func SyncEngram(cfg Config, m *manifest.Manifest) (alreadyInstalled bool, pathWarning string, err error) {
	alreadyInstalled, err = SyncEngramPlugin(cfg)
	if err != nil {
		return false, "", err
	}
	// ensureEngramBinaryWithPathInfo is EnsureEngramBinary's own implementation, additionally
	// reporting whether this call's PATH-persistence attempt actually mutated the persisted PATH and
	// which directory — see that function's own doc comment for why EnsureEngramBinary's long-standing
	// public signature is left untouched for its other callers. It never fails this call — a missing
	// binary/toolchain is reported via remediation, not an error — so its own error return here is
	// reserved for genuine I/O failures resolving state, not "binary not found".
	binaryPath, _, _, pathMutated, pathDir, pathWarning, err := ensureEngramBinaryWithPathInfo(cfg, m.Engram.Version)
	if err != nil {
		return alreadyInstalled, "", err
	}

	installedByClick := !alreadyInstalled
	existing, found, err := loadEngramState(cfg)
	if err != nil {
		return alreadyInstalled, pathWarning, err
	}

	pathMutatedByClick := pathMutated
	pathDirToPersist := pathDir
	var pathDirsToPersist []string
	if found {
		installedByClick = existing.InstalledByClick
		pathMutatedByClick = existing.PathMutatedByClick || pathMutated
		if !pathMutated {
			pathDirToPersist = existing.PathDir
		}
		// existing.PathDirs is already migrated (seeded from a legacy existing.PathDir when needed)
		// by loadEngramState above — always start accumulation from it, never from the raw JSON.
		pathDirsToPersist = append(pathDirsToPersist, existing.PathDirs...)
	}
	if pathMutated && pathDir != "" && !containsPathDir(pathDirsToPersist, pathDir) {
		// THIS run mutated the PATH at pathDir and it isn't already tracked — append it (T4-1
		// follow-up). Deliberately append-only and deduped: a later GoBinDir move is tracked
		// alongside every earlier one, and a repeated idempotent run against the same dir never
		// grows the list.
		pathDirsToPersist = append(pathDirsToPersist, pathDir)
	}

	state := engramState{
		Version:            m.Engram.Version,
		BinaryPath:         binaryPath,
		Source:             m.Engram.Source,
		InstalledByClick:   installedByClick,
		PathMutatedByClick: pathMutatedByClick,
		PathDir:            pathDirToPersist,
		PathDirs:           pathDirsToPersist,
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

// RemoveEngramPlugin reverses SyncEngram, but ONLY when click's own state says click made the
// corresponding change in the first place. It reverses TWO independently-gated things (D-9, T4-1):
//
//   - The engram@engram plugin install itself — ONLY when state.InstalledByClick is true. If a
//     developer already had Engram working before running `click install`, click uninstall leaves
//     it running untouched.
//   - Click's own PATH mutation(s) from EnsureEngramBinary's PATH-persistence step — ONLY when
//     state.PathMutatedByClick is true AND state.PathDirs is non-empty (T4-1 follow-up: EVERY
//     directory ever added, not just the latest — see engramState.PathDirs' own doc comment). This
//     is checked INDEPENDENTLY of the plugin-ownership gate above, because it is a separate mutation
//     with its own ownership: EnsureOnPath runs unconditionally regardless of whether the plugin
//     itself was already installed, so a developer's pre-existing Engram binary happening to resolve
//     from inside GoBinDir can mean PathMutatedByClick==true even when InstalledByClick==false.
//
// A failure removing the PATH entry is surfaced back as pathWarning (never as err) — mirroring
// EnsureEngramBinary/SyncEngram's own "a PATH operation failure is a warning, never fatal" contract
// — so it can never abort the rest of `click uninstall`'s own steps.
//
// It is idempotent: safe to call when Engram was never touched by click, or has already been
// removed.
func RemoveEngramPlugin(cfg Config) (pathWarning string, err error) {
	state, found, err := loadEngramState(cfg)
	if err != nil {
		if errors.Is(err, errEngramStateCorrupted) {
			// Finding 3 (review-resilience WARNING): a truncated/hand-edited engram.json means click
			// can no longer tell what it owns here — the SAME safety property InstalledByClick/
			// PathMutatedByClick exist to guarantee ("only ever reverse what click itself added")
			// requires treating "we don't know" as "touch nothing", exactly like the found=false
			// branch below already does for a MISSING state file. The one difference: an absent file
			// is the expected, silent case (click never touched this machine); a corrupted one is
			// anomalous and worth telling the developer about — surfaced as a non-empty pathWarning
			// (never fatal), mirroring every other warning this function already produces, instead of
			// the previous hard error that used to abort the rest of `click uninstall`'s own steps
			// (the exact compound bug Finding 2 + Finding 3 describe together). The file itself is
			// left untouched (not deleted) so the developer can inspect or repair it manually.
			return fmt.Sprintf(
				"no se pudo leer el estado de Engram (%s): el archivo parece estar dañado, así que por seguridad no se modificó nada relacionado con Engram. Revíselo o elimínelo manualmente; si desea desinstalar Engram usted mismo, ejecute: claude plugin uninstall %s",
				cfg.EngramStatePath(), EngramPluginID,
			), nil
		}
		return "", err
	}
	if !found {
		// click's Install() never ran against this home (or ran before this feature existed) —
		// nothing click-managed to reverse.
		return "", nil
	}

	pathWarning = removeClickOwnedPath(state)

	if !state.InstalledByClick {
		// click never owned this plugin install; leave Engram alone, just drop click's own
		// bookkeeping. The PATH reversal above (if any) still applies independently.
		if err := removeEngramState(cfg); err != nil {
			return pathWarning, err
		}
		return pathWarning, nil
	}
	installed, err := HasInstalledPluginID(cfg, EngramPluginID)
	if err != nil {
		return pathWarning, err
	}
	if installed {
		runner := commandRunnerFactory()
		if err := uninstallPluginID(runner, engramPluginName, engramMarketplaceName); err != nil {
			return pathWarning, err
		}
		if err := removeMarketplace(runner, engramMarketplaceName); err != nil {
			return pathWarning, err
		}
	}
	if err := removeEngramState(cfg); err != nil {
		return pathWarning, err
	}
	return pathWarning, nil
}

// removeClickOwnedPath reverses EVERY PATH mutation click ever recorded in state (D-9, T4-1
// follow-up) via pathStoreFactory().RemoveFromPath — but ONLY when state actually recorded owning
// at least one: state.PathMutatedByClick must be true AND state.PathDirs must be non-empty
// (loadEngramState's migration guarantees PathDirs is populated whenever a legacy PathDir was set).
// This is the exact "only reverse what click added" safety rule this feature exists to guarantee —
// a state where click never mutated the PATH (PathMutatedByClick==false) must leave the developer's
// PATH completely untouched; RemoveFromPath is not even called in that case.
//
// Every tracked directory is attempted, even after an earlier one fails — a removal failure for one
// directory must never prevent attempting the others. Failures are folded into a single non-empty
// pathWarning string (one clause per failed directory, joined with "; "), never an error — see
// RemoveEngramPlugin's own doc comment for why a PATH-removal failure is always a warning, never
// fatal.
func removeClickOwnedPath(state engramState) (pathWarning string) {
	if !state.PathMutatedByClick || len(state.PathDirs) == 0 {
		return ""
	}
	if pathStoreFactory == nil {
		return ""
	}
	store := pathStoreFactory()
	var failures []string
	for _, dir := range state.PathDirs {
		if dir == "" {
			continue
		}
		if _, err := store.RemoveFromPath(dir); err != nil {
			failures = append(failures, fmt.Sprintf("no se pudo quitar %s del PATH persistente: %v", dir, err))
		}
	}
	if len(failures) == 0 {
		return ""
	}
	return strings.Join(failures, "; ")
}

// containsPathDir reports whether dir is already present in dirs, using sameDir's platform-aware
// comparison (case-insensitive on Windows, trailing-separator-normalized) — the same rule
// persistPathToBinaryDir/computeNewPath already use for PATH-entry comparisons — so a directory
// already tracked under a different case or trailing separator on Windows is correctly recognized
// as a dupe instead of appended again.
func containsPathDir(dirs []string, dir string) bool {
	for _, d := range dirs {
		if sameDir(d, dir) {
			return true
		}
	}
	return false
}

// loadEngramState reads and parses cfg's persisted engramState. Migration (T4-1 follow-up): a
// state file written by v0.4.3 or earlier has PathDir set but no PathDirs — this seeds
// PathDirs = []string{PathDir} in memory (never rewriting the file itself here) so an install
// upgrading from that version doesn't silently "forget" the one directory it already knew about.
// A state that already has PathDirs populated is left untouched by this step.
// errEngramStateCorrupted is a sentinel wrapped into loadEngramState's returned error specifically
// when the state file's JSON itself can't be parsed — as opposed to a genuine I/O failure reading it
// (permission denied, etc.), which stays a plain hard error. RemoveEngramPlugin checks
// errors.Is(err, errEngramStateCorrupted) to treat corrupted state as a safe, non-fatal "unknown
// ownership, touch nothing" case (Finding 3) instead of aborting.
var errEngramStateCorrupted = errors.New("engram state file is corrupted")

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
		return engramState{}, false, fmt.Errorf("installer: parse engram state: %w: %w", errEngramStateCorrupted, err)
	}
	if len(state.PathDirs) == 0 && state.PathDir != "" {
		state.PathDirs = []string{state.PathDir}
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
// both `@latest` and the manifest-pinned `@v1.19.0` resolve and produce a binary that actually runs
// (v1.19.0 re-verified installing cleanly via `go install` when the pin was bumped from v1.15.3).
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
		"El binario de engram no se encuentra en el PATH. Instálelo manualmente con:\n"+
			"  %s\n"+
			"Después asegúrese de que su directorio de binarios de Go (GOPATH/bin, o GOBIN si lo definió) esté en el PATH.\n"+
			"En macOS también puede usar: brew install gentleman-programming/tap/engram",
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
	path, resolvable, remediation, _, _, pathWarning, err = ensureEngramBinaryWithPathInfo(cfg, version)
	return path, resolvable, remediation, pathWarning, err
}

// ensureEngramBinaryWithPathInfo is EnsureEngramBinary's real implementation, additionally
// reporting whether THIS call's PATH-persistence attempt actually mutated the user's persisted
// PATH (pathMutated) and which directory (pathDir) — the two extra signals SyncEngram needs (D-9,
// T4-1) to record engramState.PathMutatedByClick/PathDir, so `click uninstall` can later safely
// reverse ONLY what click itself added. EnsureEngramBinary's own long-standing public signature —
// used by every existing caller/test in this package, plus `click doctor`'s checkEngramBinary and
// `click install`'s non-fatal remediation fallback — is intentionally left unchanged: those callers
// never needed pathMutated/pathDir, only pathWarning, so widening EnsureEngramBinary itself would
// force every one of them (and every existing test) to change for no behavioral benefit.
func ensureEngramBinaryWithPathInfo(cfg Config, version string) (path string, resolvable bool, remediation string, pathMutated bool, pathDir string, pathWarning string, err error) {
	path, resolvable, err = EngramBinaryResolvable(cfg)
	if err != nil {
		return "", false, "", false, "", "", err
	}
	if resolvable {
		pathMutated, pathDir, pathWarning = persistPathToBinaryDir(cfg, path)
		return path, true, "", pathMutated, pathDir, pathWarning, nil
	}
	if !goAvailable() {
		return path, false, EngramBinaryRemediationMessage(version), false, "", "", nil
	}

	runner := commandRunnerFactory()
	// Best-effort: any failure here (network, module-proxy hiccup, etc.) is folded into the same
	// "still not resolvable" remediation path below rather than propagated as a hard error.
	_ = runner.Run("go", "install", engramBinaryModulePath+"@"+version)

	path, resolvable, err = EngramBinaryResolvable(cfg)
	if err != nil {
		return "", false, "", false, "", "", err
	}
	if !resolvable {
		// JD-001 fix: `resolvable` reflects THIS process's own LookPath-based prediction of whether
		// a bare `command: "engram"` MCP launch would succeed right now — and a child `go install`
		// process can never mutate that. So on a genuinely fresh install (the exact scenario this
		// whole feature exists to fix), resolvable legitimately stays false even after `go install`
		// has verifiably written the binary to disk. PATH persistence must therefore be attempted
		// independently of `resolvable`: check directly whether the binary now exists on disk at
		// GoBinDir(cfg), and if so, persist that directory regardless of what LookPath reports.
		pathMutated, pathDir, pathWarning = pathWarningAfterGoInstall(cfg)
		return path, false, EngramBinaryRemediationMessage(version), pathMutated, pathDir, pathWarning, nil
	}
	pathMutated, pathDir, pathWarning = persistPathToBinaryDir(cfg, path)
	return path, true, "", pathMutated, pathDir, pathWarning, nil
}

// pathWarningAfterGoInstall independently checks whether the Engram binary now exists on disk at
// GoBinDir(cfg) — regardless of what the LookPath-based EngramBinaryResolvable reports — and, if
// so, attempts to persist that directory onto the user's PATH via persistPathToBinaryDir (JD-001).
// It never errors itself: GoBinDir failing to resolve, or the binary simply not existing there
// (e.g. no Go toolchain, or `go install` itself failed), both mean "not attempted" (mutated=false,
// dir="", empty pathWarning) — matching persistPathToBinaryDir's own no-error contract. mutated/dir
// (D-9, T4-1) are persistPathToBinaryDir's own extra ownership-tracking signals, forwarded
// unchanged.
func pathWarningAfterGoInstall(cfg Config) (mutated bool, dir string, pathWarning string) {
	gobin, err := GoBinDir(cfg)
	if err != nil {
		return false, "", ""
	}
	candidate := filepath.Join(gobin, engramBinaryName())
	info, statErr := os.Stat(candidate)
	if statErr != nil || info.IsDir() {
		return false, "", ""
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
// mutated/dir (D-9, T4-1) are the ownership-tracking signals SyncEngram needs to record
// engramState.PathMutatedByClick/PathDir: mutated reports whether pathStoreFactory().EnsureOnPath
// ACTUALLY changed the persisted PATH this call (i.e. its own changed==true — including the case
// where the registry write itself durably succeeded but the WM_SETTINGCHANGE broadcast afterward
// failed; the mutation still happened), and dir is the exact directory that was (or would have
// been) persisted — gobin — regardless of whether this specific call mutated anything, so a caller
// merging against previously-persisted state always knows which dir an OLDER successful call must
// have used. Both are false/empty on every early-return path below, where no EnsureOnPath call is
// even made.
//
// It never returns an error itself: GoBinDir failing to resolve, or the binary living outside
// GoBinDir, both mean "not attempted" (mutated=false, dir="", empty pathWarning) — not a warning.
// Only an actual pathStoreFactory().EnsureOnPath failure produces a non-empty, wrapped pathWarning
// (mutated then reflects whatever EnsureOnPath itself reported — see above).
// pathStoreFactory().EnsureOnPath is already idempotent (a no-op, changed=false, on a directory
// already present in the persisted PATH), so calling it directly here — rather than pre-checking
// PersistedPathContains first — never produces redundant writes on its own.
func persistPathToBinaryDir(cfg Config, binaryPath string) (mutated bool, dir string, pathWarning string) {
	gobin, err := GoBinDir(cfg)
	if err != nil {
		// No `go env`-resolvable GOBIN/GOPATH at all — nothing to compare binaryPath against.
		return false, "", ""
	}
	if !sameDir(filepath.Dir(binaryPath), gobin) {
		// Resolved from somewhere other than GoBinDir — pathStore has nothing to do here.
		return false, "", ""
	}
	if pathStoreFactory == nil {
		// No platform pathStore wired in (e.g. a build with neither pathenv_windows.go nor
		// pathenv_unix.go compiled in, which never happens in a real release build, only in some
		// narrowly-scoped unit tests) — nothing to attempt.
		return false, "", ""
	}
	changed, err := pathStoreFactory().EnsureOnPath(gobin)
	if err != nil {
		return changed, gobin, fmt.Sprintf("no se pudo agregar %s al PATH persistente: %v", gobin, err)
	}
	return changed, gobin, ""
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
