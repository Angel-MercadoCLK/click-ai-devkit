package cli

import (
	"context"
	"encoding/base64"
	"os/exec"
	"time"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/installer"
	"github.com/spf13/cobra"
)

// engramCloudCommandContext is a package-level var seam for testing, following this repo's convention.
var engramCloudCommandContext = exec.CommandContext

// engramCloudImportTimeout is a package-level var seam for testing, following this repo's convention.
var engramCloudImportTimeout = 5 * time.Second

var (
	engramCloudImportNow           = time.Now
	engramCloudImportOutcomeWriter = installer.WriteEngramCloudImportOutcome
)

func setEngramCloudCommandContextForTests(commandContext func(context.Context, string, ...string) *exec.Cmd) func() {
	previous := engramCloudCommandContext
	engramCloudCommandContext = commandContext
	return func() { engramCloudCommandContext = previous }
}

func setEngramCloudImportNowForTests(now func() time.Time) func() {
	previous := engramCloudImportNow
	engramCloudImportNow = now
	return func() { engramCloudImportNow = previous }
}

func setEngramCloudImportOutcomeWriterForTests(writer func(installer.Config, installer.EngramCloudImportOutcome) error) func() {
	previous := engramCloudImportOutcomeWriter
	engramCloudImportOutcomeWriter = writer
	return func() { engramCloudImportOutcomeWriter = previous }
}

// newEngramCloudImportCommand creates the hidden subcommand that performs a bounded
// one-shot import of Engram memories from the cloud.
//
// This command is hidden and meant to be called from the SessionStart hook generated
// by managedEngramCloudHookCommand. It runs `engram sync --cloud --import --project <project>`
// with a 5-second timeout and ALWAYS returns nil, never surfacing errors to the caller.
func newEngramCloudImportCommand() *cobra.Command {
	var projectB64 string

	cmd := &cobra.Command{
		Use:    "engram-cloud-import",
		Short:  "Internal command: bounded one-shot Engram cloud import",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			recordOutcome := func(status, reason string) {
				claudeHome, err := installer.ResolveClaudeHome()
				if err != nil {
					return
				}
				_ = engramCloudImportOutcomeWriter(installer.Config{ClaudeHome: claudeHome}, installer.EngramCloudImportOutcome{
					Timestamp: engramCloudImportNow().UTC(),
					Status:    status,
					Reason:    reason,
				})
			}

			// Decode the base64url-encoded project name
			projectBytes, err := base64.RawURLEncoding.DecodeString(projectB64)
			if err != nil {
				// Decoding errors are silently suppressed - the command must never fail
				recordOutcome(installer.EngramCloudImportOutcomeFailure, "invalid project encoding")
				return nil
			}
			project := string(projectBytes)

			// Create a context with the production timeout
			ctx, cancel := context.WithTimeout(context.Background(), engramCloudImportTimeout)
			defer cancel()

			// Run the engram sync command with the exact argv required
			engramCmd := engramCloudCommandContext(ctx, "engram", "sync", "--cloud", "--import", "--project", project)

			// The command is started; we wait for it to complete or timeout
			if err := engramCmd.Run(); err != nil {
				// ALL errors are silently suppressed:
				// - Missing binary (exec error)
				// - Timeout (ctx canceled)
				// - Unenrolled project (exit code 1 with message)
				// - Unreachable server (network error)
				// The SessionStart hook must never surface these as session errors
				if ctx.Err() == context.DeadlineExceeded {
					recordOutcome(installer.EngramCloudImportOutcomeTimeout, "import timed out")
				} else {
					recordOutcome(installer.EngramCloudImportOutcomeFailure, "import command failed")
				}
				return nil
			}

			recordOutcome(installer.EngramCloudImportOutcomeSuccess, "")
			return nil
		},
	}

	cmd.Flags().StringVar(&projectB64, "project-b64", "", "base64url-encoded project name (required)")
	_ = cmd.MarkFlagRequired("project-b64")

	return cmd
}
