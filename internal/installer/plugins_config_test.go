package installer

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/modelconfig"
)

type failingMarketplaceRunner struct {
	failAt int
	calls  int
	err    error
}

func (r *failingMarketplaceRunner) Run(name string, args ...string) error {
	r.calls++
	if r.calls == r.failAt {
		return r.err
	}
	return nil
}

func (r *failingMarketplaceRunner) Output(name string, args ...string) ([]byte, error) {
	return []byte("[]"), nil
}

type inventoryMarketplaceRunner struct {
	listOutput []byte
	calls      []commandInvocation
	err        error
	failAt     int
	runCount   int
}

func (r *inventoryMarketplaceRunner) Output(name string, args ...string) ([]byte, error) {
	r.calls = append(r.calls, commandInvocation{Name: name, Args: append([]string(nil), args...)})
	return append([]byte(nil), r.listOutput...), nil
}

func (r *inventoryMarketplaceRunner) Run(name string, args ...string) error {
	r.calls = append(r.calls, commandInvocation{Name: name, Args: append([]string(nil), args...)})
	r.runCount++
	if r.failAt == 0 || r.runCount == r.failAt {
		return r.err
	}
	return nil
}

func TestRemoveMarketplacePlugins_RemovesOnlyExactUserInstallationsAndKeepsData(t *testing.T) {
	home, state := t.TempDir(), t.TempDir()
	runner := &inventoryMarketplaceRunner{listOutput: []byte(`[
		{"id":"click-memory@click-ai-devkit","scope":"project","projectPath":"/project"},
		{"id":"click-review@click-ai-devkit","scope":"user"},
		{"id":"click-skills@click-ai-devkit","scope":"user"},
		{"id":"click-skills@click-ai-devkit","scope":"synced"}
	]`)}
	restore := SetCommandRunnerFactoryForTests(func() CommandRunner { return runner })
	defer restore()
	if err := RemoveMarketplacePlugins(Config{ClaudeHome: home, ClickStateHome: state}); err != nil {
		t.Fatalf("RemoveMarketplacePlugins() error = %v", err)
	}

	var uninstallCalls []commandInvocation
	for _, call := range runner.calls {
		if len(call.Args) > 1 && call.Args[0] == "plugin" && call.Args[1] == "uninstall" {
			uninstallCalls = append(uninstallCalls, call)
		}
		if len(call.Args) >= 3 && call.Args[0] == "plugin" && call.Args[1] == "marketplace" && call.Args[2] == "remove" {
			t.Fatalf("marketplace removal can cascade across scopes: %#v", call.Args)
		}
	}
	want := [][]string{
		{"plugin", "uninstall", "click-review@click-ai-devkit", "--scope", "user", "--keep-data"},
		{"plugin", "uninstall", "click-skills@click-ai-devkit", "--scope", "user", "--keep-data"},
	}
	if len(uninstallCalls) != len(want) {
		t.Fatalf("uninstall calls = %#v, want only user-scoped review/skills (foreign-only memory and absent SDD skipped)", uninstallCalls)
	}
	for i, expected := range want {
		if !reflect.DeepEqual(uninstallCalls[i].Args, expected) {
			t.Errorf("uninstall call %d = %#v, want %#v", i, uninstallCalls[i].Args, expected)
		}
	}
}

func TestRemoveMarketplacePlugins_ScopesEveryUserPluginRemovalAndKeepsData(t *testing.T) {
	runner := &inventoryMarketplaceRunner{listOutput: []byte(`[
		{"id":"click-memory@click-ai-devkit","scope":"user"},
		{"id":"click-review@click-ai-devkit","scope":"user"},
		{"id":"click-skills@click-ai-devkit","scope":"user"}
	]`)}
	restore := SetCommandRunnerFactoryForTests(func() CommandRunner { return runner })
	defer restore()

	if err := RemoveMarketplacePlugins(Config{ClaudeHome: t.TempDir(), ClickStateHome: t.TempDir()}); err != nil {
		t.Fatalf("RemoveMarketplacePlugins() error = %v", err)
	}
	want := map[string]bool{
		"click-memory": false,
		"click-review": false,
		"click-skills": false,
	}
	for _, call := range runner.calls {
		if len(call.Args) < 3 || call.Args[0] != "plugin" || call.Args[1] != "uninstall" {
			continue
		}
		plugin := strings.TrimSuffix(strings.TrimPrefix(call.Args[2], "click-"), "@click-ai-devkit")
		name := "click-" + plugin
		if _, ok := want[name]; !ok {
			t.Fatalf("unexpected plugin removal: %#v", call.Args)
		}
		if !reflect.DeepEqual(call.Args, []string{"plugin", "uninstall", call.Args[2], "--scope", "user", "--keep-data"}) {
			t.Errorf("%s removal args = %#v, want user scope and --keep-data", name, call.Args)
		}
		want[name] = true
	}
	for plugin, removed := range want {
		if !removed {
			t.Errorf("%s was not removed, want exact user installation removed", plugin)
		}
	}
}

func TestRemoveMarketplacePlugins_RejectsAmbiguousUserEntriesBeforeRemovingAny(t *testing.T) {
	runner := &inventoryMarketplaceRunner{listOutput: []byte(`[
		{"id":"click-review@click-ai-devkit","scope":"user"},
		{"id":"click-review@click-ai-devkit","scope":"user"},
		{"id":"click-skills@click-ai-devkit","scope":"user"}
	]`)}
	restore := SetCommandRunnerFactoryForTests(func() CommandRunner { return runner })
	defer restore()

	err := RemoveMarketplacePlugins(Config{ClaudeHome: t.TempDir(), ClickStateHome: t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "ambigu") {
		t.Fatalf("RemoveMarketplacePlugins() error = %v, want safe ambiguity error", err)
	}
	for _, call := range runner.calls {
		if len(call.Args) > 1 && call.Args[0] == "plugin" && call.Args[1] == "uninstall" {
			t.Fatalf("ambiguous inventory caused a partial uninstall: %#v", runner.calls)
		}
	}
}

func TestRemoveMarketplacePlugins_SanitizesRunnerOutputAndPreservesCause(t *testing.T) {
	secretOutput := errors.New("plugin failed with private configuration token-123")
	runner := &inventoryMarketplaceRunner{
		listOutput: []byte(`[{"id":"click-review@click-ai-devkit","scope":"user"}]`),
		err:        secretOutput,
	}
	restore := SetCommandRunnerFactoryForTests(func() CommandRunner { return runner })
	defer restore()

	err := RemoveMarketplacePlugins(Config{ClaudeHome: t.TempDir(), ClickStateHome: t.TempDir()})
	if !errors.Is(err, secretOutput) {
		t.Fatalf("RemoveMarketplacePlugins() error = %v, want wrapped cause", err)
	}
	if strings.Contains(err.Error(), "private configuration token-123") {
		t.Fatalf("RemoveMarketplacePlugins() leaked runner output: %v", err)
	}
}

func TestRemoveMarketplacePlugins_SecondFailureDoesNotClaimPreviousRemovalWasUndone(t *testing.T) {
	runnerCause := errors.New("private runner detail")
	runner := &inventoryMarketplaceRunner{
		listOutput: []byte(`[
			{"id":"click-memory@click-ai-devkit","scope":"user"},
			{"id":"click-review@click-ai-devkit","scope":"user"}
		]`),
		err:    runnerCause,
		failAt: 2,
	}
	restore := SetCommandRunnerFactoryForTests(func() CommandRunner { return runner })
	defer restore()

	err := RemoveMarketplacePlugins(Config{ClaudeHome: t.TempDir(), ClickStateHome: t.TempDir()})
	if !errors.Is(err, runnerCause) {
		t.Fatalf("RemoveMarketplacePlugins() error = %v, want wrapped runner cause", err)
	}
	if strings.Contains(err.Error(), "no se modificaron otros plugins de Click") {
		t.Fatalf("error falsely implied prior removals were undone: %v", err)
	}
	var removals []commandInvocation
	for _, call := range runner.calls {
		if len(call.Args) > 1 && call.Args[0] == "plugin" && call.Args[1] == "uninstall" {
			removals = append(removals, call)
		}
	}
	if len(removals) != 2 || removals[0].Args[2] != "click-memory@click-ai-devkit" || removals[1].Args[2] != "click-review@click-ai-devkit" {
		t.Fatalf("uninstall attempts = %#v, want memory completed before review failure", removals)
	}
}

func TestSyncMarketplacePlugins_DiagnosesCommandFailures(t *testing.T) {
	tests := []struct {
		name      string
		failAt    int
		rawError  error
		wantKind  PluginSyncFailureKind
		wantStage string
		wantText  string
	}{
		{
			name:      "marketplace add clone failure",
			failAt:    1,
			rawError:  errors.New("git clone failed: repository not found"),
			wantKind:  PluginSyncMarketplaceFailure,
			wantStage: "marketplace add/clone",
			wantText:  "repository not found",
		},
		{
			name:      "marketplace update failure",
			failAt:    2,
			rawError:  errors.New("marketplace update failed: network unavailable"),
			wantKind:  PluginSyncMarketplaceFailure,
			wantStage: "marketplace update",
			wantText:  "network unavailable",
		},
		{
			name:      "plugin install failure",
			failAt:    3,
			rawError:  errors.New("plugin install failed: manifest invalid"),
			wantKind:  PluginSyncPluginInstallFailure,
			wantStage: "plugin install",
			wantText:  "manifest invalid",
		},
		{
			name:      "unsafe executable or cwd",
			failAt:    2,
			rawError:  errors.New("Command git not found or is in an unsafe location (current directory)"),
			wantKind:  PluginSyncUnsafeLocation,
			wantStage: "marketplace update",
			wantText:  "unsafe location",
		},
		{
			name:      "missing executable",
			failAt:    1,
			rawError:  errors.New(`exec: "claude": executable file not found in %PATH%`),
			wantKind:  PluginSyncMissingExecutable,
			wantStage: "marketplace add/clone",
			wantText:  "executable file not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := &failingMarketplaceRunner{failAt: tt.failAt, err: tt.rawError}
			restoreRunner := SetCommandRunnerFactoryForTests(func() CommandRunner { return runner })
			defer restoreRunner()
			restoreSource := SetMarketplaceSourceForTests("https://example.invalid/click-ai-devkit")
			defer restoreSource()

			err := SyncMarketplacePlugins(Config{ClaudeHome: t.TempDir(), ClickStateHome: t.TempDir()}, nil)
			if err == nil {
				t.Fatal("SyncMarketplacePlugins() error = nil, want diagnostic failure")
			}
			var diagnostic *PluginSyncError
			if !errors.As(err, &diagnostic) {
				t.Fatalf("SyncMarketplacePlugins() error = %v, want *PluginSyncError", err)
			}
			if diagnostic.Kind != tt.wantKind {
				t.Fatalf("diagnostic.Kind = %q, want %q", diagnostic.Kind, tt.wantKind)
			}
			if diagnostic.Stage != tt.wantStage {
				t.Fatalf("diagnostic.Stage = %q, want %q", diagnostic.Stage, tt.wantStage)
			}
			if !errors.Is(err, tt.rawError) {
				t.Fatalf("diagnostic error does not preserve wrapped cause: %v", err)
			}
			if !strings.Contains(err.Error(), tt.wantText) {
				t.Fatalf("diagnostic error = %q, want it to contain %q", err, tt.wantText)
			}
		})
	}
}

func TestPluginSyncError_PreservesCauseAndOperationContext(t *testing.T) {
	cause := fmt.Errorf("status 1")
	err := newPluginSyncError(PluginSyncPluginInstallFailure, "plugin install", cause)
	if !errors.Is(err, cause) {
		t.Fatal("newPluginSyncError() did not preserve the original cause")
	}
	if !strings.Contains(err.Error(), "plugin install") || !strings.Contains(err.Error(), "status 1") {
		t.Fatalf("newPluginSyncError() = %q, want operation and cause", err)
	}
}

// TestSyncMarketplacePlugins_DoesNotAssignClickSDDModels verifies that plugin registration now
// delegates model selection to Claude Code rather than writing retired profile/phase config.
func TestSyncMarketplacePlugins_DoesNotAssignClickSDDModels(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir()}
	runner := newFakeCommandRunner(cfg)
	restoreRunner := SetCommandRunnerFactoryForTests(func() CommandRunner { return runner })
	defer restoreRunner()
	restoreSource := SetMarketplaceSourceForTests("https://github.com/Angel-MercadoCLK/click-ai-devkit")
	defer restoreSource()

	models := map[modelconfig.Phase]string{
		modelconfig.PhaseExplore:           "sonnet",
		modelconfig.PhasePropose:           "opus",
		modelconfig.PhaseSpec:              "sonnet",
		modelconfig.PhaseDesign:            "opus",
		modelconfig.PhaseTasks:             "sonnet",
		modelconfig.PhaseApply:             "haiku",
		modelconfig.PhaseVerify:            "opus",
		modelconfig.PhaseArchive:           "haiku",
		modelconfig.PhaseOnboard:           "haiku",
		modelconfig.PhaseJDJudgeA:          "sonnet",
		modelconfig.PhaseJDJudgeB:          "sonnet",
		modelconfig.PhaseJDFixAgent:        "sonnet",
		modelconfig.PhaseReviewRisk:        "opus",
		modelconfig.PhaseReviewReadability: "sonnet",
		modelconfig.PhaseReviewReliability: "haiku",
		modelconfig.PhaseReviewResilience:  "sonnet",
		modelconfig.PhaseReviewRefuter:     "opus",
		modelconfig.PhaseDefault:           "sonnet",
	}

	if err := SyncMarketplacePlugins(cfg, models, modelconfig.ProfileCostSaver); err != nil {
		t.Fatalf("SyncMarketplacePlugins() error = %v", err)
	}

	want := []commandInvocation{
		{Name: "claude", Args: []string{"plugin", "list", "--json"}},
		{Name: "claude", Args: []string{
			"plugin", "marketplace", "add", "https://github.com/Angel-MercadoCLK/click-ai-devkit",
			"--sparse", ".claude-plugin", "plugins",
		}},
		{Name: "claude", Args: []string{
			"plugin", "marketplace", "update", marketplaceName,
		}},
		{Name: "claude", Args: []string{"plugin", "install", "click-memory@click-ai-devkit"}},
		{Name: "claude", Args: []string{"plugin", "install", "click-review@click-ai-devkit"}},
		{Name: "claude", Args: []string{"plugin", "install", "click-skills@click-ai-devkit"}},
	}
	if !reflect.DeepEqual(runner.commands, want) {
		t.Fatalf("runner.commands = %#v, want %#v", runner.commands, want)
	}
}

func TestSyncMarketplacePlugins_RetiresLegacySDDBeforeMarketplaceAndInstallsRemainingPlugins(t *testing.T) {
	claudeHome, stateHome := t.TempDir(), t.TempDir()
	cfg := Config{ClaudeHome: claudeHome, ClickStateHome: stateHome}
	settings := `{"pluginConfigs":{"click-sdd@click-ai-devkit":{"options":{"legacy":"value"}}}}`
	if err := os.WriteFile(cfg.SettingsPath(), []byte(settings), 0o600); err != nil {
		t.Fatal(err)
	}
	runner := &sddRetirementRunner{listOutput: []byte(`[{"id":"click-sdd@click-ai-devkit","scope":"user"}]`)}
	restoreRunner := SetCommandRunnerFactoryForTests(func() CommandRunner { return runner })
	defer restoreRunner()
	restoreSource := SetMarketplaceSourceForTests("https://example.invalid/click-ai-devkit")
	defer restoreSource()

	if err := SyncMarketplacePlugins(cfg, nil); err != nil {
		t.Fatalf("SyncMarketplacePlugins() error = %v", err)
	}
	want := []commandInvocation{
		{Name: "claude", Args: []string{"plugin", "list", "--json"}},
		{Name: "claude", Args: []string{"plugin", "uninstall", ClickSDDPluginID, "--scope", "user", "--keep-data"}},
		{Name: "claude", Args: []string{"plugin", "marketplace", "add", "https://example.invalid/click-ai-devkit", "--sparse", ".claude-plugin", "plugins"}},
		{Name: "claude", Args: []string{"plugin", "marketplace", "update", marketplaceName}},
		{Name: "claude", Args: []string{"plugin", "install", "click-memory@click-ai-devkit"}},
		{Name: "claude", Args: []string{"plugin", "install", "click-review@click-ai-devkit"}},
		{Name: "claude", Args: []string{"plugin", "install", "click-skills@click-ai-devkit"}},
	}
	if !reflect.DeepEqual(runner.commands, want) {
		t.Fatalf("runner.commands = %#v, want %#v", runner.commands, want)
	}
	backups, err := filepath.Glob(filepath.Join(stateHome, clickSDDRetirementDir, "click-sdd-*.json"))
	if err != nil || len(backups) != 1 {
		t.Fatalf("retirement backups = %#v, err = %v; want exactly one durable backup", backups, err)
	}
}

func TestSyncMarketplacePlugins_RetirementFailureStopsBeforeMarketplaceMutation(t *testing.T) {
	claudeHome, stateParent := t.TempDir(), t.TempDir()
	stateHome := filepath.Join(stateParent, "state-file")
	if err := os.WriteFile(stateHome, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := Config{ClaudeHome: claudeHome, ClickStateHome: stateHome}
	if err := os.WriteFile(cfg.SettingsPath(), []byte(`{"pluginConfigs":{"click-sdd@click-ai-devkit":{"options":{"legacy":"value"}}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	runner := &sddRetirementRunner{listOutput: []byte(`[{"id":"click-sdd@click-ai-devkit","scope":"user"}]`)}
	restoreRunner := SetCommandRunnerFactoryForTests(func() CommandRunner { return runner })
	defer restoreRunner()
	restoreSource := SetMarketplaceSourceForTests("https://example.invalid/click-ai-devkit")
	defer restoreSource()

	if err := SyncMarketplacePlugins(cfg, nil); err == nil {
		t.Fatal("SyncMarketplacePlugins() error = nil; want retirement backup failure")
	}
	if len(runner.commands) != 1 || !reflect.DeepEqual(runner.commands[0].Args, []string{"plugin", "list", "--json"}) {
		t.Fatalf("commands before retirement failure = %#v; want only read-only inventory", runner.commands)
	}
}

func TestRemoveMarketplacePlugins_RetiresOnlyUserSDDAndNeverRemovesMarketplace(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir(), ClickStateHome: t.TempDir()}
	runner := &sddRetirementRunner{listOutput: []byte(`[
		{"id":"click-sdd@click-ai-devkit","scope":"project","projectPath":"/project"},
		{"id":"click-sdd@click-ai-devkit","scope":"user"},
		{"id":"click-memory@click-ai-devkit","scope":"project","projectPath":"/project"},
		{"id":"click-review@click-ai-devkit","scope":"user"},
		{"id":"click-skills@click-ai-devkit","scope":"user"}
	]`)}
	restoreRunner := SetCommandRunnerFactoryForTests(func() CommandRunner { return runner })
	defer restoreRunner()

	if err := RemoveMarketplacePlugins(cfg); err != nil {
		t.Fatalf("RemoveMarketplacePlugins() error = %v", err)
	}
	want := []commandInvocation{
		{Name: "claude", Args: []string{"plugin", "list", "--json"}},
		{Name: "claude", Args: []string{"plugin", "uninstall", ClickSDDPluginID, "--scope", "user", "--keep-data"}},
		{Name: "claude", Args: []string{"plugin", "list", "--json"}},
		{Name: "claude", Args: []string{"plugin", "uninstall", "click-review@click-ai-devkit", "--scope", "user", "--keep-data"}},
		{Name: "claude", Args: []string{"plugin", "uninstall", "click-skills@click-ai-devkit", "--scope", "user", "--keep-data"}},
	}
	if !reflect.DeepEqual(runner.commands, want) {
		t.Fatalf("runner.commands = %#v, want scoped cleanup without marketplace removal %#v", runner.commands, want)
	}
}

// TestSyncMarketplacePlugins_RefreshesMarketplaceBeforeInstalling guards the stale-schema-cache
// bug (fix/marketplace-refresh-stale-schema), root-caused live: click-sdd's plugin.json version
// never bumps past 0.1.0 across content/schema changes, so `claude plugin install` treats it as
// "already installed, nothing to check" and validates `--config key=value` flags against a STALE
// CACHED COPY of the plugin's userConfig schema — reproduced live as:
//
//	⚠ Installed, but --config not applied: --config key "review_risk_model" isn't declared in
//	this plugin's userConfig.
//
// A live fix was verified: running `claude plugin marketplace update <name>` (forcing Claude Code
// to refresh its cached plugin.json/schema from the git source) BEFORE reinstalling makes the
// --config flags apply cleanly. The stale-cache gap lives at the marketplace level — the existing
// "already on disk, declared in user settings" no-op path never refreshed anything — so
// SyncMarketplacePlugins must issue a genuine refresh unconditionally, on every sync, strictly
// BEFORE any `plugin install` call (a refresh issued after install wouldn't fix anything).
func TestSyncMarketplacePlugins_RefreshesMarketplaceBeforeInstalling(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir()}
	runner := newFakeCommandRunner(cfg)
	restoreRunner := SetCommandRunnerFactoryForTests(func() CommandRunner { return runner })
	defer restoreRunner()
	restoreSource := SetMarketplaceSourceForTests("https://github.com/Angel-MercadoCLK/click-ai-devkit")
	defer restoreSource()

	if err := SyncMarketplacePlugins(cfg, nil); err != nil {
		t.Fatalf("SyncMarketplacePlugins(nil) error = %v", err)
	}

	updateIdx, installIdx := -1, -1
	for i, cmd := range runner.commands {
		if cmd.Name != "claude" {
			continue
		}
		if len(cmd.Args) == 4 && cmd.Args[0] == "plugin" && cmd.Args[1] == "marketplace" && cmd.Args[2] == "update" {
			updateIdx = i
			if cmd.Args[3] != marketplaceName {
				t.Fatalf("marketplace update arg = %q, want %q", cmd.Args[3], marketplaceName)
			}
		}
		if installIdx == -1 && len(cmd.Args) >= 3 && cmd.Args[0] == "plugin" && cmd.Args[1] == "install" {
			installIdx = i
		}
	}
	if updateIdx == -1 {
		t.Fatalf("SyncMarketplacePlugins never invoked `claude plugin marketplace update %s`; commands = %#v", marketplaceName, runner.commandStrings())
	}
	if installIdx == -1 {
		t.Fatalf("no plugin install command found; commands = %#v", runner.commandStrings())
	}
	if updateIdx >= installIdx {
		t.Fatalf("marketplace update must run BEFORE the first plugin install; update at %d, first install at %d; commands = %#v", updateIdx, installIdx, runner.commandStrings())
	}
}

// TestSyncMarketplacePlugins_UsesNativeModelSelection confirms no Click model defaults are passed.
func TestSyncMarketplacePlugins_UsesNativeModelSelection(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir()}
	runner := newFakeCommandRunner(cfg)
	restoreRunner := SetCommandRunnerFactoryForTests(func() CommandRunner { return runner })
	defer restoreRunner()
	restoreSource := SetMarketplaceSourceForTests("https://github.com/Angel-MercadoCLK/click-ai-devkit")
	defer restoreSource()

	if err := SyncMarketplacePlugins(cfg, nil); err != nil {
		t.Fatalf("SyncMarketplacePlugins(nil) error = %v", err)
	}

	for _, cmd := range runner.commands {
		if len(cmd.Args) >= 3 && cmd.Args[0] == "plugin" && cmd.Args[1] == "install" && cmd.Args[2] == "click-sdd@click-ai-devkit" {
			t.Fatalf("SyncMarketplacePlugins() installed retired click-sdd: %#v", cmd)
		}
	}
}
