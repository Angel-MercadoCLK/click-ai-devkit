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
	want := []string{"Codex CLI", "Codex native model", "SDD assets"}
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

	if got, want := plan.InstallActionKinds(), []StepActionKind{StepActionSyncCodexGuidance, StepActionConfigureCodexNativeModel}; !reflect.DeepEqual(got, want) {
		t.Fatalf("InstallActionKinds() = %#v, want %#v", got, want)
	}
	if got, want := plan.UpdateActionKinds(), []StepActionKind{StepActionSyncCodexGuidance, StepActionConfigureCodexNativeModel}; !reflect.DeepEqual(got, want) {
		t.Fatalf("UpdateActionKinds() = %#v, want %#v", got, want)
	}
	if got, want := plan.UninstallActionKinds(), []StepActionKind{StepActionStripCodexGuidance, StepActionRemoveTargetSelection}; !reflect.DeepEqual(got, want) {
		t.Fatalf("UninstallActionKinds() = %#v, want %#v", got, want)
	}
	if got, want := plan.DoctorCheckKinds(), []DoctorCheckKind{DoctorCheckCodexGuidance}; !reflect.DeepEqual(got, want) {
		t.Fatalf("DoctorCheckKinds() = %#v, want %#v", got, want)
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
