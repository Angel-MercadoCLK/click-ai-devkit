package installer

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
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

// TestAtomicWriteFile_PreservesCallerRequestedMode proves that atomicWriteFile
// respects the mode requested by the caller. Regression test for slice 2 over-broad
// security application where settingsSecurityFactory was called unconditionally in
// the shared helper, silently overriding all callers' requested modes.
//
// Skipped on Windows because Windows does not use POSIX file modes; the
// security primitive on Windows applies a protected DACL, not a mode change.
func TestAtomicWriteFile_PreservesCallerRequestedMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on Windows: POSIX file modes are not applicable")
	}
	tmpDir := t.TempDir()
	targetPath := filepath.Join(tmpDir, "testfile.txt")
	content := []byte("test content")
	requestedMode := os.FileMode(0o644)

	if err := atomicWriteFile(targetPath, content, requestedMode); err != nil {
		t.Fatalf("atomicWriteFile failed: %v", err)
	}

	// Verify the file has the requested mode
	info, err := os.Stat(targetPath)
	if err != nil {
		t.Fatalf("failed to stat file: %v", err)
	}

	if info.Mode().Perm() != requestedMode {
		t.Errorf("atomicWriteFile did not preserve caller requested mode: got %o, want %o", info.Mode().Perm(), requestedMode)
	}
}

// TestAtomicWriteFile_DoesNotApplyOwnerOnlySecurity proves that a plain
// atomicWriteFile call does NOT invoke settingsSecurityFactory. Regression test
// for slice 2 over-broad security application where the shared helper was
// applying owner-only security to ALL callers, not just settings writes.
func TestAtomicWriteFile_DoesNotApplyOwnerOnlySecurity(t *testing.T) {
	tmpDir := t.TempDir()
	targetPath := filepath.Join(tmpDir, "testfile.txt")
	content := []byte("test content")

	// Install a recording security factory that fails if called
	securityFactoryCalled := false
	restore := SetSettingsSecurityFactoryForTests(func(path string) error {
		securityFactoryCalled = true
		return errors.New("security factory should not be called by plain atomicWriteFile")
	})
	defer restore()

	if err := atomicWriteFile(targetPath, content, 0o644); err != nil {
		t.Fatalf("atomicWriteFile failed: %v", err)
	}

	// The security factory must NOT have been called
	if securityFactoryCalled {
		t.Error("atomicWriteFile called settingsSecurityFactory when it should not have")
	}
}

// TestWriteSettingsFile_AppliesOwnerOnlySecurity proves that writeSettingsFile
// DOES invoke settingsSecurityFactory exactly once. This confirms the security
// primitive is wired correctly to the settings path only.
func TestWriteSettingsFile_AppliesOwnerOnlySecurity(t *testing.T) {
	t.Setenv("CLICK_CLAUDE_HOME", t.TempDir())

	tmpDir := t.TempDir()
	settingsPath := filepath.Join(tmpDir, "settings.json")
	settings := map[string]any{"test": "data"}

	// Install a recording security factory
	securityFactoryCallCount := 0
	restore := SetSettingsSecurityFactoryForTests(func(path string) error {
		securityFactoryCallCount++
		return nil // succeed, but record the call
	})
	defer restore()

	if err := writeSettingsFile(settingsPath, settings); err != nil {
		t.Fatalf("writeSettingsFile failed: %v", err)
	}

	// The security factory MUST have been called exactly once
	if securityFactoryCallCount != 1 {
		t.Errorf("writeSettingsFile called settingsSecurityFactory %d times, want exactly 1", securityFactoryCallCount)
	}
}
