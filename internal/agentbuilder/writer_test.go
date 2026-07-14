package agentbuilder

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

func TestRenderAgentMarkdownRejectsBlankRequiredFrontmatterFields(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*AgentSpec)
	}{
		{
			name: "description",
			mutate: func(spec *AgentSpec) {
				spec.Description = " \t "
			},
		},
		{
			name: "model",
			mutate: func(spec *AgentSpec) {
				spec.Model = ""
			},
		},
		{
			name: "tools",
			mutate: func(spec *AgentSpec) {
				spec.Tools = "\t"
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := validAgentSpec()
			tt.mutate(&spec)

			if _, err := RenderAgentMarkdown(spec); err == nil {
				t.Fatalf("RenderAgentMarkdown() error = nil, want non-nil for blank %s", tt.name)
			}
		})
	}
}

func TestInstallRejectsBlankRequiredDescriptionBeforeWriting(t *testing.T) {
	spec := validAgentSpec()
	spec.Description = "   "
	spec.Placement = PlacementShareable
	writer := newFakeFileWriter()

	if _, err := Install(spec, "", filepath.Join("testdata", "repo"), writer); err == nil {
		t.Fatal("Install() error = nil, want non-nil for blank description")
	}
	if len(writer.writePaths) != 0 {
		t.Fatalf("Install() writes = %v, want none for invalid spec", writer.writePaths)
	}
}

func TestTargetPathPersonalUsesClaudeConfigDirWhenSet(t *testing.T) {
	claudeConfigDir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", claudeConfigDir)
	t.Setenv("CLICK_CLAUDE_HOME", t.TempDir())
	spec := validAgentSpec()
	spec.Placement = PlacementPersonal

	got, err := TargetPath(spec, "", t.TempDir())
	if err != nil {
		t.Fatalf("TargetPath() error = %v", err)
	}

	wantPath(t, got, filepath.Join(claudeConfigDir, "agents", "release-helper.md"))
}

func TestTargetPathPersonalUsesClickClaudeHomeWhenClaudeConfigDirUnset(t *testing.T) {
	clickClaudeHome := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	t.Setenv("CLICK_CLAUDE_HOME", clickClaudeHome)
	spec := validAgentSpec()
	spec.Placement = PlacementPersonal

	got, err := TargetPath(spec, "", t.TempDir())
	if err != nil {
		t.Fatalf("TargetPath() error = %v", err)
	}

	wantPath(t, got, filepath.Join(clickClaudeHome, "agents", "release-helper.md"))
}

func TestTargetPathPersonalDefaultsToUserClaudeAgentsDir(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	t.Setenv("CLICK_CLAUDE_HOME", "")
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
	if err := os.WriteFile(filepath.Join(repoRoot, "plugins", "click-sdd", ".claude-plugin", "plugin.json"), validClickSDDPluginManifest(), 0o600); err != nil {
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
	writer := newFakeFileWriter()

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

func TestInstallFinalMarkdownWritesConfirmedMarkdownExactly(t *testing.T) {
	tests := []struct {
		name          string
		placement     Placement
		claudeHome    string
		repoRoot      string
		wantPath      string
		wantScaffold  bool
		seedFiles     map[string][]byte
		finalMarkdown string
	}{
		{
			name:       "personal placement",
			placement:  PlacementPersonal,
			claudeHome: filepath.Join("testdata", "claude-home"),
			repoRoot:   "",
			wantPath:   filepath.Join("testdata", "claude-home", "agents", "release-helper.md"),
			finalMarkdown: "---\n" +
				"name: \"release-helper\"\n" +
				"description: \"Confirmed edited markdown\"\n" +
				"model: \"opus\"\n" +
				"tools: \"Read, Edit\"\n" +
				"---\n\n" +
				"# Role\nPersist the confirmed preview verbatim.\n\n" +
				"## Tasks\nConfirmed tasks.\n\n" +
				"## Triggers\nConfirmed triggers.\n\n" +
				"## Hard Rules\nConfirmed hard rules.\n\n" +
				"## SDD Integration\nMode: standalone\n\n" +
				"## Tone\nConfirmed tone.\n\n" +
				"## Domain Knowledge\nConfirmed domain knowledge.\n\n" +
				"## Good Output\nConfirmed good output.\n",
		},
		{
			name:         "shareable standalone scaffolding",
			placement:    PlacementShareable,
			repoRoot:     filepath.Join("testdata", "repo"),
			wantPath:     filepath.Join("testdata", "repo", "plugins", "click-release-helper", "agents", "release-helper.md"),
			wantScaffold: true,
			seedFiles: map[string][]byte{
				filepath.Join("testdata", "repo", ".claude-plugin", "marketplace.json"): []byte(`{"plugins":[]}`),
			},
			finalMarkdown: "---\n" +
				"name: \"release-helper\"\n" +
				"description: \"Shareable edited markdown\"\n" +
				"model: \"haiku\"\n" +
				"tools: \"Read, Grep, Bash\"\n" +
				"---\n\n" +
				"# Role\nInstall through the existing shareable scaffold path.\n\n" +
				"## Tasks\nConfirmed tasks.\n\n" +
				"## Triggers\nConfirmed triggers.\n\n" +
				"## Hard Rules\nConfirmed hard rules.\n\n" +
				"## SDD Integration\nMode: standalone\n\n" +
				"## Tone\nConfirmed tone.\n\n" +
				"## Domain Knowledge\nConfirmed domain knowledge.\n\n" +
				"## Good Output\nConfirmed good output.\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := validAgentSpec()
			spec.Placement = tt.placement
			// Make the spec disagree with the confirmed markdown. If the installer re-renders from
			// AgentSpec, this assertion catches it byte-for-byte.
			spec.Model = "sonnet"
			spec.Tools = "Read, Grep"
			writer := newFakeFileWriter()
			for path, data := range tt.seedFiles {
				writer.files[path] = data
			}

			gotPath, err := InstallFinalMarkdown(spec, tt.finalMarkdown, tt.claudeHome, tt.repoRoot, writer)
			if err != nil {
				t.Fatalf("InstallFinalMarkdown() error = %v", err)
			}

			wantPath(t, gotPath, tt.wantPath)
			if string(writer.files[tt.wantPath]) != tt.finalMarkdown {
				t.Fatalf("written markdown =\n%q\nwant exact confirmed markdown\n%q", string(writer.files[tt.wantPath]), tt.finalMarkdown)
			}
			if tt.wantScaffold {
				pluginManifestPath := filepath.Join(tt.repoRoot, "plugins", "click-release-helper", ".claude-plugin", "plugin.json")
				marketplacePath := filepath.Join(tt.repoRoot, ".claude-plugin", "marketplace.json")
				for _, path := range []string{pluginManifestPath, marketplacePath} {
					if _, ok := writer.files[path]; !ok {
						t.Fatalf("InstallFinalMarkdown() did not preserve scaffold write %s; writes=%v", path, writer.writePaths)
					}
				}
			}
		})
	}
}

func TestInstallFinalMarkdownRejectsExistingTargetAgentWithoutOverwrite(t *testing.T) {
	tests := []struct {
		name       string
		placement  Placement
		claudeHome string
		repoRoot   string
		targetPath string
		seedFiles  map[string][]byte
	}{
		{
			name:       "personal placement",
			placement:  PlacementPersonal,
			claudeHome: filepath.Join("testdata", "claude-home"),
			targetPath: filepath.Join("testdata", "claude-home", "agents", "release-helper.md"),
		},
		{
			name:       "shareable standalone placement",
			placement:  PlacementShareable,
			repoRoot:   filepath.Join("testdata", "repo"),
			targetPath: filepath.Join("testdata", "repo", "plugins", "click-release-helper", "agents", "release-helper.md"),
			seedFiles: map[string][]byte{
				filepath.Join("testdata", "repo", ".claude-plugin", "marketplace.json"): []byte(`{"plugins":[]}`),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := validAgentSpec()
			spec.Placement = tt.placement
			writer := newFakeFileWriter()
			for path, data := range tt.seedFiles {
				writer.files[path] = data
			}
			existingAgent := []byte("existing personal agent; do not truncate")
			writer.files[tt.targetPath] = existingAgent

			_, err := InstallFinalMarkdown(spec, "---\nname: release-helper\n---\n", tt.claudeHome, tt.repoRoot, writer)
			if err == nil {
				t.Fatal("InstallFinalMarkdown() error = nil, want existing target error")
			}
			if !strings.Contains(err.Error(), "already exists") || !strings.Contains(err.Error(), tt.targetPath) {
				t.Fatalf("InstallFinalMarkdown() error = %q, want clear existing target path error", err)
			}
			if string(writer.files[tt.targetPath]) != string(existingAgent) {
				t.Fatalf("existing agent was overwritten:\n%s", writer.files[tt.targetPath])
			}
			for _, path := range writer.writePaths {
				if path == tt.targetPath {
					t.Fatalf("InstallFinalMarkdown() wrote existing target path %s; writes=%v", path, writer.writePaths)
				}
			}
		})
	}
}

// R1-001/R2-005 regression coverage: InstallFinalMarkdown previously performed zero
// frontmatter validation itself, relying entirely on its one caller (the interactive
// wizard) to validate first. Any future caller (batch import, a non-interactive path)
// would have inherited an unguarded write. installContent now calls
// ValidateFinalMarkdown internally, so this guard holds regardless of caller.
func TestInstallFinalMarkdownRejectsInvalidContentWithoutWriting(t *testing.T) {
	spec := validAgentSpec()
	spec.Placement = PlacementPersonal
	writer := newFakeFileWriter()

	_, err := InstallFinalMarkdown(spec, "---\nname: \"release-helper\"\ndescription: \"x\"\nmodel: \"sonnet\"\ntools: \"Read\"\n---\n\n# Role\nmissing every other required section\n", "", "", writer)
	if err == nil {
		t.Fatal("InstallFinalMarkdown() error = nil, want validation error for markdown missing required sections")
	}
	if len(writer.writePaths) != 0 {
		t.Fatalf("InstallFinalMarkdown() writes = %v, want none for invalid final markdown", writer.writePaths)
	}
}

func TestInstallReturnsWriteErrorsFromInjectedFileWriter(t *testing.T) {
	spec := validAgentSpec()
	spec.Placement = PlacementPersonal
	writer := newFakeFileWriter()
	writer.writeErr = errors.New("disk full")

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

func TestInstallShareableStandaloneRejectsExistingPluginMetadataCollision(t *testing.T) {
	repoRoot := filepath.Join("testdata", "repo")
	spec := validAgentSpec()
	spec.Name = "review"
	spec.Description = "Generated review agent."
	spec.Placement = PlacementShareable
	writer := newFakeFileWriter()

	pluginManifestPath := filepath.Join(repoRoot, "plugins", "click-review", ".claude-plugin", "plugin.json")
	marketplacePath := filepath.Join(repoRoot, ".claude-plugin", "marketplace.json")
	existingPluginManifest := []byte(`{"name":"click-review","version":"9.9.9","description":"Existing review plugin","author":{"name":"Local User"},"userConfig":{"preserve":true}}`)
	existingMarketplace := []byte(`{"plugins":[{"name":"click-review","description":"Existing review marketplace entry","version":"9.9.9","author":{"name":"Local User"},"source":"./plugins/click-review","category":"custom","homepage":"https://example.test/review"}]}`)
	writer.files[pluginManifestPath] = existingPluginManifest
	writer.files[marketplacePath] = existingMarketplace

	_, err := Install(spec, "", repoRoot, writer)
	if err == nil {
		t.Fatal("Install() error = nil, want collision error for existing click-review plugin metadata")
	}
	if !strings.Contains(err.Error(), "click-review") || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("Install() error = %q, want actionable click-review collision error", err)
	}
	if string(writer.files[pluginManifestPath]) != string(existingPluginManifest) {
		t.Fatalf("plugin manifest was overwritten:\n%s", writer.files[pluginManifestPath])
	}
	if string(writer.files[marketplacePath]) != string(existingMarketplace) {
		t.Fatalf("marketplace manifest was overwritten:\n%s", writer.files[marketplacePath])
	}
	for _, path := range writer.writePaths {
		if path == pluginManifestPath || path == marketplacePath {
			t.Fatalf("Install() wrote colliding metadata path %s; writes=%v", path, writer.writePaths)
		}
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

func TestInstallShareablePhaseSupportWithWrongClickSDDSourceScaffoldsLoadablePlugin(t *testing.T) {
	repoRoot := filepath.Join("testdata", "repo")
	spec := validAgentSpec()
	spec.SDDMode = SDDPhaseSupport
	spec.Phase = modelconfig.PhaseApply
	spec.Placement = PlacementShareable
	writer := newFakeFileWriter()
	writer.files[filepath.Join(repoRoot, ".claude-plugin", "marketplace.json")] = []byte(`{"plugins":[{"name":"click-sdd","source":"./plugins/not-click-sdd"}]}`)
	writer.files[filepath.Join(repoRoot, "plugins", "click-sdd", ".claude-plugin", "plugin.json")] = validClickSDDPluginManifest()

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

func TestInstallShareablePhaseSupportWithMalformedClickSDDManifestScaffoldsLoadablePlugin(t *testing.T) {
	repoRoot := filepath.Join("testdata", "repo")
	spec := validAgentSpec()
	spec.SDDMode = SDDPhaseSupport
	spec.Phase = modelconfig.PhaseApply
	spec.Placement = PlacementShareable
	writer := newFakeFileWriter()
	writer.files[filepath.Join(repoRoot, ".claude-plugin", "marketplace.json")] = []byte(`{"plugins":[{"name":"click-sdd","source":"./plugins/click-sdd"}]}`)
	writer.files[filepath.Join(repoRoot, "plugins", "click-sdd", ".claude-plugin", "plugin.json")] = []byte(`{"name":"click-sdd"`)

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
	writer.files[filepath.Join(repoRoot, "plugins", "click-sdd", ".claude-plugin", "plugin.json")] = validClickSDDPluginManifest()

	gotPath, err := Install(spec, "", repoRoot, writer)
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	wantPath(t, gotPath, filepath.Join(repoRoot, "plugins", "click-sdd", "agents", "release-helper.md"))
	if _, ok := writer.files[filepath.Join(repoRoot, "plugins", "click-release-helper", ".claude-plugin", "plugin.json")]; ok {
		t.Fatal("Install() scaffolded a standalone plugin even though click-sdd is registered and loadable")
	}
}

// R4-001 regression coverage: rewriting an existing marketplace.json during a shareable
// install must never silently drop fields outside the narrow known-field subset, and must
// tolerate object-form `source` values on unrelated existing entries.
func TestScaffoldShareablePluginPreservesUnknownTopLevelMarketplaceFields(t *testing.T) {
	repoRoot := filepath.Join("testdata", "repo")
	spec := validAgentSpec()
	spec.Placement = PlacementShareable
	writer := newFakeFileWriter()
	writer.files[filepath.Join(repoRoot, ".claude-plugin", "marketplace.json")] = []byte(
		`{"$schema":"https://anthropic.com/claude-code/marketplace.schema.json","name":"click-ai-devkit","strict":true,"metadata":{"maintainer":"click"},"plugins":[]}`,
	)

	if _, err := Install(spec, "", repoRoot, writer); err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	marketplacePath := filepath.Join(repoRoot, ".claude-plugin", "marketplace.json")
	var out map[string]json.RawMessage
	if err := json.Unmarshal(writer.files[marketplacePath], &out); err != nil {
		t.Fatalf("marketplace.json parse error = %v\n%s", err, writer.files[marketplacePath])
	}
	if strictRaw, ok := out["strict"]; !ok || string(strictRaw) != "true" {
		t.Fatalf("marketplace.json dropped or mutated top-level \"strict\" field: %s", writer.files[marketplacePath])
	}
	metadataRaw, ok := out["metadata"]
	if !ok {
		t.Fatalf("marketplace.json dropped top-level \"metadata\" field: %s", writer.files[marketplacePath])
	}
	var metadata map[string]string
	if err := json.Unmarshal(metadataRaw, &metadata); err != nil || metadata["maintainer"] != "click" {
		t.Fatalf("marketplace.json metadata = %s, want {\"maintainer\":\"click\"}", metadataRaw)
	}
}

func TestScaffoldShareablePluginPreservesUnknownPerPluginFields(t *testing.T) {
	repoRoot := filepath.Join("testdata", "repo")
	spec := validAgentSpec()
	spec.Placement = PlacementShareable
	writer := newFakeFileWriter()
	writer.files[filepath.Join(repoRoot, ".claude-plugin", "marketplace.json")] = []byte(
		`{"plugins":[{"name":"click-existing","description":"Existing plugin","version":"1.0.0","author":{"name":"Someone"},"source":"./plugins/click-existing","keywords":["db","review"]}]}`,
	)

	if _, err := Install(spec, "", repoRoot, writer); err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	marketplacePath := filepath.Join(repoRoot, ".claude-plugin", "marketplace.json")
	var out struct {
		Plugins []map[string]json.RawMessage `json:"plugins"`
	}
	if err := json.Unmarshal(writer.files[marketplacePath], &out); err != nil {
		t.Fatalf("marketplace.json parse error = %v\n%s", err, writer.files[marketplacePath])
	}
	var existing map[string]json.RawMessage
	for _, plugin := range out.Plugins {
		var name string
		if err := json.Unmarshal(plugin["name"], &name); err == nil && name == "click-existing" {
			existing = plugin
		}
	}
	if existing == nil {
		t.Fatalf("marketplace.json dropped existing plugin entry: %s", writer.files[marketplacePath])
	}
	keywordsRaw, ok := existing["keywords"]
	if !ok {
		t.Fatalf("marketplace.json dropped unknown per-plugin field \"keywords\": %+v", existing)
	}
	var keywords []string
	if err := json.Unmarshal(keywordsRaw, &keywords); err != nil || len(keywords) != 2 || keywords[0] != "db" {
		t.Fatalf("marketplace.json keywords = %s, want [\"db\",\"review\"]", keywordsRaw)
	}
	if len(out.Plugins) != 2 {
		t.Fatalf("marketplace.json plugins count = %d, want 2 (existing + new)", len(out.Plugins))
	}
}

func TestScaffoldShareablePluginPreservesObjectFormSourceOnExistingPlugin(t *testing.T) {
	repoRoot := filepath.Join("testdata", "repo")
	spec := validAgentSpec()
	spec.Placement = PlacementShareable
	writer := newFakeFileWriter()
	writer.files[filepath.Join(repoRoot, ".claude-plugin", "marketplace.json")] = []byte(
		`{"plugins":[{"name":"click-existing","description":"Existing plugin","version":"1.0.0","author":{"name":"Someone"},"source":{"type":"github","repo":"click/existing"}}]}`,
	)

	if _, err := Install(spec, "", repoRoot, writer); err != nil {
		t.Fatalf("Install() error = %v, want object-form source to round-trip without error", err)
	}

	marketplacePath := filepath.Join(repoRoot, ".claude-plugin", "marketplace.json")
	var out struct {
		Plugins []map[string]json.RawMessage `json:"plugins"`
	}
	if err := json.Unmarshal(writer.files[marketplacePath], &out); err != nil {
		t.Fatalf("marketplace.json parse error = %v\n%s", err, writer.files[marketplacePath])
	}
	var existingSource json.RawMessage
	var newSource json.RawMessage
	for _, plugin := range out.Plugins {
		var name string
		if err := json.Unmarshal(plugin["name"], &name); err != nil {
			continue
		}
		switch name {
		case "click-existing":
			existingSource = plugin["source"]
		case "click-release-helper":
			newSource = plugin["source"]
		}
	}
	var existingSourceObj struct {
		Type string `json:"type"`
		Repo string `json:"repo"`
	}
	if err := json.Unmarshal(existingSource, &existingSourceObj); err != nil || existingSourceObj.Repo != "click/existing" {
		t.Fatalf("existing plugin source corrupted = %s, want object-form source preserved", existingSource)
	}
	var newSourceStr string
	if err := json.Unmarshal(newSource, &newSourceStr); err != nil || newSourceStr != "./plugins/click-release-helper" {
		t.Fatalf("new plugin source = %s, want string-form ./plugins/click-release-helper", newSource)
	}
}

// R4-002 regression coverage: the 3-step shareable-install write sequence (agent .md ->
// plugin.json -> marketplace.json) must never leave silent partial/corrupt state, and a
// failure partway through must produce an error that says exactly what succeeded and
// what didn't.
func TestScaffoldShareablePluginPartialFailureReportsWhichFilesSucceeded(t *testing.T) {
	repoRoot := filepath.Join("testdata", "repo")
	spec := validAgentSpec()
	spec.Placement = PlacementShareable
	writer := &selectiveFailWriter{fakeFileWriter: newFakeFileWriter(), failWriteContains: "marketplace.json"}

	_, err := Install(spec, "", repoRoot, writer)
	if err == nil {
		t.Fatal("Install() error = nil, want error when marketplace.json write fails")
	}

	agentPath := filepath.Join(repoRoot, "plugins", "click-release-helper", "agents", "release-helper.md")
	pluginManifestPath := filepath.Join(repoRoot, "plugins", "click-release-helper", ".claude-plugin", "plugin.json")
	marketplacePath := filepath.Join(repoRoot, ".claude-plugin", "marketplace.json")

	if _, ok := writer.files[agentPath]; !ok {
		t.Fatalf("agent markdown was not written before the failure; writes=%v", writer.writePaths)
	}
	if _, ok := writer.files[pluginManifestPath]; !ok {
		t.Fatalf("plugin manifest was not written before the failure; writes=%v", writer.writePaths)
	}
	if _, ok := writer.files[marketplacePath]; ok {
		t.Fatalf("marketplace manifest was written despite injected failure; writes=%v", writer.writePaths)
	}

	msg := err.Error()
	if !strings.Contains(msg, pluginManifestPath) || !strings.Contains(msg, marketplacePath) {
		t.Fatalf("error = %q, want it to name both the succeeded plugin manifest path and the failed marketplace path", msg)
	}
}

func TestScaffoldShareablePluginAtomicMarketplaceWriteFailureLeavesExistingFileUntouched(t *testing.T) {
	repoRoot := filepath.Join("testdata", "repo")
	marketplacePath := filepath.Join(repoRoot, ".claude-plugin", "marketplace.json")
	original := []byte(`{"plugins":[{"name":"click-existing","description":"Existing","version":"1.0.0","author":{"name":"Someone"},"source":"./plugins/click-existing"}]}`)

	spec := validAgentSpec()
	spec.Placement = PlacementShareable
	writer := &selectiveFailWriter{fakeFileWriter: newFakeFileWriter(), failWriteContains: "marketplace.json"}
	writer.files[marketplacePath] = append([]byte(nil), original...)

	if _, err := Install(spec, "", repoRoot, writer); err == nil {
		t.Fatal("Install() error = nil, want error when marketplace.json write fails")
	}
	if string(writer.files[marketplacePath]) != string(original) {
		t.Fatalf("existing marketplace.json was corrupted by the failed atomic write:\ngot  %s\nwant %s", writer.files[marketplacePath], original)
	}

	// Blocker fixed: retry (a different agent, to avoid the unrelated .md/plugin.json
	// collision already covered by TestInstallFinalMarkdownRejectsExistingTargetAgentWithoutOverwrite)
	// using a writer sharing the same underlying files, without the injected failure.
	retryWriter := &fakeFileWriter{files: writer.fakeFileWriter.files}
	retrySpec := validAgentSpec()
	retrySpec.Name = "second-helper"
	retrySpec.Description = "Second helper agent."
	retrySpec.Placement = PlacementShareable

	if _, err := Install(retrySpec, "", repoRoot, retryWriter); err != nil {
		t.Fatalf("Install() after fixed blocker error = %v, want the previously-untouched marketplace.json to still be valid/parseable", err)
	}

	var marketplace struct {
		Plugins []struct {
			Name string `json:"name"`
		} `json:"plugins"`
	}
	if err := json.Unmarshal(retryWriter.files[marketplacePath], &marketplace); err != nil {
		t.Fatalf("marketplace.json parse error after retry = %v\n%s", err, retryWriter.files[marketplacePath])
	}
	if len(marketplace.Plugins) != 2 {
		t.Fatalf("marketplace plugins after retry = %+v, want original entry preserved plus the new registration", marketplace.Plugins)
	}
}

func TestScaffoldShareablePluginManifestWriteFailureAllowsCleanRetryWithoutFalseCollision(t *testing.T) {
	repoRoot := filepath.Join("testdata", "repo")
	spec := validAgentSpec()
	spec.Placement = PlacementShareable
	writer := &selectiveFailWriter{fakeFileWriter: newFakeFileWriter(), failWriteContains: "plugin.json"}

	if err := scaffoldShareablePlugin(spec, repoRoot, writer); err == nil {
		t.Fatal("scaffoldShareablePlugin() error = nil, want error when plugin.json write fails")
	}

	pluginManifestPath := filepath.Join(repoRoot, "plugins", "click-release-helper", ".claude-plugin", "plugin.json")
	if _, ok := writer.files[pluginManifestPath]; ok {
		t.Fatalf("plugin.json left a truncated/orphaned file after a failed atomic write; writes=%v", writer.writePaths)
	}

	// The atomic write leaves no trace on failure, so the collision guard must not
	// falsely report the plugin manifest as already existing.
	if err := ensureShareablePluginNameAvailable(spec, repoRoot, writer.fakeFileWriter); err != nil {
		t.Fatalf("ensureShareablePluginNameAvailable() error = %v, want nil (plugin manifest write never actually completed)", err)
	}

	// Blocker fixed: retry with a non-failing writer sharing the same underlying files.
	retryWriter := &fakeFileWriter{files: writer.fakeFileWriter.files}
	if err := scaffoldShareablePlugin(spec, repoRoot, retryWriter); err != nil {
		t.Fatalf("scaffoldShareablePlugin() retry error = %v, want success once the blocker is fixed", err)
	}
	if _, ok := retryWriter.files[pluginManifestPath]; !ok {
		t.Fatal("scaffoldShareablePlugin() retry did not write plugin.json")
	}
}

func validClickSDDPluginManifest() []byte {
	return []byte(`{"name":"click-sdd","version":"0.1.0","description":"Click SDD plugin","author":{"name":"Click AI Devkit"}}`)
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
	mkdirPath   string
	mkdirPerm   os.FileMode
	mkdirErr    error
	writePath   string
	writeData   []byte
	writePerm   os.FileMode
	writeErr    error
	writePaths  []string
	renameErr   error
	renamePaths [][2]string
	files       map[string][]byte
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

func (w *fakeFileWriter) Stat(path string) (os.FileInfo, error) {
	if w.files != nil {
		data, ok := w.files[path]
		if !ok {
			return nil, os.ErrNotExist
		}
		return fakeFileInfo{name: filepath.Base(path), size: int64(len(data))}, nil
	}
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

func (w *fakeFileWriter) Rename(oldpath, newpath string) error {
	if w.renameErr != nil {
		return w.renameErr
	}
	if w.files != nil {
		if data, ok := w.files[oldpath]; ok {
			w.files[newpath] = data
			delete(w.files, oldpath)
		}
	}
	w.renamePaths = append(w.renamePaths, [2]string{oldpath, newpath})
	return nil
}

// selectiveFailWriter wraps a fakeFileWriter and fails WriteFile only for paths
// containing failWriteContains, leaving no trace in the underlying files map for that
// call (unlike fakeFileWriter.writeErr, which records data before returning the error).
// This models a real atomic-write temp-file failure: nothing is committed at all.
type selectiveFailWriter struct {
	*fakeFileWriter
	failWriteContains string
}

func (w *selectiveFailWriter) WriteFile(path string, data []byte, perm os.FileMode) error {
	if w.failWriteContains != "" && strings.Contains(path, w.failWriteContains) {
		return errors.New("simulated write failure")
	}
	return w.fakeFileWriter.WriteFile(path, data, perm)
}

type fakeFileInfo struct {
	name string
	size int64
}

func (f fakeFileInfo) Name() string       { return f.name }
func (f fakeFileInfo) Size() int64        { return f.size }
func (f fakeFileInfo) Mode() os.FileMode  { return 0o600 }
func (f fakeFileInfo) ModTime() time.Time { return time.Time{} }
func (f fakeFileInfo) IsDir() bool        { return false }
func (f fakeFileInfo) Sys() any           { return nil }
