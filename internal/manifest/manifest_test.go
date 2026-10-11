package manifest

import (
	"os"
	"strings"
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
	if m.Engram.Version == "" {
		t.Error("Engram.Version is empty, want a placeholder value")
	}
	if m.Engram.Source == "" {
		t.Error("Engram.Source is empty, want a placeholder value")
	}

	wantPlugins := []string{"click-memory", "click-review", "click-skills"}
	if len(m.Plugins) != len(wantPlugins) {
		t.Fatalf("Plugins contains %d entries, want only retained Click plugins %v", len(m.Plugins), wantPlugins)
	}
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
			input: "schema_version: 1\n",
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

func TestGentleAIVersionMatchesManifest(t *testing.T) {
	data, err := os.ReadFile("../../GENTLE_AI_VERSION")
	if err != nil {
		t.Fatal(err)
	}
	m, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if m.GentleAI.Version != strings.TrimSpace(string(data)) {
		t.Fatalf("gentle_ai.version = %q, want GENTLE_AI_VERSION %q", m.GentleAI.Version, strings.TrimSpace(string(data)))
	}
}

func TestGentleAIQualifiedAssets(t *testing.T) {
	m, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if m.GentleAI.Version != "v4.0.0" || m.GentleAI.Module != "github.com/gentleman-programming/gentle-ai/v4/cmd/gentle-ai" {
		t.Fatalf("unqualified gentle-ai pin: %+v", m.GentleAI)
	}
	want := map[string]string{
		"linux_amd64":  "5f4417cf29c969c86da4799942fd673368840901be1bb09c779a12d7ed6096ea",
		"linux_arm64":  "1383b040c95cfc69206660d73c21907b14ad70ab913f410917c54f43d3147e57",
		"darwin_amd64": "b5b74f22b38ec3339b38e8c68f797dc76ff12ed6580826a6e425d5c718da80c1",
		"darwin_arm64": "d2159caf6d68f367b18830ece6af71ef26963d5f5320d7df6a794773f45cc7e9",
	}
	if len(m.GentleAI.Assets) != len(want) {
		t.Fatalf("assets = %+v, want four qualified platforms", m.GentleAI.Assets)
	}
	for platform, hash := range want {
		t.Run(platform, func(t *testing.T) {
			asset := m.GentleAI.Assets[platform]
			if asset.Name != "gentle-ai_4.0.0_"+platform+".tar.gz" || asset.SHA256 != hash {
				t.Fatalf("asset = %+v, want qualified name and SHA-256 %s", asset, hash)
			}
		})
	}
}
