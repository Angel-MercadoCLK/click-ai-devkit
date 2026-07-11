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
