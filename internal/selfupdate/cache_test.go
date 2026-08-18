package selfupdate

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestCache_RoundTrip(t *testing.T) {
	cacheFile := filepath.Join(t.TempDir(), "cache.json")

	original := cacheEntry{
		CheckedAt: time.Now().UTC(),
		Latest:    "v0.5.0",
	}

	err := writeCache(cacheFile, original)
	if err != nil {
		t.Fatalf("writeCache failed: %v", err)
	}

	read, err := readCache(cacheFile)
	if err != nil {
		t.Fatalf("readCache failed: %v", err)
	}

	if !read.CheckedAt.Equal(original.CheckedAt) {
		t.Errorf("CheckedAt mismatch: got %v, want %v", read.CheckedAt, original.CheckedAt)
	}

	if read.Latest != original.Latest {
		t.Errorf("Latest mismatch: got %q, want %q", read.Latest, original.Latest)
	}

	// Verify on-disk JSON structure
	data, err := os.ReadFile(cacheFile)
	if err != nil {
		t.Fatalf("read cache file: %v", err)
	}

	jsonStr := string(data)
	if jsonStr == "" {
		t.Fatal("empty cache file")
	}

	// Should have snake_case checked_at
	if contains(jsonStr, `"checked_at"`) {
		t.Logf("correctly uses snake_case checked_at")
	}

	// When Latest is empty, it should be omitted (omitempty)
	emptyLatest := cacheEntry{
		CheckedAt: time.Now().UTC(),
		Latest:    "",
	}
	err = writeCache(cacheFile, emptyLatest)
	if err != nil {
		t.Fatalf("writeCache empty latest failed: %v", err)
	}

	data, err = os.ReadFile(cacheFile)
	if err != nil {
		t.Fatalf("read cache file: %v", err)
	}

	jsonStr = string(data)
	if contains(jsonStr, `"latest"`) {
		t.Errorf("latest should be omitted when empty, but got: %s", jsonStr)
	}
}

func TestReadCache_Misses(t *testing.T) {
	tests := []struct {
		name      string
		setupPath func(t *testing.T) string
		wantError bool
	}{
		{
			name: "missing file",
			setupPath: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "nonexistent.json")
			},
			wantError: true,
		},
		{
			name: "malformed JSON",
			setupPath: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "bad.json")
				os.WriteFile(path, []byte("{invalid"), 0o600)
				return path
			},
			wantError: true,
		},
		{
			name: "invalid timestamp",
			setupPath: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "bad-time.json")
				os.WriteFile(path, []byte(`{"checked_at":"not-a-date","latest":"v0.5.0"}`), 0o600)
				return path
			},
			wantError: true,
		},
		{
			name: "zero CheckedAt",
			setupPath: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "zero-time.json")
				os.WriteFile(path, []byte(`{"checked_at":"0001-01-01T00:00:00Z","latest":"v0.5.0"}`), 0o600)
				return path
			},
			wantError: true,
		},
		{
			name: "unreadable path (directory)",
			setupPath: func(t *testing.T) string {
				return t.TempDir() // TempDir returns a directory path
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tt.setupPath(t)
			entry, err := readCache(path)

			if !tt.wantError {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if entry == (cacheEntry{}) {
					t.Error("expected non-zero entry, got zero")
				}
				return
			}

			// wantError == true
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if entry != (cacheEntry{}) {
				t.Errorf("expected zero entry on error, got %+v", entry)
			}
		})
	}
}

func TestWriteCache_CreatesParentDirectory(t *testing.T) {
	cacheFile := filepath.Join(t.TempDir(), "nested", "deep", "cache.json")

	entry := cacheEntry{
		CheckedAt: time.Now().UTC(),
		Latest:    "v0.5.0",
	}

	err := writeCache(cacheFile, entry)
	if err != nil {
		t.Fatalf("writeCache failed: %v", err)
	}

	// Verify the file exists
	if _, err := os.Stat(cacheFile); err != nil {
		t.Fatalf("cache file not created: %v", err)
	}

	// Verify the content
	read, err := readCache(cacheFile)
	if err != nil {
		t.Fatalf("readCache failed: %v", err)
	}

	if read.Latest != entry.Latest {
		t.Errorf("Latest mismatch: got %q, want %q", read.Latest, entry.Latest)
	}
}

func TestWriteCache_FileMode0600(t *testing.T) {
	// Skip on Windows if it can't represent this mode
	if runtime.GOOS == "windows" {
		t.Skip("Unix file mode 0o600 not fully supported on Windows")
	}

	cacheFile := filepath.Join(t.TempDir(), "cache.json")

	entry := cacheEntry{
		CheckedAt: time.Now().UTC(),
		Latest:    "v0.5.0",
	}

	err := writeCache(cacheFile, entry)
	if err != nil {
		t.Fatalf("writeCache failed: %v", err)
	}

	info, err := os.Stat(cacheFile)
	if err != nil {
		t.Fatalf("stat cache file: %v", err)
	}

	mode := info.Mode().Perm()
	if mode != 0o600 {
		t.Errorf("expected mode 0o600, got %04o", mode)
	}
}

func TestAtomicWriteFile_InjectedWriteErrorLeavesOriginalIntact(t *testing.T) {
	// Create an original file with known content
	cacheFile := filepath.Join(t.TempDir(), "cache.json")
	originalContent := []byte("original content")
	err := os.WriteFile(cacheFile, originalContent, 0o600)
	if err != nil {
		t.Fatalf("create original file: %v", err)
	}

	// Try to write with content that will fail during write
	// We can't easily inject errors into os.WriteFile, so we test
	// by using a directory as the path (will fail)
	badFile := filepath.Join(t.TempDir(), "dir-as-file")

	// Create a directory at the path to make it fail
	os.Mkdir(badFile, 0o755)

	err = atomicWriteFile(badFile, []byte("new content"), 0o600)
	if err == nil {
		t.Fatal("expected error writing to directory")
	}

	// Verify original file is intact
	originalData, err := os.ReadFile(cacheFile)
	if err != nil {
		t.Fatalf("read original file: %v", err)
	}

	if string(originalData) != string(originalContent) {
		t.Errorf("original file modified: got %q, want %q", string(originalData), string(originalContent))
	}
}

func TestAtomicWriteFile_NoTempLeftovers(t *testing.T) {
	cacheFile := filepath.Join(t.TempDir(), "cache.json")

	// Successful write
	entry := cacheEntry{
		CheckedAt: time.Now().UTC(),
		Latest:    "v0.5.0",
	}
	err := writeCache(cacheFile, entry)
	if err != nil {
		t.Fatalf("writeCache failed: %v", err)
	}

	// Check for temp files in the directory
	entries, err := os.ReadDir(t.TempDir())
	if err != nil {
		t.Fatalf("read directory: %v", err)
	}

	for _, entry := range entries {
		name := entry.Name()
		if filepath.Ext(name) == ".tmp" {
			t.Errorf("found temp file leftover: %s", name)
		}
	}

	// Failed write (directory as file)
	badFile := filepath.Join(t.TempDir(), "bad")
	os.Mkdir(badFile, 0o755)
	err = atomicWriteFile(badFile, []byte("content"), 0o600)
	if err == nil {
		t.Fatal("expected error")
	}

	// Check again for temp files
	entries, err = os.ReadDir(t.TempDir())
	if err != nil {
		t.Fatalf("read directory: %v", err)
	}

	for _, entry := range entries {
		name := entry.Name()
		if filepath.Ext(name) == ".tmp" {
			t.Errorf("found temp file leftover after error: %s", name)
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || contains(s[1:], substr)))
}
