# Design: Agent Builder Flow (gentle-ai parity)

## Technical Approach
Follow the `configuremodels.go` + `modelselect.go` precedent EXACTLY: menu flips a placeholder to a real dispatch → hidden TTY-guarded cobra command `click create-agent` → runs ITS OWN bubbletea program (a multi-step wizard) → deterministic Go template writer emits a Claude Code sub-agent `.md` → installs to personal/shareable location. NO LLM is invoked by the Go binary at any point. New domain package `internal/agentbuilder/` holds engine/spec/writer (pure, headless-testable); `internal/ui/agentbuilder.go` holds the wizard `tea.Model`; `internal/cli/createagent.go` is the command shell.

## Architecture Decisions

### Q1 — Engine selection
**Choice**: Real `[]Engine` data structure with one entry (`ClaudeCode`); wizard AUTO-SELECTS when `len(Engines())==1` (skips the interactive engine step entirely — no pointless single-option screen). v2 adds engines by appending to the slice; the step becomes interactive automatically.
**Rejected**: (a) hardcode Claude Code with no type → v2 is a rewrite; (b) force a one-option selector screen now → user-hostile.
**Rationale**: The data shape is the cheap, future-proof part; the UI cost of a forced single pick is pure friction. Additive, not a rewrite.

### Q2 — Artifact format (sub-agent .md vs portable SKILL.md)
**Choice**: v1 emits a Claude Code **sub-agent `.md`** (YAML frontmatter `name`/`description`/`tools` comma-string/`model` + `# Role` body), matching this repo's 5 existing agents (100% of them use this exact shape). Portable SKILL.md deferred until a 2nd engine exists.
**SCOPE FLAG (visible to user/reviewer)**: The user chose "full gentle-ai parity" over the cheaper option. This design honors parity of the FLOW (engine select → NL description → SDD mode → preview/edit → install) but NOT gentle-ai's SKILL.md wire-format. Reconciliation: gentle-ai emits SKILL.md ONLY because it installs the same file to many engines; with a single Claude Code target, the native sub-agent `.md` is the correct artifact — emitting a portable SKILL.md to a lone Claude Code target would be strictly worse (Claude Code sub-agents ARE `.md` frontmatter files). This is parity-of-flow with native-format output, NOT a silent downgrade. If the user wants the literal SKILL.md format regardless, that is a v1 change requiring explicit confirmation.
**Rejected**: emit SKILL.md now → no consumer benefits from portability; contradicts the repo's own convention.

**CONFIRMED BY USER**: this trade-off was explicitly confirmed by the user during the design session, after being told it diverges from literal gentle-ai parity. Treat this as final — do not re-open in implementation.

### Q3 — New Phase / runtime Phases
**Choice**: v1 = **Standalone + Phase Support only**. Phase Support reads the existing compile-time `modelconfig.Phases` slice as a READ-ONLY picklist ("which existing SDD phase does this agent support"); it never mutates it. **New Phase is excluded** — it is the ONLY mode that would need runtime-extensible Phases. `modelconfig` is UNCHANGED in v1.
**SCOPE FLAG (visible)**: Excluding New Phase is a real reduction from "full parity". Surfaced here, not buried. Adding it later means making `Phases` runtime-extensible (departs from the just-shipped compile-time constant) — a deliberate future decision.

### Q4 — Multi-turn handoff / LLM boundary
**Choice**: A **pure deterministic bubbletea wizard**, NOT a live LLM conversation. The Go binary CANNOT host an LLM and does NOT shell out to `claude -p` (explicitly OUT of scope — not in the proposal; default NO). The "natural-language description" is a free-text multi-line field the human types. The "9-theme interview" becomes sequential form steps (one text prompt per theme); the human answers by typing into TUI prompts. Generation is a deterministic Go template assembling the `.md` from collected fields. Menu dispatches to hidden `click create-agent` (exactly like `configure-models`); that command runs its own bubbletea program managing multi-step state internally — fully compatible with menu.go's synchronous single-shot dispatch contract (one cobra subcommand invocation, no LLM turn).
**SCOPE FLAG (visible)**: This loses the LLM interview's implicit "push back when the answer is vague" quality. Acceptable v1 tradeoff. The existing conversational `plugins/click-sdd/skills/agent-builder/SKILL.md` stays UNTOUCHED as the LLM-driven alternative path (reachable inside a real Claude session); the menu path is the deterministic wizard mirroring the same 9 themes as form fields.
**Rejected**: (a) invoke Agent/Task tool mid-flow → breaks dispatch contract, Go can't drive it; (b) shell to `claude -p` → un-specified, adds a hard dependency on a logged-in CLI.

## Data Flow
```
menu (ActionCreateAgent) ──ActionArgs──> fresh cobra: `create-agent`
   │                                          │ TTY guard (isTerminalWriter)
   │                                          ▼
   │                         ui.AgentBuilderModel (multi-step Update/View)
   │                         engine→desc→sddmode→[phase]→themes→preview→placement
   │                                          ▼ (Confirmed) AgentSpec
   └──────── back to menu ◀── agentbuilder.Install(spec, home, repo, FileWriter)
```

## Interfaces / Contracts (Go)
```go
// internal/agentbuilder/engine.go
type Engine struct {
    ID        string // "claude-code"
    Label     string // "Claude Code"
    AgentsDir func(claudeHome string) string // personal dir: filepath.Join(claudeHome,"agents")
}
func Engines() []Engine        // []Engine{ClaudeCode}
func DefaultEngine() (Engine, bool) // (Engines()[0], true) when len==1 → auto-select

// internal/agentbuilder/spec.go
type SDDMode string
const ( SDDStandalone SDDMode="standalone"; SDDPhaseSupport SDDMode="phase-support" ) // NewPhase deferred
func SDDModes() []SDDMode
type Placement string
const ( PlacementPersonal Placement="personal"; PlacementShareable Placement="shareable" )
type AgentSpec struct {
    Engine      Engine
    Name        string            // kebab-case (== filename)
    Description  string            // frontmatter description / NL summary
    SDDMode     SDDMode
    Phase       modelconfig.Phase // set only when SDDMode==SDDPhaseSupport
    Tools       string            // comma-string, e.g. "Read, Edit, Bash"
    Model       string            // sonnet|opus|haiku|inherit
    Purpose, Tasks, Triggers, Rules, Tone, Domain, GoodOutput string // 9 themes
    Placement   Placement
}

// internal/agentbuilder/writer.go
type FileWriter interface {          // injected for headless tests
    MkdirAll(path string, perm os.FileMode) error
    WriteFile(path string, data []byte, perm os.FileMode) error
    Stat(path string) (os.FileInfo, error)
}
func RenderAgentMarkdown(spec AgentSpec) (string, error)                       // pure, deterministic
func TargetPath(spec AgentSpec, claudeHome, repoRoot string) (string, error)   // personal vs shareable
func Install(spec AgentSpec, claudeHome, repoRoot string, w FileWriter) (string, error)

// internal/ui/agentbuilder.go
type Step int
const ( StepEngine Step=iota; StepDescription; StepSDDMode; StepPhase; StepThemes; StepPreview; StepPlacement; StepDone )
type AgentBuilderModel struct {
    Step Step; Spec agentbuilder.AgentSpec; engines []agentbuilder.Engine
    input textinput.Model; themeIdx int; Confirmed, Cancelled bool
}
func NewAgentBuilderModel(engines []agentbuilder.Engine) AgentBuilderModel // starts past StepEngine when len==1
func (m AgentBuilderModel) Update(msg tea.Msg) (tea.Model, tea.Cmd)         // pure, no I/O
func (m AgentBuilderModel) View() string
// Auto-skip StepPhase when Spec.SDDMode != SDDPhaseSupport.

// internal/cli/createagent.go
func newCreateAgentCommand() *cobra.Command  // Use:"create-agent", Hidden:true
func runCreateAgent(cmd *cobra.Command) error // TTY guard → runAgentBuilderTUI → agentbuilder.Install
```

## File Changes
| File | Action | Description |
|------|--------|-------------|
| `internal/agentbuilder/engine.go` | Create | Engine type, Engines(), DefaultEngine() auto-select |
| `internal/agentbuilder/spec.go` | Create | AgentSpec, SDDMode (2 values), Placement |
| `internal/agentbuilder/writer.go` | Create | RenderAgentMarkdown, TargetPath, Install, FileWriter iface |
| `internal/agentbuilder/*_test.go` | Create | golden render + fake-FileWriter install + auto-select tests |
| `internal/ui/agentbuilder.go` | Create | AgentBuilderModel wizard (pure Update/View) |
| `internal/ui/agentbuilder_test.go` | Create | drive Update with tea.KeyMsg, assert Step/Spec |
| `internal/cli/createagent.go` | Create | hidden cobra cmd, TTY guard, runs TUI + Install |
| `internal/cli/createagent_test.go` | Create | non-TTY guard returns info msg (no hang) |
| `internal/cli/root.go` | Modify | add `newCreateAgentCommand()` to root.AddCommand (line ~56-63) |
| `internal/menu/menu.go` | Modify | add `ActionCreateAgent="create-agent"`; flip "Crear tu propio agente" item to `{Action:ActionCreateAgent, Active:true}`; add ActionArgs case → `[]string{"create-agent"}` |
| `internal/menu/menu_test.go` | Modify | assert item active + ActionArgs mapping |
| `modelconfig` | UNCHANGED | v1 needs no runtime Phases |
| `plugins/click-sdd/skills/agent-builder/SKILL.md` | UNCHANGED | stays as conversational alt path |

## Testing Strategy (STRICT TDD)
| Layer | What | How |
|-------|------|-----|
| Unit (wizard) | Step transitions, auto-skip StepEngine (1 engine) + StepPhase (non-PhaseSupport), Confirmed/Cancelled | Call `m.Update(tea.KeyMsg{Type:tea.KeyEnter})` etc. directly; assert `m.Step`/`m.Spec` — same headless pattern as `modelselect_test`/`menu_test`, no real bubbletea program |
| Unit (writer) | Deterministic `.md` output | `RenderAgentMarkdown(spec)` golden-string assertion (pure fn) |
| Unit (install) | Correct target path + bytes, no real disk | Inject fake `FileWriter` capturing (path,data); `TargetPath` driven by `CLICK_CLAUDE_HOME` override + `t.TempDir()` repoRoot |
| Unit (engine) | Auto-select | `DefaultEngine()` returns (ClaudeCode,true); NewAgentBuilderModel skips StepEngine |
| Unit (cli) | Non-TTY safety | non-`*os.File` writer → prints info, no bubbletea (mirrors configuremodels) |
| Unit (menu) | Wiring | item Active + ActionArgs("create-agent") |

## Migration / Rollout
No data migration. Rollback: revert menu item to `Active:false`, delete new files, `modelconfig`/SKILL.md untouched.

## Open Questions
- [ ] User confirmation that "parity of flow, native sub-agent .md output" (Q2) satisfies the "full gentle-ai parity" mandate — flagged, not assumed. If literal SKILL.md format is required in v1, expand writer scope.
  - **STATUS: RESOLVED.** The user explicitly confirmed the Q2 trade-off (native sub-agent `.md`, not portable SKILL.md) during this planning session. Codex does not need to re-ask this.
- [ ] Shareable placement depends on spike-f's 2 unresolved MANUAL gates (live `/agents` recognition, `/reload-plugins` hot-reload) — human verification before shipping the shareable path.
  - **STATUS: STILL OPEN — MANUAL, NOT CODE.** These remain human-in-the-loop gates (see Phase 5 of `tasks.md`). They cannot be closed by Codex; document them and move on.
