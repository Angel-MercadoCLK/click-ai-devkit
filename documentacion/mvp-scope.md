# click-ai-devkit — MVP Scope (v0.1)

> Status: draft v0.1. Grounded in `00-decisions-and-open-questions.md` (D1–D20).

## 1. In scope vs out of scope

### In scope (v0.1)

| Area | What's included |
|---|---|
| Distribution | Go CLI (`click`) with `install`, `update`, `doctor`, `uninstall`. Primary channel: Click scoop bucket (D3, D5). |
| Plugins | Three markdown plugins: `click-sdd/`, `click-memory/`, `click-review/` (D9, architecture.md §1). |
| SDD flow | `click-sdd-explore → click-sdd-prd → click-sdd-design → click-sdd-tasks → click-sdd-code → click-sdd-review → click-memory-curator → Engram`, rebranded from existing SDD machinery (D9). |
| Orchestrator | `ClickOrchestrator`: professional, plain-spoken teacher persona; replies in Spanish, artifacts in English (D10). |
| Memory | Deny-by-default / allowlist policy (D6) + deterministic `memory-guard` PreToolUse hook (D7), plus human-facing policy docs. |
| Engram | Bundled, pinned per click-ai-devkit release (D1, D8). Not reimplemented. |
| Pilot | Team-wide rollout gated by a 3–5 day hardening canary (D11). |
| Metric | Minutes of context re-explanation avoided per session, via before/after self-report (D11, vision.md). |

### Out of scope (v0.1)

Per `vision.md` Non-goals — restated here as scope boundaries:

| Excluded | Why |
|---|---|
| Reimplementing Engram | Bundled dependency, pinned per release, not rebuilt (D1, D8). |
| Full Gentle-AI ecosystem | Only what Click needs is adapted/rebranded (D9). |
| Usage dashboard / telemetry | Success is measured by self-report in v0.1, not instrumentation. |
| Support for AI tools other than Claude Code | Single-tool scope for v0.1. |
| Auto-merge / auto-deploy | A human stays in the loop; `click-sdd-review` is pre-merge, not automated merge. |
| Rolling out to every Click repo on day one | Starts with the pilot; expands deliberately after go/no-go. |
| Uncontrolled/global memory | Memory is scoped and policy-gated from the start (D6). |
| brew / PowerShell installers | Optional later per D5 — timing not committed for v0.1 (open item). |

## 2. Concrete deliverables

Per the repo structure in `architecture.md` §4:

- [ ] Repo skeleton in the Click Seguros GitHub org (private) (D2)
- [ ] `README.md`
- [ ] `CLAUDE.md` (Click conventions, orchestrator activation rules)
- [ ] `SECURITY.md` (not yet authored — open item)
- [ ] Embedded CLI manifest (`manifest.yaml`) — NOT `.claude-plugin/marketplace.json` (dropped for v0.1 per D16)
- [ ] Go CLI at `cmd/click/` with `install`, `update`, `doctor`, `uninstall`
- [ ] `plugins/click-sdd/` — `click-orchestrator.md`, `click-prd-writer.md`, `click-architect.md`, `click-reviewer.md`, `click-memory-curator.md`, and the six `SKILL.md` files (`sdd-explore`, `sdd-prd`, `sdd-design`, `sdd-tasks`, `sdd-code`, `sdd-review`)
- [ ] `plugins/click-memory/` — `memory-guard` PreToolUse hook, `memory-proposal`/`memory-review` skills, and docs: `memory-policy.md`, `allowed-memory.md`, `forbidden-memory.md`, `engram-setup.md`
- [ ] `plugins/click-review/` — `click-pr-reviewer.md` agent, `pr-review`/`pre-merge-checklist` skills
- [ ] Planning docs (this set): `vision.md`, `architecture.md`, `mvp-scope.md`, `sdd-workflow.md`, `adoption-plan.md`, `references.md`

## 3. MVP acceptance criteria (definition of done)

A Click Seguros developer can go from zero to a working, guarded setup:

- [ ] `scoop bucket add click-seguros https://github.com/click-seguros/scoop-bucket` then `scoop install click` succeeds on a clean Windows machine (D18)
- [ ] `click install` completes without error and:
  - [ ] copies `click-sdd`, `click-memory`, `click-review` into `~/.claude/plugins/`
  - [ ] writes/updates `CLAUDE.md` rules (Click conventions + orchestrator activation)
  - [ ] configures the Engram MCP server entry at the pinned version (D8)
  - [ ] registers the `memory-guard` PreToolUse hook in Claude Code settings
- [ ] `click doctor` reports all of the above as healthy
- [ ] Opening Claude Code activates `ClickOrchestrator` (replies in Spanish, produces English artifacts — D10)
- [ ] The `click-sdd-*` flow is invocable end to end: explore → prd → design → tasks → code → review
- [ ] Engram memory is online: a `mem_save` from `click-memory-curator` is retrievable in a later session
- [ ] `memory-guard` is enforcing: a red-team PII/insurance-data payload sent to `mem_save` is blocked or redacted before reaching Engram, in every test case (100% pass rate required per D11)
- [ ] `click update` re-syncs plugins and the pinned Engram version without manual file edits
- [ ] `click uninstall` reverses the install (removes plugins, hook registration, CLAUDE.md additions)

## 4. Success criterion + metric (restated)

**Primary metric:** minutes of context re-explanation avoided per session.

Measured via before/after self-report starting on day one of the canary — not automated telemetry (no dashboard in v0.1, per Non-goals in `vision.md`).

## 5. Risks and mitigations

| Risk | Mitigation |
|---|---|
| `memory-guard` has a gap and lets sensitive data through | Hardening canary (observe+block mode) + mandatory 100% red-team PII test before team-wide rollout (D11) |
| Go CLI needs ongoing maintenance the team may not be resourced for | Accepted deliberately as the cost of install-UX control (D5); scope kept thin (install/update/doctor/uninstall only) |
| Upstream Engram changes break compatibility | Engram is pinned per click-ai-devkit release, not floating latest (D8); updates are explicit via `click update` |
| Devs paste real insurance/PII data into a prompt that then gets summarized into memory | Two-layer defense: human-facing policy docs + deterministic guard, independent of model behavior (D6, D7) |
| Installation distribution path was ambiguous (marketplace vs. CLI) | Resolved by D16: marketplace.json dropped for v0.1; CLI uses embedded manifest.yaml; native Marketplace is a v0.2-optional path |
| Canary too small/short to catch real-world edge cases | Canary scope (3–5 days, 2–3 devs) is a deliberate trade-off per D11; go/no-go gate can extend or repeat the canary if issues surface |

## 6. Open items (not resolved here — carried forward from the decisions doc)

- SDD flow defaults: interactive vs. automatic, strict-TDD on/off by default.
- Concrete PII/insurance pattern set for `memory-guard`.
- Authoring of `allowed-memory.md`, `forbidden-memory.md`, `SECURITY.md`.
- brew/PowerShell installer timing (optional later per D5).
- Scoop bucket repo location/CI relative to click-ai-devkit itself.
