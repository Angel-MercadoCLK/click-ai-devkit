package agentbuilder

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/modelconfig"
)

func TestRenderAgentMarkdownGoldenOutput(t *testing.T) {
	spec := AgentSpec{
		Engine:      ClaudeCode,
		Name:        "release-helper",
		Description: "Helps prepare release notes.",
		SDDMode:     SDDPhaseSupport,
		Phase:       modelconfig.PhaseApply,
		Tools:       "Read, Grep, Edit",
		Model:       "sonnet",
		Purpose:     "Turn merged pull requests into release notes.",
		Tasks:       "Read merged PRs and group changes by user-facing area.",
		Triggers:    "Use when a release candidate is ready.",
		Rules:       "Never invent merged work.",
		Tone:        "Clear, direct, and concise.",
		Domain:      "Go CLI release management.",
		GoodOutput:  "A Markdown changelog grouped by area.",
		Placement:   PlacementPersonal,
	}

	got, err := RenderAgentMarkdown(spec)
	if err != nil {
		t.Fatalf("RenderAgentMarkdown() error = %v", err)
	}

	want := "---\n" +
		"name: \"release-helper\"\n" +
		"description: \"Helps prepare release notes.\"\n" +
		"model: \"sonnet\"\n" +
		"tools: \"Read, Grep, Edit\"\n" +
		"---\n\n" +
		"# Role\n" +
		"Turn merged pull requests into release notes.\n\n" +
		"## Tasks\n" +
		"Read merged PRs and group changes by user-facing area.\n\n" +
		"## Triggers\n" +
		"Use when a release candidate is ready.\n\n" +
		"## Hard Rules\n" +
		"Never invent merged work.\n\n" +
		"## SDD Integration\n" +
		"Mode: phase-support\n" +
		"Phase: apply\n\n" +
		"## Tone\n" +
		"Clear, direct, and concise.\n\n" +
		"## Domain Knowledge\n" +
		"Go CLI release management.\n\n" +
		"## Good Output\n" +
		"A Markdown changelog grouped by area.\n\n"

	if got != want {
		t.Fatalf("RenderAgentMarkdown() =\n%q\nwant\n%q", got, want)
	}
}

func TestRenderAgentMarkdownQuotesFrontmatterScalars(t *testing.T) {
	spec := validAgentSpec()
	spec.Description = `Release helper: drafts "notes" and C:\release paths`
	spec.Tools = `Read, Edit, Bash("git status")`
	spec.Model = `sonnet\fast`

	got, err := RenderAgentMarkdown(spec)
	if err != nil {
		t.Fatalf("RenderAgentMarkdown() error = %v", err)
	}

	wantLines := []string{
		"name: \"release-helper\"",
		"description: \"Release helper: drafts \\\"notes\\\" and C:\\\\release paths\"",
		"model: \"sonnet\\\\fast\"",
		"tools: \"Read, Edit, Bash(\\\"git status\\\")\"",
	}
	for _, want := range wantLines {
		if !strings.Contains(got, want) {
			t.Fatalf("RenderAgentMarkdown() missing %q in:\n%s", want, got)
		}
	}
}

func TestRenderAgentMarkdownRejectsFrontmatterNewlineInjection(t *testing.T) {
	spec := validAgentSpec()
	spec.Description = "safe\ntools: Bash(*)"

	if _, err := RenderAgentMarkdown(spec); err == nil {
		t.Fatal("RenderAgentMarkdown() error = nil, want non-nil for newline in frontmatter scalar")
	}
}

func TestTargetPathPersonalUsesClaudeConfigDirWhenSet(t *testing.T) {
	claudeConfigDir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", claudeConfigDir)
	spec := validAgentSpec()
	spec.Placement = PlacementPersonal

	got, err := TargetPath(spec, "", t.TempDir())
	if err != nil {
		t.Fatalf("TargetPath() error = %v", err)
	}

	wantPath(t, got, filepath.Join(claudeConfigDir, "agents", "release-helper.md"))
}

func TestTargetPathPersonalDefaultsToUserClaudeAgentsDir(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	spec := validAgentSpec()
	spec.Placement = PlacementPersonal
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("os.UserHomeDir() error = %v", err)
	}

	got, err := TargetPath(spec, "", t.TempDir())
	if err != nil {
		t.Fatalf("TargetPath() error = %v", err)
	}

	wantPath(t, got, filepath.Join(home, ".claude", "agents", "release-helper.md"))
}

func TestTargetPathShareableWithRegisteredClickSDDUsesClickSDDAgentsForPhaseSupport(t *testing.T) {
	repoRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repoRoot, ".claude-plugin"), 0o755); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, ".claude-plugin", "marketplace.json"), []byte(`{"plugins":[{"name":"click-sdd","source":"./plugins/click-sdd"}]}`), 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(repoRoot, "plugins", "click-sdd", ".claude-plugin"), 0o755); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, "plugins", "click-sdd", ".claude-plugin", "plugin.json"), []byte(`{"name":"click-sdd"}`), 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}
	spec := validAgentSpec()
	spec.SDDMode = SDDPhaseSupport
	spec.Placement = PlacementShareable

	got, err := TargetPath(spec, "", repoRoot)
	if err != nil {
		t.Fatalf("TargetPath() error = %v", err)
	}

	wantPath(t, got, filepath.Join(repoRoot, "plugins", "click-sdd", "agents", "release-helper.md"))
}

func TestTargetPathShareableWithUnregisteredClickSDDReturnsScaffoldPath(t *testing.T) {
	repoRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repoRoot, ".claude-plugin"), 0o755); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, ".claude-plugin", "marketplace.json"), []byte(`{"plugins":[]}`), 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}
	spec := validAgentSpec()
	spec.SDDMode = SDDPhaseSupport
	spec.Placement = PlacementShareable

	got, err := TargetPath(spec, "", repoRoot)
	if err != nil {
		t.Fatalf("TargetPath() error = %v", err)
	}

	wantPath(t, got, filepath.Join(repoRoot, "plugins", "click-release-helper", "agents", "release-helper.md"))
}

func TestTargetPathShareableWithoutMarketplaceReturnsScaffoldPath(t *testing.T) {
	repoRoot := t.TempDir()
	spec := validAgentSpec()
	spec.Placement = PlacementShareable

	got, err := TargetPath(spec, "", repoRoot)
	if err != nil {
		t.Fatalf("TargetPath() error = %v", err)
	}

	wantPath(t, got, filepath.Join(repoRoot, "plugins", "click-release-helper", "agents", "release-helper.md"))
}

func TestInstallWritesRenderedAgentWithInjectedFileWriter(t *testing.T) {
	claudeHome := filepath.Join("testdata", "claude-home")
	spec := validAgentSpec()
	spec.Placement = PlacementPersonal
	writer := &fakeFileWriter{}

	gotPath, err := Install(spec, claudeHome, "", writer)
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	want := filepath.Join(claudeHome, "agents", "release-helper.md")
	wantPath(t, gotPath, want)
	wantPath(t, writer.mkdirPath, filepath.Dir(want))
	if writer.mkdirPerm != 0o755 {
		t.Fatalf("MkdirAll perm = %#o, want 0755", writer.mkdirPerm)
	}
	wantPath(t, writer.writePath, want)
	if writer.writePerm != 0o600 {
		t.Fatalf("WriteFile perm = %#o, want 0600", writer.writePerm)
	}
	wantContent, err := RenderAgentMarkdown(spec)
	if err != nil {
		t.Fatalf("RenderAgentMarkdown() error = %v", err)
	}
	if string(writer.writeData) != wantContent {
		t.Fatalf("written data =\n%q\nwant\n%q", string(writer.writeData), wantContent)
	}
}

func TestInstallReturnsWriteErrorsFromInjectedFileWriter(t *testing.T) {
	spec := validAgentSpec()
	spec.Placement = PlacementPersonal
	writer := &fakeFileWriter{writeErr: errors.New("disk full")}

	if _, err := Install(spec, filepath.Join("testdata", "claude-home"), "", writer); err == nil {
		t.Fatal("Install() error = nil, want non-nil when injected writer fails")
	}
}

func TestInstallShareableStandaloneScaffoldsLoadablePlugin(t *testing.T) {
	repoRoot := filepath.Join("testdata", "repo")
	spec := validAgentSpec()
	spec.Description = "Release helper: drafts notes"
	spec.Placement = PlacementShareable
	writer := newFakeFileWriter()
	writer.files[filepath.Join(repoRoot, ".claude-plugin", "marketplace.json")] = []byte(`{"plugins":[]}`)

	gotPath, err := Install(spec, "", repoRoot, writer)
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	pluginName := "click-release-helper"
	agentPath := filepath.Join(repoRoot, "plugins", pluginName, "agents", "release-helper.md")
	pluginManifestPath := filepath.Join(repoRoot, "plugins", pluginName, ".claude-plugin", "plugin.json")
	marketplacePath := filepath.Join(repoRoot, ".claude-plugin", "marketplace.json")
	wantPath(t, gotPath, agentPath)

	for _, path := range []string{agentPath, pluginManifestPath, marketplacePath} {
		if _, ok := writer.files[path]; !ok {
			t.Fatalf("Install() did not write %s; writes=%v", path, writer.writePaths)
		}
	}

	var pluginManifest struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Version     string `json:"version"`
		Author      struct {
			Name string `json:"name"`
		} `json:"author"`
	}
	if err := json.Unmarshal(writer.files[pluginManifestPath], &pluginManifest); err != nil {
		t.Fatalf("plugin.json parse error = %v\n%s", err, writer.files[pluginManifestPath])
	}
	if pluginManifest.Name != pluginName || pluginManifest.Description != spec.Description || pluginManifest.Version == "" || pluginManifest.Author.Name == "" {
		t.Fatalf("plugin manifest = %+v, want loadable manifest for %s", pluginManifest, pluginName)
	}

	var marketplace struct {
		Plugins []struct {
			Name   string `json:"name"`
			Source string `json:"source"`
		} `json:"plugins"`
	}
	if err := json.Unmarshal(writer.files[marketplacePath], &marketplace); err != nil {
		t.Fatalf("marketplace.json parse error = %v\n%s", err, writer.files[marketplacePath])
	}
	if len(marketplace.Plugins) != 1 || marketplace.Plugins[0].Name != pluginName || marketplace.Plugins[0].Source != "./plugins/click-release-helper" {
		t.Fatalf("marketplace plugins = %+v, want registration for %s", marketplace.Plugins, pluginName)
	}
}

func TestInstallShareableStandaloneCreatesMarketplaceWhenMissing(t *testing.T) {
	repoRoot := filepath.Join("testdata", "repo")
	spec := validAgentSpec()
	spec.Placement = PlacementShareable
	writer := newFakeFileWriter()

	if _, err := Install(spec, "", repoRoot, writer); err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	marketplacePath := filepath.Join(repoRoot, ".claude-plugin", "marketplace.json")
	if _, ok := writer.files[marketplacePath]; !ok {
		t.Fatalf("Install() did not create marketplace.json; writes=%v", writer.writePaths)
	}
}

func TestInstallShareablePhaseSupportWithoutMarketplaceScaffoldsLoadablePlugin(t *testing.T) {
	repoRoot := filepath.Join("testdata", "repo")
	spec := validAgentSpec()
	spec.SDDMode = SDDPhaseSupport
	spec.Phase = modelconfig.PhaseApply
	spec.Placement = PlacementShareable
	writer := newFakeFileWriter()

	gotPath, err := Install(spec, "", repoRoot, writer)
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	pluginName := "click-release-helper"
	agentPath := filepath.Join(repoRoot, "plugins", pluginName, "agents", "release-helper.md")
	pluginManifestPath := filepath.Join(repoRoot, "plugins", pluginName, ".claude-plugin", "plugin.json")
	marketplacePath := filepath.Join(repoRoot, ".claude-plugin", "marketplace.json")
	wantPath(t, gotPath, agentPath)

	for _, path := range []string{agentPath, pluginManifestPath, marketplacePath} {
		if _, ok := writer.files[path]; !ok {
			t.Fatalf("Install() did not write %s; writes=%v", path, writer.writePaths)
		}
	}

	var marketplace struct {
		Plugins []struct {
			Name   string `json:"name"`
			Source string `json:"source"`
		} `json:"plugins"`
	}
	if err := json.Unmarshal(writer.files[marketplacePath], &marketplace); err != nil {
		t.Fatalf("marketplace.json parse error = %v\n%s", err, writer.files[marketplacePath])
	}
	if len(marketplace.Plugins) != 1 || marketplace.Plugins[0].Name != pluginName || marketplace.Plugins[0].Source != "./plugins/click-release-helper" {
		t.Fatalf("marketplace plugins = %+v, want registration for %s", marketplace.Plugins, pluginName)
	}
}

func TestInstallShareablePhaseSupportWithUnregisteredClickSDDScaffoldsLoadablePlugin(t *testing.T) {
	repoRoot := filepath.Join("testdata", "repo")
	spec := validAgentSpec()
	spec.SDDMode = SDDPhaseSupport
	spec.Phase = modelconfig.PhaseApply
	spec.Placement = PlacementShareable
	writer := newFakeFileWriter()
	writer.files[filepath.Join(repoRoot, ".claude-plugin", "marketplace.json")] = []byte(`{"plugins":[]}`)

	gotPath, err := Install(spec, "", repoRoot, writer)
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	pluginName := "click-release-helper"
	agentPath := filepath.Join(repoRoot, "plugins", pluginName, "agents", "release-helper.md")
	pluginManifestPath := filepath.Join(repoRoot, "plugins", pluginName, ".claude-plugin", "plugin.json")
	marketplacePath := filepath.Join(repoRoot, ".claude-plugin", "marketplace.json")
	wantPath(t, gotPath, agentPath)

	for _, path := range []string{agentPath, pluginManifestPath, marketplacePath} {
		if _, ok := writer.files[path]; !ok {
			t.Fatalf("Install() did not write %s; writes=%v", path, writer.writePaths)
		}
	}
}

func TestInstallShareablePhaseSupportWithMissingClickSDDManifestScaffoldsLoadablePlugin(t *testing.T) {
	repoRoot := filepath.Join("testdata", "repo")
	spec := validAgentSpec()
	spec.SDDMode = SDDPhaseSupport
	spec.Phase = modelconfig.PhaseApply
	spec.Placement = PlacementShareable
	writer := newFakeFileWriter()
	writer.files[filepath.Join(repoRoot, ".claude-plugin", "marketplace.json")] = []byte(`{"plugins":[{"name":"click-sdd","source":"./plugins/click-sdd"}]}`)

	gotPath, err := Install(spec, "", repoRoot, writer)
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	pluginName := "click-release-helper"
	agentPath := filepath.Join(repoRoot, "plugins", pluginName, "agents", "release-helper.md")
	pluginManifestPath := filepath.Join(repoRoot, "plugins", pluginName, ".claude-plugin", "plugin.json")
	wantPath(t, gotPath, agentPath)
	if _, ok := writer.files[pluginManifestPath]; !ok {
		t.Fatalf("Install() did not write standalone plugin manifest; writes=%v", writer.writePaths)
	}
}

func TestInstallShareablePhaseSupportWithRegisteredClickSDDUsesClickSDDAgents(t *testing.T) {
	repoRoot := filepath.Join("testdata", "repo")
	spec := validAgentSpec()
	spec.SDDMode = SDDPhaseSupport
	spec.Phase = modelconfig.PhaseApply
	spec.Placement = PlacementShareable
	writer := newFakeFileWriter()
	writer.files[filepath.Join(repoRoot, ".claude-plugin", "marketplace.json")] = []byte(`{"plugins":[{"name":"click-sdd","source":"./plugins/click-sdd"}]}`)
	writer.files[filepath.Join(repoRoot, "plugins", "click-sdd", ".claude-plugin", "plugin.json")] = []byte(`{"name":"click-sdd"}`)

	gotPath, err := Install(spec, "", repoRoot, writer)
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	wantPath(t, gotPath, filepath.Join(repoRoot, "plugins", "click-sdd", "agents", "release-helper.md"))
	if _, ok := writer.files[filepath.Join(repoRoot, "plugins", "click-release-helper", ".claude-plugin", "plugin.json")]; ok {
		t.Fatal("Install() scaffolded a standalone plugin even though click-sdd is registered and loadable")
	}
}

func validAgentSpec() AgentSpec {
	return AgentSpec{
		Engine:      ClaudeCode,
		Name:        "release-helper",
		Description: "Helps prepare release notes.",
		SDDMode:     SDDStandalone,
		Tools:       "Read, Grep",
		Model:       "sonnet",
		Purpose:     "Turn merged pull requests into release notes.",
		Tasks:       "Read merged PRs and group changes by user-facing area.",
		Triggers:    "Use when a release candidate is ready.",
		Rules:       "Never invent merged work.",
		Tone:        "Clear, direct, and concise.",
		Domain:      "Go CLI release management.",
		GoodOutput:  "A Markdown changelog grouped by area.",
		Placement:   PlacementPersonal,
	}
}

func newFakeFileWriter() *fakeFileWriter {
	return &fakeFileWriter{files: map[string][]byte{}}
}

func wantPath(t *testing.T, got, want string) {
	t.Helper()
	if got != filepath.Clean(want) {
		t.Fatalf("path = %q, want %q", got, filepath.Clean(want))
	}
}

type fakeFileWriter struct {
	mkdirPath  string
	mkdirPerm  os.FileMode
	mkdirErr   error
	writePath  string
	writeData  []byte
	writePerm  os.FileMode
	writeErr   error
	writePaths []string
	files      map[string][]byte
}

func (w *fakeFileWriter) MkdirAll(path string, perm os.FileMode) error {
	w.mkdirPath = path
	w.mkdirPerm = perm
	return w.mkdirErr
}

func (w *fakeFileWriter) WriteFile(path string, data []byte, perm os.FileMode) error {
	w.writePath = path
	w.writeData = append([]byte(nil), data...)
	w.writePerm = perm
	w.writePaths = append(w.writePaths, path)
	if w.files != nil {
		w.files[path] = append([]byte(nil), data...)
	}
	return w.writeErr
}

func (w *fakeFileWriter) Stat(string) (os.FileInfo, error) {
	return nil, errors.New("fake stat not configured")
}

func (w *fakeFileWriter) ReadFile(path string) ([]byte, error) {
	if w.files != nil {
		if data, ok := w.files[path]; ok {
			return append([]byte(nil), data...), nil
		}
		return nil, os.ErrNotExist
	}
	return nil, errors.New("fake stat not configured")
}
