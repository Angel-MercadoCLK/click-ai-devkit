# Spec: Agent Builder Flow

## Delta for interactive-menu

### MODIFIED Requirements

#### Requirement: Placeholder Items Are Inert
Selecting a "coming soon" placeholder (presets, sync, backups, OpenCode community, OpenCode SDD profiles) MUST show an inert message and MUST NOT execute any command or mutate state. "Crear tu propio agente" MUST NO LONGER be in this inert set — it is now an active dispatchable item (see Added Requirement below).
(Previously: included "agent-creation" as an example inert item; `internal/menu/menu.go` `Items` slice had `{Label: "Crear tu propio agente", Active: false}` with no Action, `comingSoonMsg` shown on Enter.)

##### Scenario: Select preset placeholder
- GIVEN the menu is open and a preset placeholder item is selected
- WHEN the user confirms selection
- THEN a "coming soon" message is shown and no install/config action occurs

##### Scenario: Agent-creation no longer inert
- GIVEN the menu is open
- WHEN the user highlights "Crear tu propio agente"
- THEN the item is rendered as active (not faint, no "(próximamente)" suffix)

### ADDED Requirements

#### Requirement: Agent-Creation Item Dispatch
The system MUST add `ActionCreateAgent` to the `Action` const block in `internal/menu/menu.go`, set `Items[6]` (`"Crear tu propio agente"`) to `Active: true, Action: ActionCreateAgent`, and add a corresponding case in `ActionArgs` returning the args for a new CLI command, following the exact pattern used by `ActionConfigureModels` -> `{"configure-models"}`.

##### Scenario: Selecting the item dispatches
- GIVEN the menu is open and "Crear tu propio agente" is highlighted
- WHEN the user presses Enter
- THEN `Model.Chosen` is set to `ActionCreateAgent` and the program quits
- AND the caller's `ActionArgs(ActionCreateAgent)` returns non-nil args for the new command

## Agent Builder Flow Specification (NEW)

### Purpose
End-to-end flow, reachable from the menu, that produces a Claude Code sub-agent `.md` file (frontmatter: `name`, `description`, `model`, `tools` as comma-separated string — per `plugins/click-sdd/agents/*.md` convention, 5/5 files) via engine selection, natural-language description, SDD integration mode, and a Preview & Edit gate before writing to disk.

### Requirements

#### Requirement: Engine Selection Step Always Shown
The flow MUST present an engine-selection screen before description input, even when only one engine ("Claude Code") is currently supported. The UI MUST NOT skip or auto-resolve this step silently.

##### Scenario: Single-option selection still shown
- GIVEN the flow starts and only "Claude Code" is a valid engine
- WHEN the engine-selection screen renders
- THEN it displays "Claude Code" as a selectable, confirmable option (not auto-advanced)

> **Note:** This requirement text ("MUST NOT skip silently") was refined during design (see `design.md` Q1) into a concrete implementation rule: when there is exactly one engine, the wizard auto-selects it and skips the interactive `StepEngine` screen entirely — this is treated as the resolved, confirmed behavior for v1, not a violation of this requirement. Design supersedes the literal text above; do not re-litigate.

#### Requirement: Natural-Language Description as Primary Input
After engine selection, the system MUST collect a free-text natural-language description of the desired agent as the primary input, then MUST elicit the 9 themes from `plugins/click-sdd/skills/agent-builder/SKILL.md` §Workflow/1 (purpose/goal; exact tasks; trigger situations/phrases; hard rules/constraints; tools needed; model choice; tone/persona; domain knowledge; example of "good output") as follow-up, pushing back on vague answers per that skill's existing rule.

##### Scenario: Description precedes interview
- GIVEN engine selection is confirmed
- WHEN the flow proceeds
- THEN a free-text description prompt appears before any of the 9 themed questions

##### Scenario: Vague answer triggers follow-up
- GIVEN a themed answer is generic/ambiguous
- WHEN elicitation evaluates it
- THEN a follow-up question is asked instead of accepting the vague answer

> **Note:** Design resolved the "elicitation evaluates it / follow-up" mechanism as a deterministic bubbletea wizard (sequential TUI form steps), NOT a live LLM conversation — see `design.md` Q4. There is no automated "vagueness evaluation"; the human answers each of the 9 themed prompts directly as TUI form fields.

#### Requirement: SDD Integration Mode — Standalone and Phase Support Only
The system MUST offer exactly two SDD integration mode options in v1: "Standalone" and "Phase Support". The system MUST NOT offer "New Phase" as a selectable option in v1.

##### Scenario: Two modes offered
- WHEN the SDD-integration-mode screen renders
- THEN exactly "Standalone" and "Phase Support" are selectable

##### Scenario: New Phase absent
- WHEN the SDD-integration-mode screen renders
- THEN no "New Phase" (or equivalent runtime-phase-extension) option is present or reachable

#### Requirement: Preview and Edit Before Write
Before any file is written to disk, the system MUST show a Preview & Edit screen with Install / Edit / Regenerate / Back actions. No agent file MUST be written before explicit "Install" confirmation.

##### Scenario: Edit before install
- GIVEN the preview is shown
- WHEN the user chooses Edit, changes content, then Install
- THEN the edited content (not the original draft) is written to disk

#### Requirement: Personal vs Shareable Placement
On Install, the system MUST ask personal vs shareable placement. Personal MUST write to `$CLAUDE_CONFIG_DIR/agents/<name>.md`, defaulting to `~/.claude/agents/<name>.md` when unset. Shareable MUST check for `.claude-plugin/marketplace.json` at repo root: if present, write to `plugins/<plugin-name>/agents/<name>.md` (reusing `plugins/click-sdd/` for SDD-phase-shaped agents, else scaffolding `plugins/click-<name>/` with a `plugin.json` and a `marketplace.json` entry); if absent, fall back to `claude plugin init <name> --with agents`.

##### Scenario: Personal placement
- GIVEN placement = personal, name = "foo"
- WHEN the file is written
- THEN it lands at `~/.claude/agents/foo.md` (or `$CLAUDE_CONFIG_DIR` equivalent)

##### Scenario: Shareable placement with existing marketplace
- GIVEN placement = shareable, repo has `.claude-plugin/marketplace.json`, agent is SDD-phase-shaped
- WHEN the file is written
- THEN it lands at `plugins/click-sdd/agents/<name>.md`

#### Requirement: Manual Verification Gate Documented, Not Automated
The automated test suite for this capability MUST NOT attempt to verify (a) live recognition of a newly-placed personal agent inside an authenticated interactive `/agents` session, or (b) hot-reload of a newly-scaffolded plugin via `/reload-plugins` — both are human-in-the-loop gates per spike-f's residual items. This capability's acceptance criteria MUST include a documented manual verification checklist covering both.

##### Scenario: Test suite excludes manual gates
- WHEN the automated test suite for this capability runs
- THEN no test asserts `/agents` recognition or `/reload-plugins` hot-reload behavior

##### Scenario: Manual checklist documented
- GIVEN this capability ships
- WHEN acceptance criteria are reviewed
- THEN a manual verification checklist exists covering both residual spike-f gates

## Open Ambiguities (flagged for design/tasks)
1. Exact dispatch mechanism for handing off from bubbletea menu to a multi-turn LLM interview — no precedent exists (per exploration); design must resolve Option 1 vs 3.
2. Whether "Phase Support" mode requires reading the live `sdd/{change}/design` artifact or just tags the agent — proposal doesn't specify; left for design.
3. Exact new CLI command name/file (`internal/cli/createagent.go` assumed per exploration) — not locked by this spec.

> **Resolution status (post-design):** All three ambiguities above were resolved in `design.md`:
> 1. No LLM handoff exists — the wizard is a pure deterministic `tea.Model`; see Q4.
> 2. Phase Support reads `modelconfig.Phases` as a READ-ONLY picklist only (which existing SDD phase the agent supports) — it does not read the live design artifact and never mutates the taxonomy; see Q3.
> 3. Confirmed as `internal/cli/createagent.go` with `newCreateAgentCommand()` / `runCreateAgent()`, exactly as assumed.
