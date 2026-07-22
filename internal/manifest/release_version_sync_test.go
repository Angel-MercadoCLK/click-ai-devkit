package manifest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestReleaseVersionsMatchShippedMetadata prevents the embedded release manifest from drifting
// from the values shipped by the v0.5.2 release: the Scoop manifest owns click's release version,
// and each plugin.json owns its plugin version.
func TestReleaseVersionsMatchShippedMetadata(t *testing.T) {
	m, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	var scoop struct {
		Version string `json:"version"`
	}
	readJSON(t, filepath.Join("..", "..", "bucket", "click.json"), &scoop)
	if m.ClickVersion != scoop.Version {
		t.Errorf("manifest.yaml click_version = %q, want %q from bucket/click.json", m.ClickVersion, scoop.Version)
	}

	for name, plugin := range m.Plugins {
		var metadata struct {
			Version string `json:"version"`
		}
		readJSON(t, filepath.Join("..", "..", plugin.Path, ".claude-plugin", "plugin.json"), &metadata)
		if plugin.Version != metadata.Version {
			t.Errorf("manifest.yaml plugins[%q].version = %q, want %q from %s/.claude-plugin/plugin.json", name, plugin.Version, metadata.Version, plugin.Path)
		}
	}
}

func readJSON(t *testing.T, path string, target any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	if err := json.Unmarshal(data, target); err != nil {
		t.Fatalf("json.Unmarshal(%s) error = %v", path, err)
	}
}
