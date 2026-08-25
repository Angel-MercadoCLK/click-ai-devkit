//go:build !windows

package installer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestApplyOwnerOnlySecurity_PosixMode0600(t *testing.T) {
	t.Setenv("CLICK_CLAUDE_HOME", t.TempDir())

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test-settings.json")

	// Create a test file with some content
	if err := os.WriteFile(testFile, []byte(`{"test": "data"}`), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Apply owner-only security
	if err := Apply(testFile); err != nil {
		t.Fatalf("Apply() failed: %v", err)
	}

	// Verify mode is 0600
	info, err := os.Stat(testFile)
	if err != nil {
		t.Fatalf("failed to stat file: %v", err)
	}
	if got := info.Mode().Perm(); got != 0600 {
		t.Errorf("Apply() resulted in mode %o, want 0600", got)
	}

	// Verify OwnerOnly reports true
	only, err := OwnerOnly(testFile)
	if err != nil {
		t.Fatalf("OwnerOnly() failed: %v", err)
	}
	if !only {
		t.Errorf("OwnerOnly() reported false, want true")
	}
}
