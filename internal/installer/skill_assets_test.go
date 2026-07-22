package installer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestClickholaAssets verifies PR1's content-only slice: the canonical Spanish OpenClaw
// SKILL.md for clickhola and its English thin Claude Code alias. Both documents must parse
// as YAML and retain the contract markers described in the design/spec.
func TestClickholaAssets(t *testing.T) {
	tests := []struct {
		name         string
		path         string
		wantName     string
		wantInvokable bool
		wantMarkers  []string
		wantMissing  []string
	}{
		{
			name:         "openclaw_clickhola",
			path:         filepath.Join("..", "..", "internal", "installer", "assets", "openclaw", "skills", "clickhola", "SKILL.md"),
			wantName:     "clickhola",
			wantInvokable: true,
			wantMarkers: []string{
				"interview",
				"one question at a time",
				"HTML+CSS",
				"kebab-case",
				"sdd/",
				"/elicitation",
				"Problem",
				"Users",
				"Goal",
				"Scope",
				"Business rules",
				"Open questions",
			},
			wantMissing: []string{
				"{{CLICK_BIN}}",
			},
		},
		{
			name:         "click_sdd_clickhola_alias",
			path:         filepath.Join("..", "..", "plugins", "click-sdd", "skills", "clickhola", "SKILL.md"),
			wantName:     "clickhola",
			wantMarkers: []string{
				"click-elicitor",
				"orchestrator",
				"requirements-elicitation",
				"Step 1",
			},
			wantMissing: []string{
				"Engram Cloud enrollment",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := os.ReadFile(tt.path)
			if err != nil {
				t.Fatalf("ReadFile(%s) error = %v", tt.path, err)
			}
			content := string(data)

			frontmatter, body, ok := splitFrontmatter(content)
			if !ok {
				t.Fatalf("%s does not contain a valid YAML frontmatter block", tt.path)
			}

			var meta struct {
				Name         string `yaml:"name"`
				Description  string `yaml:"description"`
				UserInvokable bool  `yaml:"user-invocable"`
				Metadata     struct {
					OpenClaw struct {
						Requires struct {
							Bins []string `yaml:"bins"`
						} `yaml:"requires"`
					} `yaml:"openclaw"`
				} `yaml:"metadata"`
			}
			if err := yaml.Unmarshal([]byte(frontmatter), &meta); err != nil {
				t.Fatalf("yaml.Unmarshal(frontmatter) error = %v", err)
			}

			if meta.Name != tt.wantName {
				t.Errorf("frontmatter name = %q, want %q", meta.Name, tt.wantName)
			}
			if meta.Description == "" {
				t.Errorf("frontmatter description is empty, want non-empty")
			}
		if tt.wantInvokable && !meta.UserInvokable {
			t.Errorf("frontmatter user-invocable = %v, want true", meta.UserInvokable)
		}

			if tt.name == "openclaw_clickhola" {
				bins := meta.Metadata.OpenClaw.Requires.Bins
				hasEngram := false
				for _, b := range bins {
					if b == "engram" {
						hasEngram = true
						break
					}
				}
				if !hasEngram {
					t.Errorf("metadata.openclaw.requires.bins = %v, want it to contain %q", bins, "engram")
				}
			}

			// Body must not be empty.
			if strings.TrimSpace(body) == "" {
				t.Errorf("SKILL.md body is empty, want content after frontmatter")
			}

			for _, marker := range tt.wantMarkers {
				if !strings.Contains(content, marker) {
					t.Errorf("%s does not contain required marker %q", tt.path, marker)
				}
			}
			for _, marker := range tt.wantMissing {
				if strings.Contains(content, marker) {
					t.Errorf("%s must not contain marker %q", tt.path, marker)
				}
			}
		})
	}
}

// splitFrontmatter extracts the YAML frontmatter block delimited by `---` lines and the
// remaining body. It returns false if no closed frontmatter block is found.
func splitFrontmatter(content string) (frontmatter, body string, ok bool) {
	const delim = "---"
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != delim {
		return "", "", false
	}
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == delim {
			return strings.Join(lines[1:i], "\n"), strings.Join(lines[i+1:], "\n"), true
		}
	}
	return "", "", false
}
