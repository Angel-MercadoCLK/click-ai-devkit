package installer

import (
	"reflect"
	"testing"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/modelconfig"
)

// TestSyncMarketplacePlugins_PassesPerPhaseConfigFlagsForClickSDD guards D25's runtime wiring:
// installing click-sdd must carry one `--config <phase>_model=<alias>` pair per phase, in
// modelconfig.Phases order, using the exact `--config key=value` repeated-flag syntax verified
// against the real `claude` CLI in Step 0. click-memory and click-review must NOT receive any
// --config flags — they have no userConfig schema.
func TestSyncMarketplacePlugins_PassesPerPhaseConfigFlagsForClickSDD(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir()}
	runner := newFakeCommandRunner(cfg)
	restoreRunner := SetCommandRunnerFactoryForTests(func() CommandRunner { return runner })
	defer restoreRunner()
	restoreSource := SetMarketplaceSourceForTests("https://github.com/Angel-MercadoCLK/click-ai-devkit")
	defer restoreSource()

	models := map[modelconfig.Phase]string{
		modelconfig.PhaseOrchestrator:  "opus",
		modelconfig.PhasePRDWriter:     "sonnet",
		modelconfig.PhaseArchitect:     "opus",
		modelconfig.PhaseReviewer:      "haiku",
		modelconfig.PhaseMemoryCurator: "sonnet",
	}

	if err := SyncMarketplacePlugins(models); err != nil {
		t.Fatalf("SyncMarketplacePlugins() error = %v", err)
	}

	want := []commandInvocation{
		{Name: "claude", Args: []string{
			"plugin", "marketplace", "add", "https://github.com/Angel-MercadoCLK/click-ai-devkit",
			"--sparse", ".claude-plugin", "plugins",
		}},
		{Name: "claude", Args: []string{
			"plugin", "install", "click-sdd@click-ai-devkit",
			"--config", "orchestration_profile=default",
			"--config", "orchestrator_model=opus",
			"--config", "prd_writer_model=sonnet",
			"--config", "architect_model=opus",
			"--config", "reviewer_model=haiku",
			"--config", "memory_curator_model=sonnet",
		}},
		{Name: "claude", Args: []string{"plugin", "install", "click-memory@click-ai-devkit"}},
		{Name: "claude", Args: []string{"plugin", "install", "click-review@click-ai-devkit"}},
	}
	if !reflect.DeepEqual(runner.commands, want) {
		t.Fatalf("runner.commands = %#v, want %#v", runner.commands, want)
	}
}

// TestSyncMarketplacePlugins_DefaultsWhenModelsNil confirms a nil models map (e.g. a caller that
// never resolved user overrides) still installs click-sdd with D25's defaults rather than
// omitting --config entirely, keeping install always self-describing.
func TestSyncMarketplacePlugins_DefaultsWhenModelsNil(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir()}
	runner := newFakeCommandRunner(cfg)
	restoreRunner := SetCommandRunnerFactoryForTests(func() CommandRunner { return runner })
	defer restoreRunner()
	restoreSource := SetMarketplaceSourceForTests("https://github.com/Angel-MercadoCLK/click-ai-devkit")
	defer restoreSource()

	if err := SyncMarketplacePlugins(nil); err != nil {
		t.Fatalf("SyncMarketplacePlugins(nil) error = %v", err)
	}

	wantClickSDD := commandInvocation{Name: "claude", Args: []string{
		"plugin", "install", "click-sdd@click-ai-devkit",
		"--config", "orchestration_profile=default",
		"--config", "orchestrator_model=opus",
		"--config", "prd_writer_model=opus",
		"--config", "architect_model=opus",
		"--config", "reviewer_model=opus",
		"--config", "memory_curator_model=sonnet",
	}}
	if !reflect.DeepEqual(runner.commands[1], wantClickSDD) {
		t.Fatalf("runner.commands[1] = %#v, want %#v", runner.commands[1], wantClickSDD)
	}
}

func TestSyncMarketplacePluginsForProfile_UsesProfileNameAndDefaults(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir()}
	runner := newFakeCommandRunner(cfg)
	restoreRunner := SetCommandRunnerFactoryForTests(func() CommandRunner { return runner })
	defer restoreRunner()
	restoreSource := SetMarketplaceSourceForTests("https://github.com/Angel-MercadoCLK/click-ai-devkit")
	defer restoreSource()

	profile := modelconfig.DefaultProfile()
	profile.Models[modelconfig.PhaseOrchestrator] = "sonnet"
	profile.Models[modelconfig.PhaseMemoryCurator] = "opus"

	if err := SyncMarketplacePluginsForProfile(profile, nil); err != nil {
		t.Fatalf("SyncMarketplacePluginsForProfile(profile, nil) error = %v", err)
	}

	wantClickSDD := commandInvocation{Name: "claude", Args: []string{
		"plugin", "install", "click-sdd@click-ai-devkit",
		"--config", "orchestration_profile=default",
		"--config", "orchestrator_model=sonnet",
		"--config", "prd_writer_model=opus",
		"--config", "architect_model=opus",
		"--config", "reviewer_model=opus",
		"--config", "memory_curator_model=opus",
	}}
	if !reflect.DeepEqual(runner.commands[1], wantClickSDD) {
		t.Fatalf("runner.commands[1] = %#v, want %#v", runner.commands[1], wantClickSDD)
	}
}
