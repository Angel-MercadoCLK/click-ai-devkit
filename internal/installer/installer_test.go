package installer

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/manifest"
)

func TestInstall_RegistersPluginsAndWritesManagedState(t *testing.T) {
	claudeHome := t.TempDir()
	cfg := Config{ClaudeHome: claudeHome}
	runner := newFakeCommandRunner(cfg)
	restoreRunner := SetCommandRunnerFactoryForTests(func() CommandRunner { return runner })
	defer restoreRunner()
	restoreSource := SetMarketplaceSourceForTests("https://github.com/Angel-MercadoCLK/click-ai-devkit")
	defer restoreSource()

	if err := Install(cfg, nil); err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	for _, plugin := range managedPlugins {
		ok, err := HasInstalledPlugin(cfg, plugin)
		if err != nil {
			t.Fatalf("HasInstalledPlugin(%q) error = %v", plugin, err)
		}
		if !ok {
			t.Fatalf("Install() did not register %s", plugin)
		}
	}

	if ok, err := HasManagedBlock(cfg.ClaudeMDPath()); err != nil {
		t.Fatalf("HasManagedBlock() error = %v", err)
	} else if !ok {
		t.Fatal("Install() did not write the managed CLAUDE.md block")
	}
	if registered, err := HasMemoryGuardHook(cfg); err != nil {
		t.Fatalf("HasMemoryGuardHook() error = %v", err)
	} else if !registered {
		t.Fatal("Install() did not register the memory-guard hook")
	}

	wantCommands := []string{
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
	if got := runner.commandStrings(); !reflect.DeepEqual(got, wantCommands) {
		t.Fatalf("command order = %#v, want %#v", got, wantCommands)
	}
}

func TestInstall_TwiceIsIdempotent(t *testing.T) {
	claudeHome := t.TempDir()
	cfg := Config{ClaudeHome: claudeHome}
	runner := newFakeCommandRunner(cfg)
	restoreRunner := SetCommandRunnerFactoryForTests(func() CommandRunner { return runner })
	defer restoreRunner()

	if err := Install(cfg, nil); err != nil {
		t.Fatalf("first Install() error = %v", err)
	}
	if err := Install(cfg, nil); err != nil {
		t.Fatalf("second Install() error = %v", err)
	}

	claudeMD, err := os.ReadFile(cfg.ClaudeMDPath())
	if err != nil {
		t.Fatalf("ReadFile(CLAUDE.md) error = %v", err)
	}
	if n := strings.Count(string(claudeMD), managedBeginMarker); n != 1 {
		t.Fatalf("CLAUDE.md has %d begin markers after two installs, want exactly 1", n)
	}
	for _, plugin := range managedPlugins {
		ok, err := HasInstalledPlugin(cfg, plugin)
		if err != nil {
			t.Fatalf("HasInstalledPlugin(%q) error = %v", plugin, err)
		}
		if !ok {
			t.Fatalf("Install() lost installed state for %s on second run", plugin)
		}
	}
}

func TestUninstall_ReversesInstall(t *testing.T) {
	claudeHome := t.TempDir()
	cfg := Config{ClaudeHome: claudeHome}
	runner := newFakeCommandRunner(cfg)
	restoreRunner := SetCommandRunnerFactoryForTests(func() CommandRunner { return runner })
	defer restoreRunner()

	if err := Install(cfg, nil); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if err := Uninstall(cfg); err != nil {
		t.Fatalf("Uninstall() error = %v", err)
	}

	for _, plugin := range managedPlugins {
		ok, err := HasInstalledPlugin(cfg, plugin)
		if err != nil {
			t.Fatalf("HasInstalledPlugin(%q) error = %v", plugin, err)
		}
		if ok {
			t.Fatalf("Uninstall() left %s registered", plugin)
		}
	}
	if ok, err := HasManagedBlock(cfg.ClaudeMDPath()); err != nil {
		t.Fatalf("HasManagedBlock() error = %v", err)
	} else if ok {
		t.Fatal("Uninstall() left the managed CLAUDE.md block behind")
	}
	if registered, err := HasMemoryGuardHook(cfg); err != nil {
		t.Fatalf("HasMemoryGuardHook() error = %v", err)
	} else if registered {
		t.Fatal("Uninstall() left the memory-guard hook behind")
	}
}

func TestUninstall_ReversesEngramMCPConfiguredByUpdate(t *testing.T) {
	claudeHome := t.TempDir()
	cfg := Config{ClaudeHome: claudeHome}
	binaryPath := filepath.Join(t.TempDir(), "engram.exe")
	if err := os.WriteFile(binaryPath, []byte("binary"), 0o644); err != nil {
		t.Fatalf("WriteFile(binary) error = %v", err)
	}
	t.Setenv(engramBinaryPathEnvOverride, binaryPath)
	runner := newFakeCommandRunner(cfg)
	restoreRunner := SetCommandRunnerFactoryForTests(func() CommandRunner { return runner })
	defer restoreRunner()

	if err := Install(cfg, nil); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	m, err := manifest.Load()
	if err != nil {
		t.Fatalf("manifest.Load() error = %v", err)
	}
	if err := ConfigureEngramMCP(cfg, m); err != nil {
		t.Fatalf("ConfigureEngramMCP() error = %v", err)
	}

	if err := Uninstall(cfg); err != nil {
		t.Fatalf("Uninstall() error = %v", err)
	}
	if _, err := os.Stat(cfg.EngramMCPConfigPath()); !os.IsNotExist(err) {
		t.Fatalf("Uninstall() left the Engram MCP config behind")
	}
	if _, err := os.Stat(cfg.EngramStatePath()); !os.IsNotExist(err) {
		t.Fatalf("Uninstall() left the Engram state file behind")
	}
}

func TestUninstall_NoopWhenAlreadyUninstalled(t *testing.T) {
	claudeHome := t.TempDir()
	cfg := Config{ClaudeHome: claudeHome}
	runner := newFakeCommandRunner(cfg)
	restoreRunner := SetCommandRunnerFactoryForTests(func() CommandRunner { return runner })
	defer restoreRunner()

	if err := Uninstall(cfg); err != nil {
		t.Fatalf("Uninstall() on a never-installed home error = %v, want nil", err)
	}
}

func TestInstallThenUninstallThenInstallAgain_Succeeds(t *testing.T) {
	claudeHome := t.TempDir()
	cfg := Config{ClaudeHome: claudeHome}
	runner := newFakeCommandRunner(cfg)
	restoreRunner := SetCommandRunnerFactoryForTests(func() CommandRunner { return runner })
	defer restoreRunner()

	if err := Install(cfg, nil); err != nil {
		t.Fatalf("first Install() error = %v", err)
	}
	if err := Uninstall(cfg); err != nil {
		t.Fatalf("Uninstall() error = %v", err)
	}
	if err := Install(cfg, nil); err != nil {
		t.Fatalf("re-Install() after uninstall error = %v", err)
	}

	for _, plugin := range managedPlugins {
		ok, err := HasInstalledPlugin(cfg, plugin)
		if err != nil {
			t.Fatalf("HasInstalledPlugin(%q) error = %v", plugin, err)
		}
		if !ok {
			t.Fatalf("re-Install() did not register %s", plugin)
		}
	}
	if ok, err := HasManagedBlock(cfg.ClaudeMDPath()); err != nil {
		t.Fatalf("HasManagedBlock() error = %v", err)
	} else if !ok {
		t.Fatal("re-Install() did not write the managed CLAUDE.md block")
	}
}
