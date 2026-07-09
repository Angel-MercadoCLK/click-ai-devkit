# Spike C — How Claude Code actually installs/loads plugins

> Status: spike result. Grounded in 00-decisions-and-open-questions.md.
> Verified against the `claude` CLI (`/c/Users/CLK090/.local/bin/claude`) and the real
> `~/.claude` plugin registry on the user's machine.

## The problem this spike answers

v0.1's `click install` COPIES plugin folders into `~/.claude/plugins/click-sdd/` etc. But Claude
Code does **not** load loose folders. Real plugins (engram, ui-ux-pro-max, vercel) are registered:
- `~/.claude/plugins/known_marketplaces.json` → the marketplace source (a github repo), cloned to
  `~/.claude/plugins/marketplaces/<name>/` (a full git checkout containing `.claude-plugin/marketplace.json`)
- `~/.claude/plugins/installed_plugins.json` → the installed plugin, cached at
  `~/.claude/plugins/cache/<marketplace>/<plugin>/<version>/`, with version + gitCommitSha
- `~/.claude/settings.json` → `enabledPlugins: { "engram@engram": true }`

So our click-* plugins were never registered anywhere → **inert files, never loaded.** This is the
root cause of "the skills don't actually install."

## KEY FINDING: the `claude` CLI installs plugins non-interactively

We do NOT need to hand-write the registry files (fragile, format could change). The official
`claude` CLI exposes the whole plugin lifecycle:

```
claude plugin marketplace add <source>   # source = URL | path | GitHub repo
      --scope user|project|local         # default user
      --sparse <paths...>                # monorepo: limit checkout, e.g. --sparse .claude-plugin plugins
claude plugin marketplace list | remove | update
claude plugin install <plugin>           # or <plugin>@<marketplace>
      --scope user|project|local         # default user
      --config <key=value>               # set a userConfig option declared in the plugin manifest (repeatable)
claude plugin enable | disable | list | uninstall | update | validate <path>
claude plugin init|new <name>            # scaffold a new plugin at ~/.claude/skills/<name>/
```

Also: `claude --plugin-dir <path>` / `--plugin-url <url>` load a plugin for a single session
(handy for testing before publishing).

## Correct install model for click-ai-devkit (reverses D16)

`click-ai-devkit` must ship a `.claude-plugin/marketplace.json` at the repo root listing the three
plugins (click-sdd, click-memory, click-review) with their `plugins/<name>` paths. Then `click install`
becomes an ORCHESTRATOR over the official CLI — not a registry writer:

```
claude plugin marketplace add https://github.com/Angel-MercadoCLK/click-ai-devkit \
    --sparse .claude-plugin plugins
claude plugin install click-sdd@click-ai-devkit
claude plugin install click-memory@click-ai-devkit
claude plugin install click-review@click-ai-devkit
```

This makes D16 a confirmed **mistake to reverse**: the marketplace.json is not an optional second
install path — it is THE mechanism Claude Code uses to load a plugin's agents/skills/hooks/MCP.

## Consequences for the v0.2 asks

- **#3 Actually install Engram** → trivial and official now:
  `claude plugin marketplace add https://github.com/Gentleman-Programming/engram` +
  `claude plugin install engram@engram`. That IS how Engram is installed on this machine
  (engram@engram, cached under plugins/cache/engram/...). The engram plugin brings its own
  `.mcp.json`; the engram binary is separate (scoop/brew/release) — confirm whether the plugin
  install alone is enough or the binary must also be fetched (Spike A: binary at
  AppData/Local/engram/bin/engram.exe on this machine).
- **#2 Model per SDD phase** → likely via a `userConfig` schema declared in the click-sdd plugin
  manifest, set with `claude plugin install ... --config model.explore=opus` (same path as the
  interactive `/plugin configure`). ALTERNATIVE: write the chosen `model:` into each installed
  agent's frontmatter. Decide during build — verify userConfig can drive per-agent model.
- **#4 Skills actually load** → solved for free once we register via the CLI instead of copying.
- **#1 Interactive TUI** → the TUI becomes a bubbletea front-end that runs the above `claude plugin`
  commands with a progress view + the model-routing choices.
- **#5 Agent-creator** → `claude plugin init` scaffolds a plugin/skill; our agent-builder skill can
  drive an interview then call it (or write the agent .md directly into a plugin it manages).

## What the `click` CLI is still uniquely for (its real value-add)

The `claude` CLI handles plugin registration; `click` still owns: the branded interactive TUI, the
model-routing config, the deterministic **memory-guard** PreToolUse hook (still ours), the Engram
binary pin/verification, `doctor`, and one-command orchestration of all the `claude plugin` calls.

## Residual to confirm during build (don't guess)

1. Does `--sparse .claude-plugin plugins` correctly register a monorepo whose marketplace.json lists
   plugins under `plugins/<name>`? Test with `claude plugin marketplace add <local path> --sparse ...`
   against a THROWAWAY `~/.claude` (CLICK_CLAUDE_HOME-style), never the real one.
2. Can a `userConfig` schema drive per-agent model selection, or must we rewrite agent frontmatter?
3. Does `claude plugin install engram@engram` alone make Engram fully work, or is the separate
   binary fetch still required (and how does gentle-ai/the engram installer do it)?
4. Exact marketplace.json schema — validate with `claude plugin validate <path>` before shipping.
