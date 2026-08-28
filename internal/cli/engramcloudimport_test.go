package cli

import (
	"context"
	"errors"
	"os/exec"
	"testing"
	"time"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/installer"
)

// TestEngramCloudImportCommand_ExactEngramArgv verifies that running the hidden
// command with an injected fake engramCloudCommandContext records argv EXACTLY:
// ["sync","--cloud","--import","--project","team-hive"]
func TestEngramCloudImportCommand_ExactEngramArgv(t *testing.T) {
	// Capture the argv passed to the fake engram command
	var capturedArgs []string

	// Inject a fake context that records argv
	originalCtx := engramCloudCommandContext
	t.Cleanup(func() {
		engramCloudCommandContext = originalCtx
	})
	engramCloudCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		capturedArgs = args
		// Return a fake command that never actually runs
		return exec.CommandContext(ctx, "true")
	}

	// Run the hidden command with --project-b64 for "team-hive"
	cmd := newEngramCloudImportCommand()
	cmd.SetArgs([]string{"--project-b64", "dGVhbS1oaXZl"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() returned error: %v", err)
	}

	expectedArgs := []string{"sync", "--cloud", "--import", "--project", "team-hive"}
	if !equalSlices(capturedArgs, expectedArgs) {
		t.Errorf("captured argv = %v, want %v", capturedArgs, expectedArgs)
	}

	// Dedicated assertion on --import token, same reasoning as 3.1
	foundImport := false
	for _, arg := range capturedArgs {
		if arg == "--import" {
			foundImport = true
			break
		}
	}
	if !foundImport {
		t.Errorf("captured argv %v does not contain --import token", capturedArgs)
	}
}

func equalSlices(a, b []string) bool {
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

// TestEngramCloudImportCommand_BoundedAndAlwaysSucceeds verifies that the command
// respects the timeout and always returns nil (never fails), even when the child
// command blocks on ctx.Done().
func TestEngramCloudImportCommand_BoundedAndAlwaysSucceeds(t *testing.T) {
	// Inject a short timeout to make tests fast
	originalTimeout := engramCloudImportTimeout
	t.Cleanup(func() {
		engramCloudImportTimeout = originalTimeout
	})
	engramCloudImportTimeout = 10 * time.Millisecond

	// Inject a fake command context that creates a blocking command
	originalCtx := engramCloudCommandContext
	t.Cleanup(func() {
		engramCloudCommandContext = originalCtx
	})
	engramCloudCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		// Create a command that blocks until ctx is done
		return exec.CommandContext(ctx, "sleep", "1000") // 1000 seconds, will be killed by timeout
	}

	// Run the hidden command
	cmd := newEngramCloudImportCommand()
	cmd.SetArgs([]string{"--project-b64", "dGVhbS1oaXZl"})

	start := time.Now()
	err := cmd.Execute()
	elapsed := time.Since(start)

	// The command must always return nil, even on timeout or missing binary
	if err != nil {
		t.Fatalf("Execute() returned error (must always succeed): %v", err)
	}

	// Verify the command terminated quickly (well under 5 seconds, closer to our 10ms timeout)
	// The actual elapsed time should be roughly the 10ms timeout plus process cleanup overhead
	if elapsed > 5*time.Second {
		t.Errorf("Execute() took %v, expected it to be bounded by timeout (well under 5s)", elapsed)
	}

	// Add a helper-process test proving the child actually exits
	// We do this by checking that the elapsed time is significantly less than 1000 seconds
	// (the sleep duration we injected), proving the process was killed by the timeout
	if elapsed > time.Minute {
		t.Errorf("Execute() took %v, but the child command sleeps for 1000s; the timeout must have killed it", elapsed)
	}
}

func TestEngramCloudImportCommand_RecordsSuccessOutcome(t *testing.T) {
	claudeHome := t.TempDir()
	t.Setenv("CLICK_CLAUDE_HOME", claudeHome)
	fixedNow := time.Date(2026, time.August, 27, 12, 0, 0, 0, time.UTC)
	restoreNow := setEngramCloudImportNowForTests(func() time.Time { return fixedNow })
	t.Cleanup(restoreNow)
	restoreCommand := setEngramCloudCommandContextForTests(func(ctx context.Context, name string, args ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "true")
	})
	t.Cleanup(restoreCommand)

	cmd := newEngramCloudImportCommand()
	cmd.SetArgs([]string{"--project-b64", "dGVhbS1oaXZl"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v, want nil", err)
	}

	outcome, found, err := installer.LoadEngramCloudImportOutcome(installer.Config{ClaudeHome: claudeHome})
	if err != nil || !found {
		t.Fatalf("LoadEngramCloudImportOutcome() = (%+v, %v, %v), want a record", outcome, found, err)
	}
	if outcome.Status != installer.EngramCloudImportOutcomeSuccess || !outcome.Timestamp.Equal(fixedNow) {
		t.Fatalf("outcome = %+v, want success at %s", outcome, fixedNow)
	}
}

func TestEngramCloudImportCommand_RecordsFailureAndStillReturnsNil(t *testing.T) {
	claudeHome := t.TempDir()
	t.Setenv("CLICK_CLAUDE_HOME", claudeHome)
	restoreCommand := setEngramCloudCommandContextForTests(func(ctx context.Context, name string, args ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "cmd", "/c", "exit 1")
	})
	t.Cleanup(restoreCommand)

	cmd := newEngramCloudImportCommand()
	cmd.SetArgs([]string{"--project-b64", "dGVhbS1oaXZl"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v, want nil", err)
	}

	outcome, found, err := installer.LoadEngramCloudImportOutcome(installer.Config{ClaudeHome: claudeHome})
	if err != nil || !found {
		t.Fatalf("LoadEngramCloudImportOutcome() = (%+v, %v, %v), want a record", outcome, found, err)
	}
	if outcome.Status != installer.EngramCloudImportOutcomeFailure || outcome.Reason == "" {
		t.Fatalf("outcome = %+v, want recorded failure with a reason", outcome)
	}
}

func TestEngramCloudImportCommand_RecordWriteFailureIsSwallowed(t *testing.T) {
	t.Setenv("CLICK_CLAUDE_HOME", t.TempDir())
	restoreWrite := setEngramCloudImportOutcomeWriterForTests(func(installer.Config, installer.EngramCloudImportOutcome) error {
		return errors.New("record write failed")
	})
	t.Cleanup(restoreWrite)
	restoreCommand := setEngramCloudCommandContextForTests(func(ctx context.Context, name string, args ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "true")
	})
	t.Cleanup(restoreCommand)

	cmd := newEngramCloudImportCommand()
	cmd.SetArgs([]string{"--project-b64", "dGVhbS1oaXZl"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v, want nil when recording fails", err)
	}
}
