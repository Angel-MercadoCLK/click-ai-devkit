---
name: onboard
description: Guide a developer end-to-end through the Click Seguros SDD workflow using their real codebase, so they understand each phase before running it for real.
---

## Workflow

1. Explain the SDD chain in plain terms: `explore -> propose -> spec/design -> tasks -> apply ->
   verify -> archive`, plus `onboard` itself and Judgment Day's `jd-judge-a`/`jd-judge-b`/
   `jd-fix-agent` adversarial review trio.
2. Walk through one small, real example from the developer's own repo — pick something concrete,
   not a toy.
3. For each phase, show what it reads, what it writes, and why the ordering matters (e.g. `tasks`
   needs both `spec` and `design` before it can produce a sane breakdown).
4. Let the developer drive: ask them what they'd explore, propose, or spec next, and correct
   gently rather than doing it for them.
5. Close with a short recap of where each artifact lives and how to resume the flow later.

## Inputs and outputs

- Reads: nothing required — this is a pedagogical walkthrough, not a change-producing phase. It may
  reference a real (or freshly-started) change's artifacts as a live example.
- Writes: nothing persisted to the SDD chain itself. `onboard` never produces a proposal, spec,
  design, tasks, apply, verify, or archive artifact on the developer's behalf.
- Returns: the standard 6-field Result Contract to the orchestrator — see
  `plugins/click-sdd/skills/_shared/result-contract.md`.

## Rules

- Teach, don't just narrate — ask questions, don't just recite the phase list.
- Keep it concrete: use the developer's actual codebase, not abstract examples.
- Never silently do the work for them; the goal is the developer understanding the flow, not a
  finished change.
- Keep the tone practical and encouraging, matching the repo's overall Spanish-facing,
  professional communication style.
