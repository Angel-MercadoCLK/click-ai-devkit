# Tasks: Gestionar backups

STRICT TDD — every code task is RED (write failing test) → GREEN (minimal impl) → keep refactors
separate. All tests use `t.TempDir()` and the `CLICK_CLAUDE_HOME` override; never touch real `~/.claude`.
Artifacts in English; TUI/CLI user strings in Spanish (D10). Ordered so each slice is independently shippable.

## Slice 1 — Backup domain package (`internal/backup`)

1. [ ] RED: `backup_test.go` — `Create` writes a blob + manifest entry; `List` returns it. GREEN: `NewStore`, `Create`, `List`, manifest+blob I/O, SHA-256 canonical hash.
2. [ ] RED: dedup — second `Create` with identical artifacts returns `created=false` and no new blob/manifest entry. GREEN: hash lookup in manifest before write.
3. [ ] RED: `Restore(id)` returns the exact artifacts map stored. GREEN: read blob by hash.
4. [ ] RED: `Pin`/`Unpin` flip the manifest flag; unknown id errors. GREEN: manifest mutate + persist.
5. [ ] RED: `Prune(5)` deletes oldest UNPINNED beyond 5 newest unpinned, keeps all pinned, deletes orphan blobs. GREEN: sort newest-first, partition pinned, delete surplus.
6. [ ] REFACTOR: extract canonical-JSON + manifest read/write helpers; keep coverage green.

## Slice 2 — Installer accessors + managed-block read + capture hooks

7. [ ] RED: `config_test.go` — `BackupsDir()/BackupsManifestPath()/BackupBlobPath` resolve under `click-ai-devkit/backups`. GREEN: add accessors to `config.go`.
8. [ ] RED: `claudemd_test.go` — `ReadManagedBlockContent` returns content between markers (absent→found=false); round-trips with `WriteManagedBlock`. GREEN: add function using `findMarkers`.
9. [ ] RED: `install`/`update` capture — a snapshot exists after `runInstall`/`runUpdate` with a pre-existing CLAUDE.md block + models.json. GREEN: capture + `Prune(5)` at top of `runInstall`/`runUpdate` (best-effort, log-and-continue).

## Slice 3 — Backup TUI (`internal/ui/backupselect.go`)

10. [ ] RED: `NewBackupListModel(snaps)` renders one row per snapshot with timestamp, short hash, pin marker; empty-state line when none. GREEN: Model + View.
11. [ ] RED: keys — j/k move; r/enter → `Action=restore`+`SelectedID`+quit; p → `Action=pin-toggle`; x → `Action=prune`; q/esc → `Cancelled`. GREEN: Update.
12. [ ] REFACTOR: align key-help Spanish line with existing screens.

## Slice 4 — CLI command + menu wiring

13. [ ] RED: `backups_test.go` — non-TTY prints no-terminal message, no store mutation. GREEN: `newBackupsCommand()` with TTY gate (copy `configure-models`).
14. [ ] RED: each TUI action applies the right effect (restore→`WriteManagedBlock`+write models.json; pin/unpin/prune→store) via injected fake selector + fake store. GREEN: dispatch logic.
15. [ ] RED: register in `root.go`; `menu_test.go` — item Active + `ActionArgs(ActionBackups)==["backups"]`. GREEN: edit `menu.go` (`ActionBackups`, Item, `ActionArgs`) + `root.go` `AddCommand`.
16. [ ] REFACTOR + full `go test ./...`; `go vet`.

## Review Workload Forecast

- Estimated changed lines: ~520 (4 new files + tests + 6 edits). **>400 budget.**
- Decision needed before apply: Yes
- Chained PRs recommended: Yes
- 400-line budget risk: High
- Recommended stacked slices (each shippable, self-verifying): PR1=Slice 1 (domain, ~180 LOC), PR2=Slice 2 (accessors+capture, ~90 LOC), PR3=Slice 3 (TUI, ~150 LOC), PR4=Slice 4 (cli+menu wiring, ~100 LOC). PR4 depends on PR1–PR3; PR2 depends on PR1.

## Acceptance Criteria

- [ ] Identical consecutive snapshots are stored once (dedup verified by blob count).
- [ ] After 6+ distinct unpinned snapshots, only 5 remain; a pinned one survives beyond keep-5.
- [ ] Restore of a `claude-md-block` snapshot leaves non-click CLAUDE.md content byte-for-byte intact.
- [ ] `click backups` on a non-TTY prints the Spanish no-terminal notice and mutates nothing.
- [ ] Menu "Gestionar backups" is active and dispatches `click backups`.
- [ ] `go test ./...` green; lockstep/menu tests unaffected.
- [ ] MANUAL: verify the TUI interactively on a real terminal (restore/pin/prune round-trip) — headless tests cannot drive a live bubbletea program.
