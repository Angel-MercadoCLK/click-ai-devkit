> Status: draft v0.1. Grounded in `00-decisions-and-open-questions.md` (D1–D20).

# click-ai-devkit — Product Requirements Document (PRD)

> Product-level framing of click-ai-devkit v0.1. This document goes deeper than `vision.md` on
> personas, user stories, metrics, and milestones. It **references** `vision.md` for the problem
> narrative and value proposition rather than repeating them, and links to `requirements.md` for
> the full FR/NFR list rather than duplicating it.

## 1. Summary

`click-ai-devkit` is an internal, installable Claude Code system that standardizes AI-assisted
development at **Click Seguros**. One install brings a custom orchestrator (`ClickOrchestrator`),
an internal SDD flow (`click-sdd-*`), specialized agents, internal skills, a memory policy, and a
bundled, pinned Engram instance for persistent memory. See `vision.md` for the full problem
narrative and value proposition.

**One-line value:** every Click developer gets the same orchestrator, the same planning flow, and
the same data-safety guardrails from a single command — instead of each person reinventing (or
skipping) them.

## 2. Problem

Summarized from `vision.md` (§"The problem"):

1. **Context re-explanation** — every new session/dev restates the same architecture, conventions,
   and gotchas to the AI.
2. **No shared discipline** — no common flow from idea to shipped AI-assisted change.
3. **No safety net for sensitive data** — nothing stops policy numbers, claims (siniestros) data,
   amounts, or customer identifiers from reaching an AI memory store.

## 3. Goals & non-goals

### Goals (v0.1)

- One-command setup that reproducibly installs the orchestrator, SDD flow, memory guard, and
  pinned Engram (D1, D3, D5, D8).
- A single, repeatable AI-assisted flow: explore → PRD → design → tasks → code → review → memory
  curation (D9).
- Deterministic protection so sensitive insurance/PII data never reaches persistent memory
  (D6, D7).
- A plain-spoken orchestrator that explains itself, replying in Spanish, producing English
  artifacts (D4, D10).
- A safe, gradual rollout gated by a hardening canary (D11).

### Non-goals (v0.1)

Restated from `vision.md` Non-goals — see it for rationale. In short: not reimplementing Engram;
not installing the full Gentle-AI ecosystem; no dashboard/telemetry; Claude Code only; no
auto-merge/auto-deploy; not every repo on day one; no uncontrolled/global memory.

## 4. Target users & personas

| Persona | Who | Primary jobs-to-be-done |
|---|---|---|
| **Click developer** (primary) | Click Seguros software developer using Claude Code day to day. | "Set up AI tooling once and stay current without a manual checklist." "Take a feature from idea to reviewed code with one clear flow." "Not re-explain the same context every session." "Never accidentally leak customer/policy/claims data into AI memory." |
| **Engineering lead** (secondary) | Engineering leadership responsible for consistency and data safety of AI-assisted work. | "Have confidence AI-assisted work is consistent and reviewable across the team." "Know that sensitive data cannot reach persistent memory — by a control, not a policy PDF." "Roll this out gradually with a clear go/no-go gate." |

See `vision.md` §"Who it's for" for the fuller audience framing.

## 5. User stories & scenarios

Grouped by theme. Acceptance criteria align to `mvp-scope.md` §3 and cite the backing
requirement IDs from `requirements.md`.

### 5.1 Setup

**US-1 — One-command install.**
> As a Click developer, I want to install the whole devkit with one command, so that I don't hand-copy files into `~/.claude`.

Acceptance criteria:
- `scoop bucket add click https://github.com/Angel-MercadoCLK/click-ai-devkit` then `scoop install click` succeeds on a clean Windows machine. (FR-016, D23)
- `click install` copies the three plugins, writes `CLAUDE.md` rules, configures the pinned Engram MCP entry, and registers the `memory-guard` hook — without error. (FR-001–FR-005)
- `click doctor` reports every step healthy. (FR-009, NFR-012)

**US-2 — Stay current safely.**
> As a Click developer, I want to update the devkit without manual edits, so that everyone runs the same reproducible setup.

Acceptance criteria:
- `click update` re-syncs plugins and the pinned Engram version with no manual file edits. (FR-006–FR-008)
- Running the same release twice produces no further change (idempotent). (NFR-004)

**US-3 — Clean removal.**
> As a Click developer, I want to fully uninstall, so that I can revert to a plain Claude Code setup at any time.

Acceptance criteria:
- `click uninstall` removes the plugins, deregisters the hook, and strips the `CLAUDE.md` additions — leaving a setup equivalent to before install. (FR-011–FR-013, FR-052)

### 5.2 SDD flow

**US-4 — One clear way to work with AI.**
> As a Click developer, I want a single guided flow from idea to reviewed code, so that I don't ad-hoc-prompt every feature differently.

Acceptance criteria:
- The chain `click-sdd-explore → prd → design → tasks → code → review → click-memory-curator` is invocable end to end for one request. (FR-025–FR-032)
- `ClickOrchestrator` is active on opening Claude Code, drives the flow, and explains each handoff in plain language. (FR-033, FR-037)

**US-5 — Understand what the AI is doing, in my language.**
> As a Click developer, I want the orchestrator to reply in Spanish and explain itself plainly, so that I always know why it did something.

Acceptance criteria:
- Orchestrator replies to the developer in Spanish. (FR-035)
- All produced artifacts (PRD, design, tasks, memory entries) are in English. (FR-036)
- Explanations use plain language, no unexplained jargon. (FR-034, NFR-010)

### 5.3 Memory safety

**US-6 — A safety net I don't have to think about.**
> As a Click developer, I want sensitive data screened out of memory automatically, so that I can't accidentally leak policy/claims/customer data.

Acceptance criteria:
- A red-team PII/insurance payload sent to `mem_save` is blocked or redacted before reaching Engram, in every test case. (FR-041, FR-043)
- Enforcement holds even if the model ignores the policy docs — the deterministic hook intercepts every `mem_save`. (FR-038, FR-039, NFR-001, NFR-002)

**US-7 — Persist only useful technical knowledge.**
> As a Click developer, I want only durable technical knowledge remembered, so that memory stays useful and safe.

Acceptance criteria:
- `click-memory-curator` proposes only technical-knowledge entries (decisions, conventions, patterns, gotchas, bugfixes) — no requirements text, code diffs, or business data. (FR-031)
- Deny-by-default: nothing persists unless it matches an allowed category. (FR-040, NFR-011)
- A curated entry is retrievable in a later session by another developer. (FR-032; mvp-scope.md §3)

### 5.4 Review

**US-8 — Catch issues before merge.**
> As a Click developer, I want a pre-merge review against Click standards, so that AI-assisted changes meet the same bar as any other change.

Acceptance criteria:
- `click-sdd-review` runs a pre-PR/pre-merge check via `click-reviewer` and can loop back to `click-sdd-code` when fixes are needed. (FR-030)
- No auto-merge/auto-deploy — a human stays in the loop. (FR-058)

### 5.5 Adoption

**US-9 — Confidence to roll out.**
> As an engineering lead, I want a gated rollout with a hard safety test, so that the team only adopts this once the guard is proven.

Acceptance criteria:
- Team-wide rollout proceeds only after the `memory-guard` passes the 100% red-team PII test during the canary. (FR-043; adoption-plan.md §4)
- `memory-guard` can be disabled independently, and `click uninstall` provides a per-developer rollback path, if the guard misbehaves. (FR-014, FR-044, FR-053)

**US-10 — Measure the payoff.**
> As an engineering lead, I want a lightweight way to measure re-explanation minutes saved, so that we can judge the pilot without building telemetry.

Acceptance criteria:
- A per-session self-report mini-log captures estimated-vs-actual re-explanation minutes. (adoption-plan.md §5)
- No dashboard/telemetry is built for v0.1. (FR-056)

## 6. Success metrics

### Primary

**Minutes of context re-explanation avoided per session** (D11, vision.md §"Success metric").

- **How measured:** before/after self-report. After each AI-assisted session, a participant
  estimates the minutes they *would have* spent re-explaining context (architecture, conventions,
  prior decisions) without the tool, versus what they actually spent.
- **Instrumentation:** a lightweight per-session mini-log (adoption-plan.md §5), not automated
  telemetry — no dashboard in v0.1.
- **Starts:** day one of the canary.

### Secondary signals

| Signal | What it tells us | Source |
|---|---|---|
| Red-team PII test result | Safety readiness — must be 100% blocked/redacted before rollout | D11; adoption-plan.md §4 |
| `click doctor` health across canary participants | Install reproducibility holds in practice | adoption-plan.md §4 |
| Guard false-positive rate (qualitative) | Whether the guard is usable, not just safe | adoption-plan.md §4, §7 |
| Team-wide adoption after go/no-go | Whether the tool actually gets used post-canary | adoption-plan.md §2 |

## 7. Scope (v0.1)

Full detail in `mvp-scope.md` §1. Summary:

**In:** Go CLI (`install`/`update`/`doctor`/`uninstall`) via Click scoop bucket; three markdown
plugins (`click-sdd`, `click-memory`, `click-review`); the SDD flow; `ClickOrchestrator`;
deny-by-default memory policy + `memory-guard` hook + policy docs; bundled/pinned Engram;
canary-gated pilot; self-report metric.

**Out:** reimplementing Engram; the full Gentle-AI ecosystem; dashboard/telemetry; non-Claude-Code
tools; auto-merge/auto-deploy; day-one all-repo rollout; uncontrolled/global memory;
brew/PowerShell installers (optional later per D5 — see Open questions).

## 8. Requirements summary

The full traceable list lives in `requirements.md`. Grouped by area:

| Area | Requirement IDs |
|---|---|
| CLI install/update/doctor/uninstall | FR-001–FR-015, FR-049–FR-053 |
| Distribution (scoop primary, brew/PS optional) | FR-016–FR-018 |
| Engram bundling & pinning | FR-019–FR-021, FR-051 |
| Three plugins | FR-022–FR-024 |
| SDD phases | FR-025–FR-032 |
| ClickOrchestrator persona/language | FR-033–FR-037 |
| memory-guard hook | FR-038–FR-044 |
| Memory policy docs | FR-045–FR-048 |
| v0.1 non-goals (Won't) | FR-054–FR-060 |
| Non-functional (security, reproducibility, cross-platform, performance, maintainability, auditability, usability, privacy) | NFR-001–NFR-012 |

Key non-functional guarantees: no sensitive data ever reaches Engram, enforced independently of
the model (NFR-001, NFR-002); same release → same setup + same Engram version (NFR-003, NFR-004);
one-command setup (NFR-009); deny-by-default posture appropriate for an insurer (NFR-011).

## 9. Milestones / phases

Aligned to `adoption-plan.md` §1–§2.

| Phase | Goal | Exit criteria |
|---|---|---|
| **Build** | Repo skeleton + Go CLI + three plugins + memory-guard + pinned Engram wired. | `mvp-scope.md` §2 deliverables complete; §3 acceptance criteria met on a clean machine. |
| **Canary** (3–5 days, 2–3 devs) | Validate the safety control and install under real usage; `memory-guard` in observe+block mode. | Go/no-go gate (adoption-plan.md §4): **100% red-team PII test**, healthy `click doctor` throughout, no usability-blocking false positives, self-report logs exist. |
| **Team-wide rollout** (same sprint) | Open to the whole team once the canary passes. | Install completed for the rest of the team; self-report continues; support plan active (adoption-plan.md §7). |
| **Steady state** | Normal use; iterate on the guard pattern set. | Ongoing self-report; false-positive/negative triage loop (adoption-plan.md §7). |

The red-team gate is a **hard gate**, not a target: if it does not pass 100%, rollout does not
proceed — the pattern set is fixed and re-tested first (D11; adoption-plan.md §1).

## 10. Risks & mitigations

Extends `mvp-scope.md` §5.

| Risk | Mitigation | Trace |
|---|---|---|
| `memory-guard` has a gap and lets sensitive data through | Canary in observe+block mode + mandatory 100% red-team PII test before rollout | D11; mvp-scope.md §5 |
| Guard is *too* aggressive (blocks legitimate technical saves), making the tool annoying | Qualitative usability check in the go/no-go gate; false-positive triage loop; iterative pattern-set tuning | adoption-plan.md §4, §7 |
| Go CLI needs ongoing maintenance the team may be under-resourced for | Accepted deliberately (D5); scope kept thin (install/update/doctor/uninstall only) | D5; mvp-scope.md §5 |
| Upstream Engram changes break compatibility | Engram pinned per release, not floating latest; updates explicit via `click update` | D8; mvp-scope.md §5 |
| Devs paste real data into a prompt later summarized into memory | Two-layer defense: policy docs + deterministic guard, independent of model behavior | D6, D7; mvp-scope.md §5 |
| Distribution path was ambiguous (marketplace vs. CLI) | Resolved by D24: ship marketplace.json and have the Go CLI orchestrate the native `claude plugin` install path | D24 |
| Canary too small/short to catch real-world edge cases | Deliberate trade-off (D11); go/no-go can extend or repeat the canary | D11; mvp-scope.md §5 |
| Metric is self-reported and subjective | Accepted for v0.1 (no telemetry, per Non-goals); keep the log under a minute so devs actually fill it | vision.md; adoption-plan.md §5 |

## 11. Open questions (carried forward)

Not resolved here — tracked in `00-decisions-and-open-questions.md` §3 and mirrored in the source
docs' open-item sections. Also see `requirements.md` §6 (Open requirements).

- SDD flow defaults: interactive-vs-automatic, and strict-TDD on/off for `click-sdd-code`.
- Concrete PII/insurance pattern set for `memory-guard`.
- Authoring of `allowed-memory.md`, `forbidden-memory.md`, and `SECURITY.md`.
- brew/PowerShell installer timing (optional later per D5).
- Scoop bucket repo ownership/location and its CI release process.

## 12. Related documents

- `vision.md` — problem, value proposition, principles, success-metric definition.
- `requirements.md` — full FR/NFR list, constraints, assumptions, traceability to D1–D20.
- `architecture.md` — components, install flow, security/memory enforcement architecture.
- `mvp-scope.md` — in/out scope, deliverables, acceptance criteria, risks.
- `sdd-workflow.md` — phase-by-phase detail and a worked example.
- `adoption-plan.md` — canary process, roles, go/no-go gate, metric log, rollback.
