package installer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestRepairRedactedEngramCloudTokenAfterRestore_RemovesPlaceholder guards a rollback hazard a
// fresh review found in this branch: snapshot.go's redactEngramCloudToken permanently replaces
// env.ENGRAM_CLOUD_TOKEN's value with the literal "[REDACTED]" in settings.json backups.
// ApplyPreparedRestore writes that backup back byte-for-byte, so without this repair step, a
// rollback would leave the literal placeholder sitting in the live settings.json as if it were a
// real, usable token — silently breaking every subsequent Engram Cloud sync.
func TestRepairRedactedEngramCloudTokenAfterRestore_RemovesPlaceholder(t *testing.T) {
	t.Setenv("CLICK_CLAUDE_HOME", t.TempDir())
	dir := t.TempDir()
	cfg := Config{ClaudeHome: dir}
	settingsPath := filepath.Join(dir, "settings.json")

	original := `{"env":{"FOREIGN_KEY":"keep-me","ENGRAM_CLOUD_AUTOSYNC":"1","ENGRAM_CLOUD_SERVER":"https://engram.example.com","ENGRAM_CLOUD_TOKEN":"[REDACTED]"}}`
	if err := os.WriteFile(settingsPath, []byte(original), 0o600); err != nil {
		t.Fatalf("seed settings.json: %v", err)
	}

	repaired, err := RepairRedactedEngramCloudTokenAfterRestore(cfg)
	if err != nil {
		t.Fatalf("RepairRedactedEngramCloudTokenAfterRestore() error = %v", err)
	}
	if !repaired {
		t.Fatal("RepairRedactedEngramCloudTokenAfterRestore() repaired = false, want true (placeholder was present)")
	}

	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings.json after repair: %v", err)
	}
	settings := map[string]any{}
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("parse repaired settings.json: %v", err)
	}
	env, ok := settings["env"].(map[string]any)
	if !ok {
		t.Fatal("env block missing after repair")
	}
	if _, present := env["ENGRAM_CLOUD_TOKEN"]; present {
		t.Fatal("ENGRAM_CLOUD_TOKEN still present after repair; want it removed")
	}
	if got := env["FOREIGN_KEY"]; got != "keep-me" {
		t.Fatalf("FOREIGN_KEY = %v, want unchanged after repair", got)
	}
	if got := env["ENGRAM_CLOUD_AUTOSYNC"]; got != "1" {
		t.Fatalf("ENGRAM_CLOUD_AUTOSYNC = %v, want unchanged after repair (only the token is repaired)", got)
	}
	if got := env["ENGRAM_CLOUD_SERVER"]; got != "https://engram.example.com" {
		t.Fatalf("ENGRAM_CLOUD_SERVER = %v, want unchanged after repair", got)
	}
}

// TestRepairRedactedEngramCloudTokenAfterRestore_NoOpOnRealToken proves the repair never touches a
// genuine token: it must only ever match the exact literal placeholder string, never a real
// server-issued token value (even one that happens to look unusual).
func TestRepairRedactedEngramCloudTokenAfterRestore_NoOpOnRealToken(t *testing.T) {
	t.Setenv("CLICK_CLAUDE_HOME", t.TempDir())
	dir := t.TempDir()
	cfg := Config{ClaudeHome: dir}
	settingsPath := filepath.Join(dir, "settings.json")

	original := `{"env":{"ENGRAM_CLOUD_TOKEN":"a-real-token-value"}}`
	if err := os.WriteFile(settingsPath, []byte(original), 0o600); err != nil {
		t.Fatalf("seed settings.json: %v", err)
	}
	before, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings.json before repair: %v", err)
	}

	repaired, err := RepairRedactedEngramCloudTokenAfterRestore(cfg)
	if err != nil {
		t.Fatalf("RepairRedactedEngramCloudTokenAfterRestore() error = %v", err)
	}
	if repaired {
		t.Fatal("RepairRedactedEngramCloudTokenAfterRestore() repaired = true, want false (token was real, not the placeholder)")
	}

	after, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings.json after repair: %v", err)
	}
	if string(before) != string(after) {
		t.Fatal("settings.json was modified even though no repair was needed")
	}
}

// TestRepairRedactedEngramCloudTokenAfterRestore_MissingSettingsFileIsNoOp proves a rollback of a
// snapshot that never had a settings.json (e.g., Engram Cloud never configured) does not error.
func TestRepairRedactedEngramCloudTokenAfterRestore_MissingSettingsFileIsNoOp(t *testing.T) {
	t.Setenv("CLICK_CLAUDE_HOME", t.TempDir())
	cfg := Config{ClaudeHome: t.TempDir()}

	repaired, err := RepairRedactedEngramCloudTokenAfterRestore(cfg)
	if err != nil {
		t.Fatalf("RepairRedactedEngramCloudTokenAfterRestore() error = %v, want nil for a missing settings.json", err)
	}
	if repaired {
		t.Fatal("RepairRedactedEngramCloudTokenAfterRestore() repaired = true, want false for a missing settings.json")
	}
}
