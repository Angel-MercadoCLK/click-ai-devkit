// Package installer owns the logic that installs and reverses click-ai-devkit's Claude Code
// plugin(s) into a developer's Claude Code setup (Config.ClaudeHome).
//
// Slice 1 (tracer bullet, implementation-plan.md §3) implements just enough to prove the
// mechanic end to end with a single stub plugin: copying files into ~/.claude/plugins/ and
// patching a managed block into ~/.claude/CLAUDE.md. The real click-sdd/click-memory/click-review
// plugins, the Engram MCP entry, and the memory-guard hook registration land in later slices.
package installer

// Install performs click-ai-devkit's Slice 1 tracer-bullet install: it copies the stub plugin
// and writes the managed CLAUDE.md block into cfg.ClaudeHome. Slice 2 also registers the
// memory-guard PreToolUse hook in settings.json. It does not print anything itself — internal/cli
// wraps each step with ui.Renderer.RunStep for styled output (tech-spec.md §2.1). Idempotent:
// running Install twice against the same cfg leaves the same end state as running it once.
func Install(cfg Config) error {
	if err := CopyStubPlugin(cfg); err != nil {
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

// Uninstall reverses Install exactly: removes the stub plugin directory and strips the managed
// CLAUDE.md block, and removes the managed memory-guard hook entry while leaving unrelated Claude
// settings intact. It is idempotent — safe to call when already uninstalled.
func Uninstall(cfg Config) error {
	if err := RemoveStubPlugin(cfg); err != nil {
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
