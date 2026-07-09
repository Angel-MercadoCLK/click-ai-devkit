// Package installer owns the logic that installs and reverses click-ai-devkit's Claude Code
// plugin(s) into a developer's Claude Code setup (Config.ClaudeHome).
//
// v0.2 foundation installs the real click-sdd, click-memory, and click-review plugins through the
// native `claude plugin` marketplace flow while keeping the CLI thin: it patches the managed
// CLAUDE.md block and wires the memory-guard hook.
package installer

import (
	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/manifest"
	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/modelconfig"
)

// Install performs click-ai-devkit's current install flow: it registers the Click marketplace,
// installs the three managed plugins through Claude Code, registers (or respectfully skips) the
// Engram plugin, and writes the managed CLAUDE.md block into cfg.ClaudeHome. Slice 2 also
// registers the memory-guard PreToolUse hook in settings.json. It does not print anything itself —
// internal/cli wraps each step with ui.Renderer.RunStep for styled output (tech-spec.md §2.1).
// Idempotent: running Install twice against the same cfg leaves the same end state as running it
// once, and never reinstalls or clobbers an Engram setup a developer already had working.
//
// models is the per-phase click-sdd model selection (D25); pass nil to install with defaults
// (modelconfig.Resolve fills every phase). Install does not persist models to disk — that's
// internal/cli's job (installer.SaveModels), matching how RegisterMemoryGuardHook et al. are also
// orchestrated from the cli layer rather than bundled here.
func Install(cfg Config, models map[modelconfig.Phase]string) error {
	if err := SyncMarketplacePlugins(models); err != nil {
		return err
	}
	m, err := manifest.Load()
	if err != nil {
		return err
	}
	if _, err := SyncEngram(cfg, m); err != nil {
		return err
	}
	if err := WriteManagedBlock(cfg.ClaudeMDPath(), DefaultManagedContent); err != nil {
		return err
	}
	if err := RegisterMemoryGuardHook(cfg); err != nil {
		return err
	}
	return nil
}

// Uninstall reverses everything Install (and `click update`'s re-sync) can have written:
// uninstalls the managed plugins, removes the click-ai-devkit marketplace, strips the managed
// CLAUDE.md block, removes the managed memory-guard hook entry, and reverses the Engram plugin —
// but ONLY when click's own state says click installed Engram itself (RemoveEngramPlugin respects
// a pre-existing developer setup and leaves it running). It is idempotent — safe to call when
// already uninstalled, or when Engram was never touched by click in the first place.
func Uninstall(cfg Config) error {
	if err := RemoveMarketplacePlugins(); err != nil {
		return err
	}
	if err := RemoveEngramPlugin(cfg); err != nil {
		return err
	}
	if err := StripManagedBlock(cfg.ClaudeMDPath()); err != nil {
		return err
	}
	if err := UnregisterMemoryGuardHook(cfg); err != nil {
		return err
	}
	return nil
}
