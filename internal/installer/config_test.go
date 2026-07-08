package installer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveClaudeHome_UsesEnvOverride(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("CLICK_CLAUDE_HOME", tmp)

	got, err := ResolveClaudeHome()
	if err != nil {
		t.Fatalf("ResolveClaudeHome() error = %v", err)
	}
	if got != tmp {
		t.Fatalf("ResolveClaudeHome() = %q, want the CLICK_CLAUDE_HOME override %q", got, tmp)
	}
}

func TestResolveClaudeHome_DefaultsUnderUserHome(t *testing.T) {
	t.Setenv("CLICK_CLAUDE_HOME", "")

	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("no user home dir available in this environment: %v", err)
	}

	got, err := ResolveClaudeHome()
	if err != nil {
		t.Fatalf("ResolveClaudeHome() error = %v", err)
	}
	want := filepath.Join(home, ".claude")
	if got != want {
		t.Fatalf("ResolveClaudeHome() = %q, want %q", got, want)
	}
}

func TestConfig_PluginDirAndClaudeMDPath(t *testing.T) {
	cfg := Config{ClaudeHome: filepath.Join("some", "home", ".claude")}

	wantPluginDir := filepath.Join("some", "home", ".claude", "plugins", "click-sdd")
	if got := cfg.ClickSDDPluginDir(); got != wantPluginDir {
		t.Errorf("ClickSDDPluginDir() = %q, want %q", got, wantPluginDir)
	}

	wantClaudeMD := filepath.Join("some", "home", ".claude", "CLAUDE.md")
	if got := cfg.ClaudeMDPath(); got != wantClaudeMD {
		t.Errorf("ClaudeMDPath() = %q, want %q", got, wantClaudeMD)
	}
}
