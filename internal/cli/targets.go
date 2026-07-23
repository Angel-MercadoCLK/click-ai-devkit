package cli

import (
	"fmt"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/installer"
	"github.com/spf13/cobra"
)

// newTargetsCommand reports the supported runtime targets and their current detection status.
// Detection is intentionally read-only and reuses installer path-resolution seams; this command
// does not install, configure, or persist a selected target.
func newTargetsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "targets",
		Short: "Detecta los runtimes compatibles y resume sus capacidades",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTargets(cmd)
		},
	}
	cmd.SilenceUsage = true
	return cmd
}

func runTargets(cmd *cobra.Command) error {
	out := cmd.OutOrStdout()
	r := rendererFor(cmd, out)
	claudeHome, err := installer.ResolveClaudeHome()
	if err != nil {
		return err
	}
	selection, configured, err := installer.LoadTargetSelection(installer.Config{ClaudeHome: claudeHome})
	if err != nil {
		return err
	}

	claudePath, claudeDetected := installer.ClaudePath()
	if claudeDetected {
		fmt.Fprintln(out, r.Success("Claude Code: detectado (resuelto en "+claudePath+")"))
	} else {
		fmt.Fprintln(out, r.Info("Claude Code: no detectado"))
	}
	fmt.Fprintln(out, r.Info("  Capacidades: flujo completo de plugins, SDD y modelos."))

	openClawPath, openClawDetected := installer.OpenClawPath()
	if openClawDetected {
		openClawHome, err := installer.ResolveOpenClawHome()
		if err != nil {
			return err
		}
		workspace := (installer.Config{OpenClawHome: openClawHome}).OpenClawWorkspaceDir()
		fmt.Fprintln(out, r.Success("OpenClaw: detectado (resuelto en "+openClawPath+")"))
		fmt.Fprintln(out, r.Info("  Workspace: "+workspace))
	} else {
		fmt.Fprintln(out, r.Info("OpenClaw: no detectado"))
	}
	fmt.Fprintln(out, r.Info("  Capacidades: SDD portable, Engram, memory guard y recomendación de modelos."))
	fmt.Fprintln(out, r.Info("  Nota: la recomendación de modelos para OpenClaw es portable; click no aplica modelos nativamente allí."))
	codexPath, codexDetected := installer.CodexPath()
	if codexDetected {
		fmt.Fprintln(out, r.Success("Codex: detectado (resuelto en "+codexPath+")"))
		fmt.Fprintln(out, r.Info("  Capacidades: AGENTS.md gestionado y flujo SDD portable."))
	} else {
		fmt.Fprintln(out, r.Info("Codex: no detectado"))
		fmt.Fprintln(out, r.Info("  Capacidades: guía AGENTS.md y flujo SDD portable disponibles al habilitar Codex."))
	}
	fmt.Fprintln(out, r.Info("  Nota: no modifica config.toml ni el modelo; tampoco credenciales ni skills/plugins nativos de Codex."))
	if configured {
		fmt.Fprintln(out, r.Info(fmt.Sprintf("  Selección persistente: Claude Code=%t, OpenClaw=%t, Codex=%t.", selection.Claude, selection.OpenClaw, selection.Codex)))
	} else {
		fmt.Fprintln(out, r.Info("  Selección persistente: Claude Code seleccionado; OpenClaw sigue la autodetección y Codex requiere habilitación explícita."))
	}
	fmt.Fprintln(out, r.Warn("Otros runtimes: no soportados todavía; click no los detecta ni gestiona."))
	return nil
}
