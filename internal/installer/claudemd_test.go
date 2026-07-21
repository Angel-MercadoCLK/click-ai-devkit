package installer

import (
	"errors"
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

// containsBareLF reports whether s contains a "\n" that is NOT part of a "\r\n" pair — i.e. a bare
// LF line ending mixed into otherwise-CRLF content. Used to assert that CRLF-preserving writes
// don't silently introduce LF-only lines.
func containsBareLF(s string) bool {
	return strings.Contains(strings.ReplaceAll(s, "\r\n", ""), "\n")
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

// --- CRLF regression coverage (Finding 1) ---
//
// On a CRLF-saved file (Notepad, or any editor with files.eol: "\r\n" — common on Windows, this
// project's primary supported platform), the old splitLines/findMarkers left every line ending in
// a trailing "\r", so exact-string marker comparison never matched. That made WriteManagedBlock
// append a brand-new block on every run instead of replacing (violating idempotency),
// StripManagedBlock a silent no-op, and HasManagedBlock report "missing" for a block that was
// actually present. The tests below exercise each of those symptoms directly against CRLF input.

func TestWriteManagedBlock_InsertIntoCRLFFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "CLAUDE.md")
	existing := "# My own notes\r\nSome existing developer content.\r\n"
	if err := os.WriteFile(path, []byte(existing), 0o644); err != nil {
		t.Fatalf("seed WriteFile() error = %v", err)
	}

	if err := WriteManagedBlock(path, "managed content"); err != nil {
		t.Fatalf("WriteManagedBlock() error = %v", err)
	}

	got := readFile(t, path)
	if !strings.Contains(got, "# My own notes\r\n") {
		t.Fatalf("WriteManagedBlock() output = %q, want the original CRLF-ended developer line preserved as CRLF", got)
	}
	if containsBareLF(got) {
		t.Fatalf("WriteManagedBlock() output = %q, want the whole file to stay CRLF (no bare \\n introduced)", got)
	}
	if !strings.Contains(got, managedBeginMarker+"\r\n") {
		t.Fatalf("WriteManagedBlock() output = %q, want the managed block written with CRLF endings to match the file", got)
	}
}

func TestWriteManagedBlock_ReplaceExistingCRLFBlockNoDuplication(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "CLAUDE.md")
	existing := "# Before\r\nkeep me\r\n" + managedBeginMarker + "\r\nversion A\r\n" + managedEndMarker + "\r\n"
	if err := os.WriteFile(path, []byte(existing), 0o644); err != nil {
		t.Fatalf("seed WriteFile() error = %v", err)
	}

	if err := WriteManagedBlock(path, "version B"); err != nil {
		t.Fatalf("WriteManagedBlock() error = %v", err)
	}

	got := readFile(t, path)
	if n := strings.Count(got, managedBeginMarker); n != 1 {
		t.Fatalf("WriteManagedBlock() begin marker appears %d times on a CRLF file, want exactly 1 (got %q)", n, got)
	}
	if n := strings.Count(got, managedEndMarker); n != 1 {
		t.Fatalf("WriteManagedBlock() end marker appears %d times on a CRLF file, want exactly 1 (got %q)", n, got)
	}
	if strings.Contains(got, "version A") {
		t.Fatalf("WriteManagedBlock() output = %q, want the old CRLF block content replaced, not kept", got)
	}
	if !strings.Contains(got, "version B") {
		t.Fatalf("WriteManagedBlock() output = %q, want the new content present", got)
	}
	if !strings.Contains(got, "keep me") {
		t.Fatalf("WriteManagedBlock() output = %q, want unrelated content preserved", got)
	}
}

func TestStripManagedBlock_RemovesCRLFBlock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "CLAUDE.md")
	before := "# My own notes\r\nSome existing content\r\n"
	after := "# trailing developer content\r\n"
	full := before + managedBeginMarker + "\r\nmanaged body\r\n" + managedEndMarker + "\r\n" + after
	if err := os.WriteFile(path, []byte(full), 0o644); err != nil {
		t.Fatalf("seed WriteFile() error = %v", err)
	}

	if err := StripManagedBlock(path); err != nil {
		t.Fatalf("StripManagedBlock() error = %v", err)
	}

	got := readFile(t, path)
	if strings.Contains(got, managedBeginMarker) || strings.Contains(got, managedEndMarker) || strings.Contains(got, "managed body") {
		t.Fatalf("StripManagedBlock() output = %q, want the CRLF managed block fully removed", got)
	}
	want := before + after
	if got != want {
		t.Fatalf("StripManagedBlock() output = %q, want surrounding CRLF content byte-for-byte intact: %q", got, want)
	}
}

func TestHasManagedBlock_TrueForCRLFFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "CLAUDE.md")
	full := managedBeginMarker + "\r\nbody\r\n" + managedEndMarker + "\r\n"
	if err := os.WriteFile(path, []byte(full), 0o644); err != nil {
		t.Fatalf("seed WriteFile() error = %v", err)
	}

	got, err := HasManagedBlock(path)
	if err != nil {
		t.Fatalf("HasManagedBlock() error = %v", err)
	}
	if !got {
		t.Fatal("HasManagedBlock() on a well-formed CRLF block = false, want true")
	}
}

// TestWriteManagedBlock_IdempotentOnCRLFFile simulates the realistic sequence: click first writes
// the file (LF, its own default), the developer's editor re-saves it as CRLF between runs, then
// click runs twice more with identical content. Exactly one block must remain throughout.
func TestWriteManagedBlock_IdempotentOnCRLFFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "CLAUDE.md")

	if err := WriteManagedBlock(path, "same content"); err != nil {
		t.Fatalf("first WriteManagedBlock() error = %v", err)
	}

	lf := readFile(t, path)
	crlf := strings.ReplaceAll(lf, "\n", "\r\n")
	if err := os.WriteFile(path, []byte(crlf), 0o644); err != nil {
		t.Fatalf("simulate editor CRLF re-save WriteFile() error = %v", err)
	}

	if err := WriteManagedBlock(path, "same content"); err != nil {
		t.Fatalf("second WriteManagedBlock() error = %v", err)
	}
	if err := WriteManagedBlock(path, "same content"); err != nil {
		t.Fatalf("third WriteManagedBlock() error = %v", err)
	}

	got := readFile(t, path)
	if n := strings.Count(got, managedBeginMarker); n != 1 {
		t.Fatalf("WriteManagedBlock() begin marker appears %d times after repeated runs on a CRLF file, want exactly 1 (got %q)", n, got)
	}
	if n := strings.Count(got, managedEndMarker); n != 1 {
		t.Fatalf("WriteManagedBlock() end marker appears %d times after repeated runs on a CRLF file, want exactly 1 (got %q)", n, got)
	}
}

// TestWriteManagedBlock_MixedLineEndingFile covers a hand-edited file with both CRLF and bare-LF
// lines: WriteManagedBlock must neither crash nor lose/corrupt any surrounding content, and must
// still find and replace (not duplicate) an existing CRLF-styled block.
func TestWriteManagedBlock_MixedLineEndingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "CLAUDE.md")
	existing := "# notes\r\nplain-lf-line\n" + managedBeginMarker + "\r\nversion A\r\n" + managedEndMarker + "\r\nmore\r\n"
	if err := os.WriteFile(path, []byte(existing), 0o644); err != nil {
		t.Fatalf("seed WriteFile() error = %v", err)
	}

	if err := WriteManagedBlock(path, "version B"); err != nil {
		t.Fatalf("WriteManagedBlock() on a mixed-line-ending file error = %v, want no crash/error", err)
	}

	got := readFile(t, path)
	if n := strings.Count(got, managedBeginMarker); n != 1 {
		t.Fatalf("WriteManagedBlock() begin marker appears %d times on a mixed-ending file, want exactly 1 (got %q)", n, got)
	}
	for _, want := range []string{"# notes", "plain-lf-line", "more", "version B"} {
		if !strings.Contains(got, want) {
			t.Fatalf("WriteManagedBlock() output = %q, want %q preserved (no content loss/corruption on a mixed-ending file)", got, want)
		}
	}
	if strings.Contains(got, "version A") {
		t.Fatalf("WriteManagedBlock() output = %q, want the old block content replaced", got)
	}
}

// TestWriteManagedBlock_PreservesOriginalCRLFEndingRoundTrip is the explicit "preserve the file's
// original dominant line ending" assertion: rewriting a CRLF file must not silently normalize it
// to LF (which would blow up a developer's git diff on every install/update run).
func TestWriteManagedBlock_PreservesOriginalCRLFEndingRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "CLAUDE.md")
	existing := "# Before\r\nkeep me\r\n"
	if err := os.WriteFile(path, []byte(existing), 0o644); err != nil {
		t.Fatalf("seed WriteFile() error = %v", err)
	}

	if err := WriteManagedBlock(path, "managed content"); err != nil {
		t.Fatalf("WriteManagedBlock() error = %v", err)
	}

	got := readFile(t, path)
	if containsBareLF(got) {
		t.Fatalf("WriteManagedBlock() output = %q, want the file's original CRLF line ending preserved throughout (no bare LF introduced)", got)
	}
	if !strings.Contains(got, "\r\n") {
		t.Fatalf("WriteManagedBlock() output = %q, want CRLF endings present", got)
	}
}

// --- Symlink / atomic-write regression coverage (Finding 2) ---

// TestWriteManagedBlock_WritesThroughSymlinkPreservingIt covers the CLAUDE.md half of the
// symlink-safety finding: a developer managing ~/.claude/ with a dotfiles repo commonly has
// CLAUDE.md symlinked into their dotfiles checkout. WriteManagedBlock must write through that
// symlink to its real target, leaving the symlink itself undisturbed.
func TestWriteManagedBlock_WritesThroughSymlinkPreservingIt(t *testing.T) {
	requireSymlinkSupport(t)

	root := t.TempDir()
	realDir := filepath.Join(root, "real")
	linkDir := filepath.Join(root, "linked")
	if err := os.MkdirAll(realDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(realDir) error = %v", err)
	}
	if err := os.MkdirAll(linkDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(linkDir) error = %v", err)
	}

	realTarget := filepath.Join(realDir, "CLAUDE.md")
	if err := os.WriteFile(realTarget, []byte("# dotfiles-managed notes\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(realTarget) error = %v", err)
	}

	symlinkPath := filepath.Join(linkDir, "CLAUDE.md")
	if err := os.Symlink(realTarget, symlinkPath); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}

	if err := WriteManagedBlock(symlinkPath, "managed content"); err != nil {
		t.Fatalf("WriteManagedBlock() error = %v", err)
	}

	info, err := os.Lstat(symlinkPath)
	if err != nil {
		t.Fatalf("Lstat(symlinkPath) error = %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("symlinkPath is no longer a symlink after WriteManagedBlock() (mode = %v) — it was destructively de-symlinked", info.Mode())
	}

	gotContent := readFile(t, realTarget)
	if !strings.Contains(gotContent, "# dotfiles-managed notes") || !strings.Contains(gotContent, managedBeginMarker) {
		t.Fatalf("realTarget content = %q, want the original content preserved plus the managed block written through", gotContent)
	}
}

// TestWriteManagedBlock_InjectedWriteErrorLeavesOriginalIntact is the strict-TDD RED/GREEN proof
// that writeFileEnsuringDir now goes through atomicWriteFile's temp-file+rename path instead of a
// direct, non-atomic os.WriteFile: injecting a failing createTempFile must surface an error AND
// leave the original file byte-for-byte untouched. Against the old os.WriteFile-based
// implementation this injection is a no-op (createTempFile is never consulted), so the write
// silently succeeds and this test fails.
func TestWriteManagedBlock_InjectedWriteErrorLeavesOriginalIntact(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "CLAUDE.md")
	original := "# original content that must survive\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatalf("seed WriteFile() error = %v", err)
	}

	injectedErr := errors.New("injected write failure")
	old := createTempFile
	createTempFile = func(dir, pattern string) (tempFileWriter, error) {
		return &fakeFailingTempFile{name: filepath.Join(dir, ".click-injected-fake"), writeErr: injectedErr}, nil
	}
	defer func() { createTempFile = old }()

	err := WriteManagedBlock(path, "new content that must never land")
	if err == nil {
		t.Fatal("WriteManagedBlock() error = nil, want the injected write error to propagate (proves it now goes through atomicWriteFile)")
	}

	got := readFile(t, path)
	if got != original {
		t.Fatalf("file content = %q after a failed write, want untouched original %q", got, original)
	}
}
