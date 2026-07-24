# Design: Repository Production Quality

## Technical Approach

Replace the partially duplicated command orchestration with one declarative, target-scoped plan. `install`, `update`, `configure-targets`, `uninstall`, rollback, progress rendering, preview, snapshot selection, and doctor expected state will project from the same plan; commands remain Cobra adapters. This reconciles the current `BuildInstallPlan`, string-only preview builders, Claude-home target state, line-based Codex TOML editing, and assumed OpenClaw arguments without treating any as a verified contract.

## Architecture Decisions

| Decision | Choice | Alternatives considered | Rationale |
|---|---|---|---|
| Plan authority | Immutable `TargetPlan` of capabilities and ordered `Step`s (preflight, writes, snapshot paths, rollback, doctor check, progress label). | Parallel lists and command-local `RunStep` calls. | One source prevents preview/execution/rollback drift. |
| Neutral state | Versioned Click-owned state under an OS user config directory, with an injectable path provider; legacy Claude state is read only for migration. | Continue `targets.json` under `ClaudeHome`. | Codex/OpenClaw-only installs cannot depend on Claude. |
| Uninstall lifecycle | `uninstall` loads neutral selection and projects ordered teardown steps from the same plan; it removes neutral state only after all selected teardown steps succeed. | Claude-first unconditional teardown or a separate hard-coded list. | Claude-free uninstall must neither resolve Claude home nor diverge from install/update ownership. |
| Codex write | TOML parser/edit model for the documented root `model` key; preserve unrelated/table-scoped data and atomically replace only after parse/render succeeds. | Prefix scan plus `os.WriteFile`. | The current scan can match table keys and write non-atomically. |
| Native process boundary | Qualified, allowlisted adapters with absolute binary, argument vector, safe working directory, timeout, exit/output evidence, and test seam. | Shell strings or inferred CLI APIs. | Prevents cwd injection and unverified OpenClaw mutation. |
| OpenClaw release gate | Keep native model mutation unavailable until a real installed-CLI probe proves command, keys, JSON flag, and observable result; otherwise plan emits an actionable skipped/blocked step. | Ship the currently assumed `config set` shape. | Documentation itself marks that shape pending confirmation. |
| Review workload guard | This session uses an 800 authored-line budget (additions + deletions); generated goldens remain in snapshot identity but not risk count. | The shared 400-line default or an unconditional chain. | `openspec/config.yaml` and the session set 800; auto-forecast chains only when the 800-line risk is high. |

## Data Flow

```text
target selection + detected capabilities
              -> PlanBuilder -> preview / confirmation / snapshot manifest
                                -> install/update/uninstall adapters -> executor -> progress + execution journal
                                -> rollback and doctor projections
```

`Step` records target ownership, prerequisites, declared paths, apply/undo action, and expected diagnosis. Confirmation displays exactly those mutations. After confirmation, snapshot all declared pre-state paths, execute in order, and on failure reverse-rollback only completed owned steps. `doctor` is read-only and reports the same expected steps plus repair guidance; `update` is the qualified repair path. Native model changes in noninteractive mode require the corresponding explicit model flag.

`uninstall` first loads neutral selection, then projects only selected target teardown. It snapshots each declared Click-owned artifact and neutral state, journals completed steps, and reverses completed teardown from those snapshots on failure. It removes neutral state last, after successful teardown; failed removal restores its snapshot. It never deletes user-managed files outside declared plan paths and never resolves Claude home/preflight when Claude is unselected.

## Interfaces / Contracts

```go
type Step struct { ID string; Target Target; Snapshot []Path; Apply Action; Undo Action; Check Check }
type TargetPlan struct { Selection TargetSelection; Steps []Step }
```

`Action` is typed, never a shell command string. `UninstallAction` is a target-scoped adapter with declared owned paths and reversible result; the Cobra adapter reports the journal, stops on a failed planned step, and rolls back completed declared teardown. The Codex action accepts only validated TOML and commits through the installer atomic writer. The OpenClaw adapter is registered only with a proven `Contract`; failed qualification, missing binary, timeout, or unexpected output causes no native write. Every subprocess uses a Click-controlled absolute work directory, never caller cwd; Git recovery reports the failed stage, cleans temporary artifacts, and leaves user repositories untouched. Cache checks diagnose stale marketplace/Engram state, prescribe `click update`, then require a restarted session before health is claimed.

## File Changes

| File | Action | Description |
|---|---|---|
| `internal/installer/plan.go`, `plan_test.go` | Create | Plan, step contracts, projections, journal, and RED tests. |
| `internal/cli/{install,update,preview,installplan,configuretargets,uninstall,rollback}.go` | Modify | Build/confirm/execute every lifecycle command, including target-scoped uninstall, exclusively from the plan. |
| `internal/cli/{commands_test,uninstall_test}.go` | Modify/Create | Fake command seams and RED coverage for uninstall selection, journal, rollback, and neutral-state cleanup. |
| `internal/installer/{config,targets,snapshot,codexmodel,openclawmodel,plugins,gitpreflight}.go` | Modify | Neutral state, atomic TOML, qualified adapters, safe cwd/recovery. |
| `internal/doctor/checks.go` | Modify | Plan-derived target, cache, and repair diagnostics. |
| `internal/menu/menu.go`, `internal/cli/rootdefault.go` | Modify | Grouped, availability-aware menu retaining exact `Plugins`. |
| `.github/workflows/{ci,release}.yml`, `.goreleaser.yaml` | Modify | Windows package/runtime qualification before publication. |
| `README.md`, `documentacion/{codex-target,portability-runbook}.md` | Modify | Current contracts, recovery, and historical boundaries. |

## Testing Strategy

| Layer | What to Test | Approach |
|---|---|---|
| Unit (RED first) | Plan projections, neutral migration, uninstall teardown/journal/rollback/last-state cleanup, TOML tables/comments/error atomicity, menu groups. | Temp homes and injected filesystem/process/path seams. |
| Integration | Claude-free Codex/OpenClaw install/update/configure/uninstall; Windows safe-cwd Git recovery; stale cache -> update -> restarted doctor. | Fake binaries, isolated config directory, no real Claude home. |
| CI/release | Windows `click.exe` package behavior, manifest/version/Scoop metadata, supported target contracts. | Matrix evidence gates publication; execute the packaged Windows artifact. |
| Manual smoke | Real OpenClaw contract probe; Scoop stale-bucket recovery; live restart visibility. | Recorded version/output evidence; never substituted by mocks. |

## Threat Matrix

| Boundary | Applicability | Safe/failure behavior | Planned RED tests |
|---|---|---|---|
| Documentation-like paths | N/A — no executable classification. | — | — |
| Git repository selection | Applicable — marketplace subprocesses run on Windows. | Absolute Click workdir; invalid/unsafe cwd fails cleanly, cleans temp state, changes no repo. | Relative, absolute, non-repo, and unsafe cwd selectors. |
| Commit state | N/A — no commit automation. | — | — |
| Push state | N/A — no push automation. | — | — |
| PR commands | N/A — no PR automation. | — | — |

## Migration / Rollout

Atomically write neutral state only after confirmed selection; import valid legacy selection once. Uninstall snapshots selected owned state and neutral state, rolls back completed teardown on failure, and deletes only Click-owned neutral state after success. Deliver four verifiable slices: plan/state, native contracts, diagnostics/menu, then docs/release. Tasks use the session's 800-line guard and MUST state `Decision needed before apply`, `Chained PRs recommended`, and `800-line budget risk`; a high risk requires chained slices. Block release if OpenClaw qualification or Windows package evidence is absent.

## Open Questions

None. OpenClaw CLI verification is a release prerequisite, not an assumption.
