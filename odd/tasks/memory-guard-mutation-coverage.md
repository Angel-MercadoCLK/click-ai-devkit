# memory-guard-mutation-coverage

## Objective
Every Engram operation that persists content is screened by `click memory-guard`, under every tool-name form the supported runtimes expose.

## Problem
The Claude Code PreToolUse hook matches only `mcp__plugin_engram_engram__mem_save` (`internal/installer/hooksettings.go:13`), and `evaluateMemoryGuard` allows any other tool name unscreened (`internal/cli/memoryguard.go:111-115`). Content can therefore reach persistent memory through `mem_update` (granted to `click-apply`, `plugins/click-sdd/agents/click-apply.md:4,45`), other persisting tools, or the standalone MCP naming `mcp__engram__*`.

## Why
Click Seguros handles insurance/customer data; the memory guard is the devkit's main enterprise control. Source: Codex gpt-6-astra high audit, 2026-10-08, verified by the parent.

## Scope (authorized)
- Claude Code hook registration, upgrade of existing installs that hold the legacy single-tool matcher, uninstall cleanup, doctor check.
- Guard tool-name evaluation (`internal/cli/memoryguard.go`) and which payload fields are scanned per tool.
- OpenClaw `hooks.js` tool recognition, kept consistent with Claude.
- Docs that describe guard coverage.
Out of scope: guard rule semantics (blocklist vs allowlist), gentle-ai migration, retiring click-sdd.

## Constraints
- STRICT TDD (D13, `AGENTS.md`). Mode: on. Source: repo `AGENTS.md`. Runner: `go test ./...`.
- Tests never touch real `~/.claude`; use `CLICK_CLAUDE_HOME`.
- Dev-facing CLI strings in Spanish (D10); code/docs/commits in English.
- Fail closed on guard errors (existing behavior).

## Tasks
- [x] T1 — Define the set of persisting Engram tools and accepted name forms in one place; guard evaluates all of them and scans the correct content fields. Route: delegated (writer trigger: 2+ non-trivial files).
- [ ] T2 — Hook registration covers all of them; existing installs with the legacy matcher are upgraded idempotently; uninstall and doctor recognize old and new entries. Route: delegated.
- [ ] T3 — OpenClaw hook recognition and docs aligned with the new coverage. Route: delegated.

## Acceptance criteria
- A blocked payload is denied via `mem_save`, `mem_update` and every other persisting tool, in plugin (`mcp__plugin_engram_engram__*`) and standalone (`mcp__engram__*`) forms.
- Read-only Engram tools stay allowed without scanning.
- Re-running install on a legacy settings file yields exactly one Click guard entry.
- `go build ./...`, `go vet ./...`, `go test ./...` pass.

## Delivery
Strategy: ask-on-risk. Forecast: ~250-400 authored changed lines.

## Progress / evidence
- Branch `fix/memory-guard-mutation-coverage` created from `main` @ e845227.
- RDD: on (global).
- Engram mirror `odd/memory-guard-mutation-coverage/tasks`: PENDING (mem_current_project ambiguous: engram cwd resolves to parent dir, no click-ai-devkit project).

### T1 evidence
- Route: delegated writer (writer trigger: 2+ non-trivial files).
- Single source of truth: `internal/guard/tools.go` (`PersistingEngramTools`, `IsPersistingEngramTool`, `ToolMatcherPattern`). Name forms: `mcp__plugin_engram_engram__<tool>` and `mcp__engram__<tool>`.
- Persisting set: mem_capture_passive, mem_compare, mem_judge, mem_save, mem_save_prompt, mem_session_end, mem_session_summary, mem_update. Evidence: `engram mcp` tools/list on installed Engram v3.2.1 (pinned ENGRAM_VERSION is v1.20.0; the set is a superset, extra names are harmless). Not persisting-with-text: mem_pin/unpin, mem_delete, mem_session_start, mem_merge_projects, mem_review; read-only: mem_search, mem_context, mem_get_observation, mem_current_project, mem_timeline, mem_stats, mem_list_projects, mem_doctor, mem_suggest_topic_key.
- Guard scans the whole canonical `tool_input` JSON (every field), unchanged mechanism; fail-closed paths untouched.
- RED: `go test ./internal/cli -run 'TestEvaluateMemoryGuard|TestMemoryGuardCommand_DeniesMemUpdate'` failed (mem_update, mem_save standalone, all other persisting tools); `go test ./internal/guard` failed to compile (undefined PersistingEngramTools/IsPersistingEngramTool).
- GREEN: TestEvaluateMemoryGuard_DeniesEveryPersistingEngramToolInBothNameForms, TestEvaluateMemoryGuard_ScansEveryFieldOfMemUpdate, TestEvaluateMemoryGuard_AllowsReadOnlyAndUnrelatedToolsWithoutScanning, TestMemoryGuardCommand_DeniesMemUpdateAndStandaloneForms, TestPersistingEngramTools_*, TestIsPersistingEngramTool, TestToolMatcherPattern_MatchesExactlyTheAcceptedNames.
- Known environmental failure (also fails on base e845227): `TestDispatch_SubcommandRuntimeFailure_PrintsErrorExactlyOnceNoUsageDump`.

## Next step
T2.
