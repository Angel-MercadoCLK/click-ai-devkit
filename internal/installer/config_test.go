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

	wantMemoryPluginDir := filepath.Join("some", "home", ".claude", "plugins", "click-memory")
	if got := cfg.ClickMemoryPluginDir(); got != wantMemoryPluginDir {
		t.Errorf("ClickMemoryPluginDir() = %q, want %q", got, wantMemoryPluginDir)
	}

	wantReviewPluginDir := filepath.Join("some", "home", ".claude", "plugins", "click-review")
	if got := cfg.ClickReviewPluginDir(); got != wantReviewPluginDir {
		t.Errorf("ClickReviewPluginDir() = %q, want %q", got, wantReviewPluginDir)
	}

	wantStatePath := filepath.Join("some", "home", ".claude", "click-ai-devkit", "engram.json")
	if got := cfg.EngramStatePath(); got != wantStatePath {
		t.Errorf("EngramStatePath() = %q, want %q", got, wantStatePath)
	}

	wantClaudeMD := filepath.Join("some", "home", ".claude", "CLAUDE.md")
	if got := cfg.ClaudeMDPath(); got != wantClaudeMD {
		t.Errorf("ClaudeMDPath() = %q, want %q", got, wantClaudeMD)
	}
}

// TestConfig_ProfileArtifactPath guards the per-profile artifact file path PR2b's
// profile_artifacts.go will read/write: <ClaudeHome>/click-ai-devkit/profiles/<name>.json.
func TestConfig_ProfileArtifactPath(t *testing.T) {
	cfg := Config{ClaudeHome: filepath.Join("some", "home", ".claude")}

	want := filepath.Join("some", "home", ".claude", "click-ai-devkit", "profiles", "cost-saver.json")
	if got := cfg.ProfileArtifactPath("cost-saver"); got != want {
		t.Errorf("ProfileArtifactPath(%q) = %q, want %q", "cost-saver", got, want)
	}
}

// TestConfig_ProfileAgentsDir guards the per-profile agent-substrate directory PR2b's
// profile_artifacts.go will write generated markdown agents under:
// <ClaudeHome>/click-ai-devkit/profiles/<name>/agents.
func TestConfig_ProfileAgentsDir(t *testing.T) {
	cfg := Config{ClaudeHome: filepath.Join("some", "home", ".claude")}

	want := filepath.Join("some", "home", ".claude", "click-ai-devkit", "profiles", "cost-saver", "agents")
	if got := cfg.ProfileAgentsDir("cost-saver"); got != want {
		t.Errorf("ProfileAgentsDir(%q) = %q, want %q", "cost-saver", got, want)
	}
}

// TestConfig_BackupDir guards snapshot.go's run-snapshot storage location (install-reliability-
// foundation change): <ClaudeHome>/click-ai-devkit/backups.
func TestConfig_BackupDir(t *testing.T) {
	cfg := Config{ClaudeHome: filepath.Join("some", "home", ".claude")}

	want := filepath.Join("some", "home", ".claude", "click-ai-devkit", "backups")
	if got := cfg.BackupDir(); got != want {
		t.Errorf("BackupDir() = %q, want %q", got, want)
	}
}
