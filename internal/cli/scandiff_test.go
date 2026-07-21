package cli

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/installer"
)

// --- Phase 1: pure scanDiff parsing (no seam) ---------------------------------------------------
//
// Fixture note (dogfooding, task 5.1): the shape-valid "secret" strings below are deliberately
// synthetic — obvious placeholders, plus AWS's own official documentation example key — so they
// exercise the REAL guard.ScanWithError regexes without looking like a live credential to GitHub's
// push-protection scanner. This note deliberately does NOT repeat those literal values: prose can
// describe a fixture without quoting it, which is cheaper than suppressing a line that never needed
// to carry the value. The fixtures themselves still legitimately trigger internal/guard's patterns
// — that is the point — and the `click:allow-secret` comments on those specific lines (added when
// this change was dogfooded against its own diff) are the intended, correct resolution: never a
// weakening of the patterns, and never an exclusion of this test path.

func TestScanDiff_DetectsAddedLineSecret_TracksFileAndLine(t *testing.T) {
	diff := "diff --git a/config/settings.go b/config/settings.go\n" +
		"index 1111111..2222222 100644\n" +
		"--- a/config/settings.go\n" +
		"+++ b/config/settings.go\n" +
		"@@ -3,3 +3,4 @@ package config\n" +
		" const existing = \"keep\"\n" +
		"-const removed = \"gone\"\n" +
		"+const slackToken = \"xoxb-EXAMPLE-FAKE-TOKEN-FOR-TESTS-ONLY\" // click:allow-secret synthetic fixture, not a real token\n" +
		"+const another = \"safe value\"\n" +
		" const trailingContext = \"keep2\"\n"

	outcome := scanDiff(diff)

	if outcome.addedLines != 2 {
		t.Fatalf("addedLines = %d, want 2 (only + lines, never +++/--- headers or - lines)", outcome.addedLines)
	}
	if len(outcome.findings) != 1 {
		t.Fatalf("findings = %#v, want exactly 1 raw match (suppression is a separate pass)", outcome.findings)
	}
	f := outcome.findings[0]
	if f.file != "config/settings.go" {
		t.Fatalf("finding.file = %q, want %q", f.file, "config/settings.go")
	}
	if f.line != 4 {
		t.Fatalf("finding.line = %d, want 4 (hunk starts new-file at line 3, first added line is line 4)", f.line)
	}
	if f.decision.Category != "secrets" {
		t.Fatalf("finding.decision.Category = %q, want %q", f.decision.Category, "secrets")
	}
}

func TestScanDiff_NeverScansRemovedLines(t *testing.T) {
	diff := "diff --git a/x.go b/x.go\n" +
		"--- a/x.go\n" +
		"+++ b/x.go\n" +
		"@@ -1,1 +1,1 @@\n" +
		"-const key = \"AKIAIOSFODNN7EXAMPLE\" // click:allow-secret synthetic AWS docs placeholder, not a real key\n" +
		"+const key = \"safe replacement value\"\n"

	outcome := scanDiff(diff)

	if len(outcome.findings) != 0 {
		t.Fatalf("findings = %#v, want zero — the secret only appears on a removed (-) line, which must never be scanned (design decision 2)", outcome.findings)
	}
}

func TestScanDiff_DevNullFileHeader_NeverAttributesToDeletedFilePath(t *testing.T) {
	diff := "diff --git a/gone.go b/gone.go\n" +
		"deleted file mode 100644\n" +
		"--- a/gone.go\n" +
		"+++ /dev/null\n" +
		"@@ -1,1 +0,0 @@\n" +
		"-const key = \"AKIAIOSFODNN7EXAMPLE\" // click:allow-secret synthetic AWS docs placeholder, not a real key\n"

	outcome := scanDiff(diff)

	if len(outcome.findings) != 0 {
		t.Fatalf("findings = %#v, want zero (deleted file has no added lines)", outcome.findings)
	}
}

func TestScanDiff_NoPathExclusion_TestGoFileStillFlagged(t *testing.T) {
	diff := "diff --git a/internal/cli/fixture_test.go b/internal/cli/fixture_test.go\n" +
		"--- a/internal/cli/fixture_test.go\n" +
		"+++ b/internal/cli/fixture_test.go\n" +
		"@@ -1,1 +1,2 @@\n" +
		" package cli\n" +
		"+const tokenFixture = \"xoxb-EXAMPLE-FAKE-TOKEN-FOR-TESTS-ONLY\" // click:allow-secret synthetic fixture, not a real token\n"

	outcome := scanDiff(diff)

	if len(outcome.findings) != 1 {
		t.Fatalf("findings = %#v, want exactly 1 — a *_test.go path must be scanned exactly like any other path, no path-based exclusion", outcome.findings)
	}
	if outcome.findings[0].file != "internal/cli/fixture_test.go" {
		t.Fatalf("finding.file = %q, want the *_test.go path itself", outcome.findings[0].file)
	}
}

// --- Phase 1: suppression filtering (no seam) ----------------------------------------------------

func TestFilterSuppressed_SameLineReasonSuppresses(t *testing.T) {
	diff := "diff --git a/main.go b/main.go\n" +
		"--- a/main.go\n" +
		"+++ b/main.go\n" +
		"@@ -1,1 +1,2 @@\n" +
		" package main\n" +
		"+const key = \"AKIAIOSFODNN7EXAMPLE\" // click:allow-secret AWS docs example key, not a real credential\n"

	findings := scanDiff(diff).findings
	if len(findings) != 1 {
		t.Fatalf("raw findings = %#v, want exactly 1 before suppression", findings)
	}
	if got := filterSuppressed(findings); len(got) != 0 {
		t.Fatalf("filterSuppressed() = %#v, want empty — same-line marker with a reason must suppress", got)
	}
}

func TestFilterSuppressed_PreviousLineReasonSuppresses(t *testing.T) {
	diff := "diff --git a/main.go b/main.go\n" +
		"--- a/main.go\n" +
		"+++ b/main.go\n" +
		"@@ -1,1 +1,3 @@\n" +
		" package main\n" +
		"+// click:allow-secret AWS docs example key, not a real credential\n" +
		"+const key = \"AKIAIOSFODNN7EXAMPLE\"\n"

	findings := scanDiff(diff).findings
	if len(findings) != 1 {
		t.Fatalf("raw findings = %#v, want exactly 1 before suppression", findings)
	}
	if got := filterSuppressed(findings); len(got) != 0 {
		t.Fatalf("filterSuppressed() = %#v, want empty — a reasoned marker on the immediately preceding diff line must suppress", got)
	}
}

func TestFilterSuppressed_BareMarkerWithoutReason_DoesNotSuppress(t *testing.T) {
	diff := "diff --git a/main.go b/main.go\n" +
		"--- a/main.go\n" +
		"+++ b/main.go\n" +
		"@@ -1,1 +1,3 @@\n" +
		" package main\n" +
		"+// click:allow-secret\n" +
		// The marker on the simulated diff line above is INTENTIONALLY reason-less: that is exactly
		// what this test proves does NOT suppress. The marker below is a separate, genuine
		// suppression for THIS Go source line, and must sit on the immediately preceding line to
		// take effect (dogfooded per task 5.1).
		// click:allow-secret synthetic AWS documentation placeholder fixture, not a real key
		"+const key = \"AKIAIOSFODNN7EXAMPLE\"\n"

	findings := scanDiff(diff).findings
	if len(findings) != 1 {
		t.Fatalf("raw findings = %#v, want exactly 1 before suppression", findings)
	}
	got := filterSuppressed(findings)
	if len(got) != 1 {
		t.Fatalf("filterSuppressed() = %#v, want the finding still reported — a bare marker with no reason text must NOT suppress", got)
	}
}

// --- Phase 2: git diff acquisition seam ------------------------------------------------------

func TestResolveDiffTarget_UpstreamConfigured_ReturnsUpstream(t *testing.T) {
	got := resolveDiffTarget("origin/feature-x\n", nil)
	if got != "origin/feature-x" {
		t.Fatalf("resolveDiffTarget() = %q, want %q (trimmed upstream)", got, "origin/feature-x")
	}
}

func TestResolveDiffTarget_NoUpstreamConfigured_FallsBackToMain(t *testing.T) {
	got := resolveDiffTarget("", errors.New("fatal: no upstream configured for branch"))
	if got != "main" {
		t.Fatalf("resolveDiffTarget() = %q, want %q (fallback on error)", got, "main")
	}
}

func TestResolveDiffTarget_EmptyUpstreamOutputWithNilErr_FallsBackToMain(t *testing.T) {
	got := resolveDiffTarget("   \n", nil)
	if got != "main" {
		t.Fatalf("resolveDiffTarget() = %q, want %q (fallback on blank output even without an error)", got, "main")
	}
}

// TestRunGitDiff_GitMissing_ReturnsExitCode2 is the hermetic git-missing test: it fakes
// installer's BinaryLookup (the SAME seam GitAvailable/GitPath already use) so this never depends
// on whether the real test machine has git on PATH — the exact CI-PATH bug class that broke the
// prior PR (task 5.2). It calls runGitDiff directly (not through the gitDiffFunc var) because this
// is the one branch of the real implementation that IS hermetically testable without a real git
// repository: GitAvailable's own lookup is what's faked, so runGitDiff never gets far enough to
// invoke an actual git subprocess.
func TestRunGitDiff_GitMissing_ReturnsExitCode2(t *testing.T) {
	restore := installer.SetBinaryLookupFactoryForTests(func() installer.BinaryLookup {
		return cliFakeBinaryLookup{resolved: map[string]string{}}
	})
	defer restore()

	_, err := runGitDiff()
	if err == nil {
		t.Fatal("runGitDiff() error = nil when git is missing from PATH, want a non-nil error")
	}
	var exitErr interface{ ExitCode() int }
	if !errors.As(err, &exitErr) {
		t.Fatalf("runGitDiff() error = %T %v, want an exit-coded error", err, err)
	}
	if exitErr.ExitCode() != 2 {
		t.Fatalf("ExitCode() = %d, want 2", exitErr.ExitCode())
	}
	if !strings.Contains(err.Error(), "git") {
		t.Fatalf("runGitDiff() error = %v, want it to mention git", err)
	}
}

// TestSetGitDiffFuncForTests_RestoresOriginalOnCleanup proves the seam itself (design decision 6)
// correctly overrides and restores gitDiffFunc, using function-pointer identity comparison so this
// test never has to actually CALL the restored (real, git-invoking) function — keeping it fully
// hermetic. This mirrors uninstall.go's SetRemoveEngramPluginFuncForTests test pattern.
func TestSetGitDiffFuncForTests_RestoresOriginalOnCleanup(t *testing.T) {
	before := reflect.ValueOf(gitDiffFunc).Pointer()

	restore := SetGitDiffFuncForTests(func() (string, error) { return "fake diff text", nil })
	if got := reflect.ValueOf(gitDiffFunc).Pointer(); got == before {
		t.Fatal("gitDiffFunc pointer unchanged after SetGitDiffFuncForTests, want the override to take effect")
	}
	gotText, err := gitDiffFunc()
	if err != nil || gotText != "fake diff text" {
		t.Fatalf("gitDiffFunc() = (%q, %v), want (%q, nil)", gotText, err, "fake diff text")
	}

	restore()
	if got := reflect.ValueOf(gitDiffFunc).Pointer(); got != before {
		t.Fatalf("gitDiffFunc pointer after restore = %v, want original %v", got, before)
	}
}

// --- Phase 3: command wiring, exit codes, Spanish output --------------------------------------

func TestScanDiffCommand_CleanDiff_ExitsZeroWithSpanishConfirmation(t *testing.T) {
	diff := "diff --git a/x.go b/x.go\n" +
		"--- a/x.go\n" +
		"+++ b/x.go\n" +
		"@@ -1,1 +1,2 @@\n" +
		" package x\n" +
		"+const safe = \"value\"\n"
	restore := SetGitDiffFuncForTests(func() (string, error) { return diff, nil })
	defer restore()

	out, err := execRoot(t, t.TempDir(), "scan-diff")
	if err != nil {
		t.Fatalf("scan-diff error = %v, output:\n%s", err, out)
	}
	if !strings.Contains(out, "sin hallazgos bloqueantes") {
		t.Fatalf("output = %q, want a clean-run Spanish confirmation", out)
	}
	if !strings.Contains(out, "1 líneas agregadas") {
		t.Fatalf("output = %q, want it to report the count of added lines reviewed", out)
	}
}

func TestScanDiffCommand_UnsuppressedFinding_ExitsOneWithFileLineCategory(t *testing.T) {
	diff := "diff --git a/config/settings.go b/config/settings.go\n" +
		"--- a/config/settings.go\n" +
		"+++ b/config/settings.go\n" +
		"@@ -1,1 +1,2 @@\n" +
		" package config\n" +
		// Inside the simulated diff this value stays unsuppressed on purpose, so the test can prove
		// scan-diff's exit-1 path. The marker below suppresses only THIS Go source line, and must
		// sit immediately above it to take effect (dogfooded per task 5.1).
		// click:allow-secret synthetic fixture value, not a real token
		"+const slackToken = \"xoxb-EXAMPLE-FAKE-TOKEN-FOR-TESTS-ONLY\"\n"
	restore := SetGitDiffFuncForTests(func() (string, error) { return diff, nil })
	defer restore()

	out, err := execRoot(t, t.TempDir(), "scan-diff")
	if err == nil {
		t.Fatalf("scan-diff error = nil, want non-nil for an unsuppressed finding, output:\n%s", out)
	}
	if !errors.Is(err, errScanDiffBlocked) {
		t.Fatalf("scan-diff error = %v, want errScanDiffBlocked", err)
	}
	var exitErr interface{ ExitCode() int }
	if errors.As(err, &exitErr) {
		t.Fatalf("scan-diff error = %v implements ExitCode(), want the plain errScanDiffBlocked sentinel (exit 1 via main.go's default os.Exit(1) path, distinct from exit 2 tooling failures)", err)
	}
	if !strings.Contains(out, "config/settings.go:2") {
		t.Fatalf("output = %q, want it to name file:line", out)
	}
	if !strings.Contains(out, "[secrets]") {
		t.Fatalf("output = %q, want it to name the category", out)
	}
	if !strings.Contains(out, "click:allow-secret") {
		t.Fatalf("output = %q, want it to mention the suppression convention", out)
	}
	if !strings.Contains(out, "hallazgo(s) bloqueante(s)") {
		t.Fatalf("output = %q, want the blocked summary line", out)
	}
}

func TestScanDiffCommand_SuppressedFinding_ExitsZero(t *testing.T) {
	diff := "diff --git a/config/settings.go b/config/settings.go\n" +
		"--- a/config/settings.go\n" +
		"+++ b/config/settings.go\n" +
		"@@ -1,1 +1,2 @@\n" +
		" package config\n" +
		"+const slackToken = \"xoxb-EXAMPLE-FAKE-TOKEN-FOR-TESTS-ONLY\" // click:allow-secret synthetic fixture, not a real token\n"
	restore := SetGitDiffFuncForTests(func() (string, error) { return diff, nil })
	defer restore()

	out, err := execRoot(t, t.TempDir(), "scan-diff")
	if err != nil {
		t.Fatalf("scan-diff error = %v, want nil — the only finding carries a valid suppression, output:\n%s", err, out)
	}
	if !strings.Contains(out, "sin hallazgos bloqueantes") {
		t.Fatalf("output = %q, want a clean-run Spanish confirmation", out)
	}
}

func TestScanDiffCommand_GitAcquisitionFailure_ExitsTwo(t *testing.T) {
	restore := SetGitDiffFuncForTests(func() (string, error) {
		return "", &exitCodeError{code: 2, msg: "scan-diff: git no está instalado o no está en el PATH."}
	})
	defer restore()

	out, err := execRoot(t, t.TempDir(), "scan-diff")
	var exitErr interface{ ExitCode() int }
	if !errors.As(err, &exitErr) {
		t.Fatalf("scan-diff error = %T %v, want an exit-coded error, output:\n%s", err, err, out)
	}
	if exitErr.ExitCode() != 2 {
		t.Fatalf("ExitCode() = %d, want 2", exitErr.ExitCode())
	}
}

func TestRootCommand_Help_ListsScanDiffAsVisibleCommand(t *testing.T) {
	out, err := execRoot(t, t.TempDir(), "--help")
	if err != nil {
		t.Fatalf("--help error = %v", err)
	}
	if !strings.Contains(out, "scan-diff") {
		t.Fatalf("--help output = %q, want it to list scan-diff as a visible command", out)
	}
}
