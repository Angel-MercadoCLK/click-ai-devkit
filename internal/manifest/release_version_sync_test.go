package manifest

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestScoopMetadataIsIndependentOfRuntimePins validates the published Scoop artifact without
// coupling it to source-only runtime pins in the embedded manifest.
func TestScoopMetadataIsIndependentOfRuntimePins(t *testing.T) {
	manifestData, err := os.ReadFile("manifest.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var manifestFields map[string]any
	if err := yaml.Unmarshal(manifestData, &manifestFields); err != nil {
		t.Fatal(err)
	}
	if _, exists := manifestFields["click_version"]; exists {
		t.Fatal("manifest.yaml still contains click_version; CLI release identity comes from GoReleaser ldflags")
	}

	var scoop struct {
		Version      string `json:"version"`
		Architecture struct {
			Bit64 struct {
				URL  string `json:"url"`
				Hash string `json:"hash"`
			} `json:"64bit"`
		} `json:"architecture"`
	}
	readJSON(t, filepath.Join("..", "..", "bucket", "click.json"), &scoop)
	if err := validateScoopMetadata(scoop.Version, scoop.Architecture.Bit64.URL, scoop.Architecture.Bit64.Hash); err != nil {
		t.Fatalf("invalid published Scoop metadata: %v", err)
	}
}

func TestValidateScoopMetadata(t *testing.T) {
	valid := struct{ version, rawURL, hash string }{
		version: "0.7.1",
		rawURL:  "https://github.com/Angel-MercadoCLK/click-ai-devkit/releases/download/v0.7.1/click_0.7.1_windows_amd64.zip",
		hash:    "c81dd027e5cd5ae51491355685cae60e515fb5695e68cfe45b25cf2dd6de4916",
	}
	tests := []struct {
		name    string
		version string
		rawURL  string
		hash    string
		wantErr bool
	}{
		{name: "consistent artifact", version: valid.version, rawURL: valid.rawURL, hash: valid.hash},
		{name: "invalid version", version: "latest", rawURL: valid.rawURL, hash: valid.hash, wantErr: true},
		{name: "URL does not match version", version: "0.8.0", rawURL: valid.rawURL, hash: valid.hash, wantErr: true},
		{name: "non-release URL", version: valid.version, rawURL: "https://example.com/click.zip", hash: valid.hash, wantErr: true},
		{name: "invalid SHA-256", version: valid.version, rawURL: valid.rawURL, hash: "not-a-hash", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateScoopMetadata(tt.version, tt.rawURL, tt.hash)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateScoopMetadata() error = %v, wantErr %t", err, tt.wantErr)
			}
		})
	}
}

var scoopVersionPattern = regexp.MustCompile(`^\d+\.\d+\.\d+$`)
var sha256Pattern = regexp.MustCompile(`^[a-fA-F0-9]{64}$`)

func validateScoopMetadata(version, rawURL, hash string) error {
	if !scoopVersionPattern.MatchString(version) {
		return fmt.Errorf("version %q is not a stable semantic version", version)
	}
	parsedURL, err := url.Parse(rawURL)
	if err != nil || parsedURL.Scheme != "https" || parsedURL.Host != "github.com" {
		return fmt.Errorf("URL %q is not a GitHub HTTPS release URL", rawURL)
	}
	wantPath := fmt.Sprintf("/Angel-MercadoCLK/click-ai-devkit/releases/download/v%s/click_%s_windows_amd64.zip", version, version)
	if parsedURL.Path != wantPath {
		return fmt.Errorf("URL path %q does not match version %q", parsedURL.Path, version)
	}
	if !sha256Pattern.MatchString(hash) {
		return fmt.Errorf("hash %q is not a SHA-256", hash)
	}
	return nil
}

// TestPluginVersionsMatchShippedMetadata ensures the embedded plugin pins stay in sync with the
// retained plugin manifests; plugin metadata is independent of the published Scoop release.
func TestPluginVersionsMatchShippedMetadata(t *testing.T) {
	m, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
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
