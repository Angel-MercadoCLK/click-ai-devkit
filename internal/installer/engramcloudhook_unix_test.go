//go:build !windows

package installer

import (
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
