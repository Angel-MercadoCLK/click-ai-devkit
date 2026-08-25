//go:build windows

package installer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestApplyOwnerOnlySecurity_ProtectedDACL(t *testing.T) {
	t.Setenv("CLICK_CLAUDE_HOME", t.TempDir())

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test-settings.json")

	if err := os.WriteFile(testFile, []byte(`{"test": "data"}`), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	if err := Apply(testFile); err != nil {
		t.Fatalf("Apply() failed: %v", err)
	}

	only, err := OwnerOnly(testFile)
	if err != nil {
		t.Fatalf("OwnerOnly() failed: %v", err)
	}
	if !only {
		t.Errorf("OwnerOnly() reported false, want true")
	}
}
