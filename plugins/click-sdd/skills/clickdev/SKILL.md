---
name: clickdev
description: Route a developer's clickdev request to the existing explore/propose SDD flow using the saved clickhola brief as grounding. This is a thin alias only.
---

## Workflow

1. Receive the request that would normally be handled by `/clickdev`.
2. Load the saved brief from Engram under `sdd/{change-name}/elicitation`.
3. Feed that brief as grounding into the existing `explore` → `propose` SDD flow.
4. Do not add new behavior here; keep this file declarative.

## Inputs and outputs

- Reads: the change name from the developer and the existing `sdd/{change-name}/elicitation` brief.
- Writes: nothing directly. Delegates to the orchestrator's existing explore/propose flow.
- Returns: the standard Result Contract to the orchestrator — see `plugins/click-sdd/skills/_shared/result-contract.md`.

## Rules

- This skill is a thin alias only.
- All behavior lives in the orchestrator's existing explore/propose flow.
- Keep this file declarative.
