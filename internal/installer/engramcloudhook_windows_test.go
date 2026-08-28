//go:build windows

package installer

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/manifest"
)

func TestManagedEngramCloudHookCommand_WindowsWrapper(t *testing.T) {
	project := "team-hive"
	result, err := managedEngramCloudHookCommand(project)
	if err != nil {
		t.Fatalf("managedEngramCloudHookCommand(%q) returned error: %v", project, err)
	}

	expected := `cmd.exe /d /s /c "click engram-cloud-import --project-b64 dGVhbS1oaXZl & exit /b 0"`
	if result != expected {
		t.Errorf("managedEngramCloudHookCommand(%q) = %q, want %q", project, result, expected)
	}

	// Guard against a real typo: ensure subcommand spelling is "engram-cloud-import", not "enram-cloud-import"
	if !strings.Contains(result, "engram-cloud-import") {
		t.Errorf("managedEngramCloudHookCommand(%q) result %q does not contain correct subcommand 'engram-cloud-import'", project, result)
	}

	// Verify --project-b64 payload base64url-decodes back to the original project string
	if !strings.Contains(result, "--project-b64") {
		t.Errorf("managedEngramCloudHookCommand(%q) result %q does not contain --project-b64 flag", project, result)
	}

	// Extract and verify the base64 payload
	expectedB64 := base64.RawURLEncoding.EncodeToString([]byte(project))
	if !strings.Contains(result, expectedB64) {
		t.Errorf("managedEngramCloudHookCommand(%q) result %q does not contain expected base64 payload %q", project, result, expectedB64)
	}
}

func TestManagedEngramCloudHookCommand_UnifiedImporterOnAllPlatforms(t *testing.T) {
	project := "team-hive"
	result, err := managedEngramCloudHookCommand(project)
	if err != nil {
		t.Fatalf("managedEngramCloudHookCommand(%q) returned error: %v", project, err)
	}

	// The command should invoke click engram-cloud-import with base64url-encoded project
	if !strings.Contains(result, "click engram-cloud-import") {
		t.Fatalf("managedEngramCloudHookCommand(%q) result %q does not contain click engram-cloud-import", project, result)
	}

	if !strings.Contains(result, "--project-b64") {
		t.Fatalf("managedEngramCloudHookCommand(%q) result %q does not contain --project-b64 flag", project, result)
	}

	payload := strings.Fields(strings.SplitN(result, "--project-b64 ", 2)[1])[0]
	decoded, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		t.Fatalf("managedEngramCloudHookCommand(%q) payload %q does not base64url-decode: %v", project, payload, err)
	}
	if string(decoded) != project {
		t.Fatalf("managedEngramCloudHookCommand(%q) payload decodes to %q", project, decoded)
	}

	// On Windows, the exit suppression is "exit /b 0" at the end
	if !strings.Contains(result, "exit /b 0") {
		t.Fatalf("managedEngramCloudHookCommand(%q) result %q does not suppress non-zero exit (must contain exit /b 0)", project, result)
	}
}

func TestConfigureEngramCloudSessionSync_ReplacesLegacyTimeoutHook(t *testing.T) {
	t.Setenv("CLICK_CLAUDE_HOME", t.TempDir())

	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")

	// Seed settings.json with the OLD POSIX timeout hook
	seedSettings := `{
  "env": {
    "ENGRAM_CLOUD_AUTOSYNC": "1",
    "ENGRAM_CLOUD_SERVER": "https://engram.example.com",
    "ENGRAM_CLOUD_TOKEN": "test-token"
  },
  "hooks": {
    "SessionStart": [
      {
        "matcher": "",
        "hooks": [{
          "type": "command",
          "command": "timeout 5 engram sync --cloud --import --project team-hive || true"
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

	m := &manifest.Manifest{
		EngramCloud: manifest.EngramCloud{
			Server:  "https://engram.example.com",
			Project: "team-hive",
		},
	}

	token := "test-token"
	mode := CloudTokenPersistencePersist

	// Configure should replace the legacy hook with the new unified form
	if err := ConfigureEngramCloudSessionSync(cfg, m, mode, token); err != nil {
		t.Fatalf("ConfigureEngramCloudSessionSync() error = %v", err)
	}

	// Read back and verify only the new unified hook exists
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	// Verify the old timeout hook is NOT present
	if strings.Contains(string(data), "timeout 5 engram sync --cloud --import --project") {
		t.Fatal("old timeout hook should be replaced")
	}

	// Verify the new unified hook IS present
	if !strings.Contains(string(data), "click engram-cloud-import") {
		t.Fatal("new unified hook should be present")
	}

	// Verify exactly ONE managed hook entry
	hooks, ok := settings["hooks"].(map[string]any)
	if !ok {
		t.Fatalf("Expected hooks to be present")
	}

	sessionStart, ok := hooks["SessionStart"].([]any)
	if !ok {
		t.Fatalf("Expected SessionStart to be present")
	}

	managedCount := 0
	for _, rawEntry := range sessionStart {
		entry, ok := rawEntry.(map[string]any)
		if !ok {
			continue
		}
		matcher, _ := entry["matcher"].(string)
		if matcher == "" {
			managedCount++
			entryHooks, _ := entry["hooks"].([]any)
			for _, hookRaw := range entryHooks {
				hook, ok := hookRaw.(map[string]any)
				if !ok {
					continue
				}
				if hook["type"] == "command" {
					cmd, _ := hook["command"].(string)
					// Verify it's the new unified form, not the old timeout form
					if strings.Contains(cmd, "timeout 5 engram sync") {
						t.Fatalf("Found old timeout hook instead of new unified hook: %s", cmd)
					}
					if !strings.Contains(cmd, "click engram-cloud-import") {
						t.Fatalf("Hook is not the new unified form: %s", cmd)
					}
				}
			}
		}
	}

	if managedCount != 1 {
		t.Fatalf("Expected exactly 1 managed hook entry, got %d", managedCount)
	}
}
