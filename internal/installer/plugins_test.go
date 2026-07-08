package installer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	clickmemory "github.com/Angel-MercadoCLK/click-ai-devkit/plugins/click-memory"
	clickreview "github.com/Angel-MercadoCLK/click-ai-devkit/plugins/click-review"
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

func TestCopyClickMemoryPlugin_CreatesExpectedFiles(t *testing.T) {
	claudeHome := t.TempDir()
	cfg := Config{ClaudeHome: claudeHome}

	if err := CopyClickMemoryPlugin(cfg); err != nil {
		t.Fatalf("CopyClickMemoryPlugin() error = %v", err)
	}

	wantPluginJSON, err := clickmemory.Files.ReadFile(".claude-plugin/plugin.json")
	if err != nil {
		t.Fatalf("read embedded plugin.json: %v", err)
	}
	wantPolicy, err := clickmemory.Files.ReadFile("docs/memory-policy.md")
	if err != nil {
		t.Fatalf("read embedded memory-policy.md: %v", err)
	}

	gotPluginJSON, err := os.ReadFile(filepath.Join(cfg.ClickMemoryPluginDir(), ".claude-plugin", "plugin.json"))
	if err != nil {
		t.Fatalf("CopyClickMemoryPlugin() did not create plugin.json: %v", err)
	}
	if string(gotPluginJSON) != string(wantPluginJSON) {
		t.Errorf("copied plugin.json = %q, want %q", gotPluginJSON, wantPluginJSON)
	}

	gotPolicy, err := os.ReadFile(filepath.Join(cfg.ClickMemoryPluginDir(), "docs", "memory-policy.md"))
	if err != nil {
		t.Fatalf("CopyClickMemoryPlugin() did not create memory-policy.md: %v", err)
	}
	if string(gotPolicy) != string(wantPolicy) {
		t.Errorf("copied memory-policy.md = %q, want %q", gotPolicy, wantPolicy)
	}
}

func TestClickMemoryPlugin_ManifestAndFilesAreStructurallyValid(t *testing.T) {
	type pluginManifest struct {
		Name        string `json:"name"`
		Version     string `json:"version"`
		Description string `json:"description"`
		Author      string `json:"author"`
	}

	data, err := clickmemory.Files.ReadFile(".claude-plugin/plugin.json")
	if err != nil {
		t.Fatalf("ReadFile(plugin.json) error = %v", err)
	}

	var manifest pluginManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("plugin.json parse error = %v", err)
	}
	if manifest.Name != "click-memory" {
		t.Fatalf("manifest.Name = %q, want click-memory", manifest.Name)
	}
	if manifest.Version == "" || manifest.Description == "" || manifest.Author == "" {
		t.Fatalf("manifest fields missing: %+v", manifest)
	}

	expectedFiles := []string{
		"skills/memory-proposal/SKILL.md",
		"skills/memory-review/SKILL.md",
		"docs/memory-policy.md",
		"docs/allowed-memory.md",
		"docs/forbidden-memory.md",
		"docs/engram-setup.md",
	}
	for _, name := range expectedFiles {
		content, err := clickmemory.Files.ReadFile(name)
		if err != nil {
			t.Fatalf("expected file %s missing: %v", name, err)
		}
		if len(strings.TrimSpace(string(content))) == 0 {
			t.Fatalf("expected file %s is empty", name)
		}
	}
}

func TestCopyClickMemoryPlugin_IdempotentOnSecondCall(t *testing.T) {
	claudeHome := t.TempDir()
	cfg := Config{ClaudeHome: claudeHome}

	if err := CopyClickMemoryPlugin(cfg); err != nil {
		t.Fatalf("first CopyClickMemoryPlugin() error = %v", err)
	}
	if err := CopyClickMemoryPlugin(cfg); err != nil {
		t.Fatalf("second CopyClickMemoryPlugin() error = %v", err)
	}

	entries, err := os.ReadDir(cfg.ClickMemoryPluginDir())
	if err != nil {
		t.Fatalf("ReadDir(%s) error = %v", cfg.ClickMemoryPluginDir(), err)
	}
	if len(entries) != 3 {
		t.Fatalf("ClickMemoryPluginDir() has %d entries after two installs, want exactly 3 (.claude-plugin, skills, docs)", len(entries))
	}
}

func TestRemoveClickMemoryPlugin_RemovesDirectory(t *testing.T) {
	claudeHome := t.TempDir()
	cfg := Config{ClaudeHome: claudeHome}

	if err := CopyClickMemoryPlugin(cfg); err != nil {
		t.Fatalf("CopyClickMemoryPlugin() error = %v", err)
	}
	if err := RemoveClickMemoryPlugin(cfg); err != nil {
		t.Fatalf("RemoveClickMemoryPlugin() error = %v", err)
	}

	if _, err := os.Stat(cfg.ClickMemoryPluginDir()); !os.IsNotExist(err) {
		t.Fatalf("RemoveClickMemoryPlugin() left %s behind", cfg.ClickMemoryPluginDir())
	}
}

func TestRemoveClickMemoryPlugin_NoopWhenAlreadyAbsent(t *testing.T) {
	claudeHome := t.TempDir()
	cfg := Config{ClaudeHome: claudeHome}

	if err := RemoveClickMemoryPlugin(cfg); err != nil {
		t.Fatalf("RemoveClickMemoryPlugin() on an absent plugin error = %v, want nil", err)
	}
}

func TestCopyClickReviewPlugin_CreatesExpectedFiles(t *testing.T) {
	claudeHome := t.TempDir()
	cfg := Config{ClaudeHome: claudeHome}

	if err := CopyClickReviewPlugin(cfg); err != nil {
		t.Fatalf("CopyClickReviewPlugin() error = %v", err)
	}

	wantPluginJSON, err := clickreview.Files.ReadFile(".claude-plugin/plugin.json")
	if err != nil {
		t.Fatalf("read embedded plugin.json: %v", err)
	}
	wantAgent, err := clickreview.Files.ReadFile("agents/click-pr-reviewer.md")
	if err != nil {
		t.Fatalf("read embedded click-pr-reviewer.md: %v", err)
	}

	gotPluginJSON, err := os.ReadFile(filepath.Join(cfg.ClickReviewPluginDir(), ".claude-plugin", "plugin.json"))
	if err != nil {
		t.Fatalf("CopyClickReviewPlugin() did not create plugin.json: %v", err)
	}
	if string(gotPluginJSON) != string(wantPluginJSON) {
		t.Errorf("copied plugin.json = %q, want %q", gotPluginJSON, wantPluginJSON)
	}

	gotAgent, err := os.ReadFile(filepath.Join(cfg.ClickReviewPluginDir(), "agents", "click-pr-reviewer.md"))
	if err != nil {
		t.Fatalf("CopyClickReviewPlugin() did not create click-pr-reviewer.md: %v", err)
	}
	if string(gotAgent) != string(wantAgent) {
		t.Errorf("copied click-pr-reviewer.md = %q, want %q", gotAgent, wantAgent)
	}
}

func TestClickReviewPlugin_ManifestAndFilesAreStructurallyValid(t *testing.T) {
	type pluginManifest struct {
		Name        string `json:"name"`
		Version     string `json:"version"`
		Description string `json:"description"`
		Author      string `json:"author"`
	}

	data, err := clickreview.Files.ReadFile(".claude-plugin/plugin.json")
	if err != nil {
		t.Fatalf("ReadFile(plugin.json) error = %v", err)
	}

	var manifest pluginManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("plugin.json parse error = %v", err)
	}
	if manifest.Name != "click-review" {
		t.Fatalf("manifest.Name = %q, want click-review", manifest.Name)
	}
	if manifest.Version == "" || manifest.Description == "" || manifest.Author == "" {
		t.Fatalf("manifest fields missing: %+v", manifest)
	}

	expectedFiles := []string{
		"agents/click-pr-reviewer.md",
		"skills/pr-review/SKILL.md",
		"skills/pre-merge-checklist/SKILL.md",
	}
	for _, name := range expectedFiles {
		content, err := clickreview.Files.ReadFile(name)
		if err != nil {
			t.Fatalf("expected file %s missing: %v", name, err)
		}
		if len(strings.TrimSpace(string(content))) == 0 {
			t.Fatalf("expected file %s is empty", name)
		}
	}
}

func TestCopyClickReviewPlugin_IdempotentOnSecondCall(t *testing.T) {
	claudeHome := t.TempDir()
	cfg := Config{ClaudeHome: claudeHome}

	if err := CopyClickReviewPlugin(cfg); err != nil {
		t.Fatalf("first CopyClickReviewPlugin() error = %v", err)
	}
	if err := CopyClickReviewPlugin(cfg); err != nil {
		t.Fatalf("second CopyClickReviewPlugin() error = %v", err)
	}

	entries, err := os.ReadDir(cfg.ClickReviewPluginDir())
	if err != nil {
		t.Fatalf("ReadDir(%s) error = %v", cfg.ClickReviewPluginDir(), err)
	}
	if len(entries) != 3 {
		t.Fatalf("ClickReviewPluginDir() has %d entries after two installs, want exactly 3 (.claude-plugin, agents, skills)", len(entries))
	}
}

func TestRemoveClickReviewPlugin_RemovesDirectory(t *testing.T) {
	claudeHome := t.TempDir()
	cfg := Config{ClaudeHome: claudeHome}

	if err := CopyClickReviewPlugin(cfg); err != nil {
		t.Fatalf("CopyClickReviewPlugin() error = %v", err)
	}
	if err := RemoveClickReviewPlugin(cfg); err != nil {
		t.Fatalf("RemoveClickReviewPlugin() error = %v", err)
	}

	if _, err := os.Stat(cfg.ClickReviewPluginDir()); !os.IsNotExist(err) {
		t.Fatalf("RemoveClickReviewPlugin() left %s behind", cfg.ClickReviewPluginDir())
	}
}

func TestRemoveClickReviewPlugin_NoopWhenAlreadyAbsent(t *testing.T) {
	claudeHome := t.TempDir()
	cfg := Config{ClaudeHome: claudeHome}

	if err := RemoveClickReviewPlugin(cfg); err != nil {
		t.Fatalf("RemoveClickReviewPlugin() on an absent plugin error = %v, want nil", err)
	}
}
