// Package manifest defines the embedded release manifest that click reads to know what
// "this click-ai-devkit release" means: the click-sdd/click-memory/click-review plugin
// versions and the pinned Engram version. The manifest is embedded into the binary at build
// time (go:embed) so `click doctor` and `click update` never need a network call to know what
// they should install — see tech-spec.md §2.3.
//
// There is deliberately no .claude-plugin/marketplace.json anywhere in this repo (D16,
// implementation-plan.md Phase 0 DoD): this manifest is the only source of truth for install
// content.
package manifest

import (
	_ "embed"
	"fmt"

	"gopkg.in/yaml.v3"
)

//go:embed manifest.yaml
var embeddedManifest []byte

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

// EngramCloud holds the optional non-secret Engram Cloud enrollment config surfaced in
// manifest.yaml. Empty values mean local-only Engram behavior (no cloud enrollment).
type EngramCloud struct {
	Server  string `yaml:"server"`
	Project string `yaml:"project"`
}

// Manifest is the parsed shape of manifest.yaml, per tech-spec.md §2.3.
type Manifest struct {
	SchemaVersion        int               `yaml:"schema_version"`
	ClickVersion         string            `yaml:"click_version"`
	Engram               Engram            `yaml:"engram"`
	Plugins              map[string]Plugin `yaml:"plugins"`
	MinClaudeCodeVersion string            `yaml:"min_claude_code_version"`
	EngramCloud          EngramCloud       `yaml:"engram_cloud"`
}

// Load parses the manifest embedded into the binary at build time.
func Load() (*Manifest, error) {
	var m Manifest
	if err := yaml.Unmarshal(embeddedManifest, &m); err != nil {
		return nil, fmt.Errorf("manifest: parse embedded manifest.yaml: %w", err)
	}
	return &m, nil
}
