package installer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStripCodexGuidance_LeavesEmptyFileWhenOnlyBlock(t *testing.T) {
	claudeHome := t.TempDir()
	codexHome := t.TempDir()
	cfg := Config{ClaudeHome: claudeHome, CodexHome: codexHome}

	// Write a file with ONLY the managed block
	if err := WriteManagedBlock(cfg.CodexAgentsMDPath(), DefaultCodexAgentsContent); err != nil {
		t.Fatalf("WriteManagedBlock() error = %v", err)
	}

	// Strip the block
	if err := StripCodexGuidance(cfg); err != nil {
		t.Fatalf("StripCodexGuidance() error = %v", err)
	}

	// NEW CONTRACT: file should still exist and be 0 bytes, NOT deleted
	info, err := os.Stat(cfg.CodexAgentsMDPath())
	if err != nil {
		t.Fatalf("Stat(CodexAgentsMDPath) after StripCodexGuidance error = %v, want file to exist (NEW contract: leave empty file)", err)
	}
	if info.Size() != 0 {
		t.Fatalf("Stat(CodexAgentsMDPath).Size = %d, want 0 (NEW contract: leave empty file, not delete it)", info.Size())
	}
}

func TestStripCodexGuidance_NoopWhenFileMissing(t *testing.T) {
	claudeHome := t.TempDir()
	codexHome := t.TempDir()
	cfg := Config{ClaudeHome: claudeHome, CodexHome: codexHome}

	// File doesn't exist, should be no-op with no error
	if err := StripCodexGuidance(cfg); err != nil {
		t.Fatalf("StripCodexGuidance() on missing file error = %v, want nil", err)
	}

	// Should not have created the file
	if _, err := os.Stat(cfg.CodexAgentsMDPath()); !os.IsNotExist(err) {
		t.Fatalf("Stat(CodexAgentsMDPath) after StripCodexGuidance on missing file = %v, want file to still not exist", err)
	}
}

func TestStripCodexGuidance_PreservesUserContentOutsideMarkers(t *testing.T) {
	claudeHome := t.TempDir()
	codexHome := t.TempDir()
	cfg := Config{ClaudeHome: claudeHome, CodexHome: codexHome}

	// Create file with user content before and after the managed block
	userBefore := "# My own Codex guidance\nUser content here.\n"
	userAfter := "# More user notes\nTrailing content.\n"
	path := cfg.CodexAgentsMDPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	// Build the full file content: userBefore + block + userAfter
	block := buildManagedBlock(DefaultCodexAgentsContent)
	fullContent := userBefore + joinLines(block) + userAfter
	if err := os.WriteFile(path, []byte(fullContent), 0o644); err != nil {
		t.Fatalf("WriteFile(fullContent) error = %v", err)
	}

	// Now strip the block
	if err := StripCodexGuidance(cfg); err != nil {
		t.Fatalf("StripCodexGuidance() error = %v", err)
	}

	// User content should be preserved
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile after StripCodexGuidance error = %v", err)
	}
	gotStr := string(got)
	if !strings.Contains(gotStr, userBefore) {
		t.Fatalf("StripCodexGuidance output = %q, want user content before markers preserved", gotStr)
	}
	if !strings.Contains(gotStr, userAfter) {
		t.Fatalf("StripCodexGuidance output = %q, want user content after markers preserved", gotStr)
	}
	if strings.Contains(gotStr, managedBeginMarker) || strings.Contains(gotStr, managedEndMarker) {
		t.Fatalf("StripCodexGuidance output = %q, want markers removed", gotStr)
	}
}
