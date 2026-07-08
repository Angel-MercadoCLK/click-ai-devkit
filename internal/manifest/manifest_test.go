package manifest

import "testing"

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

	wantPlugins := []string{"click-sdd", "click-memory", "click-review"}
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
