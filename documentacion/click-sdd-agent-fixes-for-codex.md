# click-sdd agent-definition fixes — verified findings for implementation

**Audience**: an implementing agent (Codex) with repo write access. Every finding below was
independently re-verified against the actual files in this repository — not taken on the word of
the session report that originally surfaced them. Where that report's diagnosis was wrong, this
document says so explicitly and gives the corrected mechanism, so the fix targets the real defect.

**Scope**: `plugins/click-sdd/` only (the Click Seguros SDD orchestration plugin). This is unrelated
to the `engram-mcp-resolution` change (installer/doctor PATH-persistence work, already shipped in
`v0.4.2`) — do not conflate the two. `click doctor` does not and cannot catch anything in this
document: its checks validate installer/registration state, never the *content* of an agent's
`tools:` frontmatter or a skill file's instructions.

---

## Summary table

| ID | Verdict | Severity | File(s) | One-line fix |
|----|---------|----------|---------|---------------|
| CONFIRMED-1 | Real bug | High | `plugins/click-sdd/agents/click-orchestrator.md` | Add Engram MCP tools so the documented memory-handoff flow is actually executable. |
| CONFIRMED-2 | Real gap (design, not code defect) | Medium | `plugins/click-sdd/agents/click-orchestrator.md` | Add an explicit instruction to pass resolved `SKILL.md` paths into every specialist delegation. |
| OBSERVED-3 | Likely real, but is an LLM-compliance gap, not a static defect | Medium | `plugins/click-sdd/agents/click-orchestrator.md` | Harden the already-correct Model Routing section; do not rewrite its content. |
| CORRECTED-4 | **Misdiagnosis in the original report — do not implement as originally framed** | — | `plugins/click-sdd/agents/*.md`, `plugins/click-sdd/skills/*/SKILL.md` | No `Skill` tool grant is needed or used anywhere in this system. Do not add one. |
| UNVERIFIED-5 | Cannot confirm or refute from static files | — | `plugins/click-sdd/agents/click-architect.md` | Needs a live repro, not a file patch. See notes. |

---

## CONFIRMED-1 — Every click-sdd agent lacks Engram MCP tools; the documented memory flow is structurally impossible

**Severity: High.** This is the one finding from the original report that is not only real, but
*worse* than reported.

### Evidence (verified now, file read directly)

`plugins/click-sdd/agents/click-orchestrator.md`, frontmatter:
```yaml
tools: Read, Write, Edit, Glob, Grep, Bash, Agent
```

`plugins/click-sdd/agents/click-memory-curator.md`, frontmatter:
```yaml
tools: Read, Write, Edit, Glob, Grep
```

Neither lists any `mcp__plugin_engram_engram__*` tool. Grepping every agent under
`plugins/click-sdd/agents/` confirms **none** of the five agents in this plugin
(`click-orchestrator`, `click-prd-writer`, `click-architect`, `click-reviewer`,
`click-memory-curator`) have any Engram tool grant.

Yet `click-orchestrator.md` itself states, in its own "Delegation contract" section (lines 57–60):

> "Engram is always part of the working model. Durable technical knowledge, progress artifacts,
> decisions, and important discoveries must be handed to `click-memory-curator` or persisted
> through the established memory flow; the memory-guard remains the safety boundary. **You do not
> persist memory directly unless the curator confirms it is durable technical knowledge.**"

That last sentence is the key textual detail the original report missed: it implies the
**orchestrator** is the one expected to call `mem_save`, *gated on* the curator's advisory
confirmation — not that the curator itself calls `mem_save`. Read literally, the curator's job is
advisory (produce a recommendation), and the orchestrator is the one who should actually persist it.

Compare against the working pattern already used elsewhere in this repo: the generic `sdd-apply`
and `sdd-verify` agent definitions (a *different*, already-functioning agent family — see
`~/.claude/agents/sdd-apply.md` equivalent, or the agent roster surfaced in-session) explicitly list
`mcp__plugin_engram_engram__mem_search`, `mcp__plugin_engram_engram__mem_get_observation`,
`mcp__plugin_engram_engram__mem_save`, `mcp__plugin_engram_engram__mem_update` in their `tools:`
frontmatter. That is the proven, working convention for granting a Claude Code subagent Engram
access: **the exact MCP tool names must appear in the agent's own `tools:` list.** There is no
other mechanism (a plugin being "enabled" in `settings.json` does not implicitly grant its MCP
tools to every subagent — each subagent's frontmatter is its own allowlist).

### Root cause

`click-memory-curator.md` was authored to describe *what* durable knowledge should look like, but
neither it nor `click-orchestrator.md` (whichever of the two is meant to actually call `mem_save`)
was ever given the tool grant to do so. The documented flow describes a capability that does not
exist in either agent's actual tool list.

### Fix

Add Engram MCP tools to **both** agents, split by responsibility to match the text's own intent:

**`click-orchestrator.md`** (the agent that gates and performs the actual persistence per its own
documented text) — add to `tools:`:
```
mcp__plugin_engram_engram__mem_search, mcp__plugin_engram_engram__mem_get_observation, mcp__plugin_engram_engram__mem_save, mcp__plugin_engram_engram__mem_update
```

**`click-memory-curator.md`** (advisory role — needs to check for existing/duplicate entries before
recommending a new one, but per the text does not itself persist) — add to `tools:`:
```
mcp__plugin_engram_engram__mem_search, mcp__plugin_engram_engram__mem_get_observation
```

If, after reading this, the maintainer's actual intent was the reverse (curator persists directly,
orchestrator just delegates) — that is a legitimate alternative reading — then swap which agent
gets the write tools (`mem_save`/`mem_update`) accordingly. **Pick one owner and grant it the write
tools; do not grant write tools to both**, to avoid two independent code paths that can both decide
to persist the same discovery.

### Verification

1. `plugins/click-sdd/agents/click-orchestrator.md` and `click-memory-curator.md` frontmatter
   `tools:` lines contain the tools listed above.
2. Run (or re-run) a full click-sdd cycle end to end and confirm at least one `mem_save` call
   actually reaches the `PreToolUse` `memory-guard` hook (check the hook's local audit log, or
   confirm a new observation exists in Engram for that project afterward).
3. Confirm the `matcher` in `~/.claude/settings.json`'s `PreToolUse` hook
   (`mcp__plugin_engram_engram__mem_save`) still fires for calls made from within a
   `click-orchestrator`/`click-memory-curator` subagent context (it is a global hook, not
   agent-scoped, so it should fire regardless — but confirm empirically, don't assume).

---

## CONFIRMED-2 — The orchestrator never explicitly instructs itself to pass resolved skill paths into delegate prompts

**Severity: Medium.** This is the *real* mechanism behind what the original report observed (spec/
apply/archive allegedly done "inline" rather than via a loaded skill) — but the report's stated
*cause* ("no tiene el tool Skill") is wrong. See CORRECTED-4 below for the full correction; this
entry is the accurate replacement finding.

### Evidence

`plugins/click-sdd/agents/click-orchestrator.md`, "Flow" section, states:

> "Each phase name below is the exact skill under `plugins/click-sdd/skills/`."

and later, per-agent "Phase mapping" sections (e.g. in `click-architect.md`):

> "This agent owns two phases: `design` (`plugins/click-sdd/skills/design/SKILL.md`, model-routed
> via `design_model`) and `tasks` (...)."

This correctly *names* which skill file backs which phase. But nowhere in `click-orchestrator.md`
is there an explicit instruction telling the orchestrator to **include the resolved `SKILL.md` file
path as text inside the `Agent()` delegation prompt**, so the delegated specialist agent actually
`Read()`s it before doing the phase's work. The file states *where* the skill lives and *who owns*
it, but not *how the orchestrator hands it off*.

Contrast this with the separate, already-functioning `sdd-orchestrator-workflow.md` convention used
by this same repo's generic SDD agent family, which is explicit about exactly this step
("Sub-Agent Launch Protocol: ALL sub-agent launch prompts... MUST include pre-resolved skill paths
from the skill registry. ... pass exact `SKILL.md` paths"). `click-orchestrator.md` has no
equivalent instruction.

### Fix

Add a new subsection to `click-orchestrator.md` (near "Model routing", since it is the same kind of
per-delegation mechanical requirement), e.g.:

```markdown
## Skill hand-off

- Every specialist delegation must include the resolved `plugins/click-sdd/skills/<phase>/SKILL.md`
  path as literal text in the `Agent` prompt, and instruct the delegate to `Read` that file first,
  before doing any phase work.
- Do not summarize or paraphrase the skill's content into the delegation prompt from memory —
  pass the path and let the specialist agent load it directly, so the skill file remains the single
  source of truth for that phase's procedure.
```

### Verification

Re-run a click-sdd cycle and inspect the actual `Agent()` prompts issued for `propose`/`spec`/
`design`/`apply`/`archive` (or their equivalent specialist delegations) — confirm each one contains
a literal `plugins/click-sdd/skills/.../SKILL.md` path, and that the specialist's own tool-use trace
shows a `Read` call against that exact path before producing the phase's artifact.

---

## OBSERVED-3 — Model routing per phase: the instruction is already correct; the reported failure is an execution-compliance gap

**Severity: Medium — recommend hardening, not rewriting.**

### Evidence

`plugins/click-sdd/agents/click-orchestrator.md`'s "Model routing" section (lines 68–111) is
already extremely explicit:

> "Once per session, before your first `Agent` delegation, read the resolved choice from
> `pluginConfigs["click-sdd@click-ai-devkit"].options` in Claude Code's `settings.json` and cache
> the phase→model map for the rest of the session."
>
> "Pass the resolved alias as the `model` param on every `Agent` tool delegation you make..."

This is not vague or missing — it is a clear, direct, mandatory-sounding instruction. Cross-checked
against `plugins/click-sdd/.claude-plugin/plugin.json`'s `userConfig` block: all 18 phase-model keys
plus `orchestration_profile` and `default_model` are declared there and match what the instruction
text references (`propose_model` defaults `opus`, `design_model` defaults `opus`, `verify_model`
defaults `opus`, `archive_model`/`onboard_model` default `haiku`, everything else `sonnet`) — the
config schema and the instruction text are internally consistent with each other.

The original report's own session observed the orchestrator delegating `propose`/`design`/`verify`
in `sonnet` instead of the documented `opus`, and `archive` inline in `sonnet` instead of `haiku`.
That is a real, reproducible-sounding discrepancy between documented behavior and observed
behavior — but because the instruction text is already about as explicit as prose gets, **this is
most likely the orchestrating LLM failing to reliably follow its own already-correct instructions**,
not a gap in what the file says. Treat it as a prompt-robustness problem, not a missing-instruction
problem.

### Fix (hardening, not a rewrite)

Do not rewrite the Model Routing section's content — it is accurate. Instead:

1. Add a one-line, impossible-to-miss checklist gate immediately before the "Flow" numbered list,
   e.g.: `**Before any Agent call in this session: have you resolved and cached the phase→model
   map? If not, stop and do that first.**`
2. Consider whether the harness can enforce this outside of agent discretion — e.g., a thin wrapper
   or pre-flight check in `internal/cli` that resolves the phase model and could, in principle, be
   surfaced to the orchestrator as a ready-made lookup rather than relying on the orchestrator to
   read and parse `settings.json` itself from scratch each session. That is a larger, separate
   design question — flag it to the maintainer rather than silently deciding it here.
3. Before concluding this is a systemic, repeatable failure worth deeper engineering investment,
   re-run the same scenario at least once more. A single anecdote from one session is not yet proof
   of a reliably-reproducible defect.

### Verification

Re-run a full click-sdd cycle (ideally 2–3 times) and confirm, for each `Agent` delegation, that the
`model` parameter passed matches the resolved alias for that phase per `settings.json`'s
`pluginConfigs["click-sdd@click-ai-devkit"].options`.

---

## CORRECTED-4 — "Missing Skill tool" is not a bug; no agent in this system is designed to use a `Skill` tool grant

**This is a correction to the original report, not a new finding to implement as originally framed.
Do not add a `Skill` tool to any agent's frontmatter — it would not fix anything and may not even be
a valid capability to grant a delegated subagent.**

### Evidence

Read `plugins/click-sdd/skills/archive/SKILL.md` directly: it has a frontmatter with only `name:`
and `description:` — **no `tools:` field at all**. It is plain-text workflow instructions (a
numbered procedure), not an invokable capability. The same structure holds for every file under
`plugins/click-sdd/skills/`.

None of the five click-sdd agents (`click-orchestrator`, `click-prd-writer`, `click-architect`,
`click-reviewer`, `click-memory-curator`) list `Skill` in their `tools:` frontmatter — and neither
does any agent in the separate, already-working generic `sdd-*` family used elsewhere in this
session (`sdd-apply`, `sdd-verify`, etc. — none of them have `Skill` either). This is not an
oversight repeated five times; it is the actual, consistent, working design: **a "skill" in this
system is a markdown file of instructions meant to be opened with the ordinary `Read` tool (which
every click-sdd agent already has), not a named capability invoked through Claude Code's own
`Skill` tool** (that tool is tied to the main session's `<available_skills>` catalog injection
mechanism and is not something a delegated subagent is designed to receive or use here at all).

The original report's specific claim — "el orquestador no puede invocar esos SKILL.md directamente...
solo puede... hacerlo inline sin cargar el archivo de skill" — draws the right *observation*
(possibly true: the specialist agents may not have actually read the skill files) from the *wrong
mechanism* (a missing `Skill` tool grant, which was never how this works). The correct underlying
gap, if the observation holds, is CONFIRMED-2 above: nothing tells the orchestrator to pass the
skill file's path into the delegation prompt in the first place.

### Action for Codex

Do not implement anything from the original report's "el orquestador no tiene el tool Skill" framing.
If that exact wording appears elsewhere (issue trackers, other notes), correct it to point at
CONFIRMED-2 instead.

---

## UNVERIFIED-5 — Bash tool availability inside a nested Agent call

**Cannot be confirmed or refuted by reading files. Flagging honestly rather than guessing.**

### Evidence

`plugins/click-sdd/agents/click-architect.md` frontmatter, line 4:
```yaml
tools: Read, Write, Edit, Glob, Grep, Bash
```
`Bash` **is** declared. The original report's session claimed a nested `Agent`-calling-`Agent`
scenario (orchestrator delegates to `click-architect` for `verify`, which itself may have been
running inside another agent's context) reported "no Bash tool available in that runtime" and fell
back to a manual trace instead of actually executing tests.

Static analysis of the agent definition file cannot confirm or deny a runtime tool-grant
materialization behavior for nested delegation — that is a harness/infrastructure question, not
something visible in a markdown frontmatter. It is also worth noting `verify` is not
`click-architect`'s owned phase per its own "Phase mapping" section (`click-architect` owns
`design` and `tasks`; `click-reviewer` owns `verify`) — so the scenario described (using
`click-architect` for `verify`) may itself have been an orchestrator routing mistake unrelated to
the Bash-availability question, which muddies what that session's report was actually observing.

### Recommendation

Do not attempt a speculative file-level "fix" for this. If it recurs, reproduce it minimally and
deliberately: delegate `click-architect` directly (not nested inside another delegation) and confirm
`Bash` works standalone; then delegate it nested inside exactly one other `Agent` call and compare.
Only file/patch a fix once the actual failure boundary (standalone vs. nested; `click-architect`
specifically vs. any agent) is empirically isolated.

---

## What NOT to touch

- `internal/installer/`, `internal/doctor/`, `internal/ui/renderer.go`, `internal/cli/install.go`/
  `update.go` — all part of the unrelated, already-shipped (`v0.4.2`) `engram-mcp-resolution` change.
  Nothing in this document implicates that code.
- `plugins/click-memory/`, `plugins/click-review/` — not implicated by anything verified here.
- Do not add a `Skill` tool anywhere (see CORRECTED-4).
