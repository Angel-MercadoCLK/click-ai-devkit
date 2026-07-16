# click-ai-devkit

An installable Claude Code system for Click Seguros: a custom orchestrator
(`ClickOrchestrator`), an internal SDD flow, specialized agents and skills, a deterministic
memory-safety guard, and a bundled, pinned Engram instance — installed and managed by a single
Go CLI, `click`.

## Status

**v0.2.1**, released and installable today via `scoop install click` (see Install below). The
`click` CLI has a real install/uninstall/update/doctor implementation, a standing interactive
menu (bare `click` on a TTY), an 18-phase per-phase model configuration (9 SDD flow phases +
Judgment Day's 3 roles + the 5 review-lens roles) with orchestration profiles
(balanced/cost-saver/quality/custom), and an agent-builder flow nearing merge on its own branch
(see `internal/cli/`, `internal/installer/`, `internal/modelconfig/`, `internal/menu/`) — see
`documentacion/implementation-plan.md` for the full build plan and slice history.

## Install

`click` is installable via scoop directly from this repo — GoReleaser publishes the manifest
into the `bucket/` folder on every tagged release, so no separate bucket repo is needed:

```
scoop bucket add click https://github.com/Angel-MercadoCLK/click-ai-devkit
scoop install click
```

This works today (`bucket/click.json` is published and live at v0.2.1+). Brew tap for
macOS/Linux is scaffolded in `.goreleaser.yaml` but deferred — see `documentacion/tech-spec.md` §6.

> **`scoop update` needs git.** scoop refreshes its buckets with `git pull`; if git is not
> installed for scoop, `scoop update click` silently keeps reading the stale local manifest and
> reports the currently-installed version as "latest" even when a newer `click` release exists. If
> a new version is not showing up, run `scoop install git` first, then `scoop update; scoop update
> click`. (This is scoop's own requirement, separate from the `git` that `click install`/`update`
> need to register the plugin marketplace — see `PreflightGit`.)

## Commands

| Command | What it does |
|---|---|
| `click` (no args, TTY) | Launches the standing interactive menu (`internal/menu`) to reach install/update/doctor/uninstall/configure-models without memorizing flags. Non-TTY or `--no-interactive` prints help instead. |
| `click install` | Registers the Click marketplace with `claude plugin marketplace add`, installs `click-sdd`, `click-memory`, and `click-review` via the native `claude plugin` CLI, writes/updates the managed `CLAUDE.md` block, registers the `memory-guard` PreToolUse hook, and lets you choose per-phase models (or an orchestration profile) interactively. |
| `click doctor` | Read-only health check: verifies the three plugins are actually registered in Claude Code, the managed `CLAUDE.md` block, and the `memory-guard` hook registration. Never mutates state. |
| `click uninstall` | Reverses everything `install`/`update` write: uninstalls the three plugins through the native `claude plugin` CLI, removes the Click marketplace registration, strips the managed `CLAUDE.md` block, deregisters the `memory-guard` hook, and removes the Engram MCP config/state if `update` ever configured it. Idempotent. |
| `click update` | Re-runs the native `claude plugin` install flow to re-sync the three plugins, rewrites the managed `CLAUDE.md` block, re-registers the `memory-guard` hook, and configures the pinned Engram MCP entry. |
| `click configure-models` | Reopens the per-phase model selection TUI (18 phases, or pick an orchestration profile) without a full install/update, preserving the currently persisted profile label. Hidden from `--help`, reached primarily through the standing menu. |
| `click --version` | Prints the CLI version, injected at build time via `ldflags` (`internal/version`). |

## What gets installed

- Three Claude Code plugins registered through the native marketplace/registry flow: `click-sdd`,
  `click-memory`, and `click-review`.
- A managed block in `~/.claude/CLAUDE.md`, delimited by markers so it can be inserted, replaced,
  or fully removed without touching the rest of the file.
- The `memory-guard` PreToolUse hook entry in `~/.claude/settings.json`.
- Engram installed as a Claude Code plugin (`engram@engram`), idempotent and respectful of a
  pre-existing setup; its binary is provisioned via `go install` when missing. A small click state
  file records what click itself installed, so `uninstall` never removes a dev's pre-existing Engram.
- Context7 registered as a user-scope HTTP MCP via `claude mcp add` — also idempotent and respectful.

## memory-guard

`memory-guard` is a Claude Code PreToolUse hook that intercepts every `mem_save` call before it
can reach Engram. It is:

- **Block-only** as of v0.2.1 (`internal/guard`): a matching forbidden pattern denies the call
  outright; there is no redaction path yet (still planned, not scheduled).
- **Fail-closed**: any internal error (payload decode failure, pattern-load failure, panic) also
  results in a deny, never a silent allow.
- **Hash-only audit**: every decision is appended to a local JSONL log
  (`~/.claude/logs/click-memory-guard.jsonl`) containing only a SHA-256 hash of the payload, the
  decision, category, and session id — never the raw content (`internal/audit`).

## Repo layout

- `cmd/click/` — CLI entrypoint.
- `internal/cli/` — cobra command tree (install/doctor/uninstall/update/configure-models/memory-guard).
- `internal/installer/` — install/uninstall logic: plugins, `CLAUDE.md` block, hook registration, Engram MCP config.
- `internal/doctor/` — read-only health checks.
- `internal/guard/` — the memory-guard pattern-matching engine.
- `internal/audit/` — hash-only audit logging for guard decisions.
- `internal/manifest/` — the embedded release manifest (plugin/Engram version pins).
- `internal/menu/` — the standing interactive menu (bare `click` on a TTY).
- `internal/modelconfig/` — the 18-phase per-phase model taxonomy, defaults, and orchestration profiles.
- `internal/ui/` — shared bubbletea TUI screens (model selection, profile selection, rendering helpers).
- `internal/version/` — build-time version metadata injected via `ldflags`.
- `plugins/` — the three plugin source trees served by the Click marketplace.

## Docs

Full planning and design docs live in [`documentacion/`](documentacion/), including the vision,
decisions log (`00-decisions-and-open-questions.md`), technical spec, and implementation plan.

## Development

```
go build ./...
go test ./...
```

STRICT TDD is mandatory for any Go change in this repo: write a failing test first, then the
minimal implementation to make it pass. See `documentacion/00-decisions-and-open-questions.md`
(D13) and `CLAUDE.md`.
