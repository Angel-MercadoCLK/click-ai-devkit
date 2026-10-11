package guard

import (
	"reflect"
	"regexp"
	"sort"
	"testing"
)

func TestPersistingEngramTools_IncludesEveryContentBearingTool(t *testing.T) {
	want := []string{
		"mem_capture_passive", "mem_compare", "mem_judge", "mem_save", "mem_save_prompt",
		"mem_session_end", "mem_session_summary", "mem_update",
	}
	got := PersistingEngramTools()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("PersistingEngramTools() = %v, want %v", got, want)
	}
	if !sort.StringsAreSorted(got) {
		t.Fatalf("PersistingEngramTools() must be sorted for stable output, got %v", got)
	}
}

func TestPersistingEngramTools_ReturnsACopy(t *testing.T) {
	first := PersistingEngramTools()
	first[0] = "tampered"
	if PersistingEngramTools()[0] == "tampered" {
		t.Fatal("PersistingEngramTools() leaked its backing slice")
	}
}

func TestIsPersistingEngramTool(t *testing.T) {
	tests := map[string]bool{
		"mcp__plugin_engram_engram__mem_save":        true,
		"mcp__plugin_engram_engram__mem_update":      true,
		"mcp__engram__mem_save":                      true,
		"mcp__engram__mem_session_end":               true,
		"mcp__plugin_engram_engram__mem_search":      false,
		"mcp__engram__mem_get_observation":           false,
		"mcp__engram__mem_save_extra":                false,
		"mcp__other__mem_save":                       false,
		"xmcp__engram__mem_save":                     false,
		"mem_save":                                   false,
		"":                                           false,
		"mcp__plugin_engram_engram__mem_save\x00":    false,
		"mcp__plugin_engram_engram__MEM_SAVE":        false,
		"mcp__plugin_engram_engram____mem_save":      false,
		"mcp__plugin_engram_engram__mem_save_prompt": true,
	}
	for name, want := range tests {
		if got := IsPersistingEngramTool(name); got != want {
			t.Errorf("IsPersistingEngramTool(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestToolMatcherPattern_MatchesExactlyTheAcceptedNames(t *testing.T) {
	re, err := regexp.Compile(ToolMatcherPattern())
	if err != nil {
		t.Fatalf("ToolMatcherPattern() is not a valid regexp: %v", err)
	}
	names := []string{
		"mcp__plugin_engram_engram__mem_save", "mcp__plugin_engram_engram__mem_update",
		"mcp__engram__mem_save", "mcp__engram__mem_judge", "mcp__engram__mem_search",
		"mcp__plugin_engram_engram__mem_search", "mcp__engram__mem_save_extra",
		"prefix_mcp__engram__mem_save", "mcp__other__mem_save", "Bash",
	}
	for _, name := range names {
		if got, want := re.MatchString(name), IsPersistingEngramTool(name); got != want {
			t.Errorf("pattern match(%q) = %v, IsPersistingEngramTool = %v; they must agree", name, got, want)
		}
	}
}
