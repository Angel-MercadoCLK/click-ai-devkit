package installer

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const (
	// MemoryGuardToolMatcher must use the plugin-scoped tool name. Verify the exact scoped runtime
	// name before shipping; a bare mcp__engram__mem_save matcher never fires for a plugin-bundled MCP.
	MemoryGuardToolMatcher = "mcp__plugin_engram_engram__mem_save"
	MemoryGuardCommand     = "click memory-guard"
)

// RegisterMemoryGuardHook ensures Claude Code's PreToolUse settings invoke click memory-guard for
// Engram mem_save calls.
func RegisterMemoryGuardHook(cfg Config) error {
	settings, err := readSettingsFile(cfg.SettingsPath())
	if err != nil {
		return err
	}
	entries := getPreToolUseEntries(settings)
	if hasHookEntry(entries, MemoryGuardToolMatcher, MemoryGuardCommand) {
		return nil
	}
	entries = append(entries, map[string]any{
		"matcher": MemoryGuardToolMatcher,
		"hooks": []any{map[string]any{
			"type":    "command",
			"command": MemoryGuardCommand,
		}},
	})
	setPreToolUseEntries(settings, entries)
	return writeSettingsFile(cfg.SettingsPath(), settings)
}

// UnregisterMemoryGuardHook removes the managed click memory-guard PreToolUse hook entry.
func UnregisterMemoryGuardHook(cfg Config) error {
	settings, err := readSettingsFile(cfg.SettingsPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	entries := getPreToolUseEntries(settings)
	filtered := make([]any, 0, len(entries))
	for _, raw := range entries {
		entry, ok := raw.(map[string]any)
		if !ok {
			filtered = append(filtered, raw)
			continue
		}
		matcher, _ := entry["matcher"].(string)
		hooks, _ := entry["hooks"].([]any)
		if matcher != MemoryGuardToolMatcher {
			filtered = append(filtered, raw)
			continue
		}

		remainingHooks := make([]any, 0, len(hooks))
		for _, hookRaw := range hooks {
			hook, ok := hookRaw.(map[string]any)
			if !ok {
				remainingHooks = append(remainingHooks, hookRaw)
				continue
			}
			if hook["type"] == "command" && hook["command"] == MemoryGuardCommand {
				continue
			}
			remainingHooks = append(remainingHooks, hookRaw)
		}
		if len(remainingHooks) == 0 {
			continue
		}
		entry["hooks"] = remainingHooks
		filtered = append(filtered, entry)
	}
	setPreToolUseEntries(settings, filtered)
	return writeSettingsFile(cfg.SettingsPath(), settings)
}

// HasMemoryGuardHook reports whether the managed PreToolUse hook is registered.
func HasMemoryGuardHook(cfg Config) (bool, error) {
	settings, err := readSettingsFile(cfg.SettingsPath())
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return hasHookEntry(getPreToolUseEntries(settings), MemoryGuardToolMatcher, MemoryGuardCommand), nil
}

// writeJSONFile is a small shared helper for the handful of Click-managed JSON files this package
// writes (per-phase models, Engram state, ...) that aren't Claude Code's own settings.json shape.
func writeJSONFile(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o600)
}

func readSettingsFile(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{}, nil
		}
		return nil, fmt.Errorf("installer: read settings: %w", err)
	}
	if len(data) == 0 {
		return map[string]any{}, nil
	}
	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		return nil, fmt.Errorf("installer: parse settings: %w", err)
	}
	if settings == nil {
		settings = map[string]any{}
	}
	return settings, nil
}

func writeSettingsFile(path string, settings map[string]any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("installer: create settings dir: %w", err)
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("installer: marshal settings: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("installer: write settings: %w", err)
	}
	return nil
}

func getPreToolUseEntries(settings map[string]any) []any {
	hooks, ok := settings["hooks"].(map[string]any)
	if !ok {
		return nil
	}
	entries, _ := hooks["PreToolUse"].([]any)
	return entries
}

func setPreToolUseEntries(settings map[string]any, entries []any) {
	hooks, ok := settings["hooks"].(map[string]any)
	if !ok || hooks == nil {
		hooks = map[string]any{}
	}
	if len(entries) == 0 {
		delete(hooks, "PreToolUse")
	} else {
		hooks["PreToolUse"] = entries
	}
	if len(hooks) == 0 {
		delete(settings, "hooks")
		return
	}
	settings["hooks"] = hooks
}

func hasHookEntry(entries []any, matcher, command string) bool {
	for _, raw := range entries {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if entry["matcher"] != matcher {
			continue
		}
		hooks, _ := entry["hooks"].([]any)
		for _, hookRaw := range hooks {
			hook, ok := hookRaw.(map[string]any)
			if !ok {
				continue
			}
			if hook["type"] == "command" && hook["command"] == command {
				return true
			}
		}
	}
	return false
}
