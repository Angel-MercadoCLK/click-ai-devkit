//go:build !windows

// PR3 of the engram-mcp-resolution chain: the POSIX (macOS/Linux) pathStore implementation. It
// persists the Go bin dir onto the user's *persisted* PATH by editing shell rc files, mirroring
// pathenv_windows.go's registry-based approach but for shell-rc-based shells.
//
// Design (sdd/engram-mcp-resolution/design, D-3/D-4/D-7):
//   - zsh    -> ~/.zshrc only
//   - bash   -> the login-chain file (first existing of ~/.bash_profile, ~/.bash_login,
//     ~/.profile; ~/.bash_profile if none exist) AND ~/.bashrc, so both login and
//     non-login bash sessions pick up the change.
//   - fish   -> skipped entirely (no mutation); remediation messaging is a CLI-layer concern
//     (pathWarning, wired in PR4), not this package's.
//   - other  -> ~/.profile
//
// Idempotency is content-based, not marker-based: a pre-existing manual `export PATH=...` (or
// bare `PATH=...`) line that already yields dir once $HOME/$GOPATH/$GOBIN are expanded counts as
// "already present" — per target file — so click never appends a redundant managed block next to
// a developer's own working PATH line. Comparison is case-sensitive (unlike the Windows
// implementation in pathenv.go/pathenv_windows.go): POSIX paths are case-sensitive on the
// filesystems these shells commonly run on (ext4, most Linux; case-sensitive-configurable APFS /
// case-sensitive HFS+ on macOS), so treating them case-sensitively is the safe default.
//
// Mutation reuses PR1's atomicWriteFile (temp-in-same-dir + fsync + rename) for every rc write —
// never claudemd.go's WriteManagedBlock, which truncates+rewrites non-atomically and was
// explicitly scoped to the versioned, replaceable CLAUDE.md project file, not real user rc files
// (design review, D-7). The pure line-splice helpers below are this file's own (posix-prefixed,
// PATH-specific markers distinct from CLAUDE.md's) rather than claudemd.go's marker-specific
// buildManagedBlock/spliceManagedBlock/findMarkers, which are hardcoded to the CLAUDE.md markers
// via package-level consts; this file does reuse claudemd.go's three fully generic (marker-free)
// helpers — readFileOrEmpty, splitLines, joinLines, equalLines — since those already have no
// CLAUDE.md-specific behavior baked in.
package installer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// posixManagedBeginMarker / posixManagedEndMarker delimit click's managed PATH block in a shell rc
// file. Deliberately distinct from claudemd.go's managedBeginMarker/managedEndMarker (design
// D-7) — they mark unrelated content in unrelated files.
const (
	posixManagedBeginMarker = "# >>> click-ai-devkit go bin (managed) >>>"
	posixManagedEndMarker   = "# <<< click-ai-devkit go bin (managed) <<<"
)

// init wires the default pathStoreFactory for every non-Windows build, mirroring
// pathenv_windows.go's windows-tagged init(). The two are mutually exclusive by build tag, so
// exactly one of them ever compiles into a given binary — there is no runtime conflict between
// them.
func init() {
	pathStoreFactory = func() pathStore { return osPathStore{} }
}

// osPathStore is the POSIX pathStore implementation: shell-rc-file-backed, targeting the files
// posixShellTargets resolves for the user's detected shell.
type osPathStore struct{}

// PersistedPathContains reports whether dir is already present in the persisted user PATH: true
// only if EVERY one of the current shell's target rc files already contains a PATH-setting line
// that yields dir once variables are expanded (click's own managed block counts too, since it's a
// normal `export PATH=...` line like any other). This mirrors what a fully successful EnsureOnPath
// run guarantees — bash's login-chain file and .bashrc are read by different session types, so
// both must independently end up correct before dir can be considered genuinely persisted.
// Requiring only ONE target to match would silently mask a partial write (e.g. a prior EnsureOnPath
// run that wrote the login-chain file but crashed/failed before reaching .bashrc). If there are no
// applicable target files (fish shell — posixShellTargets returns nil), there is nothing to check,
// so this reports false, not vacuously true.
func (osPathStore) PersistedPathContains(dir string) (bool, error) {
	targets, err := posixShellTargets()
	if err != nil {
		return false, err
	}
	if len(targets) == 0 {
		return false, nil
	}
	for _, path := range targets {
		content, err := readFileOrEmpty(path)
		if err != nil {
			return false, fmt.Errorf("installer: read %s: %w", path, err)
		}
		if !rcContainsDir(content, dir) {
			return false, nil
		}
	}
	return true, nil
}

// EnsureOnPath adds dir to every applicable target rc file that doesn't already provide it
// (per-file content check, so a file with a pre-existing manual PATH line is left untouched while
// a sibling target file without one still gets click's managed block — bash's login-chain file and
// .bashrc are read in different session types, so both need to end up correct independently).
// changed reports whether ANY target file was actually mutated.
func (osPathStore) EnsureOnPath(dir string) (bool, error) {
	targets, err := posixShellTargets()
	if err != nil {
		return false, err
	}
	changedAny := false
	for _, path := range targets {
		content, err := readFileOrEmpty(path)
		if err != nil {
			return changedAny, fmt.Errorf("installer: read %s: %w", path, err)
		}
		if rcContainsDir(content, dir) {
			continue
		}
		changed, err := writeManagedBlockAtomicUnix(path, content, dir)
		if err != nil {
			return changedAny, err
		}
		if changed {
			changedAny = true
		}
	}
	return changedAny, nil
}

// posixShellTargets resolves the rc file(s) EnsureOnPath/PersistedPathContains must target for the
// current user's detected shell ($SHELL, per design D-3). Returns an empty (nil) slice — not an
// error — for fish, since fish is intentionally skipped (CLI-layer remediation messaging handles
// it, not a pathStore mutation).
func posixShellTargets() ([]string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("installer: resolve home dir: %w", err)
	}
	switch posixShellKind() {
	case "zsh":
		return []string{filepath.Join(home, ".zshrc")}, nil
	case "bash":
		login := bashLoginFile(home)
		bashrc := filepath.Join(home, ".bashrc")
		if login == bashrc {
			return []string{bashrc}, nil
		}
		return []string{login, bashrc}, nil
	case "fish":
		return nil, nil
	default:
		return []string{filepath.Join(home, ".profile")}, nil
	}
}

// posixShellKind classifies $SHELL into "zsh", "bash", "fish", or "other" by the basename of the
// shell path (e.g. "/bin/bash" -> "bash", "/usr/local/bin/zsh" -> "zsh").
func posixShellKind() string {
	base := filepath.Base(os.Getenv("SHELL"))
	switch {
	case strings.Contains(base, "zsh"):
		return "zsh"
	case strings.Contains(base, "bash"):
		return "bash"
	case strings.Contains(base, "fish"):
		return "fish"
	default:
		return "other"
	}
}

// bashLoginFile resolves bash's login-shell rc target per its own documented lookup order: the
// first EXISTING file among ~/.bash_profile, ~/.bash_login, ~/.profile. If none of the three exist
// yet (fresh install), it defaults to ~/.bash_profile — bash's primary/first-choice file — so a
// fresh install gets a canonical, predictable target rather than silently doing nothing.
func bashLoginFile(home string) string {
	candidates := []string{
		filepath.Join(home, ".bash_profile"),
		filepath.Join(home, ".bash_login"),
		filepath.Join(home, ".profile"),
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return candidates[0]
}

// rcContainsDir reports whether content (an rc file's current text) has an uncommented PATH
// assignment line (`PATH=...` or `export PATH=...`) that, once $HOME/$GOPATH/$GOBIN are expanded,
// yields dir as one of its colon-separated entries.
func rcContainsDir(content, dir string) bool {
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		value, ok := extractPathAssignmentValue(trimmed)
		if !ok {
			continue
		}
		if posixPathValueContains(expandPosixPathVars(value), dir) {
			return true
		}
	}
	return false
}

// extractPathAssignmentValue extracts the right-hand side of a `PATH=...` or `export PATH=...`
// line (surrounding quotes stripped), or reports ok=false if line isn't such an assignment.
func extractPathAssignmentValue(line string) (value string, ok bool) {
	line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
	if !strings.HasPrefix(line, "PATH=") {
		return "", false
	}
	value = strings.TrimPrefix(line, "PATH=")
	value = strings.Trim(value, `"'`)
	return value, true
}

// expandPosixPathVars expands $HOME/$GOPATH/$GOBIN (both bare and ${...} braced forms) in value
// using the current process environment — the same variables a real shell would substitute when
// it sources the rc file. Any other variable reference (including $PATH, which self-referential
// PATH lines commonly contain) is left untouched; it never contains dir, so it never causes a
// false match.
func expandPosixPathVars(value string) string {
	replacer := strings.NewReplacer(
		"${HOME}", os.Getenv("HOME"),
		"$HOME", os.Getenv("HOME"),
		"${GOPATH}", os.Getenv("GOPATH"),
		"$GOPATH", os.Getenv("GOPATH"),
		"${GOBIN}", os.Getenv("GOBIN"),
		"$GOBIN", os.Getenv("GOBIN"),
	)
	return replacer.Replace(value)
}

// posixPathValueContains reports whether dir is among value's ":"-separated entries.
// Case-sensitive and trailing-"/"-normalized (POSIX PATH entries are case-sensitive, unlike
// Windows's pathListContains in pathenv.go).
func posixPathValueContains(value, dir string) bool {
	target := normalizePosixPathEntry(dir)
	for _, entry := range strings.Split(value, ":") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if normalizePosixPathEntry(entry) == target {
			return true
		}
	}
	return false
}

// normalizePosixPathEntry trims surrounding whitespace (already done by callers) and a single
// trailing "/" so "/x/go/bin" and "/x/go/bin/" compare equal. No case-folding — POSIX paths are
// case-sensitive.
func normalizePosixPathEntry(entry string) string {
	return strings.TrimSuffix(entry, "/")
}

// buildPosixManagedBlock builds the 3-line managed block click writes/replaces in an rc file:
// begin marker, a single `export PATH="$PATH:dir"` line, end marker.
func buildPosixManagedBlock(dir string) []string {
	return []string{
		posixManagedBeginMarker,
		fmt.Sprintf(`export PATH="$PATH:%s"`, dir),
		posixManagedEndMarker,
	}
}

// findPosixMarkers locates the first well-formed posix managed-block marker pair in lines,
// mirroring claudemd.go's findMarkers but against this file's own PATH-specific markers.
func findPosixMarkers(lines []string) (begin, end int) {
	begin, end = -1, -1
	for i, l := range lines {
		if l == posixManagedBeginMarker && begin == -1 {
			begin = i
		}
		if l == posixManagedEndMarker && begin != -1 && i > begin {
			end = i
			break
		}
	}
	if end == -1 {
		begin = -1
	}
	return begin, end
}

// splicePosixManagedBlock inserts block into existing (replacing an existing posix managed block
// in place if one is found — e.g. GoBinDir() changed since the last run — appending otherwise).
// changed is false when the result would be byte-identical to existing.
func splicePosixManagedBlock(existing string, block []string) (result string, changed bool) {
	lines := splitLines(existing)
	begin, end := findPosixMarkers(lines)

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

// writeManagedBlockAtomicUnix splices click's managed PATH block (for dir) into existingContent
// and, if that changes anything, atomically writes the result to path via PR1's atomicWriteFile —
// preserving path's existing file mode if it already exists, defaulting to 0o644 for a brand-new
// file. Returns changed=false, nil error with no write at all when the splice is a no-op (existing
// block already matches).
func writeManagedBlockAtomicUnix(path, existingContent, dir string) (bool, error) {
	block := buildPosixManagedBlock(dir)
	updated, changed := splicePosixManagedBlock(existingContent, block)
	if !changed {
		return false, nil
	}

	mode := os.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode()
	}

	if err := atomicWriteFile(path, []byte(updated), mode); err != nil {
		return false, fmt.Errorf("installer: atomic write %s: %w", path, err)
	}
	return true, nil
}
