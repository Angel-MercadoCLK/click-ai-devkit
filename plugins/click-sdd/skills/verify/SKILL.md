---
name: verify
description: Review a completed Click Seguros change before PR/merge, validate the implementation against spec/design/tasks, and decide whether it passes or loops back to implementation.
---

## Workflow

1. Read the implementation against the spec, design, and task list.
2. Check correctness, tests, maintainability, and safety.
3. Separate blockers from suggestions.
4. Return clear pass/fail findings in English.
5. If needed, send the change back to `apply` with concrete fixes.

## Inputs and outputs

- Reads: `spec`, `tasks`, and the apply-progress artifact (what was actually implemented and how).
- Writes: a verify report with findings classified as CRITICAL / WARNING / SUGGESTION and a clear
  verdict (pass, pass with warnings, or send back to `apply`).
- Returns: the standard 6-field Result Contract to the orchestrator — see
  `plugins/click-sdd/skills/_shared/result-contract.md`.

## Rules

- Be evidence-based — re-check claims against the actual code, don't trust the apply summary alone.
- Keep the review actionable.
- Do not approve changes that drift from the agreed scope.
- CRITICAL findings block; WARNING and SUGGESTION do not block but must be reported.
- Hand off to `archive` once the change passes.
