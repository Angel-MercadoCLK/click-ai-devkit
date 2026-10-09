# gentle-ai-distribution

## Objective
click-ai-devkit becomes a Click Seguros distribution layered on a pinned, qualified gentle-ai release (not a fork). Click adopts ODD; the in-repo `click-sdd` orchestration engine is retired.

## Problem
The devkit maintains its own SDD orchestration engine (`plugins/click-sdd`, 35 files, 18 model keys) and fell behind upstream gentle-ai, which retired SDD in v4.0.0 (2026-10-01) in favour of ODD. There is no executable gentle-ai integration today (only reference comments in `internal/modelconfig/modelconfig.go:2`, `internal/ui/renderer.go:2`).

## Why
Decision 2026-10-08 (user): avoid repeating the maintenance gap a fork would create (gentle-ai churns ~1000 commits/month). Click owns only what is Click-specific.

## Ownership split
- Click layer: rules/conventions, Click skills, memory guard + insurance data policy, extra permissions, Scoop installer/doctor/rollback, model profiles mapped to gentle-ai roles.
- gentle-ai: workflow (ODD), review (RDD), Engram lifecycle, runtime adapters.

## Scope (authorized by user 2026-10-08: "work all of this")
Record the decision; pin and install gentle-ai from the click installer; Click layer on top; retire click-sdd (registration, installed copies, assets); doctor/rollback ownership; refresh docs.
Out of scope: push, PR creation, merge, release publishing (user decisions).

## Constraints
- STRICT TDD (D13, `AGENTS.md`). Mode: on. Source: repo `AGENTS.md`. Runner: `go test ./...` (plus `go build ./...`, `go vet ./...`).
- Tests never touch real `~/.claude` / `~/.codex` / `~/.gentle-ai`; isolated homes only (`CLICK_CLAUDE_HOME` etc.).
- Dev-facing CLI strings in Spanish (D10); code/docs/commits in English.
- Decision history is superseded, never rewritten.
- Never stage with `-a`; stage explicit paths.
- ~400 authored changed lines per task is a planning heuristic only.

## Model team (user instruction)
Current instruction (2026-10-09): writer Codex `gpt-6-luna` high; parent reviews each task before closure and continuation. This supersedes the earlier Sol/Astra/Opus assignment for remaining work. Native RDD remains governed by the user-owned switch and per-candidate consent.

## Resumed authorization (2026-10-09)
- User authorized completing the remaining plan, task-by-task implementation/review, commits, and pushes after successful checks. No merge or release publishing is authorized.
- Local work stays on `feat/gentle-ai-distribution`; existing T03a edits must be preserved and verified, not blindly reapplied.
- Push destination and credential confirmation remain pending; no remote operation before that confirmation.
- Resume point: T01–T03 committed; T03a source/tests are present but uncommitted and unchecked. Historical T03a verification is in Engram #509–#511; fresh checks are required before closure.

## Defaults taken (recommended by exploration; user may override)
- Pin gentle-ai **v4.0.0 stable**, exact release artifact + SHA-256, never "latest"; suppress upstream self-update (`GENTLE_AI_NO_SELF_UPDATE=1`).
- Click provisions the checksum-qualified upstream binary itself; Scoop still installs only Click (`bucket/click.json` has no dependency today).
- `click-review` keeps insurance/business checklist content only, not an independent review engine.

## Decided by user
- 2026-10-08 chain strategy: feature-branch-chain.
- 2026-10-08 Engram project: commit `.engram/config.json` (`project_name: click-ai-devkit`) so the team shares one Engram project; lands with T01.
- 2026-10-08 Windows provisioning: gentle-ai publishes no Windows binary (v3.5–v4.0.0 ship only darwin/linux amd64/arm64 tar.gz + minisign-signed `checksums.txt`; README documents Windows as `go install`). Windows uses `go install github.com/gentleman-programming/gentle-ai/v4/cmd/gentle-ai@v4.0.0` (integrity via Go checksum DB, requires Go >= 1.25.10, same pattern click uses for Engram); macOS/Linux use the official release tar.gz verified against the SHA-256 pinned in click. D48 must be amended accordingly (T02).
- 2026-10-09 gentle-ai selection: components `engram,skills,context7,permissions`, persona `neutral`; no `sdd`, `gga`, `theme`, `claude-theme`, `opencode-gentle-logo`. Agents `claude-code,codex,openclaw` (IDs verified in gentle-ai v4.0.0 `internal/model/types.go:58,64,70`; OpenClaw is supported upstream — the exploration claim that it was Click-only was wrong). Rollout: T03 installs gentle-ai with `skills,permissions,persona` only; T04 adds `engram,context7` while removing click's own registration, so no commit double-registers them.
- 2026-10-09 Engram Cloud: click keeps its cloud layer (D40–D47 stay in force) on top of gentle-ai-owned Engram (binary, MCP, registration). T04 re-points the cloud layer at the gentle-ai-installed Engram.
- 2026-10-08 pin drift: warn and re-pin — `click doctor` reports an installed gentle-ai that differs from the pin as drift; `click install`/`click update` reinstall the qualified version and say so explicitly (may downgrade a dev-chosen newer version).

## Open product decisions
None at present.

## Tasks
- [x] T01 — Record distribution/ownership decision; supersede D1/D8/D9/D12/D25, revisit D36/D38. Files: `documentacion/00-decisions-and-open-questions.md`, `documentacion/vision.md`. Route: inline-eligible docs; delegated to Sol per model-team instruction. Evidence: D48 added; Astra xhigh review CHANGES_REQUIRED (1 blocker: pin drift left open; 2 warnings: chain strategy missing, historical vision bullets rewritten) → all fixed by Sol; parent spot check of D48 text and restored bullets. Docs only, no Go tests.
- [x] T02 — Qualified upstream pin: `GENTLE_AI_VERSION`, `internal/installer/gentleai.go` (+test), `internal/manifest/{manifest.go,manifest.yaml,release_qualification_test.go}`, `.github/workflows/release.yml`. Tests: checksum/version rejection, self-update suppression. Route: delegated (writer trigger). Evidence: Astra xhigh CHANGES_REQUIRED (2 blockers: saved Go config/GOFLAGS bypass of checksum DB and build — parent reproduced with GOENV file; 4 warnings: toolchain gate order, HTTPS redirect downgrade, RC version parsing, decompression budget) → all fixed by Sol with RED→GREEN tests; controlled Go env GOENV=off, GOFLAGS cleared, GOSUMDB=sum.golang.org, GONOSUMDB/GOPRIVATE/GONOPROXY=none, GOPROXY=https://proxy.golang.org, GOTOOLCHAIN=local. Outside sandbox: build/vet OK, `go test ./...` only known env failure. CI release.yml not touched (deferred).
- [x] T03 — Upstream install selection/orchestration: `internal/installer/{plan.go,installer.go}`, `internal/cli/{install.go,update.go}` (+tests). Tests: exact argv, selected-agent scope, failures, idempotency. Route: delegated. Evidence: argv `install --agents <active> --preset full-gentleman --components skills,permissions,persona --persona neutral --scope global` + GENTLE_AI_NO_SELF_UPDATE=1, validated against real gentle-ai 4.0.0 `--dry-run` in an isolated HOME (exit 0). Order: snapshot → deferred bookkeeping → gentle-ai → targets/models. Astra xhigh CHANGES_REQUIRED (3 warnings: Windows-unsafe stat in tests, failure after models/targets mutation, matching pin skipped PATH repair; 1 suggestion: explicit preset) → all fixed RED→GREEN. D49 (selection, explicit preset, staged T04 rollout), D50 (Engram Cloud layer kept). Outside sandbox: build/vet OK, `go test ./...` only known env failure.
- [x] T03a — Fix review findings on T03: `click update` must narrow persisted target intent by availability (`selection.X && home != ""`), never widen it; correct the comment; keep a sanitized failure cause for gentle-ai provisioning/install errors (no raw subprocess output, no env/secrets). Files: `internal/cli/update.go`, `internal/cli/update_test.go`, `internal/installer/gentleai_install.go`, `internal/installer/gentleai_install_test.go`. Route: delegated (existing Sol implementation verified by Luna high; parent review). Evidence: commit `1e3848a`, focused tests/build/vet and parent spot check PASS; known full package-suite failures remain documented below. Native four-lens review approved and acknowledged without findings.
- [ ] T04 — Transfer Engram/Context7 ownership to gentle-ai: `internal/installer/{engram.go,context7.go,codexmcp.go,openclawmcp.go}` (+tests). Tests: pre-existing installs, non-destructive uninstall. Route: delegated. Engram Cloud decided (keep click layer).
- [ ] T05 — Click-only guidance (no competing SDD/ODD instructions): `internal/installer/{claudemd.go,codex.go,openclaw.go}` (+tests). Tests: foreign/upstream section preservation. Route: delegated.
- [ ] T06 — Guard/policy/permissions overlay: `internal/installer/hooksettings.go`, new `internal/installer/permissions.go` (+tests), `plugins/click-memory/docs/{memory-policy.md,allowed-memory.md}`. Tests: hook coexistence with gentle-ai hooks, deny preservation. Route: delegated.
- [ ] T07 — Role-profile migration to gentle-ai roles: `internal/modelconfig/*`, `internal/installer/{models.go,codexmodelprofile.go}`, `internal/cli/configuremodels.go`, `internal/ui/modelselect.go` (+tests). Requires qualifying gentle-ai's supported assignment-write path first. Route: delegated.
- [ ] T08 — Retire click-sdd registration and installed copies: `internal/installer/{plugins.go,plan.go}`, manifests, `internal/cli/uninstall.go`, `scripts/windows-package-smoke.ps1` (+tests). Tests: cleanup preserves remaining plugins. Route: delegated.
- [ ] T09 — Remove retired assets/entry points: `plugins/click-sdd/`, OpenClaw `click-sdd*` skill assets, `internal/installer/openclawskills.go`, `internal/agentbuilder/writer.go` (+tests). Route: delegated.
- [ ] T10 — Doctor and rollback ownership for gentle-ai-managed surfaces: `internal/doctor/*`, `internal/installer/{snapshot.go,snapshot_projection.go}`, `internal/cli/rollback_test.go`. Tests: partial install, drift-safe restore. Route: delegated.
- [ ] T11 — Refresh live direction docs, root `CLAUDE.md`/`AGENTS.md`/`README.md`; mark historical reports superseded; full `go test ./...`, `go build ./...`, isolated Windows package smoke. Route: delegated.

## Acceptance criteria
- `click install` on a clean isolated home installs the pinned gentle-ai (checksum-verified) and the Click layer; no click-sdd artifacts remain.
- Existing installs with click-sdd are migrated non-destructively; foreign settings/hooks/sections are preserved.
- Memory guard keeps covering every persisting Engram tool alongside gentle-ai hooks.
- `click doctor` reports gentle-ai pin drift and Click-layer health; rollback restores Click-owned surfaces only.
- `go build ./...`, `go vet ./...`, `go test ./...` pass.

## Delivery
Strategy: ask-on-risk. Forecast: ~3000+ authored changed lines (plus the click-sdd asset deletion). Chain strategy: feature-branch-chain (user, 2026-10-08) — tracker branch `feat/gentle-ai-distribution` accumulates integration; slice PRs target the previous slice branch; only the tracker merges to main.
Slices: slice 1 = 9f894b6..0aa7f56 (T01+T02, reviewed); slice 2 = b3f53e6 (T03, reviewed); slice 3 = b3f53e6..1e3848a (T03a plus tracking, 235 authored changed lines, reviewed). Further slices recorded as they land. These are local review boundaries; no PR or remote slice branch has been created in this resumed session.
Base: `fix/memory-guard-mutation-coverage` @ 9c6825f (unpushed). Branch: `feat/gentle-ai-distribution`.

## Progress / evidence
- 2026-10-08: exploration by Sol (read-only), parent spot check confirmed `internal/installer/engram.go:523-525` accepts any resolvable Engram binary without version check.

- 2026-10-08 T01: Sol deleted the untracked `.engram/config.json` as "task-generated metadata" without reporting it; parent recreated it. Writer prompts must list it as do-not-touch.

- 2026-10-08 T02 (Sol): partial in sandbox; outside sandbox `go build ./...` OK, `go vet ./...` OK, `go test ./...` only failure is `TestDispatch_SubcommandRuntimeFailure_PrintsErrorExactlyOnceNoUsageDump` (internal/cli), which also fails on base 9f894b6 → pre-existing environmental failure (the test reads real `~/.codex/AGENTS.md` and the ambient `openclaw` binary). ~926 authored lines (platform-specific safety + failure-path tests). Astra review done (see T02).

## Review (RDD) — slice 1 (T01+T02)
- Assessment: high (`high_risk`, process execution in `internal/installer/gentleai.go`), base 9c6825f, commits 9f894b6..0aa7f56, 10 files / 1399 lines. Consent: granted by user.
- Native 4-lens review, lineage `review-e8fccfe78dfdb011`: approved; acknowledged, authority burned (consumed revision sha256:cee92f47...). Reviewed boundary is now 0aa7f56.
- Non-blocking findings (separate later work, never a reason to re-review this candidate):
  - WARNING R2-seam-doc-misleading `gentleai.go:350-354`; R4-go-install-stderr-dropped `gentleai.go:218-219` (go install stderr lost on failure); R4-windows-rename-running-exe `gentleai.go:344` (rename over a running gentle-ai.exe fails on Windows).
  - SUGGESTION R2-go-min-literal `gentleai.go:211-216`; R2-shadow-version-pkg `gentleai.go:267-268`; R2-task-tracker-files `odd/tasks/gentle-ai-distribution.md:47`; R3-go-version-field-count `gentleai.go:243-249`; R3-oversize-download-untested `gentleai.go:126-129`; R3-probe-format-unqualified `gentleai.go:263-266`.
- Hook-suggested START over the accumulated branch (base e845227) was not run: ODD reviews slices from the last reviewed boundary, and this slice already had a bound transaction.

## Review (RDD) — slice 2 (T03)
- Assessment: high (`high_risk`: update path, process execution in `gentleai_install.go`), base 0aa7f56, commit b3f53e6, 13 files / 949 lines. Consent: granted by user.
- Native 4-lens review, lineage `review-aa66951080f9e88a`: approved; acknowledged, authority burned (consumed revision sha256:db1d3ceb...). Reviewed boundary is now b3f53e6.
- Non-blocking findings:
  - Accepted into T03a (behavior regression introduced by T03): WARNING R1-update-selection-widened-by-home / R3-update-selection-overrides-persisted-intent / R2-update-selection-comment-contradicts-code `internal/cli/update.go:58-61` (`selection.Codex = cfg.CodexHome != ""` re-enables a target the user disabled); WARNING R4-gentleai-failure-cause-discarded `internal/installer/gentleai_install.go:84-86`.
  - Later work: WARNING R2-dry-run-encoded-as-decline `internal/cli/install.go:144-146`; SUGGESTION R1-pin-match-trusts-self-reported-version `gentleai_install.go:63-69`, R2-dry-run-flag-literal `install.go:45`, R2-garbled-test-doc-comment `preview_test.go:131-132`, R2-test-message-index-mismatch `preview_test.go:104`.

## Follow-ups (outside current scope)
- Test isolation leak: `TestDispatch_SubcommandRuntimeFailure_PrintsErrorExactlyOnceNoUsageDump` runs doctor against the real `~/.codex/AGENTS.md` and ambient `openclaw`, violating the isolated-home rule; it fails on any machine with OpenClaw installed.

## Next step
T04 — transfer Engram/Context7 ownership to gentle-ai while preserving the Click Engram Cloud layer. T03a is closed locally; push destination/credential confirmation remains pending.

## Resumed T03a verification (2026-10-09)
- Writer/verifier: GPT-6 Luna high. Existing four-file implementation preserved; no additional behavior changes needed. Normalized with gofmt before verification.
- Parent review: inspected all four file diffs plus the update/resolver integration. No blocking findings. The resolver already filters target availability, so explicit AND strengthens the invariant rather than proving a pre-existing end-to-end reactivation bug.
- PASS: `go test ./internal/installer ./internal/cli -run 'TestGentleAI|TestUpdate' -count=1`; `go build ./...`; `go vet ./...`; `git diff --check`.
- Parent spot check PASS: `go test ./internal/installer ./internal/cli -run 'TestGentleAIInstall_FailuresPreserveSanitizedCause|TestUpdate_PersistedIntentNarrowsTargetsWithoutRewritingSelection' -count=1`.
- FAIL: `go test ./internal/installer ./internal/cli -count=1`, reproducing the four previously recorded environmental failures: `TestResolveGitExecutionContext_NonRepositorySelectorCreatesRecoveryRepoAndCleansItOnFailure`, `TestExecCommandRunner_CommandDirInitializationFailureIsClear`, `TestExecCommandRunner_NonRepositoryUsesInitializedIsolatedRepo`, `TestDispatch_SubcommandRuntimeFailure_PrintsErrorExactlyOnceNoUsageDump`. Full-suite green is NOT claimed.
- Runtime harness: mocked upstream runner and binary lookup, temporary target homes, exact selected-agent argv, dry-run non-execution, and unchanged persisted target bytes. No real installs. Windows execution not performed in this Linux session.
- Rollback boundary: the T03a changes in `internal/cli/update.go`, `internal/cli/update_test.go`, `internal/installer/gentleai_install.go`, `internal/installer/gentleai_install_test.go` only.
- Native assessment: high (`hot_path`, `process_boundary`), consent granted. Commit `1e3848a`; reviewed range `b3f53e6..1e3848a`, five files / 235 authored changed lines.
- Native four-lens review `review-338c091ed4a54094`: all findings lists empty; approved, exact acknowledgement succeeded, authority burned at revision `sha256:341d14f90ca051113ec1daef64b084c81fed4d1da71d1319e362d3990403ec49`. New reviewed boundary: `1e3848a`. Initial reviewer startup was blocked by read-only sandbox state; after native STATUS reoffered the same slots, the user-approved elevated retry completed. No candidate correction required.
- No push, PR, merge, release, or Windows execution performed.
