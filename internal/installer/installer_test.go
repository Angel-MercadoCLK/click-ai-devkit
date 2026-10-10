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
	}
	if got := runner.commandStrings(); !reflect.DeepEqual(got, wantCommands) {
		t.Fatalf("command order = %#v, want %#v", got, wantCommands)
	}
}

func TestInstall_DoesNotOwnEngramOrContext7Registration(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir()}
	runner := newFakeCommandRunner(cfg)
	restoreRunner := SetCommandRunnerFactoryForTests(func() CommandRunner { return runner })
	defer restoreRunner()
	restoreSource := SetMarketplaceSourceForTests("https://github.com/Angel-MercadoCLK/click-ai-devkit")
	defer restoreSource()
	if err := Install(cfg, nil); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	for _, command := range runner.commandStrings() {
		if strings.Contains(command, "engram@engram") || strings.Contains(command, "mcp add") || strings.Contains(command, "go install") {
			t.Fatalf("Click Install() invoked shared Engram/Context7 lifecycle command %q", command)
		}
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

// TestUninstall_RemovesEngramCloudState proves the uninstall round-trip for the Engram Cloud
// enrollment record: Install() writes engram-cloud.json when a machine is enrolled, so Uninstall()
// must reverse that write like every other managed artifact (docs promise "uninstall should remove
// Click-managed configuration cleanly"). Uninstall must NOT attempt to un-enroll the shared cloud
// project — it only deletes click's own local bookkeeping file, fully offline.
func TestUninstall_RemovesEngramCloudState(t *testing.T) {
	claudeHome := t.TempDir()
	cfg := Config{ClaudeHome: claudeHome}
	seedResolvableEngram(t)
	runner := newFakeCommandRunner(cfg)
	restoreRunner := SetCommandRunnerFactoryForTests(func() CommandRunner { return runner })
	defer restoreRunner()

	if err := writeJSONFile(cfg.EngramCloudStatePath(), engramCloudState{Enrolled: true, Server: "http://127.0.0.1:18080", Project: "click-ai-devkit"}); err != nil {
		t.Fatalf("writeJSONFile(engram cloud state) error = %v", err)
	}

	if err := Uninstall(cfg); err != nil {
		t.Fatalf("Uninstall() error = %v", err)
	}

	if _, statErr := os.Stat(cfg.EngramCloudStatePath()); !os.IsNotExist(statErr) {
		t.Fatal("Uninstall() left the Engram Cloud enrollment state file behind")
	}
}

// TestUninstall_PreservesLegacyEngramOwnershipState keeps a prior Click Engram install intact now
// that the upstream component owns the shared registration and binary lifecycle.
func TestUninstall_PreservesLegacyEngramOwnershipState(t *testing.T) {
	claudeHome := t.TempDir()
	cfg := Config{ClaudeHome: claudeHome}
	runner := newFakeCommandRunner(cfg)
	restoreRunner := SetCommandRunnerFactoryForTests(func() CommandRunner { return runner })
	defer restoreRunner()
	seedEngramAlreadyInstalled(t, cfg)
	state := engramState{Version: "1.0.0", BinaryPath: filepath.Join(t.TempDir(), "engram"), InstalledByClick: true}
	if err := writeJSONFile(cfg.EngramStatePath(), state); err != nil {
		t.Fatal(err)
	}
	ok, err := HasInstalledPluginID(cfg, EngramPluginID)
	if err != nil {
		t.Fatalf("HasInstalledPluginID() error = %v", err)
	}
	if !ok {
		t.Fatal("fixture did not register engram@engram")
	}

	if err := Uninstall(cfg); err != nil {
		t.Fatalf("Uninstall() error = %v", err)
	}
	if ok, err := HasInstalledPluginID(cfg, EngramPluginID); err != nil {
		t.Fatalf("HasInstalledPluginID() error = %v", err)
	} else if !ok {
		t.Fatal("Uninstall() removed shared engram@engram registration")
	}
	if got, found, err := loadEngramState(cfg); err != nil || !found || !reflect.DeepEqual(got, state) {
		t.Fatalf("Engram state = (%+v, %t), err = %v, want legacy ownership metadata preserved", got, found, err)
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

// TestInstall_DoesNotRunEngramCloudLifecycle keeps shared Engram enrollment in the CLI layer after
// gentle-ai has installed or reconciled the executable.
func TestInstall_DoesNotRunEngramCloudLifecycle(t *testing.T) {
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

	for _, cmd := range runner.commandStrings() {
		if strings.Contains(cmd, "engram cloud") || strings.Contains(cmd, "engram@engram") {
			t.Fatalf("Click installer invoked upstream Engram lifecycle command %q", cmd)
		}
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
