# Security

## Scope

This file documents the data-safety model for persistent memory in click-ai-devkit.

It does **not** define general application-security policy for Click Seguros products. It is specifically about preventing sensitive insurance and customer data from reaching persistent memory through Engram.

## Data-safety guarantee

click-ai-devkit uses a **deny-by-default / allowlist** memory policy.

Only durable technical knowledge may be persisted, such as:

- architecture decisions
- design decisions
- conventions
- reusable patterns
- technical gotchas
- bugfixes with technical root cause

The following categories must never reach persistent memory:

- PII
- policy numbers
- claim identifiers / siniestro identifiers
- monetary amounts
- customer identifiers

## Two-layer enforcement model

click-ai-devkit uses two complementary layers.

### Layer 1 — Human-facing policy

The `click-memory` plugin provides:

- `memory-policy.md`
- `allowed-memory.md`
- `forbidden-memory.md`
- curation skills for proposing and reviewing entries

This layer guides what an agent should attempt to save.

### Layer 2 — Deterministic enforcement

The `click memory-guard` PreToolUse hook scans every `mem_save` payload before it reaches Engram.

Characteristics:

- deterministic pattern matching
- independent of model compliance
- block-only in v0.1
- fail-closed on internal errors
- local hash-only audit log

The enforced category source of truth is `internal/guard/patterns.yaml`.

## Guard behavior in v0.1

- The guard is **block-only**.
- Unsafe payloads are denied instead of redacted and forwarded.
- Internal failures must return a blocking outcome.
- Audit entries store **SHA-256 hashes only**, never raw payloads.
- No telemetry or network reporting is performed by the guard.

## Reporting a suspected leak

If you suspect that sensitive data could have reached persistent memory:

1. Do **not** paste the sensitive data into chat, tickets, or bug reports.
2. Preserve only the minimum safe technical context needed to investigate.
3. Report the issue to the Click maintainer or security-adjacent owner responsible for click-ai-devkit.
4. Include safe evidence such as:
   - approximate timestamp
   - whether the event was allow or deny
   - the local audit log hash entry
   - the command or workflow being used
5. Treat the issue as a blocker until the pattern set and tests are reviewed.

## Verification and hard gate

The guard is backed by a red-team test harness. The rollout gate is strict: the red-team suite must pass 100% before wider adoption proceeds.

## Rollback and containment

If the memory path must be disabled quickly:

- remove or disable the managed PreToolUse hook entry, or
- run `click uninstall` to reverse the Click-managed setup

The immediate goal is to stop additional memory writes until the issue is understood.

## Pattern-set evolution

The pattern set is expected to evolve. Policy docs in `plugins/click-memory/docs/` must stay aligned with `internal/guard/patterns.yaml` so the human-facing guidance matches the enforced behavior.
