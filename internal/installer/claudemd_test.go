package installer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	return string(data)
}

func TestWriteManagedBlock_CreatesFileWhenMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "CLAUDE.md")

	if err := WriteManagedBlock(path, "hello from click"); err != nil {
		t.Fatalf("WriteManagedBlock() error = %v", err)
	}

	got := readFile(t, path)
	if !strings.Contains(got, managedBeginMarker) || !strings.Contains(got, managedEndMarker) {
		t.Fatalf("WriteManagedBlock() output = %q, want both markers present", got)
	}
	if !strings.Contains(got, "hello from click") {
		t.Fatalf("WriteManagedBlock() output = %q, want the content present", got)
	}
}

func TestWriteManagedBlock_AppendsPreservingExistingContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "CLAUDE.md")
	existing := "# My own notes\nSome existing developer content.\n"
	if err := os.WriteFile(path, []byte(existing), 0o644); err != nil {
		t.Fatalf("seed WriteFile() error = %v", err)
	}

	if err := WriteManagedBlock(path, "managed content"); err != nil {
		t.Fatalf("WriteManagedBlock() error = %v", err)
	}

	got := readFile(t, path)
	if !strings.Contains(got, "# My own notes") || !strings.Contains(got, "Some existing developer content.") {
		t.Fatalf("WriteManagedBlock() output = %q, want existing content preserved", got)
	}
	if !strings.Contains(got, managedBeginMarker) {
		t.Fatalf("WriteManagedBlock() output = %q, want the managed block appended", got)
	}
	// existing content must come before the managed block
	if strings.Index(got, "Some existing developer content.") > strings.Index(got, managedBeginMarker) {
		t.Fatalf("WriteManagedBlock() output = %q, want existing content before the managed block", got)
	}
}

func TestWriteManagedBlock_IdempotentReinsertNoDuplication(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "CLAUDE.md")

	if err := WriteManagedBlock(path, "same content"); err != nil {
		t.Fatalf("first WriteManagedBlock() error = %v", err)
	}
	if err := WriteManagedBlock(path, "same content"); err != nil {
		t.Fatalf("second WriteManagedBlock() error = %v", err)
	}

	got := readFile(t, path)
	if n := strings.Count(got, managedBeginMarker); n != 1 {
		t.Fatalf("WriteManagedBlock() begin marker appears %d times, want exactly 1 (got %q)", n, got)
	}
	if n := strings.Count(got, managedEndMarker); n != 1 {
		t.Fatalf("WriteManagedBlock() end marker appears %d times, want exactly 1 (got %q)", n, got)
	}
}

func TestWriteManagedBlock_ReplacesInPlaceWhenContentChanges(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "CLAUDE.md")
	existing := "# Before\nkeep me\n"
	if err := os.WriteFile(path, []byte(existing), 0o644); err != nil {
		t.Fatalf("seed WriteFile() error = %v", err)
	}

	if err := WriteManagedBlock(path, "version A"); err != nil {
		t.Fatalf("first WriteManagedBlock() error = %v", err)
	}
	if err := WriteManagedBlock(path, "version B"); err != nil {
		t.Fatalf("second WriteManagedBlock() error = %v", err)
	}

	got := readFile(t, path)
	if strings.Contains(got, "version A") {
		t.Fatalf("WriteManagedBlock() output = %q, want the old block content replaced, not kept", got)
	}
	if !strings.Contains(got, "version B") {
		t.Fatalf("WriteManagedBlock() output = %q, want the new block content present", got)
	}
	if !strings.Contains(got, "keep me") {
		t.Fatalf("WriteManagedBlock() output = %q, want unrelated content preserved", got)
	}
	if n := strings.Count(got, managedBeginMarker); n != 1 {
		t.Fatalf("WriteManagedBlock() begin marker appears %d times, want exactly 1", n)
	}
}

func TestStripManagedBlock_RemovesBlockPreservesSurroundingContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "CLAUDE.md")

	before := "# My own notes\nSome existing content\n"
	after := "# trailing developer content\n"
	full := before + managedBeginMarker + "\nmanaged body\n" + managedEndMarker + "\n" + after
	if err := os.WriteFile(path, []byte(full), 0o644); err != nil {
		t.Fatalf("seed WriteFile() error = %v", err)
	}

	if err := StripManagedBlock(path); err != nil {
		t.Fatalf("StripManagedBlock() error = %v", err)
	}

	got := readFile(t, path)
	if strings.Contains(got, managedBeginMarker) || strings.Contains(got, managedEndMarker) || strings.Contains(got, "managed body") {
		t.Fatalf("StripManagedBlock() output = %q, want the managed block fully removed", got)
	}
	want := before + after
	if got != want {
		t.Fatalf("StripManagedBlock() output = %q, want surrounding content byte-for-byte intact: %q", got, want)
	}
}

func TestStripManagedBlock_NoopWhenBlockAbsent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "CLAUDE.md")
	original := "# just developer content, no click block here\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatalf("seed WriteFile() error = %v", err)
	}

	if err := StripManagedBlock(path); err != nil {
		t.Fatalf("StripManagedBlock() error = %v", err)
	}

	got := readFile(t, path)
	if got != original {
		t.Fatalf("StripManagedBlock() modified a file with no managed block: got %q, want %q", got, original)
	}
}

func TestStripManagedBlock_NoopWhenFileMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "CLAUDE.md")

	if err := StripManagedBlock(path); err != nil {
		t.Fatalf("StripManagedBlock() on a missing file error = %v, want nil", err)
	}

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("StripManagedBlock() on a missing file created one at %s", path)
	}
}

func TestHasManagedBlock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "CLAUDE.md")

	got, err := HasManagedBlock(path)
	if err != nil {
		t.Fatalf("HasManagedBlock() on missing file error = %v", err)
	}
	if got {
		t.Fatal("HasManagedBlock() on missing file = true, want false")
	}

	if err := WriteManagedBlock(path, "content"); err != nil {
		t.Fatalf("WriteManagedBlock() error = %v", err)
	}

	got, err = HasManagedBlock(path)
	if err != nil {
		t.Fatalf("HasManagedBlock() error = %v", err)
	}
	if !got {
		t.Fatal("HasManagedBlock() after WriteManagedBlock() = false, want true")
	}

	if err := StripManagedBlock(path); err != nil {
		t.Fatalf("StripManagedBlock() error = %v", err)
	}

	got, err = HasManagedBlock(path)
	if err != nil {
		t.Fatalf("HasManagedBlock() error = %v", err)
	}
	if got {
		t.Fatal("HasManagedBlock() after StripManagedBlock() = true, want false")
	}
}
