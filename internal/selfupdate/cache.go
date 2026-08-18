package selfupdate

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// cacheEntry represents the stored cache data.
type cacheEntry struct {
	CheckedAt time.Time `json:"checked_at"`
	Latest    string    `json:"latest,omitempty"`
}

// readCache reads the cache from disk.
// Returns a zero cacheEntry and a non-nil error if the file doesn't exist,
// is malformed, or cannot be read.
func readCache(path string) (cacheEntry, error) {
	var zero cacheEntry

	data, err := os.ReadFile(path)
	if err != nil {
		return zero, fmt.Errorf("read cache: %w", err)
	}

	var entry cacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return zero, fmt.Errorf("decode cache: %w", err)
	}

	if entry.CheckedAt.IsZero() {
		return zero, fmt.Errorf("invalid cache: missing checked_at")
	}

	return entry, nil
}

// writeCache writes the cache to disk atomically.
// Creates parent directories if needed.
func writeCache(path string, entry cacheEntry) error {
	// Create parent directories if they don't exist
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create cache dir: %w", err)
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("encode cache: %w", err)
	}

	return atomicWriteFile(path, data, 0o600)
}

// atomicWriteFile writes content to path atomically.
// Follows the temp-file+rename pattern: write to temp file in same dir,
// sync to disk, close, chmod, then rename over target.
// If any step fails, the original file remains intact.
func atomicWriteFile(path string, content []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	base := filepath.Base(path)

	// Create temp file in the same directory to ensure same filesystem
	tempFile, err := os.CreateTemp(dir, base+".tmp")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	tempPath := tempFile.Name()

	// Clean up the temp file unless it was successfully renamed into place.
	// The handle must be closed before removal: on Windows an open file cannot
	// be deleted, so an early return that skipped Close would leak both the
	// descriptor and the temp file.
	closed := false
	renamed := false
	defer func() {
		if !closed {
			tempFile.Close()
		}
		if !renamed {
			os.Remove(tempPath)
		}
	}()

	// Write content
	if _, err := tempFile.Write(content); err != nil {
		return fmt.Errorf("write temp: %w", err)
	}

	// Sync to ensure data is on disk
	if err := tempFile.Sync(); err != nil {
		return fmt.Errorf("sync temp: %w", err)
	}

	// Close the file before chmod (Windows requirement)
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("close temp: %w", err)
	}
	closed = true

	// Set permissions
	if err := os.Chmod(tempPath, mode); err != nil {
		return fmt.Errorf("chmod temp: %w", err)
	}

	// Atomic rename over target
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("rename: %w", err)
	}
	renamed = true

	// Temp file is now the target, don't delete it
	return nil
}
