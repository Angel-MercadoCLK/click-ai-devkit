package modelconfig

import (
	"reflect"
	"testing"
)

func TestDefaults_MatchesD25(t *testing.T) {
	want := map[Phase]string{
		PhaseOrchestrator:  "opus",
		PhasePRDWriter:     "opus",
		PhaseArchitect:     "opus",
		PhaseReviewer:      "opus",
		PhaseMemoryCurator: "sonnet",
	}
	if got := Defaults(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Defaults() = %#v, want %#v", got, want)
	}
}

func TestDefaults_ReturnsFreshMapEachCall(t *testing.T) {
	a := Defaults()
	a[PhaseOrchestrator] = "haiku"
	b := Defaults()
	if b[PhaseOrchestrator] != "opus" {
		t.Fatalf("Defaults() mutation leaked across calls: b[orchestrator] = %q, want %q", b[PhaseOrchestrator], "opus")
	}
}

func TestResolve(t *testing.T) {
	tests := []struct {
		name      string
		overrides map[Phase]string
		want      map[Phase]string
	}{
		{
			name:      "nil overrides returns defaults",
			overrides: nil,
			want:      Defaults(),
		},
		{
			name:      "empty overrides returns defaults",
			overrides: map[Phase]string{},
			want:      Defaults(),
		},
		{
			name: "single override wins, others keep default",
			overrides: map[Phase]string{
				PhaseOrchestrator: "haiku",
			},
			want: map[Phase]string{
				PhaseOrchestrator:  "haiku",
				PhasePRDWriter:     "opus",
				PhaseArchitect:     "opus",
				PhaseReviewer:      "opus",
				PhaseMemoryCurator: "sonnet",
			},
		},
		{
			name: "all five overridden",
			overrides: map[Phase]string{
				PhaseOrchestrator:  "sonnet",
				PhasePRDWriter:     "haiku",
				PhaseArchitect:     "sonnet",
				PhaseReviewer:      "haiku",
				PhaseMemoryCurator: "opus",
			},
			want: map[Phase]string{
				PhaseOrchestrator:  "sonnet",
				PhasePRDWriter:     "haiku",
				PhaseArchitect:     "sonnet",
				PhaseReviewer:      "haiku",
				PhaseMemoryCurator: "opus",
			},
		},
		{
			name: "empty-string override is ignored, default kept",
			overrides: map[Phase]string{
				PhaseOrchestrator: "",
			},
			want: Defaults(),
		},
		{
			name: "unknown phase key in overrides is ignored",
			overrides: map[Phase]string{
				Phase("not_a_real_phase"): "opus",
			},
			want: Defaults(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Resolve(tt.overrides)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("Resolve(%#v) = %#v, want %#v", tt.overrides, got, tt.want)
			}
		})
	}
}

func TestResolve_DoesNotMutateOverridesOrLeakIntoLaterCalls(t *testing.T) {
	overrides := map[Phase]string{PhaseOrchestrator: "haiku"}
	_ = Resolve(overrides)
	again := Resolve(nil)
	if again[PhaseOrchestrator] != "opus" {
		t.Fatalf("Resolve(nil) after a prior override call = %q, want default %q", again[PhaseOrchestrator], "opus")
	}
}

func TestResolveForProfile_UsesProfileDefaultsBeforeOverrides(t *testing.T) {
	profile := DefaultProfile()
	profile.Models = map[Phase]string{
		PhaseOrchestrator:  "sonnet",
		PhasePRDWriter:     "sonnet",
		PhaseArchitect:     "haiku",
		PhaseReviewer:      "haiku",
		PhaseMemoryCurator: "opus",
	}
	overrides := map[Phase]string{
		PhaseArchitect:       "opus",
		Phase("not_a_phase"): "sonnet",
		PhaseReviewer:        "",
	}

	got := ResolveForProfile(profile, overrides)
	want := map[Phase]string{
		PhaseOrchestrator:  "sonnet",
		PhasePRDWriter:     "sonnet",
		PhaseArchitect:     "opus",
		PhaseReviewer:      "haiku",
		PhaseMemoryCurator: "opus",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ResolveForProfile(profile, overrides) = %#v, want %#v", got, want)
	}
}

func TestProfiles_ReturnsBuiltInDefaultProfile(t *testing.T) {
	profiles := Profiles()
	if len(profiles) != 1 {
		t.Fatalf("Profiles() returned %d profiles, want exactly the built-in default profile", len(profiles))
	}
	if profiles[0].Name != ProfileDefault {
		t.Fatalf("Profiles()[0].Name = %q, want %q", profiles[0].Name, ProfileDefault)
	}

	profiles[0].Models[PhaseOrchestrator] = "haiku"
	again := Profiles()
	if again[0].Models[PhaseOrchestrator] != "opus" {
		t.Fatalf("Profiles() leaked model mutation: got %q, want opus", again[0].Models[PhaseOrchestrator])
	}
}

func TestPhases_FixedOrder(t *testing.T) {
	want := []Phase{PhaseOrchestrator, PhasePRDWriter, PhaseArchitect, PhaseReviewer, PhaseMemoryCurator}
	if !reflect.DeepEqual(Phases, want) {
		t.Fatalf("Phases = %#v, want %#v", Phases, want)
	}
}

func TestPhase_ConfigKey(t *testing.T) {
	tests := []struct {
		phase Phase
		want  string
	}{
		{PhaseOrchestrator, "orchestrator_model"},
		{PhasePRDWriter, "prd_writer_model"},
		{PhaseArchitect, "architect_model"},
		{PhaseReviewer, "reviewer_model"},
		{PhaseMemoryCurator, "memory_curator_model"},
	}
	for _, tt := range tests {
		if got := tt.phase.ConfigKey(); got != tt.want {
			t.Errorf("Phase(%q).ConfigKey() = %q, want %q", tt.phase, got, tt.want)
		}
	}
}

func TestModels_ContainsExpectedCycleOptions(t *testing.T) {
	want := []string{"opus", "sonnet", "haiku"}
	if !reflect.DeepEqual(Models, want) {
		t.Fatalf("Models = %#v, want %#v", Models, want)
	}
}

func TestDefaultProfile_DefinesRuntimePolicySubstrate(t *testing.T) {
	profile := DefaultProfile()

	if profile.Name != ProfileDefault {
		t.Fatalf("DefaultProfile().Name = %q, want %q", profile.Name, ProfileDefault)
	}
	if !reflect.DeepEqual(profile.Models, Defaults()) {
		t.Fatalf("DefaultProfile().Models = %#v, want defaults %#v", profile.Models, Defaults())
	}
	if !profile.Delegation.SimpleInlineAllowed {
		t.Fatal("DefaultProfile().Delegation.SimpleInlineAllowed = false, want true")
	}
	if !profile.Delegation.EngramRequired {
		t.Fatal("DefaultProfile().Delegation.EngramRequired = false, want true")
	}

	wantTriggers := []string{
		"broad_exploration",
		"multi_file_implementation",
		"test_or_tool_execution",
		"review",
		"context_expansion",
	}
	if !reflect.DeepEqual(profile.Delegation.MandatoryDelegationTriggers, wantTriggers) {
		t.Fatalf("DefaultProfile().Delegation.MandatoryDelegationTriggers = %#v, want %#v", profile.Delegation.MandatoryDelegationTriggers, wantTriggers)
	}

	wantChain := []string{"explore", "prd", "design", "tasks", "code", "review", "memory"}
	if !reflect.DeepEqual(profile.PhaseChain, wantChain) {
		t.Fatalf("DefaultProfile().PhaseChain = %#v, want %#v", profile.PhaseChain, wantChain)
	}
}

func TestResolveProfile_DefaultAndUnknownFallback(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want ProfileName
	}{
		{name: "empty resolves to default", in: "", want: ProfileDefault},
		{name: "default resolves to default", in: "default", want: ProfileDefault},
		{name: "unknown resolves to default", in: "custom-later", want: ProfileDefault},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveProfile(tt.in)
			if got.Name != tt.want {
				t.Fatalf("ResolveProfile(%q).Name = %q, want %q", tt.in, got.Name, tt.want)
			}
		})
	}
}

func TestDefaultProfile_ReturnsFreshCopies(t *testing.T) {
	first := DefaultProfile()
	first.Models[PhaseOrchestrator] = "haiku"
	first.Delegation.MandatoryDelegationTriggers[0] = "mutated"
	first.PhaseChain[0] = "mutated"

	second := DefaultProfile()
	if second.Models[PhaseOrchestrator] != "opus" {
		t.Fatalf("DefaultProfile() model mutation leaked: got %q, want opus", second.Models[PhaseOrchestrator])
	}
	if second.Delegation.MandatoryDelegationTriggers[0] != "broad_exploration" {
		t.Fatalf("DefaultProfile() trigger mutation leaked: got %q", second.Delegation.MandatoryDelegationTriggers[0])
	}
	if second.PhaseChain[0] != "explore" {
		t.Fatalf("DefaultProfile() phase-chain mutation leaked: got %q", second.PhaseChain[0])
	}
}
