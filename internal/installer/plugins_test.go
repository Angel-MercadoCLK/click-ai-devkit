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
		filepath.Join("agents", "click-jd-judge-a.md"),
		filepath.Join("agents", "click-jd-judge-b.md"),
		filepath.Join("agents", "click-jd-fix-agent.md"),
		filepath.Join("agents", "click-review-risk.md"),
		filepath.Join("agents", "click-review-readability.md"),
		filepath.Join("agents", "click-review-reliability.md"),
		filepath.Join("agents", "click-review-resilience.md"),
		filepath.Join("agents", "click-review-refuter.md"),
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

func TestInstalledPluginPath_UsesRegistryInstallPath(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir()}
	path := filepath.Join(cfg.ClaudeHome, "plugins", "cache", "engram", "engram", "0.1.1")
	data, err := json.Marshal(map[string]any{
		"plugins": map[string]any{EngramPluginID: []map[string]any{{"installPath": path}}},
	})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(cfg.InstalledPluginsPath()), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(cfg.InstalledPluginsPath(), data, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	got, found, err := InstalledPluginPath(cfg, EngramPluginID)
	if err != nil {
		t.Fatalf("InstalledPluginPath() error = %v", err)
	}
	if !found || got != path {
		t.Fatalf("InstalledPluginPath() = (%q, %v), want (%q, true)", got, found, path)
	}
}

// TestListClickPlugins_DistinguishesEnabledFromDisabled guards the FIX 4 contract: a managed
// plugin registered in installed_plugins.json but disabled in settings.json's enabledPlugins must
// report Installed==true, Enabled==false — so `click plugins` can never claim a plugin is active
// while `click doctor` (which requires enabledPlugins[id]==true) says it is not.
func TestListClickPlugins_DistinguishesEnabledFromDisabled(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir()}
	if err := os.MkdirAll(filepath.Join(cfg.ClaudeHome, "plugins"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	sddID := "click-sdd@" + marketplaceName
	memID := "click-memory@" + marketplaceName
	writeJSONForTest(t, cfg.InstalledPluginsPath(), map[string]any{"plugins": map[string]any{
		sddID: []map[string]any{{"installPath": "/cache/click-sdd"}},
		memID: []map[string]any{{"installPath": "/cache/click-memory"}},
	}})
	writeJSONForTest(t, cfg.SettingsPath(), map[string]any{"enabledPlugins": map[string]bool{
		sddID: true,
		memID: false,
	}})

	statuses, err := ListClickPlugins(cfg)
	if err != nil {
		t.Fatalf("ListClickPlugins() error = %v", err)
	}
	byName := map[string]ClickPluginStatus{}
	for _, s := range statuses {
		byName[s.Name] = s
	}
	if sdd := byName["click-sdd"]; !sdd.Installed || !sdd.Enabled {
		t.Fatalf("click-sdd = %+v, want Installed && Enabled", sdd)
	}
	if mem := byName["click-memory"]; !mem.Installed || mem.Enabled {
		t.Fatalf("click-memory = %+v, want Installed but NOT Enabled (registered but disabled)", mem)
	}
}

func writeJSONForTest(t *testing.T, path string, data any) {
	t.Helper()
	raw, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
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
