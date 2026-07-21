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
//
// Hardening T1-3: the memory-policy reference used to be a bare repo-relative path
// ("plugins/click-memory/docs/memory-policy.md"), which is wrong/ambiguous for any developer whose
// CLAUDE.md lives outside the click-ai-devkit repo itself (the real installed file lives under
// Claude Code's own plugin cache, e.g. ~/.claude/plugins/cache/click-ai-devkit/click-memory/<ver>/
// docs/). This constant has no access to ClaudeHome or the installed plugin version at generation
// time (it is a plain string, not a template — see WriteManagedBlock's callers in install.go/
// update.go), so resolving a real absolute path here would require threading Config and the
// click-memory plugin's resolved version through this constant into a template function, which is
// non-trivial plumbing for a low-severity issue. Right-sized fix: the wording below no longer
// claims a specific (and possibly wrong) path — it names the plugin and doc filenames only. If a
// future change wants the fully-resolved absolute path, it needs a deliberate scoping decision (a
// template function taking Config + the click-memory version), not a quick patch here.
const DefaultManagedContent = `Use ClickOrchestrator by default for this repository and delegate phase work to the click-sdd agents and skills.
Reply to the developer in Spanish. Produce all artifacts (PRD, design, tasks, code comments, and memory entries) in English. Keep explanations plain, direct, and free of regional slang.
Before any mem_save, review the click-memory plugin's policy docs (memory-policy.md, allowed-memory.md, forbidden-memory.md) under its installed docs/ directory. The deterministic memory-guard hook enforces this policy even if a model attempts something unsafe.
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
	begin, end := findMarkers(crlfAwareSplitLines(existing))
	return begin != -1 && end != -1, nil
}

// ManagedBlockBody returns the current managed block's BODY — the lines strictly between the
// begin/end markers — as it exists in the file at path right now. ok is false (with no error)
// when the file has no well-formed managed block, mirroring HasManagedBlock's own "well-formed"
// definition, so callers can tell "nothing to hash" apart from a real read error. Used by `click
// doctor`'s body-hash drift check (managed-block-integrity capability) — this function only reads,
// it never writes.
func ManagedBlockBody(path string) (body string, ok bool, err error) {
	existing, err := readFileOrEmpty(path)
	if err != nil {
		return "", false, fmt.Errorf("installer: read %s: %w", path, err)
	}
	lines := crlfAwareSplitLines(existing)
	begin, end := findMarkers(lines)
	if begin == -1 || end == -1 {
		return "", false, nil
	}
	return joinLines(lines[begin+1 : end]), true, nil
}

// ManagedBlockBodyHash returns the canonical sha256 hex digest of the live managed block's body at
// path, via canonicalContentHash (snapshot.go) — the SAME LF-canonicalization + hash algorithm
// PR3's rollback drift check already uses, so a CRLF-saved managed block never counts as drift
// here either. ok mirrors ManagedBlockBody's: false (no error) means there is no well-formed
// managed block to hash at all.
func ManagedBlockBodyHash(path string) (hash string, ok bool, err error) {
	body, ok, err := ManagedBlockBody(path)
	if err != nil || !ok {
		return "", ok, err
	}
	return canonicalContentHash(body), true, nil
}

// ExpectedManagedBlockHash returns the canonical sha256 hash of THIS click version's compile-time
// DefaultManagedContent — `click doctor`'s expected-value baseline for the managed-block drift
// check (design's "Drift hash" decision: no sidecar, compare live body to DefaultManagedContent).
// It is always computed fresh at call time; nothing about it is ever persisted or cached, so there
// is no baseline file that can itself drift out of sync with the binary.
func ExpectedManagedBlockHash() string {
	return canonicalContentHash(DefaultManagedContent)
}

func buildManagedBlock(content string) []string {
	body := crlfAwareSplitLines(content)
	lines := make([]string, 0, len(body)+2)
	lines = append(lines, managedBeginMarker)
	lines = append(lines, body...)
	lines = append(lines, managedEndMarker)
	return lines
}

// splitLines splits s into lines with no trailing empty element for a trailing newline. An empty
// string yields a nil slice (zero lines), matching "file doesn't exist / is empty".
//
// NOTE: this does NOT strip a trailing "\r" from CRLF-ended lines — it is also used unchanged by
// pathenv_unix.go's POSIX shell-rc-file managed-block logic, which is out of scope for the CRLF
// fix below and must not have its behavior altered as a side effect. claudemd.go's own managed-
// block logic uses crlfAwareSplitLines (below) instead, precisely to avoid touching this shared
// helper.
func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(strings.TrimRight(s, "\n"), "\n")
}

// joinLines is splitLines's inverse: it always terminates the result with a single trailing
// newline (or returns "" for zero lines), so files written by this package end cleanly. Also used
// unchanged by pathenv_unix.go — see splitLines's note above.
func joinLines(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}

// crlfAwareSplitLines is claudemd.go's own line-ending-agnostic variant of splitLines: it splits
// exactly like splitLines, then additionally trims a trailing "\r" from every line. This is the
// fix for the CRLF managed-block bug: on a CRLF-saved file (Notepad, or any editor configured with
// files.eol: "\r\n" — the common case on Windows, this project's primary supported platform),
// plain strings.Split(s, "\n") leaves every line ending in "\r", so findMarkers's exact-string
// comparison against the \r-free managedBeginMarker/managedEndMarker constants never matched.
// That made WriteManagedBlock append a brand-new managed block on every run instead of replacing
// the existing one (breaking the "idempotent by construction" contract), StripManagedBlock a
// silent no-op that could never find the block to remove, and HasManagedBlock report a present
// block as missing.
//
// Deliberately kept separate from splitLines/joinLines (rather than changing them in place):
// those two are also used unchanged by pathenv_unix.go's POSIX shell-rc-file managed-block logic,
// which is a different file/behavior out of scope for this fix.
func crlfAwareSplitLines(s string) []string {
	lines := splitLines(s)
	for i, l := range lines {
		lines[i] = strings.TrimSuffix(l, "\r")
	}
	return lines
}

// detectLineEnding inspects s — the file's ORIGINAL, unmodified content — and reports which line
// ending WriteManagedBlock/StripManagedBlock should use when writing the file back out.
//
// Design decision (finding: CRLF handling must be line-ending agnostic): PRESERVE the file's
// existing dominant line ending rather than normalizing everything to LF. Normalizing would
// silently rewrite EVERY line of a developer's CRLF-saved CLAUDE.md into a massive, unrelated git
// diff on their very next `click update`, even though only the managed block's own content
// actually changed — a much worse outcome than simply matching what their editor already writes.
//
// It counts CRLF ("\r\n") occurrences against bare-LF ("\n" not immediately preceded by "\r") and
// returns whichever style is more frequent — the file's DOMINANT ending. A genuinely
// mixed-line-ending file (e.g. hand-edited across two different editors/tools) is handled by this
// same majority rule rather than treated as an error: nothing crashes and no line's content is
// lost (crlfAwareSplitLines strips "\r" from every line regardless of position before any
// comparison happens) — the file is simply normalized to ONE consistent ending on write, matching
// whichever style already predominates. Ties — including an empty file, a file with no newlines at
// all, or an exact count tie — default to "\n", matching this package's original LF-only behavior
// and the correct choice for a file this package is creating from scratch.
//
// Out of scope: legacy Mac-classic bare-"\r" (no "\n") line endings are not specifically detected
// or normalized; such content is exceptionally rare from any editor still in use and was not part
// of the reported finding (CRLF vs LF specifically).
func detectLineEnding(s string) string {
	crlf := strings.Count(s, "\r\n")
	lf := strings.Count(s, "\n") - crlf
	if crlf > lf {
		return "\r\n"
	}
	return "\n"
}

// joinWithLineEnding is crlfAwareSplitLines's inverse: it terminates the result with a single
// trailing lineEnding (or returns "" for zero lines), letting callers thread detectLineEnding's
// result through so the file's original style round-trips instead of always emitting "\n" (see
// joinLines's note above for why this is a separate function rather than a change to joinLines
// itself).
func joinWithLineEnding(lines []string, lineEnding string) string {
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, lineEnding) + lineEnding
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
	lines := crlfAwareSplitLines(existing)
	begin, end := findMarkers(lines)
	lineEnding := detectLineEnding(existing)

	if begin != -1 && end != -1 {
		current := lines[begin : end+1]
		if equalLines(current, block) {
			return existing, false
		}
		newLines := make([]string, 0, len(lines)-len(current)+len(block))
		newLines = append(newLines, lines[:begin]...)
		newLines = append(newLines, block...)
		newLines = append(newLines, lines[end+1:]...)
		return joinWithLineEnding(newLines, lineEnding), true
	}

	newLines := make([]string, 0, len(lines)+len(block))
	newLines = append(newLines, lines...)
	newLines = append(newLines, block...)
	return joinWithLineEnding(newLines, lineEnding), true
}

// removeManagedBlock strips the first well-formed marker pair (and everything between, inclusive)
// from existing. changed is false when no such pair was found, so callers can treat it as a
// true no-op.
func removeManagedBlock(existing string) (result string, changed bool) {
	lines := crlfAwareSplitLines(existing)
	begin, end := findMarkers(lines)
	if begin == -1 || end == -1 {
		return existing, false
	}
	newLines := make([]string, 0, len(lines)-(end-begin+1))
	newLines = append(newLines, lines[:begin]...)
	newLines = append(newLines, lines[end+1:]...)
	return joinWithLineEnding(newLines, detectLineEnding(existing)), true
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

// writeFileEnsuringDir creates path's parent directory if missing, then writes content via
// atomicWriteFile (pathenv.go) rather than a direct os.WriteFile. This reuses the SAME
// symlink-resolving, temp-file+rename helper already relied on for shell rc files instead of a
// second parallel implementation: a plain os.WriteFile is non-atomic (a crash mid-write can leave
// a truncated CLAUDE.md) and, combined with a broken symlink, can behave surprisingly — see
// atomicWriteFile's own doc comment for the full symlink-safety contract.
func writeFileEnsuringDir(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return atomicWriteFile(path, []byte(content), 0o644)
}
