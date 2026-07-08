package manifest

import (
	"os"
	"strings"
	"testing"
)

// TestEngramVersionMatchesManifest guards against the repo-root ENGRAM_VERSION file drifting from
// manifest.yaml's committed engram.version between releases.
//
// .github/workflows/release.yml reads ENGRAM_VERSION and patches manifest.yaml's engram.version
// from it, but only at tag-triggered release time — nothing enforces the two values matching in
// the repo as committed the rest of the time (e.g. right after someone bumps one file and forgets
// the other). This test closes that gap by asserting they agree at any point in the commit
// history, not just at release time.
func TestEngramVersionMatchesManifest(t *testing.T) {
	data, err := os.ReadFile("../../ENGRAM_VERSION")
	if err != nil {
		t.Fatalf("ReadFile(../../ENGRAM_VERSION) error = %v", err)
	}
	wantVersion := strings.TrimSpace(string(data))

	m, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if m.Engram.Version != wantVersion {
		t.Errorf("manifest.yaml engram.version = %q, want %q (from repo-root ENGRAM_VERSION)", m.Engram.Version, wantVersion)
	}
}
