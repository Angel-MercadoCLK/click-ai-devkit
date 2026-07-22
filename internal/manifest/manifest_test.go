package manifest

import (
	"testing"

	"gopkg.in/yaml.v3"
)

// TestLoad_ParsesEmbeddedManifest is a Phase 0 smoke test: it just asserts the embedded
// manifest.yaml parses without error and the placeholder fields we expect at bootstrap time are
// populated. Real schema/content assertions land once Slice 1+ gives the manifest real values.
func TestLoad_ParsesEmbeddedManifest(t *testing.T) {
	m, err := Load()
	if err != nil {
		t.Fatalf("Load() returned an error: %v", err)
	}
	if m == nil {
		t.Fatal("Load() returned a nil manifest with no error")
	}

	if m.SchemaVersion != 1 {
		t.Errorf("SchemaVersion = %d, want 1", m.SchemaVersion)
	}
	if m.ClickVersion == "" {
		t.Error("ClickVersion is empty, want a placeholder value")
	}
	if m.Engram.Version == "" {
		t.Error("Engram.Version is empty, want a placeholder value")
	}
	if m.Engram.Source == "" {
		t.Error("Engram.Source is empty, want a placeholder value")
	}

	wantPlugins := []string{"click-sdd", "click-memory", "click-review", "click-skills"}
	for _, name := range wantPlugins {
		p, ok := m.Plugins[name]
		if !ok {
			t.Errorf("Plugins[%q] missing, want an entry", name)
			continue
		}
		if p.Version == "" {
			t.Errorf("Plugins[%q].Version is empty, want a placeholder value", name)
		}
		if p.Path == "" {
			t.Errorf("Plugins[%q].Path is empty, want a placeholder value", name)
		}
	}
}

// TestManifest_EngramCloudBlock verifies the optional engram_cloud manifest block is parsed when
// present and remains the zero value when absent, preserving backward-compatible local-only behavior.
func TestManifest_EngramCloudBlock(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  EngramCloud
	}{
		{
			name: "present block with values",
			input: `engram_cloud:
  server: "http://127.0.0.1:18080"
  project: "click-ai-devkit"`,
			want: EngramCloud{
				Server:  "http://127.0.0.1:18080",
				Project: "click-ai-devkit",
			},
		},
		{
			name:  "absent block yields zero value",
			input: "schema_version: 1\nclick_version: \"0.0.0\"\n",
			want:  EngramCloud{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var m Manifest
			if err := yaml.Unmarshal([]byte(tt.input), &m); err != nil {
				t.Fatalf("yaml.Unmarshal(%q) error = %v", tt.input, err)
			}
			if m.EngramCloud != tt.want {
				t.Errorf("EngramCloud = %+v, want %+v", m.EngramCloud, tt.want)
			}
		})
	}
}
