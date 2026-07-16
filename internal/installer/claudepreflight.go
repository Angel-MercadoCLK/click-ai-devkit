package installer

import "errors"

// ClaudeMissingMessage is the single source of truth for the actionable Spanish text shown when
// claude isn't resolvable on PATH. click needs the claude CLI because SyncMarketplacePlugins
// registers every click-ai-devkit plugin (click-sdd, click-memory, click-review, click-skills) by
// shelling out to `claude plugin marketplace add`/`claude plugin install` (plugins.go's
// pluginCLIBinary): on a machine with no Claude Code installed, that call fails deep inside plugin
// registration with a cryptic Go error ("exec: \"claude\": executable file not found in %PATH%"),
// surfacing only after the developer already went through the whole interactive model-selection
// TUI. PreflightClaude surfaces this same requirement up front, before any other install step —
// mirroring PreflightGit/GitMissingMessage's exact pattern, so `click install`/`click update` never
// give a developer conflicting instructions for their two most fundamental dependencies.
const ClaudeMissingMessage = "claude no está instalado o no está en el PATH. click lo necesita para registrar sus plugins en Claude Code. Instalá Claude Code (https://docs.claude.com/en/docs/claude-code) y asegurate de que el comando claude quede disponible en el PATH antes de volver a ejecutar click install/click update."

// ClaudePath resolves claude's absolute path via the same injectable BinaryLookup used by
// GitPath/ResolveEngramBinaryPath (engram.go's binaryLookupFactory) — so tests can fake PATH
// resolution deterministically instead of depending on whether the real test machine has Claude
// Code installed. ok=false when claude is not resolvable on PATH.
func ClaudePath() (path string, ok bool) {
	resolved, err := binaryLookupFactory().LookPath("claude")
	if err != nil {
		return "", false
	}
	return resolved, true
}

// ClaudeAvailable reports whether claude is resolvable on PATH.
func ClaudeAvailable() bool {
	_, ok := ClaudePath()
	return ok
}

// PreflightClaude fails fast with ClaudeMissingMessage when claude is not resolvable on PATH. Call
// it before any step that (transitively) shells out to the claude CLI — install and update both
// do, via SyncMarketplacePlugins — so a missing claude surfaces immediately, with an actionable
// message, instead of a cryptic failure deep inside plugin registration after the interactive TUI.
func PreflightClaude() error {
	if ClaudeAvailable() {
		return nil
	}
	return errors.New(ClaudeMissingMessage)
}
