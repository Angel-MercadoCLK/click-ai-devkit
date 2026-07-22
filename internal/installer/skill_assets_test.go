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
// as YAML and retain the concrete contract sections/phrases supplied by the user.
func TestClickholaAssets(t *testing.T) {
	tests := []struct {
		name          string
		path          string
		wantName      string
		wantInvokable bool
		wantDesc      string
		wantMarkers   []string
		wantMissing   []string
	}{
		{
			name:          "openclaw_clickhola",
			path:          filepath.Join("..", "..", "internal", "installer", "assets", "openclaw", "skills", "clickhola", "SKILL.md"),
			wantName:      "clickhola",
			wantInvokable: true,
			wantDesc:      "non-technical requesters who ambiguously ask to build/imagine an app, screen, or feature",
			wantMarkers: []string{
				"# clickhola — captura de ideas para Click AI (perfil no técnico)",
				"habla en español",
				"sin jerga técnica",
				"el solicitante no programa",
				"una pregunta por turno",
				"espera",
				"1) **problema/resultado deseado**",
				"2) **usuarios**",
				"3) **apariencia/función imaginada y pasos del usuario**",
				"4) **lo que NO debe hacer o límites importantes**",
				"límites importantes",
				"detente cuando sea suficiente",
				"HTML",
				"CSS",
				"inline",
				"sin dependencias externas",
				"bosquejo de referencia desechable",
				"kebab-case",
				"confirma",
				"mem_save",
				"sdd/{change-name}/elicitation",
				"Source: clickhola (OpenClaw)",
				"Problem",
				"Users",
				"Goal",
				"Scope (in-out)",
				"Business rules & edge cases",
				"Open questions",
				"no inventes requisitos",
				"nunca incluyas credenciales",
			},
			wantMissing: []string{
				"{{CLICK_BIN}}",
			},
		},
		{
			name:     "click_sdd_clickhola_alias",
			path:     filepath.Join("..", "..", "plugins", "click-sdd", "skills", "clickhola", "SKILL.md"),
			wantName: "clickhola",
			wantMarkers: []string{
				"click-elicitor",
				"requirements-elicitation",
				"Paso 1",
				"alias",
			},
			wantMissing: []string{
				"duplicate elicitation logic",
				"executor",
				"Engram Cloud",
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
				Name          string `yaml:"name"`
				Description   string `yaml:"description"`
				UserInvokable bool   `yaml:"user-invocable"`
				Metadata      struct {
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
			if tt.wantDesc != "" && !strings.Contains(meta.Description, tt.wantDesc) {
				t.Errorf("frontmatter description = %q, want it to contain %q", meta.Description, tt.wantDesc)
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
