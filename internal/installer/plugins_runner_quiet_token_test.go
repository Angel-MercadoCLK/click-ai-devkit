package installer

import (
	"os"
	"testing"
)

// TestExecCommandRunner_RunQuietlyForwardsEngramCloudToken guards against the exact regression a
// fresh review found in this branch's own history: RunQuietly is the ONE call path (the Engram
// cloud step — config/enroll/upgrade/sync) that legitimately needs ENGRAM_CLOUD_TOKEN to
// authenticate. A prior fix for the unrelated generic-subprocess leak (F1) accidentally made
// RunQuietly share the same token-filtering fallback as Run/Output, which would have silently
// broken cloud enrollment for every real user who correctly exported the token. Uses the same
// helper-process re-exec pattern as TestExecCommandRunner_DoesNotLeakEngramCloudTokenToGenericSubprocess,
// but asserts the OPPOSITE outcome: the child spawned via RunQuietly MUST see the token.
func TestExecCommandRunner_RunQuietlyForwardsEngramCloudToken(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS_QUIET_TOKEN_CHECK") == "1" {
		// This process IS the child spawned by runner.RunQuietly() below. Report whether the
		// token reached our own environment, then exit immediately.
		if v, present := os.LookupEnv("ENGRAM_CLOUD_TOKEN"); present && v == os.Getenv("EXPECTED_TOKEN_FOR_CHECK") {
			os.Exit(0) // correct: the token was forwarded
		}
		os.Exit(1) // missing or wrong: RunQuietly failed to forward the token
	}

	fakeToken := "fake-quiet-token-67890"
	t.Setenv("ENGRAM_CLOUD_TOKEN", fakeToken)
	t.Setenv("EXPECTED_TOKEN_FOR_CHECK", fakeToken)
	t.Setenv("GO_WANT_HELPER_PROCESS_QUIET_TOKEN_CHECK", "1")

	runner := execCommandRunner{}
	if err := runner.RunQuietly(os.Args[0],
		"-test.run=^TestExecCommandRunner_RunQuietlyForwardsEngramCloudToken$"); err != nil {
		t.Fatalf("RunQuietly() did not forward ENGRAM_CLOUD_TOKEN to its child (child reported it was missing or wrong): %v", err)
	}
}
