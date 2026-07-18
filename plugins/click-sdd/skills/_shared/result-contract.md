# Result Contract

Canonical definition of the 6-field structured result that every click-sdd phase-executor
agent returns to the orchestrator when a delegated phase completes. This document is the single
source of truth for field names, allowed values, and semantics — agent files carry a short inline
echo of these 6 fields (portability: sub-agents may not auto-load `_shared/` files) plus a pointer
back here. A field's SEMANTICS live only in this doc; only a field-NAME change (rare) needs to
touch every agent file. The Go conformance tests in `internal/installer/portability_test.go`
guard that every phase-executor agent DECLARES all 6 field names; they do not (yet) assert that
each agent's inline allowed-value text matches this doc — value-level consistency is enforced by
review, and a stricter value-drift test is a candidate for the Phase 4 gatekeeper work.

This doc is also the source Phase 4's Mode Gatekeeper validates the envelope against.

## The 6 fields

1. `status` — one of `done` | `blocked` | `partial`.
   - `done` = the phase finished its assigned work.
   - `partial` = some work is done, more remains (e.g. an apply batch with tasks left).
   - `blocked` = the phase cannot proceed without a developer decision or missing input.

2. `executive_summary` — a single sentence describing what the phase accomplished.

3. `artifacts` — the Engram topic key(s) persisted (e.g. `sdd/{change-name}/design`) and/or file
   paths written or read. Review-lens roles return their findings ledger rows here.

4. `next_recommended` — the next phase token for the orchestrator to run, or `none` if terminal.
   Allowed tokens: `sdd-explore`, `sdd-propose`, `sdd-spec`, `sdd-design`, `sdd-tasks`,
   `sdd-apply`, `sdd-verify`, `sdd-archive`, the review-routing token `review-refuter`, or `none`.
   (Derived examples: `explore` → `sdd-propose`; `apply` → `sdd-verify` or `sdd-apply`;
   `archive` → `none`; `review-risk` → `review-refuter` or `sdd-verify`.)

5. `risks` — unresolved unknowns, assumptions, design deviations, or blocked items; `None` if
   there are none.

6. `skill_resolution` — how the phase's skill file was loaded. Canonical values click phases
   emit: `paths-injected` (the orchestrator passed the exact `SKILL.md` path and the agent read
   it) or `none` (no external skill file — inline-contract roles like the 5 review lenses always
   report `none`). The gatekeeper's ACCEPTED superset also includes the gentle-ai fallback values
   `fallback-registry` and `fallback-path` (self-loaded from a registry / `SKILL: Load` path) for
   forward-compat, but no click agent currently emits them.

## Ownership rule

- Agent files carry the inline echo of these 6 fields (the WHAT-is-returned).
- SKILL.md files carry the procedure and only REFERENCE this contract — they never restate the
  6 field names/values inline.
