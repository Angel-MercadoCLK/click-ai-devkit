package installer

import (
	"path/filepath"
	"strings"
	"testing"
)

// orphanRoleAgentTokens lists the 12 SDD roles that previously had no dedicated click-branded
// agent file (they either fell back to a generic delegation or had no click asset at all). Each
// token here is the exact modelconfig.Phase string / skill-dir name, and each must resolve to a
// real, non-empty agents/click-{token}.md file so click-sdd never needs gentle-ai's
// ~/.claude/agents/sdd-*.md (or any generic/unnamed agent) to run a full SDD cycle standalone.
var orphanRoleAgentTokens = []string{
	"explore",
	"apply",
	"archive",
	"onboard",
	"jd-judge-a",
	"jd-judge-b",
	"jd-fix-agent",
	"review-risk",
	"review-readability",
	"review-reliability",
	"review-resilience",
	"review-refuter",
}

// TestClickSDD_AllOrphanRolesHaveDedicatedAgent asserts every one of the 12 previously-orphan SDD
// roles now resolves to a real, non-empty click-branded agent file under plugins/click-sdd/agents/.
func TestClickSDD_AllOrphanRolesHaveDedicatedAgent(t *testing.T) {
	for _, token := range orphanRoleAgentTokens {
		token := token
		t.Run(token, func(t *testing.T) {
			relPath := filepath.Join("plugins", "click-sdd", "agents", "click-"+token+".md")
			data := mustReadRepoFile(t, "plugins", "click-sdd", "agents", "click-"+token+".md")
			if len(strings.TrimSpace(string(data))) == 0 {
				t.Fatalf("expected agent file %s to be non-empty", relPath)
			}
		})
	}
}

// TestClickOrchestrator_NamesEveryDedicatedAgent asserts click-orchestrator.md's delegation
// instructions name every one of the 12 dedicated click-{token} agents explicitly, so no orphan
// SDD role's delegation can silently fall back to a generic/unnamed agent.
func TestClickOrchestrator_NamesEveryDedicatedAgent(t *testing.T) {
	data := mustReadRepoFile(t, "plugins", "click-sdd", "agents", "click-orchestrator.md")
	content := string(data)

	for _, token := range orphanRoleAgentTokens {
		want := "click-" + token
		if !strings.Contains(content, want) {
			t.Errorf("click-orchestrator.md does not name %q for delegation", want)
		}
	}

	if strings.Contains(strings.ToLower(content), "general-purpose") {
		t.Errorf("click-orchestrator.md still references a generic-purpose fallback agent")
	}
}

// TestDefaultManagedContent_IsSelfContained guards Decision 4 (design doc): the managed CLAUDE.md
// block must stay free of any reference to gentle-ai's own orchestrator asset paths, so a machine
// with no gentle-ai installed still gets a fully self-contained ClickOrchestrator activation.
func TestDefaultManagedContent_IsSelfContained(t *testing.T) {
	forbidden := []string{"gentle-ai", "sdd-orchestrator-workflow", "~/.claude"}
	for _, substr := range forbidden {
		if strings.Contains(DefaultManagedContent, substr) {
			t.Errorf("DefaultManagedContent contains forbidden substring %q, want none (gentle-ai-free)", substr)
		}
	}
}
