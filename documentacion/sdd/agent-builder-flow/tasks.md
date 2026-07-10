# Tasks: Agent Builder Flow

Handoff note for executor (Codex or other): this list assumes ONLY the persisted spec (`spec.md`) and design (`design.md`) artifacts in this same directory as context, plus this file. Read both fully before starting. Go 1.24.2, cobra/bubbletea, STRICT TDD mandatory (`go test ./...` — write a FAILING test first for every unit, then minimal code to pass, then refactor).

## Guardrails — do NOT touch / do NOT add
- `modelconfig.Phases` stays READ-ONLY and UNMODIFIED. Phase Support mode only reads it as a picklist; never mutate it, never make it runtime-extensible.
- NO LLM invocation anywhere in this flow (no `claude -p`, no Agent/Task tool mid-flow). The wizard is a pure deterministic `tea.Model` — human types answers into TUI prompts.
- Do NOT implement "New Phase" as an SDD integration mode. Only `SDDStandalone` and `SDDPhaseSupport` exist in v1.
- Do NOT emit a portable `SKILL.md` format. Output is a native Claude Code sub-agent `.md` (YAML frontmatter `name`/`description`/`model`/`tools` comma-string) matching `plugins/click-sdd/agents/*.md` exactly.
- `plugins/click-sdd/skills/agent-builder/SKILL.md` stays UNTOUCHED — separate conversational alt path, out of scope.

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~1100-1300 (3 new packages/files + goldens + wizard + CLI + menu wiring) |
| 400-line budget risk | High |
| Chained PRs recommended | Yes |
| Suggested split | PR 1 (agentbuilder domain) → PR 2 (wizard UI) → PR 3 (CLI + menu wiring) |
| Delivery strategy | ask-on-risk |
| Chain strategy | pending (orchestrator must ask user before apply; branch context — this change plans on top of `feat/skill-content-taxonomy`, tip of a prior shipped chain — suggests `feature-branch-chain` as the precedent-consistent default to propose) |

Decision needed before apply: Yes
Chained PRs recommended: Yes
Chain strategy: pending
400-line budget risk: High

### Suggested Work Units
| Unit | Goal | Likely PR | Notes |
|------|------|-----------|-------|
| 1 | `internal/agentbuilder/{engine,spec,writer}.go` + tests — pure domain, headless-testable, no UI/CLI dependency | PR 1 | ~600 lines incl. golden tests. If `feature-branch-chain`: base = `feat/skill-content-taxonomy`. |
| 2 | `internal/ui/agentbuilder.go` + test — wizard `tea.Model`, depends on Unit 1 types | PR 2 | ~500 lines incl. Update/View tests. Base = PR 1 branch. |
| 3 | `internal/cli/createagent.go` + `root.go` + `menu.go`/`menu_test.go` wiring — depends on Units 1-2 | PR 3 | ~250 lines. Base = PR 2 branch. |

## Phase 1: Domain — `internal/agentbuilder/`
- [ ] 1.1 RED `engine_test.go`: `DefaultEngine()` returns `(ClaudeCode, true)` when `len(Engines())==1`.
- [ ] 1.2 GREEN `engine.go`: `Engine` struct, `Engines()`, `DefaultEngine()`.
- [ ] 1.3 RED `spec_test.go`: `SDDModes()` returns exactly `[SDDStandalone, SDDPhaseSupport]`; no "New Phase" value exists.
- [ ] 1.4 GREEN `spec.go`: `AgentSpec`, `SDDMode` consts (2 only), `Placement` consts, per design's Interfaces/Contracts section.
- [ ] 1.5 RED `writer_test.go`: `RenderAgentMarkdown(spec)` golden-string test — frontmatter matches `plugins/click-sdd/agents/*.md` shape (`name`/`description`/`model`/`tools` comma-string) exactly.
- [ ] 1.6 GREEN `writer.go`: `RenderAgentMarkdown()` (pure template fn).
- [ ] 1.7 RED `writer_test.go`: `TargetPath()` — personal → `$CLAUDE_CONFIG_DIR/agents/<name>.md` (default `~/.claude/agents/<name>.md`); shareable with `.claude-plugin/marketplace.json` present → `plugins/click-sdd/agents/<name>.md` (SDD-phase-shaped) or scaffolds `plugins/click-<name>/`; shareable without marketplace.json → falls back to `claude plugin init <name> --with agents` path.
- [ ] 1.8 GREEN `writer.go`: `TargetPath()`.
- [ ] 1.9 RED `writer_test.go`: `Install()` with fake `FileWriter` (in-memory, captures path+bytes) — no real disk I/O in tests.
- [ ] 1.10 GREEN `writer.go`: `FileWriter` interface, `Install()`.

## Phase 2: Wizard UI — `internal/ui/agentbuilder.go`
- [ ] 2.1 RED `agentbuilder_test.go`: `NewAgentBuilderModel([]Engine{ClaudeCode})` starts at `StepDescription` (auto-skips `StepEngine` when `len==1`).
- [ ] 2.2 GREEN: `Step` consts, `AgentBuilderModel`, `NewAgentBuilderModel` auto-select logic.
- [ ] 2.3 RED: with 2 fake engines, model starts at `StepEngine` and shows both as selectable/confirmable (future-proofing; spec requires this step never silently skipped when >1 engine).
- [ ] 2.4 GREEN: `Update()` `StepEngine` handling.
- [ ] 2.5 RED: `StepDescription` free-text input captured via `tea.KeyMsg`, occurs before any of the 9 themed prompts.
- [ ] 2.6 GREEN: `Update()` `StepDescription`.
- [ ] 2.7 RED: `StepSDDMode` offers exactly 2 options (Standalone/PhaseSupport); no "New Phase" option reachable via any key sequence.
- [ ] 2.8 GREEN: `Update()` `StepSDDMode`.
- [ ] 2.9 RED: `StepPhase` shown only when `Spec.SDDMode==SDDPhaseSupport`, else auto-skips straight to `StepThemes`.
- [ ] 2.10 GREEN: `Update()` `StepPhase` auto-skip (reads `modelconfig.Phases` read-only).
- [ ] 2.11 RED: 9 sequential themed steps populate `Spec.Purpose, Tasks, Triggers, Rules, Tone, Domain, GoodOutput` (+ Tools/Model) per spec's §Workflow/1 theme list.
- [ ] 2.12 GREEN: `Update()` `StepThemes` sequence.
- [ ] 2.13 RED: `StepPreview` exposes Install/Edit/Regenerate/Back; choosing Edit then changing content means the EDITED text (not original draft) is what's on `Spec` after confirming.
- [ ] 2.14 GREEN: `Update()`/`View()` `StepPreview`.
- [ ] 2.15 RED: `StepPlacement` (personal/shareable) then `Confirmed=true` set ONLY after explicit "Install" — never before.
- [ ] 2.16 GREEN: `Update()` `StepPlacement`, `Confirmed`/`Cancelled` flags.

## Phase 3: CLI command — `internal/cli/createagent.go`
- [ ] 3.1 RED `createagent_test.go`: non-TTY writer (not `*os.File`) → prints info message, does NOT start bubbletea (mirror `configuremodels_test.go` pattern exactly).
- [ ] 3.2 GREEN: `newCreateAgentCommand()` (`Use:"create-agent"`, `Hidden:true`), `runCreateAgent()` TTY guard.
- [ ] 3.3 GREEN: wire `runAgentBuilderTUI` → on `Confirmed`, call `agentbuilder.Install(spec, claudeHome, repoRoot, w)`.
- [ ] 3.4 Modify `internal/cli/root.go`: add `newCreateAgentCommand()` to `root.AddCommand(...)` near the existing `newConfigureModelsCommand()` registration (~line 56-63).

## Phase 4: Menu dispatch — `internal/menu/menu.go`
- [ ] 4.1 RED `menu_test.go`: assert `Items[6]` (`"Crear tu propio agente"`) has `Active==true`, `Action==ActionCreateAgent` (was `Active:false`, no Action).
- [ ] 4.2 RED `menu_test.go`: assert `ActionArgs(ActionCreateAgent) == []string{"create-agent"}`.
- [ ] 4.3 GREEN `menu.go`: add `ActionCreateAgent` to the `Action` const block, flip `Items[6]`, add `ActionArgs` case mirroring `ActionConfigureModels -> {"configure-models"}`.

## Phase 5: Manual Verification (NOT automated — do not write a test for this)
- [ ] 5.1 MANUAL checklist, run by a human operator in a live authenticated Claude Code session AFTER shareable-placement code ships:
  - (a) Write a personal agent via the flow, open `/agents` in that session, confirm the new agent is recognized/listed.
  - (b) Write a shareable agent that scaffolds a new plugin, run `/reload-plugins`, confirm hot-reload picks it up without restart.
  - These 2 gates are explicitly excluded from the automated suite per spec's "Manual Verification Gate Documented, Not Automated" requirement — spike-f residual items, human-in-the-loop only.
