# click-ai-devkit

An installable Claude Code system for Click Seguros: a custom orchestrator
(`ClickOrchestrator`), an internal SDD flow, specialized agents and skills, a deterministic
memory-safety guard, and a bundled, pinned Engram instance — installed and managed by a single
Go CLI, `click`.

## Status

**v0.1.** The `click` CLI has a real install/uninstall/update/doctor implementation (see
`internal/cli/` and `internal/installer/`) — see `documentacion/implementation-plan.md` for the
full build plan and slice history.

## Install

`click` is installable via the Click scoop bucket (bucket repo pending creation — not live yet):

```
scoop bucket add click https://github.com/Angel-MercadoCLK/scoop-bucket
scoop install click
```

(Brew tap for macOS/Linux is scaffolded in `.goreleaser.yaml` but not yet a primary channel —
see `documentacion/tech-spec.md` §6.)

## Commands

| Command | What it does |
|---|---|
| `click install` | Copies the `click-sdd`, `click-memory`, and `click-review` plugins into `~/.claude/plugins/`, writes/updates the managed `CLAUDE.md` block, and registers the `memory-guard` PreToolUse hook. |
| `click doctor` | Read-only health check: verifies the three plugins, the managed `CLAUDE.md` block, and the `memory-guard` hook registration (5 checks total). Never mutates state. |
| `click uninstall` | Reverses everything `install`/`update` write: removes the three plugins, strips the managed `CLAUDE.md` block, deregisters the `memory-guard` hook, and removes the Engram MCP config/state if `update` ever configured it. Idempotent. |
| `click update` | Re-syncs the three plugins, rewrites the managed `CLAUDE.md` block, re-registers the `memory-guard` hook, and configures the pinned Engram MCP entry. This is currently the only command that writes the Engram MCP entry — `click install` does not configure it yet (tracked as a follow-up). |
| `click --version` | Prints the CLI version, injected at build time via `ldflags` (`internal/version`). |

## What gets installed

- Three plugins under `~/.claude/plugins/`: `click-sdd`, `click-memory`, `click-review`, embedded
  into the binary at build time (`go:embed`).
- A managed block in `~/.claude/CLAUDE.md`, delimited by markers so it can be inserted, replaced,
  or fully removed without touching the rest of the file.
- The `memory-guard` PreToolUse hook entry in `~/.claude/settings.json`.
- A pinned Engram MCP entry (`~/.claude/mcp/engram.json`) plus a small state file recording the
  pinned version — written by `click update` (see the table above).

## memory-guard

`memory-guard` is a Claude Code PreToolUse hook that intercepts every `mem_save` call before it
can reach Engram. It is:

- **Block-only** in v0.1 (`internal/guard`): a matching forbidden pattern denies the call outright;
  there is no redaction path yet (planned for v0.2).
- **Fail-closed**: any internal error (payload decode failure, pattern-load failure, panic) also
  results in a deny, never a silent allow.
- **Hash-only audit**: every decision is appended to a local JSONL log
  (`~/.claude/logs/click-memory-guard.jsonl`) containing only a SHA-256 hash of the payload, the
  decision, category, and session id — never the raw content (`internal/audit`).

## Repo layout

- `cmd/click/` — CLI entrypoint.
- `internal/cli/` — cobra command tree (install/doctor/uninstall/update/memory-guard).
- `internal/installer/` — install/uninstall logic: plugins, `CLAUDE.md` block, hook registration, Engram MCP config.
- `internal/doctor/` — read-only health checks.
- `internal/guard/` — the memory-guard pattern-matching engine.
- `internal/audit/` — hash-only audit logging for guard decisions.
- `internal/manifest/` — the embedded release manifest (plugin/Engram version pins).
- `plugins/` — the three embedded plugins' source content.

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
