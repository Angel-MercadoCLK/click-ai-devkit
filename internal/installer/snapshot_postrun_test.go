package installer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestRecordSnapshotPostRun tests the post-run snapshot recording.
func TestRecordSnapshotPostRun(t *testing.T) {
	// Test 1: records ExpectedPostRunHash for present files
	t.Run("records ExpectedPostRunHash for present files", func(t *testing.T) {
		tmpDir := t.TempDir()
		cfg := Config{
			ClickStateHome: tmpDir,
			ClaudeHome:     tmpDir,
		}

		// Create a file that will be snapshotted
		// Config uses ClaudeHome to derive ClaudeMDPath and SettingsPath
		testFile := filepath.Join(tmpDir, "CLAUDE.md")
		testContent := "test content\nwith lines\n"
		if err := os.WriteFile(testFile, []byte(testContent), 0o600); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		if err := SnapshotRun(cfg); err != nil {
			t.Fatalf("Failed to create snapshot: %v", err)
		}

		// Modify the file to simulate post-run state
		modifiedContent := "modified content\nwith different lines\n"
		if err := os.WriteFile(testFile, []byte(modifiedContent), 0o600); err != nil {
			t.Fatalf("Failed to modify test file: %v", err)
		}

		// Record post-run hashes
		if err := RecordSnapshotPostRun(cfg); err != nil {
			t.Fatalf("RecordSnapshotPostRun failed: %v", err)
		}

		// Read the manifest and verify ExpectedPostRunHash
		manifestPath := snapshotManifestPath(cfg)
		data, err := os.ReadFile(manifestPath)
		if err != nil {
			t.Fatalf("Failed to read manifest: %v", err)
		}

		var manifest runManifest
		if err := json.Unmarshal(data, &manifest); err != nil {
			t.Fatalf("Failed to parse manifest: %v", err)
		}

		if len(manifest.Entries) == 0 {
			t.Fatal("Manifest has no entries")
		}

		entry := manifest.Entries[0]
		if entry.ExpectedPostRunHash == "" {
			t.Error("ExpectedPostRunHash should be set")
		}

		expectedHash := CanonicalContentHash(modifiedContent)
		if entry.ExpectedPostRunHash != expectedHash {
			t.Errorf("ExpectedPostRunHash mismatch: got %s, want %s", entry.ExpectedPostRunHash, expectedHash)
		}
	})

	// Test 2: additionally records ExpectedPostRunManagedHash only for managed-content-veto entries
	t.Run("records ExpectedPostRunManagedHash for managed-content-veto entries", func(t *testing.T) {
		tmpDir := t.TempDir()
		cfg := Config{
			ClickStateHome: tmpDir,
			ClaudeHome:     tmpDir,
		}

		// Create a markdown file with managed block
		mdFile := filepath.Join(tmpDir, "CLAUDE.md")
		mdContent := "# >>> click-ai-devkit (managed) >>>\nmanaged content\n# <<< click-ai-devkit (managed) <<<\n"
		if err := os.WriteFile(mdFile, []byte(mdContent), 0o600); err != nil {
			t.Fatalf("Failed to create markdown file: %v", err)
		}

		// Create a settings.json with owned hook
		settingsFile := filepath.Join(tmpDir, "settings.json")
		settingsContent := `{"hooks":{"PreToolUse":[{"matcher":"` + MemoryGuardToolMatcher + `","hooks":[{"type":"command","command":"` + MemoryGuardCommand + `"}]}]}}`
		if err := os.WriteFile(settingsFile, []byte(settingsContent), 0o600); err != nil {
			t.Fatalf("Failed to create settings file: %v", err)
		}

		// Create snapshot with managed-content-veto policy
		if err := SnapshotRun(cfg); err != nil {
			t.Fatalf("Failed to create snapshot: %v", err)
		}

		// Record post-run hashes
		if err := RecordSnapshotPostRun(cfg); err != nil {
			t.Fatalf("RecordSnapshotPostRun failed: %v", err)
		}

		// Read manifest and verify managed hashes
		manifestPath := snapshotManifestPath(cfg)
		data, err := os.ReadFile(manifestPath)
		if err != nil {
			t.Fatalf("Failed to read manifest: %v", err)
		}

		var manifest runManifest
		if err := json.Unmarshal(data, &manifest); err != nil {
			t.Fatalf("Failed to parse manifest: %v", err)
		}

		// Find the markdown and settings entries
		var mdEntry, settingsEntry *manifestEntry
		for i := range manifest.Entries {
			if manifest.Entries[i].OriginalPath == mdFile {
				mdEntry = &manifest.Entries[i]
			} else if manifest.Entries[i].OriginalPath == settingsFile {
				settingsEntry = &manifest.Entries[i]
			}
		}

		if mdEntry == nil || settingsEntry == nil {
			t.Fatal("Expected to find both markdown and settings entries")
		}

		// Both should have ExpectedPostRunHash
		if mdEntry.ExpectedPostRunHash == "" || settingsEntry.ExpectedPostRunHash == "" {
			t.Error("Both entries should have ExpectedPostRunHash")
		}

		// Both should have ExpectedPostRunManagedHash (managed-content-veto policy)
		if mdEntry.ExpectedPostRunManagedHash == "" || settingsEntry.ExpectedPostRunManagedHash == "" {
			t.Error("Both entries should have ExpectedPostRunManagedHash for managed-content-veto")
		}

		// Verify the managed hashes match the projections
		expectedMdHash := managedMarkdownProjectionHash(mdContent)
		if mdEntry.ExpectedPostRunManagedHash != expectedMdHash {
			t.Errorf("Markdown managed hash mismatch: got %s, want %s", mdEntry.ExpectedPostRunManagedHash, expectedMdHash)
		}

		expectedSettingsHash := managedSettingsProjectionHash(settingsContent)
		if settingsEntry.ExpectedPostRunManagedHash != expectedSettingsHash {
			t.Errorf("Settings managed hash mismatch: got %s, want %s", settingsEntry.ExpectedPostRunManagedHash, expectedSettingsHash)
		}
	})

	// Test 3: leaves both post fields absent for currently-absent files
	t.Run("leaves post fields absent for absent files", func(t *testing.T) {
		tmpDir := t.TempDir()
		cfg := Config{
			ClickStateHome: tmpDir,
			ClaudeHome:     tmpDir,
		}

		// Create a snapshot with a file that doesn't exist (no-prior-state marker)
		var err error
		err = SnapshotRun(cfg)
		if err != nil {
			t.Fatalf("Failed to create snapshot: %v", err)
		}

		// Record post-run hashes (file still doesn't exist)
		if err := RecordSnapshotPostRun(cfg); err != nil {
			t.Fatalf("RecordSnapshotPostRun failed: %v", err)
		}

		// Read manifest and verify fields are still unset
		manifestPath := snapshotManifestPath(cfg)
		data, err := os.ReadFile(manifestPath)
		if err != nil {
			t.Fatalf("Failed to read manifest: %v", err)
		}

		var manifest runManifest
		if err := json.Unmarshal(data, &manifest); err != nil {
			t.Fatalf("Failed to parse manifest: %v", err)
		}

		if len(manifest.Entries) == 0 {
			t.Fatal("Manifest has no entries")
		}

		entry := manifest.Entries[0]
		if entry.ExpectedPostRunHash != "" || entry.ExpectedPostRunManagedHash != "" {
			t.Errorf("Post fields should be unset for absent files, got hash=%s managedHash=%s",
				entry.ExpectedPostRunHash, entry.ExpectedPostRunManagedHash)
		}
	})

	// Test 4: only manifest.json is rewritten (backup files untouched)
	t.Run("only rewrites manifest, not backup files", func(t *testing.T) {
		tmpDir := t.TempDir()
		cfg := Config{
			ClickStateHome: tmpDir,
			ClaudeHome:     tmpDir,
		}

		// Create and snapshot a file
		testFile := filepath.Join(tmpDir, "CLAUDE.md")
		testContent := "original content\n"
		if err := os.WriteFile(testFile, []byte(testContent), 0o600); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		if err := SnapshotRun(cfg); err != nil {
			t.Fatalf("Failed to create snapshot: %v", err)
		}

		// Record the backup file's mtime and content hash
		backupDir := snapshotLatestDir(cfg)
		backupFiles, err := os.ReadDir(backupDir)
		if err != nil {
			t.Fatalf("Failed to read backup dir: %v", err)
		}

		var backupPath string
		for _, bf := range backupFiles {
			if bf.Name() != snapshotManifestName {
				backupPath = filepath.Join(backupDir, bf.Name())
				break
			}
		}

		if backupPath == "" {
			t.Fatal("No backup file found")
		}

		backupInfo, err := os.Stat(backupPath)
		if err != nil {
			t.Fatalf("Failed to stat backup file: %v", err)
		}
		backupMtime := backupInfo.ModTime()

		backupData, err := os.ReadFile(backupPath)
		if err != nil {
			t.Fatalf("Failed to read backup file: %v", err)
		}
		backupHash := CanonicalContentHash(string(backupData))

		// Record post-run hashes
		if err := RecordSnapshotPostRun(cfg); err != nil {
			t.Fatalf("RecordSnapshotPostRun failed: %v", err)
		}

		// Verify backup file is unchanged
		backupInfoAfter, err := os.Stat(backupPath)
		if err != nil {
			t.Fatalf("Failed to stat backup file after: %v", err)
		}

		if backupInfoAfter.ModTime() != backupMtime {
			t.Error("Backup file mtime should not change")
		}

		backupDataAfter, err := os.ReadFile(backupPath)
		if err != nil {
			t.Fatalf("Failed to read backup file after: %v", err)
		}

		if CanonicalContentHash(string(backupDataAfter)) != backupHash {
			t.Error("Backup file content should not change")
		}

		// Verify no new generation directory was created
		parentDir := filepath.Dir(backupDir)
		entries, err := os.ReadDir(parentDir)
		if err != nil {
			t.Fatalf("Failed to read parent dir: %v", err)
		}

		genCount := 0
		for _, e := range entries {
			if e.IsDir() && e.Name() != "latest" {
				genCount++
			}
		}

		if genCount > 0 {
			t.Errorf("No new generation directory should be created, found %d", genCount)
		}
	})

	// Test 5: injected write failure leaves previous manifest valid
	t.Run("write failure leaves previous manifest valid", func(t *testing.T) {
		tmpDir := t.TempDir()
		cfg := Config{
			ClickStateHome: tmpDir,
			ClaudeHome:     tmpDir,
		}

		// Create and snapshot a file
		testFile := filepath.Join(tmpDir, "CLAUDE.md")
		testContent := "test content\n"
		if err := os.WriteFile(testFile, []byte(testContent), 0o600); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		if err := SnapshotRun(cfg); err != nil {
			t.Fatalf("Failed to create snapshot: %v", err)
		}

		// Record the original manifest content
		manifestPath := snapshotManifestPath(cfg)
		originalData, err := os.ReadFile(manifestPath)
		if err != nil {
			t.Fatalf("Failed to read original manifest: %v", err)
		}
		originalHash := CanonicalContentHash(string(originalData))

		// Inject write failure via createTempFile seam
		restore := SetCreateTempFileForTests(func(dir, pattern string) (tempFileWriter, error) {
			// Return a writer that always fails on Write
			return &failingWriter{}, nil
		})
		defer restore()

		// Attempt to record post-run hashes (should fail)
		err = RecordSnapshotPostRun(cfg)
		if err == nil {
			t.Error("Expected write failure, got nil")
		}

		// Verify original manifest is still valid and unchanged
		dataAfter, err := os.ReadFile(manifestPath)
		if err != nil {
			t.Fatalf("Failed to read manifest after: %v", err)
		}

		if CanonicalContentHash(string(dataAfter)) != originalHash {
			t.Error("Original manifest should be unchanged after write failure")
		}

		// Verify manifest is still parseable
		var manifest runManifest
		if err := json.Unmarshal(dataAfter, &manifest); err != nil {
			t.Fatalf("Manifest should still be parseable after write failure: %v", err)
		}
	})

	// Test 6: calling with no manifest present returns a defined error
	t.Run("no manifest returns defined error", func(t *testing.T) {
		tmpDir := t.TempDir()
		cfg := Config{
			ClickStateHome: tmpDir,
			ClaudeHome:     tmpDir,
		}

		// Don't create any snapshot
		err := RecordSnapshotPostRun(cfg)
		if err == nil {
			t.Error("Expected error when no manifest exists")
		}

		// Verify it's a defined error (not nil)
		if err == nil {
			t.Fatal("Error should not be nil")
		}
	})
}

// failingWriter is a tempFileWriter that always fails on Write.
type failingWriter struct{}

func (f *failingWriter) Write(p []byte) (int, error) {
	return 0, os.ErrClosed
}

func (f *failingWriter) Sync() error {
	return os.ErrClosed
}

func (f *failingWriter) Close() error {
	return os.ErrClosed
}

func (f *failingWriter) Name() string {
	return "failing.tmp"
}

// SetCreateTempFileForTests sets a custom createTempFile function for testing.
func SetCreateTempFileForTests(fn func(dir, pattern string) (tempFileWriter, error)) func() {
	old := createTempFile
	createTempFile = func(dir, pattern string) (tempFileWriter, error) {
		return fn(dir, pattern)
	}
	return func() { createTempFile = old }
}
