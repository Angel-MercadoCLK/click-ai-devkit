//go:build windows

package installer

import (
	"encoding/base64"
	"fmt"
)

// managedEngramCloudHookCommand returns a Windows cmd.exe command that performs a bounded
// one-shot import of Engram memories from the cloud. The command is designed to run
// as a SessionStart hook and must never fail the session even if the cloud is
// unreachable or the project is not enrolled.
//
// Windows has no POSIX `timeout ... || true` syntax, so we delegate the 5-second bound
// to a hidden `click engram-cloud-import` subcommand implemented in Go.
func managedEngramCloudHookCommand(project string) (string, error) {
	if project == "" {
		return "", fmt.Errorf("project name cannot be empty")
	}

	// Encode the project name using base64url so it can never break out of cmd.exe quoting
	projectB64 := base64.RawURLEncoding.EncodeToString([]byte(project))

	// Use cmd.exe /d /s /c to run the command, then "exit /b 0" to ensure the whole chain succeeds
	cmd := `cmd.exe /d /s /c "click engram-cloud-import --project-b64 ` + projectB64 + ` & exit /b 0"`
	return cmd, nil
}
