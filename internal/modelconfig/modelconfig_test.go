package modelconfig

import (
	"reflect"
	"testing"
)

// wantDefaults mirrors the real 18-phase SDD taxonomy's default model assignment: opus for the
// architecturally-heavy phases (propose, design, verify), haiku for the cheap/mechanical phases
// (archive, onboard), sonnet for everything else.
func wantDefaults() map[Phase]string {
	return map[Phase]string{
		PhaseExplore:           "sonnet",
		PhasePropose:           "opus",
		PhaseSpec:              "sonnet",
		PhaseDesign:            "opus",
		PhaseTasks:             "sonnet",
		PhaseApply:             "sonnet",
		PhaseVerify:            "opus",
		PhaseArchive:           "haiku",
		PhaseOnboard:           "haiku",
		PhaseJDJudgeA:          "sonnet",
		PhaseJDJudgeB:          "sonnet",
		PhaseJDFixAgent:        "sonnet",
		PhaseReviewRisk:        "sonnet",
		PhaseReviewReadability: "sonnet",
		PhaseReviewReliability: "sonnet",
		PhaseReviewResilience:  "sonnet",
		PhaseReviewRefuter:     "sonnet",
		PhaseDefault:           "sonnet",
	}
}

func TestDefaults_MatchesRealTaxonomy(t *testing.T) {
	if got := Defaults(); !reflect.DeepEqual(got, wantDefaults()) {
		t.Fatalf("Defaults() = %#v, want %#v", got, wantDefaults())
	}
}

func TestDefaults_ReturnsFreshMapEachCall(t *testing.T) {
	a := Defaults()
	a[PhasePropose] = "haiku"
	b := Defaults()
	if b[PhasePropose] != "opus" {
		t.Fatalf("Defaults() mutation leaked across calls: b[propose] = %q, want %q", b[PhasePropose], "opus")
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
				PhaseApply: "haiku",
			},
			want: func() map[Phase]string {
				w := wantDefaults()
				w[PhaseApply] = "haiku"
				return w
			}(),
		},
		{
			name: "empty-string override is ignored, default kept",
			overrides: map[Phase]string{
				PhaseApply: "",
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
		{
			// D-migration: Resolve must silently drop old (pre-realignment) phase keys instead of
			// treating them as valid overrides — a stale caller passing the invented 5-phase
			// taxonomy must not corrupt the resolved map.
			name: "old-taxonomy phase keys are ignored",
			overrides: map[Phase]string{
				Phase("orchestrator"):   "opus",
				Phase("prd_writer"):     "opus",
				Phase("architect"):      "opus",
				Phase("reviewer"):       "opus",
				Phase("memory_curator"): "sonnet",
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
	overrides := map[Phase]string{PhaseApply: "haiku"}
	_ = Resolve(overrides)
	again := Resolve(nil)
	if again[PhaseApply] != "sonnet" {
		t.Fatalf("Resolve(nil) after a prior override call = %q, want default %q", again[PhaseApply], "sonnet")
	}
}

func TestPhases_FixedOrder(t *testing.T) {
	want := []Phase{
		PhaseExplore, PhasePropose, PhaseSpec, PhaseDesign, PhaseTasks, PhaseApply, PhaseVerify,
		PhaseArchive, PhaseOnboard, PhaseJDJudgeA, PhaseJDJudgeB, PhaseJDFixAgent,
		PhaseReviewRisk, PhaseReviewReadability, PhaseReviewReliability, PhaseReviewResilience,
		PhaseReviewRefuter, PhaseDefault,
	}
	if !reflect.DeepEqual(Phases, want) {
		t.Fatalf("Phases = %#v, want %#v", Phases, want)
	}
}

func TestPhases_HasEighteenPhases(t *testing.T) {
	if got := len(Phases); got != 18 {
		t.Fatalf("len(Phases) = %d, want 18", got)
	}
}

func TestPhase_ConfigKey(t *testing.T) {
	tests := []struct {
		phase Phase
		want  string
	}{
		{PhaseExplore, "explore_model"},
		{PhasePropose, "propose_model"},
		{PhaseSpec, "spec_model"},
		{PhaseDesign, "design_model"},
		{PhaseTasks, "tasks_model"},
		{PhaseApply, "apply_model"},
		{PhaseVerify, "verify_model"},
		{PhaseArchive, "archive_model"},
		{PhaseOnboard, "onboard_model"},
		{PhaseJDJudgeA, "jd_judge_a_model"},
		{PhaseJDJudgeB, "jd_judge_b_model"},
		{PhaseJDFixAgent, "jd_fix_agent_model"},
		{PhaseReviewRisk, "review_risk_model"},
		{PhaseReviewReadability, "review_readability_model"},
		{PhaseReviewReliability, "review_reliability_model"},
		{PhaseReviewResilience, "review_resilience_model"},
		{PhaseReviewRefuter, "review_refuter_model"},
		{PhaseDefault, "default_model"},
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
