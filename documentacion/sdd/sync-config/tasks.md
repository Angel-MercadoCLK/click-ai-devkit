# Tasks: Sincronizar configuración

STRICT TDD: every GREEN task's edit must be preceded by its RED task (failing test written and
run first). Codex has zero prior context — every task names exact files/functions from
`documentacion/sdd/sync-config/design.md`.

## Ordered Checklist

### Slice A — `installer.SyncMarketplacePluginConfigs`
- [ ] RED: in `internal/installer/plugins_test.go`, add
      `TestSyncMarketplacePluginConfigs_NeverRefreshesMarketplace` (assert the fake runner's
      recorded commands contain NO `plugin marketplace add`) and
      `TestSyncMarketplacePluginConfigs_InstallsAllManagedPluginsWithConfigFlags` (assert 3×
      `plugin install <plugin>@click-ai-devkit` with click-sdd carrying the `--config
      orchestration_profile=...` and per-phase `--config <phase>_model=...` flags, mirroring the
      existing `TestSyncMarketplacePlugins_*` assertions already in this file). Use
      `SetCommandRunnerFactoryForTests`/`SetMarketplaceSourceForTests`, same harness as existing
      `plugins_test.go` tests. Run `go test ./internal/installer/...` — confirm failure (function
      doesn't exist).
- [ ] GREEN: in `internal/installer/plugins.go` — extract the existing per-plugin loop out of
      `SyncMarketplacePlugins` (lines ~130-138) into a private helper, e.g.
      `installManagedPlugins(runner CommandRunner, resolved map[modelconfig.Phase]string, name
      modelconfig.ProfileName) error`; `SyncMarketplacePlugins` calls `addMarketplace` then this
      helper (unchanged behavior). Add exported
      `SyncMarketplacePluginConfigs(models map[modelconfig.Phase]string, profile
      ...modelconfig.ProfileName) error` that resolves the profile the same way
      `SyncMarketplacePlugins` does, then calls ONLY the helper (no `addMarketplace`). Run tests —
      must pass. Re-run the full `internal/installer` suite to confirm `SyncMarketplacePlugins`'s
      existing tests are unaffected.

### Slice B — `click sync` command
- [ ] RED: create `internal/cli/sync_test.go` — using the same isolated-temp-`Config`-dir +
      fake-`CommandRunner` harness as `install_test.go`/`update_test.go`:
      `TestRunSync_WritesManagedBlockAndMemoryGuardHook`,
      `TestRunSync_CallsSyncMarketplacePluginConfigsNotSyncMarketplacePlugins` (assert no `plugin
      marketplace add` in recorded commands),
      `TestRunSync_NeverCallsMigrateIfStale_ModelsJSONUnchanged` (seed a stale models.json fixture,
      assert byte-identical after `runSync`),
      `TestRunSync_FallsBackToBalancedWhenModelsJSONMissing` (no models.json on disk; assert
      `--config orchestration_profile=balanced` in the recorded install commands, and assert NO
      models.json is written as a side effect). Run — confirm compile failure (`runSync` doesn't
      exist).
- [ ] GREEN: create `internal/cli/sync.go` — `newSyncCommand()` (`Use: "sync"`, `Hidden: true`),
      `runSync(cmd *cobra.Command, args []string) error`: resolve `claudeHome`/`cfg`, call
      `installer.LoadModelsWithProfile(cfg)` (fallback `modelconfig.ProfileBalanced` +
      `modelconfig.Defaults()` when not found, matching `update.go`'s existing fallback), then run
      3 `r.RunStep` blocks calling `installer.SyncMarketplacePluginConfigs(models, profile)`,
      `installer.WriteManagedBlock(cfg.ClaudeMDPath(), installer.DefaultManagedContent)`,
      `installer.RegisterMemoryGuardHook(cfg)`, in that order, then print `"Sync completo."` via
      `r.Info`. Run tests — must pass.
- [ ] GREEN: register `newSyncCommand()` in `internal/cli/root.go`'s `root.AddCommand(...)` list.

### Slice C — wire the menu item
- [ ] RED: extend `internal/menu/menu_test.go`'s `ActionArgs` table test with
      `{action: ActionSync, want: []string{"sync"}}`. Run — confirm failure (`ActionSync`
      undefined).
- [ ] GREEN: in `internal/menu/menu.go` — add `ActionSync = "sync"` const, flip the "Sincronizar
      configuración" `Items` row to `Action: ActionSync, Active: true`, add the matching case in
      `ActionArgs`. Run — must pass.

### Idempotency check
- [ ] Add/confirm a test asserting `runSync` run twice back-to-back produces byte-identical
      CLAUDE.md and settings.json on the second run (both underlying installer functions are
      already idempotent by their own doc contracts — this test only needs to prove `sync.go`
      doesn't break that property, e.g. by resolving profile/models differently on each call).

### Final verification
- [ ] `go build ./...` — full build passes.
- [ ] `go test ./...` — full suite green.
- [ ] `gofmt -l .` — no unformatted files.

## Review Workload Forecast

Estimated diff size: Slice A ~130 lines (plugins.go extraction + new tests), Slice B ~150 lines
(sync.go + sync_test.go + root.go), Slice C ~20 lines → **~300 lines total**, under the 400-line
PR review budget.

- Decision needed before apply: No
- Chained PRs recommended: No
- 400-line budget risk: Low

Single PR is feasible; no slicing required.

## Acceptance Criteria

- [ ] `installer.SyncMarketplacePluginConfigs` issues zero `plugin marketplace add` commands and
      exactly one `plugin install <id>@click-ai-devkit` per entry in `managedPlugins`, with
      click-sdd carrying the full `--config` flag set.
- [ ] `click sync` writes/refreshes the CLAUDE.md managed block and the memory-guard PreToolUse
      hook, using the exact same content/idempotency guarantees as `click install`/`click update`.
- [ ] `click sync` never calls `MigrateIfStale`, `SyncEngram`, or `SyncContext7` — verified by
      absence of their side effects (no `models.json.bak`, no Engram plugin install command, no
      `claude mcp add` command) in the recorded fake-runner command list.
- [ ] `click sync` never writes `models.json` (no `SaveModelsWithProfile` call).
- [ ] Menu row "Sincronizar configuración" is active, dispatches to `sync`, and the existing
      `runMenuLoop`/`dispatch` round-trip still passes.
- [ ] Full existing `internal/installer` and `internal/cli` suites remain green — zero behavior
      change to `click update` or `click install`.
