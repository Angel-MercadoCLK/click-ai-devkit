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

	alreadyInstalled, err := SyncEngram(cfg, m)
	if err != nil {
		t.Fatalf("SyncEngram() error = %v", err)
	}
	if alreadyInstalled {
		t.Fatal("SyncEngram() alreadyInstalled = true on a fresh home, want false")
	}

	want := []commandInvocation{
		{Name: "claude", Args: []string{"plugin", "marketplace", "add", "https://github.com/Gentleman-Programming/engram"}},
		{Name: "claude", Args: []string{"plugin", "install", "engram@engram"}},
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

	alreadyInstalled, err := SyncEngram(cfg, m)
	if err != nil {
		t.Fatalf("SyncEngram() error = %v", err)
	}
	if !alreadyInstalled {
		t.Fatal("SyncEngram() alreadyInstalled = false when engram@engram was pre-seeded as installed, want true")
	}
	if len(runner.commands) != 0 {
		t.Fatalf("SyncEngram() issued commands %#v against an already-installed Engram, want zero (no reinstall/clobber)", runner.commands)
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

	if _, err := SyncEngram(cfg, m); err != nil {
		t.Fatalf("first SyncEngram() error = %v", err)
	}
	alreadyInstalled, err := SyncEngram(cfg, m)
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
	if err := RemoveEngramPlugin(cfg); err != nil {
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
	if _, err := SyncEngram(cfg, m); err != nil {
		t.Fatalf("SyncEngram() error = %v", err)
	}

	if err := RemoveEngramPlugin(cfg); err != nil {
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
	if _, err := SyncEngram(cfg, m); err != nil {
		t.Fatalf("SyncEngram() error = %v", err)
	}

	if err := RemoveEngramPlugin(cfg); err != nil {
		t.Fatalf("RemoveEngramPlugin() error = %v", err)
	}

	if len(runner.commands) != 0 {
		t.Fatalf("RemoveEngramPlugin() issued commands %#v against a pre-existing install, want zero", runner.commands)
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

	if err := RemoveEngramPlugin(cfg); err != nil {
		t.Fatalf("RemoveEngramPlugin() on a never-synced home error = %v, want nil", err)
	}
	if len(runner.commands) != 0 {
		t.Fatalf("RemoveEngramPlugin() issued commands %#v on a never-synced home, want zero", runner.commands)
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

	path, resolvable, remediation, err := EnsureEngramBinary(cfg, "v1.15.3")
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
	if len(runner.commands) != 0 {
		t.Fatalf("EnsureEngramBinary() issued commands %#v for an already-resolvable binary, want zero", runner.commands)
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

	path, resolvable, remediation, err := EnsureEngramBinary(cfg, "v1.15.3")
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

	want := []commandInvocation{
		{Name: "go", Args: []string{"install", "github.com/Gentleman-Programming/engram/cmd/engram@v1.15.3"}},
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

	_, resolvable, remediation, err := EnsureEngramBinary(cfg, "v1.15.3")
	if err != nil {
		t.Fatalf("EnsureEngramBinary() error = %v", err)
	}
	if resolvable {
		t.Fatal("EnsureEngramBinary() resolvable = true when neither the binary nor Go are on PATH")
	}
	if len(runner.commands) != 0 {
		t.Fatalf("EnsureEngramBinary() issued commands %#v when Go is unavailable, want zero", runner.commands)
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

	if _, resolvable, remediation, err := EnsureEngramBinary(cfg, "v1.15.3"); err != nil || !resolvable {
		t.Fatalf("first EnsureEngramBinary() resolvable = %v remediation = %q err = %v, want true/empty/nil", resolvable, remediation, err)
	}
	if len(runner.commands) != 1 {
		t.Fatalf("after first EnsureEngramBinary(), commands = %#v, want exactly 1 (the go install)", runner.commands)
	}

	if _, resolvable, remediation, err := EnsureEngramBinary(cfg, "v1.15.3"); err != nil || !resolvable || remediation != "" {
		t.Fatalf("second EnsureEngramBinary() resolvable = %v remediation = %q err = %v, want true/empty/nil", resolvable, remediation, err)
	}
	if len(runner.commands) != 1 {
		t.Fatalf("second EnsureEngramBinary() issued extra commands %#v, want still exactly 1 (idempotent, no reinstall)", runner.commands)
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
