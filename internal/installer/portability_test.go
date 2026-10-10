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

// TestClickSDD_AllOrphanRolesHaveDedicatedAgent asserts every one of the 12 previously-orphan SDD
// roles now has a real, non-empty click-branded agent file under plugins/click-sdd/agents/. This
// static check does not claim that a live Claude marketplace cache has been refreshed.
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

// phaseExecutorAgentTokens are the 15 agent-file tokens (basename minus "click-") whose retained
// source files carry the inline Result Contract: the 12 orphanRoleAgentTokens plus prd-writer,
// architect, and reviewer. These are agent-file tokens, not phase tokens: propose/spec ->
// prd-writer, design/tasks -> architect, verify -> reviewer.
var phaseExecutorAgentTokens = append(append([]string{}, orphanRoleAgentTokens...),
	"prd-writer",
	"architect",
	"reviewer",
)

// resultContractExemptAgentTokens documents the 3 roles that are NOT required to carry the full
// 6-field Result Contract, each with the rationale for its exemption.
var resultContractExemptAgentTokens = []string{
	// orchestrator: consumer of the contract, not a returner — never emits its own envelope.
	"orchestrator",
	// memory-curator: post-cycle memory curation, not a chained phase in the taxonomy.
	"memory-curator",
	// elicitor: conversational interviewer with no Write/mem/skill tools; returns a requirements
	// brief, not a structured envelope.
	"elicitor",
}

// TestPhaseExecutorAgentsDeclareResultContract asserts every one of the 15 phase-executor agent
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

// TestResultContractExemptAgentsNotRequired documents intent: the 3 exempt roles must never be
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

// TestClickOrchestrator_HasModeGatekeeperSection asserts click-orchestrator.md defines the
// Automatic Mode Gatekeeper section (Phase 4/5 design Decision 1): scoped to execution_mode
// automatic, naming all 5 gate checks, and stating the retry-once/stop mechanics.
func TestClickOrchestrator_HasModeGatekeeperSection(t *testing.T) {
	data := mustReadRepoFile(t, "plugins", "click-sdd", "agents", "click-orchestrator.md")
	content := string(data)

	marker := "## Automatic Mode Gatekeeper"
	start := strings.Index(content, marker)
	if start == -1 {
		t.Fatalf("click-orchestrator.md does not contain section heading %q", marker)
	}

	rest := content[start+len(marker):]
	section := rest
	if end := strings.Index(rest, "\n## "); end != -1 {
		section = rest[:end]
	}
	lowerSection := strings.ToLower(section)

	required := []string{
		"execution_mode",
		"automatic",
		"contract conformance",
		"artifact existence",
		"no hallucination",
		"no drift",
		"routing coherence",
		"once",
		"stop",
	}
	for _, want := range required {
		if !strings.Contains(lowerSection, want) {
			t.Errorf("Automatic Mode Gatekeeper section does not contain %q", want)
		}
	}
}

// TestClickOrchestrator_JudgmentDayMandatory asserts Flow item 6 requires Judgment Day review
// (jd-judge-a + jd-judge-b + jd-fix-agent) unconditionally after design and after apply, in both
// execution modes, with no "optionally"/conditional language left in that flow item (Phase 4/5
// design Decision 4).
func TestClickOrchestrator_JudgmentDayMandatory(t *testing.T) {
	data := mustReadRepoFile(t, "plugins", "click-sdd", "agents", "click-orchestrator.md")
	content := string(data)

	marker := "6. **Mandatory Judgment Day"
	start := strings.Index(content, marker)
	if start == -1 {
		t.Fatalf("click-orchestrator.md does not contain Flow item marker %q", marker)
	}

	rest := content[start:]
	section := rest
	if end := strings.Index(rest, "\n7. "); end != -1 {
		section = rest[:end]
	}

	requiredExact := []string{
		"click-jd-judge-a",
		"click-jd-judge-b",
		"click-jd-fix-agent",
	}
	for _, want := range requiredExact {
		if !strings.Contains(section, want) {
			t.Errorf("Mandatory Judgment Day flow item does not contain %q", want)
		}
	}

	lowerSection := strings.ToLower(section)
	requiredCaseInsensitive := []string{"mandatory", "must", "both"}
	for _, want := range requiredCaseInsensitive {
		if !strings.Contains(lowerSection, want) {
			t.Errorf("Mandatory Judgment Day flow item does not contain (case-insensitive) %q", want)
		}
	}

	if strings.Contains(lowerSection, "optional") {
		t.Errorf("Mandatory Judgment Day flow item still contains conditional/optional language")
	}
}

// TestClickArchitect_TasksForecastPersisted asserts click-architect.md documents that the Review
// Workload Forecast is persisted inside the sdd/{change-name}/tasks artifact body (Phase 5 design
// Decision 2), not as a separate Engram topic.
func TestClickArchitect_TasksForecastPersisted(t *testing.T) {
	data := mustReadRepoFile(t, "plugins", "click-sdd", "agents", "click-architect.md")
	content := strings.ToLower(string(data))

	required := []string{
		"review workload forecast",
		"sdd/{change-name}/tasks",
	}
	for _, want := range required {
		if !strings.Contains(content, want) {
			t.Errorf("click-architect.md does not contain %q", want)
		}
	}
}

// TestClickOrchestrator_HasReviewWorkloadGuardSection asserts click-orchestrator.md defines the
// Review Workload Guard section (Phase 5 design Decision 3): the 4 named delivery_strategy modes,
// that it runs in both execution modes, and that it reads the tasks forecast before apply.
func TestClickOrchestrator_HasReviewWorkloadGuardSection(t *testing.T) {
	data := mustReadRepoFile(t, "plugins", "click-sdd", "agents", "click-orchestrator.md")
	content := string(data)

	marker := "## Review Workload Guard"
	start := strings.Index(content, marker)
	if start == -1 {
		t.Fatalf("click-orchestrator.md does not contain section heading %q", marker)
	}

	rest := content[start+len(marker):]
	section := rest
	if end := strings.Index(rest, "\n## "); end != -1 {
		section = rest[:end]
	}
	lowerSection := strings.ToLower(section)

	required := []string{
		"delivery_strategy",
		"ask-on-risk",
		"auto-chain",
		"single-pr",
		"exception-ok",
		"both",
		"forecast",
		"apply",
	}
	for _, want := range required {
		if !strings.Contains(lowerSection, want) {
			t.Errorf("Review Workload Guard section does not contain %q", want)
		}
	}
}

// TestClickOrchestrator_GuardPositionedBetweenTasksAndApply asserts the Review Workload Guard
// section sits between the Flow section and the Interactive default / Automatic Mode Gatekeeper
// sections, and that Flow item 5 (apply) forward-references it (Phase 5 design Decision 3).
func TestClickOrchestrator_GuardPositionedBetweenTasksAndApply(t *testing.T) {
	data := mustReadRepoFile(t, "plugins", "click-sdd", "agents", "click-orchestrator.md")
	content := string(data)

	guardIdx := strings.Index(content, "## Review Workload Guard")
	flowIdx := strings.Index(content, "## Flow")
	interactiveIdx := strings.Index(content, "## Interactive default")
	gatekeeperIdx := strings.Index(content, "## Automatic Mode Gatekeeper")

	if guardIdx == -1 || flowIdx == -1 || interactiveIdx == -1 || gatekeeperIdx == -1 {
		t.Fatalf("missing one of the required section markers (guard=%d flow=%d interactive=%d gatekeeper=%d)", guardIdx, flowIdx, interactiveIdx, gatekeeperIdx)
	}

	if !(guardIdx > flowIdx) {
		t.Errorf("expected Review Workload Guard section (%d) to come after Flow section (%d)", guardIdx, flowIdx)
	}
	if !(guardIdx < interactiveIdx) {
		t.Errorf("expected Review Workload Guard section (%d) to come before Interactive default section (%d)", guardIdx, interactiveIdx)
	}
	if !(guardIdx < gatekeeperIdx) {
		t.Errorf("expected Review Workload Guard section (%d) to come before Automatic Mode Gatekeeper section (%d)", guardIdx, gatekeeperIdx)
	}

	itemStart := strings.Index(content, "5. **Before ")
	if itemStart == -1 {
		t.Fatalf("click-orchestrator.md does not contain Flow item 5 marker %q", "5. **Before ")
	}
	rest := content[itemStart:]
	itemSection := rest
	if end := strings.Index(rest, "\n6. "); end != -1 {
		itemSection = rest[:end]
	}
	if !strings.Contains(itemSection, "Review Workload Guard") {
		t.Errorf("Flow item 5 does not forward-reference the Review Workload Guard section")
	}
}
