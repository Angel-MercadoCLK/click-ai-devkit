# Design: Gestionar backups (backup manager: list / restore / prune / pin)

> **Estado (2026-07-16):** este diseño completo NO se implementó todavía. En su lugar se
> publicó una versión mínima (`internal/cli/managebackups.go`, comando `click manage-backups`)
> que solo ve/restaura/elimina la única copia de seguridad existente hoy (`models.json.bak`,
> generada por `installer.MigrateIfStale`), sin interfaz interactiva ni deduplicación por
> contenido. Fue una decisión explícita: entregar algo real y de bajo riesgo ahora en vez de dejar
> el ítem del menú inactivo, sin descartar este diseño más completo para una iteración futura.
> Si se retoma este diseño, `internal/cli/managebackups.go` y su `ActionManageBackups` deberían
> reemplazarse o convivir con `internal/cli/backups.go`/`ActionBackups` según se decida entonces.

## Technical Approach

Introduce a content-addressed backup manager that snapshots click's two managed artifacts —
the CLAUDE.md managed block content and `models.json` — at each `install`/`update`, deduplicates
identical snapshots by SHA-256, retains the 5 most-recent unpinned snapshots (pinned exempt), and
exposes list/restore/pin/unpin/prune through a headless bubbletea TUI wired to the currently inert
menu item at `internal/menu/menu.go:54` via a hidden `click backups` cobra command.

This absorbs gentle-ai's backup improvement (content-hash dedup + keep-5 + pinning) and reuses
click's existing delimited managed-block writer (`installer.WriteManagedBlock`) for safe restore.

## Architecture Decisions

| Decision | Choice | Rejected alternative | Rationale |
|----------|--------|----------------------|-----------|
| Where backups live | New dir `<ClaudeHome>/click-ai-devkit/backups/` (sibling of `models.json`, `engram.json`) via new `Config.BackupsDir()` | Under `~/.claude` root; or reuse `.bak` suffix files | State dir already holds click-managed JSON (`ModelsPath`, `EngramStatePath`); keeps everything click-owned in one place, test-overridable via `CLICK_CLAUDE_HOME`. |
| Storage layout | `manifest.json` (ordered metadata list) + `blobs/<sha256>.json` (deduped payloads) | Whole-file `.bak` copies; one self-contained file per snapshot | Content-addressed blobs give dedup for free; manifest carries `pinned` flag + order without re-reading blobs. |
| Dedup granularity | Snapshot-level: `Hash = sha256(canonical-JSON(artifacts map))`; skip create if hash already in manifest | Per-artifact blobs | gentle-ai semantics = "identical snapshots not re-stored"; snapshot-level is simplest and matches the feature. |
| What is snapshotted | Two artifacts by logical name: `claude-md-block` (managed-block CONTENT only, between markers) and `models-json` (raw file bytes) | Whole CLAUDE.md file | Snapshotting only the managed block + restoring via `WriteManagedBlock` guarantees non-click content in CLAUDE.md is never touched. |
| Retention | `Prune(keep=5)`: delete oldest UNPINNED snapshots beyond the 5 newest unpinned; pinned always kept and never counted against the budget | Time-based expiry | Matches gentle-ai keep-5 + pinning exactly; deterministic and easy to TDD. |
| Capture points | Call `backup.Store.Create(...)` + `Prune(5)` at the START of `runInstall` and `runUpdate`, before any mutation of CLAUDE.md/`models.json` | Capture inside `WriteManagedBlock`/`SaveModels` | Keeps the domain writers pure; a single capture site per command is clearer and avoids double snapshots. Existing `models.json.bak` from `MigrateIfStale` stays untouched (out of scope) to bound blast radius. |
| Restore mechanism | Restore `claude-md-block` via `installer.WriteManagedBlock(cfg.ClaudeMDPath(), content)`; restore `models-json` by writing raw bytes to `cfg.ModelsPath()` | Overwrite whole files | Reuses the tested, idempotent block splicer so surrounding CLAUDE.md content is preserved byte-for-byte. |
| TUI location/shape | New `internal/ui/backupselect.go`, pure Model/Update/View exposing result fields (like `ModelSelectModel.Confirmed/Cancelled`); no I/O inside the model | Interactive prompts inside domain | Mirrors existing headless-testable TUI pattern; dispatch/TTY-gate lives in the cobra command. |
| Menu/command wiring | Add `ActionBackups="backups"`; flip the inert item Active; add `ActionArgs` case → `["backups"]`; register hidden `newBackupsCommand()` in `root.go` | Run TUI from inside menu Update | Identical to the proven `configure-models` dispatch contract (menu records action, fresh root re-dispatches). |

## Data Flow

    install/update ─► backup.Store.Create(snapshot) ─► dedup? ─► blobs/<hash>.json + manifest.json
                                                                        │
    menu "Gestionar backups" ─► click backups (TTY-gated) ─► ui.BackupListModel
                                                                        │ (restore/pin/unpin/prune)
                            ┌──────────────── restore ───────────────────┤
                            ▼                                             ▼
    WriteManagedBlock(CLAUDE.md)  +  write models.json          Pin/Unpin/Prune ─► manifest.json

## File Changes

| File | Action | Description |
|------|--------|-------------|
| `internal/backup/backup.go` | Create | Domain: `Snapshot`, `Store`, `NewStore`, `Create`, `List`, `Restore`, `Pin`, `Unpin`, `Prune`, SHA-256 canonical hashing, manifest+blob I/O. |
| `internal/backup/backup_test.go` | Create | Unit tests over `t.TempDir()`: dedup, keep-5, pinned-exempt prune, restore round-trip, ordering. |
| `internal/installer/config.go` | Modify | Add `BackupsDir()`, `BackupsManifestPath()`, `BackupBlobPath(hash)` accessors under `click-ai-devkit/backups`. |
| `internal/installer/claudemd.go` | Modify | Add exported `ReadManagedBlockContent(path) (content string, found bool, err error)` — returns the lines BETWEEN markers (exclusive), reusing `findMarkers`/`splitLines`/`joinLines`. |
| `internal/installer/claudemd_test.go` | Modify | Tests for `ReadManagedBlockContent` (present / absent / round-trips with `WriteManagedBlock`). |
| `internal/ui/backupselect.go` | Create | `BackupListModel` pure Model/Update/View; rows show timestamp, short hash, pin marker; keys r=restore, p=pin toggle, x=prune, q/esc cancel; exposes `Action`, `SelectedID`, `Cancelled`. |
| `internal/ui/backupselect_test.go` | Create | Headless key-driven tests (feed `tea.KeyMsg`), View assertions. |
| `internal/cli/backups.go` | Create | Hidden `newBackupsCommand()`, TTY gate (copy `configure-models` pattern), builds `backup.NewStore(cfg.BackupsDir())`, runs TUI, applies chosen action, restores via installer. |
| `internal/cli/backups_test.go` | Create | Non-TTY no-op message; action dispatch with injected fake TUI selector + fake store. |
| `internal/cli/root.go` | Modify | Add `newBackupsCommand()` to `root.AddCommand(...)`. |
| `internal/menu/menu.go` | Modify | Add `ActionBackups`; set `{Label:"Gestionar backups", Action:ActionBackups, Active:true}`; add `ActionArgs` case → `[]string{"backups"}`. |
| `internal/menu/menu_test.go` | Modify | Assert item active + `ActionArgs(ActionBackups)==["backups"]`. |
| `internal/cli/install.go` | Modify | At top of `runInstall`, capture snapshot + `Prune(5)` before mutations (best-effort; log-and-continue on error). |
| `internal/cli/update.go` | Modify | Same capture at top of `runUpdate`. |

## Interfaces / Contracts

```go
// internal/backup/backup.go
type Snapshot struct {
    ID        string            // sortable, e.g. RFC3339 "20060102T150405Z" + short hash
    CreatedAt time.Time
    Hash      string            // sha256 hex of canonical-JSON(Artifacts)
    Pinned    bool
    Artifacts map[string]string // logical name -> content; keys: "claude-md-block", "models-json"
}

type Store struct{ Dir string }
func NewStore(dir string) *Store
// Create dedups: returns created=false and the existing Snapshot if Hash already present.
func (s *Store) Create(artifacts map[string]string, now time.Time) (snap Snapshot, created bool, err error)
func (s *Store) List() ([]Snapshot, error)                 // newest first
func (s *Store) Restore(id string) (map[string]string, error)
func (s *Store) Pin(id string) error
func (s *Store) Unpin(id string) error
func (s *Store) Prune(keep int) (prunedIDs []string, err error) // pinned exempt, never counted

// internal/installer/claudemd.go (new)
func ReadManagedBlockContent(path string) (content string, found bool, err error)
```

Manifest shape: `{"schema_version":1,"snapshots":[{"id","created_at","hash","pinned"}...]}`. Blobs:
`blobs/<hash>.json` = canonical JSON of the `Artifacts` map.

## Testing Strategy

| Layer | What to Test | Approach |
|-------|-------------|----------|
| Unit (backup) | dedup skip, keep-5 prune, pinned exemption, restore round-trip, newest-first order, missing-dir bootstrap | `t.TempDir()`, injected `now time.Time` for deterministic IDs |
| Unit (installer) | `ReadManagedBlockContent` present/absent + `WriteManagedBlock` round-trip preserves outside content | table tests, temp files |
| Unit (ui) | cursor move, restore/pin/prune key → result fields, View shows pin marker + short hash | feed `tea.KeyMsg`, assert model + View |
| Unit (cli) | non-TTY prints no-terminal message and no-ops; each action applies correct store/installer call | inject fake selector + fake store (interface) |
| Unit (menu) | item active, `ActionArgs` mapping | existing menu test style |

## Migration / Rollout

No data migration. Backups dir is created lazily on first `Create`. Existing `models.json.bak`
(written by `MigrateIfStale`) is left as-is and is out of scope. Feature is additive; if the
backups dir is empty the TUI shows an empty-state line and all actions are no-ops.

## Open Questions

- [ ] Confirm snapshot ID format (RFC3339-compact vs ULID) — default to compact UTC timestamp + short hash.
- [ ] Should `uninstall` also capture a final snapshot before `StripManagedBlock`? Proposed: yes, low-risk add; mark optional.
