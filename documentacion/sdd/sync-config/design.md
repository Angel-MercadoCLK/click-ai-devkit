# Design: Sincronizar configuración (menu item)

## Technical Approach

Turn `internal/menu/menu.go:53` ("Sincronizar configuración", currently `Active: false`) into a
real menu action that re-applies click's managed config surfaces — CLAUDE.md managed block,
click-sdd's `--config` plugin flags, and the memory-guard PreToolUse hook — WITHOUT refreshing
the plugin marketplace or touching Engram/Context7 version pins. This is deliberately a strict
subset of `click update` (`internal/cli/update.go`), which additionally runs `MigrateIfStale`,
refreshes the marketplace (`addMarketplace`), and re-pins Engram to `manifest.Engram.Version`.

## Architecture Decisions

### Decision 1: Split `SyncMarketplacePlugins` into a marketplace-refresh path and a config-only path

`installer.SyncMarketplacePlugins` (`internal/installer/plugins.go:116`) currently always calls
`addMarketplace` (refreshes the local marketplace cache — the actual "pull possibly-newer plugin
code" step) before looping `installMarketplacePlugin` per managed plugin with `--config` flags.

| Option | Tradeoff | Decision |
|---|---|---|
| Add a `skipMarketplaceRefresh bool` param to `SyncMarketplacePlugins` | Breaks the doc-comment-pinned "trailing-variadic, 1-2 arg call sites frozen" contract already established for `install.go`/`update.go` | Rejected |
| New exported `SyncMarketplacePluginConfigs(models, profile...)`, sharing a private loop helper with `SyncMarketplacePlugins`, that skips `addMarketplace` entirely | Zero risk to existing call sites; `plugin install` still runs against the ALREADY-cached marketplace metadata, so it re-applies `--config` flags without fetching newer plugin code | **Chosen** |

Rationale for the "no addMarketplace = no version bump" assumption: `addMarketplace` is the only
step that refreshes the marketplace's known commit; `installMarketplacePlugin` (`claude plugin
install <id>`) installs from whatever the marketplace cache already holds. This is inferred from
the existing Step-0-verified comments in `plugins.go`, not from a fresh CLI spike — flagged as an
Open Question.

### Decision 2: `click sync` never calls `MigrateIfStale`

`update.go` migrates a stale `models.json` (schema regeneration) before re-syncing. Sync's job is
narrower: re-apply config that's already valid, never mutate/migrate stored state. If
`models.json` is missing, sync falls back to `balanced + Defaults()` in memory (same fallback
`update.go` uses) but does NOT persist it — `click install`/`click update` remain the only
commands that write `models.json`.

### Decision 3: No Engram, no Context7, no `SaveModelsWithProfile` re-persist in sync

Per the literal scope (CLAUDE.md block, plugin configs, memory-guard hook only): `SyncEngram` and
`SyncContext7` are excluded — Engram pins a specific `manifest.Engram.Version`, which is a
version-management concern that belongs to `update`. Since sync never changes the in-memory
`models`/`profile`, re-persisting `models.json` is unnecessary and would be a no-op write; skipped
to keep sync's side effects minimal and auditable.

### Decision 4: New hidden cobra command `click sync`, no TUI, no TTY gate

Matches `update.go`'s own shape exactly (no bubbletea, no interactive prompt) — sync is
inherently non-interactive, so it needs no `isTerminalWriter` guard, unlike `install-preset`.

## Data Flow

    menu.go (Active item) ──Enter──▶ ActionSync ──▶ dispatch(["sync"])
         │
         ▼
    cli.runSync
         │  LoadModelsWithProfile(cfg) (fallback: balanced + Defaults(), NOT persisted)
         ▼
    installer.SyncMarketplacePluginConfigs(models, profile)   // NO addMarketplace
    installer.WriteManagedBlock(cfg.ClaudeMDPath(), DefaultManagedContent)
    installer.RegisterMemoryGuardHook(cfg)

## File Changes

| File | Action | Description |
|---|---|---|
| `internal/installer/plugins.go` | Modify | Extract private `installManagedPlugins(runner, resolved, profileName) error` loop from `SyncMarketplacePlugins`; add `SyncMarketplacePluginConfigs(models, profile...) error` that resolves + calls the loop WITHOUT `addMarketplace` |
| `internal/installer/plugins_test.go` | Modify | New tests: `SyncMarketplacePluginConfigs` never issues `plugin marketplace add`; still issues `plugin install <id> --config ...` per managed plugin |
| `internal/cli/sync.go` | Create | `newSyncCommand`, `runSync` |
| `internal/cli/sync_test.go` | Create | Asserts exact step order/output; asserts `SyncEngram`/`SyncContext7`/`MigrateIfStale` are NOT invoked (via a fake `CommandRunner` + isolated `Config` dirs, same harness as `install_test.go`) |
| `internal/cli/root.go` | Modify | Register `newSyncCommand()` |
| `internal/menu/menu.go` | Modify | Flip item `Active: true`, add `ActionSync` const, `ActionArgs` case → `{"sync"}` |
| `internal/menu/menu_test.go` | Modify | Extend `ActionArgs` table with the new case |

## Interfaces / Contracts

```go
// internal/installer/plugins.go
func SyncMarketplacePluginConfigs(models map[modelconfig.Phase]string,
    profile ...modelconfig.ProfileName) error
```

## Testing Strategy

| Layer | What to Test | Approach |
|---|---|---|
| Unit | `SyncMarketplacePluginConfigs` | Fake `CommandRunner` (`newFakeCommandRunner`, already exists); assert command list has NO `plugin marketplace add`, has 3× `plugin install ... --config` |
| Unit | `runSync` | Isolated temp `Config` dirs (existing `install_test.go` pattern); assert CLAUDE.md block written, memory-guard hook registered, models.json UNCHANGED |
| Integration | Idempotency | Run `runSync` twice; second run produces byte-identical CLAUDE.md/settings.json (both `WriteManagedBlock`/`RegisterMemoryGuardHook` are already idempotent by contract) |

## Migration / Rollout

No migration required. Purely additive; `SyncMarketplacePlugins`'s existing behavior and call
sites (`install.go`, `update.go`) are untouched.

## Open Questions

- [ ] Confirm against the real `claude` CLI (not just existing Step-0 comments) that `plugin
      install <id>@marketplace` without a prior `plugin marketplace add` truly never fetches a
      newer plugin version from an already-registered marketplace.
- [ ] Should `click sync`'s menu label/output warn the user that Engram/Context7 versions are
      intentionally left untouched (vs. silently doing less than `click update`)?
