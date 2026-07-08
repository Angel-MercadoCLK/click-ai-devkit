# click-ai-devkit

An installable Claude Code system for Click Seguros: a custom orchestrator
(`ClickOrchestrator`), an internal SDD flow, specialized agents and skills, a deterministic
memory-safety guard, and a bundled, pinned Engram instance — installed and managed by a single
Go CLI, `click`.

## Status

**v0.1 in progress.** This repo currently contains the Phase 0 bootstrap skeleton only: a
compilable `click` CLI with stub `install` / `update` / `doctor` / `uninstall` commands. No real
install logic exists yet — see `documentacion/implementation-plan.md` for the build plan.

## Install (future)

Once a release is published, `click` will be installable via the Click scoop bucket:

```
scoop bucket add click-seguros <url>
scoop install click
```

(Brew tap for macOS/Linux follows the same binary — see `documentacion/tech-spec.md` §6.)

## Docs

Full planning and design docs live in [`documentacion/`](documentacion/), including the vision,
decisions log, technical spec, and implementation plan.
