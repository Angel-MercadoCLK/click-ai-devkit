//go:build !windows

package installer

import (
	"encoding/base64"
	"fmt"
)

// managedEngramCloudHookCommand returns a POSIX shell command that performs a bounded
// one-shot import of Engram memories from the cloud. The command is designed to run
// as a SessionStart hook and must never fail the session even if the cloud is
// unreachable or the project is not enrolled.
func managedEngramCloudHookCommand(project string) (string, error) {
	if project == "" {
		return "", fmt.Errorf("project name cannot be empty")
	}

	// base64url keeps the project inert in the shell; the hidden click command owns
	// the timeout and always reports success to the SessionStart hook.
	projectB64 := base64.RawURLEncoding.EncodeToString([]byte(project))
	return fmt.Sprintf("click engram-cloud-import --project-b64 %s || true", projectB64), nil
}
