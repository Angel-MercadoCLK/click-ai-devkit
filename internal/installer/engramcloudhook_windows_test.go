//go:build windows

package installer

import (
	"encoding/base64"
	"strings"
	"testing"
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
