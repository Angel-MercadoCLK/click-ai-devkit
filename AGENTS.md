# click-ai-devkit — contributor guide

`click-ai-devkit` is a Go CLI (`click`) that installs Click Seguros's Claude Code system
(orchestrator, SDD flow, plugins, memory-guard, pinned Engram) into a developer's `~/.claude`,
plus the supported OpenClaw-native workflow and Codex `AGENTS.md` guidance when those targets are
selected. See `documentacion/portability-runbook.md` and `documentacion/codex-target.md`.

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

Docs, README, code, code comments, commit messages, and PR descriptions are in English. Per D10
(`documentacion/00-decisions-and-open-questions.md`), dev-facing CLI/TUI **string literals** — the
text a developer actually sees when running `click` (menu labels, prompts, flag help,
`click doctor`/`click install` output, etc.) — are Spanish, matching the shipped UI. Do not write
new dev-facing strings in English or translate existing Spanish ones; do not contradict D10.

## Commits

Conventional commits. No AI attribution in commit messages.

## Decisions

The locked decisions live in `documentacion/00-decisions-and-open-questions.md` (see that file for
the current range). Read it before changing behavior or docs — do not contradict a locked decision.

In particular: ship and maintain `.claude-plugin/marketplace.json` because Claude Code loads the
Click plugins through its native `claude plugin` registry flow (D24 supersedes D16). OpenClaw and
Codex use their own documented target boundaries; do not copy Claude plugin registration into them.

## Plugins

The four plugins (`plugins/click-sdd/`, `plugins/click-memory/`, `plugins/click-review/`,
`plugins/click-skills/`) are
served through the repo marketplace manifest. When adding or changing plugin files, keep
`.claude-plugin/marketplace.json` and the native `claude plugin` install flow consistent.
(`internal/installer/plugins.go`), the relevant `internal/doctor` check, and their tests
together — these four stay in sync by convention, not by a generated check.

## SDD phase taxonomy

The real SDD phase chain is 18 phases (was 13 before the 5 review-lens roles landed):
`default`, `explore`, `propose`, `spec`, `design`, `tasks`, `apply`, `verify`, `archive`,
`onboard`, `jd-judge-a`, `jd-judge-b`, `jd-fix-agent`, `review-risk`, `review-readability`,
`review-reliability`, `review-resilience`, `review-refuter`. Each has a `<phase>_model` config key
in `plugins/click-sdd/.claude-plugin/plugin.json` — see
`plugins/click-sdd/agents/click-orchestrator.md` for the routing rules. Do not reintroduce the
deprecated 5-phase taxonomy (`orchestrator`/`prd_writer`/`architect`/`reviewer`/`memory_curator`
as phase keys) in any new agent, skill, or config file.
