package installer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	clicksdd "github.com/Angel-MercadoCLK/click-ai-devkit/plugins/click-sdd"
)

func TestCopyClickSDDPlugin_CreatesExpectedFiles(t *testing.T) {
	claudeHome := t.TempDir()
	cfg := Config{ClaudeHome: claudeHome}

	if err := CopyClickSDDPlugin(cfg); err != nil {
		t.Fatalf("CopyClickSDDPlugin() error = %v", err)
	}

	wantPluginJSON, err := clicksdd.Files.ReadFile(".claude-plugin/plugin.json")
	if err != nil {
		t.Fatalf("read embedded plugin.json: %v", err)
	}
	wantOrchestrator, err := clicksdd.Files.ReadFile("agents/click-orchestrator.md")
	if err != nil {
		t.Fatalf("read embedded click-orchestrator.md: %v", err)
	}

	gotPluginJSON, err := os.ReadFile(filepath.Join(cfg.ClickSDDPluginDir(), ".claude-plugin", "plugin.json"))
	if err != nil {
		t.Fatalf("CopyClickSDDPlugin() did not create plugin.json: %v", err)
	}
	if string(gotPluginJSON) != string(wantPluginJSON) {
		t.Errorf("copied plugin.json = %q, want %q", gotPluginJSON, wantPluginJSON)
	}

	gotOrchestrator, err := os.ReadFile(filepath.Join(cfg.ClickSDDPluginDir(), "agents", "click-orchestrator.md"))
	if err != nil {
		t.Fatalf("CopyClickSDDPlugin() did not create click-orchestrator.md: %v", err)
	}
	if string(gotOrchestrator) != string(wantOrchestrator) {
		t.Errorf("copied click-orchestrator.md = %q, want %q", gotOrchestrator, wantOrchestrator)
	}
}

func TestClickSDDPlugin_ManifestAndFilesAreStructurallyValid(t *testing.T) {
	type pluginManifest struct {
		Name        string `json:"name"`
		Version     string `json:"version"`
		Description string `json:"description"`
		Author      string `json:"author"`
	}

	data, err := clicksdd.Files.ReadFile(".claude-plugin/plugin.json")
	if err != nil {
		t.Fatalf("ReadFile(plugin.json) error = %v", err)
	}

	var manifest pluginManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("plugin.json parse error = %v", err)
	}
	if manifest.Name != "click-sdd" {
		t.Fatalf("manifest.Name = %q, want click-sdd", manifest.Name)
	}
	if manifest.Version == "" || manifest.Description == "" || manifest.Author == "" {
		t.Fatalf("manifest fields missing: %+v", manifest)
	}

	expectedFiles := []string{
		"agents/click-orchestrator.md",
		"agents/click-prd-writer.md",
		"agents/click-architect.md",
		"agents/click-reviewer.md",
		"agents/click-memory-curator.md",
		"skills/sdd-explore/SKILL.md",
		"skills/sdd-prd/SKILL.md",
		"skills/sdd-design/SKILL.md",
		"skills/sdd-tasks/SKILL.md",
		"skills/sdd-code/SKILL.md",
		"skills/sdd-review/SKILL.md",
	}
	for _, name := range expectedFiles {
		content, err := clicksdd.Files.ReadFile(name)
		if err != nil {
			t.Fatalf("expected file %s missing: %v", name, err)
		}
		if len(strings.TrimSpace(string(content))) == 0 {
			t.Fatalf("expected file %s is empty", name)
		}
	}
}

func TestCopyClickSDDPlugin_IdempotentOnSecondCall(t *testing.T) {
	claudeHome := t.TempDir()
	cfg := Config{ClaudeHome: claudeHome}

	if err := CopyClickSDDPlugin(cfg); err != nil {
		t.Fatalf("first CopyClickSDDPlugin() error = %v", err)
	}
	if err := CopyClickSDDPlugin(cfg); err != nil {
		t.Fatalf("second CopyClickSDDPlugin() error = %v", err)
	}

	entries, err := os.ReadDir(cfg.ClickSDDPluginDir())
	if err != nil {
		t.Fatalf("ReadDir(%s) error = %v", cfg.ClickSDDPluginDir(), err)
	}
	if len(entries) != 3 {
		t.Fatalf("ClickSDDPluginDir() has %d entries after two installs, want exactly 3 (.claude-plugin, agents, skills)", len(entries))
	}
}

func TestRemoveClickSDDPlugin_RemovesDirectory(t *testing.T) {
	claudeHome := t.TempDir()
	cfg := Config{ClaudeHome: claudeHome}

	if err := CopyClickSDDPlugin(cfg); err != nil {
		t.Fatalf("CopyClickSDDPlugin() error = %v", err)
	}
	if err := RemoveClickSDDPlugin(cfg); err != nil {
		t.Fatalf("RemoveClickSDDPlugin() error = %v", err)
	}

	if _, err := os.Stat(cfg.ClickSDDPluginDir()); !os.IsNotExist(err) {
		t.Fatalf("RemoveClickSDDPlugin() left %s behind", cfg.ClickSDDPluginDir())
	}
}

func TestRemoveClickSDDPlugin_NoopWhenAlreadyAbsent(t *testing.T) {
	claudeHome := t.TempDir()
	cfg := Config{ClaudeHome: claudeHome}

	if err := RemoveClickSDDPlugin(cfg); err != nil {
		t.Fatalf("RemoveClickSDDPlugin() on an absent plugin error = %v, want nil", err)
	}
}
