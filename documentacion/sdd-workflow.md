# click-ai-devkit — SDD Workflow

> Status: draft v0.1. Grounded in `00-decisions-and-open-questions.md` (D1–D20).

## 1. Overview

The Click SDD flow is a **reuse/rebrand** of existing SDD machinery (D9), not a rewrite. Click's own value-add concentrates in two places: the **memory layer** (curator + guard, so nothing sensitive is ever persisted) and **review** (Click-specific pre-merge checks). `ClickOrchestrator` drives the whole flow and explains each step in plain language as it hands off between phases (D10).

```
User Request
   │
   ▼
ClickOrchestrator
   │
   ▼
click-sdd-explore   → click-sdd-prd   → click-sdd-design   → click-sdd-tasks
   │
   ▼
click-sdd-code   → click-sdd-review
   │
   ▼
click-memory-curator   →   memory-guard   →   Engram
```

All artifacts produced along this flow are in English; the orchestrator's conversation with the developer stays in Spanish (D4, D10).

## 2. Phase by phase

| Phase | Purpose | Inputs | Outputs | Agent | Handoff |
|---|---|---|---|---|---|
| **click-sdd-explore** | Investigate the codebase, compare possible approaches before committing to a plan | Developer's request, existing codebase | Exploration notes (approaches considered, trade-offs) | `click-orchestrator` (drives) + explore skill | Orchestrator summarizes findings to the dev in Spanish, proposes moving to PRD |
| **click-sdd-prd** | Capture what/why: requirements, scope, acceptance criteria | Exploration output (or direct request if skipped) | PRD artifact (English) | `click-prd-writer` | Orchestrator confirms scope with the dev before design |
| **click-sdd-design** | Define the technical approach and architecture decisions | Approved PRD | Design artifact: architecture decisions, approach (English) | `click-architect` | Orchestrator flags any architecture trade-offs to the dev in plain language |
| **click-sdd-tasks** | Break the design into an ordered, actionable task list | Approved design (+ PRD) | Task breakdown (English) | `click-architect` or dedicated tasks skill | Orchestrator confirms task order/scope, then proceeds to implementation |
| **click-sdd-code** | Implement the tasks | Task breakdown, design, PRD | Code changes | Implementation skill (driven by orchestrator) | Orchestrator reports progress per task, surfaces blockers |
| **click-sdd-review** | Pre-PR / pre-merge check — Click-specific standards | Implemented code | Review findings (pass/fail, issues to fix) | `click-reviewer` | Orchestrator relays findings to the dev; loops back to `click-sdd-code` if fixes are needed |
| **click-memory-curator** | Decide what, if anything, from this cycle is worth persisting as durable technical knowledge | Full cycle output (PRD, design, decisions, gotchas found during review) | Proposed memory entries (English) | `click-memory-curator` | Proposed entries are sent to `mem_save`, which routes through the guard before Engram |

## 3. Where the CLI does *not* participate

Per the "thin installer" guardrail (D3, architecture.md §1): the `click` CLI does not run any phase above, does not call Claude, and does not touch git or PRs. Everything from `click-sdd-explore` through `click-memory-curator` runs inside Claude Code, driven by the installed markdown agents and skills. The CLI's job ends at getting that markdown (and Engram, and the hook) onto the developer's machine.

## 4. How memory fits

```
click-memory-curator proposes entries
        │
        ▼
memory-policy.md / allowed-memory.md / forbidden-memory.md
   (human-facing layer — guides what the curator attempts to save)
        │
        ▼
mem_save call
        │
        ▼
memory-guard (deterministic PreToolUse hook)
   pattern-matches every mem_save payload, independent of the model
        │
    ┌───┴────┐
    ▼        ▼
  allow   block / redact
    │        │
    ▼        ▼
 Engram   rejected/stripped — never reaches Engram
```

- **What the curator proposes:** only technical knowledge — architecture/design decisions, conventions, patterns, tech gotchas, bugfixes surfaced during the cycle (D6). It does not propose to save requirements text, code diffs, or anything containing real business data.
- **How the guard gates it:** the `memory-guard` PreToolUse hook scans every `mem_save` call before it reaches Engram, regardless of what the curator (or any other agent) intended to save. This is deterministic pattern matching, not a model judgment call (D7).
- **Deny-by-default in practice:** nothing is persisted unless it matches an allowed technical-knowledge category. Anything that looks like PII, a policy number, claims (siniestros) data, an amount, or a customer identifier is always blocked or redacted, with no exception path in v0.1 (D6).

## 5. Open items — explicitly not decided here

These are carried forward from `00-decisions-and-open-questions.md` §3 and are **not resolved** by this document:

- **Interactive vs. automatic default.** Whether the Click SDD flow pauses for developer confirmation between phases by default, or runs phases back-to-back automatically, is still open.
- **Strict-TDD default.** Whether `click-sdd-code` enforces strict TDD (tests before implementation) by default, or this is opt-in, is still open.

Both will be settled during the doc/build phase per the decisions log — this workflow document describes the phase sequence and handoffs, not these two runtime defaults.

## 6. Concrete end-to-end example

A small, illustrative feature: **"Add a `status` filter to the internal claims-intake dashboard's task list view."**

1. **click-sdd-explore** — Orchestrator reads the dashboard's existing list-view code, finds it already has a similar filter for `priority`, and proposes reusing that pattern for `status` rather than building a new filter mechanism.
2. **click-sdd-prd** — `click-prd-writer` captures: who needs this (internal ops team), what statuses must be filterable, and the acceptance criterion ("user can filter the list by status and the URL reflects the filter state").
3. **click-sdd-design** — `click-architect` decides to extend the existing `priority` filter's query-param pattern for `status`, rather than introducing a new filter component — an explicit architecture decision.
4. **click-sdd-tasks** — Task list: (1) add `status` to the filter query-param parser, (2) add the UI control, (3) wire it to the existing filter state, (4) add tests.
5. **click-sdd-code** — Implementation proceeds task by task.
6. **click-sdd-review** — `click-reviewer` checks the change against Click's pre-merge checklist (e.g., no hardcoded status values, filter state persists correctly).
7. **click-memory-curator** — Proposes one memory entry: *"Dashboard list views use a shared query-param filter pattern (see `priority` filter) — reuse it for new filterable fields instead of building bespoke filter UI."* This is a pattern/convention, contains no claims data or customer identifiers, so it passes `memory-guard` and lands in Engram.
8. **Next session, different developer:** asks the orchestrator to add a `region` filter to the same view. Engram surfaces the stored pattern; the developer does not need to re-discover or be re-told about the shared filter convention — this is the re-explanation-minutes-avoided metric in action.
