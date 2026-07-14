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
//
// It also guards design D3: the FIRST --config flag on click-sdd must always be
// `orchestration_profile=<name>`, ahead of the 18 per-phase flags — SyncMarketplacePlugins's
// trailing variadic profile argument (see plugins.go) is how a caller opts into a non-balanced
// profile without breaking the two existing 1-arg callers (cli/install.go, cli/update.go; PR3
// migrates them to actually pass the selected profile through).
func TestSyncMarketplacePlugins_PassesPerPhaseConfigFlagsForClickSDD(t *testing.T) {
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

	if err := SyncMarketplacePlugins(models, modelconfig.ProfileCostSaver); err != nil {
		t.Fatalf("SyncMarketplacePlugins() error = %v", err)
	}

	want := []commandInvocation{
		{Name: "claude", Args: []string{
			"plugin", "marketplace", "add", "https://github.com/Angel-MercadoCLK/click-ai-devkit",
			"--sparse", ".claude-plugin", "plugins",
		}},
		{Name: "claude", Args: []string{
			"plugin", "marketplace", "update", marketplaceName,
		}},
		{Name: "claude", Args: []string{
			"plugin", "install", "click-sdd@click-ai-devkit",
			"--config", "orchestration_profile=cost-saver",
			"--config", "explore_model=sonnet",
			"--config", "propose_model=opus",
			"--config", "spec_model=sonnet",
			"--config", "design_model=opus",
			"--config", "tasks_model=sonnet",
			"--config", "apply_model=haiku",
			"--config", "verify_model=opus",
			"--config", "archive_model=haiku",
			"--config", "onboard_model=haiku",
			"--config", "jd_judge_a_model=sonnet",
			"--config", "jd_judge_b_model=sonnet",
			"--config", "jd_fix_agent_model=sonnet",
			"--config", "review_risk_model=opus",
			"--config", "review_readability_model=sonnet",
			"--config", "review_reliability_model=haiku",
			"--config", "review_resilience_model=sonnet",
			"--config", "review_refuter_model=opus",
			"--config", "default_model=sonnet",
		}},
		{Name: "claude", Args: []string{"plugin", "install", "click-memory@click-ai-devkit"}},
		{Name: "claude", Args: []string{"plugin", "install", "click-review@click-ai-devkit"}},
		{Name: "claude", Args: []string{"plugin", "install", "click-skills@click-ai-devkit"}},
	}
	if !reflect.DeepEqual(runner.commands, want) {
		t.Fatalf("runner.commands = %#v, want %#v", runner.commands, want)
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

	if err := SyncMarketplacePlugins(nil); err != nil {
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
		"--config", "orchestration_profile=balanced",
		"--config", "explore_model=sonnet",
		"--config", "propose_model=opus",
		"--config", "spec_model=sonnet",
		"--config", "design_model=opus",
		"--config", "tasks_model=sonnet",
		"--config", "apply_model=sonnet",
		"--config", "verify_model=opus",
		"--config", "archive_model=haiku",
		"--config", "onboard_model=haiku",
		"--config", "jd_judge_a_model=sonnet",
		"--config", "jd_judge_b_model=sonnet",
		"--config", "jd_fix_agent_model=sonnet",
		"--config", "review_risk_model=sonnet",
		"--config", "review_readability_model=sonnet",
		"--config", "review_reliability_model=sonnet",
		"--config", "review_resilience_model=sonnet",
		"--config", "review_refuter_model=sonnet",
		"--config", "default_model=sonnet",
	}}
	if !reflect.DeepEqual(runner.commands[2], wantClickSDD) {
		t.Fatalf("runner.commands[2] = %#v, want %#v", runner.commands[2], wantClickSDD)
	}
}

// TestClickSDDConfigArgs_ProfileFlagFirstThenPhasesInOrder is the narrow, direct proof for design
// D3's emission order requirement: clickSDDConfigArgs's very first pair MUST be
// "--config orchestration_profile=<name>", followed by exactly one "--config <phase>_model=<alias>"
// pair per modelconfig.Phases entry, in modelconfig.Phases order — independent of
// TestSyncMarketplacePlugins_PassesPerPhaseConfigFlagsForClickSDD, which proves the same thing only
// indirectly through the full command list.
func TestClickSDDConfigArgs_ProfileFlagFirstThenPhasesInOrder(t *testing.T) {
	resolved := modelconfig.Defaults()

	got := clickSDDConfigArgs(modelconfig.ProfileQuality, resolved)

	if len(got) < 2 || got[0] != "--config" || got[1] != "orchestration_profile=quality" {
		t.Fatalf("clickSDDConfigArgs()[:2] = %#v, want [\"--config\" \"orchestration_profile=quality\"]", got[:min(2, len(got))])
	}
	rest := got[2:]
	if len(rest) != len(modelconfig.Phases)*2 {
		t.Fatalf("len(clickSDDConfigArgs()[2:]) = %d, want %d (2 per phase)", len(rest), len(modelconfig.Phases)*2)
	}
	for i, phase := range modelconfig.Phases {
		flag := rest[i*2]
		pair := rest[i*2+1]
		wantPair := phase.ConfigKey() + "=" + resolved[phase]
		if flag != "--config" || pair != wantPair {
			t.Errorf("phase %d (%q): got (%q, %q), want (\"--config\", %q)", i, phase, flag, pair, wantPair)
		}
	}
}
