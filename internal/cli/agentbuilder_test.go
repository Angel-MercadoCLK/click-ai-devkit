package cli

import (
	"bytes"
	"fmt"
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
		"# Role\nThis exact markdown came from the wizard.\n\n" +
		"## Tasks\nConfirmed tasks.\n\n" +
		"## Triggers\nConfirmed triggers.\n\n" +
		"## Hard Rules\nConfirmed hard rules.\n\n" +
		"## SDD Integration\nMode: standalone\n\n" +
		"## Tone\nConfirmed tone.\n\n" +
		"## Domain Knowledge\nConfirmed domain knowledge.\n\n" +
		"## Good Output\nConfirmed good output.\n"

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
		"# Role\nThis exact markdown came from the wizard.\n\n" +
		"## Tasks\nConfirmed tasks.\n\n" +
		"## Triggers\nConfirmed triggers.\n\n" +
		"## Hard Rules\nConfirmed hard rules.\n\n" +
		"## SDD Integration\nMode: standalone\n\n" +
		"## Tone\nConfirmed tone.\n\n" +
		"## Domain Knowledge\nConfirmed domain knowledge.\n\n" +
		"## Good Output\nConfirmed good output.\n"

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
		"# Role\nThis exact markdown came from the wizard.\n\n" +
		"## Tasks\nConfirmed tasks.\n\n" +
		"## Triggers\nConfirmed triggers.\n\n" +
		"## Hard Rules\nConfirmed hard rules.\n\n" +
		"## SDD Integration\nMode: standalone\n\n" +
		"## Tone\nConfirmed tone.\n\n" +
		"## Domain Knowledge\nConfirmed domain knowledge.\n\n" +
		"## Good Output\nConfirmed good output.\n"

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
		"# Role\nThis exact markdown came from the wizard.\n\n" +
		"## Tasks\nConfirmed tasks.\n\n" +
		"## Triggers\nConfirmed triggers.\n\n" +
		"## Hard Rules\nConfirmed hard rules.\n\n" +
		"## SDD Integration\nMode: standalone\n\n" +
		"## Tone\nConfirmed tone.\n\n" +
		"## Domain Knowledge\nConfirmed domain knowledge.\n\n" +
		"## Good Output\nConfirmed good output.\n"

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

// R4-003 regression coverage: checkAgentBuilderNameAvailable is the closure
// runAgentBuilderWizardTUI wires into ui.WithNameAvailabilityCheck. It must resolve
// claudeHome/repoRoot the same way runAgentBuilderInteractive resolves them right
// before installing, so the mid-wizard collision probe agrees with the real install
// path.
func TestCheckAgentBuilderNameAvailableUsesResolvedClaudeHomeAndRepoRoot(t *testing.T) {
	t.Run("personal placement checks resolved claude home", func(t *testing.T) {
		claudeHome := t.TempDir()
		if err := os.MkdirAll(filepath.Join(claudeHome, "agents"), 0o755); err != nil {
			t.Fatalf("os.MkdirAll() error = %v", err)
		}
		if err := os.WriteFile(filepath.Join(claudeHome, "agents", "release-helper.md"), []byte("existing"), 0o600); err != nil {
			t.Fatalf("os.WriteFile() error = %v", err)
		}
		spec := cliValidAgentSpec()
		spec.Placement = agentbuilder.PlacementPersonal
		deps := agentBuilderCommandDeps{
			resolveClaudeHomeOverride: func() (string, error) { return claudeHome, nil },
		}

		if err := checkAgentBuilderNameAvailable(spec, deps); err == nil {
			t.Fatal("checkAgentBuilderNameAvailable() error = nil, want collision error for existing personal target")
		}
	})

	t.Run("shareable placement does not resolve repo root when unused", func(t *testing.T) {
		spec := cliValidAgentSpec()
		spec.Placement = agentbuilder.PlacementPersonal
		deps := agentBuilderCommandDeps{
			resolveClaudeHomeOverride: func() (string, error) { return t.TempDir(), nil },
			resolveRepoRoot: func() (string, error) {
				t.Fatal("resolveRepoRoot() called for a personal-placement check, want it skipped")
				return "", nil
			},
		}

		if err := checkAgentBuilderNameAvailable(spec, deps); err != nil {
			t.Fatalf("checkAgentBuilderNameAvailable() error = %v, want nil for a free personal name", err)
		}
	})

	t.Run("shareable placement resolves repo root", func(t *testing.T) {
		repoRoot := t.TempDir()
		spec := cliValidAgentSpec()
		spec.Placement = agentbuilder.PlacementShareable
		deps := agentBuilderCommandDeps{
			resolveRepoRoot: func() (string, error) { return repoRoot, nil },
		}

		if err := checkAgentBuilderNameAvailable(spec, deps); err != nil {
			t.Fatalf("checkAgentBuilderNameAvailable() error = %v, want nil for a free shareable name", err)
		}
	})
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

// R2-007 (D10) regression coverage: internal/cli/agentbuilder.go mixed English
// ("agent-builder finished without a confirmed agent") with Spanish user-facing
// messages in the same file. Every RunE error message returned by this file must be
// Spanish, not raw English.
func TestAgentBuilderCommand_ErrorsAreSpanishNotRawEnglish(t *testing.T) {
	assertSpanish := func(t *testing.T, err error, forbidden ...string) {
		t.Helper()
		if err == nil {
			t.Fatal("error = nil, want a Spanish error message")
		}
		for _, f := range forbidden {
			if strings.Contains(err.Error(), f) {
				t.Fatalf("error = %q, want no raw English fragment %q (D10)", err.Error(), f)
			}
		}
	}

	t.Run("wizard finished without confirming", func(t *testing.T) {
		cmd := &cobra.Command{}
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		deps := agentBuilderCommandDeps{
			runWizard: func(*cobra.Command) (ui.AgentBuilderModel, error) {
				return ui.AgentBuilderModel{Confirmed: false, Cancelled: false}, nil
			},
		}
		err := runAgentBuilderInteractive(cmd, deps)
		assertSpanish(t, err, "finished without a confirmed agent")
	})

	t.Run("wizard confirmed empty final markdown", func(t *testing.T) {
		cmd := &cobra.Command{}
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		deps := agentBuilderCommandDeps{
			runWizard: func(*cobra.Command) (ui.AgentBuilderModel, error) {
				return ui.AgentBuilderModel{Confirmed: true, FinalMarkdown: "   "}, nil
			},
		}
		err := runAgentBuilderInteractive(cmd, deps)
		assertSpanish(t, err, "confirmed empty final markdown")
	})

	t.Run("install failure is wrapped with a Spanish lead message", func(t *testing.T) {
		spec := cliValidAgentSpec()
		cmd := &cobra.Command{}
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		deps := agentBuilderCommandDeps{
			runWizard: func(*cobra.Command) (ui.AgentBuilderModel, error) {
				return ui.AgentBuilderModel{Confirmed: true, Spec: spec, FinalMarkdown: "---\nname: \"release-helper\"\n---\n\n# Role\nx\n"}, nil
			},
			installFinalMarkdown: func(agentbuilder.AgentSpec, string, string, string, agentbuilder.FileWriter) (string, error) {
				return "", fmt.Errorf("agentbuilder: target agent already exists at /tmp/agents/release-helper.md")
			},
		}
		err := runAgentBuilderInteractive(cmd, deps)
		if err == nil {
			t.Fatal("error = nil, want the wrapped install error")
		}
		if !strings.Contains(err.Error(), "no se pudo instalar el agente") {
			t.Fatalf("error = %q, want a Spanish lead message before the technical detail", err.Error())
		}
	})
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
