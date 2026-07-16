package installer

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/manifest"
)

// seedResolvableEngramBinary points CLICK_ENGRAM_BINARY_PATH at a real, existing dummy file so
// EnsureEngramBinary's initial EngramBinaryResolvable check short-circuits as already-resolvable —
// neutralizing Slice 3b's new go-install provisioning path for tests that only care about
// SyncEngram's plugin-registration behavior (unrelated to binary provisioning). Without this, these
// tests would non-deterministically issue (or not issue) an extra `go install` command depending on
// whether the machine running them happens to have a real `go` toolchain and/or a real `engram`
// binary on PATH.
func seedResolvableEngramBinary(t *testing.T) {
	t.Helper()
	binaryPath := filepath.Join(t.TempDir(), "engram.exe")
	if err := os.WriteFile(binaryPath, []byte("binary"), 0o644); err != nil {
		t.Fatalf("WriteFile(binary) error = %v", err)
	}
	t.Setenv(engramBinaryPathEnvOverride, binaryPath)
}

// TestSyncEngram_InstallsWhenNotPresent covers the common case: a developer who has never touched
// Engram runs `click install`. SyncEngram must register the Engram marketplace (no --sparse — the
// engram repo has no plugins/ directory, only plugin/claude-code/, confirmed against the real CLI
// in Step 0 / spike-e-engram-install.md) and install engram@engram, then record click-owned state.
func TestSyncEngram_InstallsWhenNotPresent(t *testing.T) {
	claudeHome := t.TempDir()
	cfg := Config{ClaudeHome: claudeHome}
	runner := newFakeCommandRunner(cfg)
	restoreRunner := SetCommandRunnerFactoryForTests(func() CommandRunner { return runner })
	defer restoreRunner()
	restoreSource := SetEngramMarketplaceSourceForTests("https://github.com/Gentleman-Programming/engram")
	defer restoreSource()
	seedResolvableEngramBinary(t)

	m, err := manifest.Load()
	if err != nil {
		t.Fatalf("manifest.Load() error = %v", err)
	}

	alreadyInstalled, pathWarning, err := SyncEngram(cfg, m)
	if err != nil {
		t.Fatalf("SyncEngram() error = %v", err)
	}
	if alreadyInstalled {
		t.Fatal("SyncEngram() alreadyInstalled = true on a fresh home, want false")
	}
	// seedResolvableEngramBinary points CLICK_ENGRAM_BINARY_PATH outside any `go env`-resolved
	// GoBinDir, and this test's fake runner never stubs "go env GOBIN"/"go env GOPATH" — PATH
	// persistence must not even be attempted.
	if pathWarning != "" {
		t.Fatalf("SyncEngram() pathWarning = %q, want empty (not attempted)", pathWarning)
	}

	want := []commandInvocation{
		{Name: "claude", Args: []string{"plugin", "marketplace", "add", "https://github.com/Gentleman-Programming/engram"}},
		{Name: "claude", Args: []string{"plugin", "install", "engram@engram"}},
		// EnsureEngramBinary's Phase 4 signal-wiring (D-5): once resolvable, it queries GoBinDir to
		// decide whether to attempt PATH persistence — these two lookups are genuinely issued even
		// though they end up failing (unstubbed) and no pathStore call follows.
		{Name: "go", Args: []string{"env", "GOBIN"}},
		{Name: "go", Args: []string{"env", "GOPATH"}},
	}
	if !reflect.DeepEqual(runner.commands, want) {
		t.Fatalf("runner.commands = %#v, want %#v (no --sparse: engram's repo layout has no plugins/ dir)", runner.commands, want)
	}

	installed, err := HasInstalledPluginID(cfg, EngramPluginID)
	if err != nil {
		t.Fatalf("HasInstalledPluginID() error = %v", err)
	}
	if !installed {
		t.Fatal("SyncEngram() did not register engram@engram")
	}

	stateData, err := os.ReadFile(cfg.EngramStatePath())
	if err != nil {
		t.Fatalf("ReadFile(EngramStatePath) error = %v", err)
	}
	var state engramState
	if err := json.Unmarshal(stateData, &state); err != nil {
		t.Fatalf("json.Unmarshal(state) error = %v", err)
	}
	if !state.InstalledByClick {
		t.Fatal("state.InstalledByClick = false after a fresh SyncEngram(), want true")
	}
	if state.Version != m.Engram.Version {
		t.Fatalf("state.Version = %q, want %q", state.Version, m.Engram.Version)
	}
	if state.Source != m.Engram.Source {
		t.Fatalf("state.Source = %q, want %q", state.Source, m.Engram.Source)
	}
}

// TestSyncEngram_SkipsWhenAlreadyInstalled is the critical "respect an existing developer setup"
// contract: many developers (including real machines this was verified against) already have
// Engram installed and working. click must never reinstall or clobber it — just detect it and
// move on with a friendly skip, recording that click did NOT own this install (so Uninstall later
// knows to leave it alone).
func TestSyncEngram_SkipsWhenAlreadyInstalled(t *testing.T) {
	claudeHome := t.TempDir()
	cfg := Config{ClaudeHome: claudeHome}
	runner := newFakeCommandRunner(cfg)
	restoreRunner := SetCommandRunnerFactoryForTests(func() CommandRunner { return runner })
	defer restoreRunner()

	seedEngramAlreadyInstalled(t, cfg)
	seedResolvableEngramBinary(t)

	m, err := manifest.Load()
	if err != nil {
		t.Fatalf("manifest.Load() error = %v", err)
	}

	alreadyInstalled, _, err := SyncEngram(cfg, m)
	if err != nil {
		t.Fatalf("SyncEngram() error = %v", err)
	}
	if !alreadyInstalled {
		t.Fatal("SyncEngram() alreadyInstalled = false when engram@engram was pre-seeded as installed, want true")
	}
	// No `claude plugin ...` command must be issued against an already-installed Engram (no
	// reinstall/clobber). The two "go env GOBIN"/"go env GOPATH" commands ARE expected here: they
	// come from EnsureEngramBinary's Phase 4 signal-wiring (D-5) unconditionally checking whether
	// the resolved binary lives inside GoBinDir before attempting PATH persistence — unrelated to,
	// and not a regression of, the plugin-registration skip this test guards.
	for _, inv := range runner.commands {
		if inv.Name == "claude" {
			t.Fatalf("SyncEngram() issued a claude command %#v against an already-installed Engram, want none (no reinstall/clobber)", inv)
		}
	}

	stateData, err := os.ReadFile(cfg.EngramStatePath())
	if err != nil {
		t.Fatalf("ReadFile(EngramStatePath) error = %v", err)
	}
	var state engramState
	if err := json.Unmarshal(stateData, &state); err != nil {
		t.Fatalf("json.Unmarshal(state) error = %v", err)
	}
	if state.InstalledByClick {
		t.Fatal("state.InstalledByClick = true for a pre-existing install click never touched, want false")
	}
}

// TestSyncEngram_SecondRunPreservesClickOwnership is a regression test for a real bug found
// during Step 0 end-to-end verification against the actual `claude` CLI: `click install` calls
// SyncEngram every run (it's meant to be idempotent), and by the SECOND run engram@engram is
// already installed — by click itself. A naive "alreadyInstalled implies click didn't own it"
// derivation flips InstalledByClick to false on that second run, which then made
// RemoveEngramPlugin wrongly skip removing an install click actually added. Ownership must be
// decided once (the first time click ever touches this ClaudeHome) and preserved afterward.
func TestSyncEngram_SecondRunPreservesClickOwnership(t *testing.T) {
	claudeHome := t.TempDir()
	cfg := Config{ClaudeHome: claudeHome}
	runner := newFakeCommandRunner(cfg)
	restoreRunner := SetCommandRunnerFactoryForTests(func() CommandRunner { return runner })
	defer restoreRunner()
	restoreSource := SetEngramMarketplaceSourceForTests("https://github.com/Gentleman-Programming/engram")
	defer restoreSource()
	seedResolvableEngramBinary(t)

	m, err := manifest.Load()
	if err != nil {
		t.Fatalf("manifest.Load() error = %v", err)
	}

	if _, _, err := SyncEngram(cfg, m); err != nil {
		t.Fatalf("first SyncEngram() error = %v", err)
	}
	alreadyInstalled, _, err := SyncEngram(cfg, m)
	if err != nil {
		t.Fatalf("second SyncEngram() error = %v", err)
	}
	if !alreadyInstalled {
		t.Fatal("second SyncEngram() alreadyInstalled = false, want true (idempotent skip)")
	}

	stateData, err := os.ReadFile(cfg.EngramStatePath())
	if err != nil {
		t.Fatalf("ReadFile(EngramStatePath) error = %v", err)
	}
	var state engramState
	if err := json.Unmarshal(stateData, &state); err != nil {
		t.Fatalf("json.Unmarshal(state) error = %v", err)
	}
	if !state.InstalledByClick {
		t.Fatal("state.InstalledByClick flipped to false after a second, idempotent SyncEngram() run — click's own install ownership must be preserved, not re-derived from current install state")
	}

	// The real-world symptom: Uninstall must still remove an install click owns, even after
	// multiple `click install`/`click update` runs in between.
	if _, err := RemoveEngramPlugin(cfg); err != nil {
		t.Fatalf("RemoveEngramPlugin() error = %v", err)
	}
	installed, err := HasInstalledPluginID(cfg, EngramPluginID)
	if err != nil {
		t.Fatalf("HasInstalledPluginID() error = %v", err)
	}
	if installed {
		t.Fatal("RemoveEngramPlugin() left a click-owned engram install registered after two SyncEngram() runs")
	}
}

// TestRemoveEngramPlugin_RemovesWhenClickInstalledIt covers the normal uninstall path: click
// installed Engram itself, so `click uninstall` reverses that.
func TestRemoveEngramPlugin_RemovesWhenClickInstalledIt(t *testing.T) {
	claudeHome := t.TempDir()
	cfg := Config{ClaudeHome: claudeHome}
	runner := newFakeCommandRunner(cfg)
	restoreRunner := SetCommandRunnerFactoryForTests(func() CommandRunner { return runner })
	defer restoreRunner()
	restoreSource := SetEngramMarketplaceSourceForTests("https://github.com/Gentleman-Programming/engram")
	defer restoreSource()
	seedResolvableEngramBinary(t)

	m, err := manifest.Load()
	if err != nil {
		t.Fatalf("manifest.Load() error = %v", err)
	}
	if _, _, err := SyncEngram(cfg, m); err != nil {
		t.Fatalf("SyncEngram() error = %v", err)
	}

	if _, err := RemoveEngramPlugin(cfg); err != nil {
		t.Fatalf("RemoveEngramPlugin() error = %v", err)
	}

	installed, err := HasInstalledPluginID(cfg, EngramPluginID)
	if err != nil {
		t.Fatalf("HasInstalledPluginID() error = %v", err)
	}
	if installed {
		t.Fatal("RemoveEngramPlugin() left engram@engram registered after click owned the install")
	}
	if _, err := os.Stat(cfg.EngramStatePath()); !os.IsNotExist(err) {
		t.Fatalf("RemoveEngramPlugin() left the engram state file behind (err = %v)", err)
	}
}

// TestRemoveEngramPlugin_RespectsPreExistingInstall is the flip side: if Engram was already
// installed before click touched this machine, `click uninstall` must NOT remove it — only clean
// up click's own bookkeeping file.
func TestRemoveEngramPlugin_RespectsPreExistingInstall(t *testing.T) {
	claudeHome := t.TempDir()
	cfg := Config{ClaudeHome: claudeHome}
	runner := newFakeCommandRunner(cfg)
	restoreRunner := SetCommandRunnerFactoryForTests(func() CommandRunner { return runner })
	defer restoreRunner()

	seedEngramAlreadyInstalled(t, cfg)
	seedResolvableEngramBinary(t)

	m, err := manifest.Load()
	if err != nil {
		t.Fatalf("manifest.Load() error = %v", err)
	}
	if _, _, err := SyncEngram(cfg, m); err != nil {
		t.Fatalf("SyncEngram() error = %v", err)
	}

	if _, err := RemoveEngramPlugin(cfg); err != nil {
		t.Fatalf("RemoveEngramPlugin() error = %v", err)
	}

	// No `claude plugin ...` command must be issued against a pre-existing install (neither
	// SyncEngram's plugin-registration skip nor RemoveEngramPlugin's respect-what-click-doesn't-own
	// path touch the plugin). The two "go env GOBIN"/"go env GOPATH" commands are SyncEngram's own
	// EnsureEngramBinary PATH-persistence check (D-5) — unrelated to this test's guard.
	for _, inv := range runner.commands {
		if inv.Name == "claude" {
			t.Fatalf("RemoveEngramPlugin() issued a claude command %#v against a pre-existing install, want none", inv)
		}
	}
	installed, err := HasInstalledPluginID(cfg, EngramPluginID)
	if err != nil {
		t.Fatalf("HasInstalledPluginID() error = %v", err)
	}
	if !installed {
		t.Fatal("RemoveEngramPlugin() removed a pre-existing Engram install it never owned")
	}
	if _, err := os.Stat(cfg.EngramStatePath()); !os.IsNotExist(err) {
		t.Fatalf("RemoveEngramPlugin() left click's own state file behind after respecting a pre-existing install (err = %v)", err)
	}
}

// TestRemoveEngramPlugin_NoopWhenNeverSynced covers a `click uninstall` run against a home where
// `click install` never ran (or ran before this feature existed): nothing to reverse, no error.
func TestRemoveEngramPlugin_NoopWhenNeverSynced(t *testing.T) {
	claudeHome := t.TempDir()
	cfg := Config{ClaudeHome: claudeHome}
	runner := newFakeCommandRunner(cfg)
	restoreRunner := SetCommandRunnerFactoryForTests(func() CommandRunner { return runner })
	defer restoreRunner()

	if _, err := RemoveEngramPlugin(cfg); err != nil {
		t.Fatalf("RemoveEngramPlugin() on a never-synced home error = %v, want nil", err)
	}
	if len(runner.commands) != 0 {
		t.Fatalf("RemoveEngramPlugin() issued commands %#v on a never-synced home, want zero", runner.commands)
	}
}

// TestRemoveEngramPlugin_ReversesClickOwnedPath is D-9's core uninstall-reversal contract: when
// click's own state recorded PathMutatedByClick=true and a PathDir, RemoveEngramPlugin must call
// pathStoreFactory().RemoveFromPath with exactly that dir.
func TestRemoveEngramPlugin_ReversesClickOwnedPath(t *testing.T) {
	claudeHome := t.TempDir()
	cfg := Config{ClaudeHome: claudeHome}
	runner := newFakeCommandRunner(cfg)
	restoreRunner := SetCommandRunnerFactoryForTests(func() CommandRunner { return runner })
	defer restoreRunner()

	dir := filepath.Join(t.TempDir(), "gobin")
	if err := writeJSONFile(cfg.EngramStatePath(), engramState{
		InstalledByClick:   true,
		PathMutatedByClick: true,
		PathDir:            dir,
	}); err != nil {
		t.Fatalf("writeJSONFile(EngramStatePath) error = %v", err)
	}

	store := &fakeConfigurablePathStore{}
	restoreStore := SetPathStoreFactoryForTests(func() pathStore { return store })
	defer restoreStore()

	pathWarning, err := RemoveEngramPlugin(cfg)
	if err != nil {
		t.Fatalf("RemoveEngramPlugin() error = %v", err)
	}
	if pathWarning != "" {
		t.Fatalf("RemoveEngramPlugin() pathWarning = %q, want empty on a successful reversal", pathWarning)
	}
	if len(store.removeFromPathCalls) != 1 || store.removeFromPathCalls[0] != dir {
		t.Fatalf("store.removeFromPathCalls = %#v, want exactly one call with %q", store.removeFromPathCalls, dir)
	}
}

// TestRemoveEngramPlugin_DoesNotTouchPathWhenNotClickOwned is D-9's "only reverse what click
// added" safety rule: when state.PathMutatedByClick is false (click never mutated the PATH itself),
// RemoveEngramPlugin must never call RemoveFromPath at all — even though state.PathDir happens to
// be set (e.g. a stale/legacy value from before this ownership flag existed), it must not be
// trusted on its own.
func TestRemoveEngramPlugin_DoesNotTouchPathWhenNotClickOwned(t *testing.T) {
	claudeHome := t.TempDir()
	cfg := Config{ClaudeHome: claudeHome}
	runner := newFakeCommandRunner(cfg)
	restoreRunner := SetCommandRunnerFactoryForTests(func() CommandRunner { return runner })
	defer restoreRunner()

	if err := writeJSONFile(cfg.EngramStatePath(), engramState{
		InstalledByClick:   true,
		PathMutatedByClick: false,
		PathDir:            filepath.Join(t.TempDir(), "gobin"),
	}); err != nil {
		t.Fatalf("writeJSONFile(EngramStatePath) error = %v", err)
	}

	store := &fakeConfigurablePathStore{}
	restoreStore := SetPathStoreFactoryForTests(func() pathStore { return store })
	defer restoreStore()

	pathWarning, err := RemoveEngramPlugin(cfg)
	if err != nil {
		t.Fatalf("RemoveEngramPlugin() error = %v", err)
	}
	if pathWarning != "" {
		t.Fatalf("RemoveEngramPlugin() pathWarning = %q, want empty (nothing to reverse)", pathWarning)
	}
	if len(store.removeFromPathCalls) != 0 {
		t.Fatalf("store.removeFromPathCalls = %#v, want zero — click never recorded mutating the PATH itself", store.removeFromPathCalls)
	}
}

// TestRemoveEngramPlugin_DoesNotTouchPathWhenPathDirEmpty triangulates the same safety rule against
// the other half of the guard: PathMutatedByClick alone, without a recorded PathDir, must also
// never call RemoveFromPath (there is nothing to pass it).
func TestRemoveEngramPlugin_DoesNotTouchPathWhenPathDirEmpty(t *testing.T) {
	claudeHome := t.TempDir()
	cfg := Config{ClaudeHome: claudeHome}
	runner := newFakeCommandRunner(cfg)
	restoreRunner := SetCommandRunnerFactoryForTests(func() CommandRunner { return runner })
	defer restoreRunner()

	if err := writeJSONFile(cfg.EngramStatePath(), engramState{
		InstalledByClick:   true,
		PathMutatedByClick: true,
		PathDir:            "",
	}); err != nil {
		t.Fatalf("writeJSONFile(EngramStatePath) error = %v", err)
	}

	store := &fakeConfigurablePathStore{}
	restoreStore := SetPathStoreFactoryForTests(func() pathStore { return store })
	defer restoreStore()

	if _, err := RemoveEngramPlugin(cfg); err != nil {
		t.Fatalf("RemoveEngramPlugin() error = %v", err)
	}
	if len(store.removeFromPathCalls) != 0 {
		t.Fatalf("store.removeFromPathCalls = %#v, want zero — PathDir was empty", store.removeFromPathCalls)
	}
}

// TestRemoveEngramPlugin_PathRemovalFailureSurfacesAsWarningNotError covers the non-fatal
// requirement: a RemoveFromPath failure must surface as a non-empty pathWarning while
// RemoveEngramPlugin itself still returns err=nil AND still completes the rest of its own
// reversal (plugin uninstall, state file removal) — mirroring EnsureEngramBinary/SyncEngram's own
// "a PATH operation failure is a warning, never fatal" contract.
func TestRemoveEngramPlugin_PathRemovalFailureSurfacesAsWarningNotError(t *testing.T) {
	claudeHome := t.TempDir()
	cfg := Config{ClaudeHome: claudeHome}
	runner := newFakeCommandRunner(cfg)
	restoreRunner := SetCommandRunnerFactoryForTests(func() CommandRunner { return runner })
	defer restoreRunner()
	restoreSource := SetEngramMarketplaceSourceForTests("https://github.com/Gentleman-Programming/engram")
	defer restoreSource()
	seedResolvableEngramBinary(t)

	m, err := manifest.Load()
	if err != nil {
		t.Fatalf("manifest.Load() error = %v", err)
	}
	if _, _, err := SyncEngram(cfg, m); err != nil {
		t.Fatalf("SyncEngram() error = %v", err)
	}

	dir := filepath.Join(t.TempDir(), "gobin")
	state := readEngramStateForTest(t, cfg)
	state.PathMutatedByClick = true
	state.PathDir = dir
	if err := writeJSONFile(cfg.EngramStatePath(), state); err != nil {
		t.Fatalf("writeJSONFile(EngramStatePath) error = %v", err)
	}

	removeErr := errors.New("acceso denegado al registro")
	store := &fakeConfigurablePathStore{removeFromPathErr: removeErr}
	restoreStore := SetPathStoreFactoryForTests(func() pathStore { return store })
	defer restoreStore()

	pathWarning, err := RemoveEngramPlugin(cfg)
	if err != nil {
		t.Fatalf("RemoveEngramPlugin() error = %v, want nil — a PATH-removal failure must not fail uninstall", err)
	}
	if pathWarning == "" {
		t.Fatal("RemoveEngramPlugin() pathWarning = \"\", want a non-empty warning")
	}
	if !strings.Contains(pathWarning, removeErr.Error()) {
		t.Fatalf("pathWarning = %q, want it to wrap %q", pathWarning, removeErr.Error())
	}
	installed, err := HasInstalledPluginID(cfg, EngramPluginID)
	if err != nil {
		t.Fatalf("HasInstalledPluginID() error = %v", err)
	}
	if installed {
		t.Fatal("RemoveEngramPlugin() left engram@engram registered despite the unrelated PATH-removal failure")
	}
	if _, err := os.Stat(cfg.EngramStatePath()); !os.IsNotExist(err) {
		t.Fatalf("RemoveEngramPlugin() left the engram state file behind despite the unrelated PATH-removal failure (err = %v)", err)
	}
}

// TestEngramBinaryResolvable_PrefersEnvOverride guards `click doctor`'s ability to check whether
// the Engram binary the plugin's bundled .mcp.json bare `command: "engram"` will actually resolve
// on PATH — the residual fragile part confirmed in Step 0 (spike-e-engram-install.md).
func TestEngramBinaryResolvable_PrefersEnvOverride(t *testing.T) {
	claudeHome := t.TempDir()
	cfg := Config{ClaudeHome: claudeHome}
	binaryPath := filepath.Join(t.TempDir(), "engram.exe")
	if err := os.WriteFile(binaryPath, []byte("binary"), 0o644); err != nil {
		t.Fatalf("WriteFile(binary) error = %v", err)
	}
	t.Setenv(engramBinaryPathEnvOverride, binaryPath)

	path, ok, err := EngramBinaryResolvable(cfg)
	if err != nil {
		t.Fatalf("EngramBinaryResolvable() error = %v", err)
	}
	if !ok {
		t.Fatalf("EngramBinaryResolvable() ok = false for an existing override binary at %s", path)
	}
	if path != binaryPath {
		t.Fatalf("EngramBinaryResolvable() path = %q, want %q", path, binaryPath)
	}
}

// TestEngramBinaryResolvable_MissingBinary covers the "not on PATH, not at the default location"
// case doctor must surface as unhealthy rather than silently reporting a phantom path as fine.
func TestEngramBinaryResolvable_MissingBinary(t *testing.T) {
	claudeHome := t.TempDir()
	cfg := Config{ClaudeHome: claudeHome}
	// Point the override at a path that does not exist, so resolution succeeds (no LookPath call)
	// but the existence check fails — deterministic without touching the real PATH.
	t.Setenv(engramBinaryPathEnvOverride, filepath.Join(claudeHome, "does-not-exist", "engram.exe"))

	_, ok, err := EngramBinaryResolvable(cfg)
	if err != nil {
		t.Fatalf("EngramBinaryResolvable() error = %v", err)
	}
	if ok {
		t.Fatal("EngramBinaryResolvable() ok = true for a binary path that does not exist on disk")
	}
}

// fakeBinaryLookup fakes PATH resolution for EnsureEngramBinary/ResolveEngramBinaryPath's
// BinaryLookup dependency, the same factory-injected pattern CommandRunner already uses (see
// plugins.go's commandRunnerFactory). Its map is mutated by fakeGoInstallRunner below to simulate a
// `go install` run making a previously-missing binary newly resolvable, without ever touching a
// real developer machine's PATH.
type fakeBinaryLookup struct {
	resolved map[string]string
}

func (f *fakeBinaryLookup) LookPath(name string) (string, error) {
	if path, ok := f.resolved[name]; ok {
		return path, nil
	}
	return "", errors.New("fakeBinaryLookup: not found: " + name)
}

// fakeGoInstallRunner is a CommandRunner fake purpose-built for exercising EnsureEngramBinary's
// `go install` step in isolation: when it sees `go install <module>@<version>`, it writes a dummy
// binary file at binaryPath and registers it in the shared fakeBinaryLookup, simulating what a real
// `go install` does to PATH resolution (GOPATH/bin gaining a new binary) — reusing the existing
// CommandRunner interface rather than inventing a second command-running abstraction.
type fakeGoInstallRunner struct {
	commands   []commandInvocation
	lookup     *fakeBinaryLookup
	binaryName string
	binaryPath string
	failWith   error
}

func (f *fakeGoInstallRunner) Run(name string, args ...string) error {
	f.commands = append(f.commands, commandInvocation{Name: name, Args: append([]string(nil), args...)})
	if f.failWith != nil {
		return f.failWith
	}
	if name == "go" && len(args) > 0 && args[0] == "install" {
		if err := os.WriteFile(f.binaryPath, []byte("binary"), 0o644); err != nil {
			return err
		}
		f.lookup.resolved[f.binaryName] = f.binaryPath
	}
	return nil
}

func (f *fakeGoInstallRunner) Output(name string, args ...string) ([]byte, error) {
	f.commands = append(f.commands, commandInvocation{Name: name, Args: append([]string(nil), args...)})
	return []byte{}, nil
}

// TestEnsureEngramBinary_AlreadyResolvable_NoInstall covers the idempotent no-op case: the binary
// is already resolvable (a developer had it on PATH before `click install` ever ran), so no `go
// install` command is issued at all.
func TestEnsureEngramBinary_AlreadyResolvable_NoInstall(t *testing.T) {
	claudeHome := t.TempDir()
	cfg := Config{ClaudeHome: claudeHome}
	existingPath := filepath.Join(t.TempDir(), "engram.exe")
	if err := os.WriteFile(existingPath, []byte("binary"), 0o644); err != nil {
		t.Fatalf("WriteFile(binary) error = %v", err)
	}

	lookup := &fakeBinaryLookup{resolved: map[string]string{engramBinaryName(): existingPath}}
	restoreLookup := SetBinaryLookupFactoryForTests(func() BinaryLookup { return lookup })
	defer restoreLookup()

	runner := newFakeCommandRunner(cfg)
	restoreRunner := SetCommandRunnerFactoryForTests(func() CommandRunner { return runner })
	defer restoreRunner()

	path, resolvable, remediation, pathWarning, err := EnsureEngramBinary(cfg, "v1.15.3")
	if err != nil {
		t.Fatalf("EnsureEngramBinary() error = %v", err)
	}
	if !resolvable {
		t.Fatal("EnsureEngramBinary() resolvable = false for an already-resolvable binary")
	}
	if remediation != "" {
		t.Fatalf("EnsureEngramBinary() remediation = %q, want empty when already resolvable", remediation)
	}
	if path != existingPath {
		t.Fatalf("EnsureEngramBinary() path = %q, want %q", path, existingPath)
	}
	// No `go install` must be issued for an already-resolvable binary. EnsureEngramBinary's Phase 4
	// signal-wiring (D-5) DOES still issue "go env GOBIN"/"go env GOPATH" — it unconditionally
	// checks whether the resolved binary lives inside GoBinDir before attempting PATH persistence —
	// unrelated to, and not a regression of, this test's "no go install" guard.
	for _, inv := range runner.commands {
		if inv.Name == "go" && len(inv.Args) > 0 && inv.Args[0] == "install" {
			t.Fatalf("EnsureEngramBinary() issued a go install command %#v for an already-resolvable binary, want none", inv)
		}
	}
	// existingPath is NOT inside a `go env`-resolved GoBinDir (the fake runner never stubs "go env
	// GOBIN"/"go env GOPATH", so GoBinDir errors) — PATH persistence must not even be attempted.
	if pathWarning != "" {
		t.Fatalf("EnsureEngramBinary() pathWarning = %q, want empty when GoBinDir cannot be resolved (not attempted)", pathWarning)
	}
}

// TestEnsureEngramBinary_MissingWithGoPresent_RunsGoInstall covers the core provisioning path: the
// binary is missing but Go is on PATH, so EnsureEngramBinary must run exactly
// `go install github.com/Gentleman-Programming/engram/cmd/engram@<version>` via the CommandRunner
// interface, then re-check resolvability.
func TestEnsureEngramBinary_MissingWithGoPresent_RunsGoInstall(t *testing.T) {
	claudeHome := t.TempDir()
	cfg := Config{ClaudeHome: claudeHome}
	binaryPath := filepath.Join(t.TempDir(), "engram.exe")

	lookup := &fakeBinaryLookup{resolved: map[string]string{"go": "/usr/bin/go"}}
	restoreLookup := SetBinaryLookupFactoryForTests(func() BinaryLookup { return lookup })
	defer restoreLookup()

	runner := &fakeGoInstallRunner{lookup: lookup, binaryName: engramBinaryName(), binaryPath: binaryPath}
	restoreRunner := SetCommandRunnerFactoryForTests(func() CommandRunner { return runner })
	defer restoreRunner()

	path, resolvable, remediation, pathWarning, err := EnsureEngramBinary(cfg, "v1.15.3")
	if err != nil {
		t.Fatalf("EnsureEngramBinary() error = %v", err)
	}
	if !resolvable {
		t.Fatalf("EnsureEngramBinary() resolvable = false after a successful go install, remediation = %q", remediation)
	}
	if remediation != "" {
		t.Fatalf("EnsureEngramBinary() remediation = %q, want empty after a successful go install", remediation)
	}
	if path != binaryPath {
		t.Fatalf("EnsureEngramBinary() path = %q, want %q", path, binaryPath)
	}
	// fakeGoInstallRunner.Output always returns []byte{}, nil (it never stubs "go env GOBIN"/"go
	// env GOPATH"), so GoBinDir errors and PATH persistence is skipped — not attempted.
	if pathWarning != "" {
		t.Fatalf("EnsureEngramBinary() pathWarning = %q, want empty when GoBinDir cannot be resolved (not attempted)", pathWarning)
	}

	want := []commandInvocation{
		{Name: "go", Args: []string{"install", "github.com/Gentleman-Programming/engram/cmd/engram@v1.15.3"}},
		// EnsureEngramBinary's Phase 4 signal-wiring (D-5): once resolvable, it queries GoBinDir to
		// decide whether to attempt PATH persistence. fakeGoInstallRunner.Output always returns
		// empty (never stubs these keys), so GoBinDir errors and no pathStore call follows — but the
		// two lookups themselves are still genuinely issued.
		{Name: "go", Args: []string{"env", "GOBIN"}},
		{Name: "go", Args: []string{"env", "GOPATH"}},
	}
	if !reflect.DeepEqual(runner.commands, want) {
		t.Fatalf("runner.commands = %#v, want %#v", runner.commands, want)
	}
}

// TestEnsureEngramBinary_MissingWithoutGo_ReturnsRemediation covers the "nothing click can safely
// do" case: no Go toolchain on PATH, so no command is issued at all, and the caller gets back a
// remediation message with the exact manual next step — not a generic "some message" placeholder.
func TestEnsureEngramBinary_MissingWithoutGo_ReturnsRemediation(t *testing.T) {
	claudeHome := t.TempDir()
	cfg := Config{ClaudeHome: claudeHome}

	lookup := &fakeBinaryLookup{resolved: map[string]string{}}
	restoreLookup := SetBinaryLookupFactoryForTests(func() BinaryLookup { return lookup })
	defer restoreLookup()

	runner := newFakeCommandRunner(cfg)
	restoreRunner := SetCommandRunnerFactoryForTests(func() CommandRunner { return runner })
	defer restoreRunner()

	_, resolvable, remediation, pathWarning, err := EnsureEngramBinary(cfg, "v1.15.3")
	if err != nil {
		t.Fatalf("EnsureEngramBinary() error = %v", err)
	}
	if resolvable {
		t.Fatal("EnsureEngramBinary() resolvable = true when neither the binary nor Go are on PATH")
	}
	if len(runner.commands) != 0 {
		t.Fatalf("EnsureEngramBinary() issued commands %#v when Go is unavailable, want zero", runner.commands)
	}
	// Never resolvable, so PATH persistence is never even reached.
	if pathWarning != "" {
		t.Fatalf("EnsureEngramBinary() pathWarning = %q, want empty when the binary never resolved", pathWarning)
	}

	wantCmd := "go install github.com/Gentleman-Programming/engram/cmd/engram@v1.15.3"
	if !strings.Contains(remediation, wantCmd) {
		t.Fatalf("remediation = %q, want it to contain the exact command %q", remediation, wantCmd)
	}
	if !strings.Contains(remediation, "GOPATH/bin") {
		t.Fatalf("remediation = %q, want it to mention GOPATH/bin", remediation)
	}
	if !strings.Contains(remediation, "brew install gentleman-programming/tap/engram") {
		t.Fatalf("remediation = %q, want it to mention the brew alternative", remediation)
	}
}

// TestEnsureEngramBinary_Idempotent_NoReinstallAfterSuccess is the idempotency regression guard:
// once a `go install` run has made the binary resolvable, a later call must not re-run it.
func TestEnsureEngramBinary_Idempotent_NoReinstallAfterSuccess(t *testing.T) {
	claudeHome := t.TempDir()
	cfg := Config{ClaudeHome: claudeHome}
	binaryPath := filepath.Join(t.TempDir(), "engram.exe")

	lookup := &fakeBinaryLookup{resolved: map[string]string{"go": "/usr/bin/go"}}
	restoreLookup := SetBinaryLookupFactoryForTests(func() BinaryLookup { return lookup })
	defer restoreLookup()

	runner := &fakeGoInstallRunner{lookup: lookup, binaryName: engramBinaryName(), binaryPath: binaryPath}
	restoreRunner := SetCommandRunnerFactoryForTests(func() CommandRunner { return runner })
	defer restoreRunner()

	if _, resolvable, remediation, pathWarning, err := EnsureEngramBinary(cfg, "v1.15.3"); err != nil || !resolvable || pathWarning != "" {
		t.Fatalf("first EnsureEngramBinary() resolvable = %v remediation = %q pathWarning = %q err = %v, want true/empty/empty/nil", resolvable, remediation, pathWarning, err)
	}
	// EnsureEngramBinary's Phase 4 signal-wiring (D-5) unconditionally queries GoBinDir once
	// resolvable (2 extra "go env" commands, both empty here since fakeGoInstallRunner never stubs
	// them, so PATH persistence itself is never attempted) — the idempotency guarantee this test
	// covers is specifically about "go install" never re-running, not about the total command count.
	if n := countGoInstalls(runner.commands); n != 1 {
		t.Fatalf("after first EnsureEngramBinary(), go install count = %d in %#v, want exactly 1", n, runner.commands)
	}

	if _, resolvable, remediation, pathWarning, err := EnsureEngramBinary(cfg, "v1.15.3"); err != nil || !resolvable || remediation != "" || pathWarning != "" {
		t.Fatalf("second EnsureEngramBinary() resolvable = %v remediation = %q pathWarning = %q err = %v, want true/empty/empty/nil", resolvable, remediation, pathWarning, err)
	}
	if n := countGoInstalls(runner.commands); n != 1 {
		t.Fatalf("second EnsureEngramBinary() go install count = %d in %#v, want still exactly 1 (idempotent, no reinstall)", n, runner.commands)
	}
}

// countGoInstalls counts how many `go install ...` invocations appear in commands, ignoring the
// `go env GOBIN`/`go env GOPATH` lookups EnsureEngramBinary's PATH-persistence check (D-5) also
// issues once the binary is resolvable.
func countGoInstalls(commands []commandInvocation) int {
	n := 0
	for _, inv := range commands {
		if inv.Name == "go" && len(inv.Args) > 0 && inv.Args[0] == "install" {
			n++
		}
	}
	return n
}

// fakeConfigurablePathStore is a pathStore double for Phase 4's signal-wiring tests: it records
// every EnsureOnPath call (dir argument) and can be configured to fail, letting tests prove both
// (a) a successful PATH persistence issues exactly one EnsureOnPath call and yields an empty
// pathWarning, and (b) a failing one surfaces a non-empty, wrapped pathWarning WITHOUT making
// EnsureEngramBinary itself return an error (the binary is still provisioned/resolvable regardless
// of whether PATH persistence succeeded).
type fakeConfigurablePathStore struct {
	ensureOnPathCalls []string
	ensureOnPathErr   error
	// ensureOnPathNoChange, when true, makes EnsureOnPath report changed=false (still recording the
	// call) instead of its default true — simulating an idempotent no-op re-run (e.g. dir already
	// on the persisted PATH) without needing an error. Used by the D-9 ownership-merge tests to
	// exercise SyncEngram's SECOND-run behavior against a store that mutated on the first call and
	// legitimately does not mutate again on the second.
	ensureOnPathNoChange bool

	removeFromPathCalls []string
	removeFromPathErr   error
}

func (f *fakeConfigurablePathStore) PersistedPathContains(dir string) (bool, error) {
	return false, nil
}

func (f *fakeConfigurablePathStore) EnsureOnPath(dir string) (bool, error) {
	f.ensureOnPathCalls = append(f.ensureOnPathCalls, dir)
	if f.ensureOnPathErr != nil {
		return false, f.ensureOnPathErr
	}
	if f.ensureOnPathNoChange {
		return false, nil
	}
	return true, nil
}

func (f *fakeConfigurablePathStore) RemoveFromPath(dir string) (bool, error) {
	f.removeFromPathCalls = append(f.removeFromPathCalls, dir)
	if f.removeFromPathErr != nil {
		return false, f.removeFromPathErr
	}
	return true, nil
}

// seedResolvableEngramBinaryInGoBinDir wires cfg's engram binary to resolve from INSIDE a fake
// `go env GOBIN`-reported directory: it writes a dummy binary at gobin/<engramBinaryName> and
// points CLICK_ENGRAM_BINARY_PATH at it, then stubs "go env GOBIN" on runner so GoBinDir(cfg)
// resolves to the exact same directory. This is the one precondition EnsureEngramBinary's PATH
// persistence step requires before it will call pathStoreFactory().EnsureOnPath at all (D-5: skip
// brew-resolved installs, env overrides pointed elsewhere, etc.).
func seedResolvableEngramBinaryInGoBinDir(t *testing.T, runner *fakeCommandRunner) (gobin, binaryPath string) {
	t.Helper()
	gobin = filepath.Join(t.TempDir(), "gobin")
	if err := os.MkdirAll(gobin, 0o755); err != nil {
		t.Fatalf("MkdirAll(gobin) error = %v", err)
	}
	binaryPath = filepath.Join(gobin, engramBinaryName())
	if err := os.WriteFile(binaryPath, []byte("binary"), 0o644); err != nil {
		t.Fatalf("WriteFile(binary) error = %v", err)
	}
	t.Setenv(engramBinaryPathEnvOverride, binaryPath)
	runner.lookup["go env GOBIN"] = []byte(gobin + "\n")
	return gobin, binaryPath
}

// TestEnsureEngramBinary_PersistsPathWhenResolvedFromGoBinDir_Success covers the strict-TDD
// requirement (a): a successful PATH-persistence attempt calls pathStoreFactory().EnsureOnPath
// exactly once with the resolved GoBinDir, and yields an empty pathWarning alongside a nil err.
func TestEnsureEngramBinary_PersistsPathWhenResolvedFromGoBinDir_Success(t *testing.T) {
	claudeHome := t.TempDir()
	cfg := Config{ClaudeHome: claudeHome}
	runner := newFakeCommandRunner(cfg)
	restoreRunner := SetCommandRunnerFactoryForTests(func() CommandRunner { return runner })
	defer restoreRunner()

	gobin, _ := seedResolvableEngramBinaryInGoBinDir(t, runner)

	store := &fakeConfigurablePathStore{}
	restoreStore := SetPathStoreFactoryForTests(func() pathStore { return store })
	defer restoreStore()

	_, resolvable, _, pathWarning, err := EnsureEngramBinary(cfg, "v1.15.3")
	if err != nil {
		t.Fatalf("EnsureEngramBinary() error = %v", err)
	}
	if !resolvable {
		t.Fatal("EnsureEngramBinary() resolvable = false, want true")
	}
	if pathWarning != "" {
		t.Fatalf("EnsureEngramBinary() pathWarning = %q, want empty on a successful PATH persist", pathWarning)
	}
	if len(store.ensureOnPathCalls) != 1 || store.ensureOnPathCalls[0] != gobin {
		t.Fatalf("store.ensureOnPathCalls = %#v, want exactly one call with %q", store.ensureOnPathCalls, gobin)
	}
}

// TestEnsureEngramBinary_PathPersistenceErrorSurfacesAsWarningNotError covers requirement (b): a
// PATH-persistence error must surface as a non-empty, wrapped pathWarning while EnsureEngramBinary
// itself still returns err=nil and resolvable=true — the binary IS provisioned, PATH persistence
// merely failed.
func TestEnsureEngramBinary_PathPersistenceErrorSurfacesAsWarningNotError(t *testing.T) {
	claudeHome := t.TempDir()
	cfg := Config{ClaudeHome: claudeHome}
	runner := newFakeCommandRunner(cfg)
	restoreRunner := SetCommandRunnerFactoryForTests(func() CommandRunner { return runner })
	defer restoreRunner()

	gobin, _ := seedResolvableEngramBinaryInGoBinDir(t, runner)

	persistErr := errors.New("acceso denegado al registro")
	store := &fakeConfigurablePathStore{ensureOnPathErr: persistErr}
	restoreStore := SetPathStoreFactoryForTests(func() pathStore { return store })
	defer restoreStore()

	_, resolvable, _, pathWarning, err := EnsureEngramBinary(cfg, "v1.15.3")
	if err != nil {
		t.Fatalf("EnsureEngramBinary() error = %v, want nil — a PATH-persistence failure must not fail binary provisioning", err)
	}
	if !resolvable {
		t.Fatal("EnsureEngramBinary() resolvable = false, want true — the binary itself is still provisioned")
	}
	if pathWarning == "" {
		t.Fatal("EnsureEngramBinary() pathWarning = \"\", want a non-empty wrapped warning")
	}
	if !strings.Contains(pathWarning, persistErr.Error()) {
		t.Fatalf("pathWarning = %q, want it to wrap %q", pathWarning, persistErr.Error())
	}
	if !strings.Contains(pathWarning, gobin) {
		t.Fatalf("pathWarning = %q, want it to mention the target directory %q", pathWarning, gobin)
	}
	if len(store.ensureOnPathCalls) != 1 {
		t.Fatalf("store.ensureOnPathCalls = %#v, want exactly one attempt", store.ensureOnPathCalls)
	}
}

// TestEnsureEngramBinary_SkipsPathPersistenceWhenBinaryNotFromGoBinDir proves the design's
// "resolves from within GoBinDir()" gate: a binary resolved from anywhere else (brew, a test/
// deployment env override pointed elsewhere, the DefaultEngramBinaryPath fallback) must never
// trigger a PATH-persistence attempt at all — pathStoreFactory().EnsureOnPath must not be called.
func TestEnsureEngramBinary_SkipsPathPersistenceWhenBinaryNotFromGoBinDir(t *testing.T) {
	claudeHome := t.TempDir()
	cfg := Config{ClaudeHome: claudeHome}
	runner := newFakeCommandRunner(cfg)
	restoreRunner := SetCommandRunnerFactoryForTests(func() CommandRunner { return runner })
	defer restoreRunner()

	// GoBinDir resolves fine (go env GOBIN is stubbed)…
	gobin := filepath.Join(t.TempDir(), "gobin")
	runner.lookup["go env GOBIN"] = []byte(gobin + "\n")
	// …but the resolved binary lives somewhere else entirely (simulating brew / a manually-managed
	// install), NOT inside gobin.
	elsewhere := filepath.Join(t.TempDir(), "elsewhere", "engram.exe")
	if err := os.MkdirAll(filepath.Dir(elsewhere), 0o755); err != nil {
		t.Fatalf("MkdirAll(elsewhere) error = %v", err)
	}
	if err := os.WriteFile(elsewhere, []byte("binary"), 0o644); err != nil {
		t.Fatalf("WriteFile(elsewhere) error = %v", err)
	}
	t.Setenv(engramBinaryPathEnvOverride, elsewhere)

	store := &fakeConfigurablePathStore{}
	restoreStore := SetPathStoreFactoryForTests(func() pathStore { return store })
	defer restoreStore()

	_, resolvable, _, pathWarning, err := EnsureEngramBinary(cfg, "v1.15.3")
	if err != nil {
		t.Fatalf("EnsureEngramBinary() error = %v", err)
	}
	if !resolvable {
		t.Fatal("EnsureEngramBinary() resolvable = false, want true")
	}
	if pathWarning != "" {
		t.Fatalf("EnsureEngramBinary() pathWarning = %q, want empty (not attempted)", pathWarning)
	}
	if len(store.ensureOnPathCalls) != 0 {
		t.Fatalf("store.ensureOnPathCalls = %#v, want zero — binary did not resolve from within GoBinDir", store.ensureOnPathCalls)
	}
}

// TestSyncEngram_ForwardsPathWarningFromEnsureEngramBinary covers requirement (c): SyncEngram must
// forward EnsureEngramBinary's pathWarning unchanged to its own caller.
func TestSyncEngram_ForwardsPathWarningFromEnsureEngramBinary(t *testing.T) {
	claudeHome := t.TempDir()
	cfg := Config{ClaudeHome: claudeHome}
	runner := newFakeCommandRunner(cfg)
	restoreRunner := SetCommandRunnerFactoryForTests(func() CommandRunner { return runner })
	defer restoreRunner()
	restoreSource := SetEngramMarketplaceSourceForTests("https://github.com/Gentleman-Programming/engram")
	defer restoreSource()

	seedResolvableEngramBinaryInGoBinDir(t, runner)

	persistErr := errors.New("no se pudo escribir el rc file")
	store := &fakeConfigurablePathStore{ensureOnPathErr: persistErr}
	restoreStore := SetPathStoreFactoryForTests(func() pathStore { return store })
	defer restoreStore()

	m, err := manifest.Load()
	if err != nil {
		t.Fatalf("manifest.Load() error = %v", err)
	}

	_, pathWarning, err := SyncEngram(cfg, m)
	if err != nil {
		t.Fatalf("SyncEngram() error = %v, want nil — PATH persistence failure must not fail SyncEngram", err)
	}
	if pathWarning == "" {
		t.Fatal("SyncEngram() pathWarning = \"\", want EnsureEngramBinary's non-empty pathWarning forwarded unchanged")
	}
	if !strings.Contains(pathWarning, persistErr.Error()) {
		t.Fatalf("SyncEngram() pathWarning = %q, want it to wrap %q (forwarded from EnsureEngramBinary)", pathWarning, persistErr.Error())
	}
}

// TestSyncEngram_RecordsPathOwnershipWhenPathMutated is D-9's core ownership-recording contract: a
// run whose PATH-persistence attempt actually mutated the persisted PATH (EnsureOnPath
// changed==true) must record engramState.PathMutatedByClick=true and PathDir=the exact directory
// that was added — the two signals RemoveEngramPlugin later needs to know it is safe to reverse.
func TestSyncEngram_RecordsPathOwnershipWhenPathMutated(t *testing.T) {
	claudeHome := t.TempDir()
	cfg := Config{ClaudeHome: claudeHome}
	runner := newFakeCommandRunner(cfg)
	restoreRunner := SetCommandRunnerFactoryForTests(func() CommandRunner { return runner })
	defer restoreRunner()
	restoreSource := SetEngramMarketplaceSourceForTests("https://github.com/Gentleman-Programming/engram")
	defer restoreSource()

	gobin, _ := seedResolvableEngramBinaryInGoBinDir(t, runner)

	store := &fakeConfigurablePathStore{}
	restoreStore := SetPathStoreFactoryForTests(func() pathStore { return store })
	defer restoreStore()

	m, err := manifest.Load()
	if err != nil {
		t.Fatalf("manifest.Load() error = %v", err)
	}

	if _, _, err := SyncEngram(cfg, m); err != nil {
		t.Fatalf("SyncEngram() error = %v", err)
	}

	state := readEngramStateForTest(t, cfg)
	if !state.PathMutatedByClick {
		t.Fatal("state.PathMutatedByClick = false after a SyncEngram() run whose EnsureOnPath reported changed=true, want true")
	}
	if state.PathDir != gobin {
		t.Fatalf("state.PathDir = %q, want %q", state.PathDir, gobin)
	}
}

// TestSyncEngram_SecondRunPreservesPathOwnership is TestSyncEngram_SecondRunPreservesClickOwnership's
// D-9 counterpart: once PathMutatedByClick is true, a LATER idempotent SyncEngram() run whose
// EnsureOnPath is a no-op (changed==false, dir already on the persisted PATH) must NOT flip it back
// to false, and must NOT blank out the previously-recorded PathDir.
func TestSyncEngram_SecondRunPreservesPathOwnership(t *testing.T) {
	claudeHome := t.TempDir()
	cfg := Config{ClaudeHome: claudeHome}
	runner := newFakeCommandRunner(cfg)
	restoreRunner := SetCommandRunnerFactoryForTests(func() CommandRunner { return runner })
	defer restoreRunner()
	restoreSource := SetEngramMarketplaceSourceForTests("https://github.com/Gentleman-Programming/engram")
	defer restoreSource()

	gobin, _ := seedResolvableEngramBinaryInGoBinDir(t, runner)

	store := &fakeConfigurablePathStore{}
	restoreStore := SetPathStoreFactoryForTests(func() pathStore { return store })
	defer restoreStore()

	m, err := manifest.Load()
	if err != nil {
		t.Fatalf("manifest.Load() error = %v", err)
	}

	if _, _, err := SyncEngram(cfg, m); err != nil {
		t.Fatalf("first SyncEngram() error = %v", err)
	}
	firstState := readEngramStateForTest(t, cfg)
	if !firstState.PathMutatedByClick || firstState.PathDir != gobin {
		t.Fatalf("first SyncEngram() state = %+v, want PathMutatedByClick=true PathDir=%q", firstState, gobin)
	}

	// Simulate the second run's EnsureOnPath being a genuine idempotent no-op (dir already on the
	// persisted PATH from the first run) — NOT an error.
	store.ensureOnPathNoChange = true

	if _, _, err := SyncEngram(cfg, m); err != nil {
		t.Fatalf("second SyncEngram() error = %v", err)
	}
	secondState := readEngramStateForTest(t, cfg)
	if !secondState.PathMutatedByClick {
		t.Fatal("state.PathMutatedByClick flipped to false after a second, idempotent (EnsureOnPath changed=false) SyncEngram() run — it must be preserved, not re-derived from this run's own result")
	}
	if secondState.PathDir != gobin {
		t.Fatalf("state.PathDir = %q after the idempotent second run, want it preserved as %q", secondState.PathDir, gobin)
	}
}

// readEngramStateForTest reads and unmarshals cfg's persisted engramState, failing the test on any
// error — a small shared helper for the D-9 ownership tests above/below that need to assert on
// PathMutatedByClick/PathDir specifically (the existing tests in this file that only cared about
// InstalledByClick each inlined this themselves; this is intentionally the same read+unmarshal
// they use, factored out only for the newer tests to avoid repeating it four more times).
func readEngramStateForTest(t *testing.T, cfg Config) engramState {
	t.Helper()
	data, err := os.ReadFile(cfg.EngramStatePath())
	if err != nil {
		t.Fatalf("ReadFile(EngramStatePath) error = %v", err)
	}
	var state engramState
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatalf("json.Unmarshal(state) error = %v", err)
	}
	return state
}

// TestLoadEngramState_MigratesLegacyPathDirToPathDirs is the T4-1 follow-up migration contract: a
// state file written by v0.4.3 or earlier (only PathDir set, no PathDirs field at all) must load
// with PathDirs seeded from it in memory — an install upgrading from that version must not silently
// "forget" the one directory it already knew about.
func TestLoadEngramState_MigratesLegacyPathDirToPathDirs(t *testing.T) {
	claudeHome := t.TempDir()
	cfg := Config{ClaudeHome: claudeHome}

	legacyDir := filepath.Join(t.TempDir(), "gobin")
	if err := writeJSONFile(cfg.EngramStatePath(), engramState{
		InstalledByClick:   true,
		PathMutatedByClick: true,
		PathDir:            legacyDir,
	}); err != nil {
		t.Fatalf("writeJSONFile(EngramStatePath) error = %v", err)
	}

	state, found, err := loadEngramState(cfg)
	if err != nil {
		t.Fatalf("loadEngramState() error = %v", err)
	}
	if !found {
		t.Fatal("loadEngramState() found = false, want true")
	}
	if len(state.PathDirs) != 1 || state.PathDirs[0] != legacyDir {
		t.Fatalf("state.PathDirs = %#v, want exactly [%q] migrated from the legacy PathDir field", state.PathDirs, legacyDir)
	}
	if state.PathDir != legacyDir {
		t.Fatalf("state.PathDir = %q, want it preserved as %q", state.PathDir, legacyDir)
	}
}

// TestSyncEngram_TracksAllPathDirsAcrossGoBinDirMoves is the T4-1 follow-up core contract: TWO
// separate SyncEngram runs, each resolving a DIFFERENT GoBinDir and each actually mutating the
// persisted PATH, must leave BOTH directories recorded in PathDirs — not just the latest one (which
// is all the older PathDir-only tracking preserved).
func TestSyncEngram_TracksAllPathDirsAcrossGoBinDirMoves(t *testing.T) {
	claudeHome := t.TempDir()
	cfg := Config{ClaudeHome: claudeHome}
	runner := newFakeCommandRunner(cfg)
	restoreRunner := SetCommandRunnerFactoryForTests(func() CommandRunner { return runner })
	defer restoreRunner()
	restoreSource := SetEngramMarketplaceSourceForTests("https://github.com/Gentleman-Programming/engram")
	defer restoreSource()

	store := &fakeConfigurablePathStore{}
	restoreStore := SetPathStoreFactoryForTests(func() pathStore { return store })
	defer restoreStore()

	m, err := manifest.Load()
	if err != nil {
		t.Fatalf("manifest.Load() error = %v", err)
	}

	firstGobin, _ := seedResolvableEngramBinaryInGoBinDir(t, runner)
	if _, _, err := SyncEngram(cfg, m); err != nil {
		t.Fatalf("first SyncEngram() error = %v", err)
	}

	// Simulate Go being reinstalled with a different GOPATH/GOBIN between two `click update` runs —
	// a brand-new gobin, distinct from the first.
	secondGobin, _ := seedResolvableEngramBinaryInGoBinDir(t, runner)
	if _, _, err := SyncEngram(cfg, m); err != nil {
		t.Fatalf("second SyncEngram() error = %v", err)
	}

	state := readEngramStateForTest(t, cfg)
	if len(state.PathDirs) != 2 {
		t.Fatalf("state.PathDirs = %#v, want exactly 2 entries — both GoBinDir moves must be tracked", state.PathDirs)
	}
	if state.PathDirs[0] != firstGobin || state.PathDirs[1] != secondGobin {
		t.Fatalf("state.PathDirs = %#v, want [%q, %q] in first-added order", state.PathDirs, firstGobin, secondGobin)
	}
}

// TestSyncEngram_DedupesRepeatedPathDirMutations proves the append-only PathDirs accumulation is
// deduped: even a store that reports changed=true on EVERY call (not a realistic idempotent
// EnsureOnPath, but a defensive edge case) against the SAME dir across two SyncEngram runs must not
// grow PathDirs past one entry for that dir.
func TestSyncEngram_DedupesRepeatedPathDirMutations(t *testing.T) {
	claudeHome := t.TempDir()
	cfg := Config{ClaudeHome: claudeHome}
	runner := newFakeCommandRunner(cfg)
	restoreRunner := SetCommandRunnerFactoryForTests(func() CommandRunner { return runner })
	defer restoreRunner()
	restoreSource := SetEngramMarketplaceSourceForTests("https://github.com/Gentleman-Programming/engram")
	defer restoreSource()

	gobin, _ := seedResolvableEngramBinaryInGoBinDir(t, runner)

	// ensureOnPathNoChange stays false: store.EnsureOnPath reports changed=true on every call, even
	// against the same, already-tracked dir on the second run — SyncEngram's own dedupe guard (not
	// the store) must be what keeps PathDirs from growing.
	store := &fakeConfigurablePathStore{}
	restoreStore := SetPathStoreFactoryForTests(func() pathStore { return store })
	defer restoreStore()

	m, err := manifest.Load()
	if err != nil {
		t.Fatalf("manifest.Load() error = %v", err)
	}

	if _, _, err := SyncEngram(cfg, m); err != nil {
		t.Fatalf("first SyncEngram() error = %v", err)
	}
	if _, _, err := SyncEngram(cfg, m); err != nil {
		t.Fatalf("second SyncEngram() error = %v", err)
	}

	state := readEngramStateForTest(t, cfg)
	if len(state.PathDirs) != 1 || state.PathDirs[0] != gobin {
		t.Fatalf("state.PathDirs = %#v after two runs against the same dir, want exactly [%q]", state.PathDirs, gobin)
	}
}

// TestRemoveEngramPlugin_ReversesAllTrackedPathDirs is the T4-1 follow-up counterpart of
// TestRemoveEngramPlugin_ReversesClickOwnedPath: a state recording TWO PathDirs entries must call
// pathStoreFactory().RemoveFromPath for BOTH, not just the latest (state.PathDir).
func TestRemoveEngramPlugin_ReversesAllTrackedPathDirs(t *testing.T) {
	claudeHome := t.TempDir()
	cfg := Config{ClaudeHome: claudeHome}
	runner := newFakeCommandRunner(cfg)
	restoreRunner := SetCommandRunnerFactoryForTests(func() CommandRunner { return runner })
	defer restoreRunner()

	firstDir := filepath.Join(t.TempDir(), "gobin-old")
	secondDir := filepath.Join(t.TempDir(), "gobin-new")
	if err := writeJSONFile(cfg.EngramStatePath(), engramState{
		InstalledByClick:   true,
		PathMutatedByClick: true,
		PathDir:            secondDir,
		PathDirs:           []string{firstDir, secondDir},
	}); err != nil {
		t.Fatalf("writeJSONFile(EngramStatePath) error = %v", err)
	}

	store := &fakeConfigurablePathStore{}
	restoreStore := SetPathStoreFactoryForTests(func() pathStore { return store })
	defer restoreStore()

	pathWarning, err := RemoveEngramPlugin(cfg)
	if err != nil {
		t.Fatalf("RemoveEngramPlugin() error = %v", err)
	}
	if pathWarning != "" {
		t.Fatalf("RemoveEngramPlugin() pathWarning = %q, want empty on a successful reversal of both dirs", pathWarning)
	}
	if len(store.removeFromPathCalls) != 2 {
		t.Fatalf("store.removeFromPathCalls = %#v, want exactly 2 calls (one per tracked PathDirs entry)", store.removeFromPathCalls)
	}
	if store.removeFromPathCalls[0] != firstDir || store.removeFromPathCalls[1] != secondDir {
		t.Fatalf("store.removeFromPathCalls = %#v, want [%q, %q] in tracked order", store.removeFromPathCalls, firstDir, secondDir)
	}
}

// partialFailPathStore is a pathStore double that fails RemoveFromPath only for one specific,
// pre-configured directory — letting
// TestRemoveEngramPlugin_PathRemovalFailureOnOneDirStillAttemptsTheOther prove a failure removing
// ONE tracked directory does not prevent removeClickOwnedPath from attempting the OTHERS (T4-1
// follow-up).
type partialFailPathStore struct {
	failDir             string
	failErr             error
	removeFromPathCalls []string
}

func (f *partialFailPathStore) PersistedPathContains(dir string) (bool, error) { return false, nil }

func (f *partialFailPathStore) EnsureOnPath(dir string) (bool, error) { return true, nil }

func (f *partialFailPathStore) RemoveFromPath(dir string) (bool, error) {
	f.removeFromPathCalls = append(f.removeFromPathCalls, dir)
	if dir == f.failDir {
		return false, f.failErr
	}
	return true, nil
}

// TestRemoveEngramPlugin_PathRemovalFailureOnOneDirStillAttemptsTheOther is the T4-1 follow-up
// resilience contract: RemoveFromPath failing for ONE tracked directory must not prevent
// removeClickOwnedPath from attempting the OTHER — both are always attempted, and the failure is
// folded into pathWarning, never fatal.
func TestRemoveEngramPlugin_PathRemovalFailureOnOneDirStillAttemptsTheOther(t *testing.T) {
	claudeHome := t.TempDir()
	cfg := Config{ClaudeHome: claudeHome}
	runner := newFakeCommandRunner(cfg)
	restoreRunner := SetCommandRunnerFactoryForTests(func() CommandRunner { return runner })
	defer restoreRunner()

	firstDir := filepath.Join(t.TempDir(), "gobin-old")
	secondDir := filepath.Join(t.TempDir(), "gobin-new")
	if err := writeJSONFile(cfg.EngramStatePath(), engramState{
		InstalledByClick:   true,
		PathMutatedByClick: true,
		PathDir:            secondDir,
		PathDirs:           []string{firstDir, secondDir},
	}); err != nil {
		t.Fatalf("writeJSONFile(EngramStatePath) error = %v", err)
	}

	removeErr := errors.New("acceso denegado al registro")
	store := &partialFailPathStore{failDir: firstDir, failErr: removeErr}
	restoreStore := SetPathStoreFactoryForTests(func() pathStore { return store })
	defer restoreStore()

	pathWarning, err := RemoveEngramPlugin(cfg)
	if err != nil {
		t.Fatalf("RemoveEngramPlugin() error = %v, want nil — a PATH-removal failure must not fail uninstall", err)
	}
	if !strings.Contains(pathWarning, removeErr.Error()) {
		t.Fatalf("pathWarning = %q, want it to mention %q", pathWarning, removeErr.Error())
	}
	if !strings.Contains(pathWarning, firstDir) {
		t.Fatalf("pathWarning = %q, want it to mention the failing directory %q", pathWarning, firstDir)
	}
	if len(store.removeFromPathCalls) != 2 || store.removeFromPathCalls[0] != firstDir || store.removeFromPathCalls[1] != secondDir {
		t.Fatalf("store.removeFromPathCalls = %#v, want both %q and %q attempted despite the first failing", store.removeFromPathCalls, firstDir, secondDir)
	}
}

// fakeHonestGoInstallRunner is a CommandRunner fake that models what a real `go install` actually
// does — unlike fakeGoInstallRunner above (per JD-001, the root cause of why 5 rounds of prior
// review missed this bug): it writes the provisioned binary to disk at binaryPath, but it does NOT
// also retroactively register that binary in any BinaryLookup/LookPath map. A real `go install`
// runs as a CHILD process; it can never mutate the PARENT process's own PATH/LookPath resolution.
// It also answers `go env GOBIN` with gobin so installer.GoBinDir(cfg) resolves deterministically.
type fakeHonestGoInstallRunner struct {
	commands   []commandInvocation
	binaryPath string
	gobin      string
}

func (f *fakeHonestGoInstallRunner) Run(name string, args ...string) error {
	f.commands = append(f.commands, commandInvocation{Name: name, Args: append([]string(nil), args...)})
	if name == "go" && len(args) > 0 && args[0] == "install" {
		return os.WriteFile(f.binaryPath, []byte("binary"), 0o644)
	}
	return nil
}

func (f *fakeHonestGoInstallRunner) Output(name string, args ...string) ([]byte, error) {
	f.commands = append(f.commands, commandInvocation{Name: name, Args: append([]string(nil), args...)})
	if name == "go" && len(args) == 2 && args[0] == "env" && args[1] == "GOBIN" {
		return []byte(f.gobin + "\n"), nil
	}
	return []byte{}, nil
}

// TestEnsureEngramBinary_PersistsPathAfterHonestGoInstall_FreshMachine is JD-001's regression test:
// the exact fresh-install scenario this whole change exists to fix. Go is on PATH, the Engram
// binary is not, `go install` runs and genuinely writes the binary to disk at the real GoBinDir —
// but (honestly, unlike fakeGoInstallRunner) does NOT make it newly resolvable via LookPath in THIS
// process. EnsureEngramBinary must still independently detect the binary now exists on disk at
// GoBinDir and attempt PATH persistence against that location, even though `resolvable` itself
// (this process's own live LookPath prediction) legitimately stays false.
func TestEnsureEngramBinary_PersistsPathAfterHonestGoInstall_FreshMachine(t *testing.T) {
	claudeHome := t.TempDir()
	cfg := Config{ClaudeHome: claudeHome}

	// "go" itself resolves on PATH; "engram" never does — and nothing in this test's fake
	// retroactively adds it once "go install" runs (the one thing fakeGoInstallRunner got wrong).
	lookup := &fakeBinaryLookup{resolved: map[string]string{"go": "/usr/bin/go"}}
	restoreLookup := SetBinaryLookupFactoryForTests(func() BinaryLookup { return lookup })
	defer restoreLookup()

	gobin := filepath.Join(t.TempDir(), "gobin")
	if err := os.MkdirAll(gobin, 0o755); err != nil {
		t.Fatalf("MkdirAll(gobin) error = %v", err)
	}
	binaryPath := filepath.Join(gobin, engramBinaryName())
	runner := &fakeHonestGoInstallRunner{binaryPath: binaryPath, gobin: gobin}
	restoreRunner := SetCommandRunnerFactoryForTests(func() CommandRunner { return runner })
	defer restoreRunner()

	store := &fakeConfigurablePathStore{}
	restoreStore := SetPathStoreFactoryForTests(func() pathStore { return store })
	defer restoreStore()

	_, resolvable, remediation, pathWarning, err := EnsureEngramBinary(cfg, "v1.15.3")
	if err != nil {
		t.Fatalf("EnsureEngramBinary() error = %v", err)
	}
	// resolvable legitimately stays false: it answers "would a bare `command: engram` MCP launch
	// succeed right now, in THIS process" — and this process's own LookPath still cannot see what
	// `go install` just wrote to disk. That is NOT what this test guards.
	if resolvable {
		t.Fatal("EnsureEngramBinary() resolvable = true, want false — this process's LookPath still cannot see the freshly-installed binary")
	}
	if remediation == "" {
		t.Fatal("EnsureEngramBinary() remediation = \"\", want non-empty — resolvable is still false from this process's point of view")
	}
	if _, statErr := os.Stat(binaryPath); statErr != nil {
		t.Fatalf("go install fake did not write the binary to disk at %s: %v", binaryPath, statErr)
	}
	// This is JD-001's actual assertion: PATH persistence must be attempted against the real
	// GoBinDir even though `resolvable` stayed false, because the binary genuinely exists on disk
	// there now.
	if len(store.ensureOnPathCalls) != 1 || store.ensureOnPathCalls[0] != gobin {
		t.Fatalf("store.ensureOnPathCalls = %#v, want exactly one call with %q — a fresh go install must trigger PATH persistence independently of the resolvable flag", store.ensureOnPathCalls, gobin)
	}
	if pathWarning != "" {
		t.Fatalf("EnsureEngramBinary() pathWarning = %q, want empty on a successful PATH persist", pathWarning)
	}
}

// TestEnsureEngramBinary_SecondCallAfterHonestGoInstall_StillPersistsPath is JD-002's regression
// test: doctor's checkEngramPath remediation message tells the user to re-run `click install`/
// `click update`. Before the JD-001 fix this was permanently ineffective — `go install` is
// idempotent and EnsureEngramBinary hit the identical resolvable=false, no-persistence branch every
// time. After the fix, a second call (the user literally re-running `click install`) must still
// reach and re-attempt PATH persistence — proving the remediation advice is now actionable, not a
// dead end.
func TestEnsureEngramBinary_SecondCallAfterHonestGoInstall_StillPersistsPath(t *testing.T) {
	claudeHome := t.TempDir()
	cfg := Config{ClaudeHome: claudeHome}

	lookup := &fakeBinaryLookup{resolved: map[string]string{"go": "/usr/bin/go"}}
	restoreLookup := SetBinaryLookupFactoryForTests(func() BinaryLookup { return lookup })
	defer restoreLookup()

	gobin := filepath.Join(t.TempDir(), "gobin")
	if err := os.MkdirAll(gobin, 0o755); err != nil {
		t.Fatalf("MkdirAll(gobin) error = %v", err)
	}
	binaryPath := filepath.Join(gobin, engramBinaryName())
	runner := &fakeHonestGoInstallRunner{binaryPath: binaryPath, gobin: gobin}
	restoreRunner := SetCommandRunnerFactoryForTests(func() CommandRunner { return runner })
	defer restoreRunner()

	store := &fakeConfigurablePathStore{}
	restoreStore := SetPathStoreFactoryForTests(func() pathStore { return store })
	defer restoreStore()

	if _, _, _, pathWarning, err := EnsureEngramBinary(cfg, "v1.15.3"); err != nil || pathWarning != "" {
		t.Fatalf("first EnsureEngramBinary() pathWarning = %q err = %v, want empty/nil", pathWarning, err)
	}
	if _, _, _, pathWarning, err := EnsureEngramBinary(cfg, "v1.15.3"); err != nil || pathWarning != "" {
		t.Fatalf("second EnsureEngramBinary() (simulating a re-run per doctor's advice) pathWarning = %q err = %v, want empty/nil", pathWarning, err)
	}
	if len(store.ensureOnPathCalls) != 2 {
		t.Fatalf("store.ensureOnPathCalls = %#v, want exactly two attempts — doctor's \"re-run click install\" remediation must actually retry PATH persistence, not be a permanent no-op", store.ensureOnPathCalls)
	}
}

func seedEngramAlreadyInstalled(t *testing.T, cfg Config) {
	t.Helper()
	registry := map[string]any{
		"version": 2,
		"plugins": map[string]any{
			EngramPluginID: []map[string]any{{
				"scope":       "user",
				"installPath": filepath.Join(cfg.ClaudeHome, "plugins", "cache", "engram", "engram", "0.1.1"),
				"version":     "0.1.1",
			}},
		},
	}
	if err := writeJSONFile(cfg.InstalledPluginsPath(), registry); err != nil {
		t.Fatalf("writeJSONFile(InstalledPluginsPath) error = %v", err)
	}
	settings := map[string]any{"enabledPlugins": map[string]bool{EngramPluginID: true}}
	if err := writeJSONFile(cfg.SettingsPath(), settings); err != nil {
		t.Fatalf("writeJSONFile(SettingsPath) error = %v", err)
	}
}
