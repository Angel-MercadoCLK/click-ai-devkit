package installer

import (
	"fmt"
	"os"
	"strings"
)

// DefaultCodexAgentsContent is guidance only. It deliberately excludes Claude Agent/Skill/plugin
// registry instructions. Native Codex model configuration is separate and explicit.
const DefaultCodexAgentsContent = `Use Click's portable SDD workflow for repository changes: explore, propose, spec, design, tasks, apply, verify, and archive, with onboard available for setup. Follow the workflow guidance shipped with click-ai-devkit; do not assume Claude Code agents, skills, plugins, or registries exist.
Codex model selection is user-owned unless explicitly selected during installation; Click never changes credentials or providers implicitly.
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
	if data, err := os.ReadFile(cfg.CodexAgentsMDPath()); err == nil && strings.TrimSpace(string(data)) == "" {
		if removeErr := os.Remove(cfg.CodexAgentsMDPath()); removeErr != nil && !os.IsNotExist(removeErr) {
			return fmt.Errorf("installer: remove empty Codex AGENTS.md: %w", removeErr)
		}
	}
	return nil
}
