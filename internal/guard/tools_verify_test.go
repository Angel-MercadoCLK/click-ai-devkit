package guard

import (
	"strings"
	"testing"
)

func TestVerifyToolMatcher_AcceptsTheCanonicalPattern(t *testing.T) {
	if err := VerifyToolMatcher(ToolMatcherPattern()); err != nil {
		t.Fatalf("VerifyToolMatcher(ToolMatcherPattern()) = %v, want nil", err)
	}
}

func TestVerifyToolMatcher_RejectsMatchersThatDoNotCompile(t *testing.T) {
	err := VerifyToolMatcher("^(?:mcp__engram__")
	if err == nil {
		t.Fatal("VerifyToolMatcher accepted a matcher that does not compile as a regular expression")
	}
	if !strings.Contains(err.Error(), "no compila") {
		t.Fatalf("error = %q, want it to say the matcher does not compile", err)
	}
}

func TestVerifyToolMatcher_RejectsMatchersThatMissAPersistingTool(t *testing.T) {
	// The legacy mem_save-only matcher misses every other persisting tool and the standalone name form.
	err := VerifyToolMatcher("mcp__plugin_engram_engram__mem_save")
	if err == nil {
		t.Fatal("VerifyToolMatcher accepted a matcher that leaves persisting tools unscreened")
	}
	for _, want := range []string{"mcp__plugin_engram_engram__mem_update", "mcp__engram__mem_save"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, want it to name the uncovered tool %q", err, want)
		}
	}
}

func TestVerifyToolMatcher_RejectsMatchersThatOvermatchReadOnlyTools(t *testing.T) {
	err := VerifyToolMatcher(".*")
	if err == nil {
		t.Fatal("VerifyToolMatcher accepted a matcher that also matches read-only tools")
	}
	if !strings.Contains(err.Error(), "mem_search") {
		t.Fatalf("error = %q, want it to name an overmatched read-only tool", err)
	}
}

func TestVerifyToolMatcher_AcceptsAnEquivalentHandWrittenMatcher(t *testing.T) {
	// Behavior, not the exact string, decides: a differently spelled matcher with the same reach is fine.
	matcher := "^mcp__(?:plugin_engram_engram|engram)__(?:mem_capture_passive|mem_compare|mem_judge|mem_save|mem_save_prompt|mem_session_end|mem_session_summary|mem_update)$"
	if err := VerifyToolMatcher(matcher); err != nil {
		t.Fatalf("VerifyToolMatcher(equivalent) = %v, want nil", err)
	}
}
