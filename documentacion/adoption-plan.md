# click-ai-devkit — Adoption Plan

> Status: draft v0.1. Grounded in `00-decisions-and-open-questions.md` (D1–D20).

## 1. Rollout shape (D11)

Team-wide rollout, gated by a short hardening canary:

```
Canary (3–5 days, 2–3 devs)
   memory-guard in observe+block mode
   red-team PII test → must pass 100%
        │
        ▼ (pass)
Team-wide rollout, same sprint
        │
        ▼
Re-explanation minutes tracked via before/after self-report, from day one of the canary
```

If the red-team test does not pass 100%, the rollout does **not** proceed to the whole team — the guard's pattern set is fixed and re-tested first (architecture.md §6). This is a hard gate, not a target.

## 2. Timeline / phases

| Phase | Duration | Who | What happens |
|---|---|---|---|
| **Canary setup** | Day 0 | Canary owner | Install click-ai-devkit for 2–3 canary devs; confirm `click doctor` is green for each |
| **Canary run** | Days 1–5 | 2–3 canary devs | Normal AI-assisted work using `ClickOrchestrator` + `click-sdd-*`; `memory-guard` runs in observe+block mode; self-report log filled after each session |
| **Red-team PII test** | During canary (before go/no-go) | Canary owner + supporting engineer | Deliberate attempts to get PII/insurance data (policy numbers, claims data, amounts, customer identifiers) saved to memory; every attempt must be blocked or redacted |
| **Go/no-go review** | End of canary window | Canary owner + engineering leadership | Review red-team results (must be 100% pass) and self-report data; decide to proceed, extend the canary, or fix and re-run |
| **Team-wide rollout** | Same sprint as go/no-go | Canary owner + engineering leadership | Install click-ai-devkit for the rest of the team |
| **Steady state** | Ongoing | Whole team | Normal use; self-report continues; support plan (§5) active |

## 3. Roles

| Role | Responsibility |
|---|---|
| **Canary owner** | Runs the canary end to end: coordinates the 2–3 devs, collects self-report logs, runs/verifies the red-team PII test, presents the go/no-go recommendation |
| **Supporting engineer(s)** | Help construct red-team test cases, triage any `memory-guard` false positives/negatives found during the canary |
| **Canary devs (2–3)** | Use click-ai-devkit for real work during the canary window, fill in the self-report log after each AI-assisted session, report friction or bugs |
| **Engineering leadership** | Approves/denies the go/no-go decision; owns the call to extend the canary if the red-team test fails |

## 4. Go/no-go gate

Proceed to team-wide rollout **only if all of the following hold** at the end of the canary window:

- [ ] Red-team PII test: 100% of attempts blocked or redacted (no exceptions)
- [ ] `click doctor` reports healthy installs for all canary participants throughout the window
- [ ] No canary participant reports the `memory-guard` blocking legitimate technical-knowledge saves at a rate that makes the tool unusable (a qualitative check, not a hard number — flag for leadership judgment if it comes up)
- [ ] Self-report logs exist for the canary window (even if the re-explanation-minutes numbers are inconclusive at this early stage — the point of the canary gate is safety, not yet proving the metric)

If the red-team test fails: fix the `memory-guard` pattern set, re-run the red-team test, and repeat until it passes 100% before advancing. Do not relax the gate to "mostly passes."

## 5. Metric instrumentation — self-report format

No dashboard or telemetry in v0.1 (per `vision.md` Non-goals). Instrumentation is a lightweight per-session log, filled in by the developer right after an AI-assisted session.

**Suggested per-session mini-log (one entry per session):**

| Field | Example |
|---|---|
| Date | 2026-07-07 |
| Developer | (initials or name) |
| Feature/task worked on | "Add status filter to claims-intake dashboard" |
| Estimated minutes *would have* spent re-explaining context without the tool | 15 |
| Actual minutes spent re-explaining (if any) | 2 |
| What context was reused (if known) | "Filter pattern convention from a prior session" |
| Notes | free text — friction, surprises, guard false positives, etc. |

This can be a shared spreadsheet or a simple markdown log per canary dev — the format matters more than the tool. Keep it to under a minute to fill in, or devs will skip it.

## 6. Training / enablement

A short "getting started" for devs, to hand out at canary kickoff and team-wide rollout:

1. Install: `scoop bucket add click https://github.com/Angel-MercadoCLK/scoop-bucket` → `scoop install click` → `click install` → `click doctor` to confirm.
2. Open Claude Code — `ClickOrchestrator` is active. It replies in Spanish and explains what it's doing at each step.
3. Start any feature by describing it to the orchestrator — it will walk you through explore → PRD → design → tasks → code → review. You don't need to invoke each phase manually.
4. What gets remembered: only technical decisions, conventions, and gotchas — never real customer/policy/claims data. If you ever see the guard block something, that's it working as intended — report it if it blocks something that *should* have been fine (false positive) so the pattern set can improve.
5. Fill in the self-report mini-log after each AI-assisted session during the canary (§5) — this is how the pilot's success gets measured.

## 7. Support plan

- **During the canary:** canary owner is the direct point of contact for questions, blockers, and bug reports from the 2–3 canary devs.
- **At team-wide rollout:** canary owner (or a designated successor) remains the point of contact; `click doctor` is the first troubleshooting step for any install issue.
- **Memory-guard false positives/negatives:** reported to the canary owner/supporting engineer, who can adjust the pattern set (this is expected to be an iterative process, not a one-time authoring task — see the open item on the concrete PII/insurance pattern set).

## 8. Rollback / kill-switch

If `memory-guard` misbehaves (blocks too aggressively, or — worse — fails to block something it should have caught) at any point, during the canary or after team-wide rollout:

- **Immediate mitigation:** `click uninstall` removes the plugins, the hook registration, and the CLAUDE.md additions from a developer's machine, reverting to a plain Claude Code setup.
- **Targeted mitigation:** the `memory-guard` hook registration can be disabled in Claude Code settings without a full uninstall, if only the guard (not the whole devkit) is misbehaving — stopping all memory writes until the pattern set is fixed is preferable to letting unguarded writes through.
- **Team-wide rollback:** if an issue is found after team-wide rollout that wasn't caught in the canary, the same `click uninstall` path applies per developer; there is no server-side kill switch in v0.1 since there is no central service beyond the bundled Engram instance each developer runs.

## 9. Open items (not resolved here — carried forward)

- Concrete PII/insurance pattern set for the memory-guard (affects how much canary tuning is needed).
- `allowed-memory.md` / `forbidden-memory.md` / `SECURITY.md` authoring — referenced by training material above but not yet written.
- Interactive vs. automatic SDD default, and strict-TDD default — may affect how the "getting started" walkthrough reads once settled.
