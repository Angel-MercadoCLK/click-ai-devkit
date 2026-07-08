package audit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAppend_WritesHashOnly(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "memory-guard.jsonl")
	entry := Entry{
		Decision:      "deny",
		Category:      "pii",
		ContentSHA256: SHA256("customer email juan.perez@clickseguros.com.ar"),
		SessionID:     "session-123",
	}

	if err := Append(logPath, entry); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if strings.Contains(string(data), "juan.perez@clickseguros.com.ar") {
		t.Fatal("audit log contains raw payload, want hashes only")
	}

	var got Entry
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if got.ContentSHA256 != entry.ContentSHA256 {
		t.Fatalf("ContentSHA256 = %q, want %q", got.ContentSHA256, entry.ContentSHA256)
	}
	if got.SessionID != entry.SessionID {
		t.Fatalf("SessionID = %q, want %q", got.SessionID, entry.SessionID)
	}
}
