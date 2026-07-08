package installer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/manifest"
)

func TestInstall_CopiesPluginAndWritesManagedBlock(t *testing.T) {
	claudeHome := t.TempDir()
	cfg := Config{ClaudeHome: claudeHome}

	if err := Install(cfg); err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(cfg.ClickSDDPluginDir(), ".claude-plugin", "plugin.json")); err != nil {
		t.Errorf("Install() did not copy click-sdd plugin.json: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cfg.ClickMemoryPluginDir(), ".claude-plugin", "plugin.json")); err != nil {
		t.Errorf("Install() did not copy click-memory plugin.json: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cfg.ClickReviewPluginDir(), ".claude-plugin", "plugin.json")); err != nil {
		t.Errorf("Install() did not copy click-review plugin.json: %v", err)
	}

	ok, err := HasManagedBlock(cfg.ClaudeMDPath())
	if err != nil {
		t.Fatalf("HasManagedBlock() error = %v", err)
	}
	if !ok {
		t.Error("Install() did not write the managed CLAUDE.md block")
	}

	if registered, err := HasMemoryGuardHook(cfg); err != nil {
		t.Fatalf("HasMemoryGuardHook() error = %v", err)
	} else if !registered {
		t.Error("Install() did not register the memory-guard hook")
	}
}

func TestInstall_TwiceIsIdempotent(t *testing.T) {
	claudeHome := t.TempDir()
	cfg := Config{ClaudeHome: claudeHome}

	if err := Install(cfg); err != nil {
		t.Fatalf("first Install() error = %v", err)
	}
	if err := Install(cfg); err != nil {
		t.Fatalf("second Install() error = %v", err)
	}

	entries, err := os.ReadDir(cfg.ClickSDDPluginDir())
	if err != nil {
		t.Fatalf("ReadDir(%s) error = %v", cfg.ClickSDDPluginDir(), err)
	}
	if len(entries) != 3 {
		t.Errorf("ClickSDDPluginDir() has %d entries after two installs, want exactly 3", len(entries))
	}

	claudeMD, err := os.ReadFile(cfg.ClaudeMDPath())
	if err != nil {
		t.Fatalf("ReadFile(CLAUDE.md) error = %v", err)
	}
	if n := strings.Count(string(claudeMD), managedBeginMarker); n != 1 {
		t.Errorf("CLAUDE.md has %d begin markers after two installs, want exactly 1", n)
	}
}

func TestUninstall_ReversesInstall(t *testing.T) {
	claudeHome := t.TempDir()
	cfg := Config{ClaudeHome: claudeHome}

	if err := Install(cfg); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if err := Uninstall(cfg); err != nil {
		t.Fatalf("Uninstall() error = %v", err)
	}

	if _, err := os.Stat(cfg.ClickSDDPluginDir()); !os.IsNotExist(err) {
		t.Error("Uninstall() left the plugin directory behind")
	}
	if _, err := os.Stat(cfg.ClickMemoryPluginDir()); !os.IsNotExist(err) {
		t.Error("Uninstall() left the click-memory plugin directory behind")
	}
	if _, err := os.Stat(cfg.ClickReviewPluginDir()); !os.IsNotExist(err) {
		t.Error("Uninstall() left the click-review plugin directory behind")
	}

	ok, err := HasManagedBlock(cfg.ClaudeMDPath())
	if err != nil {
		t.Fatalf("HasManagedBlock() error = %v", err)
	}
	if ok {
		t.Error("Uninstall() left the managed CLAUDE.md block behind")
	}
	if registered, err := HasMemoryGuardHook(cfg); err != nil {
		t.Fatalf("HasMemoryGuardHook() error = %v", err)
	} else if registered {
		t.Error("Uninstall() left the memory-guard hook behind")
	}
}

// TestUninstall_ReversesEngramMCPConfiguredByUpdate covers an asymmetry between what `click
// update` writes and what `click uninstall` reverses: ConfigureEngramMCP is only ever called by
// `click update` (never by `click install`), so a developer who ran install then update ends up
// with an MCP config + state file on disk that plain Install()/Uninstall() never touch. Uninstall
// must still remove them, since it must reverse everything install *and* update wrote.
func TestUninstall_ReversesEngramMCPConfiguredByUpdate(t *testing.T) {
	claudeHome := t.TempDir()
	cfg := Config{ClaudeHome: claudeHome}
	binaryPath := filepath.Join(t.TempDir(), "engram.exe")
	if err := os.WriteFile(binaryPath, []byte("binary"), 0o644); err != nil {
		t.Fatalf("WriteFile(binary) error = %v", err)
	}
	t.Setenv("CLICK_ENGRAM_BINARY_PATH", binaryPath)

	if err := Install(cfg); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	m, err := manifest.Load()
	if err != nil {
		t.Fatalf("manifest.Load() error = %v", err)
	}
	if err := ConfigureEngramMCP(cfg, m); err != nil {
		t.Fatalf("ConfigureEngramMCP() error = %v", err)
	}

	if err := Uninstall(cfg); err != nil {
		t.Fatalf("Uninstall() error = %v", err)
	}

	if _, err := os.Stat(cfg.EngramMCPConfigPath()); !os.IsNotExist(err) {
		t.Errorf("Uninstall() left the Engram MCP config behind at %s (err = %v)", cfg.EngramMCPConfigPath(), err)
	}
	if _, err := os.Stat(cfg.EngramStatePath()); !os.IsNotExist(err) {
		t.Errorf("Uninstall() left the Engram state file behind at %s (err = %v)", cfg.EngramStatePath(), err)
	}
}

func TestUninstall_NoopWhenAlreadyUninstalled(t *testing.T) {
	claudeHome := t.TempDir()
	cfg := Config{ClaudeHome: claudeHome}

	if err := Uninstall(cfg); err != nil {
		t.Fatalf("Uninstall() on a never-installed home error = %v, want nil", err)
	}
}

func TestInstallThenUninstallThenInstallAgain_Succeeds(t *testing.T) {
	claudeHome := t.TempDir()
	cfg := Config{ClaudeHome: claudeHome}

	if err := Install(cfg); err != nil {
		t.Fatalf("first Install() error = %v", err)
	}
	if err := Uninstall(cfg); err != nil {
		t.Fatalf("Uninstall() error = %v", err)
	}
	if err := Install(cfg); err != nil {
		t.Fatalf("re-Install() after uninstall error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(cfg.ClickSDDPluginDir(), ".claude-plugin", "plugin.json")); err != nil {
		t.Errorf("re-Install() did not copy click-sdd plugin.json: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cfg.ClickMemoryPluginDir(), ".claude-plugin", "plugin.json")); err != nil {
		t.Errorf("re-Install() did not copy click-memory plugin.json: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cfg.ClickReviewPluginDir(), ".claude-plugin", "plugin.json")); err != nil {
		t.Errorf("re-Install() did not copy click-review plugin.json: %v", err)
	}
	ok, err := HasManagedBlock(cfg.ClaudeMDPath())
	if err != nil {
		t.Fatalf("HasManagedBlock() error = %v", err)
	}
	if !ok {
		t.Error("re-Install() did not write the managed CLAUDE.md block")
	}
}
