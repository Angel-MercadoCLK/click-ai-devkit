package installer

import (
	"fmt"
)

// DefaultCodexAgentsContent is guidance only. It deliberately excludes Claude Agent/Skill/plugin
// registry instructions. Click does not manage Codex model choices or native model settings.
const DefaultCodexAgentsContent = `Use gentle-ai's upstream Organic Driven Development (ODD) workflow for repository changes. Click supplies Click-specific policies and guidance; do not assume Claude Code agents, skills, plugins, or registries exist.
Click does not manage Codex model choices or change model settings in config.toml.
This block is managed by click: edit via "click update" and remove via "click uninstall".`

// SyncCodexGuidance writes only the Click-managed AGENTS.md block under CodexHome.
func SyncCodexGuidance(cfg Config) error {
	if cfg.CodexHome == "" {
		return nil
	}
	if err := WriteManagedBlock(cfg.CodexAgentsMDPath(), DefaultCodexAgentsContent); err != nil {
		return fmt.Errorf("installer: sync Codex AGENTS.md: %w", err)
	}
	return nil
}

// StripCodexGuidance removes only Click's managed block and preserves user guidance.
func StripCodexGuidance(cfg Config) error {
	if cfg.CodexHome == "" {
		return nil
	}
	if err := StripManagedBlock(cfg.CodexAgentsMDPath()); err != nil {
		return fmt.Errorf("installer: remove Codex AGENTS.md block: %w", err)
	}
	return nil
}
