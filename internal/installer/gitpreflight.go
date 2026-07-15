package installer

import "errors"

// GitMissingMessage is the single source of truth for the actionable Spanish text shown when git
// isn't resolvable on PATH. click needs git because SyncMarketplacePlugins registers the plugin
// marketplace via `claude plugin marketplace add <source>`, which shells out to `git clone` under
// the hood: on a machine with no git installed, that clone fails deep inside plugin registration
// with a cryptic error (reproduced live on a fresh Windows VM: "Command git not found or is in an
// unsafe location (current directory)"). PreflightGit surfaces this same requirement up front,
// before any other install step, so a developer on a fresh machine finds out instantly instead of
// hitting that mid-clone failure. Both `click install` and `click update`'s preflight, and
// doctor's checkGit, share this exact text so they never give a developer conflicting instructions
// — the same contract EngramBinaryRemediationMessage already establishes for the Engram binary.
const GitMissingMessage = "git no está instalado o no está en el PATH. click lo necesita para registrar el marketplace de plugins. Instalalo con: scoop install git (o https://git-scm.com/download/win) y volvé a intentar."

// GitPath resolves git's absolute path via the same injectable BinaryLookup used for the
// Engram/Go toolchain lookups (engram.go's ResolveEngramBinaryPath/goAvailable) — so tests can
// fake PATH resolution deterministically instead of depending on whether the real test machine has
// git installed. ok=false when git is not resolvable on PATH.
func GitPath() (path string, ok bool) {
	resolved, err := binaryLookupFactory().LookPath("git")
	if err != nil {
		return "", false
	}
	return resolved, true
}

// GitAvailable reports whether git is resolvable on PATH.
func GitAvailable() bool {
	_, ok := GitPath()
	return ok
}

// PreflightGit fails fast with GitMissingMessage when git is not resolvable on PATH. Call it
// before any step that (transitively) shells out to `claude plugin marketplace add` — install and
// update both do — so a missing git surfaces immediately, with an actionable message, instead of a
// cryptic failure deep inside plugin registration.
func PreflightGit() error {
	if GitAvailable() {
		return nil
	}
	return errors.New(GitMissingMessage)
}
