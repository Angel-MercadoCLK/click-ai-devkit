# Tasks: Presets de instalación

STRICT TDD: every GREEN task's edit must be preceded by its RED task (failing test written and
run first). Codex has zero prior context — every task names exact files/functions from
`documentacion/sdd/install-presets/design.md`.

## Ordered Checklist

### Slice A — `internal/preset` package
- [ ] RED: `internal/preset/preset_test.go` — `TestBuiltinPresets_FixedOrderAndValues` (3 presets:
      full-stack/balanced, economico/cost-saver, calidad/quality, in that order) and
      `TestResolve_HitAndMiss` (`Resolve("full-stack")` ok=true; `Resolve("bogus")` ok=false, zero
      value). Run `go test ./internal/preset/...` — confirm it fails to compile (package doesn't
      exist yet).
- [ ] GREEN: create `internal/preset/preset.go` — `Preset` struct, `BuiltinPresets()`,
      `Resolve(name string) (Preset, bool)`. Run tests — must pass.

### Slice B — `internal/ui/presetselect.go`
- [ ] RED: `internal/ui/presetselect_test.go` — mirror `profileselect_test.go`'s 4 tests
      (`TestNewPresetSelectModel_StartsOnFirstPreset`, cursor wrap down/up through all 3 presets,
      j/k parity, enter confirms + returns `tea.Quit`, esc/q cancels). Use the existing `keyMsg()`
      test helper already in the `ui` package. Run — confirm compile failure.
- [ ] GREEN: create `internal/ui/presetselect.go` — `PresetSelectModel{Cursor, Selected
      preset.Preset, Confirmed, Cancelled}`, `NewPresetSelectModel()`, `Update`, `View` (Label +
      Description per row). Run tests — must pass.

### Slice C — extract `performInstall` from `install.go` (behavior-preserving refactor)
- [ ] RED: confirm the FULL existing `internal/cli` test suite is green BEFORE refactoring
      (`go test ./internal/cli/...`) — this is the safety baseline, not a new failing test.
- [ ] GREEN: in `internal/cli/install.go`, extract `performInstall(cmd *cobra.Command, out
      io.Writer, r *ui.Renderer, cfg installer.Config, profile modelconfig.ProfileName, models
      map[modelconfig.Phase]string) error` containing every step currently after
      `resolveInstallModels` returns in `runInstall` (SyncMarketplacePlugins through "Instalación
      completa."). `runInstall` calls `performInstall` after resolving profile/models. Re-run the
      full `internal/cli` suite — must still be 100% green, unchanged behavior.

### Slice D — `click install-preset` command
- [ ] RED: `internal/cli/installpreset_test.go` — fake `presetSelector` returning a canned
      `preset.Preset`; `TestResolveAndRunInstallPreset_ConfirmedCallsPerformInstallWithResolvedProfile`
      and `TestResolveAndRunInstallPreset_CancelledSkipsPerformInstall` (assert via a
      `performInstall` seam — inject it the same way `resolveAndSaveConfiguredModels` injects
      `selector` in `configuremodels.go`). Run — confirm compile failure (file doesn't exist).
- [ ] GREEN: create `internal/cli/installpreset.go` — `newInstallPresetCommand()` (`Use:
      "install-preset"`, `Hidden: true`), `runInstallPreset` (TTY guard via `isTerminalWriter`,
      same fallback-message pattern as `configuremodels.go`), `resolveAndRunInstallPreset`,
      `runPresetSelectTUI`. Run tests — must pass.
- [ ] GREEN: register in `internal/cli/root.go`'s `root.AddCommand(...)` list. No new test needed
      (covered by existing root command-registration coverage, if any — otherwise add one
      assertion to `commands_test.go` that `install-preset` resolves).

### Slice E — wire the menu item
- [ ] RED: extend `internal/menu/menu_test.go`'s `ActionArgs` table test with
      `{action: ActionInstallPreset, want: []string{"install-preset"}}`. Run — confirm failure
      (`ActionInstallPreset` undefined).
- [ ] GREEN: in `internal/menu/menu.go` — add `ActionInstallPreset = "install-preset"` const, flip
      the "Presets de instalación" `Items` row to `Action: ActionInstallPreset, Active: true`, add
      the matching case in `ActionArgs`. Run — must pass. Also re-run
      `TestModel_Update_JKMoveCursorAndWrap`-style tests to confirm cursor math is unaffected
      (inert-vs-active row count changed, not row count itself).

### Final verification
- [ ] `go build ./...` — full build passes.
- [ ] `go test ./...` — full suite green.
- [ ] `gofmt -l .` — no unformatted files.

## Review Workload Forecast

Estimated diff size: Slice A ~100 lines, Slice B ~170 lines, Slice C ~60 lines (refactor churn),
Slice D ~160 lines, Slice E ~20 lines → **~510 lines total**, over the 400-line PR review budget.

- Decision needed before apply: Yes
- Chained PRs recommended: Yes
- 400-line budget risk: High

Recommended split (Feature Branch Chain):
- **PR1** (target: feature/tracker branch): Slices A + B — `internal/preset` +
  `internal/ui/presetselect.go` (pure, headless-testable, ~270 lines). Self-contained: compiles
  and tests green with no CLI/menu wiring yet.
- **PR2** (target: PR1's branch): Slices C + D + E — the refactor, the new command, and the menu
  wire-up (~240 lines). Depends on PR1 merging first.

## Acceptance Criteria

- [ ] `internal/preset.BuiltinPresets()` returns exactly 3 presets in fixed order; `Resolve`
      round-trips each `Name`.
- [ ] `ui.PresetSelectModel` keymap parity with `ProfileSelectModel` (j/k/arrows/enter/esc/q),
      verified by headless tests, no real bubbletea program required.
- [ ] `click install-preset` on a non-TTY writer prints the fallback message and returns nil (no
      hang, no error).
- [ ] `click install-preset` confirmed path installs with the SAME steps/side effects as
      `click install --yes --profile <preset.Profile>` (verified by both hitting `performInstall`).
- [ ] Menu row "Presets de instalación" is active, dispatches to `install-preset`, and the
      existing `runMenuLoop`/`dispatch` round-trip (already covered by `rootdefault_test.go`)
      still passes.
- [ ] Full existing `internal/cli` and `internal/menu` suites remain green after the refactor —
      zero behavior change to `click install`.
