package installer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMarketplaceManifest_Parses(t *testing.T) {
	type marketplaceManifest struct {
		Name    string `json:"name"`
		Plugins []struct {
			Name   string `json:"name"`
			Source string `json:"source"`
		} `json:"plugins"`
	}

	data := mustReadRepoFile(t, ".claude-plugin", "marketplace.json")
	var manifest marketplaceManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("marketplace.json parse error = %v", err)
	}
	if manifest.Name != marketplaceName {
		t.Fatalf("marketplace name = %q, want %q", manifest.Name, marketplaceName)
	}
	if len(manifest.Plugins) != 4 {
		t.Fatalf("marketplace plugins = %d, want 4", len(manifest.Plugins))
	}
}

func TestClickSDDPlugin_ManifestAndFilesAreStructurallyValid(t *testing.T) {
	assertPluginStructure(t, filepath.Join("plugins", "click-sdd"), "click-sdd", []string{
		filepath.Join("agents", "click-orchestrator.md"),
		filepath.Join("agents", "click-prd-writer.md"),
		filepath.Join("agents", "click-architect.md"),
		filepath.Join("agents", "click-reviewer.md"),
		filepath.Join("agents", "click-memory-curator.md"),
		filepath.Join("agents", "click-elicitor.md"),
		filepath.Join("agents", "click-explore.md"),
		filepath.Join("agents", "click-apply.md"),
		filepath.Join("agents", "click-archive.md"),
		filepath.Join("agents", "click-onboard.md"),
		filepath.Join("skills", "explore", "SKILL.md"),
		filepath.Join("skills", "propose", "SKILL.md"),
		filepath.Join("skills", "spec", "SKILL.md"),
		filepath.Join("skills", "design", "SKILL.md"),
		filepath.Join("skills", "tasks", "SKILL.md"),
		filepath.Join("skills", "apply", "SKILL.md"),
		filepath.Join("skills", "verify", "SKILL.md"),
		filepath.Join("skills", "archive", "SKILL.md"),
		filepath.Join("skills", "onboard", "SKILL.md"),
		filepath.Join("skills", "jd-judge-a", "SKILL.md"),
		filepath.Join("skills", "jd-judge-b", "SKILL.md"),
		filepath.Join("skills", "jd-fix-agent", "SKILL.md"),
		filepath.Join("skills", "agent-builder", "SKILL.md"),
	})
}

func TestClickMemoryPlugin_ManifestAndFilesAreStructurallyValid(t *testing.T) {
	assertPluginStructure(t, filepath.Join("plugins", "click-memory"), "click-memory", []string{
		filepath.Join("skills", "memory-proposal", "SKILL.md"),
		filepath.Join("skills", "memory-review", "SKILL.md"),
		filepath.Join("docs", "memory-policy.md"),
		filepath.Join("docs", "allowed-memory.md"),
		filepath.Join("docs", "forbidden-memory.md"),
		filepath.Join("docs", "engram-setup.md"),
	})
}

func TestClickReviewPlugin_ManifestAndFilesAreStructurallyValid(t *testing.T) {
	assertPluginStructure(t, filepath.Join("plugins", "click-review"), "click-review", []string{
		filepath.Join("agents", "click-pr-reviewer.md"),
		filepath.Join("skills", "pr-review", "SKILL.md"),
		filepath.Join("skills", "pre-merge-checklist", "SKILL.md"),
	})
}

func assertPluginStructure(t *testing.T, pluginDir, wantName string, expectedFiles []string) {
	t.Helper()
	type pluginManifest struct {
		Name        string `json:"name"`
		Version     string `json:"version"`
		Description string `json:"description"`
		Author      struct {
			Name string `json:"name"`
		} `json:"author"`
	}

	data := mustReadRepoFile(t, pluginDir, ".claude-plugin", "plugin.json")
	var manifest pluginManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("plugin.json parse error = %v", err)
	}
	if manifest.Name != wantName {
		t.Fatalf("manifest.Name = %q, want %q", manifest.Name, wantName)
	}
	if manifest.Version == "" || manifest.Description == "" || manifest.Author.Name == "" {
		t.Fatalf("manifest fields missing: %+v", manifest)
	}

	for _, name := range expectedFiles {
		content := mustReadRepoFile(t, pluginDir, name)
		if len(strings.TrimSpace(string(content))) == 0 {
			t.Fatalf("expected file %s is empty", name)
		}
	}
}

func mustReadRepoFile(t *testing.T, elems ...string) []byte {
	t.Helper()
	parts := append([]string{"..", ".."}, elems...)
	path := filepath.Join(parts...)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	return data
}
