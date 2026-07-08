> Status: draft v0.1. Grounded in `00-decisions-and-open-questions.md` (D1–D20).

# click-ai-devkit — Requirements

> Audience: engineers building click-ai-devkit, and anyone validating that the build matches the
> locked decisions. Each requirement traces to a decision (D#) and/or a source doc. Where a topic
> is genuinely undecided, it is marked **OPEN** rather than resolved here.

## 1. How to read this document

- **FR-###** = functional requirement. **NFR-###** = non-functional requirement.
- **MoSCoW** tag per requirement: **Must** / **Should** / **Could** / **Won't** (v0.1).
- **Trace** = the decision(s) and/or source doc backing the requirement.
- `Won't` requirements are v0.1 non-goals, restated here as explicit scope boundaries (see
  `vision.md` Non-goals, `mvp-scope.md` §1).

## 2. Functional requirements

### 2.1 CLI — `click install`

| ID | Requirement | MoSCoW | Trace |
|---|---|---|---|
| FR-001 | `click install` MUST copy the three plugins (`click-sdd/`, `click-memory/`, `click-review/`) into `~/.claude/plugins/`. | Must | D1, D9; architecture.md §2 |
| FR-002 | `click install` MUST write/update the developer's `CLAUDE.md` with Click conventions and `ClickOrchestrator` activation rules. | Must | architecture.md §2; mvp-scope.md §3 |
| FR-003 | `click install` MUST configure the Engram MCP server entry, pointing at the Engram version pinned by the current click-ai-devkit release. | Must | D1, D8; architecture.md §2 |
| FR-004 | `click install` MUST register the `memory-guard` PreToolUse hook in Claude Code settings. | Must | D7; architecture.md §2 |
| FR-005 | `click install` MUST complete without error on a clean machine and leave a state that `click doctor` reports as fully healthy. | Must | mvp-scope.md §3 (acceptance criteria) |

### 2.2 CLI — `click update`

| ID | Requirement | MoSCoW | Trace |
|---|---|---|---|
| FR-006 | `click update` MUST re-sync the installed plugins to the version pinned in the current click-ai-devkit release. | Must | D8; architecture.md §2 |
| FR-007 | `click update` MUST update the Engram version to the one pinned by the current release (no floating "latest"). | Must | D1, D8 |
| FR-008 | `click update` MUST require no manual file edits by the developer to complete a successful update. | Must | mvp-scope.md §3 |

### 2.3 CLI — `click doctor`

| ID | Requirement | MoSCoW | Trace |
|---|---|---|---|
| FR-009 | `click doctor` MUST verify and report, per check, whether: plugins are present in `~/.claude/plugins/`, `CLAUDE.md` rules are present, the Engram MCP entry is configured at the pinned version, and the `memory-guard` hook is registered. | Must | architecture.md §2; mvp-scope.md §3 |
| FR-010 | `click doctor` MUST be usable as the first troubleshooting step for any install issue, both during the canary and at team-wide rollout. | Must | adoption-plan.md §7 |

### 2.4 CLI — `click uninstall`

| ID | Requirement | MoSCoW | Trace |
|---|---|---|---|
| FR-011 | `click uninstall` MUST remove the three plugins from `~/.claude/plugins/`. | Must | adoption-plan.md §8 |
| FR-012 | `click uninstall` MUST deregister the `memory-guard` PreToolUse hook. | Must | adoption-plan.md §8 |
| FR-013 | `click uninstall` MUST remove the Click-specific additions made to `CLAUDE.md` by `click install`. | Must | adoption-plan.md §8 |
| FR-014 | It MUST be possible to disable only the `memory-guard` hook registration (without a full uninstall), as a targeted mitigation if the guard alone is misbehaving. | Must | adoption-plan.md §8 |

### 2.5 CLI scope guardrail

| ID | Requirement | MoSCoW | Trace |
|---|---|---|---|
| FR-015 | The `click` CLI MUST NOT execute any SDD phase, call Claude, or touch git/PRs. Its responsibility ends at install/update/doctor/uninstall. | Must | "CLI scope guardrail," decisions doc; sdd-workflow.md §3 |

### 2.6 Distribution

| ID | Requirement | MoSCoW | Trace |
|---|---|---|---|
| FR-016 | click-ai-devkit MUST be installable via a Click-hosted scoop bucket (two-step: `scoop bucket add click-seguros https://github.com/click-seguros/scoop-bucket` then `scoop install click`) as the primary distribution channel. | Must | D3, D5, D18 |
| FR-017 | The CLI MUST be delivered as a single static Go binary with no separate runtime dependency. | Must | D5 |
| FR-018 | A brew tap (Mac/Linux) and/or PowerShell installer MAY be published from the same Go binary once prioritized. | Should | D5; **OPEN** on timing — see §6 |

### 2.7 Engram bundling and pinning

| ID | Requirement | MoSCoW | Trace |
|---|---|---|---|
| FR-019 | click-ai-devkit MUST bundle Engram as a dependency rather than reimplementing persistent-memory functionality. | Must | D1 |
| FR-020 | Each click-ai-devkit release MUST pin a specific Engram version (latest at the time that release is cut). | Must | D8 |
| FR-021 | `click install` and `click update` MUST always bring the pinned Engram version, never a floating/latest version resolved at install time. | Must | D8 |

### 2.8 The three plugins

| ID | Requirement | MoSCoW | Trace |
|---|---|---|---|
| FR-022 | `plugins/click-sdd/` MUST ship the agents `click-orchestrator.md`, `click-prd-writer.md`, `click-architect.md`, `click-reviewer.md`, `click-memory-curator.md`, and the six phase skills (`sdd-explore`, `sdd-prd`, `sdd-design`, `sdd-tasks`, `sdd-code`, `sdd-review`). | Must | D9; architecture.md §4 |
| FR-023 | `plugins/click-memory/` MUST ship the `memory-guard` hook, the `memory-proposal`/`memory-review` skills, and the docs `memory-policy.md`, `allowed-memory.md`, `forbidden-memory.md`, `engram-setup.md`. | Must | D6, D7; architecture.md §4 |
| FR-024 | `plugins/click-review/` MUST ship the `click-pr-reviewer.md` agent and the `pr-review`/`pre-merge-checklist` skills. | Must | D9; architecture.md §4 |

### 2.9 SDD phases

| ID | Requirement | MoSCoW | Trace |
|---|---|---|---|
| FR-025 | `click-sdd-explore` MUST investigate the existing codebase and compare candidate approaches before a plan is committed to. | Must | sdd-workflow.md §2 |
| FR-026 | `click-sdd-prd` MUST capture what/why: requirements, scope, and acceptance criteria, producing an English PRD artifact, via `click-prd-writer`. | Must | sdd-workflow.md §2 |
| FR-027 | `click-sdd-design` MUST define the technical approach and architecture decisions from an approved PRD, via `click-architect`. | Must | sdd-workflow.md §2 |
| FR-028 | `click-sdd-tasks` MUST break an approved design (+ PRD) into an ordered, actionable task list. | Must | sdd-workflow.md §2 |
| FR-029 | `click-sdd-code` MUST implement the task list, with the orchestrator reporting progress per task and surfacing blockers. | Must | sdd-workflow.md §2 |
| FR-030 | `click-sdd-review` MUST run a pre-PR/pre-merge check against Click-specific standards via `click-reviewer`, and MUST be able to loop back to `click-sdd-code` when fixes are required. | Must | sdd-workflow.md §2 |
| FR-031 | `click-memory-curator` MUST evaluate the full cycle's output (PRD, design, decisions, gotchas) and propose only durable technical-knowledge entries (no requirements text, code diffs, or business data) for `mem_save`. | Must | D6; sdd-workflow.md §2, §4 |
| FR-032 | The full phase chain (`click-sdd-explore → …→ click-memory-curator → Engram`) MUST be invocable end to end for a single developer request. | Must | mvp-scope.md §3 (acceptance criteria) |

### 2.10 ClickOrchestrator

| ID | Requirement | MoSCoW | Trace |
|---|---|---|---|
| FR-033 | `ClickOrchestrator` MUST drive the full SDD flow and explain each step/handoff in plain language as it moves between phases. | Must | D10; sdd-workflow.md §1 |
| FR-034 | `ClickOrchestrator`'s persona MUST be professional and plain-spoken — no unexplained jargon, no regional slang, no Gentleman persona carried over from reference projects. | Must | D10 |
| FR-035 | `ClickOrchestrator` MUST reply to developers in Spanish. | Must | D4, D10 |
| FR-036 | All artifacts `ClickOrchestrator` and its subagents produce (PRD, design, tasks, memory entries, docs) MUST be in English. | Must | D4, D10 |
| FR-037 | On activation (Claude Code opened after install), `ClickOrchestrator` MUST be the active orchestrator without additional developer configuration. | Must | mvp-scope.md §3 |

### 2.11 memory-guard PreToolUse hook

| ID | Requirement | MoSCoW | Trace |
|---|---|---|---|
| FR-038 | `memory-guard` MUST run as a Claude Code PreToolUse hook and intercept **every** `mem_save` call before it can reach Engram. | Must | D7 |
| FR-039 | `memory-guard` MUST decide allow/block/redact via deterministic pattern matching, independent of model judgment or discretion. | Must | D7 |
| FR-040 | `memory-guard` MUST enforce a deny-by-default / allowlist posture: content is persisted only if it matches an allowed technical-knowledge category (architecture/design decisions, conventions, patterns, gotchas, bugfixes). | Must | D6 |
| FR-041 | `memory-guard` MUST always block or redact content matching PII, policy numbers, claims (siniestros) data, amounts, or customer identifiers, with no exception path in v0.1. | Must | D6 |
| FR-042 | `memory-guard` MUST support both a **block** action (reject the save) and a **redact** action (strip the offending content before it reaches Engram). | Must | D7; architecture.md §5 |
| FR-043 | `memory-guard` MUST pass a 100% red-team PII/insurance-data test (every attempt blocked or redacted) as a hard gate before team-wide rollout proceeds. | Must | D11; adoption-plan.md §4 |
| FR-044 | `memory-guard` MUST be independently disable-able in Claude Code settings without a full `click uninstall`. | Must | adoption-plan.md §8 (= FR-014) |

### 2.12 Human-facing memory policy docs

| ID | Requirement | MoSCoW | Trace |
|---|---|---|---|
| FR-045 | `memory-policy.md` MUST document the deny-by-default posture and the rationale for the two-layer (policy + guard) enforcement model. | Must | D6, D7; architecture.md §5 |
| FR-046 | `allowed-memory.md` MUST enumerate the technical-knowledge categories the curator is permitted to propose for persistence. | Must | D6; **content OPEN** — see §6 |
| FR-047 | `forbidden-memory.md` MUST enumerate the data categories that are always forbidden (PII, policy numbers, claims data, amounts, customer identifiers). | Must | D6; **content OPEN** — see §6 |
| FR-048 | `engram-setup.md` MUST document how the bundled Engram instance is configured for a Click developer. | Must | architecture.md §4 |

### 2.13 CLAUDE.md rules install

| ID | Requirement | MoSCoW | Trace |
|---|---|---|---|
| FR-049 | The `CLAUDE.md` rules installed by `click install` MUST activate `ClickOrchestrator` automatically in any Click Claude Code session. | Must | architecture.md §2; mvp-scope.md §3 |
| FR-050 | The installed `CLAUDE.md` rules MUST be additive/mergeable with a developer's existing `CLAUDE.md`, and fully removable by `click uninstall` (= FR-013). | Must | adoption-plan.md §8 |

### 2.14 MCP configuration

| ID | Requirement | MoSCoW | Trace |
|---|---|---|---|
| FR-051 | The Engram MCP server entry written by `click install`/`click update` MUST reference the version pinned by the current click-ai-devkit release, not a version resolved independently at runtime. | Must | D8 |

### 2.15 Uninstall reversibility

| ID | Requirement | MoSCoW | Trace |
|---|---|---|---|
| FR-052 | Running `click uninstall` MUST return a developer's Claude Code setup to a state equivalent to before `click install` ran (plain Claude Code, no Click plugins, hook, or `CLAUDE.md` additions). | Must | adoption-plan.md §8 |
| FR-053 | Uninstall MUST be usable per-developer as the immediate-mitigation path if `memory-guard` misbehaves after team-wide rollout (no server-side kill switch exists in v0.1). | Must | adoption-plan.md §8 |

### 2.16 Won't (v0.1 non-goals, restated as scope boundaries)

| ID | Requirement | MoSCoW | Trace |
|---|---|---|---|
| FR-054 | click-ai-devkit will **not** reimplement Engram's persistence engine. | Won't | D1, D8 |
| FR-055 | click-ai-devkit will **not** install the full Gentle-AI ecosystem — only the adapted/rebranded subset Click needs. | Won't | D9; vision.md Non-goals |
| FR-056 | click-ai-devkit will **not** ship a usage dashboard or telemetry pipeline in v0.1; the success metric is self-reported. | Won't | vision.md Non-goals; mvp-scope.md §1 |
| FR-057 | click-ai-devkit will **not** support AI coding assistants other than Claude Code in v0.1. | Won't | vision.md Non-goals |
| FR-058 | click-ai-devkit will **not** auto-merge or auto-deploy; `click-sdd-review` stops at pre-merge, a human stays in the loop. | Won't | vision.md Non-goals |
| FR-059 | click-ai-devkit will **not** roll out to every Click repo on day one; adoption starts with the canary and expands deliberately. | Won't | D11; vision.md Non-goals |
| FR-060 | click-ai-devkit will **not** allow uncontrolled/global memory saves; every save is scoped and policy-gated from the start (no "save everything, sort later" mode). | Won't | D6; vision.md Non-goals |

## 3. Non-functional requirements

| ID | Requirement | MoSCoW | Trace |
|---|---|---|---|
| NFR-001 | **Security.** No PII, policy number, claims (siniestros) datum, amount, or customer identifier ever reaches Engram's persisted store, under any code path that calls `mem_save`. | Must | D6, D7 |
| NFR-002 | **Security — enforcement independence.** The PII/data-safety guarantee MUST hold even if the model attempting the save ignores or misreads the human-facing policy docs — enforcement lives in the deterministic hook, not in model compliance. | Must | D7; architecture.md §5 |
| NFR-003 | **Reproducibility.** The same click-ai-devkit release MUST produce the same installed plugin set and the same pinned Engram version on any developer's machine. | Must | D8 |
| NFR-004 | **Reproducibility.** `click update` MUST be idempotent — running it twice against the same release produces no further change and no error. | Must | D8 |
| NFR-005 | **Cross-platform.** The CLI binary MUST be buildable and runnable from the same Go codebase on Windows, macOS, and Linux; Windows/scoop is the only distribution channel committed for v0.1 (see FR-018, §6 Open requirements for brew/PS timing). | Must (binary), Should (packaging beyond scoop) | D5 |
| NFR-006 | **Performance.** The `memory-guard` hook MUST add negligible, imperceptible latency to a `mem_save` call under normal payload sizes (a developer should not notice a delay attributable to the guard). | Should | D7; architecture.md §5 (exact budget **OPEN** — no numeric target locked yet) |
| NFR-007 | **Maintainability.** The CLI MUST remain a thin installer/manager (install/update/doctor/uninstall only) — no SDD orchestration logic MUST be implemented in Go; the SDD flow stays markdown-first (agents/skills) executed by Claude Code. | Must | "CLI scope guardrail," decisions doc; architecture.md §1 |
| NFR-008 | **Auditability.** Every `memory-guard` decision (allow/block/redact) MUST be attributable after the fact well enough to support red-team test verification and false-positive/false-negative triage during and after the canary. | Should | D11; adoption-plan.md §4, §7 (exact logging mechanism **OPEN**) |
| NFR-009 | **Adoption/usability.** A Click developer MUST be able to go from a clean machine to a fully guarded, working setup via one CLI install command plus `click doctor` confirmation — no manual file copying into `~/.claude`. | Must | vision.md; mvp-scope.md §3 |
| NFR-010 | **Adoption/usability.** `ClickOrchestrator` explanations MUST be understandable by a developer without prior Claude Code/SDD experience — plain language, no unexplained jargon. | Must | D10; vision.md Principles |
| NFR-011 | **Privacy/compliance posture.** The memory system's default posture MUST be deny-by-default/allowlist, consistent with handling data for a regulated insurer — nothing is persisted unless explicitly categorized as safe technical knowledge. | Must | D6 |
| NFR-012 | **Reliability.** `click doctor` MUST accurately reflect the true state of each install step (plugins, `CLAUDE.md`, MCP entry, hook registration) — no false "healthy" reports. | Must | mvp-scope.md §3 |

## 4. Constraints

| Constraint | Source |
|---|---|
| CLI implemented in Go, shipped as a single static binary. | D5 |
| All artifacts (docs, README, `SKILL.md`, agents, memory-policy) MUST be in English; developer-facing conversation stays in Spanish. | D4, D10 |
| Built for Claude Code only — no other AI coding assistant integration in v0.1. | vision.md Non-goals |
| No usage dashboard or telemetry; success is measured via self-report. | vision.md Non-goals; mvp-scope.md §1 |
| Repo hosted in the Click Seguros GitHub org, private. | D2 |
| Engram is a bundled, pinned dependency — never reimplemented. | D1, D8 |
| The CLI is a thin installer/manager; the SDD flow itself lives in markdown, not in CLI code. | CLI scope guardrail, decisions doc |
| Team-wide rollout is gated by a hardening canary with a hard 100% red-team pass requirement — this gate cannot be relaxed to "mostly passes." | D11; adoption-plan.md §4 |

## 5. Assumptions & dependencies

| Assumption / dependency | Risk if false |
|---|---|
| Upstream Engram (`Gentleman-Programming/engram`) remains available and its releases can be pinned per click-ai-devkit release. | D8's reproducibility guarantee breaks; would need a fork or vendoring fallback. |
| Claude Code supports a PreToolUse hook capability sufficient to intercept every `mem_save` call deterministically. | D7's enforcement model has no equivalent mechanism; memory safety would fall back to policy-only (rejected posture). |
| Scoop is an acceptable and available package manager on Click developers' Windows machines. | D3's primary distribution channel would need to change or be supplemented immediately. |
| The Click Seguros GitHub org exists and a private repo can be created in it. | D2 cannot be satisfied as stated; blocks repo bootstrap. |
| Maintainers have or can acquire Go proficiency to build and maintain `cmd/click/`. | D5's accepted maintenance trade-off becomes a bottleneck; CLI could stall between releases. |
| Developers already have Claude Code installed independently of click-ai-devkit (the devkit installs *into* an existing Claude Code setup, it does not install Claude Code itself). | Install flow (architecture.md §2) assumes this precondition; if false, `click install` needs an additional prerequisite-check step. |

## 6. Open requirements

These are **not resolved** by this document — carried forward from `00-decisions-and-open-questions.md` §3 and the corresponding open-items sections of `architecture.md`, `mvp-scope.md`, `sdd-workflow.md`, and `adoption-plan.md`. No FR/NFR above should be read as silently resolving any of these.

| Open item | Affects | Where else it's tracked |
|---|---|---|
| SDD flow defaults: interactive-vs-automatic, and whether strict-TDD is on by default for `click-sdd-code`. | FR-029, FR-033 | sdd-workflow.md §5; mvp-scope.md §6 |
| Concrete PII/insurance pattern set for `memory-guard` (policy number formats, claim ID formats, amount patterns, etc.). | FR-039, FR-041, FR-043 | architecture.md §7; adoption-plan.md §9 |
| Authoring of `allowed-memory.md` and `forbidden-memory.md` content. | FR-046, FR-047 | decisions doc §3; architecture.md §7 |
| Authoring of `SECURITY.md`. | Referenced in repo structure (architecture.md §4) but not covered by any FR above. | mvp-scope.md §2, §6 |
| brew/PowerShell installer timing — whether either ships alongside the scoop bucket for v0.1 or is deferred. | FR-018, NFR-005 | D5; mvp-scope.md §1 |
| Scoop bucket repo ownership/location and its CI release process relative to click-ai-devkit itself. | FR-016 | architecture.md §7 |

## 7. Traceability — decisions to requirements

| Decision | Summary | Implementing requirement IDs |
|---|---|---|
| D1 | Engram bundled, not reimplemented | FR-001, FR-003, FR-019, FR-020, FR-054 |
| D2 | Repo hosted in Click Seguros GitHub org (private) | Constraint only (§4) — no FR; repo hosting is not a product behavior |
| D3 | CLI now, distributed via Click scoop bucket | FR-015, FR-016, FR-018, NFR-005 |
| D4 | All artifacts in English, conversation in Spanish | FR-035, FR-036; Constraint (§4) |
| D5 | Go CLI stack, single static binary | FR-017, FR-018, NFR-005, NFR-007; Constraint (§4) |
| D6 | Deny-by-default / allowlist memory policy | FR-031, FR-040, FR-041, FR-045–FR-047, NFR-001, NFR-011 |
| D7 | Deterministic PreToolUse memory-guard | FR-038, FR-039, FR-042, FR-044, NFR-001, NFR-002 |
| D8 | Engram bundled at latest, pinned per release | FR-006, FR-007, FR-019–FR-021, FR-051, NFR-003, NFR-004 |
| D9 | Reuse/rebrand existing SDD machinery | FR-022, FR-024, FR-025–FR-032, FR-055 |
| D10 | Professional plain-spoken orchestrator persona | FR-033–FR-037, FR-049, NFR-010 |
| D11 | Team-wide rollout gated by hardening canary | FR-043 (red-team gate); broader rollout process is a PRD/adoption-plan concern — see `prd.md` §Milestones and `adoption-plan.md` |
| D16 | Drop `.claude-plugin/marketplace.json` for v0.1; CLI uses embedded `manifest.yaml` | Architecture/repository structure constraint; no FR but defines CLI distribution model (vs. Marketplace) |
| D17 | Scoop only for v0.1; brew and PowerShell deferred | FR-018 (deferred); NFR-005 timing |
| D18 | Dedicated `click-seguros/scoop-bucket` repo, auto-published by CI on tag | FR-016 (scoop bucket channel); architecture/distribution constraint |
| D19 | Guard latency budget <50ms p95 | NFR-006 (sets performance constraint on `memory-guard`) |
| D20 | Guard audit log: local JSON, content hashes only, no telemetry | NFR-008 (audit trail mechanism); satisfies security constraint in FR-041 |
| D21 | memory-guard v0.1 block-only (no redaction) | FR-041, FR-042; satisfies fail-closed design principle for guard (defer redaction complexity to v0.2) |
| D22 | Pre-build spikes before coding | Architecture/implementation approach; not an FR/NFR but gates Slice 1 and Slice 2 (Engram packaging spike, PreToolUse contract spike) |

## 8. Related documents

- `vision.md` — problem, value proposition, principles, success metric definition.
- `architecture.md` — component design, install flow, security architecture.
- `mvp-scope.md` — in/out scope, deliverables checklist, acceptance criteria, risks.
- `sdd-workflow.md` — phase-by-phase detail and a worked example.
- `adoption-plan.md` — canary process, roles, go/no-go gate, rollback.
- `prd.md` — product framing of these requirements (personas, user stories, milestones).
