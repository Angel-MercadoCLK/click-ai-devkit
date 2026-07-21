package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/doctor"
	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/installer"
	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/modelconfig"
)

// seedResolvableEngram makes EnsureEngramBinary see Engram as already resolvable, so install-driven
// CLI tests are deterministic regardless of the host PATH. Without it, install issues a `go install`
// on any host with `go` but no `engram` (e.g. CI) — passing locally (dev has engram) but failing CI.
func seedResolvableEngram(t *testing.T) {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "engram")
	if err := os.WriteFile(bin, []byte("stub"), 0o755); err != nil {
		t.Fatalf("seed engram binary: %v", err)
	}
	t.Setenv("CLICK_ENGRAM_BINARY_PATH", bin)

	// checkEngramPath (`click doctor`'s PERSISTED-vs-LIVE PATH drift diagnosis, sdd/engram-mcp-
	// resolution obs #1436) reads the REAL platform pathStore (Windows registry / POSIX shell rc)
	// and the REAL live process PATH by default — neither of which this fake, hermetic CLI test
	// suite controls or wants to depend on. Force both probes to report a healthy (persisted AND
	// live) state so `click doctor` after a successful install is deterministic here regardless of
	// the actual test machine's real PATH/registry state.
	t.Cleanup(installer.SetPathPersistedProbeForTests(func(dir string) (bool, error) { return true, nil }))
	t.Cleanup(installer.SetLivePathContainsProbeForTests(func(dir string) bool { return true }))
}

// seedResolvableClickBinary makes `click doctor`'s checkClickBinary check (internal/doctor/checks.go)
// see the click binary as resolvable on PATH, via doctor.SetClickBinaryLookupForTests, so
// install-then-doctor CLI tests are deterministic regardless of whether the real test machine
// actually has click installed to PATH. Without this, these tests only pass by accident of the host
// environment: a developer machine with click on PATH (e.g. via scoop) is healthy, but CI — which
// never installs the binary it just built to PATH — reports checkClickBinary FAIL and the whole
// doctor report unhealthy, exactly mirroring seedResolvableEngram's rationale above for the engram
// binary/PATH checks.
func seedResolvableClickBinary(t *testing.T) {
	t.Helper()
	t.Cleanup(doctor.SetClickBinaryLookupForTests(func(string) (string, error) { return "/usr/bin/click", nil }))
}

// cliFakeBinaryLookup fakes PATH resolution for installer.BinaryLookup, mirroring the same
// injectable pattern already used for the Engram binary/Go toolchain lookups
// (internal/installer/engram_test.go's fakeBinaryLookup) so CLI-level tests never depend on
// whether the real test machine happens to have git on PATH.
type cliFakeBinaryLookup struct {
	resolved map[string]string
}

func (f cliFakeBinaryLookup) LookPath(name string) (string, error) {
	if path, ok := f.resolved[name]; ok {
		return path, nil
	}
	return "", fmt.Errorf("cliFakeBinaryLookup: not found: %s", name)
}

// seedResolvableGit makes installer.GitAvailable/GitPath AND installer.ClaudeAvailable/ClaudePath
// (and therefore runInstall/runUpdate's PreflightClaude + PreflightGit) see both binaries as
// resolvable, so `execRoot`'s install/update-driven CLI tests are deterministic regardless of the
// host PATH — the same rationale as seedResolvableEngram above. execRoot calls this by default;
// tests that specifically exercise a "missing" preflight path build their own cobra command
// instead of going through execRoot (see TestInstallCommand_GitMissing_AbortsBeforeMarketplace-
// Registration / TestInstallCommand_ClaudeMissing_AbortsBeforeMarketplaceRegistration).
func seedResolvableGit(t *testing.T) {
	t.Helper()
	restore := installer.SetBinaryLookupFactoryForTests(func() installer.BinaryLookup {
		return cliFakeBinaryLookup{resolved: map[string]string{"git": "/usr/bin/git", "claude": "/usr/bin/claude"}}
	})
	t.Cleanup(restore)
}

func execRoot(t *testing.T, claudeHome string, args ...string) (string, error) {
	t.Helper()
	t.Setenv("CLICK_CLAUDE_HOME", claudeHome)
	seedResolvableGit(t)

	root := NewRootCommand()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	// Pinned to an empty buffer (not left to default to the real os.Stdin) so every CLI test —
	// including the default no-arg action's interactive() TTY gate — is deterministic regardless
	// of what stdin happens to be under the test runner.
	root.SetIn(&bytes.Buffer{})
	root.SetArgs(args)

	err := root.Execute()
	return buf.String(), err
}

type testCommandRunner struct {
	home          string
	commands      []string
	plugins       map[string]bool
	pluginConfigs map[string]map[string]string
}

func newTestCommandRunner(home string) *testCommandRunner {
	return &testCommandRunner{home: home, plugins: map[string]bool{}, pluginConfigs: map[string]map[string]string{}}
}

func (r *testCommandRunner) Run(name string, args ...string) error {
	r.commands = append(r.commands, name+" "+strings.Join(args, " "))
	if name != "claude" || len(args) < 2 {
		return nil
	}
	switch {
	case len(args) >= 4 && args[0] == "plugin" && args[1] == "marketplace" && args[2] == "add":
		return r.writeSettings()
	case len(args) >= 3 && args[0] == "plugin" && args[1] == "install":
		r.plugins[args[2]] = true
		// Mirrors the real `claude` CLI: repeated `--config key=value` flags land in settings.json's
		// pluginConfigs[pluginID].options — the shape installer.AppliedClickSDDPluginConfig / doctor's
		// checkAppliedPluginConfig read back.
		if options := parseTestConfigFlags(args[3:]); len(options) > 0 {
			r.pluginConfigs[args[2]] = options
		}
		return r.writeSettings()
	case len(args) >= 3 && args[0] == "plugin" && args[1] == "uninstall":
		delete(r.plugins, args[2])
		return r.writeSettings()
	case len(args) >= 4 && args[0] == "plugin" && args[1] == "marketplace" && args[2] == "remove":
		return nil
	case len(args) >= 3 && args[0] == "mcp" && args[1] == "add":
		return r.upsertMCPServer(args[len(args)-2], args[len(args)-1])
	case len(args) >= 3 && args[0] == "mcp" && args[1] == "remove":
		return r.removeMCPServer(args[2])
	default:
		return nil
	}
}

// upsertMCPServer/removeMCPServer simulate `claude mcp add/remove`'s effect on Claude Code's own
// user-scope config file — the same file installer.HasContext7 reads directly (confirmed against
// the real CLI in documentacion/spikes/spike-g-context7.md), mirroring the equivalent simulation
// already used by installer package's own fakeCommandRunner (internal/installer/plugins.go).
func (r *testCommandRunner) context7ConfigPath() string {
	return filepath.Join(r.home, ".claude.json")
}

func (r *testCommandRunner) upsertMCPServer(name, url string) error {
	data := map[string]any{}
	if existing, err := os.ReadFile(r.context7ConfigPath()); err == nil {
		_ = json.Unmarshal(existing, &data)
	}
	servers, _ := data["mcpServers"].(map[string]any)
	if servers == nil {
		servers = map[string]any{}
	}
	servers[name] = map[string]any{"type": "http", "url": url}
	data["mcpServers"] = servers
	raw, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return os.WriteFile(r.context7ConfigPath(), raw, 0o600)
}

func (r *testCommandRunner) removeMCPServer(name string) error {
	data := map[string]any{}
	existing, err := os.ReadFile(r.context7ConfigPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if err := json.Unmarshal(existing, &data); err != nil {
		return err
	}
	if servers, ok := data["mcpServers"].(map[string]any); ok {
		delete(servers, name)
		data["mcpServers"] = servers
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return os.WriteFile(r.context7ConfigPath(), raw, 0o600)
}

// Output answers `go env GOBIN`/`go env GOPATH` with a realistic, deterministic result — GOBIN
// unset (the common case, empty string, same as a real machine that never ran `go env -w
// GOBIN=...`) and GOPATH resolving under the fake ClaudeHome — so installer.GoBinDir (consulted by
// EnsureEngramBinary's persistPathToBinaryDir and by `click doctor`'s checkEngramPath,
// sdd/engram-mcp-resolution obs #1436) always resolves successfully here, instead of erroring on
// the previous unconditional empty-bytes stub. Every other command still gets the previous
// no-op/empty-bytes behavior.
func (r *testCommandRunner) Output(name string, args ...string) ([]byte, error) {
	r.commands = append(r.commands, name+" "+strings.Join(args, " "))
	if name == "go" && len(args) == 2 && args[0] == "env" {
		switch args[1] {
		case "GOBIN":
			return []byte("\n"), nil
		case "GOPATH":
			return []byte(filepath.Join(r.home, "go") + "\n"), nil
		}
	}
	return []byte{}, nil
}

// parseTestConfigFlags extracts the "--config key=value" pairs from a `plugin install` argument
// tail, mirroring how the real `claude` CLI turns repeated --config flags into
// pluginConfigs[pluginID].options entries.
func parseTestConfigFlags(args []string) map[string]string {
	options := map[string]string{}
	for i := 0; i < len(args)-1; i++ {
		if args[i] != "--config" {
			continue
		}
		key, value, ok := strings.Cut(args[i+1], "=")
		if !ok {
			continue
		}
		options[key] = value
	}
	return options
}

func (r *testCommandRunner) writeSettings() error {
	plugins := map[string]any{}
	enabled := map[string]bool{}
	for pluginID := range r.plugins {
		plugins[pluginID] = []map[string]any{{"scope": "user", "version": "0.1.0"}}
		enabled[pluginID] = true
	}
	pluginsData, err := json.Marshal(map[string]any{"version": 2, "plugins": plugins})
	if err != nil {
		return err
	}
	pluginsPath := filepath.Join(r.home, "plugins", "installed_plugins.json")
	if err := os.MkdirAll(filepath.Dir(pluginsPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(pluginsPath, pluginsData, 0o600); err != nil {
		return err
	}
	pluginConfigs := map[string]any{}
	for pluginID, options := range r.pluginConfigs {
		optionsAny := make(map[string]any, len(options))
		for k, v := range options {
			optionsAny[k] = v
		}
		pluginConfigs[pluginID] = map[string]any{"options": optionsAny}
	}
	settingsData, err := json.Marshal(map[string]any{"enabledPlugins": enabled, "pluginConfigs": pluginConfigs})
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(r.home, "settings.json"), settingsData, 0o600)
}

func TestInstallCommand_Succeeds(t *testing.T) {
	home := t.TempDir()
	runner := newTestCommandRunner(home)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()
	restoreSource := installer.SetMarketplaceSourceForTests("https://github.com/Angel-MercadoCLK/click-ai-devkit")
	defer restoreSource()

	out, err := execRoot(t, home, "install")
	if err != nil {
		t.Fatalf("install command error = %v, output:\n%s", err, out)
	}
	if !strings.Contains(out, "click-sdd") {
		t.Errorf("install output = %q, want it to mention the click-sdd plugin step", out)
	}
	if !strings.Contains(out, "click-memory") {
		t.Errorf("install output = %q, want it to mention the click-memory plugin step", out)
	}
	if !strings.Contains(out, "click-review") {
		t.Errorf("install output = %q, want it to mention the click-review plugin step", out)
	}
	if !strings.Contains(out, "CLAUDE.md") {
		t.Errorf("install output = %q, want it to mention the CLAUDE.md step", out)
	}
}

// execRootWithGitLookup is execRoot's shape, but wired with an explicit installer.BinaryLookup
// instead of execRoot's own default seedResolvableGit — used exclusively by the "git missing"
// preflight tests below, where the whole point is to simulate git being absent from PATH.
func execRootWithGitLookup(t *testing.T, claudeHome string, lookup installer.BinaryLookup, args ...string) (string, error) {
	t.Helper()
	t.Setenv("CLICK_CLAUDE_HOME", claudeHome)
	restore := installer.SetBinaryLookupFactoryForTests(func() installer.BinaryLookup { return lookup })
	defer restore()

	root := NewRootCommand()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetIn(&bytes.Buffer{})
	root.SetArgs(args)

	err := root.Execute()
	return buf.String(), err
}

// TestInstallCommand_GitMissing_AbortsBeforeMarketplaceRegistration reproduces the exact bug
// fixed here — `click install` on a machine with no git on PATH used to fail deep inside plugin
// registration with a cryptic "Command git not found or is in an unsafe location" error, well
// after the interactive model-selection TUI had already run. With PreflightGit wired in at the top
// of runInstall, a missing git must now abort BEFORE any marketplace/plugin registration command is
// ever issued — verified here via the fake CommandRunner recording zero commands at all.
func TestInstallCommand_GitMissing_AbortsBeforeMarketplaceRegistration(t *testing.T) {
	home := t.TempDir()
	runner := newTestCommandRunner(home)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()
	// claude resolvable so PreflightClaude (which now runs first) doesn't mask the git-missing
	// scenario this test targets.
	missingGit := cliFakeBinaryLookup{resolved: map[string]string{"claude": "/usr/bin/claude"}}

	out, err := execRootWithGitLookup(t, home, missingGit, "install")
	if err == nil {
		t.Fatalf("install command error = nil when git is missing from PATH, want a non-nil actionable error, output:\n%s", out)
	}
	if !strings.Contains(err.Error(), installer.GitMissingMessage) {
		t.Fatalf("install command error = %q, want it to contain the actionable message %q", err.Error(), installer.GitMissingMessage)
	}
	if len(runner.commands) != 0 {
		t.Fatalf("runner.commands = %#v, want zero commands issued — install must abort before touching the marketplace when git is missing", runner.commands)
	}
}

// TestInstallCommand_ClaudeMissing_AbortsBeforeMarketplaceRegistration mirrors the git-missing test
// above for T1-2's PreflightClaude: on a machine with no Claude Code installed, `click install`
// used to run its whole interactive TUI and then die with a raw Go dump ("exec: \"claude\":
// executable file not found in %PATH%") deep inside SyncMarketplacePlugins. With PreflightClaude
// wired in at the very top of runInstall (before even PreflightGit), a missing claude must abort
// BEFORE any marketplace/plugin registration command is ever issued.
func TestInstallCommand_ClaudeMissing_AbortsBeforeMarketplaceRegistration(t *testing.T) {
	home := t.TempDir()
	runner := newTestCommandRunner(home)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()
	// git resolvable so this test isolates the claude-missing scenario specifically.
	missingClaude := cliFakeBinaryLookup{resolved: map[string]string{"git": "/usr/bin/git"}}

	out, err := execRootWithGitLookup(t, home, missingClaude, "install")
	if err == nil {
		t.Fatalf("install command error = nil when claude is missing from PATH, want a non-nil actionable error, output:\n%s", out)
	}
	if !strings.Contains(err.Error(), installer.ClaudeMissingMessage) {
		t.Fatalf("install command error = %q, want it to contain the actionable message %q", err.Error(), installer.ClaudeMissingMessage)
	}
	if len(runner.commands) != 0 {
		t.Fatalf("runner.commands = %#v, want zero commands issued — install must abort before touching the marketplace when claude is missing", runner.commands)
	}
}

// TestInstallCommand_GitPresent_ProceedsToMarketplaceRegistration is the GREEN counterpart: with
// git resolvable, PreflightGit must not block the install, and marketplace registration proceeds
// exactly as before this fix.
func TestInstallCommand_GitPresent_ProceedsToMarketplaceRegistration(t *testing.T) {
	home := t.TempDir()
	runner := newTestCommandRunner(home)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()
	presentGit := cliFakeBinaryLookup{resolved: map[string]string{"git": "/usr/bin/git", "claude": "/usr/bin/claude"}}

	out, err := execRootWithGitLookup(t, home, presentGit, "install")
	if err != nil {
		t.Fatalf("install command error = %v when git is present, output:\n%s", err, out)
	}
	if len(runner.commands) == 0 {
		t.Fatal("runner.commands is empty, want install to have issued marketplace/plugin registration commands when git is present")
	}
	wantCommand := "claude plugin marketplace add"
	found := false
	for _, c := range runner.commands {
		if strings.HasPrefix(c, wantCommand) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("runner.commands = %#v, want it to contain a %q command", runner.commands, wantCommand)
	}
}

// TestUpdateCommand_GitMissing_AbortsBeforeMarketplaceRegistration mirrors the install preflight
// test above for `click update`, which also re-syncs the marketplace (SyncMarketplacePlugins) and
// therefore also needs git.
func TestUpdateCommand_GitMissing_AbortsBeforeMarketplaceRegistration(t *testing.T) {
	home := t.TempDir()
	runner := newTestCommandRunner(home)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()
	// claude resolvable so PreflightClaude (which now runs first) doesn't mask the git-missing
	// scenario this test targets.
	missingGit := cliFakeBinaryLookup{resolved: map[string]string{"claude": "/usr/bin/claude"}}

	out, err := execRootWithGitLookup(t, home, missingGit, "update")
	if err == nil {
		t.Fatalf("update command error = nil when git is missing from PATH, want a non-nil actionable error, output:\n%s", out)
	}
	if !strings.Contains(err.Error(), installer.GitMissingMessage) {
		t.Fatalf("update command error = %q, want it to contain the actionable message %q", err.Error(), installer.GitMissingMessage)
	}
	if len(runner.commands) != 0 {
		t.Fatalf("runner.commands = %#v, want zero commands issued — update must abort before touching the marketplace when git is missing", runner.commands)
	}
}

// TestUpdateCommand_ClaudeMissing_AbortsBeforeMarketplaceRegistration mirrors the install-side
// claude-missing test above for `click update`, which also re-syncs the marketplace
// (SyncMarketplacePlugins) via the claude CLI and therefore also needs PreflightClaude wired in.
func TestUpdateCommand_ClaudeMissing_AbortsBeforeMarketplaceRegistration(t *testing.T) {
	home := t.TempDir()
	runner := newTestCommandRunner(home)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()
	// git resolvable so this test isolates the claude-missing scenario specifically.
	missingClaude := cliFakeBinaryLookup{resolved: map[string]string{"git": "/usr/bin/git"}}

	out, err := execRootWithGitLookup(t, home, missingClaude, "update")
	if err == nil {
		t.Fatalf("update command error = nil when claude is missing from PATH, want a non-nil actionable error, output:\n%s", out)
	}
	if !strings.Contains(err.Error(), installer.ClaudeMissingMessage) {
		t.Fatalf("update command error = %q, want it to contain the actionable message %q", err.Error(), installer.ClaudeMissingMessage)
	}
	if len(runner.commands) != 0 {
		t.Fatalf("runner.commands = %#v, want zero commands issued — update must abort before touching the marketplace when claude is missing", runner.commands)
	}
}

// TestInstallCommand_FreshInstall_NoBackupCreated guards that a fresh install (no prior
// models.json) behaves exactly as before the migration-wiring fix: no spurious models.json.bak
// gets created when there is nothing stale to back up.
func TestInstallCommand_FreshInstall_NoBackupCreated(t *testing.T) {
	home := t.TempDir()
	runner := newTestCommandRunner(home)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()

	if _, err := execRoot(t, home, "install"); err != nil {
		t.Fatalf("install command error = %v", err)
	}

	cfg := installer.Config{ClaudeHome: home}
	if _, err := os.Stat(cfg.ModelsPath() + ".bak"); !os.IsNotExist(err) {
		t.Fatalf("Stat(models.json.bak) error = %v, want no backup on a fresh install", err)
	}
}

// TestInstallCommand_MigratesStaleModelsBeforeOverwriting guards the D8 "never clobber a working
// setup without a backup" safety net for `click install`, consistent with `click update`: if
// models.json is stale (pre-taxonomy-realignment), `click install` must back it up to
// models.json.bak before overwriting it with fresh new-taxonomy defaults.
func TestInstallCommand_MigratesStaleModelsBeforeOverwriting(t *testing.T) {
	home := t.TempDir()
	runner := newTestCommandRunner(home)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()

	cfg := installer.Config{ClaudeHome: home}
	legacyRaw, err := json.Marshal(map[string]string{
		"orchestrator":   "haiku",
		"prd_writer":     "opus",
		"architect":      "opus",
		"reviewer":       "opus",
		"memory_curator": "sonnet",
	})
	if err != nil {
		t.Fatalf("json.Marshal(legacy) error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(cfg.ModelsPath()), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(cfg.ModelsPath(), legacyRaw, 0o600); err != nil {
		t.Fatalf("WriteFile(legacy models.json) error = %v", err)
	}

	if _, err := execRoot(t, home, "install"); err != nil {
		t.Fatalf("install command error = %v", err)
	}

	backup, err := os.ReadFile(cfg.ModelsPath() + ".bak")
	if err != nil {
		t.Fatalf("ReadFile(models.json.bak) error = %v, want the stale file backed up before overwrite", err)
	}
	if string(backup) != string(legacyRaw) {
		t.Fatalf("models.json.bak = %q, want verbatim legacy content %q", backup, legacyRaw)
	}

	stale, err := installer.IsStale(cfg)
	if err != nil {
		t.Fatalf("IsStale() error = %v", err)
	}
	if stale {
		t.Fatal("models.json is still stale after `click install`, want migrated to current schema")
	}
}

// TestInstallCommand_InteractiveCancel_LeavesModelsUntouched guards that cancelling the interactive
// model-selection TUI leaves any existing (even stale) models.json completely untouched: cancel must
// mean "no changes", not "migrated then abandoned". Regression test for R3-001, where MigrateIfStale
// ran unconditionally before the cancel check, so a cancelled install still backed up and regenerated
// a stale models.json.
func TestInstallCommand_InteractiveCancel_LeavesModelsUntouched(t *testing.T) {
	home := t.TempDir()
	cfg := installer.Config{ClaudeHome: home}
	legacyRaw, err := json.Marshal(map[string]string{
		"orchestrator":   "haiku",
		"prd_writer":     "opus",
		"architect":      "opus",
		"reviewer":       "opus",
		"memory_curator": "sonnet",
	})
	if err != nil {
		t.Fatalf("json.Marshal(legacy) error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(cfg.ModelsPath()), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(cfg.ModelsPath(), legacyRaw, 0o600); err != nil {
		t.Fatalf("WriteFile(legacy models.json) error = %v", err)
	}

	cmd := NewRootCommand()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	r := rendererFor(cmd, &buf)

	cancelledSelector := func(*cobra.Command, modelconfig.ProfileName) (modelconfig.ProfileName, map[modelconfig.Phase]string, bool, error) {
		return "", nil, true, nil
	}

	// nonInteractive=false forces the interactive branch regardless of the test's non-TTY buffer,
	// so the fake selector's cancel is what actually gets exercised.
	_, _, cancelled, err := resolveInstallModels(cmd, &buf, r, cfg, false, "", cancelledSelector)
	if err != nil {
		t.Fatalf("resolveInstallModels() error = %v", err)
	}
	if !cancelled {
		t.Fatal("resolveInstallModels() cancelled = false, want true")
	}

	if _, err := os.Stat(cfg.ModelsPath() + ".bak"); !os.IsNotExist(err) {
		t.Fatalf("Stat(models.json.bak) error = %v, want no backup created on interactive cancel", err)
	}
	got, err := os.ReadFile(cfg.ModelsPath())
	if err != nil {
		t.Fatalf("ReadFile(models.json) error = %v", err)
	}
	if string(got) != string(legacyRaw) {
		t.Fatalf("models.json = %q, want unchanged legacy content %q (cancel must leave disk untouched)", got, legacyRaw)
	}
}

func TestDoctorCommand_AfterInstall_Succeeds(t *testing.T) {
	seedResolvableEngram(t)
	seedResolvableClickBinary(t)
	home := t.TempDir()
	runner := newTestCommandRunner(home)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()

	if _, err := execRoot(t, home, "install"); err != nil {
		t.Fatalf("install command error = %v", err)
	}

	out, err := execRoot(t, home, "doctor")
	if err != nil {
		t.Fatalf("doctor command error = %v after a successful install, output:\n%s", err, out)
	}
}

func TestDoctorCommand_BeforeInstall_ReturnsError(t *testing.T) {
	home := t.TempDir()

	_, err := execRoot(t, home, "doctor")
	if err == nil {
		t.Fatal("doctor command on a never-installed ClaudeHome returned nil error, want a non-nil error (non-zero exit)")
	}
}

func TestUninstallCommand_ReversesInstall(t *testing.T) {
	seedResolvableEngram(t)
	seedResolvableClickBinary(t)
	home := t.TempDir()
	runner := newTestCommandRunner(home)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()

	if _, err := execRoot(t, home, "install"); err != nil {
		t.Fatalf("install command error = %v", err)
	}
	if _, err := execRoot(t, home, "doctor"); err != nil {
		t.Fatalf("doctor command error = %v right after install", err)
	}

	if _, err := execRoot(t, home, "uninstall"); err != nil {
		t.Fatalf("uninstall command error = %v", err)
	}

	_, err := execRoot(t, home, "doctor")
	if err == nil {
		t.Fatal("doctor command after uninstall returned nil error, want a non-nil error (unhealthy)")
	}
}

func TestUninstallCommand_NoopWhenAlreadyUninstalled(t *testing.T) {
	home := t.TempDir()
	runner := newTestCommandRunner(home)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()

	if _, err := execRoot(t, home, "uninstall"); err != nil {
		t.Fatalf("uninstall command on a never-installed ClaudeHome error = %v, want nil", err)
	}
}

func TestUpdateCommand_ResyncsPluginsAndWritesEngramPin(t *testing.T) {
	home := t.TempDir()
	runner := newTestCommandRunner(home)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()
	binaryPath := filepath.Join(t.TempDir(), "engram.exe")
	if err := os.WriteFile(binaryPath, []byte("binary"), 0o644); err != nil {
		t.Fatalf("WriteFile(binary) error = %v", err)
	}
	t.Setenv("CLICK_ENGRAM_BINARY_PATH", binaryPath)

	if _, err := execRoot(t, home, "install"); err != nil {
		t.Fatalf("install command error = %v", err)
	}

	out, err := execRoot(t, home, "update")
	if err != nil {
		t.Fatalf("update command error = %v", err)
	}
	if !strings.Contains(out, "click-review") {
		t.Errorf("update output = %q, want it to mention the click-review re-sync step", out)
	}
	if !strings.Contains(out, "Engram") {
		t.Errorf("update output = %q, want it to mention the Engram pin step", out)
	}

	wantCommand := "claude plugin install click-review@click-ai-devkit"
	if !contains(runner.commands, wantCommand) {
		t.Fatalf("update command sequence = %#v, want it to contain %q", runner.commands, wantCommand)
	}

	// click no longer writes an MCP config file itself — Step 0 of Slice 3 proved Claude Code
	// never reads a hand-rolled <ClaudeHome>/mcp/engram.json; only the engram plugin's own
	// bundled .mcp.json is actually loaded. `click update` now just re-syncs the engram plugin
	// (idempotent) and refreshes click's own state bookkeeping file.
	if _, err := os.Stat(filepath.Join(home, "click-ai-devkit", "engram.json")); err != nil {
		t.Fatalf("update did not write engram pinned state: %v", err)
	}
}

// TestUpdateCommand_PersistsModelsJson is a regression test for an asymmetry bug: `click install`
// both applies AND saves models.json (installer.SaveModels), but `click update` only re-applied
// the per-phase model routing config without re-persisting it to disk. On a home where
// models.json was never written (e.g. install predates this feature, or the file was lost),
// `click update` must still leave models.json present and readable afterward, exactly like
// `click install` does.
func TestUpdateCommand_PersistsModelsJson(t *testing.T) {
	home := t.TempDir()
	runner := newTestCommandRunner(home)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()
	binaryPath := filepath.Join(t.TempDir(), "engram.exe")
	if err := os.WriteFile(binaryPath, []byte("binary"), 0o644); err != nil {
		t.Fatalf("WriteFile(binary) error = %v", err)
	}
	t.Setenv("CLICK_ENGRAM_BINARY_PATH", binaryPath)

	// No prior `click install` ran, so models.json does not exist yet — exercises update's own
	// fallback-to-defaults path (installer.LoadModels found=false), which must still persist.
	if _, err := execRoot(t, home, "update"); err != nil {
		t.Fatalf("update command error = %v", err)
	}

	got, found, err := installer.LoadModels(installer.Config{ClaudeHome: home})
	if err != nil {
		t.Fatalf("LoadModels() error = %v", err)
	}
	if !found {
		t.Fatal("LoadModels() found = false after `click update`, want true — update must persist models.json like install does")
	}
	want := modelconfig.Defaults()
	for phase, model := range want {
		if got[phase] != model {
			t.Errorf("persisted models[%s] = %q, want default %q", phase, got[phase], model)
		}
	}
}

func TestRootCommand_VersionDefaultsToDev(t *testing.T) {
	home := t.TempDir()
	out, err := execRoot(t, home, "--version")
	if err != nil {
		t.Fatalf("version command error = %v", err)
	}
	if !strings.Contains(out, "dev") {
		t.Fatalf("version output = %q, want it to contain dev", out)
	}
}

func TestInstallCommand_IssuesMarketplaceCommandsInOrder(t *testing.T) {
	home := t.TempDir()
	runner := newTestCommandRunner(home)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()
	restoreSource := installer.SetMarketplaceSourceForTests("https://github.com/Angel-MercadoCLK/click-ai-devkit")
	defer restoreSource()

	if _, err := execRoot(t, home, "install"); err != nil {
		t.Fatalf("install command error = %v", err)
	}
	want := []string{
		"claude plugin marketplace add https://github.com/Angel-MercadoCLK/click-ai-devkit --sparse .claude-plugin plugins",
		"claude plugin marketplace update click-ai-devkit",
		"claude plugin install click-sdd@click-ai-devkit" +
			" --config orchestration_profile=balanced" +
			" --config explore_model=sonnet" +
			" --config propose_model=opus" +
			" --config spec_model=sonnet" +
			" --config design_model=opus" +
			" --config tasks_model=sonnet" +
			" --config apply_model=sonnet" +
			" --config verify_model=opus" +
			" --config archive_model=haiku" +
			" --config onboard_model=haiku" +
			" --config jd_judge_a_model=sonnet" +
			" --config jd_judge_b_model=sonnet" +
			" --config jd_fix_agent_model=sonnet" +
			" --config review_risk_model=sonnet" +
			" --config review_readability_model=sonnet" +
			" --config review_reliability_model=sonnet" +
			" --config review_resilience_model=sonnet" +
			" --config review_refuter_model=sonnet" +
			" --config default_model=sonnet",
		"claude plugin install click-memory@click-ai-devkit",
		"claude plugin install click-review@click-ai-devkit",
		"claude plugin install click-skills@click-ai-devkit",
	}
	if !reflect.DeepEqual(runner.commands[:6], want) {
		t.Fatalf("runner.commands[:6] = %#v, want %#v", runner.commands[:6], want)
	}
}

// TestInstallCommand_NonTTY_DoesNotHangAndResolvesToBalanced guards the CRITICAL non-TTY/CI safety
// contract (design D4): a non-interactive `click install` with no --profile flag must return
// immediately — never block waiting for interactive input — and must persist the "balanced" profile
// label alongside its (Defaults()-equal) per-phase map. execRoot always wires stdin/stdout to a
// bytes.Buffer (never a real terminal), so isNonInteractiveInstall's TTY check already forces the
// non-interactive branch on every test in this file; this test makes that safety property explicit
// with its own elapsed-time assertion instead of relying on it implicitly.
func TestInstallCommand_NonTTY_DoesNotHangAndResolvesToBalanced(t *testing.T) {
	home := t.TempDir()
	runner := newTestCommandRunner(home)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()

	start := time.Now()
	if _, err := execRoot(t, home, "install"); err != nil {
		t.Fatalf("install command error = %v", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("install command took %s on a non-TTY buffer, want it to return immediately without blocking on interactive input", elapsed)
	}

	profile, models, found, err := installer.LoadModelsWithProfile(installer.Config{ClaudeHome: home})
	if err != nil {
		t.Fatalf("LoadModelsWithProfile() error = %v", err)
	}
	if !found {
		t.Fatal("LoadModelsWithProfile() found = false after install, want true")
	}
	if profile != modelconfig.ProfileBalanced {
		t.Fatalf("persisted profile = %q, want %q", profile, modelconfig.ProfileBalanced)
	}
	if !reflect.DeepEqual(models, modelconfig.Defaults()) {
		t.Fatalf("persisted models = %#v, want Defaults() %#v", models, modelconfig.Defaults())
	}
}

// TestInstallCommand_ProfileFlag_NonInteractive_PersistsChosenProfile guards --profile's
// non-interactive contract: the flag selects a built-in preset with no prompt, and BOTH the emitted
// --config flags AND the persisted models.json profile field must reflect it.
func TestInstallCommand_ProfileFlag_NonInteractive_PersistsChosenProfile(t *testing.T) {
	home := t.TempDir()
	runner := newTestCommandRunner(home)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()
	restoreSource := installer.SetMarketplaceSourceForTests("https://github.com/Angel-MercadoCLK/click-ai-devkit")
	defer restoreSource()

	if _, err := execRoot(t, home, "install", "--profile", "cost-saver"); err != nil {
		t.Fatalf("install command error = %v", err)
	}

	wantCommand := "claude plugin install click-sdd@click-ai-devkit" +
		" --config orchestration_profile=cost-saver" +
		" --config explore_model=haiku" +
		" --config propose_model=opus" +
		" --config spec_model=haiku" +
		" --config design_model=opus" +
		" --config tasks_model=haiku" +
		" --config apply_model=haiku" +
		" --config verify_model=opus" +
		" --config archive_model=haiku" +
		" --config onboard_model=haiku" +
		" --config jd_judge_a_model=haiku" +
		" --config jd_judge_b_model=haiku" +
		" --config jd_fix_agent_model=haiku" +
		" --config review_risk_model=haiku" +
		" --config review_readability_model=haiku" +
		" --config review_reliability_model=haiku" +
		" --config review_resilience_model=haiku" +
		" --config review_refuter_model=haiku" +
		" --config default_model=haiku"
	if !contains(runner.commands, wantCommand) {
		t.Fatalf("install command sequence = %#v, want it to contain %q", runner.commands, wantCommand)
	}

	profile, _, found, err := installer.LoadModelsWithProfile(installer.Config{ClaudeHome: home})
	if err != nil {
		t.Fatalf("LoadModelsWithProfile() error = %v", err)
	}
	if !found {
		t.Fatal("LoadModelsWithProfile() found = false after install, want true")
	}
	if profile != modelconfig.ProfileCostSaver {
		t.Fatalf("persisted profile = %q, want %q", profile, modelconfig.ProfileCostSaver)
	}
}

// TestInstallCommand_ProfileFlag_UnknownValue_FallsBackToBalanced guards that an unrecognized
// --profile value never hangs or errors — it silently falls back to balanced, matching
// modelconfig.ResolveProfile's own fallback rule.
func TestInstallCommand_ProfileFlag_UnknownValue_FallsBackToBalanced(t *testing.T) {
	home := t.TempDir()
	runner := newTestCommandRunner(home)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()

	if _, err := execRoot(t, home, "install", "--profile", "not-a-real-profile"); err != nil {
		t.Fatalf("install command error = %v", err)
	}

	profile, _, found, err := installer.LoadModelsWithProfile(installer.Config{ClaudeHome: home})
	if err != nil {
		t.Fatalf("LoadModelsWithProfile() error = %v", err)
	}
	if !found {
		t.Fatal("LoadModelsWithProfile() found = false after install, want true")
	}
	if profile != modelconfig.ProfileBalanced {
		t.Fatalf("persisted profile = %q, want %q (unknown --profile falls back)", profile, modelconfig.ProfileBalanced)
	}
}

// TestResolveInstallModels_Interactive_PresetUnmodified_PersistsPresetLabel and
// TestResolveInstallModels_Interactive_TweakedPreset_PersistsCustomLabel guard the label-consistency
// rule end to end through resolveInstallModels: a preset the developer left untouched keeps its own
// name, but a hand-tweaked preset downgrades to "custom" so the persisted label never claims values
// the map no longer holds.
func TestResolveInstallModels_Interactive_PresetUnmodified_PersistsPresetLabel(t *testing.T) {
	home := t.TempDir()
	cfg := installer.Config{ClaudeHome: home}
	cmd := NewRootCommand()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	r := rendererFor(cmd, &buf)

	unmodified := modelconfig.ResolveProfile(string(modelconfig.ProfileQuality)).Models
	selector := func(*cobra.Command, modelconfig.ProfileName) (modelconfig.ProfileName, map[modelconfig.Phase]string, bool, error) {
		return modelconfig.ProfileQuality, unmodified, false, nil
	}

	profile, models, cancelled, err := resolveInstallModels(cmd, &buf, r, cfg, false, "", selector)
	if err != nil {
		t.Fatalf("resolveInstallModels() error = %v", err)
	}
	if cancelled {
		t.Fatal("resolveInstallModels() cancelled = true, want false")
	}
	if profile != modelconfig.ProfileQuality {
		t.Fatalf("resolveInstallModels() profile = %q, want %q (unmodified preset keeps its own label)", profile, modelconfig.ProfileQuality)
	}
	if !reflect.DeepEqual(models, unmodified) {
		t.Fatalf("resolveInstallModels() models = %#v, want %#v", models, unmodified)
	}
}

func TestResolveInstallModels_Interactive_TweakedPreset_PersistsCustomLabel(t *testing.T) {
	home := t.TempDir()
	cfg := installer.Config{ClaudeHome: home}
	cmd := NewRootCommand()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	r := rendererFor(cmd, &buf)

	tweaked := modelconfig.ResolveProfile(string(modelconfig.ProfileQuality)).Models
	tweaked[modelconfig.PhaseExplore] = "haiku" // hand-edit away from quality's "opus"
	selector := func(*cobra.Command, modelconfig.ProfileName) (modelconfig.ProfileName, map[modelconfig.Phase]string, bool, error) {
		return modelconfig.ProfileQuality, tweaked, false, nil
	}

	profile, models, cancelled, err := resolveInstallModels(cmd, &buf, r, cfg, false, "", selector)
	if err != nil {
		t.Fatalf("resolveInstallModels() error = %v", err)
	}
	if cancelled {
		t.Fatal("resolveInstallModels() cancelled = true, want false")
	}
	if profile != modelconfig.ProfileCustom {
		t.Fatalf("resolveInstallModels() profile = %q, want %q (tweaked preset downgrades to custom)", profile, modelconfig.ProfileCustom)
	}
	if !reflect.DeepEqual(models, tweaked) {
		t.Fatalf("resolveInstallModels() models = %#v, want %#v (tweaked values must still be preserved)", models, tweaked)
	}
}

// TestResolveInstallModels_Interactive_ThreadsProfileFlagAsInitialSelection guards the C2 fix:
// `click install --profile X` on a real terminal must pass X through to the interactive selector
// as the picker's starting profile, instead of always hardcoding balanced. The fake selector here
// stands in for runInstallSelectTUI (a real bubbletea program can't be exercised headlessly), so
// this test verifies the wiring: the initial profile resolveInstallModels hands to the selector.
func TestResolveInstallModels_Interactive_ThreadsProfileFlagAsInitialSelection(t *testing.T) {
	home := t.TempDir()
	cfg := installer.Config{ClaudeHome: home}
	cmd := NewRootCommand()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	r := rendererFor(cmd, &buf)

	var gotInitial modelconfig.ProfileName
	selector := func(_ *cobra.Command, initial modelconfig.ProfileName) (modelconfig.ProfileName, map[modelconfig.Phase]string, bool, error) {
		gotInitial = initial
		return modelconfig.ProfileCostSaver, modelconfig.ResolveProfile(string(modelconfig.ProfileCostSaver)).Models, false, nil
	}

	if _, _, _, err := resolveInstallModels(cmd, &buf, r, cfg, false, "cost-saver", selector); err != nil {
		t.Fatalf("resolveInstallModels() error = %v", err)
	}
	if gotInitial != modelconfig.ProfileCostSaver {
		t.Fatalf("selector received initial profile = %q, want %q (--profile flag must be threaded into the interactive picker)", gotInitial, modelconfig.ProfileCostSaver)
	}
}

// TestInstallCommand_NonTTY_PersistsDefaultModels guards D25's persistence contract for the
// non-interactive path (which every test in this file exercises, since execRoot writes to a
// bytes.Buffer, not a real terminal): a plain `click install` with no flags must still write
// models.json with the five defaults, so `click doctor`/`click update` have something to read.
func TestInstallCommand_NonTTY_PersistsDefaultModels(t *testing.T) {
	home := t.TempDir()
	runner := newTestCommandRunner(home)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()

	if _, err := execRoot(t, home, "install"); err != nil {
		t.Fatalf("install command error = %v", err)
	}

	got, found, err := installer.LoadModels(installer.Config{ClaudeHome: home})
	if err != nil {
		t.Fatalf("LoadModels() error = %v", err)
	}
	if !found {
		t.Fatal("LoadModels() found = false after install, want true")
	}
	want := modelconfig.Defaults()
	for phase, model := range want {
		if got[phase] != model {
			t.Errorf("persisted models[%s] = %q, want default %q", phase, got[phase], model)
		}
	}
}

// TestUpdateCommand_ReappliesPersistedModels guards the "click update re-passes the same --config
// flags" contract: a non-default model selection saved by a previous install must be re-emitted
// verbatim on the next update, not silently reset to defaults. The custom map is persisted via
// SaveModelsWithProfile with an explicit "custom" label (C1 fix): the previous version of this test
// persisted it via the profile-dropping SaveModels(cfg, custom) and then asserted
// orchestration_profile=balanced was re-emitted — that assertion pinned the C1 bug (configure-models
// silently erasing the persisted profile label) instead of guarding correct behavior. update.go
// itself only re-emits whatever label LoadModelsWithProfile returns, so a correctly persisted
// "custom" label must round-trip as "custom", not get silently relabeled to "balanced".
func TestUpdateCommand_ReappliesPersistedModels(t *testing.T) {
	home := t.TempDir()
	runner := newTestCommandRunner(home)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()

	if _, err := execRoot(t, home, "install"); err != nil {
		t.Fatalf("install command error = %v", err)
	}

	custom := map[modelconfig.Phase]string{
		modelconfig.PhaseExplore:           "haiku",
		modelconfig.PhasePropose:           "sonnet",
		modelconfig.PhaseSpec:              "haiku",
		modelconfig.PhaseDesign:            "sonnet",
		modelconfig.PhaseTasks:             "haiku",
		modelconfig.PhaseApply:             "sonnet",
		modelconfig.PhaseVerify:            "haiku",
		modelconfig.PhaseArchive:           "sonnet",
		modelconfig.PhaseOnboard:           "sonnet",
		modelconfig.PhaseJDJudgeA:          "haiku",
		modelconfig.PhaseJDJudgeB:          "haiku",
		modelconfig.PhaseJDFixAgent:        "haiku",
		modelconfig.PhaseReviewRisk:        "opus",
		modelconfig.PhaseReviewReadability: "sonnet",
		modelconfig.PhaseReviewReliability: "haiku",
		modelconfig.PhaseReviewResilience:  "opus",
		modelconfig.PhaseReviewRefuter:     "sonnet",
		modelconfig.PhaseDefault:           "haiku",
	}
	if err := installer.SaveModelsWithProfile(installer.Config{ClaudeHome: home}, modelconfig.ProfileCustom, custom); err != nil {
		t.Fatalf("SaveModelsWithProfile() error = %v", err)
	}

	if _, err := execRoot(t, home, "update"); err != nil {
		t.Fatalf("update command error = %v", err)
	}

	wantCommand := "claude plugin install click-sdd@click-ai-devkit" +
		" --config orchestration_profile=custom" +
		" --config explore_model=haiku" +
		" --config propose_model=sonnet" +
		" --config spec_model=haiku" +
		" --config design_model=sonnet" +
		" --config tasks_model=haiku" +
		" --config apply_model=sonnet" +
		" --config verify_model=haiku" +
		" --config archive_model=sonnet" +
		" --config onboard_model=sonnet" +
		" --config jd_judge_a_model=haiku" +
		" --config jd_judge_b_model=haiku" +
		" --config jd_fix_agent_model=haiku" +
		" --config review_risk_model=opus" +
		" --config review_readability_model=sonnet" +
		" --config review_reliability_model=haiku" +
		" --config review_resilience_model=opus" +
		" --config review_refuter_model=sonnet" +
		" --config default_model=haiku"
	if !contains(runner.commands, wantCommand) {
		t.Fatalf("update command sequence = %#v, want it to contain %q", runner.commands, wantCommand)
	}
}

// TestUpdateCommand_MigratesStaleModelsBeforeReapplying guards the confirmed migration wiring: if
// models.json is stale (pre-taxonomy-realignment), `click update` must back it up to
// models.json.bak and regenerate fresh new-taxonomy defaults BEFORE re-emitting --config flags —
// never re-emitting the old invented-taxonomy keys, and never silently carrying forward the old
// per-phase overrides.
func TestUpdateCommand_MigratesStaleModelsBeforeReapplying(t *testing.T) {
	home := t.TempDir()
	runner := newTestCommandRunner(home)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()
	binaryPath := filepath.Join(t.TempDir(), "engram.exe")
	if err := os.WriteFile(binaryPath, []byte("binary"), 0o644); err != nil {
		t.Fatalf("WriteFile(binary) error = %v", err)
	}
	t.Setenv("CLICK_ENGRAM_BINARY_PATH", binaryPath)

	cfg := installer.Config{ClaudeHome: home}
	legacyRaw, err := json.Marshal(map[string]string{
		"orchestrator":   "haiku",
		"prd_writer":     "opus",
		"architect":      "opus",
		"reviewer":       "opus",
		"memory_curator": "sonnet",
	})
	if err != nil {
		t.Fatalf("json.Marshal(legacy) error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(cfg.ModelsPath()), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(cfg.ModelsPath(), legacyRaw, 0o600); err != nil {
		t.Fatalf("WriteFile(legacy models.json) error = %v", err)
	}

	if _, err := execRoot(t, home, "update"); err != nil {
		t.Fatalf("update command error = %v", err)
	}

	if _, err := os.Stat(cfg.ModelsPath() + ".bak"); err != nil {
		t.Fatalf("Stat(models.json.bak) error = %v, want the stale file backed up before regen", err)
	}

	stale, err := installer.IsStale(cfg)
	if err != nil {
		t.Fatalf("IsStale() error = %v", err)
	}
	if stale {
		t.Fatal("models.json is still stale after `click update`, want migrated to current schema")
	}

	wantCommand := "claude plugin install click-sdd@click-ai-devkit" +
		" --config orchestration_profile=balanced" +
		" --config explore_model=sonnet" +
		" --config propose_model=opus" +
		" --config spec_model=sonnet" +
		" --config design_model=opus" +
		" --config tasks_model=sonnet" +
		" --config apply_model=sonnet" +
		" --config verify_model=opus" +
		" --config archive_model=haiku" +
		" --config onboard_model=haiku" +
		" --config jd_judge_a_model=sonnet" +
		" --config jd_judge_b_model=sonnet" +
		" --config jd_fix_agent_model=sonnet" +
		" --config review_risk_model=sonnet" +
		" --config review_readability_model=sonnet" +
		" --config review_reliability_model=sonnet" +
		" --config review_resilience_model=sonnet" +
		" --config review_refuter_model=sonnet" +
		" --config default_model=sonnet"
	if !contains(runner.commands, wantCommand) {
		t.Fatalf("update command sequence = %#v, want it to contain fresh-defaults command %q", runner.commands, wantCommand)
	}
}

// TestUpdateCommand_ReappliesPersistedProfile guards the "active profile re-applied on update" spec
// scenario: a non-balanced profile chosen at install time must be re-emitted (and re-persisted)
// verbatim on the next `click update`, with no interactive prompt.
func TestUpdateCommand_ReappliesPersistedProfile(t *testing.T) {
	home := t.TempDir()
	runner := newTestCommandRunner(home)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()

	if _, err := execRoot(t, home, "install", "--profile", "quality"); err != nil {
		t.Fatalf("install command error = %v", err)
	}

	if _, err := execRoot(t, home, "update"); err != nil {
		t.Fatalf("update command error = %v", err)
	}

	wantCommand := "claude plugin install click-sdd@click-ai-devkit" +
		" --config orchestration_profile=quality" +
		" --config explore_model=opus" +
		" --config propose_model=opus" +
		" --config spec_model=opus" +
		" --config design_model=opus" +
		" --config tasks_model=opus" +
		" --config apply_model=opus" +
		" --config verify_model=opus" +
		" --config archive_model=haiku" +
		" --config onboard_model=haiku" +
		" --config jd_judge_a_model=opus" +
		" --config jd_judge_b_model=opus" +
		" --config jd_fix_agent_model=opus" +
		" --config review_risk_model=opus" +
		" --config review_readability_model=opus" +
		" --config review_reliability_model=opus" +
		" --config review_resilience_model=opus" +
		" --config review_refuter_model=opus" +
		" --config default_model=opus"
	if !contains(runner.commands, wantCommand) {
		t.Fatalf("update command sequence = %#v, want it to contain %q", runner.commands, wantCommand)
	}

	profile, _, found, err := installer.LoadModelsWithProfile(installer.Config{ClaudeHome: home})
	if err != nil {
		t.Fatalf("LoadModelsWithProfile() error = %v", err)
	}
	if !found {
		t.Fatal("LoadModelsWithProfile() found = false after update, want true")
	}
	if profile != modelconfig.ProfileQuality {
		t.Fatalf("persisted profile after update = %q, want %q", profile, modelconfig.ProfileQuality)
	}
}

func TestUpdateCommand_NormalizesPartialProfileModelsJson(t *testing.T) {
	seedResolvableEngram(t)
	home := t.TempDir()
	runner := newTestCommandRunner(home)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()

	cfg := installer.Config{ClaudeHome: home}
	partial := oldThirteenPhaseModelsForProfile(modelconfig.ProfileCostSaver)
	if err := installer.SaveModelsWithProfile(cfg, modelconfig.ProfileCostSaver, partial); err != nil {
		t.Fatalf("SaveModelsWithProfile(partial cost-saver) error = %v", err)
	}

	if _, err := execRoot(t, home, "update"); err != nil {
		t.Fatalf("update command error = %v", err)
	}

	profile, got, found, err := installer.LoadModelsWithProfile(cfg)
	if err != nil {
		t.Fatalf("LoadModelsWithProfile() error = %v", err)
	}
	if !found {
		t.Fatal("LoadModelsWithProfile() found = false after update, want true")
	}
	if profile != modelconfig.ProfileCostSaver {
		t.Fatalf("persisted profile after update = %q, want %q", profile, modelconfig.ProfileCostSaver)
	}
	if len(got) != len(modelconfig.Phases) {
		t.Fatalf("persisted models has %d phases, want %d: %#v", len(got), len(modelconfig.Phases), got)
	}
	for _, phase := range []modelconfig.Phase{
		modelconfig.PhaseReviewRisk,
		modelconfig.PhaseReviewReadability,
		modelconfig.PhaseReviewReliability,
		modelconfig.PhaseReviewResilience,
		modelconfig.PhaseReviewRefuter,
	} {
		if got[phase] != "haiku" {
			t.Fatalf("persisted models[%s] = %q, want haiku from active cost-saver profile", phase, got[phase])
		}
	}
}

func TestDoctorModelsLine_ResolvesMissingReviewPhasesFromActiveProfile(t *testing.T) {
	for _, tt := range []struct {
		name    string
		profile modelconfig.ProfileName
		want    string
	}{
		{name: "cost saver", profile: modelconfig.ProfileCostSaver, want: "haiku"},
		{name: "quality", profile: modelconfig.ProfileQuality, want: "opus"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := installer.Config{ClaudeHome: t.TempDir()}
			partial := oldThirteenPhaseModelsForProfile(tt.profile)
			if err := installer.SaveModelsWithProfile(cfg, tt.profile, partial); err != nil {
				t.Fatalf("SaveModelsWithProfile(partial %s) error = %v", tt.profile, err)
			}

			line, err := formatModelsLine(cfg)
			if err != nil {
				t.Fatalf("formatModelsLine() error = %v", err)
			}

			for _, phase := range []modelconfig.Phase{
				modelconfig.PhaseReviewRisk,
				modelconfig.PhaseReviewReadability,
				modelconfig.PhaseReviewReliability,
				modelconfig.PhaseReviewResilience,
				modelconfig.PhaseReviewRefuter,
			} {
				want := string(phase) + "=" + tt.want
				if !strings.Contains(line, want) {
					t.Fatalf("formatModelsLine() = %q, want it to contain %q", line, want)
				}
			}
		})
	}
}

// TestDoctorCommand_ReportsConfiguredModels guards the doctor-output contract: it must report the
// configured per-phase models (or "defaults" pre-install) rather than staying silent about D25.
func TestDoctorCommand_ReportsConfiguredModels(t *testing.T) {
	seedResolvableEngram(t)
	seedResolvableClickBinary(t)
	home := t.TempDir()
	runner := newTestCommandRunner(home)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()

	if _, err := execRoot(t, home, "install"); err != nil {
		t.Fatalf("install command error = %v", err)
	}

	out, err := execRoot(t, home, "doctor")
	if err != nil {
		t.Fatalf("doctor command error = %v", err)
	}
	if !strings.Contains(out, "Modelos por fase de click-sdd") {
		t.Fatalf("doctor output = %q, want it to report configured models", out)
	}
	if !strings.Contains(out, "propose=opus") {
		t.Fatalf("doctor output = %q, want it to report propose=opus", out)
	}
}

// TestDoctorCommand_ReportsActiveProfile guards the "doctor reports active profile" spec
// requirement: the persisted orchestration profile name must appear in `click doctor`'s output.
func TestDoctorCommand_ReportsActiveProfile(t *testing.T) {
	seedResolvableEngram(t)
	seedResolvableClickBinary(t)
	home := t.TempDir()
	runner := newTestCommandRunner(home)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()

	if _, err := execRoot(t, home, "install", "--profile", "cost-saver"); err != nil {
		t.Fatalf("install command error = %v", err)
	}

	out, err := execRoot(t, home, "doctor")
	if err != nil {
		t.Fatalf("doctor command error = %v", err)
	}
	if !strings.Contains(out, "Perfil de orquestación: cost-saver") {
		t.Fatalf("doctor output = %q, want it to report the active profile cost-saver", out)
	}
}

// TestDoctorCommand_DoesNotMutateModelsJson guards NFR-012 / the read-only contract carried over
// from PR2: `click doctor`'s new profile-reporting line must never write to models.json.
func TestDoctorCommand_DoesNotMutateModelsJson(t *testing.T) {
	seedResolvableEngram(t)
	seedResolvableClickBinary(t)
	home := t.TempDir()
	runner := newTestCommandRunner(home)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()

	if _, err := execRoot(t, home, "install", "--profile", "quality"); err != nil {
		t.Fatalf("install command error = %v", err)
	}
	cfg := installer.Config{ClaudeHome: home}
	before, err := os.ReadFile(cfg.ModelsPath())
	if err != nil {
		t.Fatalf("ReadFile(models.json) error = %v", err)
	}

	if _, err := execRoot(t, home, "doctor"); err != nil {
		t.Fatalf("doctor command error = %v", err)
	}

	after, err := os.ReadFile(cfg.ModelsPath())
	if err != nil {
		t.Fatalf("ReadFile(models.json) error = %v", err)
	}
	if string(before) != string(after) {
		t.Fatalf("models.json changed after `click doctor`, want byte-for-byte unchanged (doctor must stay read-only)\nbefore: %s\nafter:  %s", before, after)
	}
}

func contains(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}

func oldThirteenPhaseModelsForProfile(profile modelconfig.ProfileName) map[modelconfig.Phase]string {
	models := modelconfig.ResolveForProfile(string(profile), nil)
	delete(models, modelconfig.PhaseReviewRisk)
	delete(models, modelconfig.PhaseReviewReadability)
	delete(models, modelconfig.PhaseReviewReliability)
	delete(models, modelconfig.PhaseReviewResilience)
	delete(models, modelconfig.PhaseReviewRefuter)
	return models
}

func TestRendererFor_NoColorFlagForcesPlain(t *testing.T) {
	root := NewRootCommand()
	root.SetArgs([]string{"install", "--no-color"})
	root.PersistentFlags().Parse([]string{"--no-color"})

	var buf bytes.Buffer
	r := rendererFor(root, &buf)
	if r.Color {
		t.Fatal("rendererFor() with --no-color parsed produced a color-enabled renderer")
	}
}
