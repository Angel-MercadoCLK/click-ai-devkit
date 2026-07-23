# click-ai-devkit — contributor guide

`click-ai-devkit` is a Go CLI (`click`) that installs Click Seguros's Claude Code system
(orchestrator, SDD flow, plugins, memory-guard, pinned Engram) into `~/.claude`, plus supported
OpenClaw and Codex target guidance. See `documentacion/portability-runbook.md` and
`documentacion/codex-target.md`.

## Build & test

```
go build ./...
go test ./...
```

**STRICT TDD is mandatory for any Go change.** Write a failing test first (red), then the
minimal implementation to make it pass (green). Do not skip this — see D13 in
`documentacion/00-decisions-and-open-questions.md`.

Tests must never touch the real `~/.claude`. Use the `CLICK_CLAUDE_HOME` env var override
(`t.Setenv("CLICK_CLAUDE_HOME", t.TempDir())`) — see `internal/installer/config.go` and its
tests for the established pattern.

## Language

Docs, README, code, and comments are in English. Dev-facing CLI/TUI strings are Spanish, matching
the shipped UI and D10.

## Commits

Conventional commits. No AI attribution in commit messages.

## Decisions

The locked decisions live in `documentacion/00-decisions-and-open-questions.md` (see that file for
the current range). Read it before changing behavior or docs — do not contradict a locked decision.

In particular: ship and maintain `.claude-plugin/marketplace.json` because Claude Code loads the
Click plugins through its native `claude plugin` registry flow (D24 supersedes D16). Do not assume
that registry flow is an OpenClaw or Codex activation protocol.

## Plugins

The four plugins (`plugins/click-sdd/`, `plugins/click-memory/`, `plugins/click-review/`,
`plugins/click-skills/`) are
served through the repo marketplace manifest. When adding or changing plugin files, keep
`.claude-plugin/marketplace.json` and the native `claude plugin` install flow consistent.
(`internal/installer/plugins.go`), the relevant `internal/doctor` check, and their tests
together — these four stay in sync by convention, not by a generated check.
