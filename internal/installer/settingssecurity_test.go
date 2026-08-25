package installer

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- Settings security preservation tests (slice 2) ---
//
// writeSettingsFile must apply owner-only security to the temp file BEFORE rename.
// If security application fails, the original file's bytes must survive unchanged
// and NO rename must happen.

// TestWriteSettingsFile_SecurityFailureBeforeRenamePreservesOriginal proves that
// a failing settingsSecurityFactory leaves the original file untouched and no
// rename occurs.
func TestWriteSettingsFile_SecurityFailureBeforeRenamePreservesOriginal(t *testing.T) {
	t.Setenv("CLICK_CLAUDE_HOME", t.TempDir())

	tmpDir := t.TempDir()
	settingsPath := filepath.Join(tmpDir, "settings.json")

	// Write initial content
	initialContent := map[string]any{"initial": "data"}
	initialBytes, _ := json.MarshalIndent(initialContent, "", "  ")
	initialBytes = append(initialBytes, '\n')
	if err := os.WriteFile(settingsPath, initialBytes, 0644); err != nil {
		t.Fatalf("failed to create initial file: %v", err)
	}

	// Capture initial file info for comparison
	initialInfo, err := os.Stat(settingsPath)
	if err != nil {
		t.Fatalf("failed to stat initial file: %v", err)
	}

	// Inject a failing security factory
	originalFactory := settingsSecurityFactory
	settingsSecurityFactory = func(string) error {
		return errors.New("injected security failure")
	}
	t.Cleanup(func() {
		settingsSecurityFactory = originalFactory
	})

	// Attempt to write new settings
	newContent := map[string]any{"new": "data"}
	err = writeSettingsFile(settingsPath, newContent)

	// Verify the write failed
	if err == nil {
		t.Error("writeSettingsFile should have failed but succeeded")
	} else if !strings.Contains(err.Error(), "installer: write settings") {
		t.Errorf("writeSettingsFile error should wrap the security failure, got: %v", err)
	}

	// Verify the original file still exists and has unchanged content
	currentBytes, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("failed to read file after failed write: %v", err)
	}
	if !bytes.Equal(currentBytes, initialBytes) {
		t.Errorf("original file bytes changed, wanted:\n%s\ngot:\n%s", initialBytes, currentBytes)
	}

	// Verify no rename happened (file mtime unchanged)
	currentInfo, err := os.Stat(settingsPath)
	if err != nil {
		t.Fatalf("failed to stat file after failed write: %v", err)
	}
	if !currentInfo.ModTime().Equal(initialInfo.ModTime()) {
		t.Error("file modification time changed, suggesting a rename occurred")
	}
}

// TestWriteSettingsFile_OwnerOnlyPermissionsAfterWrite proves that a successful
// writeSettingsFile leaves the file owner-only (0600 on POSIX, OwnerOnly true on Windows).
func TestWriteSettingsFile_OwnerOnlyPermissionsAfterWrite(t *testing.T) {
	t.Setenv("CLICK_CLAUDE_HOME", t.TempDir())

	tmpDir := t.TempDir()
	settingsPath := filepath.Join(tmpDir, "settings.json")

	// Write settings
	settings := map[string]any{"test": "data"}
	if err := writeSettingsFile(settingsPath, settings); err != nil {
		t.Fatalf("writeSettingsFile failed: %v", err)
	}

	// Verify owner-only
	only, err := OwnerOnly(settingsPath)
	if err != nil {
		t.Fatalf("OwnerOnly() failed: %v", err)
	}
	if !only {
		t.Error("writeSettingsFile did not result in owner-only permissions")
	}
}
