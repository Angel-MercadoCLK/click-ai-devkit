// Package modelconfig resolves per-phase model selections for click-sdd's five phase agents
// (orchestrator, prd_writer, architect, reviewer, memory_curator), per decision D25: a developer
// may override any phase's model at install time via `click install`'s interactive TUI or by
// accepting the defaults; unset phases always fall back to their default. The resolved map is
// what internal/installer turns into `--config <phase>_model=<alias>` flags on
// `claude plugin install click-sdd@click-ai-devkit`.
package modelconfig

// Phase identifies one of click-sdd's five phase agents that accept a per-phase model override
// via the plugin's userConfig schema (plugins/click-sdd/.claude-plugin/plugin.json).
type Phase string

// ProfileName identifies an orchestration runtime profile. Slice 1 ships only the built-in
// default profile; custom profiles can layer on the same substrate later without changing the base
// SDD phase chain.
type ProfileName string

const (
	PhaseOrchestrator  Phase = "orchestrator"
	PhasePRDWriter     Phase = "prd_writer"
	PhaseArchitect     Phase = "architect"
	PhaseReviewer      Phase = "reviewer"
	PhaseMemoryCurator Phase = "memory_curator"

	ProfileDefault ProfileName = "default"

	ProfileConfigKey = "orchestration_profile"
)

// DelegationPolicy describes what the active orchestration profile expects from the runtime
// orchestrator: simple inline work may stay in the coordinator, but non-trivial work must be handed
// to specialist agents and Engram remains part of the working model.
type DelegationPolicy struct {
	SimpleInlineAllowed         bool     `json:"simple_inline_allowed"`
	EngramRequired              bool     `json:"engram_required"`
	MandatoryDelegationTriggers []string `json:"mandatory_delegation_triggers"`
}

// RuntimeProfile is the resolved profile contract consumed by installed click-sdd instructions.
// It deliberately preserves the existing phase chain; profiles control policy and defaults around
// the phases, not a forked workflow.
type RuntimeProfile struct {
	Name        ProfileName      `json:"name"`
	Title       string           `json:"title"`
	Description string           `json:"description"`
	Models      map[Phase]string `json:"models"`
	Delegation  DelegationPolicy `json:"delegation"`
	PhaseChain  []string         `json:"phase_chain"`
}

// Phases lists all five phases in the fixed order click uses everywhere a stable order matters:
// TUI rows, --config flag emission, and `click doctor` output.
var Phases = []Phase{PhaseOrchestrator, PhasePRDWriter, PhaseArchitect, PhaseReviewer, PhaseMemoryCurator}

// Models are the model aliases click's TUI cycles through per phase, in cycle order.
var Models = []string{"opus", "sonnet", "haiku"}

// Defaults returns D25's default model per phase: opus for orchestrator/prd_writer/architect/
// reviewer, sonnet for memory_curator. It always returns a fresh map so callers can mutate their
// copy without affecting later calls.
func Defaults() map[Phase]string {
	return map[Phase]string{
		PhaseOrchestrator:  "opus",
		PhasePRDWriter:     "opus",
		PhaseArchitect:     "opus",
		PhaseReviewer:      "opus",
		PhaseMemoryCurator: "sonnet",
	}
}

// Resolve merges overrides onto Defaults(): any known phase with a non-empty value in overrides
// wins, every other phase keeps its default. Empty-string values and unknown phase keys in
// overrides are ignored, so callers can pass a partial or even user-tainted map safely. The
// returned map is always a fresh copy — it never aliases overrides or a cached Defaults() map.
func Resolve(overrides map[Phase]string) map[Phase]string {
	return ResolveForProfile(DefaultProfile(), overrides)
}

// ResolveForProfile merges per-phase overrides onto the active profile's model defaults. Unknown
// phases and empty override values are ignored, so callers can safely pass partial persisted state
// while preserving the profile as the first-class source of defaults.
func ResolveForProfile(profile RuntimeProfile, overrides map[Phase]string) map[Phase]string {
	resolved := copyModels(profile.Models)
	if len(resolved) == 0 {
		resolved = Defaults()
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

// Profiles returns the built-in profiles available for installation/configuration. Slice 2 exposes
// only the default profile as selectable UX; custom profiles can be added later without changing the
// base phase model.
func Profiles() []RuntimeProfile {
	return []RuntimeProfile{DefaultProfile()}
}

// DefaultProfile returns the built-in runtime profile. It is intentionally conservative: it keeps
// D25's model defaults, preserves the established phase chain, allows only small inline responses,
// and marks Engram as required context for durable knowledge and progress.
func DefaultProfile() RuntimeProfile {
	return RuntimeProfile{
		Name:        ProfileDefault,
		Title:       "Default Click SDD profile",
		Description: "Preserves the standard Click SDD phase chain while enforcing delegation for non-trivial work and keeping Engram integrated.",
		Models:      Defaults(),
		Delegation: DelegationPolicy{
			SimpleInlineAllowed: true,
			EngramRequired:      true,
			MandatoryDelegationTriggers: []string{
				"broad_exploration",
				"multi_file_implementation",
				"test_or_tool_execution",
				"review",
				"context_expansion",
			},
		},
		PhaseChain: []string{"explore", "prd", "design", "tasks", "code", "review", "memory"},
	}
}

// ResolveProfile returns the named built-in profile, falling back to default for empty or unknown
// names. This gives the runtime a first-class profile resolution step now while keeping custom
// profile loading out of Slice 1.
func ResolveProfile(name string) RuntimeProfile {
	switch ProfileName(name) {
	case "", ProfileDefault:
		return DefaultProfile()
	default:
		return DefaultProfile()
	}
}

func copyModels(models map[Phase]string) map[Phase]string {
	copy := make(map[Phase]string, len(models))
	for phase, model := range models {
		copy[phase] = model
	}
	return copy
}

// ConfigKey returns the plugin.json userConfig field name for this phase (e.g.
// "orchestrator_model"), which is also the key half of the `--config key=value` flag click passes
// to `claude plugin install click-sdd@click-ai-devkit`.
func (p Phase) ConfigKey() string {
	return string(p) + "_model"
}
