package installer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestGitAvailable_ResolvableOnPath guards the "present" branch: when the injected BinaryLookup
// resolves "git", GitAvailable must report true. Uses the same injectable BinaryLookup pattern as
// engram.go's goAvailable/ResolveEngramBinaryPath (SetBinaryLookupFactoryForTests) so this never
// depends on whether the real test machine actually has git on PATH.
func TestGitAvailable_ResolvableOnPath(t *testing.T) {
	lookup := &fakeBinaryLookup{resolved: map[string]string{"git": "/usr/bin/git"}}
	restore := SetBinaryLookupFactoryForTests(func() BinaryLookup { return lookup })
	defer restore()

	if !GitAvailable() {
		t.Fatal("GitAvailable() = false when git is resolvable on PATH, want true")
	}
}

// TestGitAvailable_MissingFromPath guards the "absent" branch — the exact fresh-machine scenario
// that motivated this fix (reproduced live on a fresh Windows VM with no git installed).
func TestGitAvailable_MissingFromPath(t *testing.T) {
	lookup := &fakeBinaryLookup{resolved: map[string]string{}}
	restore := SetBinaryLookupFactoryForTests(func() BinaryLookup { return lookup })
	defer restore()

	if GitAvailable() {
		t.Fatal("GitAvailable() = true when git is not resolvable on PATH, want false")
	}
}

// TestGitPath_ReturnsResolvedPathWhenAvailable guards that GitPath surfaces the actual resolved
// path (mirroring ResolveEngramBinaryPath/EngramBinaryResolvable's shape) so doctor's checkGit can
// report exactly where git was found, not just a bare boolean.
func TestGitPath_ReturnsResolvedPathWhenAvailable(t *testing.T) {
	lookup := &fakeBinaryLookup{resolved: map[string]string{"git": "/usr/bin/git"}}
	restore := SetBinaryLookupFactoryForTests(func() BinaryLookup { return lookup })
	defer restore()

	path, ok := GitPath()
	if !ok {
		t.Fatal("GitPath() ok = false when git is resolvable on PATH, want true")
	}
	if path != "/usr/bin/git" {
		t.Fatalf("GitPath() path = %q, want %q", path, "/usr/bin/git")
	}
}

// TestGitPath_NotOKWhenMissing guards GitPath's negative branch.
func TestGitPath_NotOKWhenMissing(t *testing.T) {
	lookup := &fakeBinaryLookup{resolved: map[string]string{}}
	restore := SetBinaryLookupFactoryForTests(func() BinaryLookup { return lookup })
	defer restore()

	if _, ok := GitPath(); ok {
		t.Fatal("GitPath() ok = true when git is not resolvable on PATH, want false")
	}
}

// TestPreflightGit_NilWhenGitPresent guards the happy path: PreflightGit must not block a machine
// that actually has git.
func TestPreflightGit_NilWhenGitPresent(t *testing.T) {
	lookup := &fakeBinaryLookup{resolved: map[string]string{"git": "/usr/bin/git"}}
	restore := SetBinaryLookupFactoryForTests(func() BinaryLookup { return lookup })
	defer restore()

	if err := PreflightGit(); err != nil {
		t.Fatalf("PreflightGit() error = %v, want nil when git is resolvable", err)
	}
}

// TestPreflightGit_ReturnsActionableErrorWhenGitMissing is the RED-then-GREEN core of this fix:
// reproduces the fresh-Windows-VM scenario (git absent from PATH) and asserts PreflightGit fails
// fast with GitMissingMessage's exact actionable Spanish text — never the cryptic
// "Command git not found or is in an unsafe location" error that used to surface deep inside
// `claude plugin marketplace add`'s own `git clone`.
func TestPreflightGit_ReturnsActionableErrorWhenGitMissing(t *testing.T) {
	lookup := &fakeBinaryLookup{resolved: map[string]string{}}
	restore := SetBinaryLookupFactoryForTests(func() BinaryLookup { return lookup })
	defer restore()

	err := PreflightGit()
	if err == nil {
		t.Fatal("PreflightGit() error = nil when git is missing from PATH, want a non-nil actionable error")
	}
	if !strings.Contains(err.Error(), GitMissingMessage) {
		t.Fatalf("PreflightGit() error = %q, want it to contain the actionable message %q", err.Error(), GitMissingMessage)
	}
}

func TestResolveGitExecutionContext_RelativeRepositorySelectorUsesAbsoluteRepositoryRoot(t *testing.T) {
	repoRoot := t.TempDir()
	if err := os.Mkdir(filepath.Join(repoRoot, ".git"), 0o755); err != nil {
		t.Fatalf("Mkdir(.git) error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(repoRoot, "nested", "deeper"), 0o755); err != nil {
		t.Fatalf("MkdirAll(nested) error = %v", err)
	}
	// Run from inside the repo so a repository-relative selector resolves (via filepath.Abs) against
	// it. The previous version derived the selector from filepath.Rel(os.Getwd(), tempDir), which
	// FAILS on Windows CI where the temp dir (C:) and the checkout (D:) live on different drives and
	// filepath.Rel cannot relate them. Resolve the root through Getwd after chdir so a symlinked temp
	// dir (macOS) still compares equal.
	t.Chdir(repoRoot)
	resolvedRoot, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}

	oldWorking := commandWorkingDir
	oldRecovery := commandTempRoot
	t.Cleanup(func() {
		commandWorkingDir = oldWorking
		commandTempRoot = oldRecovery
	})
	commandWorkingDir = func() (string, error) { return filepath.Join("nested", "deeper"), nil }
	commandTempRoot = func() string { return filepath.Join(t.TempDir(), "recovery-root") }

	ctx, err := resolveGitExecutionContext()
	if err != nil {
		t.Fatalf("resolveGitExecutionContext() error = %v", err)
	}
	if ctx.Recovered {
		t.Fatal("resolveGitExecutionContext() marked a relative repository selector as recovered, want the real repository root")
	}
	if ctx.WorkingDir != filepath.Clean(resolvedRoot) {
		t.Fatalf("resolveGitExecutionContext() WorkingDir = %q, want absolute repository root %q", ctx.WorkingDir, filepath.Clean(resolvedRoot))
	}
	if err := ctx.Cleanup(false); err != nil {
		t.Fatalf("Cleanup(false) error = %v", err)
	}
}

func TestResolveGitExecutionContext_AbsoluteRepositorySelectorUsesRepositoryRoot(t *testing.T) {
	repoRoot := t.TempDir()
	if err := os.Mkdir(filepath.Join(repoRoot, ".git"), 0o755); err != nil {
		t.Fatalf("Mkdir(.git) error = %v", err)
	}
	nested := filepath.Join(repoRoot, "nested")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("MkdirAll(nested) error = %v", err)
	}

	oldWorking := commandWorkingDir
	t.Cleanup(func() { commandWorkingDir = oldWorking })
	commandWorkingDir = func() (string, error) { return nested, nil }

	ctx, err := resolveGitExecutionContext()
	if err != nil {
		t.Fatalf("resolveGitExecutionContext() error = %v", err)
	}
	if ctx.Recovered {
		t.Fatal("resolveGitExecutionContext() marked an absolute repository selector as recovered, want the repository root")
	}
	if ctx.WorkingDir != filepath.Clean(repoRoot) {
		t.Fatalf("resolveGitExecutionContext() WorkingDir = %q, want %q", ctx.WorkingDir, filepath.Clean(repoRoot))
	}
	if err := ctx.Cleanup(true); err != nil {
		t.Fatalf("Cleanup(true) error = %v", err)
	}
}

func TestResolveGitExecutionContext_NonRepositorySelectorCreatesRecoveryRepoAndCleansItOnFailure(t *testing.T) {
	nonRepo := t.TempDir()
	recoveryParent := filepath.Join(t.TempDir(), "recovery-root")

	oldWorking := commandWorkingDir
	oldRecovery := commandTempRoot
	oldInit := commandGitInit
	t.Cleanup(func() {
		commandWorkingDir = oldWorking
		commandTempRoot = oldRecovery
		commandGitInit = oldInit
	})
	commandWorkingDir = func() (string, error) { return nonRepo, nil }
	commandTempRoot = func() string { return recoveryParent }
	commandGitInit = func(_ string, dir string) error {
		return os.MkdirAll(filepath.Join(dir, ".git"), 0o755)
	}
	restoreLookup := SetBinaryLookupFactoryForTests(func() BinaryLookup {
		return &fakeBinaryLookup{resolved: map[string]string{"git": `C:\git.exe`}}
	})
	defer restoreLookup()

	ctx, err := resolveGitExecutionContext()
	if err != nil {
		t.Fatalf("resolveGitExecutionContext() error = %v", err)
	}
	if !ctx.Recovered {
		t.Fatal("resolveGitExecutionContext() Recovered = false for a non-repository selector, want true")
	}
	if !filepath.IsAbs(ctx.WorkingDir) {
		t.Fatalf("resolveGitExecutionContext() WorkingDir = %q, want an absolute recovery repository", ctx.WorkingDir)
	}
	if _, err := os.Stat(filepath.Join(ctx.WorkingDir, ".git")); err != nil {
		t.Fatalf("recovery repository missing .git metadata: %v", err)
	}
	if err := ctx.Cleanup(false); err != nil {
		t.Fatalf("Cleanup(false) error = %v", err)
	}
	if _, err := os.Stat(ctx.WorkingDir); !os.IsNotExist(err) {
		t.Fatalf("Cleanup(false) left the recovery repository behind, err = %v", err)
	}
}

func TestResolveGitExecutionContext_UnsafeSelectorReturnsActionableErrorWithoutLeavingRecoveryArtifacts(t *testing.T) {
	unsafePath := filepath.Join(t.TempDir(), "not-a-directory.txt")
	if err := os.WriteFile(unsafePath, []byte("content"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	recoveryParent := filepath.Join(t.TempDir(), "recovery-root")

	oldWorking := commandWorkingDir
	oldRecovery := commandTempRoot
	t.Cleanup(func() {
		commandWorkingDir = oldWorking
		commandTempRoot = oldRecovery
	})
	commandWorkingDir = func() (string, error) { return unsafePath, nil }
	commandTempRoot = func() string { return recoveryParent }

	_, err := resolveGitExecutionContext()
	if err == nil {
		t.Fatal("resolveGitExecutionContext() error = nil for an unsafe cwd selector, want actionable failure")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "working directory") && !strings.Contains(strings.ToLower(err.Error()), "cwd") {
		t.Fatalf("resolveGitExecutionContext() error = %q, want actionable cwd guidance", err.Error())
	}
	entries, readErr := os.ReadDir(recoveryParent)
	if readErr != nil && !os.IsNotExist(readErr) {
		t.Fatalf("ReadDir(recoveryParent) error = %v", readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("unsafe selector left recovery artifacts behind: %#v", entries)
	}
}
