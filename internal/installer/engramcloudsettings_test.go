package installer

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/manifest"
)

// TestConfigureEngramCloudSessionSync_WritesFullEnvBlock is the RED test for task 4.1:
// with consent mode persist, the top-level `env` object gains `ENGRAM_CLOUD_AUTOSYNC: "1"`,
// the resolved `ENGRAM_CLOUD_SERVER`, and the supplied `ENGRAM_CLOUD_TOKEN`.
func TestConfigureEngramCloudSessionSync_WritesFullEnvBlock(t *testing.T) {
	t.Setenv("CLICK_CLAUDE_HOME", t.TempDir())

	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")

	// Seed an existing settings.json with some foreign entries
	if err := os.WriteFile(settingsPath, []byte(`{"foreign":"value"}`+"\n"), 0o600); err != nil {
		t.Fatalf("seed settings.json: %v", err)
	}

	cfg := Config{
		ClaudeHome: dir,
	}

	m := &manifest.Manifest{
		EngramCloud: manifest.EngramCloud{
			Server:  "https://engram.example.com",
			Project: "team-hive",
		},
	}

	token := "consented-token-value"
	mode := CloudTokenPersistencePersist

	if err := ConfigureEngramCloudSessionSync(cfg, m, mode, token); err != nil {
		t.Fatalf("ConfigureEngramCloudSessionSync() error = %v", err)
	}

	// Read back and verify the env block
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	env, ok := settings["env"].(map[string]any)
	if !ok {
		t.Fatalf("settings[\"env\"] not present or not a map, got = %T", settings["env"])
	}

	// Verify all three click-owned env keys are present with correct values
	if got, want := env["ENGRAM_CLOUD_AUTOSYNC"], "1"; got != want {
		t.Fatalf("env[\"ENGRAM_CLOUD_AUTOSYNC\"] = %v, want %q", got, want)
	}

	if got, want := env["ENGRAM_CLOUD_SERVER"], "https://engram.example.com"; got != want {
		t.Fatalf("env[\"ENGRAM_CLOUD_SERVER\"] = %v, want %q", got, want)
	}

	if got, want := env["ENGRAM_CLOUD_TOKEN"], token; got != want {
		t.Fatalf("env[\"ENGRAM_CLOUD_TOKEN\"] = %v, want %q", got, want)
	}

	// Verify foreign entry is preserved
	if got, want := settings["foreign"], "value"; got != want {
		t.Fatalf("settings[\"foreign\"] = %v, want %q", got, want)
	}
}

// TestConfigureEngramCloudSessionSync_PreservesForeignEnvEntries is the RED test for task 4.3:
// developer-owned env entries keep their original values after the merge.
func TestConfigureEngramCloudSessionSync_PreservesForeignEnvEntries(t *testing.T) {
	t.Setenv("CLICK_CLAUDE_HOME", t.TempDir())

	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")

	// Seed an existing settings.json with foreign env entries
	seedSettings := `{
  "env": {
    "MY_DEV_KEY": "my-value",
    "ANOTHER_KEY": "another-value"
  },
  "foreign": "value"
}` + "\n"
	if err := os.WriteFile(settingsPath, []byte(seedSettings), 0o600); err != nil {
		t.Fatalf("seed settings.json: %v", err)
	}

	cfg := Config{
		ClaudeHome: dir,
	}

	m := &manifest.Manifest{
		EngramCloud: manifest.EngramCloud{
			Server:  "https://engram.example.com",
			Project: "team-hive",
		},
	}

	token := "consented-token-value"
	mode := CloudTokenPersistencePersist

	if err := ConfigureEngramCloudSessionSync(cfg, m, mode, token); err != nil {
		t.Fatalf("ConfigureEngramCloudSessionSync() error = %v", err)
	}

	// Read back and verify foreign env entries are preserved
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	env, ok := settings["env"].(map[string]any)
	if !ok {
		t.Fatalf("settings[\"env\"] not present or not a map, got = %T", settings["env"])
	}

	// Verify foreign env entries are preserved with original values
	if got, want := env["MY_DEV_KEY"], "my-value"; got != want {
		t.Fatalf("env[\"MY_DEV_KEY\"] = %v, want %q", got, want)
	}

	if got, want := env["ANOTHER_KEY"], "another-value"; got != want {
		t.Fatalf("env[\"ANOTHER_KEY\"] = %v, want %q", got, want)
	}

	// Verify click-owned env keys are present
	if got, want := env["ENGRAM_CLOUD_AUTOSYNC"], "1"; got != want {
		t.Fatalf("env[\"ENGRAM_CLOUD_AUTOSYNC\"] = %v, want %q", got, want)
	}

	if got, want := env["ENGRAM_CLOUD_SERVER"], "https://engram.example.com"; got != want {
		t.Fatalf("env[\"ENGRAM_CLOUD_SERVER\"] = %v, want %q", got, want)
	}

	if got, want := env["ENGRAM_CLOUD_TOKEN"], token; got != want {
		t.Fatalf("env[\"ENGRAM_CLOUD_TOKEN\"] = %v, want %q", got, want)
	}
}

// TestConfigureEngramCloudSessionSync_IdempotentSecondRun is the RED test for task 4.5:
// a second identical run leaves the serialized settings byte-identical and a recording writer
// observes no second write.
func TestConfigureEngramCloudSessionSync_IdempotentSecondRun(t *testing.T) {
	t.Setenv("CLICK_CLAUDE_HOME", t.TempDir())

	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")

	cfg := Config{
		ClaudeHome: dir,
	}

	m := &manifest.Manifest{
		EngramCloud: manifest.EngramCloud{
			Server:  "https://engram.example.com",
			Project: "team-hive",
		},
	}

	token := "consented-token-value"
	mode := CloudTokenPersistencePersist

	// First run
	if err := ConfigureEngramCloudSessionSync(cfg, m, mode, token); err != nil {
		t.Fatalf("first ConfigureEngramCloudSessionSync() error = %v", err)
	}

	// Read the settings after first run
	firstBytes, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("ReadFile() after first run error = %v", err)
	}

	// Capture file modification time before second run
	infoBefore, err := os.Stat(settingsPath)
	if err != nil {
		t.Fatalf("Stat() before second run error = %v", err)
	}

	// Second run with identical parameters
	if err := ConfigureEngramCloudSessionSync(cfg, m, mode, token); err != nil {
		t.Fatalf("second ConfigureEngramCloudSessionSync() error = %v", err)
	}

	// Read the settings after second run
	secondBytes, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("ReadFile() after second run error = %v", err)
	}

	// The serialized bytes must be identical
	if !bytes.Equal(firstBytes, secondBytes) {
		t.Fatalf("second run changed settings: first=%q, second=%q", firstBytes, secondBytes)
	}

	// The file should not have been rewritten (no change in modification time)
	infoAfter, err := os.Stat(settingsPath)
	if err != nil {
		t.Fatalf("Stat() after second run error = %v", err)
	}

	if !infoBefore.ModTime().Equal(infoAfter.ModTime()) {
		t.Fatalf("second run rewrote the file (modification time changed): before=%v, after=%v",
			infoBefore.ModTime(), infoAfter.ModTime())
	}
}

// TestConfigureEngramCloudSessionSync_RegistersManagedHook is the RED test for task 4.7:
// registration adds one `SessionStart` entry with matcher `""` whose command is the platform
// builder's output; on POSIX the full string equals the ECS-4.1 canonical command.
func TestConfigureEngramCloudSessionSync_RegistersManagedHook(t *testing.T) {
	t.Setenv("CLICK_CLAUDE_HOME", t.TempDir())

	dir := t.TempDir()

	cfg := Config{
		ClaudeHome: dir,
	}

	m := &manifest.Manifest{
		EngramCloud: manifest.EngramCloud{
			Server:  "https://engram.example.com",
			Project: "team-hive",
		},
	}

	token := "consented-token-value"
	mode := CloudTokenPersistencePersist

	if err := ConfigureEngramCloudSessionSync(cfg, m, mode, token); err != nil {
		t.Fatalf("ConfigureEngramCloudSessionSync() error = %v", err)
	}

	// Read back and verify the SessionStart hook
	settingsPath := cfg.SettingsPath()
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	hooks, ok := settings["hooks"].(map[string]any)
	if !ok {
		t.Fatalf("settings[\"hooks\"] not present or not a map, got = %T", settings["hooks"])
	}

	sessionStart, ok := hooks["SessionStart"].([]any)
	if !ok {
		t.Fatalf("hooks[\"SessionStart\"] not present or not a slice, got = %T", hooks["SessionStart"])
	}

	// Should have exactly one SessionStart entry
	if len(sessionStart) != 1 {
		t.Fatalf("hooks[\"SessionStart\"] has %d entries, want 1", len(sessionStart))
	}

	entry, ok := sessionStart[0].(map[string]any)
	if !ok {
		t.Fatalf("SessionStart entry is not a map, got = %T", sessionStart[0])
	}

	// Verify matcher is ""
	if got, want := entry["matcher"], ""; got != want {
		t.Fatalf("entry[\"matcher\"] = %v, want %q", got, want)
	}

	// Verify the command matches the platform builder's output
	expectedCmd, err := managedEngramCloudHookCommand("team-hive")
	if err != nil {
		t.Fatalf("managedEngramCloudHookCommand() error = %v", err)
	}

	entryHooks, ok := entry["hooks"].([]any)
	if !ok {
		t.Fatalf("entry[\"hooks\"] not present or not a slice, got = %T", entry["hooks"])
	}

	if len(entryHooks) != 1 {
		t.Fatalf("entry[\"hooks\"] has %d entries, want 1", len(entryHooks))
	}

	hook, ok := entryHooks[0].(map[string]any)
	if !ok {
		t.Fatalf("hooks[0] is not a map, got = %T", entryHooks[0])
	}

	if got, want := hook["command"], expectedCmd; got != want {
		t.Fatalf("hook[\"command\"] = %v, want %q", got, want)
	}
}

// TestConfigureEngramCloudSessionSync_HookIdempotentAndForeignHooksPreserved is the RED test for task 4.8:
// re-running leaves exactly one managed entry, and foreign `SessionStart` entries plus other
// hook events remain structurally unchanged.
func TestConfigureEngramCloudSessionSync_HookIdempotentAndForeignHooksPreserved(t *testing.T) {
	t.Setenv("CLICK_CLAUDE_HOME", t.TempDir())

	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")

	// Seed with foreign hooks
	seedSettings := `{
  "hooks": {
    "SessionStart": [
      {
        "matcher": "dev-tool",
        "hooks": [{"type": "command", "command": "dev-tool start"}]
      }
    ],
    "PreToolUse": [
      {
        "matcher": "other-matcher",
        "hooks": [{"type": "command", "command": "other-command"}]
      }
    ]
  },
  "foreign": "value"
}` + "\n"
	if err := os.WriteFile(settingsPath, []byte(seedSettings), 0o600); err != nil {
		t.Fatalf("seed settings.json: %v", err)
	}

	cfg := Config{
		ClaudeHome: dir,
	}

	m := &manifest.Manifest{
		EngramCloud: manifest.EngramCloud{
			Server:  "https://engram.example.com",
			Project: "team-hive",
		},
	}

	token := "consented-token-value"
	mode := CloudTokenPersistencePersist

	// First run
	if err := ConfigureEngramCloudSessionSync(cfg, m, mode, token); err != nil {
		t.Fatalf("first ConfigureEngramCloudSessionSync() error = %v", err)
	}

	// Verify foreign SessionStart entry is preserved
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("ReadFile() after first run error = %v", err)
	}

	var settingsAfterFirst map[string]any
	if err := json.Unmarshal(data, &settingsAfterFirst); err != nil {
		t.Fatalf("json.Unmarshal() after first run error = %v", err)
	}

	hooks, ok := settingsAfterFirst["hooks"].(map[string]any)
	if !ok {
		t.Fatalf("settings[\"hooks\"] not present or not a map after first run")
	}

	sessionStart, ok := hooks["SessionStart"].([]any)
	if !ok {
		t.Fatalf("hooks[\"SessionStart\"] not a slice after first run")
	}

	// Should have exactly 2 entries: foreign + managed
	if len(sessionStart) != 2 {
		t.Fatalf("hooks[\"SessionStart\"] has %d entries after first run, want 2", len(sessionStart))
	}

	// Verify PreToolUse foreign hook is still present
	preToolUse, ok := hooks["PreToolUse"].([]any)
	if !ok {
		t.Fatalf("hooks[\"PreToolUse\"] not a slice after first run")
	}
	if len(preToolUse) != 1 {
		t.Fatalf("hooks[\"PreToolUse\"] has %d entries after first run, want 1", len(preToolUse))
	}

	// Second run
	if err := ConfigureEngramCloudSessionSync(cfg, m, mode, token); err != nil {
		t.Fatalf("second ConfigureEngramCloudSessionSync() error = %v", err)
	}

	// Verify still exactly one managed entry (idempotent)
	data, err = os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("ReadFile() after second run error = %v", err)
	}

	var settingsAfterSecond map[string]any
	if err := json.Unmarshal(data, &settingsAfterSecond); err != nil {
		t.Fatalf("json.Unmarshal() after second run error = %v", err)
	}

	hooks, ok = settingsAfterSecond["hooks"].(map[string]any)
	if !ok {
		t.Fatalf("settings[\"hooks\"] not present or not a map after second run")
	}

	sessionStart, ok = hooks["SessionStart"].([]any)
	if !ok {
		t.Fatalf("hooks[\"SessionStart\"] not a slice after second run")
	}

	// Should still have exactly 2 entries: foreign + managed (no duplicates)
	if len(sessionStart) != 2 {
		t.Fatalf("hooks[\"SessionStart\"] has %d entries after second run, want 2 (idempotent)", len(sessionStart))
	}
}

// TestConfigureEngramCloudSessionSync_DeclineWritesOnlyNonSecretKeys is the RED test for task 4.10:
// decline mode writes ENGRAM_CLOUD_AUTOSYNC and ENGRAM_CLOUD_SERVER only; the token is absent.
func TestConfigureEngramCloudSessionSync_DeclineWritesOnlyNonSecretKeys(t *testing.T) {
	t.Setenv("CLICK_CLAUDE_HOME", t.TempDir())

	dir := t.TempDir()

	cfg := Config{
		ClaudeHome: dir,
	}

	m := &manifest.Manifest{
		EngramCloud: manifest.EngramCloud{
			Server:  "https://engram.example.com",
			Project: "team-hive",
		},
	}

	token := "consented-token-value"
	mode := CloudTokenPersistenceDecline

	if err := ConfigureEngramCloudSessionSync(cfg, m, mode, token); err != nil {
		t.Fatalf("ConfigureEngramCloudSessionSync() error = %v", err)
	}

	// Read back and verify the env block
	settingsPath := cfg.SettingsPath()
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	env, ok := settings["env"].(map[string]any)
	if !ok {
		t.Fatalf("settings[\"env\"] not present or not a map, got = %T", settings["env"])
	}

	// Verify AUTOSYNC and SERVER are present
	if got, want := env["ENGRAM_CLOUD_AUTOSYNC"], "1"; got != want {
		t.Fatalf("env[\"ENGRAM_CLOUD_AUTOSYNC\"] = %v, want %q", got, want)
	}

	if got, want := env["ENGRAM_CLOUD_SERVER"], "https://engram.example.com"; got != want {
		t.Fatalf("env[\"ENGRAM_CLOUD_SERVER\"] = %v, want %q", got, want)
	}

	// Verify TOKEN is absent
	if tokenVal, present := env["ENGRAM_CLOUD_TOKEN"]; present {
		t.Fatalf("env[\"ENGRAM_CLOUD_TOKEN\"] should be absent in decline mode, got = %v", tokenVal)
	}
}

// TestConfigureEngramCloudSessionSync_DeclineRemovesPersistedToken is the RED test for task 4.11:
// a previously click-owned ENGRAM_CLOUD_TOKEN is removed when persistence is declined.
func TestConfigureEngramCloudSessionSync_DeclineRemovesPersistedToken(t *testing.T) {
	t.Setenv("CLICK_CLAUDE_HOME", t.TempDir())

	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")

	// Seed with a previously persisted token
	seedSettings := `{
  "env": {
    "ENGRAM_CLOUD_AUTOSYNC": "1",
    "ENGRAM_CLOUD_SERVER": "https://old-server.example.com",
    "ENGRAM_CLOUD_TOKEN": "old-persisted-token",
    "FOREIGN_KEY": "foreign-value"
  }
}` + "\n"
	if err := os.WriteFile(settingsPath, []byte(seedSettings), 0o600); err != nil {
		t.Fatalf("seed settings.json: %v", err)
	}

	cfg := Config{
		ClaudeHome: dir,
	}

	m := &manifest.Manifest{
		EngramCloud: manifest.EngramCloud{
			Server:  "https://engram.example.com",
			Project: "team-hive",
		},
	}

	newToken := "new-consented-token"
	mode := CloudTokenPersistenceDecline

	if err := ConfigureEngramCloudSessionSync(cfg, m, mode, newToken); err != nil {
		t.Fatalf("ConfigureEngramCloudSessionSync() error = %v", err)
	}

	// Read back and verify token is removed
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	env, ok := settings["env"].(map[string]any)
	if !ok {
		t.Fatalf("settings[\"env\"] not present or not a map, got = %T", settings["env"])
	}

	// Verify AUTOSYNC is present
	if got, want := env["ENGRAM_CLOUD_AUTOSYNC"], "1"; got != want {
		t.Fatalf("env[\"ENGRAM_CLOUD_AUTOSYNC\"] = %v, want %q", got, want)
	}

	// Verify SERVER is updated
	if got, want := env["ENGRAM_CLOUD_SERVER"], "https://engram.example.com"; got != want {
		t.Fatalf("env[\"ENGRAM_CLOUD_SERVER\"] = %v, want %q", got, want)
	}

	// Verify TOKEN is absent
	if tokenVal, present := env["ENGRAM_CLOUD_TOKEN"]; present {
		t.Fatalf("env[\"ENGRAM_CLOUD_TOKEN\"] should be absent in decline mode, got = %v", tokenVal)
	}

	// Verify foreign env entry is preserved
	if got, want := env["FOREIGN_KEY"], "foreign-value"; got != want {
		t.Fatalf("env[\"FOREIGN_KEY\"] = %v, want %q", got, want)
	}
}

// TestConfigureEngramCloudSessionSync_MalformedSettingsPreservesBytes is the RED test for task 4.13:
// malformed pre-existing settings.json yields an error and the original file bytes remain unchanged.
func TestConfigureEngramCloudSessionSync_MalformedSettingsPreservesBytes(t *testing.T) {
	t.Setenv("CLICK_CLAUDE_HOME", t.TempDir())

	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")

	// Seed with malformed JSON
	malformedBytes := []byte(`{"env": {"FOO": "bar"} invalid json}`)
	if err := os.WriteFile(settingsPath, malformedBytes, 0o600); err != nil {
		t.Fatalf("seed settings.json: %v", err)
	}

	cfg := Config{
		ClaudeHome: dir,
	}

	m := &manifest.Manifest{
		EngramCloud: manifest.EngramCloud{
			Server:  "https://engram.example.com",
			Project: "team-hive",
		},
	}

	token := "consented-token-value"
	mode := CloudTokenPersistencePersist

	// Should return an error
	if err := ConfigureEngramCloudSessionSync(cfg, m, mode, token); err == nil {
		t.Fatal("ConfigureEngramCloudSessionSync() on malformed JSON should return error, got nil")
	}

	// Verify the original bytes remain unchanged
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	if !bytes.Equal(data, malformedBytes) {
		t.Fatalf("malformed settings were modified: original=%q, got=%q", malformedBytes, data)
	}
}

// TestRemoveEngramCloudSessionSync_RemovesOnlyClickOwnedEntries is the RED test for task 4.15:
// all three click-owned env keys and only the managed hook entry are removed;
// foreign env keys and foreign hook entries retain their values;
// containers with foreign entries remain present.
func TestRemoveEngramCloudSessionSync_RemovesOnlyClickOwnedEntries(t *testing.T) {
	t.Setenv("CLICK_CLAUDE_HOME", t.TempDir())

	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")

	// Seed with click-owned and foreign entries
	seedSettings := `{
  "env": {
    "ENGRAM_CLOUD_AUTOSYNC": "1",
    "ENGRAM_CLOUD_SERVER": "https://engram.example.com",
    "ENGRAM_CLOUD_TOKEN": "consented-token-value",
    "FOREIGN_ENV_KEY": "foreign-value"
  },
  "hooks": {
    "SessionStart": [
      {
        "matcher": "",
        "hooks": [{
          "type": "command",
          "command": "cmd.exe /d /s /c \"click engram-cloud-import --project-b64 dGVhbS1oaXZl & exit /b 0\""
        }]
      },
      {
        "matcher": "foreign-hook",
        "hooks": [{"type": "command", "command": "foreign-command"}]
      }
    ],
    "PreToolUse": [
      {
        "matcher": "other-matcher",
        "hooks": [{"type": "command", "command": "other-command"}]
      }
    ]
  },
  "foreignTopLevel": "value"
}` + "\n"
	if err := os.WriteFile(settingsPath, []byte(seedSettings), 0o600); err != nil {
		t.Fatalf("seed settings.json: %v", err)
	}

	cfg := Config{
		ClaudeHome: dir,
	}

	if err := RemoveEngramCloudSessionSync(cfg); err != nil {
		t.Fatalf("RemoveEngramCloudSessionSync() error = %v", err)
	}

	// Read back and verify removal
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	// Verify foreign top-level entry is preserved
	if got, want := settings["foreignTopLevel"], "value"; got != want {
		t.Fatalf("settings[\"foreignTopLevel\"] = %v, want %q", got, want)
	}

	// Verify env block still exists with foreign entries
	env, ok := settings["env"].(map[string]any)
	if !ok {
		t.Fatalf("settings[\"env\"] not present or not a map, got = %T", settings["env"])
	}

	// Verify all three click-owned env keys are removed
	if _, present := env["ENGRAM_CLOUD_AUTOSYNC"]; present {
		t.Fatalf("env[\"ENGRAM_CLOUD_AUTOSYNC\"] should be removed, got = %v", env["ENGRAM_CLOUD_AUTOSYNC"])
	}
	if _, present := env["ENGRAM_CLOUD_SERVER"]; present {
		t.Fatalf("env[\"ENGRAM_CLOUD_SERVER\"] should be removed, got = %v", env["ENGRAM_CLOUD_SERVER"])
	}
	if _, present := env["ENGRAM_CLOUD_TOKEN"]; present {
		t.Fatalf("env[\"ENGRAM_CLOUD_TOKEN\"] should be removed, got = %v", env["ENGRAM_CLOUD_TOKEN"])
	}

	// Verify foreign env entry is preserved
	if got, want := env["FOREIGN_ENV_KEY"], "foreign-value"; got != want {
		t.Fatalf("env[\"FOREIGN_ENV_KEY\"] = %v, want %q", got, want)
	}

	// Verify hooks block still exists with foreign entries
	hooks, ok := settings["hooks"].(map[string]any)
	if !ok {
		t.Fatalf("settings[\"hooks\"] not present or not a map, got = %T", settings["hooks"])
	}

	// Verify PreToolUse is still present
	preToolUse, ok := hooks["PreToolUse"].([]any)
	if !ok {
		t.Fatalf("hooks[\"PreToolUse\"] not a slice, got = %T", hooks["PreToolUse"])
	}
	if len(preToolUse) != 1 {
		t.Fatalf("hooks[\"PreToolUse\"] should have 1 entry, got %d", len(preToolUse))
	}

	// Verify SessionStart has only foreign entry
	sessionStart, ok := hooks["SessionStart"].([]any)
	if !ok {
		t.Fatalf("hooks[\"SessionStart\"] not a slice, got = %T", hooks["SessionStart"])
	}
	if len(sessionStart) != 1 {
		t.Fatalf("hooks[\"SessionStart\"] should have 1 entry (foreign only), got %d", len(sessionStart))
	}

	entry, ok := sessionStart[0].(map[string]any)
	if !ok {
		t.Fatalf("SessionStart entry is not a map, got = %T", sessionStart[0])
	}

	if got, want := entry["matcher"], "foreign-hook"; got != want {
		t.Fatalf("entry[\"matcher\"] = %v, want %q", got, want)
	}
}

// TestRemoveEngramCloudSessionSync_PrunesClickOnlyEmptyContainersAndIsIdempotent is the RED test for task 4.16:
// a hook container left empty by removal is pruned, and a second removal run changes nothing.
func TestRemoveEngramCloudSessionSync_PrunesClickOnlyEmptyContainersAndIsIdempotent(t *testing.T) {
	t.Setenv("CLICK_CLAUDE_HOME", t.TempDir())

	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")

	// Seed with only click-owned entries (env and hook)
	seedSettings := `{
  "env": {
    "ENGRAM_CLOUD_AUTOSYNC": "1",
    "ENGRAM_CLOUD_SERVER": "https://engram.example.com",
    "ENGRAM_CLOUD_TOKEN": "consented-token-value"
  },
  "hooks": {
    "SessionStart": [
      {
        "matcher": "",
        "hooks": [{
          "type": "command",
          "command": "cmd.exe /d /s /c \"click engram-cloud-import --project-b64 dGVhbS1oaXZl & exit /b 0\""
        }]
      }
    ]
  }
}` + "\n"
	if err := os.WriteFile(settingsPath, []byte(seedSettings), 0o600); err != nil {
		t.Fatalf("seed settings.json: %v", err)
	}

	cfg := Config{
		ClaudeHome: dir,
	}

	// First removal
	if err := RemoveEngramCloudSessionSync(cfg); err != nil {
		t.Fatalf("first RemoveEngramCloudSessionSync() error = %v", err)
	}

	// Read after first removal
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("ReadFile() after first removal error = %v", err)
	}

	var settingsAfterFirst map[string]any
	if err := json.Unmarshal(data, &settingsAfterFirst); err != nil {
		t.Fatalf("json.Unmarshal() after first removal error = %v", err)
	}

	// Verify env block is pruned when empty
	if _, present := settingsAfterFirst["env"]; present {
		t.Fatalf("settings[\"env\"] should be pruned when empty, got = %v", settingsAfterFirst["env"])
	}

	// Verify hooks block is pruned when empty
	if _, present := settingsAfterFirst["hooks"]; present {
		t.Fatalf("settings[\"hooks\"] should be pruned when empty, got = %v", settingsAfterFirst["hooks"])
	}

	// Capture modification time before second removal
	infoBefore, err := os.Stat(settingsPath)
	if err != nil {
		t.Fatalf("Stat() before second removal error = %v", err)
	}

	// Second removal should be idempotent
	if err := RemoveEngramCloudSessionSync(cfg); err != nil {
		t.Fatalf("second RemoveEngramCloudSessionSync() error = %v", err)
	}

	// Verify no change on second run
	dataAfterSecond, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("ReadFile() after second removal error = %v", err)
	}

	if !bytes.Equal(data, dataAfterSecond) {
		t.Fatalf("second removal changed settings: first=%q, second=%q", data, dataAfterSecond)
	}

	infoAfter, err := os.Stat(settingsPath)
	if err != nil {
		t.Fatalf("Stat() after second removal error = %v", err)
	}

	if !infoBefore.ModTime().Equal(infoAfter.ModTime()) {
		t.Fatalf("second removal rewrote the file (modification time changed)")
	}
}