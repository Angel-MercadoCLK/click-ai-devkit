---
name: memory-review
description: Review a proposed Click Seguros memory entry against the allowed and forbidden memory rules before saving.
---

## Purpose

Use this skill immediately before `mem_save`.

## Review checklist

1. Confirm the entry is technical knowledge, not business data.
2. Check it against `allowed-memory.md`.
3. Check it against `forbidden-memory.md`.
4. Apply deny-by-default when the classification is uncertain.
5. Note that the deterministic memory-guard hook will still enforce the final block decision.

## Defense in depth

- Human-facing policy decides what should be attempted.
- The deterministic PreToolUse guard decides what can actually pass.
- In v0.1 the guard is block-only, so unsafe entries are denied rather than rewritten.

## Hard rules

- Never approve an entry that contains or hints at customer data.
- Never rely on the guard as an excuse to be careless.
- When unsure, reject the entry and ask for a safer technical summary.
