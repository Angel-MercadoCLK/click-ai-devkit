package installer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/manifest"
)

// CloudTokenPersistence represents whether the token should be persisted to settings.json
// based on user consent.
type CloudTokenPersistence int

const (
	// CloudTokenPersistencePersist means the user gave affirmative consent to store the token.
	CloudTokenPersistencePersist CloudTokenPersistence = iota
	// CloudTokenPersistenceDecline means the user declined token persistence.
	CloudTokenPersistenceDecline
	// CloudTokenPersistenceNoOp means no token is available, so no consent decision is possible.
	CloudTokenPersistenceNoOp
)

// ConfigureEngramCloudSessionSync writes the Engram Cloud environment and SessionStart hook
// to Claude Code's settings.json. It performs one read/merge/write through the secured
// writeSettingsFile, preserving all foreign entries.
func ConfigureEngramCloudSessionSync(cfg Config, m *manifest.Manifest, mode CloudTokenPersistence, token string) error {
	settings, err := readSettingsFile(cfg.SettingsPath())
	if err != nil {
		return fmt.Errorf("installer: read settings: %w", err)
	}

	// Capture the original canonical form for idempotency check
	originalBytes, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("installer: marshal original settings: %w", err)
	}
	originalBytes = append(originalBytes, '\n')

	// Ensure env block exists
	env, ok := settings["env"].(map[string]any)
	if !ok || env == nil {
		env = map[string]any{}
	}

	// Merge click-owned env keys (selective per-entry, not whole-key like PruneEmptyClickSettingsKeys)
	env["ENGRAM_CLOUD_AUTOSYNC"] = "1"
	env["ENGRAM_CLOUD_SERVER"] = m.EngramCloud.Server

	// Handle token based on persistence mode
	if mode == CloudTokenPersistencePersist {
		env["ENGRAM_CLOUD_TOKEN"] = token
	} else if mode == CloudTokenPersistenceDecline {
		// Remove click-owned token in decline mode, preserving foreign entries
		delete(env, "ENGRAM_CLOUD_TOKEN")
	}
	// NoOp mode: don't touch the token at all

	settings["env"] = env

	// Register managed SessionStart hook
	managedHookCmd, err := managedEngramCloudHookCommand(m.EngramCloud.Project)
	if err != nil {
		return fmt.Errorf("installer: generate managed hook command: %w", err)
	}

	// Ensure hooks block exists
	hooks, ok := settings["hooks"].(map[string]any)
	if !ok || hooks == nil {
		hooks = map[string]any{}
		settings["hooks"] = hooks
	}

	// Get or create SessionStart entries
	sessionStart, ok := hooks["SessionStart"].([]any)
	if !ok || sessionStart == nil {
		sessionStart = []any{}
	}

	// Remove any existing managed hooks (matcher "" with our command)
	filteredSessionStart := make([]any, 0, len(sessionStart))
	for _, rawEntry := range sessionStart {
		entry, ok := rawEntry.(map[string]any)
		if !ok {
			filteredSessionStart = append(filteredSessionStart, rawEntry)
			continue
		}

		matcher, _ := entry["matcher"].(string)
		if matcher != "" {
			// Not a managed hook - preserve it
			filteredSessionStart = append(filteredSessionStart, rawEntry)
			continue
		}

		// This is a managed hook - check if it has our command
		hooksList, ok := entry["hooks"].([]any)
		if !ok {
			filteredSessionStart = append(filteredSessionStart, rawEntry)
			continue
		}

		hasManagedCmd := false
		for _, rawHook := range hooksList {
			hook, ok := rawHook.(map[string]any)
			if !ok {
				continue
			}
			hookType, _ := hook["type"].(string)
			hookCmd, _ := hook["command"].(string)
			if hookType == "command" && hookCmd == managedHookCmd {
				hasManagedCmd = true
				break
			}
		}

		if !hasManagedCmd {
			// Different managed hook (old command) - preserve it as foreign
			filteredSessionStart = append(filteredSessionStart, rawEntry)
		}
		// If hasManagedCmd, we skip this entry (we'll add a fresh one)
	}

	// Append exactly one new managed hook entry
	managedEntry := map[string]any{
		"matcher": "",
		"hooks": []any{
			map[string]any{
				"type":    "command",
				"command": managedHookCmd,
			},
		},
	}
	filteredSessionStart = append(filteredSessionStart, managedEntry)
	hooks["SessionStart"] = filteredSessionStart

	// Capture the new canonical form
	newBytes, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("installer: marshal new settings: %w", err)
	}
	newBytes = append(newBytes, '\n')

	// Short-circuit: no write when the document is already canonical
	if bytes.Equal(originalBytes, newBytes) {
		return nil
	}

	return writeSettingsFile(cfg.SettingsPath(), settings)
}

// RemoveEngramCloudSessionSync removes click-owned Engram Cloud environment and managed hooks
// from Claude Code's settings.json. It performs one selective atomic rewrite, preserving all
// foreign entries and pruning empty containers.
func RemoveEngramCloudSessionSync(cfg Config) error {
	settings, err := readSettingsFile(cfg.SettingsPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("installer: read settings: %w", err)
	}

	changed := false

	// Remove click-owned env keys (selective, not whole-key like PruneEmptyClickSettingsKeys)
	env, ok := settings["env"].(map[string]any)
	if ok {
		if _, present := env["ENGRAM_CLOUD_AUTOSYNC"]; present {
			delete(env, "ENGRAM_CLOUD_AUTOSYNC")
			changed = true
		}
		if _, present := env["ENGRAM_CLOUD_SERVER"]; present {
			delete(env, "ENGRAM_CLOUD_SERVER")
			changed = true
		}
		if _, present := env["ENGRAM_CLOUD_TOKEN"]; present {
			delete(env, "ENGRAM_CLOUD_TOKEN")
			changed = true
		}

		// Prune env block when empty
		if len(env) == 0 {
			delete(settings, "env")
		}
	}

	// Remove managed SessionStart hooks
	hooks, ok := settings["hooks"].(map[string]any)
	if ok {
		sessionStart, ok := hooks["SessionStart"].([]any)
		if ok {
			filteredSessionStart := make([]any, 0, len(sessionStart))
			for _, rawEntry := range sessionStart {
				entry, ok := rawEntry.(map[string]any)
				if !ok {
					filteredSessionStart = append(filteredSessionStart, rawEntry)
					continue
				}

				matcher, _ := entry["matcher"].(string)
				if matcher == "" {
					// This is a managed hook - remove it (check command to be sure)
					entryHooks, _ := entry["hooks"].([]any)
					isManaged := false
					for _, hookRaw := range entryHooks {
						hook, ok := hookRaw.(map[string]any)
						if !ok {
							continue
						}
						if hook["type"] == "command" {
							cmd, _ := hook["command"].(string)
							// Check if this is our managed command (contains click engram-cloud-import or timeout engram sync)
							if cmd != "" && (strings.Contains(cmd, "click engram-cloud-import") ||
								strings.Contains(cmd, "timeout 5 engram sync --cloud --import")) {
								isManaged = true
								break
							}
						}
					}
					if isManaged {
						changed = true
						continue // Skip this entry (remove it)
					}
				}

				// Foreign hook - preserve it
				filteredSessionStart = append(filteredSessionStart, rawEntry)
			}

			if len(filteredSessionStart) != len(sessionStart) {
				hooks["SessionStart"] = filteredSessionStart
			}

			// Prune SessionStart container when empty
			if len(filteredSessionStart) == 0 {
				delete(hooks, "SessionStart")
			}

			// Prune hooks block when empty
			if len(hooks) == 0 {
				delete(settings, "hooks")
			}
		}
	}

	// Short-circuit: no write when nothing changed
	if !changed {
		return nil
	}

	return writeSettingsFile(cfg.SettingsPath(), settings)
}