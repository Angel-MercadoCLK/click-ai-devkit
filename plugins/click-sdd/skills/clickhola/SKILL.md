---
name: clickhola
description: Route a non-technical requester's clickhola elicitation to the existing click-elicitor/orchestrator Step 1 requirements-elicitation flow. This is a thin alias only; it does not execute elicitation logic or Engram Cloud wiring.
---

## Workflow

1. Receive the request that would normally be handled by `/clickhola` or the OpenClaw click-elicitor.
2. Route to the already-existing click-elicitor/orchestrator Step 1 requirements-elicitation flow.
3. Do not duplicate elicitation logic, add executors, or perform Engram Cloud wiring.

## Inputs and outputs

- Reads: the requester's natural-language problem or goal.
- Writes: nothing directly. Delegates to the orchestrator's requirements-elicitation step.
- Returns: the standard Result Contract to the orchestrator — see `plugins/click-sdd/skills/_shared/result-contract.md`.

## Rules

- This skill is a declarative alias only.
- All behavior lives in the orchestrator's Step 1 requirements-elicitation flow.
- Do not add new executor, runtime, or cloud behavior here.
