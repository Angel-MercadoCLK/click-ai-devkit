package modelconfig

import (
	"reflect"
	"testing"
)

// TestProfiles_BalancedEqualsDefaults guards the single most important profile invariant (design
// D1): the "balanced" preset must be Defaults() verbatim — not a hand-copied table that can drift.
func TestProfiles_BalancedEqualsDefaults(t *testing.T) {
	profiles := Profiles()
	for _, p := range profiles {
		if p.Name != ProfileBalanced {
			continue
		}
		if !reflect.DeepEqual(p.Models, Defaults()) {
			t.Fatalf("Profiles() balanced.Models = %#v, want Defaults() %#v", p.Models, Defaults())
		}
		return
	}
	t.Fatal("Profiles() did not contain a \"balanced\" profile")
}

// TestProfiles_EveryPresetHasAllThirteenPhases guards against a preset table silently missing a
// phase (e.g. a copy-paste table that only re-keys 5 phases instead of all 13).
func TestProfiles_EveryPresetHasAllThirteenPhases(t *testing.T) {
	for _, p := range Profiles() {
		if len(p.Models) != len(Phases) {
			t.Fatalf("Profiles(): profile %q has %d phases, want %d", p.Name, len(p.Models), len(Phases))
		}
		for _, phase := range Phases {
			if _, ok := p.Models[phase]; !ok {
				t.Fatalf("Profiles(): profile %q is missing phase %q", p.Name, phase)
			}
		}
	}
}

// TestProfiles_ContainsExpectedNames guards the three built-in presets the design calls for.
func TestProfiles_ContainsExpectedNames(t *testing.T) {
	want := []ProfileName{ProfileBalanced, ProfileCostSaver, ProfileQuality}
	var got []ProfileName
	for _, p := range Profiles() {
		got = append(got, p.Name)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Profiles() names = %#v, want %#v", got, want)
	}
}

// TestProfiles_CostSaverKeepsOpusOnlyOnHighestStakesPhases pins the exact cost-saver table: haiku
// everywhere except the three phases Defaults() itself already deems architecturally-heavy
// (propose, design, verify) — i.e. cost-saver never downgrades below what balanced already
// considers essential.
func TestProfiles_CostSaverKeepsOpusOnlyOnHighestStakesPhases(t *testing.T) {
	want := map[Phase]string{
		PhaseExplore:    "haiku",
		PhasePropose:    "opus",
		PhaseSpec:       "haiku",
		PhaseDesign:     "opus",
		PhaseTasks:      "haiku",
		PhaseApply:      "haiku",
		PhaseVerify:     "opus",
		PhaseArchive:    "haiku",
		PhaseOnboard:    "haiku",
		PhaseJDJudgeA:   "haiku",
		PhaseJDJudgeB:   "haiku",
		PhaseJDFixAgent: "haiku",
		PhaseDefault:    "haiku",
	}
	got := ResolveProfile(string(ProfileCostSaver)).Models
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("cost-saver Models = %#v, want %#v", got, want)
	}
}

// TestProfiles_QualityKeepsHaikuOnlyWhereDefaultsAlreadyDoes pins the exact quality table: opus
// everywhere except the two phases Defaults() itself already deems cheap/mechanical (archive,
// onboard) — i.e. quality never spends more than balanced already considers unnecessary.
func TestProfiles_QualityKeepsHaikuOnlyWhereDefaultsAlreadyDoes(t *testing.T) {
	want := map[Phase]string{
		PhaseExplore:    "opus",
		PhasePropose:    "opus",
		PhaseSpec:       "opus",
		PhaseDesign:     "opus",
		PhaseTasks:      "opus",
		PhaseApply:      "opus",
		PhaseVerify:     "opus",
		PhaseArchive:    "haiku",
		PhaseOnboard:    "haiku",
		PhaseJDJudgeA:   "opus",
		PhaseJDJudgeB:   "opus",
		PhaseJDFixAgent: "opus",
		PhaseDefault:    "opus",
	}
	got := ResolveProfile(string(ProfileQuality)).Models
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("quality Models = %#v, want %#v", got, want)
	}
}

func TestResolveProfile_UnknownOrEmptyOrCustom_FallsBackToBalanced(t *testing.T) {
	tests := []string{"", "not-a-real-profile", string(ProfileCustom)}
	for _, name := range tests {
		t.Run(name, func(t *testing.T) {
			got := ResolveProfile(name)
			if got.Name != ProfileBalanced {
				t.Fatalf("ResolveProfile(%q).Name = %q, want %q", name, got.Name, ProfileBalanced)
			}
			if !reflect.DeepEqual(got.Models, Defaults()) {
				t.Fatalf("ResolveProfile(%q).Models = %#v, want Defaults() %#v", name, got.Models, Defaults())
			}
		})
	}
}

func TestResolveProfile_KnownName_ReturnsThatProfile(t *testing.T) {
	got := ResolveProfile(string(ProfileCostSaver))
	if got.Name != ProfileCostSaver {
		t.Fatalf("ResolveProfile(%q).Name = %q, want %q", ProfileCostSaver, got.Name, ProfileCostSaver)
	}
	if got.Models[PhaseExplore] != "haiku" {
		t.Fatalf("ResolveProfile(cost-saver).Models[explore] = %q, want %q", got.Models[PhaseExplore], "haiku")
	}
}

func TestResolveForProfile_NoOverrides_ReturnsProfileModelsVerbatim(t *testing.T) {
	got := ResolveForProfile(string(ProfileQuality), nil)
	want := ResolveProfile(string(ProfileQuality)).Models
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ResolveForProfile(quality, nil) = %#v, want %#v", got, want)
	}
}

func TestResolveForProfile_OverridesLayerOnTopOfProfile(t *testing.T) {
	got := ResolveForProfile(string(ProfileCostSaver), map[Phase]string{
		PhaseDesign: "sonnet", // override cost-saver's opus for design
	})
	if got[PhaseDesign] != "sonnet" {
		t.Fatalf("ResolveForProfile(cost-saver, {design:sonnet})[design] = %q, want %q", got[PhaseDesign], "sonnet")
	}
	// every other phase must still come from the cost-saver base, unaffected by the single override
	if got[PhaseExplore] != "haiku" {
		t.Fatalf("ResolveForProfile(cost-saver, {design:sonnet})[explore] = %q, want %q (untouched cost-saver base)", got[PhaseExplore], "haiku")
	}
}

func TestResolveForProfile_UnknownOverrideKeyDropped(t *testing.T) {
	got := ResolveForProfile(string(ProfileBalanced), map[Phase]string{
		Phase("not_a_real_phase"): "opus",
	})
	if !reflect.DeepEqual(got, Defaults()) {
		t.Fatalf("ResolveForProfile(balanced, unknown-key) = %#v, want Defaults() %#v (unknown key must be dropped)", got, Defaults())
	}
}

func TestResolveForProfile_EmptyStringOverrideIgnored(t *testing.T) {
	got := ResolveForProfile(string(ProfileQuality), map[Phase]string{
		PhaseDesign: "",
	})
	want := ResolveProfile(string(ProfileQuality)).Models
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ResolveForProfile(quality, {design:\"\"}) = %#v, want unmodified profile %#v", got, want)
	}
}

func TestResolveForProfile_EveryPresetReturnsAllThirteenPhases(t *testing.T) {
	for _, p := range Profiles() {
		got := ResolveForProfile(string(p.Name), nil)
		if len(got) != len(Phases) {
			t.Fatalf("ResolveForProfile(%q, nil) has %d phases, want %d", p.Name, len(got), len(Phases))
		}
		for _, phase := range Phases {
			if got[phase] == "" {
				t.Fatalf("ResolveForProfile(%q, nil)[%q] is empty, want a non-empty model alias", p.Name, phase)
			}
		}
	}
}
