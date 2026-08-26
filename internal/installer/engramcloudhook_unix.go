//go:build !windows

package installer

import (
	"fmt"
	"regexp"
)

// managedEngramCloudHookCommand returns a POSIX shell command that performs a bounded
// one-shot import of Engram memories from the cloud. The command is designed to run
// as a SessionStart hook and must never fail the session even if the cloud is
// unreachable or the project is not enrolled.
func managedEngramCloudHookCommand(project string) (string, error) {
	if project == "" {
		return "", fmt.Errorf("project name cannot be empty")
	}

	// POSIX shell quoting: single-quote escape if project contains characters outside [A-Za-z0-9._-]+
	// Otherwise, leave unquoted for readability.
	var projectArg string
	if needsQuoting(project) {
		projectArg = fmt.Sprintf("'%s'", project)
	} else {
		projectArg = project
	}

	cmd := fmt.Sprintf("timeout 5 engram sync --cloud --import --project %s || true", projectArg)
	return cmd, nil
}

// needsQuoting returns true if the string contains characters that require
// shell quoting for safe use in a POSIX shell.
func needsQuoting(s string) bool {
	// Characters that are safe without quotes in POSIX shell: alphanumeric, dot, underscore, hyphen
	safePattern := regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
	return !safePattern.MatchString(s)
}
