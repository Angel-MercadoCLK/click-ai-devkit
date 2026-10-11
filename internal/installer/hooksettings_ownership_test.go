package installer

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

const driftedGuardMatcher = "mcp__engram__mem_save|mcp__plugin_engram_engram__mem_save|mcp__engram__mem_update"

func driftedGuardEntry(extraHooks ...any) map[string]any {
	hooks := append([]any{map[string]any{"type": "command", "command": MemoryGuardCommand}}, extraHooks...)
	return map[string]any{"matcher": driftedGuardMatcher, "hooks": hooks}
}

func currentGuardEntry() map[string]any {
	return map[string]any{
		"matcher": MemoryGuardToolMatcher(),
		"hooks":   []any{map[string]any{"type": "command", "command": MemoryGuardCommand}},
	}
}

func TestRegisterMemoryGuardHook_UpgradesDriftedMatcherToExactlyOneEntry(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir()}
	foreignHook := map[string]any{"type": "command", "command": "audit-extra"}
	foreignEntry := map[string]any{"matcher": "Bash", "hooks": []any{map[string]any{"type": "command", "command": "other-tool"}}}
	writeSettingsJSON(t, cfg, map[string]any{
		"hooks": map[string]any{"PreToolUse": []any{foreignEntry, driftedGuardEntry(foreignHook)}},
	})

	for i := 0; i < 2; i++ {
		if err := RegisterMemoryGuardHook(cfg); err != nil {
			t.Fatalf("RegisterMemoryGuardHook() #%d error = %v", i+1, err)
		}
	}

	if matchers := clickGuardEntryMatchers(t, cfg); len(matchers) != 1 || matchers[0] != MemoryGuardToolMatcher() {
		t.Fatalf("click guard entries = %v, want exactly [%q] (the drifted matcher must not be orphaned)", matchers, MemoryGuardToolMatcher())
	}
	raw, _ := json.Marshal(readSettingsJSON(t, cfg))
	for _, want := range []string{"audit-extra", "other-tool"} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("upgrade dropped foreign hook %q: %s", want, raw)
		}
	}
}

func TestRegisterMemoryGuardHook_CollapsesDuplicateCurrentEntries(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir()}
	writeSettingsJSON(t, cfg, map[string]any{
		"hooks": map[string]any{"PreToolUse": []any{currentGuardEntry(), currentGuardEntry()}},
	})

	if err := RegisterMemoryGuardHook(cfg); err != nil {
		t.Fatalf("RegisterMemoryGuardHook() error = %v", err)
	}

	if matchers := clickGuardEntryMatchers(t, cfg); len(matchers) != 1 || matchers[0] != MemoryGuardToolMatcher() {
		t.Fatalf("click guard entries = %v, want exactly [%q]", matchers, MemoryGuardToolMatcher())
	}
}

func TestUnregisterMemoryGuardHook_RemovesDriftedEntry(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir()}
	foreign := map[string]any{"matcher": "Bash", "hooks": []any{map[string]any{"type": "command", "command": "other-tool"}}}
	writeSettingsJSON(t, cfg, map[string]any{
		"hooks": map[string]any{"PreToolUse": []any{driftedGuardEntry(), foreign}},
	})

	if err := UnregisterMemoryGuardHook(cfg); err != nil {
		t.Fatalf("UnregisterMemoryGuardHook() error = %v", err)
	}

	if matchers := clickGuardEntryMatchers(t, cfg); len(matchers) != 0 {
		t.Fatalf("click guard entries after uninstall = %v, want none", matchers)
	}
	if entries := getPreToolUseEntries(readSettingsJSON(t, cfg)); len(entries) != 1 {
		t.Fatalf("uninstall must keep only the foreign entry, got %v", entries)
	}
}

func TestHasLegacyMemoryGuardHook_TreatsDriftedMatcherAsOutdated(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir()}
	writeSettingsJSON(t, cfg, map[string]any{
		"hooks": map[string]any{"PreToolUse": []any{driftedGuardEntry()}},
	})

	if ok, err := HasMemoryGuardHook(cfg); err != nil || ok {
		t.Fatalf("HasMemoryGuardHook() = %v, %v; want false, nil for a drifted matcher", ok, err)
	}
	outdated, err := HasLegacyMemoryGuardHook(cfg)
	if err != nil {
		t.Fatalf("HasLegacyMemoryGuardHook() error = %v", err)
	}
	if !outdated {
		t.Fatal("HasLegacyMemoryGuardHook() = false for a Click hook under a non-current matcher, want true")
	}
}

func TestManagedSettingsProjectionHash_TreatsDriftedGuardEntryAsOwned(t *testing.T) {
	content, err := json.Marshal(map[string]any{
		"hooks": map[string]any{"PreToolUse": []any{driftedGuardEntry()}},
	})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if hash := managedSettingsProjectionHash(string(content)); !strings.HasPrefix(hash, "present:") {
		t.Fatalf("managedSettingsProjectionHash(drifted entry) = %q, want a present: projection", hash)
	}
}

func TestRegisteredMemoryGuardMatchers_ReturnsOnlyClickOwnedMatchers(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir()}
	foreign := map[string]any{"matcher": "Bash", "hooks": []any{map[string]any{"type": "command", "command": "other-tool"}}}
	writeSettingsJSON(t, cfg, map[string]any{
		"hooks": map[string]any{"PreToolUse": []any{foreign, driftedGuardEntry(), legacyGuardEntry()}},
	})

	got, err := RegisteredMemoryGuardMatchers(cfg)
	if err != nil {
		t.Fatalf("RegisteredMemoryGuardMatchers() error = %v", err)
	}
	want := []string{driftedGuardMatcher, MemoryGuardLegacyToolMatcher}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("RegisteredMemoryGuardMatchers() = %v, want %v", got, want)
	}
}

func TestRegisteredMemoryGuardMatchers_MissingSettingsReturnsNone(t *testing.T) {
	got, err := RegisteredMemoryGuardMatchers(Config{ClaudeHome: t.TempDir()})
	if err != nil || len(got) != 0 {
		t.Fatalf("RegisteredMemoryGuardMatchers() = %v, %v; want none, nil", got, err)
	}
}
