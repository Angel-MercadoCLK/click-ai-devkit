package installer

import (
	"reflect"
	"strings"
	"testing"
)

func TestBuildTargetPlan_TargetFirstOrderAndSharedProjections(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir(), CodexHome: t.TempDir(), OpenClawHome: t.TempDir(), ClickStateHome: t.TempDir()}
	selection := TargetSelection{Configured: true, Claude: true, Codex: true, OpenClaw: true}

	plan := BuildTargetPlan(cfg, selection, PlanOptions{CloudResolvable: true})

	if got, want := plan.Selection, selection; got != want {
		t.Fatalf("Selection = %+v, want %+v", got, want)
	}
	got := plan.StepLabels()
	want := []string{
		"gentle-ai",
		"Claude Code",
		"Codex CLI",
		"OpenClaw",
		"Engram Cloud",
		"plugins",
		"memory guard",
		"SDD assets",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("StepLabels() = %#v, want %#v", got, want)
	}
	if !strings.Contains(plan.CapabilitiesSummary(), "Codex") || !strings.Contains(plan.CapabilitiesSummary(), "OpenClaw") || !strings.Contains(plan.CapabilitiesSummary(), "Claude Code") {
		t.Fatalf("CapabilitiesSummary() = %q, want every selected target explained", plan.CapabilitiesSummary())
	}
}

func TestBuildTargetPlan_ClaudeHasNoModelLifecycleOwnership(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir(), ClickStateHome: t.TempDir()}
	plan := BuildTargetPlan(cfg, TargetSelection{Configured: true, Claude: true}, PlanOptions{})

	for _, step := range plan.Steps {
		if step.ID == "claude-models" || step.Label == "Claude model/profile" {
			t.Fatalf("Claude plan contains retired model step: %+v", step)
		}
	}
	for _, path := range plan.SnapshotPaths() {
		if path == cfg.ModelsPath() {
			t.Fatalf("Claude plan snapshots legacy model artifact %q", path)
		}
	}
	for _, check := range plan.DoctorCheckKinds() {
		if check == DoctorCheckModelsConfig || check == DoctorCheckAppliedPluginConfig {
			t.Fatalf("Claude plan retains retired model doctor check %q", check)
		}
	}
	for _, action := range append(append(plan.InstallActionKinds(), plan.UpdateActionKinds()...), plan.UninstallActionKinds()...) {
		if action == StepActionSaveModels || action == StepActionRemoveModels {
			t.Fatalf("Claude plan retains retired model action %q", action)
		}
	}
}

func TestBuildTargetPlan_DelegatesEngramAndContext7LifecycleToGentleAI(t *testing.T) {
	selection := TargetSelection{Configured: true, Claude: true, Codex: true, OpenClaw: true}
	plan := BuildTargetPlan(Config{ClaudeHome: t.TempDir(), CodexHome: t.TempDir(), OpenClawHome: t.TempDir(), ClickStateHome: t.TempDir()}, selection, PlanOptions{CloudResolvable: true})
	for _, action := range append(append(plan.InstallActionKinds(), plan.UpdateActionKinds()...), plan.UninstallActionKinds()...) {
		switch action {
		case StepActionSyncEngram, StepActionRemoveEngram, StepActionSyncContext7, StepActionRemoveContext7,
			StepActionSyncCodexMCP, StepActionRemoveCodexMCP, StepActionRegisterOpenClawMCP, StepActionUnregisterOpenClawMCP,
			StepActionSyncOpenClawMCP:
			t.Errorf("Click action %q still owns a gentle-ai-managed registration", action)
		}
	}
}

func TestBuildTargetPlan_DoesNotRequireRetiredClickSDDOrAgentAssets(t *testing.T) {
	selection := TargetSelection{Configured: true, Claude: true}
	plan := BuildTargetPlan(Config{ClaudeHome: t.TempDir()}, selection, PlanOptions{})
	for _, check := range plan.DoctorCheckKinds() {
		if string(check) == "click-sdd-plugin" || string(check) == "engram-subagent-visibility" {
			t.Errorf("DoctorCheckKinds() retains retired Click SDD dependency %q", check)
		}
	}
}

func TestBuildTargetPlan_CodexOnlySkipsClaudeOwnedSteps(t *testing.T) {
	cfg := Config{CodexHome: t.TempDir(), ClickStateHome: t.TempDir()}
	selection := TargetSelection{Configured: true, Codex: true}

	plan := BuildTargetPlan(cfg, selection, PlanOptions{})
	got := plan.StepLabels()
	want := []string{"gentle-ai", "Codex CLI", "SDD assets"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("StepLabels() = %#v, want %#v", got, want)
	}
	if strings.Contains(plan.CapabilitiesSummary(), "Claude") {
		t.Fatalf("CapabilitiesSummary() = %q, want no Claude-owned capability in a Codex-only plan", plan.CapabilitiesSummary())
	}
}

func TestBuildTargetPlan_RetiresCodexAndOpenClawModelLifecycleActions(t *testing.T) {
	cfg := Config{CodexHome: t.TempDir(), OpenClawHome: t.TempDir(), ClickStateHome: t.TempDir()}
	selection := TargetSelection{Configured: true, Codex: true, OpenClaw: true}
	plan := BuildTargetPlan(cfg, selection, PlanOptions{})

	for _, label := range plan.StepLabels() {
		if label == "Codex model profile" || label == "Codex native model" || label == "OpenClaw native model" {
			t.Errorf("StepLabels() contains retired model step %q", label)
		}
	}
	if got, want := plan.InstallActionKinds(), []StepActionKind{StepActionSyncGentleAI, StepActionSyncOpenClawWorkspace, StepActionSyncOpenClawPlugin, StepActionSyncOpenClawSkills, StepActionSyncCodexGuidance}; !reflect.DeepEqual(got, want) {
		t.Errorf("InstallActionKinds() = %#v, want only upstream and non-model Click actions %#v", got, want)
	}
	if got, want := plan.UpdateActionKinds(), []StepActionKind{StepActionSyncGentleAI, StepActionSyncOpenClawWorkspace, StepActionSyncOpenClawPlugin, StepActionSyncOpenClawSkills, StepActionSyncCodexGuidance}; !reflect.DeepEqual(got, want) {
		t.Errorf("UpdateActionKinds() = %#v, want only upstream and non-model Click actions %#v", got, want)
	}
	if got, want := plan.UninstallActionKinds(), []StepActionKind{StepActionStripCodexGuidance, StepActionStripOpenClawWorkspace, StepActionRemoveOpenClawPlugin, StepActionRemoveOpenClawSkills, StepActionRemoveTargetSelection}; !reflect.DeepEqual(got, want) {
		t.Errorf("UninstallActionKinds() = %#v, want only Click-owned non-model cleanup %#v", got, want)
	}
	if got, want := plan.DoctorCheckKinds(), []DoctorCheckKind{DoctorCheckCodexGuidance, DoctorCheckOpenClaw}; !reflect.DeepEqual(got, want) {
		t.Errorf("DoctorCheckKinds() = %#v, want only target health and guidance checks %#v", got, want)
	}
	if !sliceContains(plan.SnapshotPaths(), cfg.CodexConfigPath()) {
		t.Errorf("SnapshotPaths() = %#v, want Codex config snapshot retained for non-model MCP changes", plan.SnapshotPaths())
	}
	for _, path := range []string{cfg.ModelsPath(), cfg.CodexModelProfilePath(), cfg.OpenClawModelProfilePath()} {
		if sliceContains(plan.SnapshotPaths(), path) {
			t.Errorf("SnapshotPaths() = %#v, want retired model artifact %q omitted from new plans", plan.SnapshotPaths(), path)
		}
	}
	if !sliceContains(plan.SnapshotPaths(), cfg.OpenClawMCPConfigPath()) {
		t.Errorf("SnapshotPaths() = %#v, want OpenClaw config retained for upstream MCP changes", plan.SnapshotPaths())
	}
}

func TestBuildTargetPlan_SnapshotPathsIncludeCodexConfigForMCPChanges(t *testing.T) {
	cfg := Config{CodexHome: t.TempDir(), ClickStateHome: t.TempDir()}
	selection := TargetSelection{Configured: true, Codex: true}

	plan := BuildTargetPlan(cfg, selection, PlanOptions{})
	paths := plan.SnapshotPaths()
	if !sliceContains(paths, cfg.CodexConfigPath()) {
		t.Fatalf("SnapshotPaths() = %#v, want Codex config.toml path %q for upstream MCP rollback", paths, cfg.CodexConfigPath())
	}
	if sliceContains(paths, cfg.ClaudeMDPath()) {
		t.Fatalf("SnapshotPaths() = %#v, want no Claude snapshot path in a Codex-only plan", paths)
	}
}

func TestBuildTargetPlan_CodexOnlyExposesLifecycleActionsForProductionCommands(t *testing.T) {
	cfg := Config{CodexHome: t.TempDir(), ClickStateHome: t.TempDir()}
	selection := TargetSelection{Configured: true, Codex: true}

	plan := BuildTargetPlan(cfg, selection, PlanOptions{})

	// Engram MCP belongs to gentle-ai; Click install only manages Codex guidance.
	if got, want := plan.InstallActionKinds(), []StepActionKind{StepActionSyncGentleAI, StepActionSyncCodexGuidance}; !reflect.DeepEqual(got, want) {
		t.Fatalf("InstallActionKinds() = %#v, want %#v", got, want)
	}
	if got, want := plan.UpdateActionKinds(), []StepActionKind{StepActionSyncGentleAI, StepActionSyncCodexGuidance}; !reflect.DeepEqual(got, want) {
		t.Fatalf("UpdateActionKinds() = %#v, want %#v", got, want)
	}
	// Uninstall removes Click guidance only and never removes shared upstream MCP registrations.
	if got, want := plan.UninstallActionKinds(), []StepActionKind{StepActionStripCodexGuidance, StepActionRemoveTargetSelection}; !reflect.DeepEqual(got, want) {
		t.Fatalf("UninstallActionKinds() = %#v, want %#v", got, want)
	}
	if got, want := plan.DoctorCheckKinds(), []DoctorCheckKind{DoctorCheckCodexGuidance}; !reflect.DeepEqual(got, want) {
		t.Fatalf("DoctorCheckKinds() = %#v, want %#v", got, want)
	}
}

// TestBuildTargetPlan_OpenClawDelegatesMCPRegistration proves that Click no longer registers or
// removes the Engram MCP server in OpenClaw.
func TestBuildTargetPlan_OpenClawDelegatesMCPRegistration(t *testing.T) {
	cfg := Config{OpenClawHome: t.TempDir(), ClickStateHome: t.TempDir()}
	selection := TargetSelection{Configured: true, OpenClaw: true}

	plan := BuildTargetPlan(cfg, selection, PlanOptions{})

	if got, want := plan.InstallActionKinds(), []StepActionKind{StepActionSyncGentleAI, StepActionSyncOpenClawWorkspace, StepActionSyncOpenClawPlugin, StepActionSyncOpenClawSkills}; !reflect.DeepEqual(got, want) {
		t.Fatalf("InstallActionKinds() = %#v, want %#v", got, want)
	}
	if got, want := plan.UpdateActionKinds(), []StepActionKind{StepActionSyncGentleAI, StepActionSyncOpenClawWorkspace, StepActionSyncOpenClawPlugin, StepActionSyncOpenClawSkills}; !reflect.DeepEqual(got, want) {
		t.Fatalf("UpdateActionKinds() = %#v, want %#v", got, want)
	}
	if got, want := plan.UninstallActionKinds(), []StepActionKind{StepActionStripOpenClawWorkspace, StepActionRemoveOpenClawPlugin, StepActionRemoveOpenClawSkills, StepActionRemoveTargetSelection}; !reflect.DeepEqual(got, want) {
		t.Fatalf("UninstallActionKinds() = %#v, want %#v", got, want)
	}
}

// TestBuildTargetPlan_ClaudePreservesModelLifecycleBoundary verifies that Click's Claude target
// plan removes only its Engram Cloud state and does not manage legacy model artifacts during
// install/update/uninstall. Order follows step order, which is what UninstallActionKinds
// concatenates — there is no separate uninstall ordering slice, unlike installActionOrder/updateActionOrder.
func TestBuildTargetPlan_ClaudePreservesModelLifecycleBoundary(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir(), ClickStateHome: t.TempDir()}
	selection := TargetSelection{Configured: true, Claude: true}

	plan := BuildTargetPlan(cfg, selection, PlanOptions{CloudResolvable: true})

	want := []StepActionKind{
		StepActionRemoveMarketplacePlugins,
		StepActionRemoveEngramCloudSessionSync,
		StepActionRemoveEngramCloudState,
		StepActionStripClaudeManagedBlock,
		StepActionUnregisterMemoryGuard,
		StepActionRemoveTargetSelection,
	}
	if got := plan.UninstallActionKinds(); !reflect.DeepEqual(got, want) {
		t.Fatalf("UninstallActionKinds() = %#v, want %#v", got, want)
	}

	// Cloud teardown must not leak into the forward directions; model artifacts are no longer
	// created or removed by Click.
	for _, action := range append(plan.InstallActionKinds(), plan.UpdateActionKinds()...) {
		if action == StepActionRemoveModels || action == StepActionSaveModels || action == StepActionRemoveEngramCloudState || action == StepActionSyncOpenClawModelProfile {
			t.Fatalf("teardown action %q leaked into the install/update action set", action)
		}
	}
}

// TestEngramCloudStatePresent is `click uninstall`'s teardown trigger for the Engram Cloud step.
// EngramCloudConfigured cannot be used there: it requires ENGRAM_CLOUD_TOKEN in the environment,
// which a developer tearing down their setup has almost never still exported — the enrollment record
// on disk is the accurate signal that there is something to remove.
func TestEngramCloudStatePresent(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir()}
	if EngramCloudStatePresent(cfg) {
		t.Fatal("EngramCloudStatePresent() = true with no state file, want false")
	}
	if err := writeJSONFile(cfg.EngramCloudStatePath(), engramCloudState{Enrolled: true}); err != nil {
		t.Fatalf("writeJSONFile(state) error = %v", err)
	}
	if !EngramCloudStatePresent(cfg) {
		t.Fatal("EngramCloudStatePresent() = false with the state file present, want true")
	}
	if EngramCloudStatePresent(Config{}) {
		t.Fatal("EngramCloudStatePresent() = true with no ClaudeHome, want false")
	}
}

func TestBuildTargetPlan_UninstallAlwaysRemovesSessionSync(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir(), ClickStateHome: t.TempDir()}
	selection := TargetSelection{Configured: true, Claude: true}

	for _, options := range []PlanOptions{{}, {CloudResolvable: true}} {
		actions := BuildTargetPlan(cfg, selection, options).UninstallActionKinds()
		index := -1
		for i, action := range actions {
			if action == StepActionRemoveEngramCloudSessionSync {
				index = i
				break
			}
		}
		if index < 0 {
			t.Fatalf("UninstallActionKinds() = %#v, want session-sync removal", actions)
		}
		if index+1 >= len(actions) || actions[index+1] != StepActionRemoveEngramCloudState {
			t.Fatalf("UninstallActionKinds() = %#v, want session-sync removal immediately before cloud state removal", actions)
		}
	}
}

func sliceContains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

// TestBuildTargetPlan_MatrixAssertions guards that every emitted SnapshotDecl across all
// plan variants has a known, non-empty Policy, and that any path appearing more than once
// (across steps) has the SAME policy every time. This is Layer B of the omission guards.
func TestBuildTargetPlan_MatrixAssertions(t *testing.T) {
	testCases := []struct {
		name      string
		selection TargetSelection
		options   PlanOptions
		setupFunc func(t *testing.T) Config
	}{
		{
			name:      "Claude-only",
			selection: TargetSelection{Claude: true},
			options:   PlanOptions{},
			setupFunc: func(t *testing.T) Config {
				return Config{
					ClaudeHome:     t.TempDir(),
					ClickStateHome: t.TempDir(),
				}
			},
		},
		{
			name:      "Codex-only",
			selection: TargetSelection{Codex: true},
			options:   PlanOptions{},
			setupFunc: func(t *testing.T) Config {
				cfg := Config{
					CodexHome:      t.TempDir(),
					ClickStateHome: t.TempDir(),
				}
				return cfg
			},
		},
		{
			name:      "OpenClaw-only",
			selection: TargetSelection{OpenClaw: true},
			options:   PlanOptions{},
			setupFunc: func(t *testing.T) Config {
				return Config{
					OpenClawHome:   t.TempDir(),
					ClickStateHome: t.TempDir(),
				}
			},
		},
		{
			name:      "Claude+Codex combined",
			selection: TargetSelection{Claude: true, Codex: true},
			options:   PlanOptions{},
			setupFunc: func(t *testing.T) Config {
				return Config{
					ClaudeHome:     t.TempDir(),
					CodexHome:      t.TempDir(),
					ClickStateHome: t.TempDir(),
				}
			},
		},
		{
			name:      "Claude+OpenClaw combined",
			selection: TargetSelection{Claude: true, OpenClaw: true},
			options:   PlanOptions{},
			setupFunc: func(t *testing.T) Config {
				return Config{
					ClaudeHome:     t.TempDir(),
					OpenClawHome:   t.TempDir(),
					ClickStateHome: t.TempDir(),
				}
			},
		},
		{
			name:      "All three combined",
			selection: TargetSelection{Claude: true, Codex: true, OpenClaw: true},
			options:   PlanOptions{},
			setupFunc: func(t *testing.T) Config {
				return Config{
					ClaudeHome:     t.TempDir(),
					CodexHome:      t.TempDir(),
					OpenClawHome:   t.TempDir(),
					ClickStateHome: t.TempDir(),
				}
			},
		},
		{
			name:      "Claude with cloud configured",
			selection: TargetSelection{Claude: true},
			options:   PlanOptions{CloudResolvable: true},
			setupFunc: func(t *testing.T) Config {
				return Config{
					ClaudeHome:     t.TempDir(),
					ClickStateHome: t.TempDir(),
				}
			},
		},
		{
			name:      "Codex model options retired",
			selection: TargetSelection{Codex: true},
			options:   PlanOptions{},
			setupFunc: func(t *testing.T) Config {
				return Config{
					CodexHome:      t.TempDir(),
					ClickStateHome: t.TempDir(),
				}
			},
		},
		{
			name:      "OpenClaw model options retired",
			selection: TargetSelection{OpenClaw: true},
			options:   PlanOptions{},
			setupFunc: func(t *testing.T) Config {
				return Config{
					OpenClawHome:   t.TempDir(),
					ClickStateHome: t.TempDir(),
				}
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := tc.setupFunc(t)
			plan := BuildTargetPlan(cfg, tc.selection, tc.options)

			// Collect all snapshot declarations and track policies per path
			pathPolicy := make(map[string]DriftPolicy)
			knownPolicies := map[DriftPolicy]bool{
				DriftPolicyWholeFileVeto:      true,
				DriftPolicyManagedContentVeto: true,
				DriftPolicyNonVeto:            true,
			}

			for _, step := range plan.Steps {
				for _, decl := range step.Snapshot {
					// Check every path has a known, non-empty policy
					if decl.Policy == "" {
						t.Fatalf("step %q has SnapshotDecl with empty Policy for path %q", step.ID, decl.Path)
					}
					if !knownPolicies[decl.Policy] {
						t.Fatalf("step %q has SnapshotDecl with unknown Policy %q for path %q", step.ID, decl.Policy, decl.Path)
					}

					// Check duplicate paths have consistent policies
					if priorPolicy, exists := pathPolicy[decl.Path]; exists {
						if priorPolicy != decl.Policy {
							t.Fatalf("path %q appears with conflicting policies: %q and %q", decl.Path, priorPolicy, decl.Policy)
						}
					} else {
						pathPolicy[decl.Path] = decl.Policy
					}
				}
			}
		})
	}
}

func TestBuildTargetPlan_CloudResolvableConfiguresSessionSync(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir(), ClickStateHome: t.TempDir()}
	selection := TargetSelection{Configured: true, Claude: true}

	// CloudResolvable means server and project are known, token optional — the Claude plan must
	// carry the session-sync configure action in install/update plus the cloud doctor and uninstall
	// actions even when no token is present (DD-4).
	plan := BuildTargetPlan(cfg, selection, PlanOptions{CloudResolvable: true})

	// Find the engram-cloud step (PlanTargetShared, added when CloudResolvable && Claude).
	var cloudStep *Step
	for i := range plan.Steps {
		if plan.Steps[i].ID == "engram-cloud" {
			cloudStep = &plan.Steps[i]
			break
		}
	}
	if cloudStep == nil {
		t.Fatalf("engram-cloud step not found in plan")
	}

	// Verify StepActionConfigureEngramCloudSessionSync is in install/update actions
	hasConfigureInstall := false
	hasConfigureUpdate := false
	for _, action := range cloudStep.InstallActions {
		if action == "configure-engram-cloud-session-sync" {
			hasConfigureInstall = true
			break
		}
	}
	for _, action := range cloudStep.UpdateActions {
		if action == "configure-engram-cloud-session-sync" {
			hasConfigureUpdate = true
			break
		}
	}
	if !hasConfigureInstall {
		t.Fatalf("InstallActions = %v, want to contain configure-engram-cloud-session-sync", cloudStep.InstallActions)
	}
	if !hasConfigureUpdate {
		t.Fatalf("UpdateActions = %v, want to contain configure-engram-cloud-session-sync", cloudStep.UpdateActions)
	}

	// Verify cloud doctor and uninstall actions are present
	hasDoctorAction := false
	hasUninstallAction := false
	for _, action := range cloudStep.DoctorChecks {
		if action == "engram-cloud" {
			hasDoctorAction = true
		}
	}
	for _, action := range cloudStep.UninstallActions {
		if action == "remove-engram-cloud-state" {
			hasUninstallAction = true
		}
	}
	if !hasDoctorAction {
		t.Fatalf("DoctorChecks = %v, want to contain engram-cloud", cloudStep.DoctorChecks)
	}
	if !hasUninstallAction {
		t.Fatalf("UninstallActions = %v, want to contain remove-engram-cloud-state", cloudStep.UninstallActions)
	}

	// The ordered action kinds must carry the configure action BEFORE the D40 enrollment action,
	// mirroring DD-4's "configure session sync, then enroll/sync" ordering.
	installKinds := plan.InstallActionKinds()
	updateKinds := plan.UpdateActionKinds()
	assertConfigureBeforeSync := func(name string, kinds []StepActionKind) {
		t.Helper()
		configureIdx := -1
		syncIdx := -1
		for i, k := range kinds {
			if k == StepActionConfigureEngramCloudSessionSync {
				configureIdx = i
			}
			if k == StepActionSyncEngramCloud {
				syncIdx = i
			}
		}
		if configureIdx == -1 {
			t.Fatalf("%s = %v, want to contain configure-engram-cloud-session-sync", name, kinds)
		}
		if syncIdx == -1 {
			t.Fatalf("%s = %v, want to contain sync-engram-cloud", name, kinds)
		}
		if configureIdx >= syncIdx {
			t.Fatalf("%s = %v, want configure-engram-cloud-session-sync (idx %d) before sync-engram-cloud (idx %d)", name, kinds, configureIdx, syncIdx)
		}
	}
	assertConfigureBeforeSync("InstallActionKinds", installKinds)
	assertConfigureBeforeSync("UpdateActionKinds", updateKinds)
}

func TestBuildTargetPlan_GentleAIFirstAndNotReversedYet(t *testing.T) {
	for _, selection := range []TargetSelection{{Claude: true}, {Codex: true}, {OpenClaw: true}, {Claude: true, Codex: true, OpenClaw: true}} {
		plan := BuildTargetPlan(Config{}, selection, PlanOptions{})
		for _, actions := range [][]StepActionKind{plan.InstallActionKinds(), plan.UpdateActionKinds()} {
			if len(actions) == 0 || string(actions[0]) != "sync-gentle-ai" {
				t.Fatalf("actions = %q, want gentle-ai first", actions)
			}
		}
		for _, step := range plan.Steps {
			if step.ID == "gentle-ai" && (len(step.Snapshot) != 0 || len(step.UninstallActions) != 0 || len(step.DoctorChecks) != 0) {
				t.Fatal("T10 surfaces wired prematurely")
			}
		}
	}
	if actions := BuildTargetPlan(Config{}, TargetSelection{}, PlanOptions{}).InstallActionKinds(); len(actions) != 0 {
		t.Fatalf("empty selection actions = %q", actions)
	}
}
