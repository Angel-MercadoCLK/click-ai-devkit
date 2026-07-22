package installer

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// seedResolvableEngram makes EnsureEngramBinary see the Engram binary as already resolvable, so the
// full-install composition tests are deterministic REGARDLESS of the machine's PATH. Without this,
// EnsureEngramBinary issues a `go install` (recorded by the fake runner) on any host that has `go`
// but not `engram` on PATH — e.g. CI — making the command-order assertions pass locally (dev has
// engram) but fail in CI. It points CLICK_ENGRAM_BINARY_PATH at a real stub file because
// EngramBinaryResolvable os.Stat()s the path.
func seedResolvableEngram(t *testing.T) {
	t.Helper()
	bin := filepath.Join(t.TempDir(), engramBinaryName())
	if err := os.WriteFile(bin, []byte("stub"), 0o755); err != nil {
		t.Fatalf("seed engram binary: %v", err)
	}
	t.Setenv(engramBinaryPathEnvOverride, bin)
}

func TestInstall_RegistersPluginsAndWritesManagedState(t *testing.T) {
	claudeHome := t.TempDir()
	cfg := Config{ClaudeHome: claudeHome}
	seedResolvableEngram(t)
	runner := newFakeCommandRunner(cfg)
	restoreRunner := SetCommandRunnerFactoryForTests(func() CommandRunner { return runner })
	defer restoreRunner()
	restoreSource := SetMarketplaceSourceForTests("https://github.com/Angel-MercadoCLK/click-ai-devkit")
	defer restoreSource()
	restoreEngramSource := SetEngramMarketplaceSourceForTests("https://github.com/Gentleman-Programming/engram")
	defer restoreEngramSource()

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
	if ok, err := HasInstalledPluginID(cfg, EngramPluginID); err != nil {
		t.Fatalf("HasInstalledPluginID(EngramPluginID) error = %v", err)
	} else if !ok {
		t.Fatal("Install() did not register engram@engram")
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
		"claude plugin marketplace update " + marketplaceName,
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
		"claude plugin marketplace add https://github.com/Gentleman-Programming/engram",
		"claude plugin install engram@engram",
		// EnsureEngramBinary's Phase 4 signal-wiring (D-5) always queries GoBinDir once the binary
		// is confirmed resolvable, to decide whether the resolved binary lives inside it and PATH
		// persistence should be attempted. seedResolvableEngram points the binary at a temp dir
		// outside any real GOBIN/GOPATH, and this fake runner never stubs "go env GOBIN"/"go env
		// GOPATH", so GoBinDir errors and no pathStore call follows — but the two `go env` lookups
		// themselves are still genuinely issued.
		"go env GOBIN",
		"go env GOPATH",
		"claude mcp add --transport http --scope user context7 https://mcp.context7.com/mcp",
	}
	if got := runner.commandStrings(); !reflect.DeepEqual(got, wantCommands) {
		t.Fatalf("command order = %#v, want %#v", got, wantCommands)
	}
}

func TestInstall_TwiceIsIdempotent(t *testing.T) {
	claudeHome := t.TempDir()
	cfg := Config{ClaudeHome: claudeHome}
	seedResolvableEngram(t)
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
	seedResolvableEngram(t)
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

// TestUninstall_ReversesEngramWhenClickInstalledIt covers the normal case: click's own Install()
// registered Engram (nothing pre-existing), so Uninstall must fully reverse it, including click's
// own state bookkeeping file.
func TestUninstall_ReversesEngramWhenClickInstalledIt(t *testing.T) {
	claudeHome := t.TempDir()
	cfg := Config{ClaudeHome: claudeHome}
	seedResolvableEngram(t)
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

	ok, err := HasInstalledPluginID(cfg, EngramPluginID)
	if err != nil {
		t.Fatalf("HasInstalledPluginID() error = %v", err)
	}
	if !ok {
		t.Fatal("Install() did not register engram@engram")
	}

	if err := Uninstall(cfg); err != nil {
		t.Fatalf("Uninstall() error = %v", err)
	}
	if ok, err := HasInstalledPluginID(cfg, EngramPluginID); err != nil {
		t.Fatalf("HasInstalledPluginID() error = %v", err)
	} else if ok {
		t.Fatal("Uninstall() left engram@engram registered after click's own Install() added it")
	}
	if _, err := os.Stat(cfg.EngramStatePath()); !os.IsNotExist(err) {
		t.Fatalf("Uninstall() left the Engram state file behind")
	}
}

// TestUninstall_RespectsEngramInstalledBeforeClick guards the "many devs already have Engram
// working" contract at the full Install/Uninstall level: if Engram was already installed before
// click's Install() ran, Uninstall must leave it registered and running.
func TestUninstall_RespectsEngramInstalledBeforeClick(t *testing.T) {
	claudeHome := t.TempDir()
	cfg := Config{ClaudeHome: claudeHome}
	seedResolvableEngram(t)
	runner := newFakeCommandRunner(cfg)
	restoreRunner := SetCommandRunnerFactoryForTests(func() CommandRunner { return runner })
	defer restoreRunner()

	seedEngramAlreadyInstalled(t, cfg)

	if err := Install(cfg, nil); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if err := Uninstall(cfg); err != nil {
		t.Fatalf("Uninstall() error = %v", err)
	}

	ok, err := HasInstalledPluginID(cfg, EngramPluginID)
	if err != nil {
		t.Fatalf("HasInstalledPluginID() error = %v", err)
	}
	if !ok {
		t.Fatal("Uninstall() removed a pre-existing Engram install click never owned")
	}
}

func TestUninstall_NoopWhenAlreadyUninstalled(t *testing.T) {
	claudeHome := t.TempDir()
	cfg := Config{ClaudeHome: claudeHome}
	seedResolvableEngram(t)
	runner := newFakeCommandRunner(cfg)
	restoreRunner := SetCommandRunnerFactoryForTests(func() CommandRunner { return runner })
	defer restoreRunner()

	if err := Uninstall(cfg); err != nil {
		t.Fatalf("Uninstall() on a never-installed home error = %v, want nil", err)
	}
}

// TestInstall_SyncEngramCloud_AfterSyncEngram is task 4.1's RED test: Install() must compose
// SyncEngramCloud immediately after SyncEngram when cloud config and token are present, preserving
// local-only behavior otherwise.
func TestInstall_SyncEngramCloud_AfterSyncEngram(t *testing.T) {
	claudeHome := t.TempDir()
	cfg := Config{ClaudeHome: claudeHome}
	seedResolvableEngram(t)
	runner := newFakeCommandRunner(cfg)
	restoreRunner := SetCommandRunnerFactoryForTests(func() CommandRunner { return runner })
	defer restoreRunner()

	t.Setenv("ENGRAM_CLOUD_TOKEN", "cloud-token")
	t.Setenv("CLICK_ENGRAM_CLOUD_SERVER", "http://127.0.0.1:18080")
	t.Setenv("CLICK_ENGRAM_CLOUD_PROJECT", "click-ai-devkit")

	if err := Install(cfg, nil); err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	var engramPluginIdx, cloudConfigIdx int
	foundPlugin, foundCloud := false, false
	for i, cmd := range runner.commandStrings() {
		if strings.HasPrefix(cmd, "claude plugin install engram@engram") {
			engramPluginIdx = i
			foundPlugin = true
		}
		if cmd == "engram cloud config --server http://127.0.0.1:18080" {
			cloudConfigIdx = i
			foundCloud = true
			break
		}
	}
	if !foundPlugin {
		t.Fatal("Install() did not register engram@engram")
	}
	if !foundCloud {
		t.Fatal("Install() did not issue engram cloud config when cloud config+token present")
	}
	if cloudConfigIdx <= engramPluginIdx {
		t.Fatalf("cloud config index %d must be after engram plugin install index %d", cloudConfigIdx, engramPluginIdx)
	}
}

// TestInstall_SyncEngramCloud_NoConfig_ZeroCloudCalls is task 4.1's no-config backward-compat test:
// when no cloud server/project/token are present, Install() must add zero engram cloud commands.
func TestInstall_SyncEngramCloud_NoConfig_ZeroCloudCalls(t *testing.T) {
	claudeHome := t.TempDir()
	cfg := Config{ClaudeHome: claudeHome}
	seedResolvableEngram(t)
	runner := newFakeCommandRunner(cfg)
	restoreRunner := SetCommandRunnerFactoryForTests(func() CommandRunner { return runner })
	defer restoreRunner()

	if err := Install(cfg, nil); err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	for _, cmd := range runner.commandStrings() {
		if strings.HasPrefix(cmd, "engram cloud") || (strings.HasPrefix(cmd, "engram ") && strings.Contains(cmd, "--cloud")) {
			t.Fatalf("Install() issued cloud command without cloud config: %q", cmd)
		}
	}
}

func TestInstallThenUninstallThenInstallAgain_Succeeds(t *testing.T) {
	claudeHome := t.TempDir()
	cfg := Config{ClaudeHome: claudeHome}
	seedResolvableEngram(t)
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
