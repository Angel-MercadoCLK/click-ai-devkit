---
name: agent-builder
description: Interview the developer in Spanish to design a new Claude Code sub-agent, confirm the spec, then generate and place a well-formed agent .md file — personal or shareable.
---

## Workflow

### 1. Interview (Spanish, plain, professional — D10 persona)

Ask focused questions **one theme at a time**, not a giant wall of questions. Cover, in
this order:

1. Purpose/goal — what problem does this agent solve, in one or two sentences.
2. Exact tasks it handles — concrete actions, not vague verbs.
3. Trigger situations/phrases — when should it fire, what would the user say.
4. Hard rules and constraints — things it must never do, boundaries.
5. Tools it needs — Read/Write/Edit/Bash/Grep/Glob/Agent/MCP tools, etc.
6. Model — opus, sonnet, haiku, or inherit; ask which trade-off (quality vs. cost/speed)
   fits.
7. Tone/persona it should use when talking to the developer.
8. Domain knowledge it needs (codebase areas, conventions, external docs).
9. What "good output" looks like — a concrete example if possible.

Push back and ask a follow-up whenever an answer is vague, generic, or could describe
many different agents. The goal is a tightly-scoped, unambiguous agent — never settle
for a generic one. Do not move to step 2 until every theme above has a concrete answer.

### 2. Summarize and confirm (Spanish)

Play back the full gathered spec in Spanish as a compact summary (purpose, tasks,
triggers, rules, tools, model, tone, domain knowledge, output shape). Ask for explicit
confirmation before generating anything. If the user corrects something, update the
summary and re-confirm.

### 3. Generate the agent file (English content — D10: artifacts are always English)

Produce:

- YAML frontmatter: `name` (kebab-case, matches the filename), `description` (used for
  auto-delegation matching — state what it does and when to use it), `model`
  (sonnet/opus/haiku/inherit, from step 1.6), `tools` as a **comma-separated string**
  (both comma-string and YAML block-list forms are valid in this ecosystem, but default
  to the comma-string form — it is what most real agent files use).
- A structured system prompt built from the interview, with these sections: Role, When
  to use / trigger conditions, Workflow or steps, Hard rules, Output format. Keep it as
  precise and unambiguous as the interview answers allow — no filler.

### 4. Ask placement — personal or shareable (Spanish, LAST step)

Ask the developer whether the agent is personal (only for them) or shareable (for the
team), then place the file accordingly.

**Personal:**
- Write to `$CLAUDE_CONFIG_DIR/agents/<name>.md`, defaulting to
  `~/.claude/agents/<name>.md` (`%USERPROFILE%\.claude\agents\<name>.md` on Windows)
  when `CLAUDE_CONFIG_DIR` is unset.
- Tell the developer (Spanish) it will be available starting their next Claude Code
  session.

**Shareable:**
- Check whether the current working repo has `.claude-plugin/marketplace.json` at its
  root.
- **If yes:** place the file at `plugins/<plugin-name>/agents/<name>.md`. Reuse
  `plugins/click-sdd/` when the new agent is clearly SDD-phase-shaped; otherwise
  scaffold a new `plugins/click-<name>/` folder with its own minimal
  `.claude-plugin/plugin.json` (mirror the exact shape of this repo's existing
  `plugin.json` files — `name`, `version`, `description`, `author`; no `agents`/`skills`
  array needed, directory presence is enough). If a new plugin folder was created, add
  its entry to the root `marketplace.json` `plugins` array (`name`, `description`,
  `version`, `author`, `source`, `category`, `homepage`), following the existing
  entries' shape exactly. Then tell the developer (Spanish) the exact next steps: review
  the file, run `claude plugin validate .`, commit and push, and how teammates pick it
  up (`claude plugin marketplace add <repo-url>` if not yet added, or
  `claude plugin update` if already installed, then
  `claude plugin install <plugin>@click-ai-devkit`).
- **If no** (this skill is running inside some other project that only has click-sdd
  installed as a plugin, not this devkit repo): fall back to
  `claude plugin init <name> --with agents --description "<short description>"`,
  scaffolding at the user's personal skills-dir. Explain (Spanish) this is a
  personal-machine auto-load path — to truly share with a team they would need to
  `git init` that scaffolded folder, host it, and have teammates run
  `claude plugin marketplace add <url>` plus
  `claude plugin install <name>@<url-based-marketplace-name>`.

## Rules

- Conduct the whole interview and every status update in Spanish, plain and
  professional — no jargon dumps, no slang (D10).
- Never generate the agent file before the developer explicitly confirms the summary
  in step 2.
- Every generated file's content (frontmatter values, system prompt, comments) is
  always in English, regardless of the interview language.
- Ask at most one theme/question group at a time; wait for the answer before moving on.
- Do not default to a generic agent shape — if any interview answer is ambiguous, ask
  a follow-up instead of guessing.
- Never write directly into a separately-hosted `claude plugin init` plugin as the
  final shareable home when this repo's own `.claude-plugin/marketplace.json` exists —
  that recreates a second distribution channel this project deliberately avoided.
