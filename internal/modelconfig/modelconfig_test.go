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
