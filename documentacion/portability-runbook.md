# click-sdd Standalone Portability — Manual Runbook

> Status: v1.0. Companion to `internal/installer/portability_test.go` and the
> `click-sdd-standalone-portability` change. Go cannot drive a live Claude Code session, so this
> runbook is the manual complement to the static structural/portability Go tests: it proves the
> *behavioral* claim (every SDD role actually resolves and runs from click assets alone) that no Go
> test can assert on its own.

## Purpose

Confirm that a full SDD cycle — `explore -> propose -> spec/design -> tasks -> apply -> verify ->
archive`, plus Judgment Day and the 4R review lenses — completes end to end on a machine with
**no gentle-ai installation present**, using only the `click-ai-devkit` marketplace's
`click-sdd` plugin. Every phase must delegate to its named `click-{token}` agent
(see `plugins/click-sdd/agents/click-orchestrator.md`'s "Skill hand-off" and "Model routing"
sections); none may fall back to a generic/unnamed agent or to any `~/.claude/agents/sdd-*.md` /
`~/.claude/skills/_shared/sdd-orchestrator-workflow.md` gentle-ai asset.

## Preconditions

1. Use a clean machine or a clean Claude Code user profile with no prior gentle-ai install. If you
   must reuse an existing machine, temporarily move `~/.claude/agents/` and
   `~/.claude/skills/` aside (e.g. rename to `~/.claude/agents.bak/` and `~/.claude/skills.bak/`)
   so gentle-ai's `sdd-*` agents and shared workflow doc cannot be found or loaded, even by
   accident.
2. Confirm no gentle-ai marketplace/plugin is registered in Claude Code's `settings.json`
   (`pluginConfigs` should have no gentle-ai entries).
3. Install `click-ai-devkit` fresh: `click install` (or the repo's documented install path), then
   verify `plugins/click-sdd/agents/click-orchestrator.md` and its 17 sibling agents exist under
   the installed plugin cache.
4. Pick a trivial, low-risk change to drive through the cycle (e.g. a one-line doc fix or a tiny,
   clearly-scoped code change) — the goal is to exercise delegation wiring, not to ship a real
   feature.

## Steps — full explore -> archive cycle

For each step below, after the phase delegates, confirm in the transcript / tool-call log which
agent actually received the `Agent` delegation. It MUST be the named `click-{token}` agent, never
`general-purpose` or any `sdd-*` gentle-ai agent.

1. **explore** — start the change with ClickOrchestrator. Confirm delegation resolves to
   `click-explore`.
2. **propose** — confirm delegation resolves to `click-prd-writer`.
3. **spec** — confirm delegation resolves to `click-prd-writer` (same agent owns both propose and
   spec — this is expected, not a gap).
4. **design** — confirm delegation resolves to `click-architect`.
5. **tasks** — confirm delegation resolves to `click-architect`.
6. **apply** — confirm delegation resolves to `click-apply`, and that it runs the phase's own
   `plugins/click-sdd/skills/apply/SKILL.md` (not a remembered/paraphrased procedure).
7. **verify** — confirm delegation resolves to `click-reviewer`.
8. **archive** — confirm delegation resolves to `click-archive`.
9. **onboard** (separate, optional check) — start a fresh onboarding walkthrough instead of a real
   change and confirm delegation resolves to `click-onboard`.

## Judgment Day + review-lens spot-check

Run this once, after `design` or `apply`, on the same trivial change (or a second one if easier to
isolate):

1. Trigger Judgment Day. Confirm `jd-judge-a` resolves to `click-jd-judge-a` and `jd-judge-b`
   resolves to `click-jd-judge-b`, running blind/independently.
2. If either judge raises a converged BLOCKER/CRITICAL finding, confirm `jd-fix-agent` resolves to
   `click-jd-fix-agent`.
3. Trigger one standard-tier review lens (pick whichever the diff's dominant risk selects, e.g.
   `review-readability` for a small refactor). Confirm it resolves to the matching
   `click-review-{lens}` agent (`click-review-risk`, `click-review-readability`,
   `click-review-reliability`, or `click-review-resilience`).
4. If a BLOCKER/CRITICAL candidate needs adversarial verification, confirm the refuter step
   resolves to `click-review-refuter`.

## Pass / fail criteria

- **Pass**: every phase and lens above resolved to its named `click-{token}` (or
  `click-prd-writer` / `click-architect` / `click-reviewer` for the phases they own) agent. Zero
  "agent not found" errors. Zero silent fallback to a generic/unnamed agent. The cycle completes
  from `explore` through `archive` without needing any gentle-ai asset.
- **Fail**: any phase either errors looking for a missing agent, or silently falls back to a
  generic/unnamed agent, or requires a gentle-ai `~/.claude` asset to complete. Treat a fail as a
  release blocker — do not ship a `click-ai-devkit` version where this runbook fails.

## Pre-release checklist item

Add to the release checklist: **"Portability runbook (`documentacion/portability-runbook.md`)
passed on a clean/gentle-ai-absent profile for this version."** This is a manual, human-run gate —
it is not automated by `go test ./...` (which only proves the static/structural claims: the 12
agent files exist, the orchestrator names them, and `DefaultManagedContent` stays gentle-ai-free).
