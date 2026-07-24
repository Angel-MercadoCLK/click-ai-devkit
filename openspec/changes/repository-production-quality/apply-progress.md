# Apply Progress: Repository Production Quality

## Result Contract

- status: blocked
- executive_summary: Completed a strict-TDD authoritative-plan corrective batch: `TargetPlan` now exposes typed lifecycle action/check kinds, install/update/preview consume that authoritative order, uninstall snapshots and tears down from the same plan, doctor resolves target-aware checks from the plan, and `configure-targets` reports the saved plan summary. Focused tests plus `go test ./... -count=1`, `go vet ./...`, `go build ./...`, and `gofmt -l .` passed. Final verification still remains blocked on external evidence: a real installed-OpenClaw portability receipt, authoritative review-lineage selection, and the external restarted-session follow-up.
- artifacts:
  - openspec: `C:\Proyectos\click-ai-devkit\openspec\changes\repository-production-quality\apply-progress.md`
  - openspec: `C:\Proyectos\click-ai-devkit\openspec\changes\repository-production-quality\tasks.md`
  - engram_topic: `sdd/repository-production-quality/apply-progress`
  - engram_topic: `sdd/repository-production-quality/tasks`
- next_recommended: `capture the external OpenClaw/restarted-session evidence, select the existing authoritative review lineage, then rerun sdd-verify`
- risks:
  - OpenClaw is not installed or executable in the verification environment (`Get-Command openclaw` returned `OPENCLAW_NOT_FOUND`), so the documented real workflow was not executed and no receipt was fabricated.
  - `gentle-ai review validate --gate post-apply --cwd C:\Proyectos\click-ai-devkit` still returns `multiple compact facade review lineages found; specify --lineage`; the maintainer must select an existing authoritative lineage without creating a new review budget.
  - External verification still needs a real restarted-session follow-up; this batch deliberately did not fabricate that runtime evidence.
- skill_resolution: `paths-injected — sdd-apply, _shared, chained-pr, and work-unit-commits were loaded from the injected paths; strict-tdd.md was loaded because strict TDD is active.`

## Mode

- Strict TDD
- Delivery strategy: auto-chain
- Chain strategy: stacked-to-main
- Current slice: PR5 authoritative TargetPlan corrective batch

## Completed Tasks

- [x] 1.1 Target-first selection, neutral state injection, and immutable plan projections
- [x] 1.2 Selected uninstall teardown, rollback restoration, and last-state cleanup
- [x] 2.1 Codex native contract writer
- [x] 2.2 OpenClaw qualification contract
- [x] 3.1 Git-safe recovery for marketplace subprocesses
- [x] 3.2 Doctor/menu alignment for grouped availability-aware UX
- [x] 4.1 CI/release gate
- [x] 4.2 Documentation alignment

## TDD Cycle Evidence

| Task | Test File | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|---|---|---|---|---|---|---|---|
| 1.1 | `internal/installer/plan_test.go`, `internal/cli/commands_test.go` | Unit | ✅ `go test ./internal/installer ./internal/cli -count=1` → installer ok, cli ok | ✅ Added failing plan/state tests for neutral state home and pre-confirm persistence | ✅ Same focused command passed after `TargetPlan`, `ClickStateHome`, and deferred selection persistence | ✅ Multi-target and Codex-only projections plus neutral-state path coverage | ✅ Reused typed plan builder and neutral-state resolver across CLI entry points |
| 1.2 | `internal/cli/uninstall_test.go` | Integration | ✅ `go test ./internal/installer ./internal/cli -count=1` → installer ok, cli ok | ✅ Added failing uninstall tests for Codex-only teardown and rollback restoration | ✅ Same focused command passed after selected teardown, rollback, and state-last cleanup wiring | ✅ Success + rollback cases cover different code paths and untouched-file behavior | ✅ Simplified uninstall execution around one planned rollback boundary |
| 2.1 | `internal/installer/codexmodel_test.go` | Unit | ✅ `go test ./internal/installer -run 'Codex|OpenClaw' -count=1` → installer ok | ✅ Added failing tests for explicit-model guidance, root-only `model` updates, and atomic parse-failure handling | ✅ Same focused command passed after parse-validated atomic root-key updates in `codexmodel.go` | ✅ Happy-path scoped update plus empty-model and invalid-TOML paths cover different outcomes | ✅ Extracted config parsing and TOML key/value helpers to keep the writer narrow and deterministic |
| 2.2 | `internal/installer/openclawmodel_test.go` | Unit | ✅ `go test ./internal/installer -run 'Codex|OpenClaw' -count=1` → installer ok | ✅ Added failing tests for qualification probe failure, timeout, unexpected output, missing binary, and absolute-binary writes only after qualification | ✅ Same focused command passed after the qualified OpenClaw contract gated all native writes | ✅ Primary-only, fallback, missing-binary, failed-probe, timeout, unexpected-output, and wrapped-command-error cases force multiple paths | ✅ Centralized the allowlisted probe and qualified-contract builder so every write uses one validated contract |
| 3.1 | `internal/installer/gitpreflight_test.go`, `internal/installer/plugins_runner_test.go` | Integration | ✅ `go test ./internal/doctor ./internal/menu ./internal/cli ./internal/installer -count=1` → doctor ok, menu ok, cli ok, installer ok | ✅ Added failing cwd-selector tests for relative, absolute, non-repo, and unsafe paths plus recovery cleanup expectations | ✅ Same focused command passed after `resolveGitExecutionContext()` and exec runner cleanup wired safe absolute repositories into marketplace subprocesses | ✅ Relative and absolute repository selectors, recovered non-repo temp repositories, and unsafe file selectors force distinct recovery paths | ✅ Moved git working-directory resolution behind one cleanup-aware helper reused by marketplace subprocess execution |
| 3.2 | `internal/doctor/checks_test.go`, `internal/menu/menu_test.go`, `internal/cli/rootdefault_test.go` | Integration | ✅ `go test ./internal/doctor ./internal/menu ./internal/cli ./internal/installer -count=1` → doctor ok, menu ok, cli ok, installer ok | ✅ Added failing doctor/menu tests for grouped sections, exact `Plugins`, unavailable OpenClaw native action guidance, and update/restart evidence | ✅ Same focused command passed after doctor added the shared OpenClaw native action status and root default injected grouped availability-aware menu items | ✅ Group headings, unavailable-row rendering, enter-on-disabled guidance, and root-menu availability injection cover distinct UI/diagnostic paths | ✅ Kept the default menu dataset reusable while letting root default supply runtime-specific availability without breaking direct command dispatch |
| 4.1 | `internal/manifest/release_qualification_test.go` | Integration | ✅ `go test ./internal/manifest -count=1` → manifest ok | ✅ Added failing workflow/release tests for `go vet`, Windows packaged smoke, GoReleaser gating, and docs/version/update checks before editing workflows or docs | ✅ Same focused command passed after `.github/workflows/{ci,release}.yml`, `.goreleaser.yaml`, and `scripts/windows-package-smoke.ps1` gated publication on repository validation plus packaged `click.exe` smoke | ✅ Separate CI, release, GoReleaser-hook, and documentation contract assertions force distinct release-qualification paths instead of one brittle check | ✅ Extracted reusable YAML/text assertion helpers so release-qualification coverage stays focused on contract drift |
| 4.2 | `internal/manifest/release_qualification_test.go` | Integration | ✅ `go test ./internal/manifest -count=1` → manifest ok | ✅ Added failing documentation contract assertions for README, Codex guidance, and OpenClaw release-evidence wording before touching docs | ✅ Same focused command passed after `README.md`, `documentacion/codex-target.md`, and `documentacion/portability-runbook.md` were rewritten to match explicit native-model boundaries and release evidence language | ✅ README metadata guidance, Codex root-model scope, and OpenClaw probe-vs-real-evidence wording cover different documentation drift cases | ✅ Kept documentation checks in one repo-contract suite so release metadata and docs evolve together |
| 5.1 corrective batch | `internal/cli/commands_test.go`, `internal/installer/plan_test.go` | Integration | ✅ `go test ./internal/installer ./internal/cli ./internal/doctor ./internal/menu -count=1` → installer ok, cli ok, doctor ok, menu ok | ✅ Added failing tests for Codex-only noninteractive omission, Codex `config.toml` rollback, stale cache → update → fresh-doctor recovery, and plan-backed Codex snapshot paths before touching production | ✅ `go test ./internal/installer ./internal/cli -run 'TestBuildTargetPlan_SnapshotPathsIncludeCodexConfigForNativeMutation|TestInstallCommand_CodexOnly_NonInteractiveOmitsNativeModelWithoutFlagAndDoesNotRequireClaude|TestUpdateCommand_CodexOnly_UsesPersistedSelectionWithoutClaudeAndOmitsNativeModelWithoutFlag|TestUpdateCommand_CodexNativeMutationFailure_RollsBackConfigToml|TestDoctorCommand_StaleCacheThenUpdateThenFreshDoctorReportsHealthy' -count=1` → installer ok, cli ok | ✅ The new cases cover Codex-only install, Codex-only update, rollback after a post-mutation failure, stale cache recovery after a fresh command restart, and snapshot-path projection instead of data-only ghost loops | ✅ Removed the duplicate `BuildInstallPlan` test-only surface, reused `TargetPlan` for preview/snapshot wiring, and kept `rootdefault.go` gofmt-clean |
| 5.1 authoritative-plan batch | `internal/installer/plan_test.go`, `internal/cli/commands_test.go` | Integration | ✅ `go test ./internal/cli ./internal/installer ./internal/doctor -count=1` → cli ok, installer ok, doctor ok | ✅ Added failing tests for plan-exposed install/update/uninstall/doctor lifecycle kinds and Codex-only doctor health without Claude before touching production | ✅ `go test ./internal/installer ./internal/cli -run 'TestBuildTargetPlan_CodexOnlyExposesLifecycleActionsForProductionCommands|TestDoctorCommand_CodexOnlySelection_DoesNotRequireClaudeAndUsesPlanChecks|TestUninstallCommand_CodexOnlySelection_DoesNotRequireClaudeAndRemovesNeutralStateLast|TestUpdateCommand_CodexOnly_UsesPersistedSelectionWithoutClaudeAndOmitsNativeModelWithoutFlag' -count=1` → installer ok, cli ok | ✅ The new plan row plus the existing Codex-only update/uninstall regressions force distinct production paths for preview/install/update/uninstall/doctor from one authoritative plan | ✅ Extracted typed lifecycle action/check enums and ordered plan projections so production command wiring no longer depends on duplicate hard-coded lifecycle lists |

## Work Unit Evidence

### PR2 Native Contracts

| Evidence | Required value |
|---|---|
| Focused test command and exact result | `go test ./internal/installer -run 'Codex|OpenClaw' -count=1` → `ok github.com/Angel-MercadoCLK/click-ai-devkit/internal/installer` |
| Runtime harness command/scenario and exact result | `go test ./internal/cli -run 'OpenClaw' -count=1` → `ok github.com/Angel-MercadoCLK/click-ai-devkit/internal/cli`; scenario: non-interactive install/update with fake OpenClaw binary exercises qualified native model writes plus OpenClaw workspace/MCP/plugin sync without touching a real runtime |
| Rollback boundary | Revert `internal/installer/{codexmodel,openclawmodel,plugins}.go`, `internal/installer/{codexmodel,openclawmodel}_test.go`, and the fake-runner probe fixture in `internal/cli/commands_test.go` to remove PR2 native-contract behavior without touching PR1 lifecycle work or later diagnostics/docs slices |

### PR3 Diagnostics and Menu Alignment

| Evidence | Required value |
|---|---|
| Focused test command and exact result | `go test ./internal/doctor ./internal/menu ./internal/cli ./internal/installer -count=1` → `ok github.com/Angel-MercadoCLK/click-ai-devkit/internal/installer` / `ok github.com/Angel-MercadoCLK/click-ai-devkit/internal/doctor` / `ok github.com/Angel-MercadoCLK/click-ai-devkit/internal/menu` / `ok github.com/Angel-MercadoCLK/click-ai-devkit/internal/cli` |
| Runtime harness command/scenario and exact result | `go test ./internal/cli -run "TestRootMenuItems_DisablesUnavailableOpenClawNativeActionWithGuidance|TestRunMenuLoop_ConfigureOpenClawModelDispatchDoesNotCrashMenu" -count=1` → `ok github.com/Angel-MercadoCLK/click-ai-devkit/internal/cli`; scenario: the standing menu keeps the OpenClaw native model action non-dispatchable with guidance when qualification is unavailable and still survives the real `configure-openclaw-model` no-args dispatch path |
| Rollback boundary | Revert the PR3 deltas in `internal/installer/{gitpreflight,plugins,openclawmodel}.go`, `internal/doctor/checks.go`, `internal/menu/menu.go`, `internal/cli/rootdefault.go`, and their paired tests to remove diagnostics/menu/recovery behavior without touching PR1 lifecycle work or PR2 native contract writers |

### PR4 Release and Documentation Gate

| Evidence | Required value |
|---|---|
| Focused test command and exact result | `go test ./internal/manifest -count=1` → `ok github.com/Angel-MercadoCLK/click-ai-devkit/internal/manifest` |
| Runtime harness command/scenario and exact result | `go build -ldflags "-X github.com/Angel-MercadoCLK/click-ai-devkit/internal/version.Version=0.5.3 -X github.com/Angel-MercadoCLK/click-ai-devkit/internal/version.Commit=pr4-smoke" -o dist\pr4-smoke\click.exe ./cmd/click; .\scripts\windows-package-smoke.ps1 -DistDir dist\pr4-smoke -ExpectedVersion 0.5.3` → success (exit 0); scenario: package the built `click.exe`, extract it, then smoke `--version`, `targets`, OpenClaw native configuration, and install/update behavior against isolated fake Claude/Git/OpenClaw/Codex runtimes |
| Rollback boundary | Revert `.github/workflows/{ci,release}.yml`, `.goreleaser.yaml`, `scripts/windows-package-smoke.ps1`, `internal/manifest/release_qualification_test.go`, and the PR4 README/Codex/OpenClaw docs to remove release/doc gating without touching PR1–PR3 lifecycle, native-contract, or diagnostics work |

### PR5 Corrective Verification Blockers

| Evidence | Required value |
|---|---|
| Focused test command and exact result | `go test ./internal/installer ./internal/cli -run 'TestBuildTargetPlan_SnapshotPathsIncludeCodexConfigForNativeMutation|TestInstallCommand_CodexOnly_NonInteractiveOmitsNativeModelWithoutFlagAndDoesNotRequireClaude|TestUpdateCommand_CodexOnly_UsesPersistedSelectionWithoutClaudeAndOmitsNativeModelWithoutFlag|TestUpdateCommand_CodexNativeMutationFailure_RollsBackConfigToml|TestDoctorCommand_StaleCacheThenUpdateThenFreshDoctorReportsHealthy' -count=1` → `ok github.com/Angel-MercadoCLK/click-ai-devkit/internal/installer` / `ok github.com/Angel-MercadoCLK/click-ai-devkit/internal/cli` |
| Runtime harness command/scenario and exact result | `go test ./internal/cli -run 'TestUpdateCommand_CodexOnly_UsesPersistedSelectionWithoutClaudeAndOmitsNativeModelWithoutFlag|TestUpdateCommand_CodexNativeMutationFailure_RollsBackConfigToml|TestDoctorCommand_StaleCacheThenUpdateThenFreshDoctorReportsHealthy' -count=1` → `ok github.com/Angel-MercadoCLK/click-ai-devkit/internal/cli`; scenario: execute real Cobra install/update/doctor flows against fake Codex/Claude/OpenClaw binaries, prove Codex-only update no longer drives Claude marketplace work, prove a failed post-mutation step rolls Codex `config.toml` back from the run snapshot, and prove a stale cache diagnosis becomes healthy after `click update` plus a fresh doctor command |
| Rollback boundary | Revert `internal/cli/{install,update,configuretargets,preview}.go`, `internal/installer/{config,plan,snapshot}.go`, and the new CLI/installer regression tests to remove this corrective batch without touching the earlier release/doc gating slice |

### PR5 Authoritative TargetPlan Batch

| Evidence | Required value |
|---|---|
| Focused test command and exact result | `go test ./internal/installer ./internal/cli -run 'TestBuildTargetPlan_CodexOnlyExposesLifecycleActionsForProductionCommands|TestDoctorCommand_CodexOnlySelection_DoesNotRequireClaudeAndUsesPlanChecks|TestUninstallCommand_CodexOnlySelection_DoesNotRequireClaudeAndRemovesNeutralStateLast|TestUpdateCommand_CodexOnly_UsesPersistedSelectionWithoutClaudeAndOmitsNativeModelWithoutFlag' -count=1` → `ok github.com/Angel-MercadoCLK/click-ai-devkit/internal/installer` / `ok github.com/Angel-MercadoCLK/click-ai-devkit/internal/cli` |
| Runtime harness command/scenario and exact result | `go test ./internal/cli -run 'TestDoctorCommand_CodexOnlySelection_DoesNotRequireClaudeAndUsesPlanChecks|TestUninstallCommand_CodexOnlySelection_DoesNotRequireClaudeAndRemovesNeutralStateLast|TestUpdateCommand_CodexOnly_UsesPersistedSelectionWithoutClaudeAndOmitsNativeModelWithoutFlag' -count=1` → `ok github.com/Angel-MercadoCLK/click-ai-devkit/internal/cli`; scenario: execute real Cobra doctor/update/uninstall flows against persisted Codex-only selection and isolated fake runtimes, proving target-first operation no longer requires Claude-only diagnostics or teardown when Claude is unselected |
| Rollback boundary | Revert `internal/installer/plan.go`, `internal/doctor/checks.go`, `internal/cli/{install,update,uninstall,doctor,configuretargets,preview}.go`, and the paired plan/CLI regression tests to remove the authoritative plan wiring without touching the external manual-evidence blockers |

## Test Summary

- Total tests written: 42 cumulative `Test*` functions in the current delta (2 added in the authoritative-plan batch)
- Total tests passing: focused and repository suites green
- Layers used: Unit, Integration
- Approval tests: None — behavior changed by spec
- Pure functions created: `BuildTargetPlan`, `TargetPlan.StepLabels`, `TargetPlan.CapabilitiesSummary`, `TargetPlan.InstallActionKinds`, `TargetPlan.UpdateActionKinds`, `TargetPlan.UninstallActionKinds`, `TargetPlan.DoctorCheckKinds`, `parseCodexModelConfig`, `parseTOMLKeyValue`, `DefaultItems`, `rootMenuItems`

## Repository Verification

- `go test ./... -count=1` → passed, exit 0
- `go vet ./...` → passed, exit 0
- `go build ./...` → passed, exit 0
- `gofmt -l .` → clean, exit 0
- Focused 24-test scenario suite → passed, exit 0, `sha256:9a15a66048746d87e48d7f089581ab20a07abc467aa658471e9d661fbd7ed753`
- Configured coverage `go test ./... -cover` → passed, exit 0; profile analysis reports 80.4% repository coverage and 77.5% changed-file coverage
- Windows packaged `click.exe` smoke → passed, exit 0, empty-output hash `sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`
- Release/document/version contract tests → 5 passed, exit 0, `sha256:62f3d2e67e75b689db652141dfe7894cb822c01d003118d863f27a65ccc8a7dc`

## Remaining Tasks

- [ ] 5.1 Final verification bundle

## Remaining Blockers

- Real installed-OpenClaw portability receipt is missing because OpenClaw is unavailable in this environment; fake binaries and smoke stubs were not substituted.
- Native review authority is ambiguous across multiple existing compact-facade lineages; the maintainer must select the authoritative existing lineage and rerun validation with `--lineage`.
- A real Claude restart plus nested-agent propagation transition remains an external follow-up; this batch only prepared the authoritative production wiring before that environment-level proof.
- Task 5.1 remains unchecked until these blockers are resolved.
