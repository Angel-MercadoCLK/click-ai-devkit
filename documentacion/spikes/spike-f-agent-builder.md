# Spike F — Agent-builder Step 0: personal vs. shareable agent mechanisms

> Status: spike result. Grounded in `00-decisions-and-open-questions.md` (D10, D24) and
> `spikes/spike-c-plugin-registration.md`.
> Verified against the `claude` CLI (`claude` 2.1.203) on this machine, the real `~/.claude`
> registry (read-only), and a THROWAWAY `CLAUDE_CONFIG_DIR` under `%TEMP%` (never the real
> `~/.claude`).

## The problem this spike answers

The planned `agent-builder` feature needs to generate a new Claude Code sub-agent from an
interview, then hand the user a working `.md` file. There are two distinct output targets:

1. **Personal** — an agent only the current user needs, usable immediately in any project.
2. **Shareable** — an agent the team can install the same way they already install
   click-sdd/click-memory/click-review (per D24: native `claude plugin` CLI + this repo's
   marketplace.json).

This spike establishes, with commands actually run, how each target works, so agent-builder's
design doesn't guess.

## Investigation 1 — PERSONAL agent target

### Evidence: real `~/.claude/agents/` on this machine

`$USERPROFILE/.claude/agents/` (== `C:\Users\CLK090\.claude\agents\`) contains 23 `.md` files
(`ard.md`, `sdd-apply.md`, `review-risk.md`, `doc-arch.md`, etc.). Every name in that directory
is exactly the set of "Available agent types for the Agent tool" this session already sees. This
is direct, structural confirmation: **Claude Code loads user-level agents from
`$CLAUDE_CONFIG_DIR/agents/<name>.md`, defaulting to `~/.claude/agents/` when `CLAUDE_CONFIG_DIR`
is unset.** On this Windows machine that resolves to `C:\Users\CLK090\.claude\agents\`
(`%USERPROFILE%\.claude\agents`).

### Exact frontmatter schema (read from real files, not memory)

```yaml
---
name: sdd-apply                 # required, matches the filename convention
description: >                  # required; used for auto-delegation matching
  Implement code changes from task definitions. Use when tasks are ready...
model: sonnet                   # optional; seen values: sonnet, opus, haiku, inherit
tools: Read, Edit, Write, Glob, Grep, Bash, mcp__plugin_engram_engram__mem_search   # optional
user-invocable: true            # optional, seen on doc-arch.md only
---
```

Two materially different, both-valid syntaxes for `tools` were found in active files on this
machine:

- **Comma-separated string** (majority of files, e.g. `sdd-apply.md`, `review-risk.md`):
  `tools: Read, Edit, Write, Glob, Grep, Bash, mcp__plugin_engram_engram__mem_search, ...`
- **YAML block list** (e.g. `doc-arch.md`, `ard.md`):
  ```yaml
  tools:
    - Read
    - Write
    - Edit
  ```

Both forms are in live use by agents this session already lists as available, so both are
accepted — agent-builder should not assume only one form works. `model:` accepted values observed
across all 23 real files: `sonnet`, `opus`, `haiku`, `inherit`. No literal full model id or
`fable` was found in the wild, but `click-orchestrator.md` (this repo's own agent) documents
`fable` and full model ids as accepted too — consistent, not contradicted.

### `claude --help` / `claude agents --help` — an important correction to assumptions

**`claude agents` is NOT about the `.md` sub-agent files at all.** Its help text says
"Manage background agents" — it manages *dispatched background sessions* (`--bg`), not the
sub-agent definitions used by the `Agent`/`Task` tool. There is no `claude agents list` (or
equivalent) that enumerates standalone `agents/*.md` definitions. The only related surface is:

- `claude --agent <agent>` — "Agent for the current session. Overrides the 'agent' setting."
  (session-level default persona, not sub-agent listing)
- `claude plugin details <plugin>` — DOES enumerate agents/skills, but only for a **plugin**
  (see Investigation 2), not for loose files under `agents/`.

**Conclusion: there is no headless/CLI command to validate or list a standalone personal agent
file.** Its validity can only be confirmed by Claude Code actually loading it in a live session
(the `/agents` slash command, interactive-only) or indirectly via `claude plugin details` if the
same `agents/` folder is wrapped in a plugin manifest (see below).

### Attempted live-recognition experiment — INCOMPLETE (documented honestly)

Set up:
```
$env:CLAUDE_CONFIG_DIR = <temp>\spike-f-experiment\config
# wrote <temp>\spike-f-experiment\config\agents\spike-test-agent.md
# (well-formed: name, description, tools, model: haiku)
```
Attempted `claude -p "list your available subagent types" --model haiku` and `claude --debug
agents -p ...` under that `CLAUDE_CONFIG_DIR` to see if the model's system prompt would surface
`spike-test-agent`. **Both failed with `Not logged in · Please run /login`** — the Bash tool in
this environment has no authenticated `claude` CLI session (this is a different execution context
than the live Claude Code session that is running this spike). `claude doctor` also hung waiting
on an interactive trust-dialog prompt for the new/untrusted throwaway config dir (confirmed via
background task, then killed) — another interactive gate that can't be crossed headlessly.

**This is a genuine residual, not a paper-over:** the personal-agent-file mechanism is confirmed
structurally (file naming/location 1:1 match with what this live session already lists as
available agents) but NOT confirmed via a fresh throwaway-dir experiment in this spike, because
no authenticated headless `claude` session was available to run one. Manual check needed: a
human, in an authenticated terminal, sets `CLAUDE_CONFIG_DIR` to a throwaway dir with one agent
file and confirms `/agents` (interactive slash command) lists it.

## Investigation 2 — SHAREABLE agent mechanism

### `claude plugin init` — exact scaffold, re-derived by actually running it

```
claude plugin init --help
```
```
Usage: claude plugin init|new [options] <name>

Scaffold a new plugin at ~/.claude/skills/<name>/ (auto-loads next session as
<name>@skills-dir)

Options:
  --author <name>         Author name (default: git config user.name)
  --author-email <email>  Author email (default: git config user.email)
  --description <text>    Manifest description
  -f, --force             Overwrite an existing .claude-plugin/ at the target
  -h, --help              Display help for command
  --with <components...>  Also scaffold: skills, agents, hooks, mcp, lsp,
                          output-style, channel
```

Ran, against a throwaway `CLAUDE_CONFIG_DIR`:
```
claude plugin init test-builder-plugin --with agents --with skills --description "spike test plugin"
```
(`--with` must be repeated per component — `--with agents,skills` errors with `Unknown --with
component "agents,skills"`.)

Result — exact tree created under `$CLAUDE_CONFIG_DIR/skills/test-builder-plugin/`:
```
.claude-plugin/plugin.json
agents/example.md
SKILL.md
skills/example/SKILL.md
```

`plugin.json` produced:
```json
{
  "$schema": "https://anthropic.com/claude-code/plugin.schema.json",
  "name": "test-builder-plugin",
  "version": "0.1.0",
  "description": "spike test plugin",
  "author": { "name": "...", "email": "..." },
  "skills": ["./"]
}
```
Note: no `agents` key is listed in `plugin.json` — agents are discovered purely by the
`agents/*.md` directory convention, exactly like this repo's own `plugins/click-sdd/` plugins
(their `plugin.json` files also declare no `agents`/`skills` arrays; the directories alone are
enough).

The scaffolded `agents/example.md` uses the **YAML block-list** `tools:` form (not the
comma-string form seen in most real `~/.claude/agents/*.md` files) — consistent with "both forms
valid" from Investigation 1.

### Does it auto-load, and under what condition?

```
claude plugin validate <path-to-test-builder-plugin> --strict   → ✔ Validation passed
claude plugin list
```
```
Skills-directory plugins (.claude/skills/*):
  ❯ test-builder-plugin@skills-dir
    Version: 0.1.0
    Scope: user
    Path: ...\config\skills\test-builder-plugin
    Status: ✔ loaded
```
```
claude plugin details test-builder-plugin@skills-dir
```
```
Component inventory
  Skills (1)  example
  Agents (1)  example
  Hooks (0)
  MCP servers (0)
  LSP servers (0)
Projected token cost: ~71 tok always-on
```
Confirmed: any plugin-shaped directory (has `.claude-plugin/plugin.json`) placed under
`$CLAUDE_CONFIG_DIR/skills/<name>/` is auto-discovered and loaded as `<name>@skills-dir`, **scope:
user, no marketplace registration required.** `claude plugin details` and `claude plugin list`
both worked without authentication (pure local static analysis — no API call), which is why they
could be used here even though the live-recognition test in Investigation 1 could not.

The one caveat also surfaced unprompted: `claude plugin list` printed a warning that a
*project-scope* `./.claude/skills/` directory existed but "was not loaded because this workspace
was not trusted... run `/reload-plugins`" — i.e. hot-reload/first-load of a new plugin inside an
**already-running interactive session** needs either a trust-dialog acceptance or the
`/reload-plugins` slash command / a relaunch. This spike could not exercise that step (no live
interactive session available headlessly) — flagged as residual, same category as spike-c's own
"confirm at runtime" caveats.

### Is a scaffolded plugin git-publishable as-is?

Yes, structurally — it's an ordinary directory (`.claude-plugin/plugin.json` + `agents/` +
`skills/`), nothing ties its git-publishability to living under `~/.claude/skills/`. But note the
**auto-load path only watches the local config dir's `skills/` subfolder** — `claude plugin init`
does not register a git remote, and there is no mechanism that pulls a `skills-dir` plugin from a
URL automatically. To make a `claude plugin init`-scaffolded plugin genuinely team-shareable you
would have to: `git init` it yourself, host it, and have every teammate run a **second**
`claude plugin marketplace add <that-repo>` + `claude plugin install <name>@<that-marketplace>` —
a parallel distribution channel to the one this repo already has.

### Comparison: `claude plugin init` (new skills-dir plugin) vs. extending `plugins/click-sdd/`

| | `claude plugin init` (own plugin, own marketplace) | Add to existing `plugins/click-sdd/agents/` (or a new `plugins/click-<name>/` in this repo) |
|---|---|---|
| Distribution channel | New, separate — needs its own repo + marketplace + install command | Reuses the marketplace.json + `claude plugin marketplace add/install` flow already verified in spike-c and already run by every teammate |
| Team onboarding cost | Extra: a second `marketplace add`/`install` per agent-builder output | Zero: `claude plugin update` on the existing install picks it up |
| Versioning/history | New repo (or a folder the user has to remember to `git init`) | Already tracked by this repo's normal git history / PR review |
| Scaffolding speed | Fast: `--with agents --with skills` gives a valid template in one command | Manual: copy an existing agent `.md` as a starting shape |
| Fits D23 philosophy ("same repo, not separate") | No — recreates exactly the "extra repo" pattern D23 rejected for the scoop bucket | Yes — consistent with D23 |

**Recommendation:** agent-builder's "shareable" output path should write the generated agent
directly into this repo's plugin tree — either `plugins/click-sdd/agents/<name>.md` if it's a
phase-shaped agent, or a new `plugins/click-<name>/` plugin folder for a standalone one — NOT into
a separately `claude plugin init`-scaffolded, separately-hosted plugin. The one place
`claude plugin init --with agents --with skills` still earns its keep: as an **internal
scaffolding step** (run once against a scratch dir to get a correctly-shaped `plugin.json` +
`agents/example.md` + `SKILL.md` template), whose generated files agent-builder then copies/adapts
into the existing repo tree before commit — never as the actual runtime distribution path. This
keeps exactly one marketplace and one install command, per D24/D23.

## Directory paths (exact, this machine)

- Personal agent target: `$CLAUDE_CONFIG_DIR/agents/<name>.md`, defaulting to
  `%USERPROFILE%\.claude\agents\<name>.md` = `C:\Users\CLK090\.claude\agents\<name>.md` when
  `CLAUDE_CONFIG_DIR` is unset (confirmed: it was unset in this shell and the real dir there
  matches the session's available-agent list exactly).
- Shareable/skills-dir scaffold target: `$CLAUDE_CONFIG_DIR/skills/<name>/` (personal-machine
  auto-load path) — NOT the recommended final home per above; the recommended final home is this
  repo's `plugins/click-sdd/agents/` or `plugins/click-<name>/`, installed via the marketplace.json
  flow already verified in spike-c.

## What could NOT be verified — residual manual-check items

1. **Live personal-agent recognition inside an authenticated interactive session.** The Bash tool
   in this environment has no logged-in `claude` CLI session (`Not logged in · Please run
   /login`), so the throwaway-dir + `spike-test-agent.md` + "does the model see it" experiment
   could not run to completion. Structural evidence (real `~/.claude/agents/` 1:1 matching this
   session's available agents) is strong but indirect. **Manual check:** in an authenticated
   terminal, set `CLAUDE_CONFIG_DIR` to a throwaway dir with one agent `.md`, start `claude`
   interactively, run `/agents`, confirm it's listed.
2. **Hot-reload of a newly-scaffolded `skills-dir` plugin inside an already-running session.**
   `claude plugin list` explicitly said a freshly-added project-scope plugin needed
   `/reload-plugins` or a relaunch because the workspace hadn't been trust-confirmed yet; this
   spike could not exercise that trust dialog or `/reload-plugins` headlessly (`claude doctor` hung
   on the same trust gate and was killed as a background task). **Manual check:** confirm
   `/reload-plugins` (or a relaunch) is sufficient and no other step is needed.
3. Whether `tools:` comma-string vs. YAML-block-list has any behavioral difference (e.g., silent
   truncation, ordering) beyond "both parse" — not exercised beyond confirming both forms are
   present in files Claude Code currently treats as valid.

No discrepancies with spike-c's prior claims about `claude plugin init` were found — its one-line
description (`scaffold a new plugin at ~/.claude/skills/<name>/`) matches exactly what was
re-derived here from `--help` and from actually running the command.
