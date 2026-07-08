# Click Memory Policy

## Purpose

This plugin is the human-facing memory layer for click-ai-devkit. It explains what kinds of information agents may attempt to persist and how that guidance works together with the deterministic `memory-guard` hook.

## Policy posture

Click Seguros uses a **deny-by-default / allowlist** memory policy.

- Persist only durable technical knowledge.
- Reject anything that is not clearly technical knowledge.
- If there is doubt, do not save it.

## Two-layer enforcement

Memory safety is enforced by two layers:

1. **Human-facing policy layer**
   - `memory-policy.md`
   - `allowed-memory.md`
   - `forbidden-memory.md`

   These files guide what an agent should attempt to save.

2. **Deterministic enforcement layer**
   - `click memory-guard` PreToolUse hook
   - `internal/guard/patterns.yaml`

   This hook scans every `mem_save` payload before it reaches Engram. It is independent of model intent.

## v0.1 behavior

- The guard is **block-only** in v0.1.
- Unsafe payloads are denied; they are not redacted and forwarded.
- The guard fails closed for internal errors.

## Audit behavior

- Guard audit logs are local only.
- Audit entries store **SHA-256 hashes**, never raw payloads.
- No telemetry or network reporting is performed by the guard.

## What belongs in memory

Only durable technical knowledge belongs in memory:

- architecture decisions
- design decisions
- conventions
- reusable patterns
- technical gotchas
- bugfixes with technical root cause

Everything else is out of scope unless it can be rewritten into a safe technical summary.
