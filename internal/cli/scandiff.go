// scandiff.go implements `click scan-diff`, a manual pre-push validation gate: it scans the
// outgoing diff's ADDED lines for secrets/credentials using the existing, unmodified
// internal/guard engine (guard.ScanWithError — the same engine memoryguard.go's PreToolUse hook
// uses). It never touches internal/guard/patterns.yaml, and it never scans removed (`-`) lines — a
// removed secret is already committed to history, catching it here is deliberately out of scope
// (design decision 2).
package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/guard"
	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/installer"
	"github.com/spf13/cobra"
)

// errScanDiffBlocked is scan-diff's policy-finding sentinel (mirrors doctor.go's errUnhealthy):
// main.go's default os.Exit(1) fallback (for any error that does NOT implement ExitCode()) gives
// an unsuppressed finding exit code 1 — distinct from exitCodeError{2}'s tooling-failure exit code.
var errScanDiffBlocked = errors.New("scan-diff: hallazgos bloqueantes sin suprimir")

// gitCommandTimeout bounds every git subprocess runGit spawns, mirroring
// internal/installer/plugins.go's commandOutputTimeout (30s, tuned for quick read-only queries:
// rev-parse and diff). Package var (not const), matching that same tunable-timeout convention.
var gitCommandTimeout = 30 * time.Second

// gitDiffFunc is scan-diff's sole test seam (design decision 6), mirroring uninstall.go's
// removeEngramPluginFunc factory-injection pattern: SetGitDiffFuncForTests lets a test fully
// replace diff acquisition with a literal diff string or a simulated failure, so no CLI-level test
// in this package needs a real git repository or a real git binary on PATH.
var gitDiffFunc = runGitDiff

// SetGitDiffFuncForTests overrides gitDiffFunc for tests and returns a restore function.
func SetGitDiffFuncForTests(fn func() (string, error)) func() {
	old := gitDiffFunc
	gitDiffFunc = fn
	return func() { gitDiffFunc = old }
}

func newScanDiffCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "scan-diff",
		Short: "Escanea el diff saliente en busca de secretos antes de hacer push",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runScanDiff(cmd)
		},
	}
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	return cmd
}

func runScanDiff(cmd *cobra.Command) error {
	out := cmd.OutOrStdout()
	r := rendererFor(cmd, out)

	diffText, err := gitDiffFunc()
	if err != nil {
		return err
	}

	outcome := scanDiff(diffText)
	unsuppressed := filterSuppressed(outcome.findings)

	if len(unsuppressed) == 0 {
		fmt.Fprintln(out, r.Success(fmt.Sprintf(
			"scan-diff: sin hallazgos bloqueantes (%d líneas agregadas revisadas).",
			outcome.addedLines,
		)))
		return nil
	}

	for _, f := range unsuppressed {
		fmt.Fprintln(out, r.Fail(fmt.Sprintf(
			"%s:%d [%s] %s (agregue «click:allow-secret <motivo>» en esta línea o la anterior si es un falso positivo)",
			f.file, f.line, f.decision.Category, f.decision.Reason,
		)))
	}
	fmt.Fprintln(out, r.Warn(fmt.Sprintf(
		"scan-diff: %d hallazgo(s) bloqueante(s). Revíselos antes de hacer push.",
		len(unsuppressed),
	)))
	return errScanDiffBlocked
}

// diffFinding is one guard.Scan match against an ADDED diff line, carrying enough context
// (lineText, prevLineText) for filterSuppressed to check without re-parsing the diff.
type diffFinding struct {
	file         string
	line         int
	decision     guard.Decision
	lineText     string
	prevLineText string
}

// scanOutcome is scanDiff's return value: every raw match found (suppression NOT yet applied — see
// filterSuppressed) plus the total count of added lines actually scanned, used for the clean-run
// summary line.
type scanOutcome struct {
	findings   []diffFinding
	addedLines int
}

const (
	newFileHeaderPrefix = "+++ "
	oldFileHeaderPrefix = "--- "
)

var hunkHeaderPattern = regexp.MustCompile(`^@@ -\d+(?:,\d+)? \+(\d+)(?:,\d+)? @@`)

// scanDiff parses diffText as unified diff text and runs every ADDED line (`+`, excluding the
// `+++`/`---` file headers) through the existing, unmodified guard.ScanWithError — never `-`
// (removed) lines. It applies ZERO path-based branching: every file and every added line is
// scanned uniformly, including *_test.go paths (spec's "Suppression Is Not Path-Based"
// requirement) — suppression itself is a SEPARATE step, see filterSuppressed.
func scanDiff(diffText string) scanOutcome {
	var outcome scanOutcome
	var currentFile string
	var lineNo int
	var prevLine string

	for _, rawLine := range strings.Split(diffText, "\n") {
		switch {
		case strings.HasPrefix(rawLine, newFileHeaderPrefix):
			path := strings.TrimPrefix(rawLine, newFileHeaderPrefix)
			if path == "/dev/null" {
				currentFile = ""
			} else {
				currentFile = strings.TrimPrefix(path, "b/")
			}
			prevLine = ""
		case strings.HasPrefix(rawLine, oldFileHeaderPrefix):
			// old-file marker: never the file this command reports findings against.
		case strings.HasPrefix(rawLine, "@@"):
			if start, ok := parseHunkStart(rawLine); ok {
				lineNo = start
			}
			prevLine = ""
		case strings.HasPrefix(rawLine, "+"):
			content := strings.TrimPrefix(rawLine, "+")
			outcome.addedLines++
			if decision, scanErr := guard.ScanWithError(content); scanErr == nil && decision.Blocked {
				outcome.findings = append(outcome.findings, diffFinding{
					file:         currentFile,
					line:         lineNo,
					decision:     decision,
					lineText:     content,
					prevLineText: prevLine,
				})
			}
			prevLine = content
			lineNo++
		case strings.HasPrefix(rawLine, "-"):
			// removed line: never scanned, never advances the new-file line counter.
		case strings.HasPrefix(rawLine, " "):
			prevLine = strings.TrimPrefix(rawLine, " ")
			lineNo++
		default:
			// diff metadata (`diff --git ...`, `index ...`, `new file mode ...`, "\ No newline...")
			// — irrelevant to file/line tracking.
		}
	}
	return outcome
}

func parseHunkStart(line string) (int, bool) {
	m := hunkHeaderPattern.FindStringSubmatch(line)
	if m == nil {
		return 0, false
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, false
	}
	return n, true
}

// suppressionPattern matches click's inline suppression convention (design decision 4): the
// literal substring "click:allow-secret" followed by a REQUIRED non-empty reason. Matched
// language-agnostically (no per-language comment-marker parsing) — deliberately: a diff spans
// arbitrary languages, and requiring a marker wouldn't verify the text sits inside a real comment
// either.
var suppressionPattern = regexp.MustCompile(`click:allow-secret\s*:?\s*(\S.*)`)

func hasSuppressionReason(line string) bool {
	m := suppressionPattern.FindStringSubmatch(line)
	return m != nil && strings.TrimSpace(m[1]) != ""
}

// filterSuppressed drops every finding whose flagged line OR immediately preceding diff line
// carries a valid (non-empty-reason) click:allow-secret marker (spec: "Inline Suppression
// Convention"). Kept as its own pass over scanDiff's raw findings — rather than inline in the
// scanning loop — so "what did guard.Scan match" and "was it suppressed" stay independently
// testable (matches the tasks.md Phase 1 RED/GREEN split).
func filterSuppressed(findings []diffFinding) []diffFinding {
	var kept []diffFinding
	for _, f := range findings {
		if hasSuppressionReason(f.lineText) || hasSuppressionReason(f.prevLineText) {
			continue
		}
		kept = append(kept, f)
	}
	return kept
}

// resolveDiffTarget is the pure branching core of the "diff range resolution" requirement: given
// the raw result of resolving the current branch's upstream
// (`git rev-parse --abbrev-ref --symbolic-full-name @{u}`), it picks the ref scan-diff diffs HEAD
// against. No seam needed — both the "upstream configured" and "no upstream configured" scenarios
// are directly unit-testable without invoking git at all.
func resolveDiffTarget(upstreamOutput string, upstreamErr error) string {
	trimmed := strings.TrimSpace(upstreamOutput)
	if upstreamErr != nil || trimmed == "" {
		return "main"
	}
	return trimmed
}

// runGit is scan-diff's minimal git subprocess runner (design decision 1): deliberately NOT
// installer.CommandRunner.Output, which returns COMBINED stdout+stderr — diff text must be parsed
// byte-exact, and stderr noise mixed into stdout would corrupt scanDiff's parsing. stdout and
// stderr are captured separately instead.
func runGit(args ...string) (stdout string, stderr string, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), gitCommandTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", args...)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	runErr := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		return "", "", fmt.Errorf("scan-diff: «git %s» no respondió dentro de %s", strings.Join(args, " "), gitCommandTimeout)
	}
	return outBuf.String(), errBuf.String(), runErr
}

// runGitDiff is gitDiffFunc's real (non-test) implementation: git rev-parse --is-inside-work-tree
// → git rev-parse --abbrev-ref --symbolic-full-name @{u} (fallback "main") → git diff --no-color
// <target>...HEAD (design decision 1). Every failure classified here as a TOOLING failure returns
// exitCodeError{2} directly (mirrors memoryguard.go's failClosed / doctor's exitCodeError usage),
// so runScanDiff's RunE can just propagate whatever gitDiffFunc returns unchanged.
func runGitDiff() (string, error) {
	if !installer.GitAvailable() {
		return "", &exitCodeError{code: 2, msg: "scan-diff: git no está instalado o no está en el PATH. Instálelo y vuelva a intentar."}
	}

	if _, stderr, err := runGit("rev-parse", "--is-inside-work-tree"); err != nil {
		return "", &exitCodeError{code: 2, msg: fmt.Sprintf(
			"scan-diff: el directorio actual no es un repositorio git (%s)", strings.TrimSpace(stderr),
		)}
	}

	upstreamOut, _, upstreamErr := runGit("rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}")
	target := resolveDiffTarget(upstreamOut, upstreamErr)

	diffOut, diffStderr, diffErr := runGit("diff", "--no-color", target+"...HEAD")
	if diffErr != nil {
		return "", &exitCodeError{code: 2, msg: fmt.Sprintf(
			"scan-diff: no se pudo obtener el diff contra %q (%s)", target, strings.TrimSpace(diffStderr),
		)}
	}
	return diffOut, nil
}
