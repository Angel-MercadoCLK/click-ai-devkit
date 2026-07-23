package installer

import "fmt"

// DefaultCodexAgentsContent is guidance only. It deliberately excludes Claude Agent/Skill/plugin
// registry instructions and does not claim to configure Codex's native settings.
const DefaultCodexAgentsContent = `Use Click's portable SDD workflow for repository changes: explore, propose, spec, design, tasks, apply, verify, and archive, with onboard available for setup. Follow the workflow guidance shipped with click-ai-devkit; do not assume Claude Code agents, skills, plugins, or registries exist.
Codex configuration and model selection remain user-owned. Click does not modify config.toml, credentials, providers, or native Codex skill/plugin packaging in this slice.
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
