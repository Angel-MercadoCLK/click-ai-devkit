package installer

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/guard"
)

func writeSettingsJSON(t *testing.T, cfg Config, settings map[string]any) {
	t.Helper()
	if err := writeSettingsFile(cfg.SettingsPath(), settings); err != nil {
		t.Fatalf("writeSettingsFile() error = %v", err)
	}
}

func readSettingsJSON(t *testing.T, cfg Config) map[string]any {
	t.Helper()
	data, err := os.ReadFile(cfg.SettingsPath())
	if err != nil {
		t.Fatalf("ReadFile(settings) error = %v", err)
	}
	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("settings is not valid JSON: %v", err)
	}
	return settings
}

func clickGuardEntryMatchers(t *testing.T, cfg Config) []string {
	t.Helper()
	var matchers []string
	for _, raw := range getPreToolUseEntries(readSettingsJSON(t, cfg)) {
		entry := raw.(map[string]any)
		hooks, _ := entry["hooks"].([]any)
		for _, hookRaw := range hooks {
			hook, _ := hookRaw.(map[string]any)
			if hook["type"] == "command" && hook["command"] == MemoryGuardCommand {
				matchers = append(matchers, entry["matcher"].(string))
			}
		}
	}
	return matchers
}

func legacyGuardEntry(extraHooks ...any) map[string]any {
	hooks := append([]any{map[string]any{"type": "command", "command": MemoryGuardCommand}}, extraHooks...)
	return map[string]any{"matcher": MemoryGuardLegacyToolMatcher, "hooks": hooks}
}

func TestMemoryGuardToolMatcher_CoversEveryPersistingToolInBothNameForms(t *testing.T) {
	re := regexp.MustCompile(MemoryGuardToolMatcher())
	for _, tool := range guard.PersistingEngramTools() {
		for _, prefix := range []string{"mcp__plugin_engram_engram__", "mcp__engram__"} {
			if !re.MatchString(prefix + tool) {
				t.Errorf("MemoryGuardToolMatcher %q does not match %q", MemoryGuardToolMatcher(), prefix+tool)
			}
		}
	}
	for _, name := range []string{"mcp__plugin_engram_engram__mem_search", "mcp__engram__mem_context", "Bash"} {
		if re.MatchString(name) {
			t.Errorf("MemoryGuardToolMatcher matches %q, which never persists content", name)
		}
	}
	if MemoryGuardToolMatcher() == MemoryGuardLegacyToolMatcher {
		t.Fatal("the current matcher must differ from the legacy mem_save-only matcher")
	}
}

func TestRegisterMemoryGuardHook_RegistersExactlyOneEntryWithFullMatcher(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir()}
	for i := 0; i < 2; i++ {
		if err := RegisterMemoryGuardHook(cfg); err != nil {
			t.Fatalf("RegisterMemoryGuardHook() #%d error = %v", i+1, err)
		}
	}
	matchers := clickGuardEntryMatchers(t, cfg)
	if len(matchers) != 1 || matchers[0] != MemoryGuardToolMatcher() {
		t.Fatalf("click guard entries = %v, want exactly [%q]", matchers, MemoryGuardToolMatcher())
	}
}

func TestRegisterMemoryGuardHook_UpgradesLegacyMatcherToExactlyOneEntry(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir()}
	foreign := map[string]any{"matcher": "Bash", "hooks": []any{map[string]any{"type": "command", "command": "other-tool"}}}
	writeSettingsJSON(t, cfg, map[string]any{
		"other": true,
		"hooks": map[string]any{"PreToolUse": []any{foreign, legacyGuardEntry()}},
	})

	for i := 0; i < 2; i++ {
		if err := RegisterMemoryGuardHook(cfg); err != nil {
			t.Fatalf("RegisterMemoryGuardHook() #%d error = %v", i+1, err)
		}
	}

	matchers := clickGuardEntryMatchers(t, cfg)
	if len(matchers) != 1 || matchers[0] != MemoryGuardToolMatcher() {
		t.Fatalf("click guard entries after upgrade = %v, want exactly [%q]", matchers, MemoryGuardToolMatcher())
	}
	settings := readSettingsJSON(t, cfg)
	if settings["other"] != true {
		t.Fatal("upgrade dropped an unrelated settings key")
	}
	var sawForeign bool
	for _, raw := range getPreToolUseEntries(settings) {
		if raw.(map[string]any)["matcher"] == "Bash" {
			sawForeign = true
		}
	}
	if !sawForeign {
		t.Fatal("upgrade dropped a foreign PreToolUse entry")
	}
}

func TestRegisterMemoryGuardHook_UpgradeKeepsForeignHooksSharingTheLegacyEntry(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir()}
	foreignHook := map[string]any{"type": "command", "command": "audit-extra"}
	writeSettingsJSON(t, cfg, map[string]any{
		"hooks": map[string]any{"PreToolUse": []any{legacyGuardEntry(foreignHook)}},
	})

	if err := RegisterMemoryGuardHook(cfg); err != nil {
		t.Fatalf("RegisterMemoryGuardHook() error = %v", err)
	}

	raw, _ := json.Marshal(readSettingsJSON(t, cfg))
	if !strings.Contains(string(raw), "audit-extra") {
		t.Fatalf("upgrade dropped a foreign hook that shared the legacy entry: %s", raw)
	}
	if matchers := clickGuardEntryMatchers(t, cfg); len(matchers) != 1 || matchers[0] != MemoryGuardToolMatcher() {
		t.Fatalf("click guard entries = %v, want exactly [%q]", matchers, MemoryGuardToolMatcher())
	}
}

func TestHasMemoryGuardHook_LegacyOnlyIsNotFullyRegistered(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir()}
	writeSettingsJSON(t, cfg, map[string]any{
		"hooks": map[string]any{"PreToolUse": []any{legacyGuardEntry()}},
	})

	ok, err := HasMemoryGuardHook(cfg)
	if err != nil {
		t.Fatalf("HasMemoryGuardHook() error = %v", err)
	}
	if ok {
		t.Fatal("HasMemoryGuardHook() = true for the legacy mem_save-only matcher, want false (coverage is incomplete)")
	}
	legacy, err := HasLegacyMemoryGuardHook(cfg)
	if err != nil {
		t.Fatalf("HasLegacyMemoryGuardHook() error = %v", err)
	}
	if !legacy {
		t.Fatal("HasLegacyMemoryGuardHook() = false, want true so doctor can tell an outdated install from a missing one")
	}
}

func TestUnregisterMemoryGuardHook_RemovesLegacyAndCurrentEntries(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir()}
	current := map[string]any{
		"matcher": MemoryGuardToolMatcher(),
		"hooks":   []any{map[string]any{"type": "command", "command": MemoryGuardCommand}},
	}
	foreign := map[string]any{"matcher": "Bash", "hooks": []any{map[string]any{"type": "command", "command": "other-tool"}}}
	writeSettingsJSON(t, cfg, map[string]any{
		"hooks": map[string]any{"PreToolUse": []any{legacyGuardEntry(), current, foreign}},
	})

	if err := UnregisterMemoryGuardHook(cfg); err != nil {
		t.Fatalf("UnregisterMemoryGuardHook() error = %v", err)
	}

	if matchers := clickGuardEntryMatchers(t, cfg); len(matchers) != 0 {
		t.Fatalf("click guard entries after uninstall = %v, want none", matchers)
	}
	entries := getPreToolUseEntries(readSettingsJSON(t, cfg))
	if len(entries) != 1 || entries[0].(map[string]any)["matcher"] != "Bash" {
		t.Fatalf("uninstall must keep only the foreign entry, got %v", entries)
	}
}

func TestManagedSettingsProjectionHash_TreatsLegacyGuardEntryAsOwned(t *testing.T) {
	content, err := json.Marshal(map[string]any{
		"hooks": map[string]any{"PreToolUse": []any{legacyGuardEntry()}},
	})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if hash := managedSettingsProjectionHash(string(content)); !strings.HasPrefix(hash, "present:") {
		t.Fatalf("managedSettingsProjectionHash(legacy entry) = %q, want a present: projection", hash)
	}
}
