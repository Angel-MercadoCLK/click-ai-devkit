package installer

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- Symlink / atomic-write regression coverage (Finding 2) ---
//
// writeSettingsFile and writeJSONFile used to write via plain os.WriteFile, which is non-atomic
// (a crash mid-write leaves a truncated/corrupted file) and inconsistent with this package's own
// existing solution (atomicWriteFile/resolveWriteTarget in pathenv.go), added specifically to avoid
// de-symlinking dotfiles-managed rc files. A developer managing ~/.claude/ with a dotfiles repo
// (chezmoi/GNU-stow/dotbot/yadm) commonly has settings.json symlinked into their dotfiles checkout;
// the tests below reuse the SAME requireSymlinkSupport/fakeFailingTempFile fixtures pathenv_test.go
// already established for atomicWriteFile itself, applied here to prove the two JSON-writing
// helpers are now wired through it instead of a second parallel implementation.

// TestWriteSettingsFile_WritesThroughSymlinkPreservingIt proves a symlinked settings.json is
// written through to its real target, leaving the symlink itself undisturbed.
func TestWriteSettingsFile_WritesThroughSymlinkPreservingIt(t *testing.T) {
	requireSymlinkSupport(t)

	root := t.TempDir()
	realDir := filepath.Join(root, "real")
	linkDir := filepath.Join(root, "linked")
	if err := os.MkdirAll(realDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(realDir) error = %v", err)
	}
	if err := os.MkdirAll(linkDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(linkDir) error = %v", err)
	}

	realTarget := filepath.Join(realDir, "settings.json")
	if err := os.WriteFile(realTarget, []byte(`{"existing":true}`+"\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(realTarget) error = %v", err)
	}

	symlinkPath := filepath.Join(linkDir, "settings.json")
	if err := os.Symlink(realTarget, symlinkPath); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}

	if err := writeSettingsFile(symlinkPath, map[string]any{"hooks": "configured"}); err != nil {
		t.Fatalf("writeSettingsFile() error = %v", err)
	}

	info, err := os.Lstat(symlinkPath)
	if err != nil {
		t.Fatalf("Lstat(symlinkPath) error = %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("symlinkPath is no longer a symlink after writeSettingsFile() (mode = %v) — it was destructively de-symlinked", info.Mode())
	}

	data, err := os.ReadFile(realTarget)
	if err != nil {
		t.Fatalf("ReadFile(realTarget) error = %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("json.Unmarshal(realTarget content) error = %v, content = %s", err, data)
	}
	if got["hooks"] != "configured" {
		t.Fatalf("realTarget content = %s, want the new settings written through the symlink", data)
	}
}

// TestWriteSettingsFile_InjectedWriteErrorLeavesOriginalIntact is the strict-TDD RED/GREEN proof
// that writeSettingsFile now goes through atomicWriteFile's temp-file+rename path: injecting a
// failing createTempFile must surface an error AND leave the original file byte-for-byte untouched.
// Against the old direct os.WriteFile implementation this injection is a no-op (createTempFile is
// never consulted), so the write silently succeeds and this test fails.
func TestWriteSettingsFile_InjectedWriteErrorLeavesOriginalIntact(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	original := []byte(`{"original":true}` + "\n")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatalf("seed WriteFile() error = %v", err)
	}

	injectedErr := errors.New("injected write failure")
	old := createTempFile
	createTempFile = func(dir, pattern string) (tempFileWriter, error) {
		return &fakeFailingTempFile{name: filepath.Join(dir, ".click-injected-fake"), writeErr: injectedErr}, nil
	}
	defer func() { createTempFile = old }()

	err := writeSettingsFile(path, map[string]any{"new": true})
	if err == nil {
		t.Fatal("writeSettingsFile() error = nil, want the injected write error to propagate")
	}

	got, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("ReadFile(path) error = %v", readErr)
	}
	if string(got) != string(original) {
		t.Fatalf("file content = %q after a failed write, want untouched original %q", got, original)
	}
}

// TestWriteJSONFile_WritesThroughSymlinkPreservingIt mirrors
// TestWriteSettingsFile_WritesThroughSymlinkPreservingIt for writeJSONFile, the shared helper this
// package's engram.go/plugins.go/models.go/context7.go/profile_artifacts.go callers all use — fixing
// it here fixes all of them without touching those files.
func TestWriteJSONFile_WritesThroughSymlinkPreservingIt(t *testing.T) {
	requireSymlinkSupport(t)

	root := t.TempDir()
	realDir := filepath.Join(root, "real")
	linkDir := filepath.Join(root, "linked")
	if err := os.MkdirAll(realDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(realDir) error = %v", err)
	}
	if err := os.MkdirAll(linkDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(linkDir) error = %v", err)
	}

	realTarget := filepath.Join(realDir, "models.json")
	if err := os.WriteFile(realTarget, []byte(`{"existing":true}`+"\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(realTarget) error = %v", err)
	}

	symlinkPath := filepath.Join(linkDir, "models.json")
	if err := os.Symlink(realTarget, symlinkPath); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}

	if err := writeJSONFile(symlinkPath, map[string]any{"profile": "default"}); err != nil {
		t.Fatalf("writeJSONFile() error = %v", err)
	}

	info, err := os.Lstat(symlinkPath)
	if err != nil {
		t.Fatalf("Lstat(symlinkPath) error = %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("symlinkPath is no longer a symlink after writeJSONFile() (mode = %v) — it was destructively de-symlinked", info.Mode())
	}

	data, err := os.ReadFile(realTarget)
	if err != nil {
		t.Fatalf("ReadFile(realTarget) error = %v", err)
	}
	if !strings.Contains(string(data), `"profile": "default"`) {
		t.Fatalf("realTarget content = %s, want the new JSON written through the symlink", data)
	}
}

// TestWriteJSONFile_InjectedWriteErrorLeavesOriginalIntact mirrors
// TestWriteSettingsFile_InjectedWriteErrorLeavesOriginalIntact for writeJSONFile.
func TestWriteJSONFile_InjectedWriteErrorLeavesOriginalIntact(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "models.json")
	original := []byte(`{"original":true}` + "\n")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatalf("seed WriteFile() error = %v", err)
	}

	injectedErr := errors.New("injected write failure")
	old := createTempFile
	createTempFile = func(dir, pattern string) (tempFileWriter, error) {
		return &fakeFailingTempFile{name: filepath.Join(dir, ".click-injected-fake"), writeErr: injectedErr}, nil
	}
	defer func() { createTempFile = old }()

	err := writeJSONFile(path, map[string]any{"new": true})
	if err == nil {
		t.Fatal("writeJSONFile() error = nil, want the injected write error to propagate")
	}

	got, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("ReadFile(path) error = %v", readErr)
	}
	if string(got) != string(original) {
		t.Fatalf("file content = %q after a failed write, want untouched original %q", got, original)
	}
}

// TestRegisterMemoryGuardHook_WritesThroughSymlinkedSettings is an end-to-end regression test for
// the exact real-world scenario the finding describes: cfg.SettingsPath() resolves to a symlink
// into a dotfiles checkout, and the public RegisterMemoryGuardHook API (not just the unexported
// helper) must write through it correctly.
func TestRegisterMemoryGuardHook_WritesThroughSymlinkedSettings(t *testing.T) {
	requireSymlinkSupport(t)

	root := t.TempDir()
	dotfilesDir := filepath.Join(root, "dotfiles")
	claudeHome := filepath.Join(root, "claude-home")
	if err := os.MkdirAll(dotfilesDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(dotfilesDir) error = %v", err)
	}
	if err := os.MkdirAll(claudeHome, 0o755); err != nil {
		t.Fatalf("MkdirAll(claudeHome) error = %v", err)
	}

	realSettings := filepath.Join(dotfilesDir, "settings.json")
	if err := os.WriteFile(realSettings, []byte(`{}`+"\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(realSettings) error = %v", err)
	}

	cfg := Config{ClaudeHome: claudeHome}
	if err := os.Symlink(realSettings, cfg.SettingsPath()); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}

	if err := RegisterMemoryGuardHook(cfg); err != nil {
		t.Fatalf("RegisterMemoryGuardHook() error = %v", err)
	}

	info, err := os.Lstat(cfg.SettingsPath())
	if err != nil {
		t.Fatalf("Lstat(cfg.SettingsPath()) error = %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("cfg.SettingsPath() is no longer a symlink after RegisterMemoryGuardHook() — it was destructively de-symlinked")
	}

	registered, err := HasMemoryGuardHook(cfg)
	if err != nil {
		t.Fatalf("HasMemoryGuardHook() error = %v", err)
	}
	if !registered {
		t.Fatal("HasMemoryGuardHook() after RegisterMemoryGuardHook() through a symlink = false, want true")
	}
}
