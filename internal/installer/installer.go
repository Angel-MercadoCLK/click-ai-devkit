// Package installer owns the logic that installs and reverses click-ai-devkit's Claude Code
// plugin(s) into a developer's Claude Code setup (Config.ClaudeHome).
//
// Slice 4 installs the real click-sdd and click-memory plugins while keeping the CLI thin: it
// copies plugin content, patches the managed CLAUDE.md block, and wires the memory-guard hook.
// The click-review plugin still lands in a later slice.
package installer

// Install performs click-ai-devkit's current install flow: it copies the click-sdd and
// click-memory plugins and
// writes the managed CLAUDE.md block into cfg.ClaudeHome. Slice 2 also registers the
// memory-guard PreToolUse hook in settings.json. It does not print anything itself — internal/cli
// wraps each step with ui.Renderer.RunStep for styled output (tech-spec.md §2.1). Idempotent:
// running Install twice against the same cfg leaves the same end state as running it once.
func Install(cfg Config) error {
	if err := CopyClickSDDPlugin(cfg); err != nil {
		return err
	}
	if err := CopyClickMemoryPlugin(cfg); err != nil {
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

// Uninstall reverses Install exactly: removes the click-sdd and click-memory plugin directories,
// strips the managed CLAUDE.md block, and removes the managed memory-guard hook entry while
// leaving unrelated Claude settings intact. It is idempotent — safe to call when already
// uninstalled.
func Uninstall(cfg Config) error {
	if err := RemoveClickSDDPlugin(cfg); err != nil {
		return err
	}
	if err := RemoveClickMemoryPlugin(cfg); err != nil {
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
