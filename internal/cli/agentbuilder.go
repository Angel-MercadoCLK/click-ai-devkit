package cli

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/agentbuilder"
	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/ui"
)

type agentBuilderCommandDeps struct {
	runWizard                 func(*cobra.Command) (ui.AgentBuilderModel, error)
	resolveClaudeHomeOverride func() (string, error)
	resolveRepoRoot           func() (string, error)
	installFinalMarkdown      func(agentbuilder.AgentSpec, string, string, string, agentbuilder.FileWriter) (string, error)
}

func newAgentBuilderCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "agent-builder",
		Short: "Crear tu propio agente para Claude Code",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAgentBuilder(cmd, agentBuilderCommandDeps{})
		},
	}
	return cmd
}

func runAgentBuilder(cmd *cobra.Command, deps agentBuilderCommandDeps) error {
	out := cmd.OutOrStdout()
	in := cmd.InOrStdin()
	if !interactive(false, out, in) {
		message := "agent-builder requiere una terminal interactiva"
		fmt.Fprintln(out, message)
		return fmt.Errorf("cli: %s", message)
	}
	return runAgentBuilderInteractive(cmd, deps)
}

func runAgentBuilderInteractive(cmd *cobra.Command, deps agentBuilderCommandDeps) error {
	deps = defaultAgentBuilderCommandDeps(deps)
	out := cmd.OutOrStdout()

	result, err := deps.runWizard(cmd)
	if err != nil {
		return err
	}
	if result.Cancelled {
		fmt.Fprintln(out, "Creación de agente cancelada.")
		return nil
	}
	if !result.Confirmed {
		// D10: dev-facing CLI/TUI string literals are Spanish. Keep the "cli: " prefix
		// (an internal Go-package label, consistent with the rest of this file's
		// wrapped errors) but the actual message must be Spanish, not English.
		return fmt.Errorf("cli: el asistente de agent-builder terminó sin confirmar el agente")
	}
	if strings.TrimSpace(result.FinalMarkdown) == "" {
		return fmt.Errorf("cli: el asistente de agent-builder confirmó un markdown final vacío")
	}

	claudeHome := ""
	if deps.resolveClaudeHomeOverride != nil {
		var err error
		claudeHome, err = deps.resolveClaudeHomeOverride()
		if err != nil {
			return err
		}
	}
	repoRoot := ""
	if result.Spec.Placement == agentbuilder.PlacementShareable {
		repoRoot, err = deps.resolveRepoRoot()
		if err != nil {
			return err
		}
	}

	path, err := deps.installFinalMarkdown(result.Spec, result.FinalMarkdown, claudeHome, repoRoot, nil)
	if err != nil {
		// D10: lead with a Spanish user-facing message; keep the wrapped domain error
		// (English) as trailing technical detail for logs/troubleshooting via %w.
		return fmt.Errorf("cli: no se pudo instalar el agente: %w", err)
	}
	fmt.Fprintf(out, "Agente instalado en %s\n", path)
	return nil
}

func defaultAgentBuilderCommandDeps(deps agentBuilderCommandDeps) agentBuilderCommandDeps {
	if deps.resolveRepoRoot == nil {
		deps.resolveRepoRoot = resolveAgentBuilderRepoRoot
	}
	if deps.installFinalMarkdown == nil {
		deps.installFinalMarkdown = agentbuilder.InstallFinalMarkdown
	}
	if deps.runWizard == nil {
		// Captures deps (already defaulted above) so the wizard can probe for a name
		// collision using the same claudeHome/repoRoot resolution InstallFinalMarkdown
		// itself would use, while the wizard is still running (R4-003).
		deps.runWizard = func(cmd *cobra.Command) (ui.AgentBuilderModel, error) {
			return runAgentBuilderWizardTUI(cmd, deps)
		}
	}
	return deps
}

func runAgentBuilderWizardTUI(cmd *cobra.Command, deps agentBuilderCommandDeps) (ui.AgentBuilderModel, error) {
	program := tea.NewProgram(
		ui.NewAgentBuilderModel(agentbuilder.Engines(), ui.WithNameAvailabilityCheck(func(spec agentbuilder.AgentSpec) error {
			return checkAgentBuilderNameAvailable(spec, deps)
		})),
		tea.WithInput(cmd.InOrStdin()),
		tea.WithOutput(cmd.OutOrStdout()),
	)
	finalModel, err := program.Run()
	if err != nil {
		return ui.AgentBuilderModel{}, fmt.Errorf("cli: run agent-builder TUI: %w", err)
	}
	result, ok := finalModel.(ui.AgentBuilderModel)
	if !ok {
		return ui.AgentBuilderModel{}, fmt.Errorf("cli: unexpected agent-builder TUI model %T", finalModel)
	}
	return result, nil
}

// checkAgentBuilderNameAvailable resolves the same claudeHome/repoRoot
// runAgentBuilderInteractive resolves right before installing, then runs a read-only
// collision probe (agentbuilder.CheckNameAvailable) so the wizard can surface a
// collision before it ever exits (R4-003).
func checkAgentBuilderNameAvailable(spec agentbuilder.AgentSpec, deps agentBuilderCommandDeps) error {
	claudeHome := ""
	if deps.resolveClaudeHomeOverride != nil {
		var err error
		claudeHome, err = deps.resolveClaudeHomeOverride()
		if err != nil {
			return err
		}
	}
	repoRoot := ""
	if spec.Placement == agentbuilder.PlacementShareable {
		var err error
		repoRoot, err = deps.resolveRepoRoot()
		if err != nil {
			return err
		}
	}
	return agentbuilder.CheckNameAvailable(spec, claudeHome, repoRoot, nil)
}

func resolveAgentBuilderRepoRoot() (string, error) {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", fmt.Errorf("cli: resolve repository root for shareable agent: %w", err)
	}
	root := strings.TrimSpace(string(out))
	if root == "" {
		return "", fmt.Errorf("cli: git returned an empty repository root")
	}
	return filepath.Clean(root), nil
}
