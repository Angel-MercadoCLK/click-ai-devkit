package installer

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/manifest"
)

// isManagedEngramCloudHookCommand reports whether a command matches click's canonical
// Engram Cloud managed hook signature, regardless of the project name it contains.
// Per DD-2, identity is by signature (shape), not exact string equality.
func isManagedEngramCloudHookCommand(cmd string) bool {
	if cmd == "" {
		return false
	}

	// Legacy POSIX signature retained for migration; remove it once no installed
	// Click client can still carry the timeout-based hook.
	legacyPOSIXPattern := regexp.MustCompile(`^timeout 5 engram sync --cloud --import --project (?:[^ ]+|'(?:[^']|'\\'')*') \|\| true$`)
	if legacyPOSIXPattern.MatchString(cmd) {
		return true
	}

	// Unified POSIX signature: click engram-cloud-import --project-b64 <base64url> || true.
	posixPattern := regexp.MustCompile(`^click engram-cloud-import --project-b64 [A-Za-z0-9_-]+ \|\| true$`)
	if posixPattern.MatchString(cmd) {
		return true
	}

	// Unified Windows signature: cmd.exe /d /s /c "click engram-cloud-import --project-b64 <base64url> & exit /b 0".
	windowsPattern := regexp.MustCompile(`^cmd\.exe /d /s /c "click engram-cloud-import --project-b64 [A-Za-z0-9_-]+ & exit /b 0"$`)
	if windowsPattern.MatchString(cmd) {
		return true
	}

	return false
}

// extractProjectFromHookCommand extracts the project name from a hook command.
// Returns empty string if extraction fails.
func extractProjectFromHookCommand(cmd string) string {
	// POSIX: timeout 5 engram sync --cloud --import --project <project> || true
	if strings.HasPrefix(cmd, "timeout 5 engram sync --cloud --import --project ") {
		project := strings.TrimSuffix(strings.TrimPrefix(cmd, "timeout 5 engram sync --cloud --import --project "), " || true")
		if strings.HasPrefix(project, "'") && strings.HasSuffix(project, "'") {
			return strings.ReplaceAll(project[1:len(project)-1], "'\\''", "'")
		}
		return project
	}

	// POSIX and Windows unified wrappers carry the project in the same base64url argument.
	if strings.HasPrefix(cmd, "click engram-cloud-import --project-b64 ") || strings.HasPrefix(cmd, "cmd.exe /d /s /c \"click engram-cloud-import --project-b64 ") {
		// Extract the base64 project
		start := strings.Index(cmd, "--project-b64 ")
		if start == -1 {
			return ""
		}
		start += len("--project-b64 ")
		end := strings.Index(cmd[start:], " ")
		if end == -1 {
			return ""
		}
		b64Project := cmd[start : start+end]
		// Decode base64 to get the actual project name
		decoded, err := base64.RawURLEncoding.DecodeString(b64Project)
		if err != nil {
			return ""
		}
		return string(decoded)
	}

	return ""
}

var configureEngramCloudSessionSyncFunc = configureEngramCloudSessionSyncImpl

func SetConfigureEngramCloudSessionSyncFuncForTests(fn func(Config, *manifest.Manifest, CloudTokenPersistence, string) error) func() {
	old := configureEngramCloudSessionSyncFunc
	configureEngramCloudSessionSyncFunc = fn
	return func() { configureEngramCloudSessionSyncFunc = old }
}

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

// CloudSessionSyncStatus is the local managed-session-sync footprint observed in settings.json.
// It is intentionally limited to presence and integrity metadata; it never exposes secret values.
type CloudSessionSyncStatus struct {
	AutosyncPresent     bool
	AutosyncValid       bool
	ServerPresent       bool
	ServerValid         bool
	TokenPresent        bool
	TokenValid          bool
	ManagedHookPresent  bool
	ManagedHookValid    bool
	HookPayloadInvalid  bool
	HookProjectMismatch bool
	OwnerOnly           bool
}

// HasManagedFootprint reports whether Click's cloud session-sync configuration is present at all.
func (s CloudSessionSyncStatus) HasManagedFootprint() bool {
	return s.AutosyncPresent || s.ServerPresent || s.TokenPresent || s.ManagedHookPresent
}

// InspectEngramCloudSessionSync reads settings.json and its local permissions without mutating
// settings or invoking any external process.
func InspectEngramCloudSessionSync(cfg Config) (CloudSessionSyncStatus, error) {
	path := cfg.SettingsPath()
	settings, err := readSettingsFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return CloudSessionSyncStatus{}, nil
		}
		return CloudSessionSyncStatus{}, fmt.Errorf("installer: read settings: %w", err)
	}

	status := CloudSessionSyncStatus{}
	if env, ok := settings["env"].(map[string]any); ok {
		_, status.AutosyncPresent = env["ENGRAM_CLOUD_AUTOSYNC"]
		if status.AutosyncPresent {
			if autosync, ok := env["ENGRAM_CLOUD_AUTOSYNC"].(string); ok && autosync == "1" {
				status.AutosyncValid = true
			}
		}

		_, status.ServerPresent = env["ENGRAM_CLOUD_SERVER"]
		if status.ServerPresent {
			if server, ok := env["ENGRAM_CLOUD_SERVER"].(string); ok && server != "" && strings.Contains(server, "://") {
				status.ServerValid = true
			}
		}

		_, status.TokenPresent = env["ENGRAM_CLOUD_TOKEN"]
		if status.TokenPresent {
			if token, ok := env["ENGRAM_CLOUD_TOKEN"].(string); ok && token != "" && token != "[REDACTED]" {
				status.TokenValid = true
			}
		}
	}

	m, err := manifest.Load()
	if err != nil {
		return CloudSessionSyncStatus{}, fmt.Errorf("installer: load manifest: %w", err)
	}
	_, resolvedProject, _ := resolveEngramCloudConfig(cfg, m)

	if hooks, ok := settings["hooks"].(map[string]any); ok {
		if sessionStart, ok := hooks["SessionStart"].([]any); ok {
			for _, rawEntry := range sessionStart {
				entry, ok := rawEntry.(map[string]any)
				if !ok || entry["matcher"] != "" {
					continue
				}
				entryHooks, _ := entry["hooks"].([]any)
				for _, rawHook := range entryHooks {
					hook, ok := rawHook.(map[string]any)
					if !ok || hook["type"] != "command" {
						continue
					}
					command, _ := hook["command"].(string)
					if isManagedEngramCloudHookCandidate(command) {
						hookProject := extractProjectFromHookCommand(command)
						if hookProject == "" {
							status.HookPayloadInvalid = true
						}
						if resolvedProject != "" && hookProject != resolvedProject {
							status.HookProjectMismatch = true
						}
					}
				}
			}
		}
	}

	if hooks, ok := settings["hooks"].(map[string]any); ok {
		if sessionStart, ok := hooks["SessionStart"].([]any); ok {
			for _, rawEntry := range sessionStart {
				entry, ok := rawEntry.(map[string]any)
				if !ok || entry["matcher"] != "" {
					continue
				}
				entryHooks, _ := entry["hooks"].([]any)
				for _, rawHook := range entryHooks {
					hook, ok := rawHook.(map[string]any)
					if !ok || hook["type"] != "command" {
						continue
					}
					command, _ := hook["command"].(string)
					if isManagedEngramCloudHookCandidate(command) {
						status.ManagedHookPresent = true
						if isManagedEngramCloudHookCommand(command) {
							status.ManagedHookValid = true
						}
					}
				}
			}
		}
	}
	ownerOnly, err := OwnerOnly(path)
	if err != nil {
		// OwnerOnly reports an explanatory error for an insecure Windows DACL. The
		// inspector represents that as an unhealthy permission result rather than
		// failing the otherwise read-only diagnostic.
		return status, nil
	}
	status.OwnerOnly = ownerOnly
	return status, nil
}

func isManagedEngramCloudHookCandidate(command string) bool {
	return (strings.HasPrefix(command, "timeout 5 engram sync --cloud ") && strings.HasSuffix(command, " || true")) ||
		(strings.HasPrefix(command, "click engram-cloud-import --project-b64 ") && strings.HasSuffix(command, " || true")) ||
		(strings.HasPrefix(command, "cmd.exe /d /s /c \"click engram-cloud-import --project-b64 ") && strings.HasSuffix(command, " & exit /b 0\""))
}

// IsManagedEngramCloudHookCommand reports whether a command matches click's canonical
// Engram Cloud managed hook signature, regardless of the project name it contains.
// Per DD-2, identity is by signature (shape), not exact string equality.
// It delegates to the unexported implementation deliberately: this predicate decides which
// SessionStart entries click owns, and two independent copies would drift the first time the
// canonical hook shape changes — production would enforce one rule while tests validated another.
func IsManagedEngramCloudHookCommand(cmd string) bool {
	return isManagedEngramCloudHookCommand(cmd)
}

// ConfigureEngramCloudSessionSync writes the Engram Cloud environment and SessionStart hook
// to Claude Code's settings.json. It performs one read/merge/write through the secured
// writeSettingsFile, preserving all foreign entries.
func ConfigureEngramCloudSessionSync(cfg Config, m *manifest.Manifest, mode CloudTokenPersistence, token string) error {
	return configureEngramCloudSessionSyncFunc(cfg, m, mode, token)
}

// configureEngramCloudSessionSyncImpl is the actual implementation of ConfigureEngramCloudSessionSync.
// This is called through the seam variable for testability.
func configureEngramCloudSessionSyncImpl(cfg Config, m *manifest.Manifest, mode CloudTokenPersistence, token string) error {
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

	// Resolve server and project, honoring env overrides
	server, project, _ := resolveEngramCloudConfig(cfg, m)

	// Fail loudly if server or project is empty - this is a bug, not a runtime condition
	if server == "" || project == "" {
		return fmt.Errorf("installer: cloud configuration incomplete: server=%q, project=%q", server, project)
	}

	// Ensure env block exists
	env, ok := settings["env"].(map[string]any)
	if !ok || env == nil {
		env = map[string]any{}
	}

	// Merge click-owned env keys (selective per-entry, not whole-key like PruneEmptyClickSettingsKeys)
	env["ENGRAM_CLOUD_AUTOSYNC"] = "1"
	env["ENGRAM_CLOUD_SERVER"] = server

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
	managedHookCmd, err := managedEngramCloudHookCommand(project)
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

	// Remove any existing managed hooks by signature, operating at nested command granularity
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

		// This is a managed hook entry - remove only click's command, preserve foreign commands
		hooksList, ok := entry["hooks"].([]any)
		if !ok {
			filteredSessionStart = append(filteredSessionStart, rawEntry)
			continue
		}

		filteredHooks := make([]any, 0, len(hooksList))
		for _, rawHook := range hooksList {
			hook, ok := rawHook.(map[string]any)
			if !ok {
				filteredHooks = append(filteredHooks, rawHook)
				continue
			}
			hookType, _ := hook["type"].(string)
			hookCmd, _ := hook["command"].(string)

			// Remove only click's managed hook by signature
			if hookType == "command" && isManagedEngramCloudHookCommand(hookCmd) {
				continue // Skip this hook (remove it)
			}
			filteredHooks = append(filteredHooks, rawHook)
		}

		// Only preserve the entry if it still has hooks (including foreign commands)
		if len(filteredHooks) > 0 {
			entry["hooks"] = filteredHooks
			filteredSessionStart = append(filteredSessionStart, entry)
		}
		// If entry becomes empty, we drop it entirely
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
	if err := RemoveEngramCloudImportOutcome(cfg); err != nil {
		return err
	}
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

	// Remove managed SessionStart hooks by signature, operating at nested command granularity
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
					// This is a managed hook entry - remove only click's command, preserve foreign commands
					entryHooks, _ := entry["hooks"].([]any)
					filteredHooks := make([]any, 0, len(entryHooks))
					for _, hookRaw := range entryHooks {
						hook, ok := hookRaw.(map[string]any)
						if !ok {
							filteredHooks = append(filteredHooks, hookRaw)
							continue
						}
						if hook["type"] == "command" {
							cmd, _ := hook["command"].(string)
							if isManagedEngramCloudHookCommand(cmd) {
								continue // Skip this hook (remove it)
							}
						}
						filteredHooks = append(filteredHooks, hookRaw)
					}

					if len(filteredHooks) > 0 {
						entry["hooks"] = filteredHooks
						filteredSessionStart = append(filteredSessionStart, entry)
					}
					// If entry becomes empty, we drop it entirely
					changed = true
				} else {
					// Foreign hook - preserve it
					filteredSessionStart = append(filteredSessionStart, rawEntry)
				}
			}

			if len(filteredSessionStart) != len(sessionStart) || changed {
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
