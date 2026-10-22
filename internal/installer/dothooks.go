package installer

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
)

// Click Dot agent companion hooks (odd/tasks/dot-agent-companion.md, S6 and the wire contract).
//
// On Windows, when Click Dot is part of the install, Claude Code's settings.json gets one hook group
// per event with no matcher, {"type":"command","command":"click dot-hook <Event>","timeout":<t>}.
// Ownership is decided by the `click dot-hook` command prefix only, so Click's memory guard, the
// Engram Cloud import hook and every third-party hook are never touched. The hidden `click dot-hook`
// command fails open, so no entry ever uses onFailure.

// DotHookCommandPrefix is the command signature that marks a hook entry as Click Dot's.
const DotHookCommandPrefix = "click dot-hook"

const (
	dotHookPermissionTimeoutSeconds = 120
	dotHookEventTimeoutSeconds      = 10
)

type dotHookEvent struct {
	name    string
	timeout int
}

// dotHookEvents is the single source of truth for the registered events and their timeouts.
var dotHookEvents = []dotHookEvent{
	{"PermissionRequest", dotHookPermissionTimeoutSeconds},
	{"SessionStart", dotHookEventTimeoutSeconds},
	{"SessionEnd", dotHookEventTimeoutSeconds},
	{"UserPromptSubmit", dotHookEventTimeoutSeconds},
	{"PreToolUse", dotHookEventTimeoutSeconds},
	{"PostToolUse", dotHookEventTimeoutSeconds},
	{"PostToolUseFailure", dotHookEventTimeoutSeconds},
	{"Notification", dotHookEventTimeoutSeconds},
	{"Stop", dotHookEventTimeoutSeconds},
}

func (e dotHookEvent) command() string { return DotHookCommandPrefix + " " + e.name }

func (e dotHookEvent) group() map[string]any {
	return map[string]any{
		"hooks": []any{map[string]any{
			"type":    "command",
			"command": e.command(),
			"timeout": e.timeout,
		}},
	}
}

func isManagedDotHookEvent(name string) bool {
	for _, event := range dotHookEvents {
		if event.name == name {
			return true
		}
	}
	return false
}

// isClickDotHook reports whether a hook entry is owned by Click Dot, by command signature.
func isClickDotHook(raw any) bool {
	hook, ok := raw.(map[string]any)
	if !ok || hook["type"] != "command" {
		return false
	}
	command, _ := hook["command"].(string)
	return command == DotHookCommandPrefix || strings.HasPrefix(command, DotHookCommandPrefix+" ")
}

func numberEquals(value any, want int) bool {
	switch v := value.(type) {
	case float64:
		return v == float64(want)
	case int:
		return v == want
	case json.Number:
		n, err := v.Int64()
		return err == nil && n == int64(want)
	}
	return false
}

// matchesDotHook reports whether raw is exactly the canonical hook for event.
func (e dotHookEvent) matches(raw any) bool {
	hook, ok := raw.(map[string]any)
	if !ok || !isClickDotHook(hook) {
		return false
	}
	return hook["command"] == e.command() && numberEquals(hook["timeout"], e.timeout)
}

func entryHasNoMatcher(entry map[string]any) bool {
	matcher, present := entry["matcher"]
	if !present {
		return true
	}
	text, isString := matcher.(string)
	return isString && text == ""
}

// removeClickDotHooks removes Click Dot's hook from every entry of one event list, except where keep
// reports true for that hook (called once per Click Dot hook found, in settings order). Foreign hooks
// sharing a group are kept; a group left with no hooks is dropped. It reports whether anything was
// removed.
func removeClickDotHooks(entries []any, keep func(entry map[string]any, hook any) bool) ([]any, bool) {
	filtered := make([]any, 0, len(entries))
	removed := false
	for _, raw := range entries {
		entry, ok := raw.(map[string]any)
		if !ok {
			filtered = append(filtered, raw)
			continue
		}
		hooks, _ := entry["hooks"].([]any)
		remaining := make([]any, 0, len(hooks))
		for _, hookRaw := range hooks {
			if isClickDotHook(hookRaw) && !keep(entry, hookRaw) {
				removed = true
				continue
			}
			remaining = append(remaining, hookRaw)
		}
		if len(remaining) == len(hooks) {
			filtered = append(filtered, raw)
			continue
		}
		if len(remaining) == 0 {
			continue
		}
		entry["hooks"] = remaining
		filtered = append(filtered, entry)
	}
	return filtered, removed
}

// hooksObject returns settings["hooks"] as a map, or an error when it holds something else (never
// clobbered). A missing key yields nil.
func hooksObject(settings map[string]any) (map[string]any, error) {
	raw, present := settings["hooks"]
	if !present {
		return nil, nil
	}
	hooks, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("installer: la clave hooks de settings.json no es un objeto; no se modifica")
	}
	return hooks, nil
}

// RegisterDotHooks ensures settings.json carries exactly one Click Dot hook group per managed event.
// It is idempotent and writes nothing when the file is already canonical. Duplicates, a wrong
// timeout, a matcher-scoped copy and Click Dot hooks under events no longer managed are normalized;
// every other entry is preserved untouched.
func RegisterDotHooks(cfg Config) error {
	settings, err := readSettingsFile(cfg.SettingsPath())
	if err != nil {
		return err
	}
	hooks, err := hooksObject(settings)
	if err != nil {
		return err
	}
	changed := false

	// Click Dot hooks under events this version no longer manages are stale: drop them.
	var stale []string
	for event := range hooks {
		if !isManagedDotHookEvent(event) {
			stale = append(stale, event)
		}
	}
	for _, event := range stale {
		entries, ok := hooks[event].([]any)
		if !ok {
			continue
		}
		filtered, removed := removeClickDotHooks(entries, func(map[string]any, any) bool { return false })
		if removed {
			changed = true
			setHookEntries(settings, event, filtered)
		}
	}

	for _, event := range dotHookEvents {
		kept := false
		entries, removed := removeClickDotHooks(getHookEntries(settings, event.name), func(entry map[string]any, hook any) bool {
			if !kept && entryHasNoMatcher(entry) && event.matches(hook) {
				kept = true
				return true
			}
			return false
		})
		if removed {
			changed = true
		}
		if !kept {
			entries = append(entries, event.group())
			changed = true
		}
		setHookEntries(settings, event.name, entries)
	}
	if !changed {
		return nil
	}
	return writeSettingsFile(cfg.SettingsPath(), settings)
}

// UnregisterDotHooks removes every Click Dot hook, whatever its event, and prunes the keys it
// empties. Nothing is written when no Click Dot hook is present.
func UnregisterDotHooks(cfg Config) error {
	settings, err := readSettingsFile(cfg.SettingsPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	hooks, err := hooksObject(settings)
	if err != nil {
		return err
	}
	var events []string
	for event := range hooks {
		events = append(events, event)
	}
	changed := false
	for _, event := range events {
		entries, ok := hooks[event].([]any)
		if !ok {
			continue
		}
		filtered, removed := removeClickDotHooks(entries, func(map[string]any, any) bool { return false })
		if removed {
			changed = true
			setHookEntries(settings, event, filtered)
		}
	}
	if !changed {
		return nil
	}
	return writeSettingsFile(cfg.SettingsPath(), settings)
}

// DotHooksApplicable reports whether the Click Dot hooks belong in this install or update: Click Dot
// is supported here (Windows amd64) and is either about to be installed from a release build or is
// already installed. Nothing is ever registered on other operating systems.
func DotHooksApplicable(clickVersion string) bool {
	manager := dotManagerFactory()
	if !manager.Supported() {
		return false
	}
	if DotReleaseVersion(clickVersion) != "" {
		return true
	}
	return manager.Inspect().ExePresent
}

type savedDotHook struct {
	event      string
	matcher    any
	hasMatcher bool
	hook       any
}

// clickDotHooks lists every Click Dot hook in settings with the group matcher it lives under.
func clickDotHooks(settings map[string]any) []savedDotHook {
	var result []savedDotHook
	hooks, _ := settings["hooks"].(map[string]any)
	for _, event := range dotHookEvents {
		entries, _ := hooks[event.name].([]any)
		for _, raw := range entries {
			entry, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			list, _ := entry["hooks"].([]any)
			for _, hook := range list {
				if isClickDotHook(hook) {
					matcher, has := entry["matcher"]
					result = append(result, savedDotHook{event: event.name, matcher: matcher, hasMatcher: has, hook: hook})
				}
			}
		}
	}
	return result
}

// restoreClickDotHooks makes currentSettings carry exactly the Click Dot hooks backupSettings has:
// ours are removed from every event, then the backup's are merged back into a group with the same
// matcher (or a new one). Foreign entries are never touched.
func restoreClickDotHooks(currentSettings, backupSettings map[string]any) error {
	hooks, _ := currentSettings["hooks"].(map[string]any)
	var events []string
	for event := range hooks {
		events = append(events, event)
	}
	for _, event := range events {
		entries, ok := hooks[event].([]any)
		if !ok {
			continue
		}
		filtered, removed := removeClickDotHooks(entries, func(map[string]any, any) bool { return false })
		if removed {
			setHookEntries(currentSettings, event, filtered)
		}
	}
	for _, saved := range clickDotHooks(backupSettings) {
		entries := getHookEntries(currentSettings, saved.event)
		merged := false
		for _, raw := range entries {
			entry, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			matcher, has := entry["matcher"]
			if has != saved.hasMatcher || (has && !reflect.DeepEqual(matcher, saved.matcher)) {
				continue
			}
			list, ok := entry["hooks"].([]any)
			if !ok {
				return fmt.Errorf("installer: la estructura de hooks de settings.json no es válida")
			}
			entry["hooks"] = append(list, saved.hook)
			merged = true
			break
		}
		if !merged {
			group := map[string]any{"hooks": []any{saved.hook}}
			if saved.hasMatcher {
				group["matcher"] = saved.matcher
			}
			setHookEntries(currentSettings, saved.event, append(entries, group))
		}
	}
	return nil
}
