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

const (
	PhaseOrchestrator  Phase = "orchestrator"
	PhasePRDWriter     Phase = "prd_writer"
	PhaseArchitect     Phase = "architect"
	PhaseReviewer      Phase = "reviewer"
	PhaseMemoryCurator Phase = "memory_curator"
)

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
	resolved := Defaults()
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

// ConfigKey returns the plugin.json userConfig field name for this phase (e.g.
// "orchestrator_model"), which is also the key half of the `--config key=value` flag click passes
// to `claude plugin install click-sdd@click-ai-devkit`.
func (p Phase) ConfigKey() string {
	return string(p) + "_model"
}
