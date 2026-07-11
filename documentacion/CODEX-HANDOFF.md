# Handoff for Codex — click-ai-devkit

This document is a self-contained handoff for an AI coding agent (Codex / OpenAI Codex CLI) with **zero memory** of the conversation that produced it. Everything you need to understand the product, what has already shipped, and what to implement next is either in this file or in the four linked SDD artifacts under `documentacion/sdd/agent-builder-flow/`. You should not need to ask the user anything covered below — except the one open question flagged in the "What Codex needs to implement now" section, which you must confirm before starting implementation.

## Original product vision

`click-ai-devkit` builds `click`, a Go CLI meant to be an installable Claude Code ecosystem manager: a custom SDD orchestrator, agents/skills, a memory-safety guard, and a bundled Engram instance. It is modeled on a real reference tool called **gentle-ai** (https://github.com/Gentleman-Programming/gentle-ai) and its companion memory system **engram** (https://github.com/Gentleman-Programming/engram). The product vision, at gentle-ai parity, is:

- `click` should auto-install its dependencies (Engram) the way `brew install gentleman-programming/tap/engram` would — not just "prepare the environment."
- `click` needs gentle-ai's actual interactive TUI experience: running the bare command shows a navigable menu (`j`/`k` navigation) with items — Start installation, Upgrade tools, Sync configs, Upgrade + Sync, Configure models, Create your own Agent, OpenCode Community Plugins, OpenCode SDD Profiles, Manage backups, Managed uninstall, Quit — with an available-updates indicator at the top.
- A "Configure models" screen (gentle-ai's "Custom Claude Assignments") lets the user pick a model (sonnet/opus/haiku) and effort level per SDD phase: Explore, Propose, Spec, Design, Tasks, Apply, Verify, Archive, Onboard, JD Judge A, JD Judge B, JD Fix Agent, General delegation.
- A "Select Ecosystem Preset" screen at install time (Dev Stack + Polish / Dev Stack / Memory Only / Custom).
- A "Create your own Agent" flow. gentle-ai's real version does: engine/target selection → natural-language agent description → SDD integration mode (Standalone / New Phase / Phase Support) → Preview & Edit → cross-engine install, producing a portable `SKILL.md`.

Before this work started, NONE of this interactivity existed in `click` — it only had `install`/`doctor`/`uninstall`/`update` subcommands, no menu, and a model-config taxonomy that didn't match the real SDD phases.

## What's already shipped

A full SDD cycle (propose → spec → design → tasks → apply → verify) was delivered for the change **`interactive-menu-and-model-taxonomy`**, as 3 chained PRs, stacked-to-main. This was independently verified against the live GitHub API (not just subagent claims): all 3 PRs were OPEN, MERGEABLE, with correct stacked base branches, and CI green (ubuntu-latest + windows-latest, GitHub Actions "CI" workflow) on all three at time of writing.

This closed the two P0 gaps identified during exploration: no standing interactive menu, and a model-config taxonomy (5 invented phases) that didn't match the real 12-13 phase SDD taxonomy.

### The 3 PRs, in required merge order

1. **PR1** — https://github.com/Angel-MercadoCLK/click-ai-devkit/pull/1 — branch `feat/model-config-taxonomy-realignment` → base `main`.
   Rewrote `internal/modelconfig` to the real 13-phase `Phase`/`Phases` taxonomy (`explore, propose, spec, design, tasks, apply, verify, archive, onboard, jd-judge-a, jd-judge-b, jd-fix-agent, default`). Introduced a versioned `models.json` (`schema_version=2`) with a backup-then-regenerate migration wired into `doctor`/`update`/`install`.

2. **PR2** — https://github.com/Angel-MercadoCLK/click-ai-devkit/pull/2 — branch `feat/interactive-menu` → base is PR1's branch (`feat/model-config-taxonomy-realignment`).
   New `internal/menu` standing bubbletea TUI, `internal/cli/rootdefault.go` default no-arg action with a TTY/CI gate and loop-back-after-dispatch, a hidden `click configure-models` subcommand. All menu text is in Spanish (repo convention).

3. **PR3** — https://github.com/Angel-MercadoCLK/click-ai-devkit/pull/3 — branch `feat/skill-content-taxonomy` → base is PR2's branch (`feat/interactive-menu`).
   Rewrote/authored `plugins/click-sdd/skills/{explore,propose,spec,design,tasks,apply,verify,archive,onboard,jd-judge-a,jd-judge-b,jd-fix-agent}/SKILL.md` (6 renamed+rewritten, 6 net-new; the `default` phase intentionally has no skill file). Fixed a real bug in `click-orchestrator.md` (stale dead config-key references left over from before PR1). Added `internal/installer/plugins_lockstep_test.go`, which guards taxonomy/skill-dir/plugin.json consistency going forward.

### Exact merge order instructions

Because this is a stacked chain, merge in this exact sequence:

1. Merge **PR1** into `main`.
2. Retarget **PR2**'s base branch from `feat/model-config-taxonomy-realignment` to `main`.
3. Merge **PR2** into `main`.
4. Retarget **PR3**'s base branch from `feat/interactive-menu` to `main`.
5. Merge **PR3** into `main`.

Do not merge out of order — PR2 depends on PR1's taxonomy types, and PR3 depends on PR2's menu wiring.

### Review results

All 3 PRs passed a full 4R adversarial review (risk / reliability / resilience / readability lenses) with 3-way refutation and 2 fix rounds. All BLOCKER/CRITICAL findings were resolved and verified via scoped re-reviews:

| id | lens | location | severity | resolution |
|----|------|----------|----------|------------|
| R3-001 | reliability | `internal/cli/install.go` | CRITICAL | Fixed — `resolveInstallModels` extracted; migration only runs on non-interactive or interactive-confirmed paths; interactive-cancel leaves disk untouched. |
| R4-001 | resilience | `internal/cli/rootdefault.go` + `root.go` | CRITICAL | Fixed — `SilenceUsage` global + `errMenuDispatchFailed` sentinel + `silenceIfAlreadyReported`: menu failures print exactly once, no usage dump. |
| RR1-001 | reliability | `internal/cli/root.go` | CRITICAL | Fixed — `SetFlagErrorFunc` restores `Usage:` for genuine flag-parse errors (this was a regression introduced by the R4-001 round-1 fix; resolved in round 2). |
| R2-001 | readability | `click-prd-writer.md` | CRITICAL | Fixed — terminology reconciled: the agent owns the propose phase; PRD is explicitly defined as this plugin's name for the proposal artifact. |
| R2-002 | readability | `click-orchestrator.md` | WARNING | Fixed — no "PRD" string remains in the orchestrator doc. |

Four WARNING-level findings remain open as non-blocking `info` (not re-reviewed, per severity floor): a cosmetic Usage/Error ordering deviation in `root.go:41-42`, a stale skill reference to a nonexistent `/sdd-explore` slash command, an unreachable/misattributed branch in `rootdefault.go`, and a non-atomic `models.json` regen-write pattern in `models.go`/`hooksettings.go` (flagged as a candidate for a small future follow-up: atomic temp+rename write).

R1 (risk) came back explicitly clean: backup ordering, exec arg-slices, 0600 file perms, no secrets, dispatch whitelist all verified sound.

**One item is still pending outside the code review**: a manual real-terminal (PTY) smoke test of the bubbletea menu, owed by the user before PR2/PR3 are considered fully done from a UX standpoint. This does not block merging from a correctness standpoint (CI is green and the review is clean), but flag it if the user asks about outstanding manual work.

## What's still a placeholder / not planned

**Ecosystem install presets** (Dev Stack + Polish / Dev Stack / Memory Only / Custom — gentle-ai's "Select Ecosystem Preset" screen) remain an inert menu placeholder in the shipped chain above. There is currently **no SDD change planned** for this. If the user wants it built, it needs its own `sdd-explore` → `sdd-propose` → ... cycle from scratch; do not assume any design decisions exist for it yet.

## What Codex needs to implement now: agent-builder-flow

A full SDD planning cycle (propose → spec → design → tasks) was completed for a change named **`agent-builder-flow`**, which turns the menu's "Crear tu propio agente" item from an inert placeholder into a real, working create-your-own-agent flow. This is **planned but not yet implemented** — implementation (`sdd-apply`) is your job.

Read these four files in full, in this order, before writing any code:

1. `documentacion/sdd/agent-builder-flow/proposal.md` — intent, scope, affected areas, risks.
2. `documentacion/sdd/agent-builder-flow/spec.md` — formal requirements and scenarios (the contract your implementation must satisfy).
3. `documentacion/sdd/agent-builder-flow/design.md` — the technical approach, architecture decisions (Q1–Q4), full Go interfaces/contracts, file-change list, and testing strategy.
4. `documentacion/sdd/agent-builder-flow/tasks.md` — the ordered, TDD-structured task checklist (RED/GREEN pairs) you should execute directly.

### Key confirmed decisions — do not re-litigate these

These were explicitly decided and, where marked, confirmed by the user during the planning session. Treat them as final:

- **v1 targets Claude Code only** as the engine. The implementation uses a real `[]Engine` type (extensible for future engines), but the wizard auto-selects it and skips the interactive engine-picker screen when there is only one engine registered.
- **v1 generates a NATIVE Claude Code sub-agent `.md`** file (YAML frontmatter: `name`/`description`/`model`/`tools` as a comma-separated string, matching the repo's existing 5 agents exactly). This is explicitly **NOT** gentle-ai's portable `SKILL.md` format. The user confirmed this trade-off explicitly after being told it diverges from literal gentle-ai parity — do not silently "fix" this back toward SKILL.md.
- **SDD integration mode: Standalone + Phase Support only in v1.** "New Phase" mode is explicitly excluded — it would require making the compile-time `modelconfig.Phases` taxonomy runtime-extensible, which is out of scope for this change. `modelconfig.Phases` must remain untouched, read-only.
- **No live LLM conversation inside the Go binary.** This is a deterministic bubbletea wizard that collects a natural-language description plus the 9 interview themes from the existing `plugins/click-sdd/skills/agent-builder/SKILL.md` as sequential TUI form steps — NOT an actual AI back-and-forth. There is no `claude -p` shell-out and no Agent/Task tool invocation mid-flow. The existing conversational `agent-builder` skill is untouched and remains a completely separate path.
- **Dispatch mechanism**: a hidden `click create-agent` cobra command (`Use:"create-agent"`, `Hidden:true`), following exactly the same pattern as the already-shipped `click configure-models` command, wired into the menu's currently-inert "Crear tu propio agente" item (`Items[6]` in `internal/menu/menu.go`).
- **STRICT TDD is mandatory for every task** — write a failing test first, then the minimal implementation to pass it, then refactor. This is repo policy (see the repo-root `CLAUDE.md`, decision D13), not optional, and applies to this change exactly as it did to the 3 already-shipped PRs.

### Review Workload Forecast — read before starting

| Field | Value |
|-------|-------|
| Estimated changed lines | ~1100-1300 |
| 400-line budget risk | High |
| Chained PRs recommended | Yes |
| Suggested split | PR 1 (agentbuilder domain) → PR 2 (wizard UI) → PR 3 (CLI + menu wiring) |
| Delivery strategy | ask-on-risk |

Suggested work units (see `tasks.md` for the full breakdown and file lists):

1. **Unit 1** — `internal/agentbuilder/{engine,spec,writer}.go` + tests. Pure domain logic, headless-testable, no UI/CLI dependency. ~600 lines including golden tests.
2. **Unit 2** — `internal/ui/agentbuilder.go` + test. The wizard `tea.Model`, depends on Unit 1's types. ~500 lines including Update/View tests.
3. **Unit 3** — `internal/cli/createagent.go` + `root.go` + `menu.go`/`menu_test.go` wiring. Depends on Units 1–2. ~250 lines.

### Open question you must confirm with the user before starting — do not assume silently

The **chain strategy** for these 3 PRs is explicitly marked `pending` in the tasks artifact. Two options exist:

- **feature-branch-chain**: stack the new PRs on top of the tip of the prior (still-unmerged, at planning time) chain, `feat/skill-content-taxonomy`.
- **stacked-to-main**: same stacking pattern used for the prior 3-PR chain, but rebased onto `main` once the prior chain is merged.

The tasks artifact recommends `feature-branch-chain` as the precedent-consistent default (since this change was planned on top of an unmerged prior chain), but this is a recommendation, not a decision. **Confirm with the user which chain strategy to use before creating any branches or PRs.** By the time you read this, the prior 3-PR chain (see "What's already shipped" above) should already be merged to `main` — verify its actual merge state via `gh pr view` before deciding whether to branch from `main` directly or from `feat/skill-content-taxonomy`.

## Manual verification required after implementation

Two items are explicitly out of scope for automated testing (per spec.md's "Manual Verification Gate Documented, Not Automated" requirement) and must stay as a documented manual checklist — do not attempt to write tests for these, and do not claim they are "done" from code alone:

1. **Live `/agents` recognition**: write a personal agent via the new flow, open `/agents` in a real, authenticated Claude Code session, and confirm the new agent is recognized and listed.
2. **`/reload-plugins` hot-reload**: write a shareable agent that scaffolds a new plugin, run `/reload-plugins` in a real Claude Code session, and confirm hot-reload picks it up without a restart.

Both are residual items from an earlier spike (`spike-f-agent-builder.md`) that could not be resolved by code — they require a human operator in a live session. Leave Phase 5 of `tasks.md` as an explicit manual checklist item, not an automated test.

## How to verify before considering this done

Before opening any PR:

1. `go build ./...` — must succeed with no errors.
2. `go vet ./...` — must be clean.
3. `go test ./... -count=1` — all tests green, including the new STRICT-TDD tests for this change.
4. `gofmt -l <touched files>` — must produce no output (i.e., all touched files are already gofmt-clean).
5. Run your own equivalent of a fresh adversarial review before opening PRs, mirroring what was done for the prior `interactive-menu-and-model-taxonomy` chain: review the complete diff across correctness, security, resilience, and readability lenses. Any BLOCKER/CRITICAL finding must be fixed and re-verified (re-review only the fix diff, not the full original diff again) before the PR is considered mergeable. WARNING/SUGGESTION-level findings can be logged as non-blocking info and left open.

Only after all of the above are green, and the chain-strategy question above has been confirmed with the user, should PRs be opened.
