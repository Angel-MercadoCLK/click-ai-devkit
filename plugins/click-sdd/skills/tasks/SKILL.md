---
name: tasks
description: Break an approved spec and design into an ordered, reviewable implementation task list for a Click Seguros change.
---

## Workflow

1. Read the spec and the design.
2. Split the work into small, dependency-aware tasks.
3. Put tests near the behavior they validate.
4. Mark blockers, sequencing, and validation steps.
5. Return the task list in English.

## Inputs and outputs

- Reads: `spec` and `design` (both required — the task list must satisfy the spec's acceptance
  criteria using the design's chosen approach).
- Writes: the ordered task list. This is what `apply` executes, task by task.
- Returns: the standard 6-field Result Contract to the orchestrator — see
  `plugins/click-sdd/skills/_shared/result-contract.md`.

## Rules

- Tasks should be executable, not vague themes.
- Keep review size under control whenever possible — flag when a batch of tasks is likely to exceed
  a reasonable PR review budget.
- Include verification steps.
