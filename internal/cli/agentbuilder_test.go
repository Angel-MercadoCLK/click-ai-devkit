package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/agentbuilder"
	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/ui"
	"github.com/spf13/cobra"
)

func TestAgentBuilderCommand_InstallsConfirmedFinalMarkdownExactly(t *testing.T) {
	claudeHome := t.TempDir()
	repoRoot := t.TempDir()
	spec := cliValidAgentSpec()
	spec.Placement = agentbuilder.PlacementShareable
	finalMarkdown := "---\n" +
		"name: \"release-helper\"\n" +
		"description: \"Edited and confirmed in preview\"\n" +
		"model: \"opus\"\n" +
		"tools: \"Read, Edit, Bash\"\n" +
		"---\n\n" +
		"# Role\nThis exact markdown came from the wizard.\n"

	cmd := &cobra.Command{}
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetIn(&bytes.Buffer{})
	deps := agentBuilderCommandDeps{
		runWizard: func(*cobra.Command) (ui.AgentBuilderModel, error) {
			return ui.AgentBuilderModel{Confirmed: true, Spec: spec, FinalMarkdown: finalMarkdown}, nil
		},
		resolveClaudeHomeOverride: func() (string, error) { return claudeHome, nil },
		resolveRepoRoot:           func() (string, error) { return repoRoot, nil },
	}

	if err := runAgentBuilderInteractive(cmd, deps); err != nil {
		t.Fatalf("runAgentBuilderInteractive() error = %v, output:\n%s", err, out.String())
	}

	wantPath := filepath.Join(repoRoot, "plugins", "click-release-helper", "agents", "release-helper.md")
	got, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", wantPath, err)
	}
	if string(got) != finalMarkdown {
		t.Fatalf("installed markdown =\n%q\nwant exact wizard FinalMarkdown\n%q", got, finalMarkdown)
	}
	if !strings.Contains(out.String(), wantPath) {
		t.Fatalf("output = %q, want installed path %q", out.String(), wantPath)
	}
}

func TestAgentBuilderCommand_PersonalInstallHonorsClaudeConfigDir(t *testing.T) {
	legacyHome := t.TempDir()
	configHome := t.TempDir()
	t.Setenv("CLICK_CLAUDE_HOME", legacyHome)
	t.Setenv("CLAUDE_CONFIG_DIR", configHome)

	spec := cliValidAgentSpec()
	spec.Placement = agentbuilder.PlacementPersonal
	finalMarkdown := "---\n" +
		"name: \"release-helper\"\n" +
		"description: \"Edited and confirmed in preview\"\n" +
		"model: \"opus\"\n" +
		"tools: \"Read, Edit, Bash\"\n" +
		"---\n\n" +
		"# Role\nThis exact markdown came from the wizard.\n"

	cmd := &cobra.Command{}
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetIn(&bytes.Buffer{})
	deps := agentBuilderCommandDeps{
		runWizard: func(*cobra.Command) (ui.AgentBuilderModel, error) {
			return ui.AgentBuilderModel{Confirmed: true, Spec: spec, FinalMarkdown: finalMarkdown}, nil
		},
	}

	if err := runAgentBuilderInteractive(cmd, deps); err != nil {
		t.Fatalf("runAgentBuilderInteractive() error = %v, output:\n%s", err, out.String())
	}

	wantPath := filepath.Join(configHome, "agents", "release-helper.md")
	got, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", wantPath, err)
	}
	if string(got) != finalMarkdown {
		t.Fatalf("installed markdown =\n%q\nwant exact wizard FinalMarkdown\n%q", got, finalMarkdown)
	}
	wrongPath := filepath.Join(legacyHome, "agents", "release-helper.md")
	if _, err := os.Stat(wrongPath); !os.IsNotExist(err) {
		t.Fatalf("personal install wrote %s with CLICK_CLAUDE_HOME set; want CLAUDE_CONFIG_DIR path %s", wrongPath, wantPath)
	}
	if !strings.Contains(out.String(), wantPath) {
		t.Fatalf("output = %q, want installed path %q", out.String(), wantPath)
	}
}

func TestAgentBuilderCommand_PersonalInstallHonorsClickClaudeHomeWhenClaudeConfigDirUnset(t *testing.T) {
	clickClaudeHome := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	t.Setenv("CLICK_CLAUDE_HOME", clickClaudeHome)

	spec := cliValidAgentSpec()
	spec.Placement = agentbuilder.PlacementPersonal
	finalMarkdown := "---\n" +
		"name: \"release-helper\"\n" +
		"description: \"Edited and confirmed in preview\"\n" +
		"model: \"opus\"\n" +
		"tools: \"Read, Edit, Bash\"\n" +
		"---\n\n" +
		"# Role\nThis exact markdown came from the wizard.\n"

	cmd := &cobra.Command{}
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetIn(&bytes.Buffer{})
	deps := agentBuilderCommandDeps{
		runWizard: func(*cobra.Command) (ui.AgentBuilderModel, error) {
			return ui.AgentBuilderModel{Confirmed: true, Spec: spec, FinalMarkdown: finalMarkdown}, nil
		},
	}

	if err := runAgentBuilderInteractive(cmd, deps); err != nil {
		t.Fatalf("runAgentBuilderInteractive() error = %v, output:\n%s", err, out.String())
	}

	wantPath := filepath.Join(clickClaudeHome, "agents", "release-helper.md")
	got, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", wantPath, err)
	}
	if string(got) != finalMarkdown {
		t.Fatalf("installed markdown =\n%q\nwant exact wizard FinalMarkdown\n%q", got, finalMarkdown)
	}
	if !strings.Contains(out.String(), wantPath) {
		t.Fatalf("output = %q, want installed path %q", out.String(), wantPath)
	}
}

func TestAgentBuilderCommand_PersonalInstallPreservesInjectedClaudeHomeOverride(t *testing.T) {
	overrideHome := t.TempDir()
	configHome := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", configHome)

	spec := cliValidAgentSpec()
	spec.Placement = agentbuilder.PlacementPersonal
	finalMarkdown := "---\n" +
		"name: \"release-helper\"\n" +
		"description: \"Edited and confirmed in preview\"\n" +
		"model: \"opus\"\n" +
		"tools: \"Read, Edit, Bash\"\n" +
		"---\n\n" +
		"# Role\nThis exact markdown came from the wizard.\n"

	cmd := &cobra.Command{}
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetIn(&bytes.Buffer{})
	deps := agentBuilderCommandDeps{
		runWizard: func(*cobra.Command) (ui.AgentBuilderModel, error) {
			return ui.AgentBuilderModel{Confirmed: true, Spec: spec, FinalMarkdown: finalMarkdown}, nil
		},
		resolveClaudeHomeOverride: func() (string, error) { return overrideHome, nil },
	}

	if err := runAgentBuilderInteractive(cmd, deps); err != nil {
		t.Fatalf("runAgentBuilderInteractive() error = %v, output:\n%s", err, out.String())
	}

	wantPath := filepath.Join(overrideHome, "agents", "release-helper.md")
	got, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", wantPath, err)
	}
	if string(got) != finalMarkdown {
		t.Fatalf("installed markdown =\n%q\nwant exact wizard FinalMarkdown\n%q", got, finalMarkdown)
	}
	configPath := filepath.Join(configHome, "agents", "release-helper.md")
	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		t.Fatalf("personal install with injected override wrote %s; want override path %s", configPath, wantPath)
	}
}

func TestAgentBuilderCommand_NonTTYReturnsGuardWithoutRunningWizard(t *testing.T) {
	home := t.TempDir()
	out, err := execRoot(t, home, "agent-builder")
	if err == nil {
		t.Fatal("agent-builder command on non-TTY error = nil, want an interactive-terminal guard")
	}
	if !strings.Contains(out, "requiere una terminal interactiva") {
		t.Fatalf("agent-builder command output = %q, want non-TTY guard message", out)
	}
}

func cliValidAgentSpec() agentbuilder.AgentSpec {
	return agentbuilder.AgentSpec{
		Engine:      agentbuilder.ClaudeCode,
		Name:        "release-helper",
		Description: "Helps prepare release notes.",
		SDDMode:     agentbuilder.SDDStandalone,
		Tools:       "Read, Grep",
		Model:       "sonnet",
		Purpose:     "Turn merged pull requests into release notes.",
		Tasks:       "Read merged PRs and group changes by user-facing area.",
		Triggers:    "Use when a release candidate is ready.",
		Rules:       "Never invent merged work.",
		Tone:        "Clear, direct, and concise.",
		Domain:      "Go CLI release management.",
		GoodOutput:  "A Markdown changelog grouped by area.",
		Placement:   agentbuilder.PlacementPersonal,
	}
}
