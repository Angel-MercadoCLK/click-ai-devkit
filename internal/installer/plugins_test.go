package installer

import (
	"os"
	"path/filepath"
	"testing"

	clickstub "github.com/Angel-MercadoCLK/click-ai-devkit/plugins/click-stub"
)

func TestCopyStubPlugin_CreatesExpectedFiles(t *testing.T) {
	claudeHome := t.TempDir()
	cfg := Config{ClaudeHome: claudeHome}

	if err := CopyStubPlugin(cfg); err != nil {
		t.Fatalf("CopyStubPlugin() error = %v", err)
	}

	wantPluginJSON, err := clickstub.Files.ReadFile("plugin.json")
	if err != nil {
		t.Fatalf("read embedded plugin.json: %v", err)
	}
	wantReadme, err := clickstub.Files.ReadFile("README.md")
	if err != nil {
		t.Fatalf("read embedded README.md: %v", err)
	}

	gotPluginJSON, err := os.ReadFile(filepath.Join(cfg.PluginDir(), "plugin.json"))
	if err != nil {
		t.Fatalf("CopyStubPlugin() did not create plugin.json: %v", err)
	}
	if string(gotPluginJSON) != string(wantPluginJSON) {
		t.Errorf("copied plugin.json = %q, want %q", gotPluginJSON, wantPluginJSON)
	}

	gotReadme, err := os.ReadFile(filepath.Join(cfg.PluginDir(), "README.md"))
	if err != nil {
		t.Fatalf("CopyStubPlugin() did not create README.md: %v", err)
	}
	if string(gotReadme) != string(wantReadme) {
		t.Errorf("copied README.md = %q, want %q", gotReadme, wantReadme)
	}
}

func TestCopyStubPlugin_IdempotentOnSecondCall(t *testing.T) {
	claudeHome := t.TempDir()
	cfg := Config{ClaudeHome: claudeHome}

	if err := CopyStubPlugin(cfg); err != nil {
		t.Fatalf("first CopyStubPlugin() error = %v", err)
	}
	if err := CopyStubPlugin(cfg); err != nil {
		t.Fatalf("second CopyStubPlugin() error = %v", err)
	}

	entries, err := os.ReadDir(cfg.PluginDir())
	if err != nil {
		t.Fatalf("ReadDir(%s) error = %v", cfg.PluginDir(), err)
	}
	if len(entries) != 2 {
		t.Fatalf("PluginDir() has %d entries after two installs, want exactly 2 (plugin.json, README.md)", len(entries))
	}
}

func TestRemoveStubPlugin_RemovesDirectory(t *testing.T) {
	claudeHome := t.TempDir()
	cfg := Config{ClaudeHome: claudeHome}

	if err := CopyStubPlugin(cfg); err != nil {
		t.Fatalf("CopyStubPlugin() error = %v", err)
	}
	if err := RemoveStubPlugin(cfg); err != nil {
		t.Fatalf("RemoveStubPlugin() error = %v", err)
	}

	if _, err := os.Stat(cfg.PluginDir()); !os.IsNotExist(err) {
		t.Fatalf("RemoveStubPlugin() left %s behind", cfg.PluginDir())
	}
}

func TestRemoveStubPlugin_NoopWhenAlreadyAbsent(t *testing.T) {
	claudeHome := t.TempDir()
	cfg := Config{ClaudeHome: claudeHome}

	if err := RemoveStubPlugin(cfg); err != nil {
		t.Fatalf("RemoveStubPlugin() on an absent plugin error = %v, want nil", err)
	}
}
