# Apply Progress: interactive-menu-and-model-taxonomy

> **Persistence note**: Engram MCP tools (`mem_search`/`mem_get_observation`/`mem_save`) were not
> available in this sub-agent's tool set for this run. Per the launch prompt's fallback
> instruction, progress is persisted here as a file instead of `sdd/interactive-menu-and-model-
> taxonomy/apply-progress` in Engram. The orchestrator/next apply run should reconcile this file
> into Engram once the MCP connection is restored.

## Scope covered this batch: Work Unit 1 of 3 (stacked-to-main chain)

**Work Unit 1 — Taxonomy SSOT + models.json migration + doctor/update/plugins wiring** — COMPLETE.

Work Unit 2 (`internal/menu` standing TUI + `root.go` default action) and Work Unit 3
(`plugins/click-sdd/skills/*` content, `agents/*.md` content, taxonomy-lockstep test) are
explicitly OUT of scope for this batch and untouched.

## Mode
Strict TDD Mode (confirmed repo policy, D13). Every unit of behavior below followed RED → GREEN,
with REFACTOR where applicable. See TDD Cycle Evidence table.

## Completed Tasks
- [x] Rewrite `internal/modelconfig/modelconfig.go`: replaced the 5 invented phases with the real
      13-phase `Phase` type + ordered `Phases` slice (explore, propose, spec, design, tasks, apply,
      verify, archive, onboard, jd-judge-a, jd-judge-b, jd-fix-agent, default).
- [x] `ConfigKey()`: hyphen→underscore + `_model` suffix (e.g. `jd-judge-a` → `jd_judge_a_model`).
- [x] `Resolve()` silently drops old/unknown phase keys (explicit test case added for the exact old
      5-key taxonomy).
- [x] Default model assignment per phase: opus→propose/design/verify, haiku→archive/onboard,
      sonnet→explore/spec/tasks/apply/jd-judge-a/jd-judge-b/jd-fix-agent/default.
- [x] `internal/installer/models.go`: added `schema_version` (target `CurrentModelsSchemaVersion =
      2`), wrapped models.json as `{schema_version, models}`.
- [x] `IsStale(cfg)`: pure read-only detection (missing/lower schema_version OR old-taxonomy keys
      present). Absent file = not stale (healthy).
- [x] `MigrateIfStale(cfg)`: backs up stale file verbatim to `models.json.bak`, then fully
      regenerates with `modelconfig.Defaults()` — never preserves/merges old per-phase overrides
      (confirmed migration behavior, D8).
- [x] `internal/doctor/checks.go`: added `checkModelsConfig` (uses `IsStale`, never
      `MigrateIfStale` — keeps `click doctor` read-only per NFR-012). Wired into `Run()`. Absent
      file reports healthy; stale file reports unhealthy without mutating the file.
- [x] `internal/cli/update.go`: wired `installer.MigrateIfStale(cfg)` before the existing
      LoadModels/SaveModels re-apply flow, so `click update` migrates a stale config before
      re-emitting `--config` flags.
- [x] `internal/installer/plugins.go`: no code change needed — `clickSDDConfigArgs` already
      iterates `modelconfig.Phases` generically, so it adopted the new taxonomy automatically once
      `modelconfig.go` changed. Verified via updated `plugins_config_test.go`.
- [x] `plugins/click-sdd/.claude-plugin/plugin.json`: userConfig keys replaced with the 13
      `<phase>_model` keys, defaults matching `modelconfig.Defaults()`.
- [x] `internal/ui/modelselect.go`: `phaseLabels` map updated to the 13-phase taxonomy (required
      for the module to keep compiling/behaving correctly; the TUI itself already drove off
      `modelconfig.Phases`/`Defaults`/`Models` generically).
- [x] Updated every existing test that referenced the old 5-phase taxonomy: `modelconfig_test.go`,
      `models_test.go`, `plugins_config_test.go`, `installer_test.go`, `commands_test.go`,
      `checks_test.go`, `modelselect_test.go`.

## Files Changed
| File | Action | What Was Done |
|------|--------|----------------|
| `internal/modelconfig/modelconfig.go` | Rewritten | Real 13-phase taxonomy, `ConfigKey()`, `Defaults()`, `Resolve()` |
| `internal/modelconfig/modelconfig_test.go` | Rewritten | Tests for new taxonomy incl. old-key-drop case |
| `internal/installer/models.go` | Rewritten | `schema_version` wrapper, `IsStale`, `MigrateIfStale` |
| `internal/installer/models_test.go` | Modified | New taxonomy fixtures + schema/stale/migrate tests |
| `internal/installer/plugins_config_test.go` | Modified | Expected `--config` flags updated to 13 phases |
| `internal/installer/installer_test.go` | Modified | `TestInstall_RegistersPluginsAndWritesManagedState` command expectation updated |
| `internal/doctor/checks.go` | Modified | Added `checkModelsConfig`, wired into `Run()` |
| `internal/doctor/checks_test.go` | Modified | Check-count bump + 2 new tests (absent=healthy, stale=unhealthy+read-only) |
| `internal/cli/update.go` | Modified | Wired `MigrateIfStale` before re-apply |
| `internal/cli/commands_test.go` | Modified | New taxonomy fixtures + new migration-wiring test |
| `internal/ui/modelselect.go` | Modified | `phaseLabels` map updated to new taxonomy |
| `internal/ui/modelselect_test.go` | Modified | Cycle-order assertions updated (phase[0] default is now sonnet, not opus) |
| `plugins/click-sdd/.claude-plugin/plugin.json` | Rewritten | userConfig keys/titles/descriptions/defaults for 13 phases |

## TDD Cycle Evidence
| Unit | Test File | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|------|-----------|-------|------------|-----|-------|-------------|----------|
| modelconfig taxonomy rewrite | `internal/modelconfig/modelconfig_test.go` | Unit | N/A (full rewrite, approval-style) | ✅ Written, confirmed fail (undefined consts) | ✅ `go test` passed | ✅ multiple phases/defaults/override/old-key cases | ✅ ConfigKey byte-loop kept simple |
| models.go schema_version + IsStale + MigrateIfStale | `internal/installer/models_test.go` | Unit | ✅ existing round-trip tests re-targeted first | ✅ Written, confirmed fail (undefined symbols) | ✅ `go test` passed | ✅ absent/current/legacy-flat/lower-schema-version cases | ➖ None needed |
| doctor stale-config check | `internal/doctor/checks_test.go` | Unit | ✅ 3/3 existing Run() tests passing before edit | ✅ Written, confirmed fail (missing check, count mismatch) | ✅ `go test` passed | ✅ absent=healthy vs stale=unhealthy, plus read-only (no mutation/.bak) assertion | ➖ None needed |
| update.go MigrateIfStale wiring | `internal/cli/commands_test.go` | Unit (cobra command level) | ✅ existing update tests passing before edit | ✅ Written, confirmed fail (no `.bak`, `Stat` error) — reverted the wiring line to prove RED before reapplying | ✅ `go test` passed after reapplying wiring | ➖ Single scenario (stale-file wiring); direct migration edge cases already covered by `models_test.go` | ➖ None needed |
| plugins.go / ui / installer_test.go taxonomy alignment | `plugins_config_test.go`, `modelselect_test.go`, `installer_test.go` | Unit | ✅ pre-existing suites | ✅ Confirmed fail against old-taxonomy expectations after modelconfig rewrite | ✅ `go test` passed after updating expectations | ➖ Approval-test style (production code unchanged/generic) | ➖ None needed |

### Test Summary
- **Total tests written/modified this batch**: ~30 (new: `TestPhases_HasThirteenPhases`, old-key-drop
  case in `TestResolve`, `TestSaveModels_WritesCurrentSchemaVersion`, 4×`TestIsStale_*`,
  3×`TestMigrateIfStale_*`, 2×`TestCheckModelsConfig_*`, `TestUpdateCommand_MigratesStaleModelsBeforeReapplying`;
  modified: taxonomy fixtures across `modelconfig_test.go`, `models_test.go`, `plugins_config_test.go`,
  `installer_test.go`, `commands_test.go`, `checks_test.go`, `modelselect_test.go`)
- **Total tests passing**: full `go test ./...` green (see verification below)
- **Layers used**: Unit only (no integration/E2E layer available/needed for this data+wiring change)
- **Approval tests** (refactoring): the modelconfig rewrite and the plugins/ui/installer_test.go
  taxonomy alignment were approval-style — production behavior contract preserved (generic
  iteration over `modelconfig.Phases`), only the taxonomy data changed
- **Pure functions created**: `Phase.ConfigKey()` (pure), `IsStale` (pure read), `MigrateIfStale`
  (impure by necessity — file I/O, but isolated and covered)

## Deviations from Design
- The launch prompt scoped wiring to "doctor/update/plugins". `internal/cli/install.go` was
  deliberately left unmigrated (it already unconditionally overwrites `models.json` via `SaveModels`
  at the end of `runInstall`, so a stale file gets clobbered by a fresh regen either way — but
  without the `.bak` safety net `MigrateIfStale` provides). This is a minor gap versus the D8
  "never clobber without a backup" philosophy for the install path specifically; flagging it for a
  follow-up decision rather than silently expanding this batch's scope beyond what was authorized.
- `internal/installer/plugins.go` itself required **zero** code changes — `clickSDDConfigArgs`
  already iterated `modelconfig.Phases` generically. Only its test file needed updating. Noting
  this since the task description implied plugins.go itself would need edits.
- `internal/ui/modelselect.go` was not explicitly listed as in-scope for Work Unit 1, but its
  `phaseLabels` map hardcoded the 5 old `Phase` constants and would not compile once
  `modelconfig.go` changed. Updated it (and its test's hardcoded default-value assertions) as a
  required consequence of the taxonomy rewrite, not a scope expansion into Work Unit 2's menu work.

## Issues Found
- `gofmt -l .` is **not** empty on this Windows checkout, but this is a pre-existing, repo-wide
  condition unrelated to this batch's edits: the working tree uses CRLF line endings (confirmed by
  diffing `git show HEAD:<file>` against the working-tree copy of files I never touched, e.g.
  `internal/installer/config.go` — both fail gofmt -l identically before and after this batch).
  Every newly-written file in this batch (`modelconfig.go`, `models.go`, and their test files,
  written via full-file rewrite) is independently gofmt-clean; files edited in-place
  (`checks.go`, `checks_test.go`, `update.go`, `commands_test.go`) inherited the pre-existing CRLF
  from their original content and therefore still show up in `gofmt -l`, but contain no real
  formatting defects (verified by normalizing line endings and re-running gofmt -l, which reports
  clean). Recommend a separate, repo-wide line-ending normalization change — out of scope here to
  avoid an unscoped, all-files diff inside a stacked PR.

## Verification (Work Unit 1 boundary)
- `go build ./...` — clean, no errors.
- `go vet ./...` — clean, no findings.
- `go test ./...` (fresh, `-count=1`) — all packages `ok`:
  `audit`, `cli`, `doctor`, `guard`, `installer`, `manifest`, `modelconfig`, `ui`
  (`cmd/click`, `internal/version`, `plugins/click-stub` have no test files, as before).
- `gofmt -l .` — pre-existing CRLF noise only (see Issues Found); zero *content* formatting defects
  introduced by this batch.
- `plugins/click-sdd/.claude-plugin/plugin.json` — validated as parseable JSON.

## Workload / PR Boundary
- Mode: stacked-to-main chained PR slice (PR1 of 3).
- Current work unit: Work Unit 1 — Taxonomy SSOT + models.json migration + doctor/update/plugins
  wiring.
- Boundary: starts at the invented 5-phase `modelconfig.go`; ends at a fully-compiling, fully-green
  repo with the real 13-phase taxonomy wired through `modelconfig`, `models.json` schema/migration,
  `click doctor`, `click update`, `click-sdd`'s `--config` flag emission, and `plugin.json`. No
  `internal/menu` code, no `root.go` default-action change, no `plugins/click-sdd/skills/*` or
  `agents/*.md` content changes — those remain for PR2/PR3.
- Rollback: `git revert` this slice cleanly reverts to the invented 5-phase taxonomy; PR2/PR3 code
  does not exist yet, so there is no cross-slice coupling to worry about.
- Estimated review budget impact: touches 13 files, all mechanically consistent with the taxonomy
  swap (data + wiring, no new architecture) — expect moderate-but-reviewable diff size, consistent
  with a deliberately-scoped Work Unit 1 slice.

## Remaining Tasks (next apply run — Work Unit 2)
- [ ] Design/scaffold `internal/menu` (or `internal/tui`, per design doc) hosting a persistent
      bubbletea program.
- [ ] `root.go`: default no-arg action — if no subcommand AND stdout is a TTY, launch the menu;
      otherwise print help (non-TTY/CI safety, matches proposal's "Bare `click` hangs CI" risk
      mitigation).
- [ ] Menu wires existing working commands (install, doctor, update, uninstall) as active items;
      presets + agent-creation appear as inert "coming soon" placeholders.
- [ ] j/k navigation + updates indicator at top, matching gentle-ai parity (per proposal's Success
      Criteria).
- [ ] Strict TDD: test model update/dispatch logic headless; keep the bubbletea `View()` thin per
      proposal's risk mitigation for the interactive-TUI package.
- [ ] Do NOT touch `plugins/click-sdd/skills/*` content, `agents/*.md` content, or attempt the
      taxonomy-lockstep test in this next batch — that is Work Unit 3 (PR3) and would fail until
      PR3's skill-content authoring lands.

## Status
Work Unit 1: 12/12 sub-tasks complete (see Completed Tasks above). `go test ./...` green.
Ready for PR1 review/merge. Work Unit 2 (menu) is next — fresh `sdd-apply` run should start there.

---

## Batch 2 (same PR1/Work Unit 1 slice): install.go MigrateIfStale gap fix

**Scope**: one gap closed — `internal/cli/install.go` now gets the same `installer.MigrateIfStale`
safety net `internal/cli/update.go` already had (D8 "never clobber a working setup without a
backup"). This was explicitly flagged as a deviation in Batch 1 above and is now resolved per
user confirmation. `internal/menu`, `root.go` default action, and `plugins/click-sdd/skills/*`/
`agents/*.md` remain untouched (still Work Unit 2/3 scope).

### Mode
Strict TDD Mode. RED confirmed before implementation.

### Completed Tasks
- [x] `internal/cli/install.go`: wired `installer.MigrateIfStale(cfg)` right after building `cfg`,
      before the existing model-selection/`SaveModels` flow — mirrors `update.go`'s wiring exactly
      (same call site pattern, same comment style referencing D8).

### TDD Cycle Evidence
| Unit | Test File | Layer | Safety Net | RED | GREEN | REFACTOR |
|------|-----------|-------|------------|-----|-------|----------|
| install.go MigrateIfStale wiring | `internal/cli/commands_test.go` | Unit (cobra command level) | existing install/update suites passing before edit | Wrote `TestInstallCommand_FreshInstall_NoBackupCreated` (passed immediately — pre-existing behavior) and `TestInstallCommand_MigratesStaleModelsBeforeOverwriting` (confirmed FAIL: `ReadFile(models.json.bak)` — file not found, since install.go didn't call `MigrateIfStale` yet) | Added the wiring line to `install.go`; both tests pass | None needed — single-line wiring, symmetric with update.go |

### Files Changed
| File | Action | What Was Done |
|------|--------|----------------|
| `internal/cli/install.go` | Modified | Added `installer.MigrateIfStale(cfg)` call before model selection, mirroring `update.go` |
| `internal/cli/commands_test.go` | Modified | Added `TestInstallCommand_FreshInstall_NoBackupCreated` (no spurious `.bak` on fresh install) and `TestInstallCommand_MigratesStaleModelsBeforeOverwriting` (stale legacy `models.json` backed up verbatim to `.bak` before regeneration, same fixture pattern as `TestUpdateCommand_MigratesStaleModelsBeforeReapplying`) |

### Verification
- `go build ./...` — clean.
- `go vet ./...` — clean.
- `go test ./... -count=1` — all packages `ok`: `audit`, `cli`, `doctor`, `guard`, `installer`,
  `manifest`, `modelconfig`, `ui` (no regressions to Work Unit 1's 12/12 sub-tasks).

### Deviations from Design
- None new. This batch closes the exact deviation flagged in Batch 1's "Deviations from Design"
  section (install.go left unmigrated). That note should now be considered resolved.

## Status (updated)
Work Unit 1: 12/12 sub-tasks complete + install.go gap fix complete. `go test ./...` green across
both batches. Ready for PR1 review/merge. Work Unit 2 (menu) is next — fresh `sdd-apply` run
should start there.

---

## Work Unit 2 (PR2 of 3, stacked-to-main chain): standing `internal/menu` + `root.go` default action

**Scope**: new `internal/menu` package (bubbletea Model/Update/View + pure `ActionArgs` dispatch),
`root.go` default no-arg action with a TTY-safe `interactive()` gate, and a new hidden
`configure-models` subcommand backing the menu's "Configure models" item. Work Unit 1's taxonomy
code (`internal/modelconfig`, `models.json` migration, doctor/update/install wiring) is untouched.
Work Unit 3 (`plugins/click-sdd/skills/*` content, `agents/*.md` content, taxonomy-lockstep test)
remains explicitly out of scope.

### Persistence note
Engram MCP tools (`mem_search`/`mem_get_observation`/`mem_save`) were not available in this
sub-agent's tool set for this run either — same as Work Unit 1. Progress persisted here as a file
per the launch prompt's fallback instruction.

### Mode
Strict TDD Mode (confirmed repo policy). Every unit of behavior below followed RED → GREEN.

### Completed Tasks
- [x] `internal/menu/menu.go`: new package hosting the standing menu's bubbletea `Model`
      (`Cursor`, `Chosen`, `StatusMsg`), the fixed `Items` list (5 active items + 7 inert
      "coming soon" placeholders + Quit), and the pure `ActionArgs(action string) []string`
      dispatch-mapping function.
- [x] j/k (and arrow key) cursor navigation with wraparound; inert items are not skipped (no
      special-casing in cursor math, per design decision 4).
- [x] Enter on an active item sets `Chosen` to that item's action and returns `tea.Quit`; Enter on
      an inert item sets a transient `StatusMsg` ("coming soon — not implemented yet") and stays
      in the menu (no quit, no dispatch).
- [x] q/esc/ctrl+c set `Chosen = ActionQuit` and quit; `ActionArgs(ActionQuit)` and any unknown
      action return `nil` — the caller treats that as "nothing to dispatch, exit cleanly."
- [x] `View()` renders a static placeholder header line (see Simplification below), every item
      label, and a `(coming soon)` suffix + dimmed style on inert rows.
- [x] `internal/cli/rootdefault.go`: `runRootDefault` (root's new `RunE`) — bare `click` on a real
      TTY launches the menu program, then (after it returns) maps `Chosen` via
      `menu.ActionArgs` and dispatches through `dispatch()`; non-TTY prints help and returns nil
      (exit 0, no hang). An unrecognized subcommand token (`args` non-empty when root's `RunE`
      fires) returns the same `unknown command %q for %q` shape cobra itself used before root had
      a `RunE` — this explicit guard was required because giving root a `RunE` at all disables
      cobra's own unknown-subcommand rejection.
- [x] `interactive(noInteractive bool, out io.Writer, in io.Reader) bool`: false when
      `--no-interactive` is set, `CI` env var is set, stdout isn't a real terminal, or stdin isn't
      a real terminal (checked independently — bubbletea starves on piped stdin even with a TTY
      stdout). Takes `io.Writer`/`io.Reader`, never touches `os.Stdout`/`os.Stdin` directly.
- [x] `dispatch(cmd *cobra.Command, args []string) error`: runs a **fresh**
      `NewRootCommand()` instance (not the live, already-executing root) attached to the caller's
      own `Out`/`Err`/`In` streams, avoiding flag-state re-entrancy.
- [x] `internal/cli/configuremodels.go`: new hidden `click configure-models` subcommand backing
      the menu's "Configure models" item — reuses `install.go`'s existing `runModelSelectTUI`
      (the same `internal/ui.ModelSelectModel` screen WU1 already relabeled to 13 phases; **not
      rebuilt**) and persists the result via `installer.SaveModels`. Registered on root
      (`Hidden: true`) so it's reachable both from the menu's dispatch path and directly
      (`click configure-models`) for scripts. Guarded by the same `isTerminalWriter` TTY check so
      it can never spin up a real bubbletea program against non-terminal output even when invoked
      directly outside the menu.
- [x] `internal/cli/root.go`: registered `RunE: runRootDefault`, added the `--no-interactive` flag
      (local to root, not `PersistentFlags`, so it never collides with `install`'s own
      `--yes`/`--non-interactive` local flags), and added `newConfigureModelsCommand()` to
      `root.AddCommand(...)`. Explicit subcommands (`install`, `update`, `doctor`, `uninstall`,
      `memory-guard`) were not touched beyond this registration — their own `RunE` functions are
      unchanged.
- [x] `internal/cli/commands_test.go`: `execRoot`'s test helper now pins `root.SetIn(&bytes.Buffer{})`
      (previously left unset, defaulting to the real `os.Stdin`) so every CLI test — including the
      new default-action TTY gate — is deterministic regardless of the test runner's actual stdin.

### Files Changed
| File | Action | What Was Done |
|------|--------|----------------|
| `internal/menu/menu.go` | Created | Standing menu `Model`/`Update`/`View`, `Items`, `ActionArgs` |
| `internal/menu/menu_test.go` | Created | Cursor nav, active/inert Enter behavior, quit keys, View content, ActionArgs mapping — all headless (no real bubbletea program) |
| `internal/cli/rootdefault.go` | Created | `runRootDefault`, `interactive()`, `isTerminalWriter`/`isTerminalReader`, `dispatch()` |
| `internal/cli/rootdefault_test.go` | Created | interactive()/isTerminal* branch coverage, non-TTY no-hang root test, unknown-subcommand-still-errors test, dispatch() unit tests |
| `internal/cli/configuremodels.go` | Created | Hidden `configure-models` subcommand, TTY-guarded, reuses `runModelSelectTUI` + `installer.SaveModels` |
| `internal/cli/root.go` | Modified | Wired `RunE`, `--no-interactive` flag, registered `configure-models` |
| `internal/cli/commands_test.go` | Modified | `execRoot` now pins `SetIn` to an empty buffer |

### TDD Cycle Evidence
| Unit | Test File | Layer | RED | GREEN | REFACTOR |
|------|-----------|-------|-----|-------|----------|
| `internal/menu` Model/Update/View/ActionArgs | `internal/menu/menu_test.go` | Unit | ✅ Written first, confirmed fail (undefined `NewModel`/`Items`/`Item`/`Model`/`ActionQuit`, package didn't exist) | ✅ `go test ./internal/menu/...` passed after `menu.go` | ➖ None needed |
| `interactive()` + `isTerminalWriter`/`isTerminalReader` + `dispatch()` + root default action + `configure-models` | `internal/cli/rootdefault_test.go` | Unit + cobra-command level | ✅ Written first, confirmed fail (undefined `interactive`/`isTerminalWriter`/`isTerminalReader`/`dispatch`, build failed) | ✅ `go test ./internal/cli/...` passed after `rootdefault.go`, `configuremodels.go`, and `root.go` wiring | ➖ None needed |

### Test Summary
- **Total tests written this batch**: 12 in `internal/menu/menu_test.go` + 14 in
  `internal/cli/rootdefault_test.go` = 26 new tests, plus the `execRoot` helper fix that benefits
  every existing CLI test.
- **Total tests passing**: full `go test ./... -count=1` green (see verification below), including
  every prior Work Unit 1 test.
- **Layers used**: Unit (menu Update/dispatch logic tested headless) + cobra-command level (root
  default action exercised through `execRoot`'s `bytes.Buffer` streams, never a real TTY).
- **No real bubbletea program was ever spun up in tests** — `menu.Model.Update`/`View` are tested
  as plain Go values via a `updateModel` helper (mirrors the existing pattern in
  `internal/ui/modelselect_test.go`); `runRootDefault`'s `tea.NewProgram(...).Run()` call is only
  reached when `interactive()` returns true, and every test forces the non-TTY branch via
  `bytes.Buffer` streams, so that branch is never exercised in the test suite by construction.

### Simplification — updates-indicator is a static placeholder
The proposal's "update-indicator display" requirement is satisfied **structurally only**: the
menu header (`headerVersion` const in `internal/menu/menu.go`) is a static string
(`"click-ai-devkit (updates: not checked)"`), not a real update-availability check. No
network/version-compare logic was implemented — this was explicitly out of scope per the launch
prompt. Flagging this clearly as a known simplification for a future task to wire real data into
the same header position.

### Deviations from Design
- **"Configure models" needed a new dispatch target that didn't exist before.** The design's
  dispatch pattern (`ActionArgs(chosen) []string` → fresh `NewRootCommand().SetArgs(args).Execute()`)
  assumes every active menu item maps to an *existing* cobra subcommand. `install`/`update`/
  `doctor`/`uninstall` already existed, but there was no standalone command for "pick per-phase
  models and save them" — that flow previously only existed inline inside `click install`. Rather
  than special-case "Configure models" outside the uniform dispatch mechanism (which the design
  explicitly wants to keep pure/testable), a new hidden `click configure-models` subcommand was
  added so the *same* `ActionArgs`/`dispatch()` pattern covers it too. This is an additive,
  backward-compatible extension of the design's Affected Areas list (`internal/cli/root.go` already
  listed as "Modified"), not a deviation from the dispatch mechanism itself.
- **Explicit guard added for the unknown-subcommand case.** Giving `root` a `RunE` disables
  cobra's own default "unknown command" rejection for unmatched subcommand tokens (cobra only
  applies that check when the command itself has no `Run`/`RunE`). `runRootDefault` restores the
  prior UX explicitly (`args non-empty → return the same error shape cobra used to produce`) so
  `click <typo>` keeps failing loudly instead of silently falling through to help text. Not called
  out in the design doc, but necessary to avoid a real regression.
- **`interactive()`'s true-TTY branch has no dedicated unit test** — only the false branch (the
  safety-critical one) is exercised deterministically via `bytes.Buffer`/`os.Pipe`, matching the
  pre-existing convention already used by `install.go`'s `isNonInteractiveInstall` (which also has
  no true-TTY unit test). A genuine positive TTY assertion would require a real pseudo-terminal,
  which isn't portable across this repo's Windows/CI test environment.

### Issues Found
None new. `gofmt -l` is clean on every file created or modified in this batch
(`internal/menu/menu.go`, `internal/menu/menu_test.go`, `internal/cli/rootdefault.go`,
`internal/cli/rootdefault_test.go`, `internal/cli/configuremodels.go`, `internal/cli/root.go`,
`internal/cli/commands_test.go`) — verified directly, zero output.

### Verification (Work Unit 2 boundary)
- `go build ./...` — clean, no errors.
- `go vet ./...` — clean, no findings.
- `go test ./... -count=1` — all packages `ok`: `audit`, `cli`, `doctor`, `guard`, `installer`,
  `manifest`, `menu` (new), `modelconfig`, `ui` (`cmd/click`, `internal/version`,
  `plugins/click-stub` have no test files, as before).
- `gofmt -l` — clean on every file created/touched this batch (see Issues Found).

### Workload / PR Boundary
- Mode: stacked-to-main chained PR slice (PR2 of 3).
- Current work unit: Work Unit 2 — standing `internal/menu` TUI + `root.go` default action +
  `configure-models` dispatch target.
- Boundary: starts at PR1's merged taxonomy realignment (no menu code, no default root action);
  ends at a fully-compiling, fully-green repo where bare `click` on a TTY opens a navigable menu
  wired to real `install`/`update`/`doctor`/`uninstall`/`configure-models` commands, with inert
  placeholders and a safe non-TTY/CI fallback. No `plugins/click-sdd/skills/*` or `agents/*.md`
  content changes — those remain PR3.
- Rollback: `git revert` this slice cleanly restores root to its Work-Unit-1 subcommand-only
  behavior (`RunE` removed, `internal/menu` and the two new `internal/cli` files disappear); PR3
  does not exist yet, so there is no cross-slice coupling.
- Estimated review budget impact: 3 new files (`internal/menu/menu.go` ~215 lines,
  `internal/cli/rootdefault.go` ~100 lines, `internal/cli/configuremodels.go` ~55 lines) + 2 new
  test files + small edits to `root.go` and `commands_test.go`. Larger than Work Unit 1's
  mechanical taxonomy swap since this introduces new architecture (a second bubbletea program +
  the dispatch mechanism), but self-contained to `internal/menu` and `internal/cli` with no
  changes to `internal/installer`/`internal/doctor`/`internal/modelconfig`.

## Remaining Tasks (next apply run — Work Unit 3)
- [ ] `plugins/click-sdd/skills/*` content: rename/rewrite skills to match the real 13-phase
      taxonomy (`explore, propose, spec, design, tasks, apply, verify, archive, onboard,
      jd-judge-a, jd-judge-b, jd-fix-agent, default`).
- [ ] `agents/*.md` content updates to match the same taxonomy.
- [ ] Taxonomy-lockstep test (explicitly deferred from WU1 and WU2 — would fail until this
      content lands).

## Status (updated)
Work Unit 1: 12/12 sub-tasks complete + install.go gap fix complete. Work Unit 2: 9/9 sub-tasks
complete — `internal/menu` package + `root.go` default action + `configure-models` dispatch
target, all TDD RED→GREEN, `go test ./...` fully green. Ready for PR2 review/merge. Work Unit 3
(`plugins/click-sdd/skills/*`, `agents/*.md`, taxonomy-lockstep test) is next — fresh `sdd-apply`
run should start there.

---

## Work Unit 2 — follow-up (loop-back + i18n), same PR2 slice

**Scope**: two verify-flagged WARNINGs from the PR2 verify report (W2, W3) closed. No new files;
`internal/menu` and `internal/cli` remain the only touched packages. Work Unit 1's taxonomy code
and Work Unit 3 (`plugins/click-sdd/skills/*`, `agents/*.md`, taxonomy-lockstep test) are still
untouched.

### Persistence note
Engram MCP tools (`mem_search`/`mem_get_observation`/`mem_save`) were not available in this
sub-agent's tool set for this run either — same as WU1/WU2. Progress persisted here as a file per
the launch prompt's fallback instruction; this section is appended, all prior sections (Work Unit
1, Batch 2, Work Unit 2) are left intact.

### Mode
Strict TDD Mode (confirmed repo policy). Both fixes below followed RED → GREEN.

### Completed Tasks
- [x] W2 fix — menu loops back after dispatching an active item instead of exiting the process.
      Added `runMenuLoop(launchMenu func() (string, error), dispatchFn func([]string) error) error`
      in `internal/cli/rootdefault.go`: launches the menu, dispatches the chosen action if any,
      then re-launches the menu again, repeating until `menu.ActionQuit` (or any chosen value that
      maps to no dispatch args via `menu.ActionArgs`) or an error from either `launchMenu` or
      `dispatchFn`. `runRootDefault` now builds `launchMenu`/`dispatchFn` closures over the real
      `tea.NewProgram(...).Run()` + `dispatch()` calls and delegates control-flow to
      `runMenuLoop` — the fresh-factory `dispatch()` pattern from Work Unit 2 is unchanged, only
      the surrounding control-flow now loops.
- [x] W3 fix — unified all menu-visible text to Spanish in `internal/menu/menu.go`: item labels
      (`Iniciar instalación`, `Actualizar herramientas`, `Configurar modelos`, `Ejecutar
      diagnóstico`, `Desinstalar`, `Salir`), inert placeholders (`Presets de instalación`, `Crear
      tu propio agente`, `Sincronizar configuración`, `Actualizar + Sincronizar`, `Gestionar
      backups`, `Plugins de la comunidad OpenCode`, `Perfiles SDD de OpenCode`), the
      updates-indicator header (`headerVersion` → `"click-ai-devkit (actualizaciones: sin
      verificar)"`), the `(coming soon)` view suffix → `(próximamente)`, and the transient
      `comingSoonMsg` → `"próximamente — todavía no implementado"`. The pre-existing Spanish
      footer (`j/k mover · enter seleccionar · q/esc salir`) was already correct and untouched.
      Code identifiers and comments were left in English per repo convention — only user-visible
      strings changed.

### Files Changed
| File | Action | What Was Done |
|------|--------|----------------|
| `internal/cli/rootdefault.go` | Modified | Added `runMenuLoop`; `runRootDefault` now builds `launchMenu`/`dispatchFn` closures and delegates to it instead of a single launch-then-dispatch-once call |
| `internal/cli/rootdefault_test.go` | Modified | Added 5 new `TestRunMenuLoop_*` tests (quit-immediately, dispatch-then-loop-until-quit, launchMenu error, dispatchFn error, empty-chosen) — all pure, injected-function tests, no real bubbletea program |
| `internal/menu/menu.go` | Modified | Translated `Items` labels, `headerVersion`, `comingSoonMsg`, and the `View()` inert-suffix string to Spanish |
| `internal/menu/menu_test.go` | Modified | `TestModel_View_InertItemsShowComingSoonSuffix` now asserts the `(próximamente)` suffix instead of the old English `(coming soon)` string |

### TDD Cycle Evidence
| Unit | Test File | Layer | RED | GREEN | REFACTOR |
|------|-----------|-------|-----|-------|----------|
| `runMenuLoop` control-flow (loop-back after dispatch) | `internal/cli/rootdefault_test.go` | Unit (pure, injected functions) | ✅ Written first, confirmed fail: `go test ./internal/cli/... -run TestRunMenuLoop` → 5x `undefined: runMenuLoop` build failure | ✅ Implemented `runMenuLoop` + rewired `runRootDefault`; `go test ./internal/cli/... -run TestRunMenuLoop -v` → 5/5 PASS | ➖ None needed — single small pure function |
| Spanish `(próximamente)` inert-item suffix | `internal/menu/menu_test.go` | Unit | ✅ Changed test assertion first, confirmed fail: `TestModel_View_InertItemsShowComingSoonSuffix` FAIL (View() still rendered `(coming soon)`) | ✅ Translated `menu.go` strings; `go test ./internal/menu/...` → PASS | ➖ None needed — literal string swap |

### Test Summary
- **Total tests written this batch**: 5 new (`TestRunMenuLoop_QuitImmediatelyReturnsNilWithoutDispatching`,
  `TestRunMenuLoop_DispatchesThenLoopsBackToMenuUntilQuit`,
  `TestRunMenuLoop_LaunchMenuErrorStopsLoopWithoutDispatching`,
  `TestRunMenuLoop_DispatchErrorStopsLoopWithoutRelaunching`, `TestRunMenuLoop_EmptyChosenReturnsNil`);
  1 existing test updated (`TestModel_View_InertItemsShowComingSoonSuffix`, Spanish assertion).
- **Total tests passing**: full `go test ./... -count=1` green, all 9 packages `ok` (no regressions
  to WU1's 12/12 or WU2's 9/9 sub-tasks — `TestRootCommand_NoArgs_NonTTY_PrintsHelpAndExitsCleanly`,
  `TestRootCommand_UnknownSubcommand_ReturnsError`, all `TestInteractive_*`/`TestIsTerminal*`
  re-run explicitly and confirmed still passing).
- **Layers used**: Unit only — `runMenuLoop` is tested as a pure function with injected
  `launchMenu`/`dispatchFn` closures (same pattern already used for `dispatch()` in WU2), no real
  bubbletea program spun up.
- **No real bubbletea program was ever spun up in tests** — same invariant as WU2; `runRootDefault`'s
  `tea.NewProgram(...).Run()` call is only reached when `interactive()` returns true, still
  unreachable in the test suite by construction (non-TTY branch forced via `bytes.Buffer`/`os.Pipe`
  everywhere).

### Deviations from Design
- None from the original design doc. This batch resolves two WARNINGs the WU2 verify report
  itself flagged as non-blocking-but-worth-confirming (W2 loop-back UX, W3 language consistency);
  the user explicitly confirmed loop-back is the correct spec-intended behavior ("Active Item
  Dispatch — ... then return to menu") before this batch started.
- `runMenuLoop`'s signature takes `dispatchFn func([]string) error` (args already resolved) rather
  than `func(action string) error` — this keeps `menu.ActionArgs` resolution (and the "nothing to
  dispatch" nil-args short-circuit) inside `runMenuLoop` itself, matching where `runRootDefault`
  previously did that resolution, and keeps the injected `dispatchFn` a thin wrapper over the
  real `dispatch()` helper.

### Issues Found
None. `gofmt -l` is clean (zero output) on every file touched this batch (`internal/cli/rootdefault.go`,
`internal/cli/rootdefault_test.go`, `internal/menu/menu.go`, `internal/menu/menu_test.go`) —
verified directly.

### Verification (this follow-up batch)
- `go build ./...` — clean, no errors.
- `go vet ./...` — clean, no findings.
- `go test ./... -count=1` — all 9 packages `ok`: `audit`, `cli`, `doctor`, `guard`, `installer`,
  `manifest`, `menu`, `modelconfig`, `ui`.
- `gofmt -l internal/menu/menu.go internal/menu/menu_test.go internal/cli/rootdefault.go internal/cli/rootdefault_test.go` — empty output (clean).
- Regression re-run: `TestRootCommand_NoArgs_NonTTY_PrintsHelpAndExitsCleanly`,
  `TestRootCommand_UnknownSubcommand_ReturnsError`, all `TestInteractive_*`, all `TestIsTerminal*`
  — all PASS, confirming the non-TTY no-hang guarantee and the unknown-subcommand-rejection fix
  from WU2 are intact.

### Workload / PR Boundary
- Mode: stacked-to-main chained PR slice (PR2 of 3) — same slice as Work Unit 2, this is a
  same-PR follow-up closing verify WARNINGs, not a new work unit.
- Boundary: starts at WU2's committed-ready state (select-and-exit, mixed-language menu); ends at
  loop-back-until-quit control-flow + fully Spanish menu UI, still 0 new files, small diff on top
  of WU2 (`rootdefault.go` +~20 lines, `rootdefault_test.go` +~115 lines of tests,
  `menu.go`/`menu_test.go` string-only edits).
- Rollback: `git revert` this follow-up cleanly restores WU2's select-and-exit + mixed-language
  behavior without touching WU1 or requiring WU3 to exist.
- Estimated review budget impact: small — mechanical control-flow extraction + string literal
  translation, no new architecture, stays well inside PR2's existing review budget.

## Status (updated again)
Work Unit 1: 12/12 sub-tasks complete + install.go gap fix complete. Work Unit 2: 9/9 sub-tasks
complete. Work Unit 2 follow-up: 2/2 sub-tasks complete (loop-back control-flow, Spanish i18n) —
both verify-flagged WARNINGs (W2, W3) from the PR2 verify report are now resolved. `go test ./...`
fully green across all batches, gofmt clean, no regressions. Ready for PR2 review/merge. Work Unit
3 (`plugins/click-sdd/skills/*`, `agents/*.md`, taxonomy-lockstep test) remains next — fresh
`sdd-apply` run should start there.
