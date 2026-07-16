package installer

import (
	"strings"
	"testing"
)

// TestClaudeAvailable_ResolvableOnPath guards the "present" branch: when the injected BinaryLookup
// resolves "claude", ClaudeAvailable must report true. Uses the same injectable BinaryLookup
// pattern as gitpreflight_test.go / engram.go's goAvailable (SetBinaryLookupFactoryForTests) so
// this never depends on whether the real test machine actually has claude on PATH.
func TestClaudeAvailable_ResolvableOnPath(t *testing.T) {
	lookup := &fakeBinaryLookup{resolved: map[string]string{"claude": "/usr/bin/claude"}}
	restore := SetBinaryLookupFactoryForTests(func() BinaryLookup { return lookup })
	defer restore()

	if !ClaudeAvailable() {
		t.Fatal("ClaudeAvailable() = false when claude is resolvable on PATH, want true")
	}
}

// TestClaudeAvailable_MissingFromPath guards the "absent" branch — the exact fresh-machine
// scenario that motivated this fix: `click install` ran its whole interactive TUI and then died
// with a raw Go dump (exec: "claude": executable file not found in %PATH%) deep inside
// plugins.go's addMarketplace/pluginCLIBinary calls.
func TestClaudeAvailable_MissingFromPath(t *testing.T) {
	lookup := &fakeBinaryLookup{resolved: map[string]string{}}
	restore := SetBinaryLookupFactoryForTests(func() BinaryLookup { return lookup })
	defer restore()

	if ClaudeAvailable() {
		t.Fatal("ClaudeAvailable() = true when claude is not resolvable on PATH, want false")
	}
}

// TestClaudePath_ReturnsResolvedPathWhenAvailable guards that ClaudePath surfaces the actual
// resolved path (mirroring GitPath/ResolveEngramBinaryPath's shape).
func TestClaudePath_ReturnsResolvedPathWhenAvailable(t *testing.T) {
	lookup := &fakeBinaryLookup{resolved: map[string]string{"claude": "/usr/bin/claude"}}
	restore := SetBinaryLookupFactoryForTests(func() BinaryLookup { return lookup })
	defer restore()

	path, ok := ClaudePath()
	if !ok {
		t.Fatal("ClaudePath() ok = false when claude is resolvable on PATH, want true")
	}
	if path != "/usr/bin/claude" {
		t.Fatalf("ClaudePath() path = %q, want %q", path, "/usr/bin/claude")
	}
}

// TestClaudePath_NotOKWhenMissing guards ClaudePath's negative branch.
func TestClaudePath_NotOKWhenMissing(t *testing.T) {
	lookup := &fakeBinaryLookup{resolved: map[string]string{}}
	restore := SetBinaryLookupFactoryForTests(func() BinaryLookup { return lookup })
	defer restore()

	if _, ok := ClaudePath(); ok {
		t.Fatal("ClaudePath() ok = true when claude is not resolvable on PATH, want false")
	}
}

// TestPreflightClaude_NilWhenClaudePresent guards the happy path: PreflightClaude must not block a
// machine that actually has claude on PATH.
func TestPreflightClaude_NilWhenClaudePresent(t *testing.T) {
	lookup := &fakeBinaryLookup{resolved: map[string]string{"claude": "/usr/bin/claude"}}
	restore := SetBinaryLookupFactoryForTests(func() BinaryLookup { return lookup })
	defer restore()

	if err := PreflightClaude(); err != nil {
		t.Fatalf("PreflightClaude() error = %v, want nil when claude is resolvable", err)
	}
}

// TestPreflightClaude_ReturnsActionableErrorWhenClaudeMissing is the RED-then-GREEN core of this
// fix: reproduces the fresh-machine scenario (claude absent from PATH) and asserts PreflightClaude
// fails fast with ClaudeMissingMessage's exact actionable Spanish text — never the cryptic
// "exec: \"claude\": executable file not found in %PATH%" error that used to surface deep inside
// SyncMarketplacePlugins' addMarketplace call, after the developer had already gone through the
// whole interactive model-selection TUI.
func TestPreflightClaude_ReturnsActionableErrorWhenClaudeMissing(t *testing.T) {
	lookup := &fakeBinaryLookup{resolved: map[string]string{}}
	restore := SetBinaryLookupFactoryForTests(func() BinaryLookup { return lookup })
	defer restore()

	err := PreflightClaude()
	if err == nil {
		t.Fatal("PreflightClaude() error = nil when claude is missing from PATH, want a non-nil actionable error")
	}
	if !strings.Contains(err.Error(), ClaudeMissingMessage) {
		t.Fatalf("PreflightClaude() error = %q, want it to contain the actionable message %q", err.Error(), ClaudeMissingMessage)
	}
}
