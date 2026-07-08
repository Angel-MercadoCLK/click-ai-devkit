# click-ai-devkit — contributor guide

`click-ai-devkit` is a Go CLI (`click`) that installs Click Seguros's Claude Code system
(orchestrator, SDD flow, plugins, memory-guard, pinned Engram) into a developer's `~/.claude`.

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

All artifacts and code comments are in English: docs, README, code, commit messages, PR
descriptions, and any string literal in source.

## Commits

Conventional commits. No AI attribution in commit messages.

## Decisions

Locked decisions (D1–D22) live in `documentacion/00-decisions-and-open-questions.md`. Read it
before changing behavior or docs — do not contradict a locked decision.

In particular: **never create `.claude-plugin/marketplace.json`** (D16). This repo's own
embedded `internal/manifest/manifest.yaml` is the only source of truth for install content.

## Plugins

The three plugins (`plugins/click-sdd/`, `plugins/click-memory/`, `plugins/click-review/`) are
embedded into the binary via `go:embed` (see each plugin's `embed.go`). When adding or changing
plugin files, update the corresponding `embed.go`, the installer copy logic
(`internal/installer/plugins.go`), the relevant `internal/doctor` check, and their tests
together — these four stay in sync by convention, not by a generated check.
