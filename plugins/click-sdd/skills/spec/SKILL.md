---
name: spec
description: Turn an approved proposal into concrete, verifiable acceptance-criteria scenarios for a Click Seguros change.
---

## Workflow

1. Read the proposal's scope and functional requirements.
2. Write acceptance-criteria scenarios that cover the happy path, edge cases, and explicit
   non-goals.
3. State each scenario so it can be checked mechanically or by direct inspection later (no vague
   "should work correctly" scenarios).
4. Flag any requirement in the proposal that is too ambiguous to turn into a scenario.
5. Return the spec artifact in English.

## Inputs and outputs

- Reads: the proposal (required).
- Writes: the spec artifact — a set of acceptance-criteria scenarios. `tasks` reads this alongside
  `design` to build the implementation task list; `verify` reads this as the source of truth for
  what "done" means.

## Rules

- Every scenario must be independently verifiable — a reviewer or the `verify` phase should be able
  to check it against the implementation without guessing intent.
- Do not restate the proposal's prose; translate it into testable behavior.
- Do not design the implementation here — that is `design`'s job.
- Flag unresolved product ambiguity back to the developer instead of resolving it by assumption.
