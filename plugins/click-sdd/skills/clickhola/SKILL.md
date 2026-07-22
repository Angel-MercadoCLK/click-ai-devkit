---
name: clickhola
description: Route a non-technical requester's clickhola elicitation to the existing click-elicitor / requirements-elicitation Paso 1 flow before building. This is a thin alias only.
---

## Workflow

1. Receive the request that would normally be handled by `/clickhola` or the OpenClaw click-elicitor.
2. Route to the already-existing click-elicitor / requirements-elicitation Paso 1 flow.
3. Do not add new behavior here; keep this file declarative.

## Inputs and outputs

- Reads: the requester's natural-language problem or goal.
- Writes: nothing directly. Delegates to the orchestrator's Paso 1 requirements-elicitation step.
- Returns: the standard Result Contract to the orchestrator — see `plugins/click-sdd/skills/_shared/result-contract.md`.

## Rules

- This skill is a thin alias only.
- All behavior lives in the orchestrator's existing requirements-elicitation flow.
- Keep this file declarative.
