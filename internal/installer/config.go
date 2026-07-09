package installer

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// claudeHomeEnvOverride lets tests (and power users) point click at a directory other than the
// real ~/.claude, without touching the developer's actual Claude Code install. Every test that
// exercises install/doctor/uninstall MUST use this override with a t.TempDir() — never the real
// home directory (implementation brief's hard safety rule).
const claudeHomeEnvOverride = "CLICK_CLAUDE_HOME"

// Config carries every path click's installer needs. It is deliberately just ClaudeHome today —
// Slice 1 only touches the plugin dir and CLAUDE.md, both derived from it.
type Config struct {
	// ClaudeHome is the root of the target Claude Code installation, normally ~/.claude.
	ClaudeHome string
}

// ResolveClaudeHome resolves the Claude Code home directory click should install into: the
// CLICK_CLAUDE_HOME env override if set (used by tests and advanced overrides), otherwise
// <user home>/.claude.
func ResolveClaudeHome() (string, error) {
	if v := os.Getenv(claudeHomeEnvOverride); v != "" {
		return v, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("installer: resolve claude home: %w", err)
	}
	return filepath.Join(home, ".claude"), nil
}

// ClickSDDPluginDir is where the click-sdd plugin is installed under this Config's ClaudeHome.
func (c Config) ClickSDDPluginDir() string {
	return filepath.Join(c.ClaudeHome, "plugins", "click-sdd")
}

// ClickMemoryPluginDir is where the click-memory plugin is installed under this Config's ClaudeHome.
func (c Config) ClickMemoryPluginDir() string {
	return filepath.Join(c.ClaudeHome, "plugins", "click-memory")
}

// ClickReviewPluginDir is where the click-review plugin is installed under this Config's ClaudeHome.
func (c Config) ClickReviewPluginDir() string {
	return filepath.Join(c.ClaudeHome, "plugins", "click-review")
}

// KnownMarketplacesPath is Claude Code's plugin marketplace registry.
func (c Config) KnownMarketplacesPath() string {
	return filepath.Join(c.ClaudeHome, "plugins", "known_marketplaces.json")
}

// InstalledPluginsPath is Claude Code's installed plugins registry.
func (c Config) InstalledPluginsPath() string {
	return filepath.Join(c.ClaudeHome, "plugins", "installed_plugins.json")
}

// DefaultEngramBinaryPath is where Click-managed Engram binaries are expected to live locally.
func (c Config) DefaultEngramBinaryPath() string {
	name := "engram"
	if runtime.GOOS == "windows" {
		name = "engram.exe"
	}
	return filepath.Join(c.ClaudeHome, "bin", name)
}

// ClaudeMDPath is the managed CLAUDE.md file's path under this Config's ClaudeHome.
func (c Config) ClaudeMDPath() string {
	return filepath.Join(c.ClaudeHome, "CLAUDE.md")
}

// SettingsPath is Claude Code's settings.json under ClaudeHome.
func (c Config) SettingsPath() string {
	return filepath.Join(c.ClaudeHome, "settings.json")
}

// EngramMCPConfigPath is the durable MCP config Click writes for the pinned Engram binary.
func (c Config) EngramMCPConfigPath() string {
	return filepath.Join(c.ClaudeHome, "mcp", "engram.json")
}

// EngramStatePath stores the Click-managed pinned Engram metadata.
func (c Config) EngramStatePath() string {
	return filepath.Join(c.ClaudeHome, "click-ai-devkit", "engram.json")
}
