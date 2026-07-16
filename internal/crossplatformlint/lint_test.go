// Package crossplatformlint hosts a repo-wide guardrail test (T3-1) against the cross-platform
// test-fixture bug class that recurred THREE times during the hardening work: a Windows drive-letter
// path used inside filepath.Join in a test that also runs on Linux/macOS CI.
//
// The specific, high-confidence anti-pattern this guards is `filepath.Join("<letter>:", ...)`. On
// Windows that yields `C:\...` (fine), but on POSIX it yields `C:/...` — a string containing a `:`
// that filepath.SplitList then fragments on the POSIX list separator, silently corrupting PATH
// fixtures (this is exactly what broke TestLivePathContains on ubuntu-latest). It is never correct in
// a cross-platform test fixture. Legitimate Windows-path *string literals* used as inputs (e.g.
// `gopath := "C:\\Users\\dev\\go"` in the GoBinDir tests, where the expected value is derived via
// filepath.Join rather than hardcoded) are deliberately NOT flagged — only the drive-letter-inside-
// filepath.Join construct is, keeping this guardrail low-false-positive.
package crossplatformlint

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// driveLetterJoin matches filepath.Join with a drive-letter string literal as its first argument,
// e.g. filepath.Join("C:", ... — the exact SplitList-colliding construct.
var driveLetterJoin = regexp.MustCompile(`filepath\.Join\(\s*"[A-Za-z]:`)

// windowsBuildTag matches a windows-only build constraint on either the modern //go:build line or the
// legacy // +build line; such files never compile on POSIX CI, so a drive letter in them is fine.
var windowsBuildTag = regexp.MustCompile(`(?m)^//go:build.*\bwindows\b|^// \+build.*\bwindows\b`)

func moduleRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed; cannot locate the module root")
	}
	dir := filepath.Dir(thisFile)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("walked to the filesystem root without finding go.mod")
		}
		dir = parent
	}
}

// TestNoDriveLetterFilepathJoinInCrossPlatformTests walks every *_test.go in the module and fails if
// any non-windows-tagged file uses filepath.Join with a drive-letter literal — preventing a fourth
// recurrence of the cross-platform fixture bug.
func TestNoDriveLetterFilepathJoinInCrossPlatformTests(t *testing.T) {
	root := moduleRoot(t)
	var offenders []string

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			base := info.Name()
			if base == ".git" || base == "vendor" || base == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, "_test.go") {
			return nil
		}
		// Don't lint this guardrail file itself — it necessarily contains the anti-pattern in prose.
		if strings.HasSuffix(path, "crossplatformlint"+string(filepath.Separator)+"lint_test.go") {
			return nil
		}
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if windowsBuildTag.Match(content) {
			return nil // windows-only file: a drive letter here never reaches POSIX CI
		}
		if loc := driveLetterJoin.FindIndex(content); loc != nil {
			line := 1 + strings.Count(string(content[:loc[0]]), "\n")
			rel, relErr := filepath.Rel(root, path)
			if relErr != nil {
				rel = path
			}
			offenders = append(offenders, rel+":"+itoa(line))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking the module for test files failed: %v", err)
	}

	if len(offenders) > 0 {
		t.Fatalf("cross-platform test fixtures use filepath.Join with a drive-letter literal (breaks on "+
			"POSIX CI via filepath.SplitList — use filepath.Join(string(filepath.Separator), ...) or a "+
			"non-Join fixture instead): %s", strings.Join(offenders, ", "))
	}
}

// TestDriveLetterDetector_Controls is the detector's own unit test: it must catch the anti-pattern
// and must NOT flag the accepted alternatives, so the guardrail can't silently rot into a no-op or a
// false-positive generator.
func TestDriveLetterDetector_Controls(t *testing.T) {
	shouldMatch := []string{
		`dir := filepath.Join("C:", "Users", "dev")`,
		`filepath.Join("D:", "data")`,
		`x := filepath.Join(  "c:", "temp")`,
	}
	for _, s := range shouldMatch {
		if !driveLetterJoin.MatchString(s) {
			t.Errorf("detector missed the anti-pattern: %q", s)
		}
	}
	shouldNotMatch := []string{
		`dir := filepath.Join(string(filepath.Separator), "home", "dev")`, // the fix
		`gopath := "C:\\Users\\dev\\go"`,                                  // legitimate Windows string-literal fixture
		`want := filepath.Join(gopath, "bin")`,                            // derive expected via Join, no drive literal
		`t.Setenv("PATH", other)`,
	}
	for _, s := range shouldNotMatch {
		if driveLetterJoin.MatchString(s) {
			t.Errorf("detector false-positived on accepted code: %q", s)
		}
	}
}

// itoa avoids pulling strconv into a test that otherwise needs nothing else from it.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
