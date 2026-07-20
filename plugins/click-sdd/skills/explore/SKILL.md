---
name: explore
description: Investigate the existing codebase and compare candidate approaches before committing to a plan for a Click Seguros change. First phase of the SDD chain, invoked by /sdd-explore or click-orchestrator when a request needs codebase understanding.
---

## Workflow

1. Read the relevant code before proposing an approach.
2. Summarize the current state and affected files.
3. Compare realistic implementation approaches.
4. Recommend one approach with trade-offs.
5. Return exploration notes in English.

## Inputs and outputs

- Reads: the existing codebase. No prior SDD artifact is required — `explore` is the entry point of
  the chain (`explore -> propose -> spec/design -> tasks -> apply -> verify -> archive`).
- Writes: exploration notes only (current state, affected files, compared approaches, a
  recommendation). No proposal, spec, design, tasks, or code changes come out of this phase.
- Returns: the standard 6-field Result Contract to the orchestrator — see
  `plugins/click-sdd/skills/_shared/result-contract.md`.

## Rules

- Do not modify code in this phase.
- Prefer existing project patterns.
- Keep findings concise and evidence-based.
- Hand off to `propose` once the current state and viable approaches are understood.
