package installer

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// RecordSnapshotPostRun records post-run content hashes in the active snapshot manifest.
// It reads the current content of each snapshotted file, computes hashes, and atomically
// updates the manifest file only. Backup files are never touched, and no new snapshot
// generation directory is created.
//
// For entries that currently exist, it sets ExpectedPostRunHash to the canonical hash
// of the current raw content. If the entry's DriftPolicy is DriftPolicyManagedContentVeto,
// it also sets ExpectedPostRunManagedHash using the appropriate projection (markdown
// for .md files, settings for settings.json).
//
// For entries that are currently absent, both post-run fields remain unset.
//
// On any read or write failure, it returns an error — the caller is responsible for
// treating this as non-fatal (e.g., converting to a warning).
func RecordSnapshotPostRun(cfg Config) error {
	// Load the active manifest
	manifest, err := loadSnapshotManifest(cfg)
	if err != nil {
		return fmt.Errorf("installer: load manifest for post-run recording: %w", err)
	}

	// For each entry, record the current content hashes
	for i := range manifest.Entries {
		entry := &manifest.Entries[i]

		// Skip entries that didn't exist at snapshot time
		if !entry.Existed {
			continue
		}

		// Read the current content
		currentData, err := os.ReadFile(entry.OriginalPath)
		if err != nil {
			if os.IsNotExist(err) {
				// File no longer exists - leave post fields unset and continue
				continue
			}
			return fmt.Errorf("installer: read %s for post-run recording: %w", entry.OriginalPath, err)
		}

		// Set ExpectedPostRunHash for all present files
		entry.ExpectedPostRunHash = CanonicalContentHash(string(currentData))

		// For managed-content-veto entries, also set ExpectedPostRunManagedHash
		if entry.DriftPolicy == DriftPolicyManagedContentVeto {
			// Determine which projection to use based on file type
			var managedHash string
			if strings.HasSuffix(entry.OriginalPath, ".md") {
				managedHash = managedMarkdownProjectionHash(string(currentData))
			} else if strings.HasSuffix(entry.OriginalPath, "settings.json") {
				managedHash = managedSettingsProjectionHash(string(currentData))
			} else if entry.BackupFile == "settings.json" {
				// Fallback: check backup file name for settings files
				managedHash = managedSettingsProjectionHash(string(currentData))
			}

			if managedHash != "" {
				entry.ExpectedPostRunManagedHash = managedHash
			}
		}
	}

	// Atomically rewrite only the manifest file
	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("installer: marshal post-run manifest: %w", err)
	}
	manifestData = append(manifestData, '\n')

	manifestPath := snapshotManifestPath(cfg)
	if err := atomicWriteFile(manifestPath, manifestData, 0o600); err != nil {
		return fmt.Errorf("installer: write post-run manifest: %w", err)
	}

	return nil
}
