---
name: design
description: Define the technical approach, affected architecture, and implementation constraints for a Click Seguros change, from the approved proposal.
---

## Workflow

1. Read the proposal and the relevant code.
2. Identify the affected modules and interfaces.
3. Explain the chosen approach and rejected alternatives.
4. Document test strategy and rollout considerations.
5. Return a design artifact in English.

## Inputs and outputs

- Reads: the proposal (required). Reads the relevant existing code directly — it does not wait on
  `spec` to finish; both `spec` and `design` branch off the same approved proposal.
- Writes: the design artifact — affected modules, data flow, chosen approach vs. rejected
  alternatives, test strategy, and rollback/migration notes. `tasks` reads both `spec` and `design`
  before breaking work into an ordered list.
- Returns: the standard 6-field Result Contract to the orchestrator — see
  `plugins/click-sdd/skills/_shared/result-contract.md`.

## Rules

- Prefer reuse over new abstraction.
- Make trade-offs explicit.
- Keep the design actionable for the tasks phase.
