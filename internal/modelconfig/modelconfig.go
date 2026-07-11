// Package modelconfig resolves per-phase model selections for the real SDD phase taxonomy the
// click-sdd plugin and the wider gentle-ai SDD ecosystem use — the 13 phases exercised by
// `/sdd-explore`, `/sdd-propose`, `/sdd-spec`, `/sdd-design`, `/sdd-tasks`, `/sdd-apply`,
// `/sdd-verify`, `/sdd-archive`, `/sdd-onboard`, and Judgment Day's two blind judges + fix agent,
// plus a `default` fallback for any delegation not covered by a specific phase. A developer may
// override any phase's model at install time via `click install`'s interactive TUI or by accepting
// the defaults; unset phases always fall back to their default. The resolved map is what
// internal/installer turns into `--config <phase>_model=<alias>` flags on
// `claude plugin install click-sdd@click-ai-devkit`.
//
// This package previously modeled an invented 5-phase taxonomy (orchestrator, prd_writer,
// architect, reviewer, memory_curator) that never matched the real SDD phase set used by
// click-sdd's own skills. That taxonomy has been fully replaced — see the interactive-menu-and-
// model-taxonomy change.
package modelconfig

// Phase identifies one of the real SDD phase agents that accept a per-phase model override via the
// plugin's userConfig schema (plugins/click-sdd/.claude-plugin/plugin.json).
type Phase string

const (
	PhaseExplore    Phase = "explore"
	PhasePropose    Phase = "propose"
	PhaseSpec       Phase = "spec"
	PhaseDesign     Phase = "design"
	PhaseTasks      Phase = "tasks"
	PhaseApply      Phase = "apply"
	PhaseVerify     Phase = "verify"
	PhaseArchive    Phase = "archive"
	PhaseOnboard    Phase = "onboard"
	PhaseJDJudgeA   Phase = "jd-judge-a"
	PhaseJDJudgeB   Phase = "jd-judge-b"
	PhaseJDFixAgent Phase = "jd-fix-agent"
	PhaseDefault    Phase = "default"
)

// Phases lists all thirteen phases in the fixed order click uses everywhere a stable order
// matters: TUI rows, --config flag emission, and `click doctor` output.
var Phases = []Phase{
	PhaseExplore, PhasePropose, PhaseSpec, PhaseDesign, PhaseTasks, PhaseApply, PhaseVerify,
	PhaseArchive, PhaseOnboard, PhaseJDJudgeA, PhaseJDJudgeB, PhaseJDFixAgent, PhaseDefault,
}

// Models are the model aliases click's TUI cycles through per phase, in cycle order.
var Models = []string{"opus", "sonnet", "haiku"}

// Defaults returns the default model per phase: opus for the architecturally-heavy phases
// (propose, design, verify), haiku for the cheap/mechanical phases (archive, onboard), and sonnet
// for every other phase (explore, spec, tasks, apply, jd-judge-a, jd-judge-b, jd-fix-agent,
// default). It always returns a fresh map so callers can mutate their copy without affecting later
// calls.
func Defaults() map[Phase]string {
	return map[Phase]string{
		PhaseExplore:    "sonnet",
		PhasePropose:    "opus",
		PhaseSpec:       "sonnet",
		PhaseDesign:     "opus",
		PhaseTasks:      "sonnet",
		PhaseApply:      "sonnet",
		PhaseVerify:     "opus",
		PhaseArchive:    "haiku",
		PhaseOnboard:    "haiku",
		PhaseJDJudgeA:   "sonnet",
		PhaseJDJudgeB:   "sonnet",
		PhaseJDFixAgent: "sonnet",
		PhaseDefault:    "sonnet",
	}
}

// Resolve merges overrides onto Defaults(): any known phase with a non-empty value in overrides
// wins, every other phase keeps its default. Empty-string values are ignored, so callers can pass a
// partial map safely. Unknown phase keys — including the old, pre-realignment taxonomy
// (orchestrator, prd_writer, architect, reviewer, memory_curator) a stale caller or a leftover
// on-disk config might still carry — are silently dropped rather than treated as valid overrides.
// The returned map is always a fresh copy — it never aliases overrides or a cached Defaults() map.
func Resolve(overrides map[Phase]string) map[Phase]string {
	return resolveOnto(Defaults(), overrides)
}

// ConfigKey returns the plugin.json userConfig field name for this phase (e.g. "apply_model" or
// "jd_judge_a_model" — hyphens become underscores), which is also the key half of the
// `--config key=value` flag click passes to `claude plugin install click-sdd@click-ai-devkit`.
func (p Phase) ConfigKey() string {
	key := make([]byte, 0, len(p)+len("_model"))
	for i := 0; i < len(p); i++ {
		c := p[i]
		if c == '-' {
			c = '_'
		}
		key = append(key, c)
	}
	key = append(key, "_model"...)
	return string(key)
}
