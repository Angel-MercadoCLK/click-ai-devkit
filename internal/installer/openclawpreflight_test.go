package installer

import (
	"path/filepath"
	"testing"
)

// TestOpenClawAvailable_ResolvableOnPath guards the "present" branch: when the injected
// BinaryLookup resolves "openclaw", OpenClawAvailable must report true. Uses the same injectable
// BinaryLookup pattern as claudepreflight_test.go/gitpreflight_test.go (SetBinaryLookupFactoryForTests)
// so this never depends on whether the real test machine actually has openclaw on PATH.
func TestOpenClawAvailable_ResolvableOnPath(t *testing.T) {
	lookup := &fakeBinaryLookup{resolved: map[string]string{"openclaw": "/usr/bin/openclaw"}}
	restore := SetBinaryLookupFactoryForTests(func() BinaryLookup { return lookup })
	defer restore()

	if !OpenClawAvailable() {
		t.Fatal("OpenClawAvailable() = false when openclaw is resolvable on PATH, want true")
	}
}

// TestOpenClawAvailable_MissingFromPath guards the "absent" branch. Unlike ClaudeAvailable/
// GitAvailable, OpenClaw missing from PATH is a valid, silent, non-error state — no
// PreflightOpenClaw hard-fail exists (openclaw-target-support spec's openclaw-detection
// capability, "OpenClaw absent" scenario: report absent, without error, and skip all OpenClaw
// writes). This test only proves the detection primitive itself never panics and reports false.
func TestOpenClawAvailable_MissingFromPath(t *testing.T) {
	lookup := &fakeBinaryLookup{resolved: map[string]string{}}
	restore := SetBinaryLookupFactoryForTests(func() BinaryLookup { return lookup })
	defer restore()

	if OpenClawAvailable() {
		t.Fatal("OpenClawAvailable() = true when openclaw is not resolvable on PATH, want false")
	}
}

// TestOpenClawPath_ReturnsResolvedPathWhenAvailable guards that OpenClawPath surfaces the actual
// resolved binary path (mirroring ClaudePath/GitPath's shape) so doctor's checkOpenClaw can report
// exactly where openclaw was found, not just a bare boolean.
func TestOpenClawPath_ReturnsResolvedPathWhenAvailable(t *testing.T) {
	lookup := &fakeBinaryLookup{resolved: map[string]string{"openclaw": "/usr/bin/openclaw"}}
	restore := SetBinaryLookupFactoryForTests(func() BinaryLookup { return lookup })
	defer restore()

	path, ok := OpenClawPath()
	if !ok {
		t.Fatal("OpenClawPath() ok = false when openclaw is resolvable on PATH, want true")
	}
	if path != "/usr/bin/openclaw" {
		t.Fatalf("OpenClawPath() path = %q, want %q", path, "/usr/bin/openclaw")
	}
}

// TestOpenClawPath_NotOKWhenMissing guards OpenClawPath's negative branch: no error, no panic,
// just ok=false — the exact "no error" requirement from the spec's "OpenClaw absent" scenario.
func TestOpenClawPath_NotOKWhenMissing(t *testing.T) {
	lookup := &fakeBinaryLookup{resolved: map[string]string{}}
	restore := SetBinaryLookupFactoryForTests(func() BinaryLookup { return lookup })
	defer restore()

	if _, ok := OpenClawPath(); ok {
		t.Fatal("OpenClawPath() ok = true when openclaw is not resolvable on PATH, want false")
	}
}

// TestOpenClawPresent_ReportsWithWorkspacePath is the end-to-end shape spec's openclaw-detection
// capability describes ("OpenClaw present" scenario: report OpenClaw as present WITH its
// workspace path). openclawpreflight.go stays a pure binary-presence signal (mirrors
// claudepreflight.go exactly — no workspace-path logic duplicated here); the workspace path itself
// is Config's responsibility (OpenClawWorkspaceDir, config.go). This test proves the two building
// blocks compose correctly: once OpenClawAvailable() is true, a Config carrying the detected
// OpenClawHome derives the expected workspace directory.
func TestOpenClawPresent_ReportsWithWorkspacePath(t *testing.T) {
	lookup := &fakeBinaryLookup{resolved: map[string]string{"openclaw": "/usr/bin/openclaw"}}
	restore := SetBinaryLookupFactoryForTests(func() BinaryLookup { return lookup })
	defer restore()

	if !OpenClawAvailable() {
		t.Fatal("OpenClawAvailable() = false when openclaw is resolvable on PATH, want true")
	}

	cfg := Config{OpenClawHome: filepath.Join("some", "home", ".openclaw")}
	wantWorkspaceDir := filepath.Join("some", "home", ".openclaw", "workspace")
	if got := cfg.OpenClawWorkspaceDir(); got != wantWorkspaceDir {
		t.Fatalf("OpenClawWorkspaceDir() = %q, want %q", got, wantWorkspaceDir)
	}
}
