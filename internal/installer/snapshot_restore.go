package installer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DriftReport is SnapshotDrift's typed result: the two ownership-scoped tiers a rollback decision
// needs. Vetoes lists click-owned content that was hand-edited since the snapshot (refuse-by-
// default, spec install-rollback Decision 3); WarnableNonVeto lists present non-veto files whose
// content matches neither the pre-run backup nor the recorded post-run baseline — these never
// refuse a rollback, but the rollback command warns about them and asks for consent because a
// restore still overwrites them.
type DriftReport struct {
	Vetoes          []string
	WarnableNonVeto []string
}

// entryFingerprint captures one manifest entry's current on-disk state at a point in time, so a
// prepared restore can later verify nothing changed between prepare and apply (TOCTOU guard).
// present=false means the file is currently absent; contentHash/managedHash are then zero values.
type entryFingerprint struct {
	present     bool
	contentHash string
	managedHash string
}

// managedProjectionForEntry picks the managed-content projection for an entry exactly the way
// RecordSnapshotPostRun does (.md suffix -> markdown markers, settings.json -> owned-hook
// projection). It returns "" when the entry's file type has no managed projection at all.
func managedProjectionForEntry(entry manifestEntry, content string) string {
	if strings.HasSuffix(entry.OriginalPath, ".md") {
		return managedMarkdownProjectionHash(content)
	}
	if strings.HasSuffix(entry.OriginalPath, "settings.json") || entry.BackupFile == "settings.json" {
		return managedSettingsProjectionHash(content)
	}
	return ""
}

// fingerprintEntry reads the entry's current on-disk content and hashes it. A currently absent
// file is NOT an error: it yields the zero fingerprint (present=false), which drift classification
// treats as "nothing to veto or warn about".
func fingerprintEntry(entry manifestEntry) (entryFingerprint, error) {
	data, err := os.ReadFile(entry.OriginalPath)
	if err != nil {
		if os.IsNotExist(err) {
			return entryFingerprint{}, nil
		}
		return entryFingerprint{}, fmt.Errorf("installer: read %s to check drift: %w", entry.OriginalPath, err)
	}
	fp := entryFingerprint{present: true, contentHash: CanonicalContentHash(string(data))}
	if entry.DriftPolicy == DriftPolicyManagedContentVeto {
		fp.managedHash = managedProjectionForEntry(entry, string(data))
	}
	return fp, nil
}

// classifyEntryDrift applies the entry's DriftPolicy to decide whether the current on-disk state
// vetoes a rollback or is merely warnable. A currently absent file (fp.present == false) NEVER
// vetoes nor warns — restoring simply recreates the known-good content. An entry is safe whenever
// its current state matches EITHER the pre-run backup OR the recorded post-run baseline.
func classifyEntryDrift(entry manifestEntry, backupData []byte, fp entryFingerprint) (veto, warnable bool) {
	if !fp.present {
		return false, false
	}
	matchesWholeBaseline := fp.contentHash == CanonicalContentHash(string(backupData)) ||
		(entry.ExpectedPostRunHash != "" && fp.contentHash == entry.ExpectedPostRunHash)
	switch entry.DriftPolicy {
	case DriftPolicyNonVeto:
		return false, !matchesWholeBaseline
	case DriftPolicyManagedContentVeto:
		backupManaged := managedProjectionForEntry(entry, string(backupData))
		if backupManaged == "" || fp.managedHash == "" {
			// No managed projection exists for this file type: fall back to whole-file strictness
			// rather than silently skipping the veto.
			return !matchesWholeBaseline, false
		}
		matchesManagedBaseline := fp.managedHash == backupManaged ||
			(entry.ExpectedPostRunManagedHash != "" && fp.managedHash == entry.ExpectedPostRunManagedHash)
		return !matchesManagedBaseline, false
	default:
		// DriftPolicyWholeFileVeto (loadSnapshotManifest already normalized any empty policy to it).
		return !matchesWholeBaseline, false
	}
}

// preparedRestoreEntry is one manifest entry with everything ApplyPreparedRestore needs: the
// pre-run backup content (nil for no-prior-state markers) and the fingerprint captured at prepare
// time for the TOCTOU guard.
type preparedRestoreEntry struct {
	entry       manifestEntry
	backupData  []byte
	fingerprint entryFingerprint
}

// PreparedRestore is PrepareRestore's result: the drift report plus everything needed to actually
// restore, captured WITHOUT WRITING ANYTHING, so an interactive confirmation can sit between
// prepare and apply.
type PreparedRestore struct {
	Drift   DriftReport
	entries []preparedRestoreEntry
}

// PrepareRestore loads the snapshot manifest and backup contents and fingerprints every entry's
// current on-disk state, producing the ownership-scoped drift report (the same classification
// SnapshotDrift exposes) plus a ready-to-apply restore plan. It writes nothing.
//
// PrepareRestore assumes a manifest already exists; callers that need to distinguish "no snapshot
// to restore" from a real error should check HasRestorableSnapshot first — mirroring RestoreRun's
// own contract.
func PrepareRestore(cfg Config) (PreparedRestore, error) {
	manifest, err := loadSnapshotManifest(cfg)
	if err != nil {
		return PreparedRestore{}, err
	}

	latestDir := snapshotLatestDir(cfg)
	prepared := PreparedRestore{}
	for _, entry := range manifest.Entries {
		pe := preparedRestoreEntry{entry: entry}
		if entry.Existed {
			backupPath := filepath.Join(latestDir, entry.BackupFile)
			data, readErr := os.ReadFile(backupPath)
			if readErr != nil {
				return PreparedRestore{}, fmt.Errorf("installer: read snapshot backup %s: %w", backupPath, readErr)
			}
			pe.backupData = data
		}
		fp, fpErr := fingerprintEntry(entry)
		if fpErr != nil {
			return PreparedRestore{}, fpErr
		}
		pe.fingerprint = fp
		if entry.Existed {
			veto, warnable := classifyEntryDrift(entry, pe.backupData, fp)
			if veto {
				prepared.Drift.Vetoes = append(prepared.Drift.Vetoes, entry.OriginalPath)
			}
			if warnable {
				prepared.Drift.WarnableNonVeto = append(prepared.Drift.WarnableNonVeto, entry.OriginalPath)
			}
		} else if fp.present && entry.DriftPolicy == DriftPolicyNonVeto {
			// No-prior-state entry that has since appeared: for a policy click does not own outright,
			// deleting it (ApplyPreparedRestore's no-prior-state branch) could destroy real, legitimate
			// data an external tool wrote after the snapshot (review-risk finding). There is no baseline
			// to veto against — only a decision to delete or not — so this warns exactly like a
			// present-and-drifted non-veto entry, never vetoes. whole-file-veto/managed-content-veto
			// entries are deliberately NOT extended here: click owns that content outright, so deleting a
			// no-prior-state entry it owns is the correct, expected undo, not a foreign-data risk.
			prepared.Drift.WarnableNonVeto = append(prepared.Drift.WarnableNonVeto, entry.OriginalPath)
		}
		prepared.entries = append(prepared.entries, pe)
	}
	return prepared, nil
}

// ApplyPreparedRestore executes a prepared restore. Before ANY write it re-fingerprints every
// entry and refuses if anything changed since PrepareRestore ran (the TOCTOU guard: an
// interactive confirmation prompt may have sat in between). Restore COVERAGE is the full
// manifest — including non-veto entries (only VETO SCOPE is ownership-scoped): entries with
// backed-up content are copied back byte-for-byte; no-prior-state markers (Existed=false) remove
// any file that has since appeared at the original path, which for a DriftPolicyNonVeto entry is
// exactly what PrepareRestore's WarnableNonVeto classification (see there) puts in front of the
// rollback command's warning/consent flow first. The snapshot itself is left intact.
func ApplyPreparedRestore(prepared PreparedRestore) error {
	for _, pe := range prepared.entries {
		fp, err := fingerprintEntry(pe.entry)
		if err != nil {
			return err
		}
		if fp != pe.fingerprint {
			return fmt.Errorf("installer: %s changed since the restore was prepared; refusing to restore", pe.entry.OriginalPath)
		}
	}

	for _, pe := range prepared.entries {
		if !pe.entry.Existed {
			if removeErr := os.Remove(pe.entry.OriginalPath); removeErr != nil && !os.IsNotExist(removeErr) {
				return fmt.Errorf("installer: remove %s while restoring a no-prior-state entry: %w", pe.entry.OriginalPath, removeErr)
			}
			continue
		}
		if mkdirErr := os.MkdirAll(filepath.Dir(pe.entry.OriginalPath), 0o755); mkdirErr != nil {
			return fmt.Errorf("installer: create restore dir for %s: %w", pe.entry.OriginalPath, mkdirErr)
		}
		if writeErr := atomicWriteFile(pe.entry.OriginalPath, pe.backupData, 0o600); writeErr != nil {
			return fmt.Errorf("installer: restore %s: %w", pe.entry.OriginalPath, writeErr)
		}
	}
	return nil
}
