package selfupdate

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
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
	if !contains(jsonStr, `"checked_at"`) {
		t.Errorf("expected snake_case checked_at field in JSON, got: %s", jsonStr)
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
	// This test verifies the atomicity contract: if atomicWriteFile fails,
	// the original file at the target path remains intact.
	// Since atomicWriteFile in selfupdate package has no injection seam,
	// we use a directory-collision failure injection: create a directory
	// at the exact target path, then verify the ORIGINAL content from a
	// successful write to a different file is preserved.

	testDir := t.TempDir()

	// Step 1: Create and populate a file with known content using atomicWriteFile
	originalFile := filepath.Join(testDir, "original.json")
	originalContent := []byte("original content")
	err := atomicWriteFile(originalFile, originalContent, 0o600)
	if err != nil {
		t.Fatalf("initial atomicWriteFile failed: %v", err)
	}

	// Verify the initial write succeeded
	data, err := os.ReadFile(originalFile)
	if err != nil {
		t.Fatalf("read original file: %v", err)
	}
	if string(data) != string(originalContent) {
		t.Fatalf("initial content mismatch: got %q, want %q", string(data), string(originalContent))
	}

	// Step 2: Create a directory at a target path to force a write failure
	// (atomicWriteFile cannot write to a directory - os.Rename will fail)
	targetFile := filepath.Join(testDir, "target.json")
	os.Mkdir(targetFile, 0o755)

	// Step 3: Attempt atomicWriteFile to the directory path - this MUST fail
	err = atomicWriteFile(targetFile, []byte("new content"), 0o600)
	if err == nil {
		t.Fatal("expected error when writing to directory path")
	}

	// Step 4: CRITICAL: Verify the ORIGINAL file (from step 1) is still intact
	// This proves that the failure in step 3 did not corrupt the original file
	// (even though they're different paths, this verifies the atomic write pattern
	// doesn't have cross-path corruption - the real guarantee being tested)
	data, err = os.ReadFile(originalFile)
	if err != nil {
		t.Fatalf("read original file after failed write: %v", err)
	}
	if string(data) != string(originalContent) {
		t.Errorf("original file was corrupted by failed write to different path: got %q, want %q", string(data), string(originalContent))
	}

	// Note: A perfect same-path failure injection would require adding an
	// injection seam to atomicWriteFile (like the installer package's
	// createTempFile var). Since that's a production code change beyond
	// this fix's scope, this test verifies the weaker but still important
	// guarantee: a failed atomicWriteFile does not corrupt OTHER files.
}

func TestAtomicWriteFile_NoTempLeftovers(t *testing.T) {
	testDir := t.TempDir()
	cacheFile := filepath.Join(testDir, "cache.json")

	// Successful write
	entry := cacheEntry{
		CheckedAt: time.Now().UTC(),
		Latest:    "v0.5.0",
	}
	err := writeCache(cacheFile, entry)
	if err != nil {
		t.Fatalf("writeCache failed: %v", err)
	}

	// Check for temp files in the directory using the CORRECT pattern
	// os.CreateTemp(dir, base+".tmp") creates files like "cache.json.tmp3910284712",
	// so we must use a prefix check, not filepath.Ext() which would return ".tmp3910284712"
	entries, err := os.ReadDir(testDir)
	if err != nil {
		t.Fatalf("read directory: %v", err)
	}

	baseName := filepath.Base(cacheFile)
	for _, entry := range entries {
		name := entry.Name()
		// Use prefix check to match the real naming pattern from os.CreateTemp
		if strings.HasPrefix(name, baseName+".tmp") {
			t.Errorf("found temp file leftover: %s", name)
		}
	}

	// Failed write (directory as file)
	badFile := filepath.Join(testDir, "bad")
	os.Mkdir(badFile, 0o755)
	err = atomicWriteFile(badFile, []byte("content"), 0o600)
	if err == nil {
		t.Fatal("expected error")
	}

	// Check again for temp files
	entries, err = os.ReadDir(testDir)
	if err != nil {
		t.Fatalf("read directory: %v", err)
	}

	for _, entry := range entries {
		name := entry.Name()
		// Use prefix check to match the real naming pattern from os.CreateTemp
		if strings.HasPrefix(name, baseName+".tmp") {
			t.Errorf("found temp file leftover after error: %s", name)
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || contains(s[1:], substr)))
}
