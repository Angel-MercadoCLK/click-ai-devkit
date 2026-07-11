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
	if len(manifest.Plugins) != 3 {
		t.Fatalf("marketplace plugins = %d, want 3", len(manifest.Plugins))
	}
}

func TestClickSDDPlugin_ManifestAndFilesAreStructurallyValid(t *testing.T) {
	assertPluginStructure(t, filepath.Join("plugins", "click-sdd"), "click-sdd", []string{
		filepath.Join("agents", "click-orchestrator.md"),
		filepath.Join("agents", "click-prd-writer.md"),
		filepath.Join("agents", "click-architect.md"),
		filepath.Join("agents", "click-reviewer.md"),
		filepath.Join("agents", "click-memory-curator.md"),
		filepath.Join("skills", "sdd-explore", "SKILL.md"),
		filepath.Join("skills", "sdd-prd", "SKILL.md"),
		filepath.Join("skills", "sdd-design", "SKILL.md"),
		filepath.Join("skills", "sdd-tasks", "SKILL.md"),
		filepath.Join("skills", "sdd-code", "SKILL.md"),
		filepath.Join("skills", "sdd-review", "SKILL.md"),
	})
}

func TestClickSDDPlugin_DeclaresDefaultOrchestrationProfile(t *testing.T) {
	type configField struct {
		Type        string `json:"type"`
		Title       string `json:"title"`
		Description string `json:"description"`
		Default     string `json:"default"`
	}
	type pluginManifest struct {
		UserConfig map[string]configField `json:"userConfig"`
	}

	data := mustReadRepoFile(t, "plugins", "click-sdd", ".claude-plugin", "plugin.json")
	var manifest pluginManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("plugin.json parse error = %v", err)
	}
	field, ok := manifest.UserConfig["orchestration_profile"]
	if !ok {
		t.Fatal("click-sdd plugin.json missing orchestration_profile userConfig")
	}
	if field.Type != "string" || field.Default != "default" {
		t.Fatalf("orchestration_profile = %+v, want string default profile", field)
	}
	if !strings.Contains(field.Description, "runtime profile") {
		t.Fatalf("orchestration_profile description = %q, want it to explain runtime profile resolution", field.Description)
	}
}

func TestClickOrchestrator_DocumentsRuntimeProfileAndDelegationPolicy(t *testing.T) {
	content := string(mustReadRepoFile(t, "plugins", "click-sdd", "agents", "click-orchestrator.md"))
	required := []string{
		"## Runtime profile resolution",
		"orchestration_profile",
		"built-in `default` profile",
		"simple inline work",
		"non-trivial work",
		"must delegate",
		"Engram is always part of the working model",
	}
	for _, fragment := range required {
		if !strings.Contains(content, fragment) {
			t.Fatalf("click-orchestrator.md missing %q", fragment)
		}
	}
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
