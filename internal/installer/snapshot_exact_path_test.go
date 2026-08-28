package installer

import (
	"testing"
)

// TestRedactEngramCloudToken_ExactTopLevelPathOnly verifies that redaction
// targets ONLY the exact top-level "env.ENGRAM_CLOUD_TOKEN" path and does NOT
// over-redact nested occurrences in foreign objects. This fixes the regression
// where `{"env":{"FOREIGN":{"ENGRAM_CLOUD_TOKEN":"must-stay-unchanged"}}}`
// was incorrectly redacted even though the true top-level path was absent.
func TestRedactEngramCloudToken_ExactTopLevelPathOnly(t *testing.T) {
	// Case 1: Nested token under foreign object should NOT be redacted
	// because the true top-level "env.ENGRAM_CLOUD_TOKEN" path is absent
	nestedInput := `{"env":{"FOREIGN":{"ENGRAM_CLOUD_TOKEN":"must-stay-unchanged"}}}`
	backup, err := redactEngramCloudToken([]byte(nestedInput))
	if err != nil {
		t.Fatalf("redactEngramCloudToken() error = %v", err)
	}
	if string(backup) != nestedInput {
		t.Fatalf("nested token was incorrectly redacted; got %q, want %q", string(backup), nestedInput)
	}

	// Case 2: Top-level token SHOULD still be redacted correctly
	topLevelInput := `{"env":{"ENGRAM_CLOUD_TOKEN":"real-secret"}}`
	expected := `{"env":{"ENGRAM_CLOUD_TOKEN":"[REDACTED]"}}`
	backup, err = redactEngramCloudToken([]byte(topLevelInput))
	if err != nil {
		t.Fatalf("redactEngramCloudToken() error = %v", err)
	}
	if string(backup) != expected {
		t.Fatalf("top-level token not redacted; got %q, want %q", string(backup), expected)
	}

	// Case 3: plugin.env exclusion should still work (existing test case)
	pluginInput := `{"plugin":{"env":{"ENGRAM_CLOUD_TOKEN":"plugin-token"}}}`
	backup, err = redactEngramCloudToken([]byte(pluginInput))
	if err != nil {
		t.Fatalf("redactEngramCloudToken() error = %v", err)
	}
	if string(backup) != pluginInput {
		t.Fatalf("plugin.env token was incorrectly redacted; got %q, want %q", string(backup), pluginInput)
	}
}
