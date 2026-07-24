# Tasks: Repository Production Quality

## Review Workload Forecast

| Field | Value |
|---|---|
| Estimated changed lines | 1,000–1,400 lines |
| 800-line budget risk | High |
| Chained PRs recommended | Yes |
| Delivery strategy | auto-forecast |
| Suggested split | PR1 plan/state; PR2 native contracts; PR3 diagnostics/menu; PR4 docs/release |
| Chain strategy | stacked-to-main |

Decision needed before apply: No
Chained PRs recommended: Yes
800-line budget risk: High

### Suggested Work Units

| Unit | Autonomous boundary | Focused test command | Runtime harness | Rollback point |
|---|---|---|---|---|
| PR1 | `internal/installer/plan*`, neutral state, lifecycle CLI | `go test ./internal/installer ./internal/cli -count=1` | fake config home | revert plan/state and lifecycle wiring |
| PR2 | Codex/OpenClaw adapters and qualification | `go test ./internal/installer -run 'Codex|OpenClaw' -count=1` | fake binaries; recorded real OpenClaw probe | revert native adapters/config writers |
| PR3 | Git recovery, doctor, grouped menu | `go test ./internal/doctor ./internal/menu ./internal/installer -count=1` | Windows non-repo cwd and stale-cache fixture | revert diagnostics/menu/recovery changes |
| PR4 | docs, CI, GoReleaser and release gate | `go test ./... -count=1; go vet ./...` | packaged Windows `click.exe` smoke matrix | revert workflow/docs/release metadata |

## Phase 1: Plan and Neutral Lifecycle (PR1)

- [x] 1.1 **RED** `internal/installer/plan_test.go`, `internal/cli/commands_test.go`: assert target-first selection, Claude-free lifecycle, plan projections, and no pre-confirmation mutation; **GREEN** implement immutable `TargetPlan`, typed `Step`, journal, projections, and injected neutral state in `plan.go`, `config.go`, `targets.go`; **evidence** lifecycle order passes.
- [x] 1.2 **RED** `internal/cli/uninstall_test.go`: assert selected teardown, journal, reverse rollback, last-state cleanup, and untouched files; **GREEN** update `install.go`, `update.go`, `configuretargets.go`, `uninstall.go`, `rollback.go`, `snapshot.go`; **evidence** failure restores captured snapshots only.

## Phase 2: Native Contracts (PR2)

- [x] 2.1 **RED** `internal/installer/codexmodel_test.go`: assert omitted `--yes` model makes no write, root `model` avoids table collision, data survives, parse failure is atomic, and rollback works; **GREEN** parser-aware atomic writer in `codexmodel.go`; **evidence** config assertions pass.
- [x] 2.2 **RED** `internal/installer/openclawmodel_test.go`: assert failed qualification, timeout, unexpected output, or missing binary causes no write; **GREEN** allowlisted adapter/probe with absolute binary, argv, cwd, timeout, and evidence; **evidence** only qualified contract enables action/claim.

## Phase 3: Windows Diagnostics and UX (PR3)

- [x] 3.1 **RED** `internal/installer/gitpreflight_test.go`: cover relative, absolute, non-repo, and unsafe cwd selectors; assert clean temp recovery, unchanged repositories, and actionable failure; **GREEN** safe absolute cwd/recovery in `gitpreflight.go`/`plugins.go`; **evidence** Windows matrix passes.
- [x] 3.2 **RED** doctor/menu tests: stale cache vs missing asset, update/restart evidence, unavailable native action, groups, exact `Plugins`; **GREEN** `internal/doctor/checks.go`, `internal/menu/menu.go`, `internal/cli/rootdefault.go`; **evidence** menu/doctor match plan.

## Phase 4: Release and Documentation (PR4)

- [x] 4.1 **RED** CI/release checks: fail publication for Windows package smoke, target contract, docs/version/update mismatch; **GREEN** update `.github/workflows/{ci,release}.yml` and `.goreleaser.yaml`; **evidence** packaged `click.exe` matrix gates publication.
- [x] 4.2 **RED** documentation acceptance checks for current claims and historical labels; **GREEN** update `README.md`, `documentacion/codex-target.md`, and `documentacion/portability-runbook.md`; **evidence** documented recovery/contracts match shipped behavior.

## Final Verification

- [ ] 5.1 Run `gofmt -l .`, `go vet ./...`, `go build ./...`, and `go test ./... -count=1`; attach Windows packaged smoke, real OpenClaw probe, stale-cache restart, and release-metadata evidence.
