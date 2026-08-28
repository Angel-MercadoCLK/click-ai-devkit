package installer

import (
	"os"
	"testing"
)

// TestExecCommandRunner_DoesNotLeakEngramCloudTokenToGenericSubprocess is the F1 regression test.
//
// commandEnv() legitimately returns a bare nil for the no-override case
// (TestExecRunnerRealRunLeavesClaudeConfigDirUnset pins this exact behavior, and must not be
// touched), so calling commandEnv() directly here would prove nothing: a nil result vacuously
// satisfies any "no leak" check regardless of whether real subprocess launches actually filter the
// token. The real guarantee lives in Run()/RunQuietly(), which fall back to filteredProcessEnv()
// precisely when commandEnv() returns nil — so this test goes through the real production entry
// point, Run(), with a genuine child process, using the standard Go helper-process re-exec pattern
// (as used throughout the standard library's own os/exec tests) to observe what the child actually
// sees in its own environment.
func TestExecCommandRunner_DoesNotLeakEngramCloudTokenToGenericSubprocess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS_TOKEN_LEAK_CHECK") == "1" {
		// This process IS the child spawned by runner.Run() below, re-running this same test
		// binary. Report whether the token leaked into OUR OWN environment, then exit immediately
		// — never reach the runner.Run() call further down, or the child would spawn a
		// grandchild, and so on without bound.
		if _, present := os.LookupEnv("ENGRAM_CLOUD_TOKEN"); present {
			os.Exit(1) // leak detected
		}
		os.Exit(0) // clean: the parent's token did not reach this child's environment
	}

	fakeToken := "fake-test-token-12345"
	t.Setenv("ENGRAM_CLOUD_TOKEN", fakeToken)
	t.Setenv("GO_WANT_HELPER_PROCESS_TOKEN_LEAK_CHECK", "1")

	runner := execCommandRunner{}
	if err := runner.Run(os.Args[0],
		"-test.run=^TestExecCommandRunner_DoesNotLeakEngramCloudTokenToGenericSubprocess$"); err != nil {
		t.Fatalf("ENGRAM_CLOUD_TOKEN leaked into a generic subprocess's environment (child reported it was present): %v", err)
	}
}
