package installer

import (
	"path/filepath"
	"strings"
	"testing"
)

// orphanRoleAgentTokens lists the 12 retained SDD-role agent source files. Each token is the exact
// modelconfig.Phase string and must resolve to a real, non-empty agents/click-{token}.md file while
// these inert agent sources remain in the repository during the staged retirement.
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

// TestClickSDD_AllOrphanRolesHaveDedicatedAgent asserts every retained role has a real, non-empty
// click-branded agent source under plugins/click-sdd/agents/. These inert sources are retired in a
// later slice; this check does not claim that a live Claude marketplace cache has been refreshed.
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

// TestDefaultManagedContent_DelegatesWorkflowToGentleAI guards the current distribution boundary:
// Click's managed guidance must not revive its retired orchestration instructions.
func TestDefaultManagedContent_DelegatesWorkflowToGentleAI(t *testing.T) {
	for _, obsolete := range []string{"ClickOrchestrator", "click-sdd", "sdd-orchestrator-workflow"} {
		if strings.Contains(DefaultManagedContent, obsolete) {
			t.Errorf("DefaultManagedContent contains retired workflow reference %q", obsolete)
		}
	}
	if !strings.Contains(DefaultManagedContent, "gentle-ai") || !strings.Contains(DefaultManagedContent, "ODD") {
		t.Fatal("DefaultManagedContent must direct workflow ownership to gentle-ai's ODD")
	}
}

// resultContractFieldTokens are the 6 canonical Result Contract field names still embedded inline
// in the retained phase-executor agent sources. The shared skill document is retired separately.
var resultContractFieldTokens = []string{
	"status",
	"executive_summary",
	"artifacts",
	"next_recommended",
	"risks",
	"skill_resolution",
}

// phaseExecutorAgentTokens are the 13 retained agent-file tokens (basename minus "click-") whose
// inert source files carry the inline Result Contract: the 12 orphanRoleAgentTokens plus reviewer.
var phaseExecutorAgentTokens = append(append([]string{}, orphanRoleAgentTokens...),
	"reviewer",
)

// resultContractExemptAgentTokens documents the remaining support role that is NOT required to
// carry the full 6-field Result Contract.
var resultContractExemptAgentTokens = []string{
	// memory-curator: post-cycle memory curation, not a chained phase in the taxonomy.
	"memory-curator",
}

// TestPhaseExecutorAgentsDeclareResultContract asserts every one of the 13 retained phase-executor agent
// tokens declares all 6 Result Contract fields inline in its agents/click-{token}.md file.
func TestPhaseExecutorAgentsDeclareResultContract(t *testing.T) {
	for _, token := range phaseExecutorAgentTokens {
		token := token
		t.Run(token, func(t *testing.T) {
			relPath := filepath.Join("plugins", "click-sdd", "agents", "click-"+token+".md")
			data := mustReadRepoFile(t, "plugins", "click-sdd", "agents", "click-"+token+".md")
			content := string(data)

			for _, field := range resultContractFieldTokens {
				if !strings.Contains(content, field) {
					t.Errorf("agent file %s does not declare Result Contract field %q", relPath, field)
				}
			}
		})
	}
}

// TestResultContractExemptAgentsNotRequired documents intent: the support role must never be
// silently promoted into the required phaseExecutorAgentTokens set. This is a static-list guard,
// not a content assertion — exempt files may still mention a field name in prose.
func TestResultContractExemptAgentsNotRequired(t *testing.T) {
	for _, exempt := range resultContractExemptAgentTokens {
		for _, required := range phaseExecutorAgentTokens {
			if exempt == required {
				t.Errorf("exempt agent token %q must not appear in phaseExecutorAgentTokens", exempt)
			}
		}
	}
}
