package installer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Managed-block markers. Per this slice's brief these are `#`-comment style (CLAUDE.md is
// markdown, so a leading "#" still renders as a heading if ever viewed raw — acceptable for a
// machine-oriented marker) rather than the HTML-comment style tech-spec.md §2.4 sketches; the
// exact text is pinned by the implementation brief, so it's what's implemented here. Logged as a
// deviation in Engram (click-ai-devkit/build/slice-1).
const (
	managedBeginMarker = "# >>> click-ai-devkit (managed) >>>"
	managedEndMarker   = "# <<< click-ai-devkit (managed) <<<"
)

// DefaultManagedContent is the Slice 3 managed CLAUDE.md body: it activates ClickOrchestrator,
// explains the Spanish/English split, points at the memory-policy docs, and reminds the user that
// click manages this block.
const DefaultManagedContent = `Use ClickOrchestrator by default for this repository and delegate phase work to the click-sdd agents and skills.
Reply to the developer in Spanish. Produce all artifacts (PRD, design, tasks, code comments, and memory entries) in English. Keep explanations plain, direct, and free of regional slang.
Before any mem_save, review plugins/click-memory/docs/memory-policy.md, allowed-memory.md, and forbidden-memory.md. The deterministic memory-guard hook enforces this policy even if a model attempts something unsafe.
Before opening or merging a PR, run the click-review pre-merge checklist.
This block is managed by click: edit via "click update" and remove via "click uninstall".`

// WriteManagedBlock inserts or replaces the click-ai-devkit managed block in the file at path.
// It creates the file (and any parent directories) if missing, appends the block if no markers
// are present yet, and replaces the block in place if markers already exist — idempotent by
// construction: writing the same content twice leaves the file unchanged after the first write.
// Content outside the markers is always preserved byte-for-byte.
func WriteManagedBlock(path, content string) error {
	existing, err := readFileOrEmpty(path)
	if err != nil {
		return fmt.Errorf("installer: read %s: %w", path, err)
	}

	block := buildManagedBlock(content)
	updated, changed := spliceManagedBlock(existing, block)
	if !changed {
		return nil
	}

	if err := writeFileEnsuringDir(path, updated); err != nil {
		return fmt.Errorf("installer: write %s: %w", path, err)
	}
	return nil
}

// StripManagedBlock removes the click-ai-devkit managed block (markers included) from the file
// at path, leaving the rest of the file byte-for-byte intact. It is a no-op — no error, no
// modification, no file created — if the file doesn't exist or doesn't contain the markers.
func StripManagedBlock(path string) error {
	existing, err := readFileOrEmpty(path)
	if err != nil {
		return fmt.Errorf("installer: read %s: %w", path, err)
	}
	if existing == "" {
		return nil
	}

	updated, changed := removeManagedBlock(existing)
	if !changed {
		return nil
	}

	if err := writeFileEnsuringDir(path, updated); err != nil {
		return fmt.Errorf("installer: write %s: %w", path, err)
	}
	return nil
}

// HasManagedBlock reports whether the file at path currently contains a well-formed managed
// block (both markers present, end after begin). Used by `click doctor`'s CLAUDE.md check.
func HasManagedBlock(path string) (bool, error) {
	existing, err := readFileOrEmpty(path)
	if err != nil {
		return false, fmt.Errorf("installer: read %s: %w", path, err)
	}
	begin, end := findMarkers(splitLines(existing))
	return begin != -1 && end != -1, nil
}

func buildManagedBlock(content string) []string {
	body := splitLines(content)
	lines := make([]string, 0, len(body)+2)
	lines = append(lines, managedBeginMarker)
	lines = append(lines, body...)
	lines = append(lines, managedEndMarker)
	return lines
}

// splitLines splits s into lines with no trailing empty element for a trailing newline. An empty
// string yields a nil slice (zero lines), matching "file doesn't exist / is empty".
func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(strings.TrimRight(s, "\n"), "\n")
}

// joinLines is splitLines's inverse: it always terminates the result with a single trailing
// newline (or returns "" for zero lines), so files written by this package end cleanly.
func joinLines(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}

// findMarkers locates the first well-formed begin/end marker pair in lines. Returns -1, -1 if
// no such pair exists (e.g. only a begin marker with no matching end after it).
func findMarkers(lines []string) (begin, end int) {
	begin, end = -1, -1
	for i, l := range lines {
		if l == managedBeginMarker && begin == -1 {
			begin = i
		}
		if l == managedEndMarker && begin != -1 && i > begin {
			end = i
			break
		}
	}
	if end == -1 {
		begin = -1
	}
	return begin, end
}

// spliceManagedBlock inserts block into existing (replacing an existing block in place if one
// is found, appending otherwise). changed is false when the result would be byte-identical to
// existing, so callers can skip an unnecessary write.
func spliceManagedBlock(existing string, block []string) (result string, changed bool) {
	lines := splitLines(existing)
	begin, end := findMarkers(lines)

	if begin != -1 && end != -1 {
		current := lines[begin : end+1]
		if equalLines(current, block) {
			return existing, false
		}
		newLines := make([]string, 0, len(lines)-len(current)+len(block))
		newLines = append(newLines, lines[:begin]...)
		newLines = append(newLines, block...)
		newLines = append(newLines, lines[end+1:]...)
		return joinLines(newLines), true
	}

	newLines := make([]string, 0, len(lines)+len(block))
	newLines = append(newLines, lines...)
	newLines = append(newLines, block...)
	return joinLines(newLines), true
}

// removeManagedBlock strips the first well-formed marker pair (and everything between, inclusive)
// from existing. changed is false when no such pair was found, so callers can treat it as a
// true no-op.
func removeManagedBlock(existing string) (result string, changed bool) {
	lines := splitLines(existing)
	begin, end := findMarkers(lines)
	if begin == -1 || end == -1 {
		return existing, false
	}
	newLines := make([]string, 0, len(lines)-(end-begin+1))
	newLines = append(newLines, lines[:begin]...)
	newLines = append(newLines, lines[end+1:]...)
	return joinLines(newLines), true
}

func equalLines(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func readFileOrEmpty(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return string(data), nil
}

func writeFileEnsuringDir(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}
