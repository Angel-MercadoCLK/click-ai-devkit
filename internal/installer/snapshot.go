package installer

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// snapshotManifestName is the fixed filename SnapshotRun writes inside BackupDir()/latest/,
// recording what each entry's original path was, where its copy landed, and whether the source
// existed at snapshot time.
const snapshotManifestName = "manifest.json"

// manifestEntry is one snapshotted file's record inside manifest.json. When Existed is false, the
// source file did not exist at snapshot time (spec Decision 1's "no-prior-state" case): BackupFile
// is then deliberately left empty — there is nothing to copy — and this explicit, structured
// Existed=false marker (never an empty/missing file, never an error) IS the no-prior-state marker
// the spec requires.
type manifestEntry struct {
	OriginalPath string `json:"originalPath"`
	BackupFile   string `json:"backupFile"`
	Existed      bool   `json:"existed"`
}

// runManifest is manifest.json's on-disk shape: one entry per file SnapshotRun/RestoreRun manage.
type runManifest struct {
	Entries []manifestEntry `json:"entries"`
}

// snapshotSource pairs a Config-resolved original path with the fixed filename its copy uses
// inside backups/latest/.
type snapshotSource struct {
	originalPath string
	backupFile   string
}

// snapshotSources returns the fixed set of files a run-start snapshot covers: CLAUDE.md and
// settings.json (design's Data Flow — the two root-level files `click install`/`click update`
// write to, ahead of any external `claude` subprocess invocation). Order is fixed so
// manifest.json's entry order is deterministic across runs.
func snapshotSources(cfg Config) []snapshotSource {
	return []snapshotSource{
		{originalPath: cfg.ClaudeMDPath(), backupFile: "CLAUDE.md"},
		{originalPath: cfg.SettingsPath(), backupFile: "settings.json"},
	}
}

// snapshotLatestDir is the single-latest-retention snapshot directory (design's "Retention"
// decision: fixed backups/latest/, overwritten each run — no ring, no history).
func snapshotLatestDir(cfg Config) string {
	return filepath.Join(cfg.BackupDir(), "latest")
}

// snapshotManifestPath is where manifest.json lives inside the latest snapshot directory.
func snapshotManifestPath(cfg Config) string {
	return filepath.Join(snapshotLatestDir(cfg), snapshotManifestName)
}

// SnapshotRun takes exactly one run-start snapshot of CLAUDE.md and settings.json, writing it to
// BackupDir()/latest/ plus a manifest.json describing each entry. It MUST be called before step 1
// of install/update and before any external `claude` CLI subprocess invocation (spec Requirement:
// Single Run-Start Snapshot Before Any Write) — that ordering is enforced by callers (PR2), not by
// this function itself.
//
// A missing source file is NOT an error: SnapshotRun records an explicit no-prior-state marker for
// it (Existed=false, no backup file) and continues (spec Decision 1).
//
// Last-known-good safety (spec Decision 2 / design's "Retention" decision): the new snapshot is
// built ENTIRELY in a temporary sibling directory under BackupDir() first. Only after every file
// copy and the manifest itself have been written successfully does SnapshotRun remove the previous
// backups/latest/ and rename the temporary directory into its place. Any failure before that final
// swap (e.g. a disk/write error injected via the createTempFile seam) leaves the prior completed
// snapshot in backups/latest/ completely untouched and unambiguously last-known-good — and never
// touches the original source files, which SnapshotRun only ever reads.
func SnapshotRun(cfg Config) error {
	backupDir := cfg.BackupDir()
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return fmt.Errorf("installer: create backup dir %s: %w", backupDir, err)
	}

	tmpDir, err := os.MkdirTemp(backupDir, ".latest-tmp-*")
	if err != nil {
		return fmt.Errorf("installer: create temporary snapshot dir: %w", err)
	}
	swapped := false
	defer func() {
		// Best-effort cleanup: once the rename below succeeds, tmpDir no longer exists under this
		// name, so this is a harmless no-op. On any earlier failure it removes the partially built
		// temp snapshot so it never accumulates and never gets mistaken for a real snapshot.
		if !swapped {
			_ = os.RemoveAll(tmpDir)
		}
	}()

	manifest := runManifest{}
	for _, src := range snapshotSources(cfg) {
		data, readErr := os.ReadFile(src.originalPath)
		if readErr != nil {
			if os.IsNotExist(readErr) {
				manifest.Entries = append(manifest.Entries, manifestEntry{
					OriginalPath: src.originalPath,
					BackupFile:   "",
					Existed:      false,
				})
				continue
			}
			return fmt.Errorf("installer: read %s for snapshot: %w", src.originalPath, readErr)
		}

		backupPath := filepath.Join(tmpDir, src.backupFile)
		if writeErr := atomicWriteFile(backupPath, data, 0o600); writeErr != nil {
			return fmt.Errorf("installer: write snapshot backup for %s: %w", src.originalPath, writeErr)
		}
		manifest.Entries = append(manifest.Entries, manifestEntry{
			OriginalPath: src.originalPath,
			BackupFile:   src.backupFile,
			Existed:      true,
		})
	}

	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("installer: marshal snapshot manifest: %w", err)
	}
	manifestData = append(manifestData, '\n')
	if err := atomicWriteFile(filepath.Join(tmpDir, snapshotManifestName), manifestData, 0o600); err != nil {
		return fmt.Errorf("installer: write snapshot manifest: %w", err)
	}

	// Every file copy and the manifest itself are now safely on disk under tmpDir. Only now do we
	// touch backups/latest/ at all — this is the sole point where the prior snapshot could be
	// affected, and it only runs after full success above.
	latestDir := snapshotLatestDir(cfg)
	if err := os.RemoveAll(latestDir); err != nil {
		return fmt.Errorf("installer: remove previous snapshot %s: %w", latestDir, err)
	}
	if err := os.Rename(tmpDir, latestDir); err != nil {
		return fmt.Errorf("installer: activate new snapshot at %s: %w", latestDir, err)
	}
	swapped = true
	return nil
}

// RestoreRun restores CLAUDE.md and settings.json to their last run-start snapshot (spec
// Requirement: Restore Last Run Snapshot). For each manifest entry: if the source existed at
// snapshot time, its backup content is copied back over the original path byte-for-byte; if it did
// NOT exist (a no-prior-state marker), any file that has since appeared at the original path is
// removed instead of being left in place or having content fabricated for it. The snapshot itself
// is left completely intact afterward (read+write, never a consuming move) so it can be restored
// from again later.
//
// RestoreRun assumes a manifest already exists; callers that need to distinguish "no snapshot to
// restore" from a real error should check HasRunSnapshot first (the rollback command, PR3, owns
// that user-facing distinction).
func RestoreRun(cfg Config) error {
	manifest, err := loadSnapshotManifest(cfg)
	if err != nil {
		return err
	}

	latestDir := snapshotLatestDir(cfg)
	for _, entry := range manifest.Entries {
		if !entry.Existed {
			if removeErr := os.Remove(entry.OriginalPath); removeErr != nil && !os.IsNotExist(removeErr) {
				return fmt.Errorf("installer: remove %s while restoring a no-prior-state entry: %w", entry.OriginalPath, removeErr)
			}
			continue
		}

		backupPath := filepath.Join(latestDir, entry.BackupFile)
		data, readErr := os.ReadFile(backupPath)
		if readErr != nil {
			return fmt.Errorf("installer: read snapshot backup %s: %w", backupPath, readErr)
		}
		if writeErr := atomicWriteFile(entry.OriginalPath, data, 0o600); writeErr != nil {
			return fmt.Errorf("installer: restore %s: %w", entry.OriginalPath, writeErr)
		}
	}
	return nil
}

// HasRunSnapshot reports whether a completed run-start snapshot exists: specifically, whether
// manifest.json is present under BackupDir()/latest/. This answers "did a snapshot run ever
// complete" — NOT "is there content to restore": a manifest whose entries are ALL no-prior-state
// markers (every source was absent at snapshot time) still means a real run completed, so
// HasRunSnapshot reports true for it too. Callers that need the finer "nothing to restore"
// distinction (spec's install-rollback "No snapshot exists" scenario) must inspect each manifest
// entry's Existed flag themselves — that is the future rollback command's (PR3) concern, not this
// function's.
func HasRunSnapshot(cfg Config) (bool, error) {
	_, err := os.Stat(snapshotManifestPath(cfg))
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("installer: check snapshot manifest: %w", err)
	}
	return true, nil
}

// loadSnapshotManifest reads and parses BackupDir()/latest/manifest.json.
func loadSnapshotManifest(cfg Config) (runManifest, error) {
	data, err := os.ReadFile(snapshotManifestPath(cfg))
	if err != nil {
		return runManifest{}, fmt.Errorf("installer: read snapshot manifest: %w", err)
	}
	var manifest runManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return runManifest{}, fmt.Errorf("installer: parse snapshot manifest: %w", err)
	}
	return manifest, nil
}

// canonicalContentHash returns the sha256 hex digest of content after canonicalizing line endings
// to LF via crlfAwareSplitLines/joinWithLineEnding (claudemd.go) — so a CRLF-saved file and an
// LF-saved file with the same logical content always hash identically. Extracted here (rather than
// duplicated) because BOTH PR3's rollback hand-edit drift check (spec install-rollback Decision 3,
// "refuse-by-default" when current content drifts from the snapshot's recorded hash) and PR4's
// doctor managed-block drift check (spec managed-block-integrity, design's "Drift hash" decision)
// need the exact same LF-canonicalization + hash algorithm, and must never be allowed to silently
// diverge from each other.
func canonicalContentHash(content string) string {
	canonical := joinWithLineEnding(crlfAwareSplitLines(content), "\n")
	sum := sha256.Sum256([]byte(canonical))
	return hex.EncodeToString(sum[:])
}
