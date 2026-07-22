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

// TestResolveOpenClawHome_UsesEnvOverride mirrors TestResolveClaudeHome_UsesEnvOverride for
// CLICK_OPENCLAW_HOME (RED at write time: ResolveOpenClawHome does not exist until config.go's
// GREEN change).
// TestConfig_OpenClawPluginDir is PR-C task 3.9's supporting RED test: OpenClawPluginDir must join
// OpenClawHome with plugins/click-memory-guard, mirroring ClickSDDPluginDir's derivation shape under
// ClaudeHome (RED at write time: OpenClawPluginDir does not exist until config.go's GREEN change).
func TestConfig_OpenClawPluginDir(t *testing.T) {
	cfg := Config{OpenClawHome: filepath.Join("home", ".openclaw")}
	want := filepath.Join("home", ".openclaw", "plugins", "click-memory-guard")
	if got := cfg.OpenClawPluginDir(); got != want {
		t.Fatalf("OpenClawPluginDir() = %q, want %q", got, want)
	}
}

func TestResolveOpenClawHome_UsesEnvOverride(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("CLICK_OPENCLAW_HOME", tmp)

	got, err := ResolveOpenClawHome()
	if err != nil {
		t.Fatalf("ResolveOpenClawHome() error = %v", err)
	}
	if got != tmp {
		t.Fatalf("ResolveOpenClawHome() = %q, want the CLICK_OPENCLAW_HOME override %q", got, tmp)
	}
}

// TestResolveOpenClawHome_DefaultsUnderUserHome triangulates the override case above with the
// default (no env override) branch, forcing the real os.UserHomeDir()+".openclaw" join logic
// rather than a hardcoded return.
func TestResolveOpenClawHome_DefaultsUnderUserHome(t *testing.T) {
	t.Setenv("CLICK_OPENCLAW_HOME", "")

	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("no user home dir available in this environment: %v", err)
	}

	got, err := ResolveOpenClawHome()
	if err != nil {
		t.Fatalf("ResolveOpenClawHome() error = %v", err)
	}
	want := filepath.Join(home, ".openclaw")
	if got != want {
		t.Fatalf("ResolveOpenClawHome() = %q, want %q", got, want)
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

// TestConfig_OpenClawPaths_Populated guards the openclaw-target-support spec's "install-config"
// capability, "OpenClaw present" scenario: GIVEN OpenClaw detection succeeds, WHEN Config is
// built, THEN OpenClawHome and every derived path MUST be populated — mirroring how
// ClaudeMDPath/SettingsPath derive from ClaudeHome. Workspace-scoped files (AGENTS.md, SOUL.md)
// live under <OpenClawHome>/workspace, while the MCP config lives directly under <OpenClawHome>
// (matches the confirmed real openclaw.json shape referenced in design #1666).
func TestConfig_OpenClawPaths_Populated(t *testing.T) {
	cfg := Config{
		ClaudeHome:   filepath.Join("some", "home", ".claude"),
		OpenClawHome: filepath.Join("some", "home", ".openclaw"),
	}

	wantWorkspaceDir := filepath.Join("some", "home", ".openclaw", "workspace")
	if got := cfg.OpenClawWorkspaceDir(); got != wantWorkspaceDir {
		t.Errorf("OpenClawWorkspaceDir() = %q, want %q", got, wantWorkspaceDir)
	}

	wantAgentsMD := filepath.Join("some", "home", ".openclaw", "workspace", "AGENTS.md")
	if got := cfg.OpenClawAgentsMDPath(); got != wantAgentsMD {
		t.Errorf("OpenClawAgentsMDPath() = %q, want %q", got, wantAgentsMD)
	}

	wantSoulMD := filepath.Join("some", "home", ".openclaw", "workspace", "SOUL.md")
	if got := cfg.OpenClawSoulMDPath(); got != wantSoulMD {
		t.Errorf("OpenClawSoulMDPath() = %q, want %q", got, wantSoulMD)
	}

	wantMCPConfig := filepath.Join("some", "home", ".openclaw", "openclaw.json")
	if got := cfg.OpenClawMCPConfigPath(); got != wantMCPConfig {
		t.Errorf("OpenClawMCPConfigPath() = %q, want %q", got, wantMCPConfig)
	}
}

// TestConfig_OpenClawAbsent_ZeroValueDoesNotAffectClaudePaths guards the spec's "OpenClaw absent"
// scenario: GIVEN OpenClaw detection fails (OpenClawHome never set), WHEN Config is built, THEN
// OpenClaw fields MUST remain zero-value, no error MUST be raised (these are pure string-joining
// methods — there is no error return to check), and Claude-only paths MUST be completely
// unaffected. This is the exact "zero risk to Claude-only hosts" guarantee this slice must hold.
func TestConfig_OpenClawAbsent_ZeroValueDoesNotAffectClaudePaths(t *testing.T) {
	cfg := Config{ClaudeHome: filepath.Join("some", "home", ".claude")}

	if cfg.OpenClawHome != "" {
		t.Fatalf("OpenClawHome zero value = %q, want empty string", cfg.OpenClawHome)
	}

	wantClaudeMD := filepath.Join("some", "home", ".claude", "CLAUDE.md")
	if got := cfg.ClaudeMDPath(); got != wantClaudeMD {
		t.Errorf("ClaudeMDPath() = %q, want %q (must be unaffected by an unset OpenClawHome)", got, wantClaudeMD)
	}

	wantSettings := filepath.Join("some", "home", ".claude", "settings.json")
	if got := cfg.SettingsPath(); got != wantSettings {
		t.Errorf("SettingsPath() = %q, want %q (must be unaffected by an unset OpenClawHome)", got, wantSettings)
	}
}
