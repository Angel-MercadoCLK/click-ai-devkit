---
name: propose
description: Turn an approved idea (and any exploration notes) into an English proposal for a Click Seguros change — scope, requirements, acceptance criteria, and unresolved product questions.
---

## Workflow

1. Restate the problem and desired outcome.
2. Define in-scope and out-of-scope behavior.
3. Write functional requirements.
4. Note acceptance criteria at a high level (the `spec` phase turns these into full scenarios).
5. Highlight unresolved product questions.

## Inputs and outputs

- Reads: `explore`'s notes when available; otherwise the developer's request directly.
- Writes: the proposal artifact — problem statement, scope boundaries, functional requirements,
  high-level acceptance criteria, and open questions. This is the foundational document the rest of
  the chain builds on.

## Rules

- Write in English.
- Do not invent business policy.
- Keep the proposal useful for `spec` and `design`, not bloated.
- Hand off to `spec` (acceptance-criteria scenarios) and `design` (technical approach) once scope is
  clear — both read the same proposal, `design` does not wait for `spec` to finish.
