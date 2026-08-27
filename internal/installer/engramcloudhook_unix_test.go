//go:build !windows

package installer

import (
	"fmt"
	"strings"
	"testing"
)

func TestManagedEngramCloudHookCommand_POSIXExactString(t *testing.T) {
	project := "team-hive"
	result, err := managedEngramCloudHookCommand(project)
	if err != nil {
		t.Fatalf("managedEngramCloudHookCommand(%q) returned error: %v", project, err)
	}

	expected := `timeout 5 engram sync --cloud --import --project team-hive || true`
	if result != expected {
		t.Errorf("managedEngramCloudHookCommand(%q) = %q, want %q", project, result, expected)
	}

	// Dedicated assertion on --import token so dropping it fails this test specifically
	if !strings.Contains(result, "--import") {
		t.Errorf("managedEngramCloudHookCommand(%q) result %q does not contain --import token", project, result)
	}
}

func TestManagedEngramCloudHookCommand_POSIXShellQuoting(t *testing.T) {
	tests := []struct {
		name     string
		project  string
		contains string // substring that must be present
		absent   string // substring that must NOT be present
	}{
		{
			name:     "safe characters remain unquoted",
			project:  "team-hive",
			contains: `--project team-hive`,
			absent:   `'team-hive'`,
		},
		{
			name:     "space requires quoting",
			project:  "team hive",
			contains: `'team hive'`,
			absent:   "--project team hive", // without quotes would be parsed as two arguments
		},
		{
			name:     "semicolon requires quoting",
			project:  "team;hive",
			contains: `'team;hive'`,
			absent:   "--project team;hive",
		},
		{
			name:     "dollar sign requires quoting",
			project:  "team$hive",
			contains: `'team$hive'`,
			absent:   "--project team$hive",
		},
		{
			name:     "ampersand requires quoting",
			project:  "team&hive",
			contains: `'team&hive'`,
			absent:   "--project team&hive",
		},
		{
			name:     "mixed safe characters remain unquoted",
			project:  "team_123.hive-project",
			contains: `--project team_123.hive-project`,
			absent:   `'team_123.hive-project'`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := managedEngramCloudHookCommand(tt.project)
			if err != nil {
				t.Fatalf("managedEngramCloudHookCommand(%q) returned error: %v", tt.project, err)
			}

			if !strings.Contains(result, tt.contains) {
				t.Errorf("managedEngramCloudHookCommand(%q) result %q does not contain expected substring %q", tt.project, result, tt.contains)
			}

			if tt.absent != "" && strings.Contains(result, tt.absent) {
				t.Errorf("managedEngramCloudHookCommand(%q) result %q contains unexpected substring %q", tt.project, result, tt.absent)
			}
		})
	}
}

func TestManagedEngramCloudHookCommand_POSIXEscapesApostrophes(t *testing.T) {
	project := "team'oops; touch /tmp/pwned; echo '"
	result, err := managedEngramCloudHookCommand(project)
	if err != nil {
		t.Fatalf("managedEngramCloudHookCommand(%q) returned error: %v", project, err)
	}

	// The escaped form: replace each ' with '\'' (close quote, escaped literal quote, reopen quote)
	// "team'oops; touch /tmp/pwned; echo '" becomes "team'\''oops; touch /tmp/pwned; echo '\''"
	expectedEscaped := "team'\\''oops; touch /tmp/pwned; echo '\\''"
	escapedProjectArg := fmt.Sprintf("'%s'", expectedEscaped)
	expectedCmd := fmt.Sprintf("timeout 5 engram sync --cloud --import --project %s || true", escapedProjectArg)

	if result != expectedCmd {
		t.Errorf("managedEngramCloudHookCommand(%q) = %q, want %q", project, result, expectedCmd)
	}

	// Verify the apostrophe is escaped and the project name appears as a single inert argument
	// The command should contain '\'' sequences for each apostrophe
	if !strings.Contains(result, `'\''`) {
		t.Errorf("managedEngramCloudHookCommand(%q) result %q does not contain escaped apostrophe sequence '\\''", project, result)
	}

	// The raw unescaped apostrophe should NOT appear (except as part of the escape sequence)
	// Count raw apostrophes that are NOT part of the escape sequence '\'' or ''
	rawApostropheCount := strings.Count(result, "'") - strings.Count(result, `'\''`)*2 - strings.Count(result, `''`)
	if rawApostropheCount != 0 {
		t.Errorf("managedEngramCloudHookCommand(%q) result %q contains %d raw unescaped apostrophes (should be 0)", project, result, rawApostropheCount)
	}
}
