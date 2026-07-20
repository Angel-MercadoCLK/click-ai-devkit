---
name: archive
description: Close a finished, verified Click Seguros SDD change and persist its final state for future reference.
---

## Workflow

1. Confirm the change reached a PASS or PASS WITH WARNINGS verdict from `verify` — never archive a
   change that still has open CRITICAL findings.
2. Collect the final state of the proposal, spec, design, tasks, apply-progress, and verify report.
3. Write a short archive summary: what shipped, what was deliberately deferred or descoped, and any
   follow-up work flagged for later.
4. Mark the change closed in whatever artifact store is active (persist the archive record; do not
   leave the change dangling as "in progress").
5. Hand any durable technical knowledge worth keeping to memory curation.

## Inputs and outputs

- Reads: every prior artifact for the change (proposal, spec, design, tasks, apply-progress,
  verify-report).
- Writes: the archive report — a closing summary plus a pointer to where the durable record lives.
- Returns: the standard 6-field Result Contract to the orchestrator — see
  `plugins/click-sdd/skills/_shared/result-contract.md`.

## Rules

- Do not archive a change with unresolved CRITICAL findings from `verify`.
- Keep the archive summary short — it is a closing record, not a re-statement of every prior
  artifact.
- Explicitly note anything that was descoped or deferred so it is not silently lost.
- This is the last phase in the chain — nothing hands off after `archive`.
