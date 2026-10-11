// Package manifest defines the embedded release metadata and qualified upstream tool pin used
// by click. The manifest is embedded into the binary at build time (go:embed), so Click's local
// lifecycle decisions do not need a network call — see tech-spec.md §2.3.
//
// The Claude marketplace manifest separately describes retained Click plugins. Workflow and
// model configuration are owned by gentle-ai, not by a Click plugin entry.
package manifest

import (
	_ "embed"
	"fmt"

	"gopkg.in/yaml.v3"
)

//go:embed manifest.yaml
var embeddedManifest []byte

var manifestContents = embeddedManifest

// Plugin describes one of click-ai-devkit's three plugins as pinned by a release.
type Plugin struct {
	Version string `yaml:"version"`
	Path    string `yaml:"path"`
}

// Engram describes the pinned Engram version bundled with this click-ai-devkit release (D8).
type Engram struct {
	Version string `yaml:"version"`
	Source  string `yaml:"source"`
}

// GentleAIAsset is an official release archive qualified by a pinned SHA-256.
type GentleAIAsset struct {
	Name   string `yaml:"name"`
	SHA256 string `yaml:"sha256"`
}

// GentleAI is the qualified upstream CLI pin. Assets are keyed by GOOS_GOARCH;
// Windows intentionally has no archive and is built from Module@Version.
type GentleAI struct {
	Version string                   `yaml:"version"`
	Module  string                   `yaml:"module"`
	Assets  map[string]GentleAIAsset `yaml:"assets"`
}

// EngramCloud holds the optional non-secret Engram Cloud enrollment config surfaced in
// manifest.yaml. Empty values mean local-only Engram behavior (no cloud enrollment).
type EngramCloud struct {
	Server  string `yaml:"server"`
	Project string `yaml:"project"`
}

// Manifest is the parsed shape of manifest.yaml, per tech-spec.md §2.3.
type Manifest struct {
	SchemaVersion        int               `yaml:"schema_version"`
	GentleAI             GentleAI          `yaml:"gentle_ai"`
	Engram               Engram            `yaml:"engram"`
	Plugins              map[string]Plugin `yaml:"plugins"`
	MinClaudeCodeVersion string            `yaml:"min_claude_code_version"`
	EngramCloud          EngramCloud       `yaml:"engram_cloud"`
}

// Load parses the manifest embedded into the binary at build time.
func Load() (*Manifest, error) {
	var m Manifest
	if err := yaml.Unmarshal(manifestContents, &m); err != nil {
		return nil, fmt.Errorf("manifest: parse embedded manifest.yaml: %w", err)
	}
	return &m, nil
}

// SetManifestForTests replaces the embedded manifest for a test and returns a restore function.
func SetManifestForTests(m Manifest) func() {
	previous := manifestContents
	encoded, err := yaml.Marshal(m)
	if err != nil {
		panic(err)
	}
	manifestContents = encoded
	return func() { manifestContents = previous }
}
