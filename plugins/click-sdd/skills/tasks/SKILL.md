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
6. Emit the mandatory Review Workload Forecast (see below) as the last part of the tasks
   artifact.

## Inputs and outputs

- Reads: `spec` and `design` (both required — the task list must satisfy the spec's acceptance
  criteria using the design's chosen approach).
- Writes: the ordered task list, followed by the mandatory Review Workload Forecast. This is
  what `apply` executes, task by task.
- Returns: the standard 6-field Result Contract to the orchestrator — see
  `plugins/click-sdd/skills/_shared/result-contract.md`.

## Rules

- Tasks should be executable, not vague themes.
- Always emit the mandatory Review Workload Forecast (see below) — it is not optional.
- Include verification steps.

## Review Workload Forecast (mandatory)

After the ordered task list, ALWAYS emit a Review Workload Forecast: exactly these three
plain-text lines, verbatim labels, one allowed value each, persisted inside the tasks
artifact body (never a separate topic):

Decision needed before apply: Yes|No
Chained PRs recommended: Yes|No
400-line budget risk: Low|Medium|High

How to compute each line (estimate from the task list you just wrote — task count,
estimated changed-file count, and the concerns each task touches):

- Decision needed before apply — Yes if any task hinges on an unresolved choice that must
  be settled before coding: a shared interface/contract, a schema or data migration, or an
  explicit "decide between A and B" step. If every task is a mechanical, already-decided
  step, No.
- Chained PRs recommended — Yes when the tasks split cleanly into 2+ independently
  reviewable, independently mergeable groups AND the combined change is large or mixes
  unrelated concerns. Rule of thumb: recommend chaining when the list has roughly 6+ tasks,
  OR touches 3+ distinct modules/boundaries, OR its estimated total diff exceeds the session
  PR budget. Otherwise No.
- 400-line budget risk — sum each task's estimated added+modified lines (small ~10-40,
  medium ~40-120, large >120, tests included) and compare to the session's configured
  max-lines-per-PR budget (default 400; the label stays "400-line" even when the session
  budget differs). Low = comfortably under ~60% of budget; Medium = ~60-100%; High = at or
  over budget.
