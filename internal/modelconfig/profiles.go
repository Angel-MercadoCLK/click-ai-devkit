package modelconfig

// ProfileName identifies one of the built-in orchestration profiles a developer can select at
// install time instead of tuning all 13 phases by hand, or "custom" for a hand-tuned selection
// that doesn't match any built-in preset.
type ProfileName string

const (
	// ProfileBalanced is the default profile: its Models map is Defaults() verbatim.
	ProfileBalanced ProfileName = "balanced"
	// ProfileCostSaver favors haiku everywhere except the phases Defaults() itself already
	// treats as architecturally heavy (propose, design, verify), which stay on opus.
	ProfileCostSaver ProfileName = "cost-saver"
	// ProfileQuality favors opus everywhere except the phases Defaults() itself already treats
	// as cheap/mechanical (archive, onboard), which stay on haiku.
	ProfileQuality ProfileName = "quality"
	// ProfileCustom marks a hand-tuned per-phase selection that doesn't match any built-in
	// preset. It has no Models map of its own — ResolveProfile falls back to balanced for it,
	// since a custom selection's actual values live entirely in the caller's overrides.
	ProfileCustom ProfileName = "custom"
)

// ProfileConfigKey is the plugin.json userConfig field name (and the `--config` flag key) that
// carries the active profile name, alongside the 13 per-phase `<phase>_model` keys.
const ProfileConfigKey = "orchestration_profile"

// RuntimeProfile is a named, fully-resolved per-phase model map.
type RuntimeProfile struct {
	Name   ProfileName
	Models map[Phase]string
}

// Profiles returns the built-in profiles, in a fixed order (balanced, cost-saver, quality).
// "custom" is deliberately not included: it has no preset Models map of its own.
func Profiles() []RuntimeProfile {
	return []RuntimeProfile{
		{Name: ProfileBalanced, Models: Defaults()},
		{Name: ProfileCostSaver, Models: costSaverDefaults()},
		{Name: ProfileQuality, Models: qualityDefaults()},
	}
}

// costSaverDefaults returns the cost-saver preset: haiku for every phase except the three
// Defaults() already assigns opus (propose, design, verify) — cost-saver never downgrades below
// what balanced itself already deems essential.
func costSaverDefaults() map[Phase]string {
	return map[Phase]string{
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
}

// qualityDefaults returns the quality preset: opus for every phase except the two Defaults()
// already assigns haiku (archive, onboard) — quality never spends more than balanced itself
// already deems unnecessary.
func qualityDefaults() map[Phase]string {
	return map[Phase]string{
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
}

// ResolveProfile returns the named built-in profile. An empty, unknown, or "custom" name all fall
// back to the balanced profile: custom has no preset Models map of its own, its per-phase choices
// live entirely in the caller's overrides (see ResolveForProfile).
func ResolveProfile(name string) RuntimeProfile {
	for _, p := range Profiles() {
		if string(p.Name) == name {
			return p
		}
	}
	return RuntimeProfile{Name: ProfileBalanced, Models: Defaults()}
}

// ResolveForProfile resolves the full 13-phase model map for the named profile (falling back to
// balanced per ResolveProfile's rules), then layers per-phase overrides on top using the same
// semantics Resolve uses against Defaults(): empty-string values and unknown phase keys in
// overrides are silently dropped, every other phase keeps the profile's value.
func ResolveForProfile(name string, overrides map[Phase]string) map[Phase]string {
	return resolveOnto(ResolveProfile(name).Models, overrides)
}

// resolveOnto merges overrides onto base: any known phase with a non-empty value in overrides
// wins, every other phase keeps base's value. Empty-string values are ignored, and unknown phase
// keys are silently dropped. The returned map is always a fresh copy — it never aliases base or
// overrides.
func resolveOnto(base map[Phase]string, overrides map[Phase]string) map[Phase]string {
	resolved := make(map[Phase]string, len(base))
	for phase, model := range base {
		resolved[phase] = model
	}
	for phase, model := range overrides {
		if model == "" {
			continue
		}
		if _, known := resolved[phase]; !known {
			continue
		}
		resolved[phase] = model
	}
	return resolved
}
