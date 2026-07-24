package installer

import (
	"reflect"
	"strings"
	"testing"
)

func TestBuildTargetPlan_TargetFirstOrderAndSharedProjections(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir(), CodexHome: t.TempDir(), OpenClawHome: t.TempDir(), ClickStateHome: t.TempDir()}
	selection := TargetSelection{Configured: true, Claude: true, Codex: true, OpenClaw: true}

	plan := BuildTargetPlan(cfg, selection, PlanOptions{CloudConfigured: true})

	if got, want := plan.Selection, selection; got != want {
		t.Fatalf("Selection = %+v, want %+v", got, want)
	}
	got := plan.StepLabels()
	want := []string{
		"Claude Code",
		"Claude model/profile",
		"Codex CLI",
		"Codex Engram MCP",
		"Codex native model",
		"OpenClaw",
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
	want := []string{"Codex CLI", "Codex Engram MCP", "Codex native model", "SDD assets"}
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
	if got, want := plan.InstallActionKinds(), []StepActionKind{StepActionSyncCodexGuidance, StepActionSyncCodexMCP}; !reflect.DeepEqual(got, want) {
		t.Fatalf("InstallActionKinds() = %#v, want %#v", got, want)
	}
	if got, want := plan.UpdateActionKinds(), []StepActionKind{StepActionSyncCodexGuidance, StepActionSyncCodexMCP}; !reflect.DeepEqual(got, want) {
		t.Fatalf("UpdateActionKinds() = %#v, want %#v", got, want)
	}
	// Uninstall reverses the managed AGENTS.md block (StripCodexGuidance) AND deregisters Engram's
	// Codex MCP server (StepActionRemoveCodexMCP — SyncCodexMCP's reversal), then removes the neutral
	// target selection. The native config.toml model is deliberately NOT reverted (user-owned).
	if got, want := plan.UninstallActionKinds(), []StepActionKind{StepActionStripCodexGuidance, StepActionRemoveCodexMCP, StepActionRemoveTargetSelection}; !reflect.DeepEqual(got, want) {
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

	if got, want := plan.InstallActionKinds(), []StepActionKind{StepActionSyncCodexGuidance, StepActionSyncCodexMCP, StepActionConfigureCodexNativeModel}; !reflect.DeepEqual(got, want) {
		t.Fatalf("InstallActionKinds() = %#v, want %#v", got, want)
	}
	if got, want := plan.UpdateActionKinds(), []StepActionKind{StepActionSyncCodexGuidance, StepActionSyncCodexMCP, StepActionConfigureCodexNativeModel}; !reflect.DeepEqual(got, want) {
		t.Fatalf("UpdateActionKinds() = %#v, want %#v", got, want)
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

func sliceContains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
