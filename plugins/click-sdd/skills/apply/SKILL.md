---
name: apply
description: "Implement approved Click Seguros tasks with strict TDD by default: failing test first, implementation second, refactor third."
---

## Workflow

1. Read the approved tasks, spec, and design.
2. Write a failing test first for the next task.
3. Implement the smallest code change that makes the test pass.
4. Refactor while keeping tests green.
5. Mark the task complete and repeat, task by task, until the assigned scope is done.

## Inputs and outputs

- Reads: `tasks` (required — the specific task(s) assigned for this batch), `spec` and `design`
  (required — acceptance criteria and technical approach constrain every change).
- Writes: actual code changes plus updated task checkmarks. Returns a progress summary (files
  changed, tests written, remaining tasks) so a follow-up `apply` batch or `verify` can pick up
  cleanly.
- Returns: the standard 6-field Result Contract to the orchestrator — see
  `plugins/click-sdd/skills/_shared/result-contract.md`.

## Default mode

- Strict TDD is on by default.
- The only opt-out is an explicit spike or non-production exception.
- If TDD is waived, record the reason clearly.

## Rules

- Do not skip the failing-test step by default.
- Keep changes aligned with the approved design — do not freelance a different approach.
- Keep the developer informed of blockers and progress.
- Hand off to `verify` once every assigned task in this batch is complete.
