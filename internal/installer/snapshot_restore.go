package installer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DriftReport is SnapshotDrift's typed result for rollback. Vetoes lists eligible Click-owned
// content edited since the snapshot; WarnableNonVeto holds eligible entries that need a warning;
// Skipped lists shared or currently unqualified paths that are preserved and never written.
type DriftReport struct {
	Vetoes          []string
	WarnableNonVeto []string
	Skipped         []string
}

// entryFingerprint captures one manifest entry's current on-disk state at a point in time, so a
// prepared restore can later verify nothing changed between prepare and apply (TOCTOU guard).
// present=false means the file is currently absent; contentHash/managedHash are then zero values.
type entryFingerprint struct {
	present              bool
	contentHash          string
	rawHash              string
	managedHash          string
	settingsHashVariants [2]string
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

func fingerprintEntryData(entry manifestEntry) (entryFingerprint, []byte, error) {
	if err := checkRestorePath(entry.OriginalPath, true); err != nil {
		return entryFingerprint{}, nil, fmt.Errorf("installer: destino de restauración inseguro")
	}
	info, statErr := os.Lstat(entry.OriginalPath)
	if os.IsNotExist(statErr) {
		return entryFingerprint{}, nil, nil
	}
	if statErr != nil {
		return entryFingerprint{}, nil, fmt.Errorf("installer: no se pudo inspeccionar un destino de restauración")
	}
	if !info.Mode().IsRegular() {
		return entryFingerprint{}, nil, fmt.Errorf("installer: el destino de restauración no es un archivo regular")
	}
	data, err := os.ReadFile(entry.OriginalPath)
	if err != nil {
		return entryFingerprint{}, nil, fmt.Errorf("installer: no se pudo leer un destino de restauración")
	}
	fp := entryFingerprint{present: true, contentHash: CanonicalContentHash(string(data)), rawHash: rawContentHash(data)}
	if entry.DriftPolicy == DriftPolicyManagedContentVeto {
		fp.managedHash = managedProjectionForEntry(entry, string(data))
		if strings.HasSuffix(entry.OriginalPath, "settings.json") || entry.BackupFile == "settings.json" {
			fp.settingsHashVariants = [2]string{
				managedSettingsProjectionHashWithTokenPresence(string(data), false),
				managedSettingsProjectionHashWithTokenPresence(string(data), true),
			}
		}
	}
	return fp, data, nil
}

// classifyEntryDrift applies the entry's DriftPolicy to decide whether the current on-disk state
// vetoes a rollback or is merely warnable. A currently absent file (fp.present == false) NEVER
// vetoes nor warns — restoring simply recreates the known-good content. An entry is safe whenever
// its current state matches EITHER the pre-run backup OR the recorded post-run baseline.
func classifyEntryDrift(entry manifestEntry, backupData []byte, currentData []byte, fp entryFingerprint) (veto, warnable bool) {
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
		matchesManagedBaseline := fp.managedHash == backupManaged
		if strings.HasSuffix(entry.OriginalPath, "settings.json") || entry.BackupFile == "settings.json" {
			backupGuardHash := managedSettingsProjectionHashWithTokenPresence(string(backupData), false)
			currentGuardHash := managedSettingsProjectionHashWithTokenPresence(string(currentData), false)
			matchesManagedBaseline = currentGuardHash == backupGuardHash
			for _, variant := range fp.settingsHashVariants {
				if entry.ExpectedPostRunManagedHash != "" && variant == entry.ExpectedPostRunManagedHash {
					matchesManagedBaseline = true
				}
			}
		} else if entry.ExpectedPostRunManagedHash != "" && fp.managedHash == entry.ExpectedPostRunManagedHash {
			matchesManagedBaseline = true
		}
		return !matchesManagedBaseline, false
	default:
		// DriftPolicyWholeFileVeto (loadSnapshotManifest already normalized any empty policy to it).
		return !matchesWholeBaseline, false
	}
}

// preparedRestoreEntry is one eligible manifest entry with its validated backup, precomputed
// write/removal transformation, and destination fingerprint for apply-time TOCTOU checks.
type preparedRestoreEntry struct {
	entry       manifestEntry
	backupPath  string
	backupData  []byte
	backupHash  string
	currentData []byte
	fingerprint entryFingerprint
	ownership   restoreOwnership
	writeData   []byte
	write       bool
	remove      bool
}

// PreparedRestore is PrepareRestore's result: the drift report plus everything needed to actually
// restore, captured WITHOUT WRITING ANYTHING, so an interactive confirmation can sit between
// prepare and apply.
type PreparedRestore struct {
	Drift   DriftReport
	entries []preparedRestoreEntry
}

// PrepareRestore loads the snapshot manifest, selects exact owned destinations from the current
// Config, and precomputes safe transformations. Shared and unknown paths are reported as skipped;
// malformed managed content fails before any write. It writes nothing.
//
// PrepareRestore assumes a manifest already exists; callers that need to distinguish "no snapshot
// to restore" from a real error should check HasRestorableSnapshot first — mirroring RestoreRun's
// own contract.
func PrepareRestore(cfg Config) (PreparedRestore, error) {
	manifest, err := loadSnapshotManifest(cfg)
	if err != nil {
		return PreparedRestore{}, err
	}

	// Must resolve the same way loadSnapshotManifest just did, or a legacy-located manifest would
	// be paired with backup files looked up in the current (still empty) directory.
	latestDir := snapshotReadDir(cfg)
	prepared := PreparedRestore{}
	for _, entry := range manifest.Entries {
		if !isCanonicalAbsolutePath(entry.OriginalPath) {
			return PreparedRestore{}, fmt.Errorf("installer: una ruta del manifiesto de restauración no es canónica")
		}
		ownership, _ := classifyRestorePath(cfg, entry.OriginalPath)
		if ownership == restoreSkip {
			prepared.Drift.Skipped = append(prepared.Drift.Skipped, entry.OriginalPath)
			continue
		}
		if isRetiredOpenClawSkillPath(cfg, entry.OriginalPath) && !entry.Existed {
			prepared.Drift.Skipped = append(prepared.Drift.Skipped, entry.OriginalPath)
			continue
		}
		pe := preparedRestoreEntry{entry: entry, ownership: ownership}
		if err := checkRestorePath(entry.OriginalPath, true); err != nil {
			return PreparedRestore{}, fmt.Errorf("installer: destino de restauración inseguro")
		}
		if entry.Existed {
			if entry.BackupFile == "" || filepath.Clean(entry.BackupFile) != entry.BackupFile || filepath.Base(entry.BackupFile) != entry.BackupFile || entry.BackupFile == "." || entry.BackupFile == ".." {
				return PreparedRestore{}, fmt.Errorf("installer: nombre de respaldo inválido")
			}
			backupPath := filepath.Join(latestDir, entry.BackupFile)
			if err := checkRestorePath(backupPath, true); err != nil {
				return PreparedRestore{}, fmt.Errorf("installer: ruta de respaldo insegura")
			}
			data, readErr := os.ReadFile(backupPath)
			if readErr != nil {
				return PreparedRestore{}, fmt.Errorf("installer: no se pudo leer un respaldo necesario")
			}
			pe.backupPath = backupPath
			pe.backupData = data
			pe.backupHash = rawContentHash(data)
			if err := validateRetiredSkillBackup(cfg, entry, data); err != nil {
				return PreparedRestore{}, err
			}
		}
		fp, current, fpErr := fingerprintEntryData(entry)
		if fpErr != nil {
			return PreparedRestore{}, fpErr
		}
		pe.fingerprint = fp
		pe.currentData = current
		if isRetiredOpenClawSkillPath(cfg, entry.OriginalPath) && fp.present && !retiredSkillSignatureMatches(cfg, entry.OriginalPath, current) {
			prepared.Drift.Skipped = append(prepared.Drift.Skipped, entry.OriginalPath)
			continue
		}
		if !entry.Existed && ownership == restorePrivateFile && fp.present &&
			(entry.ExpectedPostRunHash == "" || fp.contentHash != entry.ExpectedPostRunHash) {
			prepared.Drift.Skipped = append(prepared.Drift.Skipped, entry.OriginalPath)
			continue
		}
		switch ownership {
		case restoreManagedMarkdown:
			pe.writeData, pe.write, pe.remove, fpErr = prepareMarkdownRestore(current, pe.backupData, entry.Existed)
		case restoreSettingsHooks:
			pe.writeData, pe.write, pe.remove, fpErr = prepareSettingsRestore(current, pe.backupData, entry.Existed)
		case restorePrivateFile:
			if entry.Existed {
				pe.writeData, pe.write = append([]byte(nil), pe.backupData...), true
			} else if fp.present {
				pe.remove = true
			}
		}
		if fpErr != nil {
			return PreparedRestore{}, fpErr
		}
		if entry.Existed {
			veto, warnable := classifyEntryDrift(entry, pe.backupData, current, fp)
			if veto {
				prepared.Drift.Vetoes = append(prepared.Drift.Vetoes, entry.OriginalPath)
			}
			if warnable {
				prepared.Drift.WarnableNonVeto = append(prepared.Drift.WarnableNonVeto, entry.OriginalPath)
			}
		} else if fp.present && entry.DriftPolicy == DriftPolicyNonVeto {
			// No-prior-state non-veto entry that has since appeared has no trustworthy baseline
			// proving that Click created its current contents. Warn rather than deleting it.
			prepared.Drift.WarnableNonVeto = append(prepared.Drift.WarnableNonVeto, entry.OriginalPath)
		}
		prepared.entries = append(prepared.entries, pe)
	}
	return prepared, nil
}

// ApplyPreparedRestore executes the prepared owned transformations. Before ANY write it verifies
// every eligible destination and backup is unchanged and safely addressed. The snapshot remains
// intact. Skipped shared or unknown paths are never re-read or mutated.
func ApplyPreparedRestore(prepared PreparedRestore) error {
	for _, pe := range prepared.entries {
		if err := checkRestorePath(pe.entry.OriginalPath, true); err != nil {
			return fmt.Errorf("installer: destino de restauración inseguro")
		}
		fp, _, err := fingerprintEntryData(pe.entry)
		if err != nil {
			return err
		}
		if fp != pe.fingerprint {
			return fmt.Errorf("installer: un archivo cambió desde la preparación; se canceló la restauración")
		}
		if pe.entry.Existed {
			if err := checkRestorePath(pe.backupPath, true); err != nil {
				return fmt.Errorf("installer: ruta de respaldo insegura")
			}
			backup, err := os.ReadFile(pe.backupPath)
			if err != nil || rawContentHash(backup) != pe.backupHash {
				return fmt.Errorf("installer: un respaldo cambió desde la preparación; se canceló la restauración")
			}
		}
	}

	for _, pe := range prepared.entries {
		if !pe.write && !pe.remove {
			continue
		}
		if err := checkRestorePath(pe.entry.OriginalPath, true); err != nil {
			return fmt.Errorf("installer: destino de restauración inseguro")
		}
		if pe.remove {
			if removeErr := os.Remove(pe.entry.OriginalPath); removeErr != nil && !os.IsNotExist(removeErr) {
				return fmt.Errorf("installer: no se pudo retirar contenido gestionado durante la restauración")
			}
			continue
		}
		if mkdirErr := os.MkdirAll(filepath.Dir(pe.entry.OriginalPath), 0o755); mkdirErr != nil {
			return fmt.Errorf("installer: no se pudo preparar el directorio de restauración")
		}
		if writeErr := atomicWriteFile(pe.entry.OriginalPath, pe.writeData, 0o600); writeErr != nil {
			return fmt.Errorf("installer: no se pudo restaurar contenido gestionado")
		}
	}
	return nil
}
