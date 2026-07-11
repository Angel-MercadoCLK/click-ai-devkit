package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"

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
}

func execRoot(t *testing.T, claudeHome string, args ...string) (string, error) {
	t.Helper()
	t.Setenv("CLICK_CLAUDE_HOME", claudeHome)

	root := NewRootCommand()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs(args)

	err := root.Execute()
	return buf.String(), err
}

type testCommandRunner struct {
	home     string
	commands []string
	plugins  map[string]bool
}

func newTestCommandRunner(home string) *testCommandRunner {
	return &testCommandRunner{home: home, plugins: map[string]bool{}}
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

func (r *testCommandRunner) Output(name string, args ...string) ([]byte, error) {
	r.commands = append(r.commands, name+" "+strings.Join(args, " "))
	return []byte{}, nil
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
	settingsData, err := json.Marshal(map[string]any{"enabledPlugins": enabled})
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

	cancelledSelector := func(*cobra.Command) (map[modelconfig.Phase]string, bool, error) {
		return nil, true, nil
	}

	// nonInteractive=false forces the interactive branch regardless of the test's non-TTY buffer,
	// so the fake selector's cancel is what actually gets exercised.
	_, cancelled, err := resolveInstallModels(cmd, &buf, r, cfg, false, cancelledSelector)
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
		"claude plugin install click-sdd@click-ai-devkit" +
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
			" --config default_model=sonnet",
		"claude plugin install click-memory@click-ai-devkit",
		"claude plugin install click-review@click-ai-devkit",
	}
	if !reflect.DeepEqual(runner.commands[:4], want) {
		t.Fatalf("runner.commands[:4] = %#v, want %#v", runner.commands[:4], want)
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
// verbatim on the next update, not silently reset to defaults.
func TestUpdateCommand_ReappliesPersistedModels(t *testing.T) {
	home := t.TempDir()
	runner := newTestCommandRunner(home)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()

	if _, err := execRoot(t, home, "install"); err != nil {
		t.Fatalf("install command error = %v", err)
	}

	custom := map[modelconfig.Phase]string{
		modelconfig.PhaseExplore:    "haiku",
		modelconfig.PhasePropose:    "sonnet",
		modelconfig.PhaseSpec:       "haiku",
		modelconfig.PhaseDesign:     "sonnet",
		modelconfig.PhaseTasks:      "haiku",
		modelconfig.PhaseApply:      "sonnet",
		modelconfig.PhaseVerify:     "haiku",
		modelconfig.PhaseArchive:    "sonnet",
		modelconfig.PhaseOnboard:    "sonnet",
		modelconfig.PhaseJDJudgeA:   "haiku",
		modelconfig.PhaseJDJudgeB:   "haiku",
		modelconfig.PhaseJDFixAgent: "haiku",
		modelconfig.PhaseDefault:    "haiku",
	}
	if err := installer.SaveModels(installer.Config{ClaudeHome: home}, custom); err != nil {
		t.Fatalf("SaveModels() error = %v", err)
	}

	if _, err := execRoot(t, home, "update"); err != nil {
		t.Fatalf("update command error = %v", err)
	}

	wantCommand := "claude plugin install click-sdd@click-ai-devkit" +
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
		" --config default_model=sonnet"
	if !contains(runner.commands, wantCommand) {
		t.Fatalf("update command sequence = %#v, want it to contain fresh-defaults command %q", runner.commands, wantCommand)
	}
}

// TestDoctorCommand_ReportsConfiguredModels guards the doctor-output contract: it must report the
// configured per-phase models (or "defaults" pre-install) rather than staying silent about D25.
func TestDoctorCommand_ReportsConfiguredModels(t *testing.T) {
	seedResolvableEngram(t)
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

func contains(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
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
