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
- [x] T2 — Hook registration covers all of them; existing installs with the legacy matcher are upgraded idempotently; uninstall and doctor recognize old and new entries. Route: delegated.
- [x] T3 — OpenClaw hook recognition and docs aligned with the new coverage. Route: delegated.

- [x] T4 — Prove the anchored regex matcher fires in a live Claude Code session (and does not fire for read-only tools); make `click doctor` verify the registered matcher actually matches every persisting tool in both name forms, and address R4 (no fallback once the legacy entry is removed) based on the live result. Route: live check inline (bounded test action); code delegated (writer trigger: 2+ non-trivial files).

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
- Commit: 2be004b `fix(guard): screen every persisting engram tool`.
- Route: delegated writer (writer trigger: 2+ non-trivial files).
- Single source of truth: `internal/guard/tools.go` (`PersistingEngramTools`, `IsPersistingEngramTool`, `ToolMatcherPattern`). Name forms: `mcp__plugin_engram_engram__<tool>` and `mcp__engram__<tool>`.
- Persisting set: mem_capture_passive, mem_compare, mem_judge, mem_save, mem_save_prompt, mem_session_end, mem_session_summary, mem_update. Evidence: `engram mcp` tools/list on installed Engram v3.2.1 (pinned ENGRAM_VERSION is v1.20.0; the set is a superset, extra names are harmless). Not persisting-with-text: mem_pin/unpin, mem_delete, mem_session_start, mem_merge_projects, mem_review; read-only: mem_search, mem_context, mem_get_observation, mem_current_project, mem_timeline, mem_stats, mem_list_projects, mem_doctor, mem_suggest_topic_key.
- Guard scans the whole canonical `tool_input` JSON (every field), unchanged mechanism; fail-closed paths untouched.
- RED: `go test ./internal/cli -run 'TestEvaluateMemoryGuard|TestMemoryGuardCommand_DeniesMemUpdate'` failed (mem_update, mem_save standalone, all other persisting tools); `go test ./internal/guard` failed to compile (undefined PersistingEngramTools/IsPersistingEngramTool).
- GREEN: TestEvaluateMemoryGuard_DeniesEveryPersistingEngramToolInBothNameForms, TestEvaluateMemoryGuard_ScansEveryFieldOfMemUpdate, TestEvaluateMemoryGuard_AllowsReadOnlyAndUnrelatedToolsWithoutScanning, TestMemoryGuardCommand_DeniesMemUpdateAndStandaloneForms, TestPersistingEngramTools_*, TestIsPersistingEngramTool, TestToolMatcherPattern_MatchesExactlyTheAcceptedNames.
- Known environmental failure (also fails on base e845227): `TestDispatch_SubcommandRuntimeFailure_PrintsErrorExactlyOnceNoUsageDump`.

### T2 evidence
- Commit: 1303d36 `fix(installer): register the memory guard for every persisting engram tool`.
- Route: delegated writer.
- Matcher registered: `^(?:mcp__plugin_engram_engram__|mcp__engram__)(?:mem_capture_passive|mem_compare|mem_judge|mem_save|mem_save_prompt|mem_session_end|mem_session_summary|mem_update)$` (`installer.MemoryGuardToolMatcher`, derived from `guard.ToolMatcherPattern()`); legacy `mcp__plugin_engram_engram__mem_save` kept as `MemoryGuardLegacyToolMatcher`.
- Register strips the legacy Click hook (foreign hooks sharing the entry survive) then ensures one current entry; uninstall, doctor (`HasLegacyMemoryGuardHook` -> "desactualizado ... click update") and snapshot projection recognize both.
- RED: `go test ./internal/installer -run 'MemoryGuard|ManagedSettingsProjection'` failed (TestMemoryGuardToolMatcher_CoversEveryPersistingToolInBothNameForms, TestHasMemoryGuardHook_LegacyOnlyIsNotFullyRegistered; first run failed to compile on undefined MemoryGuardLegacyToolMatcher); `go test ./internal/doctor -run CheckMemoryGuardHook` failed (TestCheckMemoryGuardHook_LegacyMatcherReportsOutdatedCoverage).
- GREEN: TestMemoryGuardToolMatcher_CoversEveryPersistingToolInBothNameForms, TestRegisterMemoryGuardHook_RegistersExactlyOneEntryWithFullMatcher, TestRegisterMemoryGuardHook_UpgradesLegacyMatcherToExactlyOneEntry, TestRegisterMemoryGuardHook_UpgradeKeepsForeignHooksSharingTheLegacyEntry, TestHasMemoryGuardHook_LegacyOnlyIsNotFullyRegistered, TestUnregisterMemoryGuardHook_RemovesLegacyAndCurrentEntries, TestManagedSettingsProjectionHash_TreatsLegacyGuardEntryAsOwned, TestCheckMemoryGuardHook_LegacyMatcherReportsOutdatedCoverage, TestCheckMemoryGuardHook_CurrentMatcherIsHealthy.
- Unverified: that Claude Code accepts this anchored regex form in a live session (matcher regex semantics per Claude Code hooks docs; not exercised at runtime here).

### T3 evidence
- Route: delegated writer.
- `hooks.js` now carries `PERSISTING_ENGRAM_TOOLS` (same 8 tools), prefixes `mcp__plugin_engram_engram__`, `mcp__engram__`, `engram__`, `engram.`, longest-tool fallback for names containing "engram"; rewrites to `mcp__plugin_engram_engram__<same tool>`. Fail-closed spawn/timeout/stdin semantics untouched. `internal/guard/tools_openclaw_test.go` fails if the JS list drifts from Go.
- RED: `node --test .../hooks.test.js` 6 failing (isPersistingEngramTool x3, resolvePersistingTool, beforeToolCall mem_update, buildPreToolUsePayload canonical rewrite); `go test ./internal/guard -run OpenClaw` failed ("hooks.js must declare const PERSISTING_ENGRAM_TOOLS").
- GREEN: node --test 21/21 pass; TestOpenClawHooksListsTheSamePersistingTools passes.
- Docs updated: README.md (memory-guard section), SECURITY.md, plugins/click-memory/docs/memory-policy.md, documentacion/architecture.md. Not edited (historical/locked): D7, D35 in documentacion/00-decisions-and-open-questions.md and the FR-038 matcher snippet in documentacion/tech-spec.md still say mem_save; flagged for the parent.
- OpenClaw enforcement remains UNVERIFIED at runtime (tool names are a best guess); docs say so.

### T4 evidence
- Commits: 7ac75e9 `fix(installer): identify Click guard hooks by command and expose the matcher read-only`; 7ca7fd9 `fix(doctor): verify the memory-guard matcher covers every persisting tool`.
- Route: live check inline by the parent (bounded test action); code delegated writer (writer trigger: 2+ non-trivial files).
- Live evidence (parent): Claude Code 2.1.295, `claude -p` with `--setting-sources project --settings <file> --mcp-config <fake engram MCP> --strict-mcp-config`, model haiku, 2026-10-09.
  - Run 1 (logging hook, registered matcher `^(?:mcp__plugin_engram_engram__|mcp__engram__)(?:mem_capture_passive|mem_compare|mem_judge|mem_save|mem_save_prompt|mem_session_end|mem_session_summary|mem_update)$`): hook fired for `mcp__engram__mem_update` and `mcp__engram__mem_save`, did NOT fire for `mcp__engram__mem_search`; all three tool calls executed.
  <!-- click:allow-secret synthetic sequential-placeholder PII used only for isolated guard verification; not real user data. -->
  - Run 2 (hook = real `click memory-guard` built from this branch, isolated CLICK_CLAUDE_HOME): `mem_update` with "CUIT 20-12345678-3 del asegurado" was blocked by PreToolUse ("blocked: detected pii") and never reached the MCP server; `mem_save` with "Store architecture decisions only" was allowed; audit log recorded deny(pii)/allow with content_sha256 only.
  - Not exercised live: the `mcp__plugin_engram_engram__` prefix (same regex alternation).
- R4 decision: the regex matcher is proven to fire in current Claude Code, so no legacy fallback entry is added; the doctor probe below is the ongoing safety net.
- Doctor probe: `guard.VerifyToolMatcher` (compile as regexp; must match every persisting tool in both name forms; must not match mem_search/mem_get_observation/mem_context in either form nor `Bash`). `checkMemoryGuardHook` runs it on the matchers actually registered (health is decided by behavior, not string equality); failure is Spanish with `click update`. Go RE2 is a proxy for Claude Code's regex semantics (the live run confirmed the canonical form).
  - RED: `go test ./internal/guard -run VerifyToolMatcher` failed to compile (undefined VerifyToolMatcher); `go test ./internal/doctor -run CheckMemoryGuardHook` failed: ProbeFailsWhenMatcherMissesAPersistingTool, ProbeFailsWhenMatcherDoesNotCompile, ProbeFailsWhenMatcherOvermatchesReadOnlyTools, BehaviorNotStringDecidesHealth.
  - GREEN: TestVerifyToolMatcher_{AcceptsTheCanonicalPattern,RejectsMatchersThatDoNotCompile,RejectsMatchersThatMissAPersistingTool,RejectsMatchersThatOvermatchReadOnlyTools,AcceptsAnEquivalentHandWrittenMatcher}; TestCheckMemoryGuardHook_{ProbeFails...x3,BehaviorNotStringDecidesHealth,SurfacesUnreadableSettings (already green: pins the surfaced error)} plus the T2 legacy/current tests.
- R2 swallowed legacy-check error: doctor no longer drops errors; reading registered matchers and `HasLegacyMemoryGuardHook` errors are returned as the check Detail (covered by TestCheckMemoryGuardHook_SurfacesUnreadableSettings; the legacy-call error branch is not separately testable because both reads hit the same file).
- R2 mutable `var`: `installer.MemoryGuardToolMatcher` is now a func over the unexported `memoryGuardToolMatcher`. RED: `go vet ./internal/installer ./internal/cli` failed to compile after tests switched to `MemoryGuardToolMatcher()` ("string is not a function"); GREEN after the accessor (compile-level change, no runtime assertion possible for non-reassignability).
- R3 matcher drift: Click guard hooks are identified by `MemoryGuardCommand` in register, uninstall, `HasLegacyMemoryGuardHook` (now: any Click hook under a non-current matcher), and `managedSettingsProjectionHash`. Register keeps the first Click hook under the current matcher and removes every other (drifted, legacy, duplicate); foreign hooks survive. New `installer.RegisteredMemoryGuardMatchers`.
  - RED: `go test ./internal/installer` failed: TestRegisterMemoryGuardHook_UpgradesDriftedMatcherToExactlyOneEntry, TestRegisterMemoryGuardHook_CollapsesDuplicateCurrentEntries, TestUnregisterMemoryGuardHook_RemovesDriftedEntry, TestHasLegacyMemoryGuardHook_TreatsDriftedMatcherAsOutdated, TestManagedSettingsProjectionHash_TreatsDriftedGuardEntryAsOwned.
  - GREEN: the same tests plus TestRegisteredMemoryGuardMatchers_{ReturnsOnlyClickOwnedMatchers,MissingSettingsReturnsNone}. Changed an existing assertion: TestManagedSettingsProjectionHash "ownership follows the command, not the matcher" (previously asserted a Click command under a wrong matcher is NOT owned; that is the behavior R3 intentionally reverses).
- Docs: README `click doctor` row mentions the matcher-coverage probe.

## Verification (final)
- `go build ./...` ok; `go vet ./...` ok; `go test ./...` all packages ok except the pre-existing environmental failure noted above (`TestDispatch_SubcommandRuntimeFailure_PrintsErrorExactlyOnceNoUsageDump`: real `openclaw` CLI help output contains "Usage:"; fails identically on base e845227).

## Review (RDD)
- Assessment: high (`hot_path` SECURITY.md), base e845227, commits 2be004b..b862635. Consent: granted by user.
- Native 4-lens review, lineage `review-56c48f4abac0988c`: approved; acknowledged, authority burned (consumed revision sha256:2e12bb12...).
- Non-blocking follow-ups (informational, separate later work):
  - R3/R4/R1: the anchored regex matcher is not verified in a live Claude Code session; doctor only compares the matcher string, so a non-firing matcher would still read healthy. Add a runtime smoke check or a doctor probe.
  - R3: a drifted (hand-edited) Click matcher is not recognized as Click-owned (`internal/installer/hooksettings.go:77-81`).
  - R2: mutable matcher `var` (`hooksettings.go:25`), swallowed legacy-check error (`internal/doctor/checks.go:894`), unreachable-fallback comment (`hooks.js:128-130`), duplicated prefix list in `hooks.test.js:51`, stale line anchors in this doc.

## Next step
Remaining non-blocking follow-ups (R2: unreachable-fallback comment `hooks.js:128-130`, duplicated prefix list `hooks.test.js:51`, stale line anchors in this doc); the R3/R4/R1 and R2 items for `hooksettings.go`/`checks.go` are closed by T4. Optionally re-run native review on 7ac75e9..7ca7fd9; reword D7/D35/FR-038 if wanted; push/PR are the user's call.

## Review (RDD) — T4 slice
- Assessment: medium, `slice_budget_reached`, base e78e936, commits 7ac75e9..c612a01. Consent: granted by user.
- Native review (single consolidated reliability lens), lineage acknowledged, authority burned: approved.
- Non-blocking follow-ups: R3-001 (WARNING) snapshots recorded by older builds for a hand-edited Click matcher project `absent:` and now `present:`, so SnapshotDrift may report a spurious managed-content veto after upgrade (`internal/installer/snapshot_projection.go:99`); R3-002 the "matcher que no protege" doctor branch is effectively unreachable (`internal/doctor/checks.go:914-921`); R3-003 plain-name matchers may be exact-match in Claude Code vs unanchored RE2 in the probe (`internal/guard/tools.go:95-102`), no false-healthy result.
