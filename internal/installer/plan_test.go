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
		"Claude Code",
		"Claude model/profile",
		"Codex CLI",
		"Codex Engram MCP",
		"Codex model profile",
		"Codex native model",
		"OpenClaw",
		"OpenClaw Engram MCP",
		"OpenClaw native model",
		"Engram",
		"Engram Cloud",
		"Context7",
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

func TestBuildTargetPlan_CodexOnlySkipsClaudeOwnedSteps(t *testing.T) {
	cfg := Config{CodexHome: t.TempDir(), ClickStateHome: t.TempDir()}
	selection := TargetSelection{Configured: true, Codex: true}

	plan := BuildTargetPlan(cfg, selection, PlanOptions{})
	got := plan.StepLabels()
	want := []string{"Codex CLI", "Codex Engram MCP", "Codex model profile", "Codex native model", "SDD assets"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("StepLabels() = %#v, want %#v", got, want)
	}
	if strings.Contains(plan.CapabilitiesSummary(), "Claude") {
		t.Fatalf("CapabilitiesSummary() = %q, want no Claude-owned capability in a Codex-only plan", plan.CapabilitiesSummary())
	}
}

func TestBuildTargetPlan_SnapshotPathsIncludeCodexConfigForNativeMutation(t *testing.T) {
	cfg := Config{CodexHome: t.TempDir(), ClickStateHome: t.TempDir()}
	selection := TargetSelection{Configured: true, Codex: true}

	plan := BuildTargetPlan(cfg, selection, PlanOptions{})
	paths := plan.SnapshotPaths()
	if !sliceContains(paths, cfg.CodexConfigPath()) {
		t.Fatalf("SnapshotPaths() = %#v, want Codex config.toml path %q for native rollback", paths, cfg.CodexConfigPath())
	}
	if sliceContains(paths, cfg.ClaudeMDPath()) {
		t.Fatalf("SnapshotPaths() = %#v, want no Claude snapshot path in a Codex-only plan", paths)
	}
}

func TestBuildTargetPlan_CodexOnlyExposesLifecycleActionsForProductionCommands(t *testing.T) {
	cfg := Config{CodexHome: t.TempDir(), ClickStateHome: t.TempDir()}
	selection := TargetSelection{Configured: true, Codex: true}

	plan := BuildTargetPlan(cfg, selection, PlanOptions{})

	// Without an explicit --codex-model opt-in, the native config.toml mutation is NOT part of the
	// install/update action set: a plain Codex run neither lists nor performs any native mutation.
	// StepActionSyncCodexMCP, unlike the native-model mutation, is ALWAYS present — registering
	// Engram's MCP server is independent of --codex-model.
	if got, want := plan.InstallActionKinds(), []StepActionKind{StepActionSyncCodexGuidance, StepActionSyncCodexMCP, StepActionSaveCodexModelProfile}; !reflect.DeepEqual(got, want) {
		t.Fatalf("InstallActionKinds() = %#v, want %#v", got, want)
	}
	if got, want := plan.UpdateActionKinds(), []StepActionKind{StepActionSyncCodexGuidance, StepActionSyncCodexMCP, StepActionSaveCodexModelProfile}; !reflect.DeepEqual(got, want) {
		t.Fatalf("UpdateActionKinds() = %#v, want %#v", got, want)
	}
	// Uninstall reverses the managed AGENTS.md block (StripCodexGuidance) AND deregisters Engram's
	// Codex MCP server (StepActionRemoveCodexMCP — SyncCodexMCP's reversal), then removes the neutral
	// target selection. The native config.toml model is deliberately NOT reverted (user-owned).
	if got, want := plan.UninstallActionKinds(), []StepActionKind{StepActionStripCodexGuidance, StepActionRemoveCodexMCP, StepActionRemoveCodexModelProfile, StepActionRemoveTargetSelection}; !reflect.DeepEqual(got, want) {
		t.Fatalf("UninstallActionKinds() = %#v, want %#v", got, want)
	}
	if got, want := plan.DoctorCheckKinds(), []DoctorCheckKind{DoctorCheckCodexGuidance}; !reflect.DeepEqual(got, want) {
		t.Fatalf("DoctorCheckKinds() = %#v, want %#v", got, want)
	}
}

// TestBuildTargetPlan_CodexNativeModelFlag_AddsNativeMutationAction proves the opt-in path: when the
// developer passed --codex-model (PlanOptions.CodexNativeModel), the native config.toml mutation is
// added to the install/update action set so the flag path still writes the Codex model.
func TestBuildTargetPlan_CodexNativeModelFlag_AddsNativeMutationAction(t *testing.T) {
	cfg := Config{CodexHome: t.TempDir(), ClickStateHome: t.TempDir()}
	selection := TargetSelection{Configured: true, Codex: true}

	plan := BuildTargetPlan(cfg, selection, PlanOptions{CodexNativeModel: true})

	if got, want := plan.InstallActionKinds(), []StepActionKind{StepActionSyncCodexGuidance, StepActionSyncCodexMCP, StepActionSaveCodexModelProfile, StepActionConfigureCodexNativeModel}; !reflect.DeepEqual(got, want) {
		t.Fatalf("InstallActionKinds() = %#v, want %#v", got, want)
	}
	if got, want := plan.UpdateActionKinds(), []StepActionKind{StepActionSyncCodexGuidance, StepActionSyncCodexMCP, StepActionSaveCodexModelProfile, StepActionConfigureCodexNativeModel}; !reflect.DeepEqual(got, want) {
		t.Fatalf("UpdateActionKinds() = %#v, want %#v", got, want)
	}
}

// TestBuildTargetPlan_OpenClawExposesMCPRegistrationActions proves that OpenClaw's dedicated MCP
// step contributes register-openclaw-mcp to install/update and unregister-openclaw-mcp to uninstall.
func TestBuildTargetPlan_OpenClawExposesMCPRegistrationActions(t *testing.T) {
	cfg := Config{OpenClawHome: t.TempDir(), ClickStateHome: t.TempDir()}
	selection := TargetSelection{Configured: true, OpenClaw: true}

	plan := BuildTargetPlan(cfg, selection, PlanOptions{})

	if got, want := plan.InstallActionKinds(), []StepActionKind{StepActionSyncEngram, StepActionSyncOpenClawWorkspace, StepActionSyncOpenClawMCP, StepActionRegisterOpenClawMCP, StepActionSyncOpenClawPlugin, StepActionSyncOpenClawSkills, StepActionSyncOpenClawModelProfile}; !reflect.DeepEqual(got, want) {
		t.Fatalf("InstallActionKinds() = %#v, want %#v", got, want)
	}
	if got, want := plan.UpdateActionKinds(), []StepActionKind{StepActionSyncEngram, StepActionSyncOpenClawWorkspace, StepActionSyncOpenClawMCP, StepActionRegisterOpenClawMCP, StepActionSyncOpenClawPlugin, StepActionSyncOpenClawSkills, StepActionSyncOpenClawModelProfile}; !reflect.DeepEqual(got, want) {
		t.Fatalf("UpdateActionKinds() = %#v, want %#v", got, want)
	}
	if got, want := plan.UninstallActionKinds(), []StepActionKind{StepActionStripOpenClawWorkspace, StepActionRemoveOpenClawPlugin, StepActionRemoveOpenClawSkills, StepActionUnregisterOpenClawMCP, StepActionRemoveOpenClawModelProfile, StepActionRemoveEngram, StepActionRemoveTargetSelection}; !reflect.DeepEqual(got, want) {
		t.Fatalf("UninstallActionKinds() = %#v, want %#v", got, want)
	}
}

// TestBuildTargetPlan_OpenClawNativeModelFlag_AddsNativeMutationAction is the OpenClaw counterpart:
// the native `openclaw config set` write action only appears when --openclaw-model was passed.
func TestBuildTargetPlan_OpenClawNativeModelFlag_GatesNativeMutationAction(t *testing.T) {
	cfg := Config{OpenClawHome: t.TempDir(), ClickStateHome: t.TempDir()}
	selection := TargetSelection{Configured: true, OpenClaw: true}

	withoutFlag := BuildTargetPlan(cfg, selection, PlanOptions{})
	for _, action := range withoutFlag.InstallActionKinds() {
		if action == StepActionConfigureOpenClawNativeModel {
			t.Fatalf("InstallActionKinds() = %#v, want no native OpenClaw mutation without the flag", withoutFlag.InstallActionKinds())
		}
	}

	withFlag := BuildTargetPlan(cfg, selection, PlanOptions{OpenClawNativeModel: true})
	found := false
	for _, action := range withFlag.InstallActionKinds() {
		if action == StepActionConfigureOpenClawNativeModel {
			found = true
		}
	}
	if !found {
		t.Fatalf("InstallActionKinds() = %#v, want the native OpenClaw mutation when the flag is set", withFlag.InstallActionKinds())
	}
}

// TestBuildTargetPlan_ClaudeExposesModelsAndCloudTeardown closes the two orphaned-file gaps: the
// claude-models step wrote models.json with no reversal at all, and the engram-cloud step re-synced
// engram-cloud.json with no reversal either (RemoveEngramCloudState existed but was reachable only
// from the zero-caller legacy installer.Uninstall). Both files survived `click uninstall` forever.
// Order follows step order, which is what UninstallActionKinds concatenates — there is no separate
// uninstall ordering slice, unlike installActionOrder/updateActionOrder.
func TestBuildTargetPlan_ClaudeExposesModelsAndCloudTeardown(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir(), ClickStateHome: t.TempDir()}
	selection := TargetSelection{Configured: true, Claude: true}

	plan := BuildTargetPlan(cfg, selection, PlanOptions{CloudResolvable: true})

	want := []StepActionKind{
		StepActionRemoveMarketplacePlugins,
		StepActionRemoveModels,
		StepActionRemoveEngram,
		StepActionRemoveEngramCloudState,
		StepActionRemoveContext7,
		StepActionStripClaudeManagedBlock,
		StepActionUnregisterMemoryGuard,
		StepActionRemoveTargetSelection,
	}
	if got := plan.UninstallActionKinds(); !reflect.DeepEqual(got, want) {
		t.Fatalf("UninstallActionKinds() = %#v, want %#v", got, want)
	}

	// The new teardown actions must NOT leak into the forward directions: install/update still write
	// models.json and re-sync the cloud record, they never remove them.
	for _, action := range append(plan.InstallActionKinds(), plan.UpdateActionKinds()...) {
		if action == StepActionRemoveModels || action == StepActionRemoveEngramCloudState {
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
			name:      "Codex with native model configured",
			selection: TargetSelection{Codex: true},
			options:   PlanOptions{CodexNativeModel: true},
			setupFunc: func(t *testing.T) Config {
				return Config{
					CodexHome:      t.TempDir(),
					ClickStateHome: t.TempDir(),
				}
			},
		},
		{
			name:      "OpenClaw with native model configured",
			selection: TargetSelection{OpenClaw: true},
			options:   PlanOptions{OpenClawNativeModel: true},
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
