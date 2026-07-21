package installer

import (
	"os"
	"strings"
	"testing"
	"time"
)

// TestExecCommandRunner_Output_SucceedsWithinTimeout is the happy path for T3-2: a fast, real query
// (`go version`, always available when `go test` runs) completes well within commandOutputTimeout and
// returns its output unchanged — the timeout wiring must not disturb normal Output behavior.
func TestExecCommandRunner_Output_SucceedsWithinTimeout(t *testing.T) {
	out, err := execCommandRunner{}.Output("go", "version")
	if err != nil {
		t.Fatalf("Output(go version) error = %v", err)
	}
	if !strings.Contains(string(out), "go") {
		t.Fatalf("Output(go version) = %q, want it to contain 'go'", out)
	}
}

// TestExecCommandRunner_Output_TimesOut proves the T3-2 bound actually fires: with the timeout shrunk
// to 1ns the context is already expired, so Output must return a deadline error rather than hang or
// return raw command output. This is the guarantee doctor's read-only checkEngramPath relies on
// (NFR-012: doctor never hangs).
func TestExecCommandRunner_Output_TimesOut(t *testing.T) {
	orig := commandOutputTimeout
	commandOutputTimeout = 1 * time.Nanosecond
	defer func() { commandOutputTimeout = orig }()

	_, err := execCommandRunner{}.Output("go", "version")
	if err == nil {
		t.Fatal("Output() error = nil with a 1ns timeout, want a deadline-exceeded error")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("Output() error = %v, want it to report the timeout", err)
	}
}

// TestExecCommandRunner_Run_SucceedsWithinTimeout is Finding 1's happy path: a fast, real command
// (`go version`) completes well within commandRunTimeout and Run returns its (nil) result
// unchanged — the new deadline wiring must not disturb normal Run behavior.
func TestExecCommandRunner_Run_SucceedsWithinTimeout(t *testing.T) {
	if err := (execCommandRunner{}).Run("go", "version"); err != nil {
		t.Fatalf("Run(go version) error = %v", err)
	}
}

// TestExecCommandRunner_Run_TimesOut proves Finding 1's bound actually fires: with commandRunTimeout
// shrunk to a few milliseconds, Run must cancel a subprocess that would otherwise hang forever and
// return an actionable, Spanish deadline error instead of blocking. helperCommand re-execs THIS SAME
// test binary as the "hanging" subprocess (Go's standard os/exec re-exec testing pattern — see
// TestHelperProcess below) so this test never depends on any platform-specific sleep/ping binary
// being present on PATH, and never actually waits anywhere near commandRunTimeout's real 5-minute
// default.
func TestExecCommandRunner_Run_TimesOut(t *testing.T) {
	orig := commandRunTimeout
	commandRunTimeout = 100 * time.Millisecond
	defer func() { commandRunTimeout = orig }()

	t.Setenv("CLICK_TEST_HANG_HELPER", "1")
	name, args := helperCommand(t)

	start := time.Now()
	err := (execCommandRunner{}).Run(name, args...)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("Run() error = nil with a 100ms timeout against a hanging subprocess, want a deadline-exceeded error")
	}
	if !strings.Contains(err.Error(), "no respondió") {
		t.Fatalf("Run() error = %v, want an actionable Spanish message reporting the timeout (D10)", err)
	}
	if !strings.Contains(err.Error(), name) {
		t.Fatalf("Run() error = %v, want it to name the command that timed out (%q)", err, name)
	}
	if elapsed > 10*time.Second {
		t.Fatalf("Run() took %s to return, want it bounded near commandRunTimeout (100ms) — the deadline must actually cancel the hanging subprocess, not wait for TestHelperProcess's own 1h sleep", elapsed)
	}
}

// helperCommand resolves this compiled test binary's own path so TestExecCommandRunner_Run_TimesOut
// can re-exec it as a "hanging" subprocess (see TestHelperProcess).
func helperCommand(t *testing.T) (name string, args []string) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable() error = %v", err)
	}
	return exe, []string{"-test.run=TestHelperProcess"}
}

// TestHelperProcess is not a real test: it is the subprocess entrypoint
// TestExecCommandRunner_Run_TimesOut re-execs into. It is a no-op under a normal `go test` run
// (CLICK_TEST_HANG_HELPER unset), and only blocks — far past any timeout this package's tests ever
// set — when explicitly invoked as the helper, so the PARENT process's context cancellation
// (commandRunTimeout) is what actually ends it, proving Run() really enforces the deadline instead
// of only formatting a nicer error around a subprocess that was going to exit on its own anyway.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("CLICK_TEST_HANG_HELPER") != "1" {
		return
	}
	time.Sleep(1 * time.Hour)
}
