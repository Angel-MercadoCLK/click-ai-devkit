package installer

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

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
const GitMissingMessage = "git no está instalado o no está en el PATH. click lo necesita para registrar el marketplace de plugins. Instálelo con: scoop install git (o https://git-scm.com/download/win) y vuelva a intentar."

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

type gitExecutionContext struct {
	WorkingDir string
	Recovered  bool
	Cleanup    func(success bool) error
}

func resolveGitExecutionContext() (gitExecutionContext, error) {
	workingDir, err := commandWorkingDir()
	if err != nil {
		return gitExecutionContext{}, fmt.Errorf("installer: resolve subprocess working directory: %w", err)
	}
	if strings.TrimSpace(workingDir) == "" {
		return gitExecutionContext{}, errors.New("installer: subprocess working directory selector is empty; use an absolute or repository-relative cwd")
	}
	if !filepath.IsAbs(workingDir) {
		workingDir, err = filepath.Abs(workingDir)
		if err != nil {
			return gitExecutionContext{}, fmt.Errorf("installer: resolve absolute subprocess working directory from %q: %w", workingDir, err)
		}
	}
	workingDir = filepath.Clean(workingDir)
	info, err := os.Stat(workingDir)
	if err != nil {
		return gitExecutionContext{}, fmt.Errorf("installer: inspect subprocess working directory %q: %w", workingDir, err)
	}
	if !info.IsDir() {
		return gitExecutionContext{}, fmt.Errorf("installer: subprocess working directory %q is unsafe because it is not a directory; choose a repository or let click create a recovery repo", workingDir)
	}
	if root := trustedRepositoryRoot(workingDir); root != "" {
		return gitExecutionContext{
			WorkingDir: root,
			Cleanup:    func(bool) error { return nil },
		}, nil
	}

	recoveryParent := commandTempRoot()
	if err := commandMkdirAll(recoveryParent, 0o755); err != nil {
		return gitExecutionContext{}, fmt.Errorf("installer: create isolated command repository root %q: %w", recoveryParent, err)
	}
	recoveryDir, err := os.MkdirTemp(recoveryParent, "git-recovery-")
	if err != nil {
		return gitExecutionContext{}, fmt.Errorf("installer: create isolated command repository under %q: %w", recoveryParent, err)
	}
	cleanup := func(bool) error {
		if err := os.RemoveAll(recoveryDir); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("installer: remove isolated command repository %q: %w", recoveryDir, err)
		}
		return nil
	}
	gitPath, ok := GitPath()
	if !ok {
		_ = cleanup(false)
		return gitExecutionContext{}, errors.New("installer: cannot initialize an isolated command repository because git was not found")
	}
	if err := commandGitInit(gitPath, recoveryDir); err != nil {
		_ = cleanup(false)
		return gitExecutionContext{}, fmt.Errorf("installer: initialize isolated command repository %q: %w", recoveryDir, err)
	}
	if _, err := os.Stat(filepath.Join(recoveryDir, ".git")); err != nil {
		_ = cleanup(false)
		return gitExecutionContext{}, fmt.Errorf("installer: git initialization did not create repository metadata in %q: %w", recoveryDir, err)
	}
	return gitExecutionContext{
		WorkingDir: recoveryDir,
		Recovered:  true,
		Cleanup:    cleanup,
	}, nil
}
