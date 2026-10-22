package installer

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/guard"
)

const (
	MemoryGuardCommand = "click memory-guard"

	// MemoryGuardLegacyToolMatcher is the mem_save-only matcher shipped before the guard covered
	// every persisting Engram tool. Installs that still hold it are upgraded in place and the
	// entry is recognized (never left behind) on uninstall, doctor and snapshot projection.
	MemoryGuardLegacyToolMatcher = "mcp__plugin_engram_engram__mem_save"
)

// memoryGuardToolMatcher is computed once from internal/guard's single source of truth. It is
// unexported and only readable through MemoryGuardToolMatcher so no caller can reassign it.
var memoryGuardToolMatcher = guard.ToolMatcherPattern()

// MemoryGuardToolMatcher returns the PreToolUse matcher registered for the guard: an anchored
// regular expression derived from internal/guard's single source of truth, so the hook fires for
// exactly the Engram tools the guard screens (every persisting tool, plugin and standalone MCP name
// form). Claude Code treats a matcher with regex metacharacters as a regular expression; a live
// session confirmed it fires for the persisting tools and not for read-only ones.
func MemoryGuardToolMatcher() string { return memoryGuardToolMatcher }

// clickSettingsAllowlist are the top-level keys that click manages and may prune when empty after
// uninstall. The external `claude` CLI subprocess removes click's entries from these objects but
// leaves them as empty maps; click cannot prevent that, but it can prune them afterward.
var clickSettingsAllowlist = []string{"enabledPlugins", "extraKnownMarketplaces", "pluginConfigs"}

// RegisterMemoryGuardHook ensures Claude Code's PreToolUse settings invoke click memory-guard for
// every Engram tool that persists content. It is idempotent. Click's guard hooks are identified by
// their command (MemoryGuardCommand), not by the matcher string, so an install holding the legacy
// mem_save-only matcher, a hand-edited matcher, or duplicate entries is normalized to exactly one
// Click guard hook under the current matcher. Foreign hooks (including ones sharing such an entry)
// are never touched.
func RegisterMemoryGuardHook(cfg Config) error {
	settings, err := readSettingsFile(cfg.SettingsPath())
	if err != nil {
		return err
	}
	current := MemoryGuardToolMatcher()
	kept := false
	// Keep the first Click guard hook already under the current matcher; drop every other one.
	entries, changed := removeClickGuardHooks(getPreToolUseEntries(settings), func(matcher string) bool {
		if matcher == current && !kept {
			kept = true
			return true
		}
		return false
	})
	if kept && !changed {
		return nil
	}
	if !kept {
		entries = append(entries, map[string]any{
			"matcher": current,
			"hooks": []any{map[string]any{
				"type":    "command",
				"command": MemoryGuardCommand,
			}},
		})
	}
	setPreToolUseEntries(settings, entries)
	return writeSettingsFile(cfg.SettingsPath(), settings)
}

// UnregisterMemoryGuardHook removes every Click memory-guard PreToolUse hook, whatever its matcher.
func UnregisterMemoryGuardHook(cfg Config) error {
	settings, err := readSettingsFile(cfg.SettingsPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	filtered, _ := removeClickGuardHooks(getPreToolUseEntries(settings), func(string) bool { return false })
	setPreToolUseEntries(settings, filtered)
	return writeSettingsFile(cfg.SettingsPath(), settings)
}

// removeClickGuardHooks removes Click's memory-guard command hook from every PreToolUse entry,
// regardless of its matcher, except where keep(matcher) reports true for that hook (keep is called
// once per Click hook found, in settings order). Foreign hooks sharing an entry are kept; an entry
// left with no hooks is dropped. It reports whether anything was removed.
func removeClickGuardHooks(entries []any, keep func(matcher string) bool) ([]any, bool) {
	filtered := make([]any, 0, len(entries))
	removed := false
	for _, raw := range entries {
		entry, ok := raw.(map[string]any)
		if !ok {
			filtered = append(filtered, raw)
			continue
		}
		matcher, _ := entry["matcher"].(string)
		hooks, _ := entry["hooks"].([]any)

		remainingHooks := make([]any, 0, len(hooks))
		for _, hookRaw := range hooks {
			if isClickGuardHook(hookRaw) && !keep(matcher) {
				removed = true
				continue
			}
			remainingHooks = append(remainingHooks, hookRaw)
		}
		if len(remainingHooks) == len(hooks) {
			filtered = append(filtered, raw)
			continue
		}
		if len(remainingHooks) == 0 {
			continue
		}
		entry["hooks"] = remainingHooks
		filtered = append(filtered, entry)
	}
	return filtered, removed
}

// HasMemoryGuardHook reports whether the managed PreToolUse hook is registered with the current
// matcher, i.e. covering every persisting Engram tool. A legacy mem_save-only entry does not count.
func HasMemoryGuardHook(cfg Config) (bool, error) {
	settings, err := readSettingsFile(cfg.SettingsPath())
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return hasHookEntry(getPreToolUseEntries(settings), MemoryGuardToolMatcher(), MemoryGuardCommand), nil
}

// HasLegacyMemoryGuardHook reports whether a Click guard hook is registered under a matcher other
// than the current one (the legacy mem_save-only matcher or a hand-edited one), which lets doctor
// distinguish an outdated install from a missing hook.
func HasLegacyMemoryGuardHook(cfg Config) (bool, error) {
	matchers, err := RegisteredMemoryGuardMatchers(cfg)
	if err != nil {
		return false, err
	}
	for _, matcher := range matchers {
		if matcher != MemoryGuardToolMatcher() {
			return true, nil
		}
	}
	return false, nil
}

// PruneEmptyClickSettingsKeys removes the three Click-managed top-level keys (enabledPlugins,
// extraKnownMarketplaces, pluginConfigs) from settings.json ONLY when each is an empty map, which
// is the state the external `claude` CLI subprocess leaves after removing click's entries. In
// runUninstall this runs after the whole plan-driven teardown loop, so every possible `claude`
// subprocess invocation has already happened — but it is not literally the last thing uninstall
// does; post-run snapshot recording follows it. Keys that are null, arrays, strings, numbers,
// booleans, or non-empty objects are preserved untouched; any key not in the allowlist is never
// touched. If all keys are removed leaving an empty document, writes {} instead of deleting the
// file. Performs no write at all when nothing changed.
func PruneEmptyClickSettingsKeys(cfg Config) error {
	settings, err := readSettingsFile(cfg.SettingsPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	changed := false
	for _, key := range clickSettingsAllowlist {
		value, ok := settings[key]
		if !ok {
			continue
		}
		// Delete ONLY when the value is an empty map
		if valueMap, ok := value.(map[string]any); ok && len(valueMap) == 0 {
			delete(settings, key)
			changed = true
		}
	}

	// Early return: no write when nothing changed
	if !changed {
		return nil
	}

	return writeSettingsFile(cfg.SettingsPath(), settings)
}

// writeJSONFile is a small shared helper for the handful of Click-managed JSON files this package
// writes (per-phase models, Engram state, ...) that aren't Claude Code's own settings.json shape.
//
// Writes go through atomicWriteFile (pathenv.go) rather than a direct os.WriteFile, reusing the
// SAME symlink-resolving, temp-file+rename helper already relied on for shell rc files instead of
// a second parallel implementation — that inconsistency (this helper writing non-atomically,
// straight through/over a symlink) is exactly how the symlink-safety finding this fixes came to
// exist in the first place. Every caller that goes THROUGH writeJSONFile (engram.go's
// engramState/plugins.go/models.go's SaveModelsWithProfile/context7.go/profile_artifacts.go's
// SaveProfileArtifact) gets the fix automatically without needing its own change. It does NOT cover
// every write in those same files, though: models.go's MigrateIfStale (the models.json.bak backup)
// and profile_artifacts.go's SaveMarkdownAgent (the generated agent .md) each had their own separate
// os.WriteFile call sites that bypassed this helper entirely — those two were fixed directly by
// routing them through atomicWriteFile at their own call sites, not through this function.
func writeJSONFile(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return atomicWriteFile(path, data, 0o600)
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

// writeSettingsFile writes Claude Code's settings.json via atomicWriteFileOwnerOnly (pathenv.go)
// instead of a direct os.WriteFile — see writeJSONFile's doc comment above for why: settings.json
// is exactly the file a dotfiles-managed ~/.claude/ commonly symlinks, and a non-atomic write
// through/over it is the concrete risk this fixes (broken symlink at best, a corrupted file if
// interrupted mid-write at worst). The owner-only security is applied because settings.json may
// contain sensitive data like ENGRAM_CLOUD_TOKEN in future slices.
func writeSettingsFile(path string, settings map[string]any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("installer: create settings dir: %w", err)
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("installer: marshal settings: %w", err)
	}
	data = append(data, '\n')
	if err := atomicWriteFileOwnerOnly(path, data); err != nil {
		return fmt.Errorf("installer: write settings: %w", err)
	}
	return nil
}

func getPreToolUseEntries(settings map[string]any) []any {
	return getHookEntries(settings, "PreToolUse")
}

func setPreToolUseEntries(settings map[string]any, entries []any) {
	setHookEntries(settings, "PreToolUse", entries)
}

// getHookEntries returns the entry list registered for one hook event, or nil when absent.
func getHookEntries(settings map[string]any, event string) []any {
	hooks, ok := settings["hooks"].(map[string]any)
	if !ok {
		return nil
	}
	entries, _ := hooks[event].([]any)
	return entries
}

// setHookEntries stores one hook event's entry list, pruning the event key when the list is empty
// and the whole hooks key when nothing remains.
func setHookEntries(settings map[string]any, event string, entries []any) {
	hooks, ok := settings["hooks"].(map[string]any)
	if !ok || hooks == nil {
		hooks = map[string]any{}
	}
	if len(entries) == 0 {
		delete(hooks, event)
	} else {
		hooks[event] = entries
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

// RegisteredMemoryGuardMatchers returns the PreToolUse matcher of every entry that carries Click's
// memory-guard command, in settings order. Ownership is decided by the command, not the matcher
// string, so a hand-edited or older Click matcher is still reported (doctor probes what is actually
// registered). A missing settings file yields none.
func RegisteredMemoryGuardMatchers(cfg Config) ([]string, error) {
	settings, err := readSettingsFile(cfg.SettingsPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var matchers []string
	for _, raw := range getPreToolUseEntries(settings) {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		hooks, _ := entry["hooks"].([]any)
		for _, hookRaw := range hooks {
			if isClickGuardHook(hookRaw) {
				matcher, _ := entry["matcher"].(string)
				matchers = append(matchers, matcher)
				break
			}
		}
	}
	return matchers, nil
}

func isClickGuardHook(raw any) bool {
	hook, ok := raw.(map[string]any)
	return ok && hook["type"] == "command" && hook["command"] == MemoryGuardCommand
}
