package audit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/installer"
)

const auditLogEnvOverride = "CLICK_MEMORY_GUARD_AUDIT_LOG_PATH"

// Entry is a single local JSONL audit record for one memory-guard decision.
type Entry struct {
	Timestamp     time.Time `json:"ts"`
	Decision      string    `json:"decision"`
	Category      string    `json:"category,omitempty"`
	ContentSHA256 string    `json:"content_sha256"`
	SessionID     string    `json:"session_id,omitempty"`
}

// ResolveLogPath returns the configured audit-log path. Tests should override it with
// CLICK_MEMORY_GUARD_AUDIT_LOG_PATH and a TempDir-backed file.
func ResolveLogPath() (string, error) {
	if v := os.Getenv(auditLogEnvOverride); v != "" {
		return v, nil
	}
	claudeHome, err := installer.ResolveClaudeHome()
	if err != nil {
		return "", fmt.Errorf("audit: resolve log path: %w", err)
	}
	return filepath.Join(claudeHome, "logs", "click-memory-guard.jsonl"), nil
}

// SHA256 returns the lowercase SHA-256 hex digest for the raw payload content.
func SHA256(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

// Append writes one JSON line to logPath, creating parent directories as needed.
func Append(logPath string, entry Entry) error {
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now().UTC()
	}
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return fmt.Errorf("audit: create log dir: %w", err)
	}
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("audit: open log: %w", err)
	}
	defer f.Close()

	encoded, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("audit: marshal entry: %w", err)
	}
	if _, err := f.Write(append(encoded, '\n')); err != nil {
		return fmt.Errorf("audit: append entry: %w", err)
	}
	return nil
}
