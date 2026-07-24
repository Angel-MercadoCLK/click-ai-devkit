package cli

import (
	"fmt"
	"strings"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/installer"
	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/ui"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

func newConfigureTargetsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "configure-targets",
		Short: "Selecciona los runtimes que Click debe instalar y actualizar",
		RunE:  runConfigureTargets,
	}
	cmd.SilenceUsage = true
	return cmd
}

func runConfigureTargets(cmd *cobra.Command, args []string) error {
	out := cmd.OutOrStdout()
	// This command is reachable directly and launches Bubble Tea, which needs a real terminal on BOTH
	// ends: piped/redirected stdin starves the program even when stdout is a real TTY. Require both,
	// exactly like bare `click`'s interactive() gate.
	if !isTerminalWriter(out) || !isTerminalReader(cmd.InOrStdin()) {
		fmt.Fprintln(out, "No hay terminal interactiva disponible; use `click install` o `click update` para aplicar la selección de runtimes.")
		return nil
	}
	clickStateHome, err := installer.ResolveClickStateHome()
	if err != nil {
		return err
	}
	cfg := installer.Config{ClickStateHome: clickStateHome}
	selection, _, err := installer.LoadTargetSelection(cfg)
	if err != nil {
		return err
	}
	claudeFound := installer.ClaudeAvailable()
	openClawFound := installer.OpenClawAvailable()
	codexFound := installer.CodexAvailable()
	openClawSelected := selection.OpenClaw
	if !selection.Configured {
		openClawSelected = openClawFound
	}
	model := ui.NewTargetSelectModel(claudeFound, openClawFound, selection.Claude, openClawSelected, codexFound, selection.Codex)
	program := tea.NewProgram(model, tea.WithInput(cmd.InOrStdin()), tea.WithOutput(out))
	final, err := program.Run()
	if err != nil {
		return fmt.Errorf("cli: ejecutar selección de runtimes: %w", err)
	}
	result := final.(ui.TargetSelectModel)
	if result.Cancelled {
		fmt.Fprintln(out, "Configuración cancelada.")
		return nil
	}
	if !result.Confirmed {
		return nil
	}
	if err := installer.SaveTargetSelection(cfg, installer.TargetSelection{Configured: true, Claude: result.Claude, OpenClaw: result.OpenClaw, Codex: result.Codex}); err != nil {
		return err
	}
	plan := installer.BuildTargetPlan(cfg, installer.TargetSelection{Configured: true, Claude: result.Claude, OpenClaw: result.OpenClaw, Codex: result.Codex}, installer.PlanOptions{})
	fmt.Fprintln(out, "Selección de runtimes guardada.")
	if summary := strings.TrimSpace(plan.CapabilitiesSummary()); summary != "" {
		fmt.Fprintf(out, "Plan objetivo: %s\n", summary)
	}
	return nil
}

func resolveTargetConfig(cfg installer.Config, skipOpenClaw bool, out interface{ Write([]byte) (int, error) }, r interface{ Warn(string) string }) (installer.TargetSelection, installer.Config, error) {
	selection, _, err := installer.LoadTargetSelection(cfg)
	if err != nil {
		// A corrupt/unsupported targets.json must never brick install/update. Unlike the standalone
		// `click targets` / `configure-targets` diagnostics — which are free to surface this error to
		// the user — the install/update path degrades gracefully: warn, then fall back to exactly the
		// same SAFE DEFAULT the file-absent branch of LoadTargetSelection uses (Claude on, OpenClaw
		// follows live detection, Codex off, Configured=false) and continue installing/updating.
		fmt.Fprintln(out, r.Warn("targets.json ilegible o de versión no soportada; se usa la detección por defecto (Claude + OpenClaw si está presente). Ejecute `click configure-targets` para regenerarlo."))
		selection = installer.TargetSelection{Claude: true, OpenClaw: true}
	}
	detected := installer.OpenClawAvailable()
	switch {
	case installer.ResolveOpenClawTarget(selection, detected) && !skipOpenClaw:
		cfg.OpenClawHome, err = installer.ResolveOpenClawHome()
		if err != nil {
			return installer.TargetSelection{}, installer.Config{}, err
		}
	case selection.Configured && selection.OpenClaw && !detected:
		fmt.Fprintln(out, r.Warn("OpenClaw fue seleccionado, pero no está disponible; se omite esta integración y continúa Claude Code."))
	case selection.Configured && !selection.OpenClaw && detected && !skipOpenClaw:
		// OpenClaw is installed and detected now, but a PRIOR explicit selection (e.g. a previous
		// `click install --skip-openclaw`) persisted OpenClaw=false to targets.json, so this
		// install/update silently leaves it excluded. Surface that non-fatal, informational signal so
		// the developer knows the integration is available and how to re-enable it — without changing
		// the persisted selection here.
		fmt.Fprintln(out, r.Warn("OpenClaw está disponible en este equipo, pero quedó excluido por una selección previa; vuelva a habilitarlo con `click configure-targets`."))
	}
	if installer.ResolveCodexTarget(selection, installer.CodexAvailable()) {
		cfg.CodexHome, err = installer.ResolveCodexHome()
		if err != nil {
			return installer.TargetSelection{}, installer.Config{}, err
		}
	} else if selection.Configured && selection.Codex && !installer.CodexAvailable() {
		fmt.Fprintln(out, r.Warn("Codex fue seleccionado, pero no está disponible; se omite esta integración y continúa Claude Code."))
	}
	return selection, cfg, nil
}
