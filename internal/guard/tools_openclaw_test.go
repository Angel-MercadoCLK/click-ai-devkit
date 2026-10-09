package guard

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The OpenClaw adapter cannot import Go, so hooks.js carries its own copy of the persisting-tool
// set. This test is the guardrail that keeps both copies identical.
func TestOpenClawHooksListsTheSamePersistingTools(t *testing.T) {
	path := filepath.Join("..", "installer", "assets", "openclaw", "click-memory-guard", "plugins", "hooks.js")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read hooks.js: %v", err)
	}
	block := regexp.MustCompile(`(?s)const PERSISTING_ENGRAM_TOOLS = \[(.*?)\];`).FindSubmatch(data)
	if block == nil {
		t.Fatal("hooks.js must declare `const PERSISTING_ENGRAM_TOOLS = [...]`")
	}
	var got []string
	for _, m := range regexp.MustCompile(`'([a-z_]+)'`).FindAllSubmatch(block[1], -1) {
		got = append(got, string(m[1]))
	}
	sort.Strings(got)
	if want := PersistingEngramTools(); !reflect.DeepEqual(got, want) {
		t.Fatalf("hooks.js PERSISTING_ENGRAM_TOOLS = %v, want %v", got, want)
	}
	if !strings.Contains(string(data), "const CANONICAL_TOOL_PREFIX = 'mcp__plugin_engram_engram__';") {
		t.Fatal("hooks.js must canonicalize to the plugin-scoped tool name prefix")
	}
}
