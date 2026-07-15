package installer

import (
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
