# Proposal: Agent Builder Flow (gentle-ai parity)

## Intent
The menu item "Crear tu propio agente" (`internal/menu/menu.go`) is an inert `Active:false` placeholder. Users cannot create their own agents from click-ai-devkit. This change delivers a real, gentle-ai-parity agent-builder: engine selection -> natural-language description -> SDD integration mode -> Preview & Edit -> install. It builds on the just-shipped taxonomy/menu/skills chain (`feat/skill-content-taxonomy`) and repurposes the existing `plugins/click-sdd/skills/agent-builder/SKILL.md` interview as the underlying elicitation mechanism.

## Scope
### In Scope
- Flip the menu placeholder to a real dispatch (ActionCreateAgent const + Active:true + ActionArgs case).
- Engine/target selection step (Claude Code as the only real target today; extensible shape per gentle-ai, not multi-engine code yet).
- Natural-language description as primary input, with the existing 9-theme interview as the follow-up elicitation layer.
- SDD integration mode selection: Standalone / New Phase / Phase Support (New Phase scope for v1 is a design decision — see risks).
- Preview & Edit step before finalizing (Install / Edit / Regenerate / Back).
- Generate the agent artifact and place it (personal `~/.claude/agents/` vs shareable `plugins/`), reusing spike-f/D23 conventions.

### Out of Scope
- Multi-engine implementation (OpenCode/Gemini/Codex) — design for extensibility only.
- Runtime-extensible Phases taxonomy unless design confirms New Phase mode needs it in v1.
- Implementation (sdd-apply): PLANNING-ONLY this session (propose -> spec -> design -> tasks). Apply handed off to Codex separately; tasks must be self-contained for a memoryless executor.
- Closing spike-f's 2 manual verification gates (live `/agents` recognition, `/reload-plugins` hot-reload) — human gates, not code.

## Capabilities
### New
- `agent-builder-flow`: end-to-end create-your-own-agent flow (engine select, NL description, SDD integration mode, preview/edit, install).

### Modified
- `modelconfig` (CONDITIONAL): only if "New Phase" mode must extend the compile-time `Phases` constant slice at runtime. Flag for design — may be deferred, keeping v1 to Standalone + Phase Support.

## Approach
Follow the `configuremodels.go` + `modelselect.go` precedent (TTY-guarded hidden cobra command -> bubbletea collection -> validate -> persist). Central design fork: the flow needs a multi-turn conversational/LLM elicitation step that has NO precedent in menu.go's synchronous single-shot dispatch contract. Design must resolve how a bubbletea-dispatched command hands off to (and returns from) an LLM interview — likely Option 1 or hybrid Option 3 from exploration.

## Affected Areas
| Area | Impact | Description |
|------|--------|-------------|
| `internal/menu/menu.go` | Modified | Activate placeholder, add dispatch |
| `internal/cli/createagent.go` | New | TTY-guarded cobra command |
| `internal/ui/agentbuilder*.go` | New | bubbletea flow (or LLM handoff) |
| `plugins/click-sdd/skills/agent-builder/SKILL.md` | Reused/Modified | Elicitation + generation content |
| `modelconfig` | Conditional | Only if New Phase needs runtime Phases |

## Risks
| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Scope size — likely a multi-PR chain like last change | High | Slice into stacked PRs at tasks phase |
| New Phase mode requires runtime-extensible Phases (departs from compile-time constant just shipped) | Med | Design decision: defer New Phase to later iteration; v1 = Standalone + Phase Support |
| No dispatch precedent for multi-turn LLM flow from bubbletea menu | High | Design must define handoff mechanism explicitly |
| Artifact format: repo uses sub-agent .md; gentle-ai uses portable SKILL.md | Med | sdd-design reconciles; do not force in proposal |

## Rollback Plan
Revert to `Active:false` placeholder; delete new `createagent.go`/`agentbuilder*.go`; no data migration. `plugins/agent-builder/SKILL.md` untouched if only reused.

## Dependencies
- `feat/skill-content-taxonomy` branch tip (taxonomy/menu/skills chain, unmerged).
- spike-f-agent-builder.md conclusions (D23 placement decisions).

## Success Criteria
- [ ] Menu item launches a working create-your-own-agent flow.
- [ ] Engine select, NL description, SDD integration mode, Preview & Edit all present.
- [ ] Generated agent installed to correct personal/shareable location.
- [ ] tasks artifact is self-contained for a memoryless (Codex) executor.

---

## Note on final decisions (post-design)

The proposal above listed "New Phase" as an open scope question and the LLM-handoff mechanism as unresolved. Both were resolved during the design phase (see `design.md` in this same directory):

- **SDD integration mode**: v1 ships **Standalone + Phase Support only**. "New Phase" is explicitly excluded from v1 — it would require making the compile-time `modelconfig.Phases` taxonomy runtime-extensible, which is out of scope.
- **Multi-turn handoff**: resolved as **no live LLM conversation inside the Go binary at all**. The flow is a deterministic bubbletea wizard that collects the natural-language description and the 9 interview themes as sequential TUI form steps. The existing conversational `agent-builder` skill remains untouched as a separate, LLM-driven alternative path.
- **Artifact format**: v1 emits a **native Claude Code sub-agent `.md`** (YAML frontmatter matching the repo's existing 5 agents), not gentle-ai's portable SKILL.md format. This was explicitly confirmed with the user after flagging that it diverges from literal gentle-ai parity.

These are treated as final, confirmed decisions for the implementation — do not re-litigate them. See `documentacion/CODEX-HANDOFF.md` for the full confirmed-decisions list.
