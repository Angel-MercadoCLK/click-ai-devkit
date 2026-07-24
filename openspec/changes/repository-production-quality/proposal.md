# Proposal: Repository Production Quality

## Intent

Make `click` releasable by turning the partially remediated installer into a truthful, target-first, recoverable flow. A confirmation must describe every mutation, its order, and its rollback scope—across Claude, Codex, and OpenClaw—while Windows release evidence proves the shipped binary works.

## Scope

### In Scope
- A data-driven install state machine: one plan derives preview, confirmation, execution, snapshots, rollback, progress, and doctor expectations.
- Explicit target contracts: Claude-only integrations; Codex-native config with parser-aware, atomic TOML edits; OpenClaw-native actions only after real CLI contract qualification. Noninteractive native mutation requires an explicit target-model flag.
- Click-owned neutral target-selection state; reconcile install, update, configure-targets, uninstall, and rollback for Claude-free selections.
- Windows Git cwd/recovery, stale cache/Engram diagnostic-to-repair qualification, grouped target-aware menu with exact `Plugins` label, current docs, and CI/release evidence.

### Out of Scope
- Support for runtimes other than Claude Code, Codex, and OpenClaw.
- New model providers, broad runtime abstraction, cloud service redesign, or automatic repair beyond qualified installer/update paths.
- Removing historical documentation; retain it with dated/historical framing while publishing current behavior.

## Capabilities

### New Capabilities
- `target-first-install-orchestration`: Single-source plan, target lifecycle state, preview, mutation, rollback, and recovery.
- `native-runtime-configuration`: Safe, explicit Codex and qualified OpenClaw native configuration contracts.
- `release-qualification`: Windows runtime, package, update, documentation, and release-gate evidence.

### Modified Capabilities
None; `openspec/specs/` has no existing capability specs.

## Approach

Model each selected target as declarative steps with prerequisites, writes, snapshots, rollback, and diagnostic checks. Execute that plan only after final confirmation. Keep Claude integrations Claude-owned; use a TOML-aware read/modify/write transaction for Codex; block OpenClaw mutation until its installed CLI keys and command shape are demonstrated.

## Acceptance Criteria

- [ ] Preview, spinner steps, execution order, snapshots, rollback, and doctor checks derive from the same selected plan.
- [ ] `--yes` never changes a native model without its explicit model flag; Codex preserves table-scoped keys, comments where supported, and recoverable original content.
- [ ] Claude-free selected targets persist and survive update/configure/uninstall; Claude preflight/home is required only for Claude.
- [ ] Windows qualification covers non-repo cwd, Git recovery/failure cleanup, stale cache → update → restarted-session recovery, and supported target contracts.
- [ ] Menu has coherent groups, target availability, and exact `Plugins`; current docs/version/release metadata agree while historical docs remain identifiable.
- [ ] CI/release validates packaged Windows behavior and supported update/release metadata before publication.

## Affected Areas

| Area | Impact | Description |
|---|---|---|
| `internal/cli/{install,installplan,preview,update,configuretargets}.go` | Modified | Plan-driven lifecycle |
| `internal/installer/{targets,codexmodel,openclawmodel,plugins}.go` | Modified | State, safe mutation, Windows recovery |
| `internal/doctor`, `internal/menu` | Modified | Diagnostics and grouped UX |
| `README.md`, `documentacion/`, `.github/workflows/`, `.goreleaser.yaml` | Modified | Truth and release proof |

## Risks and Rollback

| Risk | Mitigation |
|---|---|
| Incorrect native write | Atomic backups; plan-scoped rollback; explicit flags |
| Unproven OpenClaw CLI | No mutation/release claim until real-contract evidence |
| Windows environment variance | Required Windows recovery matrix |

Rollback restores each plan snapshot, clears only Click-owned neutral state, and reverts the release artifact; never delete user-managed config not captured by the plan.

## Delivery / Review Forecast

High risk; forecast four independently verifiable slices: plan/state, native contracts, diagnostics/menu, docs/release. Expected authored change exceeds the 800-line budget, so auto-forecast recommends chained PRs.

## Open Decisions

None. OpenClaw CLI command/key verification is a release prerequisite, not permission to assume a contract.
