package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/installer"
	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/modelconfig"
)

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
	default:
		return nil
	}
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

func TestDoctorCommand_AfterInstall_Succeeds(t *testing.T) {
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

	if _, err := os.Stat(filepath.Join(home, "mcp", "engram.json")); err != nil {
		t.Fatalf("update did not write engram MCP config: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, "click-ai-devkit", "engram.json")); err != nil {
		t.Fatalf("update did not write engram pinned state: %v", err)
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
			" --config orchestrator_model=opus" +
			" --config prd_writer_model=opus" +
			" --config architect_model=opus" +
			" --config reviewer_model=opus" +
			" --config memory_curator_model=sonnet",
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
		modelconfig.PhaseOrchestrator:  "haiku",
		modelconfig.PhasePRDWriter:     "sonnet",
		modelconfig.PhaseArchitect:     "haiku",
		modelconfig.PhaseReviewer:      "sonnet",
		modelconfig.PhaseMemoryCurator: "haiku",
	}
	if err := installer.SaveModels(installer.Config{ClaudeHome: home}, custom); err != nil {
		t.Fatalf("SaveModels() error = %v", err)
	}

	if _, err := execRoot(t, home, "update"); err != nil {
		t.Fatalf("update command error = %v", err)
	}

	wantCommand := "claude plugin install click-sdd@click-ai-devkit" +
		" --config orchestrator_model=haiku" +
		" --config prd_writer_model=sonnet" +
		" --config architect_model=haiku" +
		" --config reviewer_model=sonnet" +
		" --config memory_curator_model=haiku"
	if !contains(runner.commands, wantCommand) {
		t.Fatalf("update command sequence = %#v, want it to contain %q", runner.commands, wantCommand)
	}
}

// TestDoctorCommand_ReportsConfiguredModels guards the doctor-output contract: it must report the
// configured per-phase models (or "defaults" pre-install) rather than staying silent about D25.
func TestDoctorCommand_ReportsConfiguredModels(t *testing.T) {
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
	if !strings.Contains(out, "orchestrator=opus") {
		t.Fatalf("doctor output = %q, want it to report orchestrator=opus", out)
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
