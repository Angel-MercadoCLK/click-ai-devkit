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
- 2026-10-08 pin drift: warn and re-pin — `click doctor` reports an installed gentle-ai that differs from the pin as drift; `click install`/`click update` reinstall the qualified version and say so explicitly (may downgrade a dev-chosen newer version).

## Open product decisions
- Engram Cloud enrollment/token/autosync retention under D40–D47 once gentle-ai owns Engram lifecycle (blocks T04): pending.

## Tasks
- [x] T01 — Record distribution/ownership decision; supersede D1/D8/D9/D12/D25, revisit D36/D38. Files: `documentacion/00-decisions-and-open-questions.md`, `documentacion/vision.md`. Route: inline-eligible docs; delegated to Sol per model-team instruction. Evidence: D48 added; Astra xhigh review CHANGES_REQUIRED (1 blocker: pin drift left open; 2 warnings: chain strategy missing, historical vision bullets rewritten) → all fixed by Sol; parent spot check of D48 text and restored bullets. Docs only, no Go tests.
- [ ] T02 — Qualified upstream pin: `GENTLE_AI_VERSION`, `internal/installer/gentleai.go` (+test), `internal/manifest/{manifest.go,manifest.yaml,release_qualification_test.go}`, `.github/workflows/release.yml`. Tests: checksum/version rejection, self-update suppression. Route: delegated (writer trigger).
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
Slices: to be recorded as work-unit commits land.
Base: `fix/memory-guard-mutation-coverage` @ 9c6825f (unpushed). Branch: `feat/gentle-ai-distribution`.

## Progress / evidence
- 2026-10-08: exploration by Sol (read-only), parent spot check confirmed `internal/installer/engram.go:523-525` accepts any resolvable Engram binary without version check.

- 2026-10-08 T01: Sol deleted the untracked `.engram/config.json` as "task-generated metadata" without reporting it; parent recreated it. Writer prompts must list it as do-not-touch.

## Next step
T02 — qualified gentle-ai pin.
