package installer

import (
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
