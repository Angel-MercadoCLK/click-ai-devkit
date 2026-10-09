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
Writer: Codex `gpt-6.1-sol` high. Per-task reviewer: Codex `gpt-6-astra` xhigh. Final reviewer: Claude Opus xhigh.

## Defaults taken (recommended by exploration; user may override)
- Pin gentle-ai **v4.0.0 stable**, exact release artifact + SHA-256, never "latest"; suppress upstream self-update (`GENTLE_AI_NO_SELF_UPDATE=1`).
- Click provisions the checksum-qualified upstream binary itself; Scoop still installs only Click (`bucket/click.json` has no dependency today).
- `click-review` keeps insurance/business checklist content only, not an independent review engine.

## Decided by user
- 2026-10-08 chain strategy: feature-branch-chain.
- 2026-10-08 Engram project: commit `.engram/config.json` (`project_name: click-ai-devkit`) so the team shares one Engram project; lands with T01.
- 2026-10-08 Windows provisioning: gentle-ai publishes no Windows binary (v3.5–v4.0.0 ship only darwin/linux amd64/arm64 tar.gz + minisign-signed `checksums.txt`; README documents Windows as `go install`). Windows uses `go install github.com/gentleman-programming/gentle-ai/v4/cmd/gentle-ai@v4.0.0` (integrity via Go checksum DB, requires Go >= 1.25.10, same pattern click uses for Engram); macOS/Linux use the official release tar.gz verified against the SHA-256 pinned in click. D48 must be amended accordingly (T02).
- 2026-10-08 pin drift: warn and re-pin — `click doctor` reports an installed gentle-ai that differs from the pin as drift; `click install`/`click update` reinstall the qualified version and say so explicitly (may downgrade a dev-chosen newer version).

## Open product decisions
- Engram Cloud enrollment/token/autosync retention under D40–D47 once gentle-ai owns Engram lifecycle (blocks T04): pending.

## Tasks
- [x] T01 — Record distribution/ownership decision; supersede D1/D8/D9/D12/D25, revisit D36/D38. Files: `documentacion/00-decisions-and-open-questions.md`, `documentacion/vision.md`. Route: inline-eligible docs; delegated to Sol per model-team instruction. Evidence: D48 added; Astra xhigh review CHANGES_REQUIRED (1 blocker: pin drift left open; 2 warnings: chain strategy missing, historical vision bullets rewritten) → all fixed by Sol; parent spot check of D48 text and restored bullets. Docs only, no Go tests.
- [x] T02 — Qualified upstream pin: `GENTLE_AI_VERSION`, `internal/installer/gentleai.go` (+test), `internal/manifest/{manifest.go,manifest.yaml,release_qualification_test.go}`, `.github/workflows/release.yml`. Tests: checksum/version rejection, self-update suppression. Route: delegated (writer trigger). Evidence: Astra xhigh CHANGES_REQUIRED (2 blockers: saved Go config/GOFLAGS bypass of checksum DB and build — parent reproduced with GOENV file; 4 warnings: toolchain gate order, HTTPS redirect downgrade, RC version parsing, decompression budget) → all fixed by Sol with RED→GREEN tests; controlled Go env GOENV=off, GOFLAGS cleared, GOSUMDB=sum.golang.org, GONOSUMDB/GOPRIVATE/GONOPROXY=none, GOPROXY=https://proxy.golang.org, GOTOOLCHAIN=local. Outside sandbox: build/vet OK, `go test ./...` only known env failure. CI release.yml not touched (deferred).
- [ ] T03 — Upstream install selection/orchestration: `internal/installer/{plan.go,installer.go}`, `internal/cli/{install.go,update.go}` (+tests). Tests: exact argv, selected-agent scope, failures, idempotency. Route: delegated.
- [ ] T04 — Transfer Engram/Context7 ownership to gentle-ai: `internal/installer/{engram.go,context7.go,codexmcp.go,openclawmcp.go}` (+tests). Tests: pre-existing installs, non-destructive uninstall. Route: delegated. Blocked on Engram Cloud decision.
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
Slices: slice 1 = 9f894b6..0aa7f56 (T01+T02, reviewed). Further slices recorded as they land.
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

## Follow-ups (outside current scope)
- Test isolation leak: `TestDispatch_SubcommandRuntimeFailure_PrintsErrorExactlyOnceNoUsageDump` runs doctor against the real `~/.codex/AGENTS.md` and ambient `openclaw`, violating the isolated-home rule; it fails on any machine with OpenClaw installed.

## Next step
T03 — wire gentle-ai provisioning into `click install`/`click update`.
