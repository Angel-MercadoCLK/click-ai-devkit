package installer

import (
	"path/filepath"
	"testing"
)

// TestNoRelativeSnapshotPathsForClaudeLessSelections is a regression guard for a whole bug class,
// not a single defect. Most Config path helpers are rooted at ClaudeHome, which install/update
// only populate inside `if selection.Claude { ... }`. Any such helper reached by a Claude-less
// selection resolves via filepath.Join("", ...) to a RELATIVE path, and click then reads or writes
// it under whatever directory it happened to be launched from.
//
// This bit twice before: BackupDir() (which scattered the rollback safety net, and whose stray
// directory was papered over with a .gitignore entry) and EngramStatePath() (found by auditing for
// siblings right after fixing BackupDir). Rather than assert on those two by name, this walks every
// snapshot path the planner actually produces for each Claude-less selection, so a third instance
// fails here instead of silently polluting a developer's working directory.
func TestNoRelativeSnapshotPathsForClaudeLessSelections(t *testing.T) {
	cfg := Config{
		// ClaudeHome deliberately unset — that is the whole point of this test.
		ClickStateHome: t.TempDir(),
		CodexHome:      t.TempDir(),
		OpenClawHome:   t.TempDir(),
	}

	selections := []struct {
		name      string
		selection TargetSelection
	}{
		{"codex only", TargetSelection{Configured: true, Codex: true}},
		{"openclaw only", TargetSelection{Configured: true, OpenClaw: true}},
		{"codex and openclaw", TargetSelection{Configured: true, Codex: true, OpenClaw: true}},
	}

	for _, tc := range selections {
		t.Run(tc.name, func(t *testing.T) {
			plan := BuildTargetPlan(cfg, tc.selection, PlanOptions{})

			specs := plan.SnapshotSpecs()
			if len(specs) == 0 {
				t.Fatalf("planner produced no snapshot paths for %s; the guard would be vacuous", tc.name)
			}

			for _, decl := range specs {
				if decl.Path == "" {
					t.Errorf("empty snapshot path (policy %s)", decl.Policy)
					continue
				}
				if !filepath.IsAbs(decl.Path) {
					t.Errorf("relative snapshot path %q (policy %s): it would resolve under the process working directory, not a real home",
						decl.Path, decl.Policy)
				}
			}
		})
	}

	t.Run("backup destination", func(t *testing.T) {
		if got := cfg.BackupDir(); !filepath.IsAbs(got) {
			t.Errorf("BackupDir() = %q, want an absolute path", got)
		}
	})
}
