package cli

import (
	"fmt"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/installer"
	"github.com/spf13/cobra"
)

// newConfigureModelsCommand backs the menu's "Configure models" item: it reuses install.go's
// existing runModelSelectTUI (the same internal/ui.ModelSelectModel screen `click install`
// already drives — not rebuilt here) and persists the result with installer.SaveModels, the same
// way `click install`/`click update` do. Hidden from `click --help` since it's primarily reached
// through the standing menu, but still directly runnable (`click configure-models`) for scripts
// or developers who want to skip the menu.
func newConfigureModelsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "configure-models",
		Short:  "Interactively choose click-sdd's per-phase models and save them",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfigureModels(cmd)
		},
	}
	return cmd
}

func runConfigureModels(cmd *cobra.Command) error {
	out := cmd.OutOrStdout()
	r := rendererFor(cmd, out)

	// Same non-hang safety net as bare `click`'s interactive() gate: this command can be reached
	// directly (not only through the already-TTY-gated menu), so it must never spin up a real
	// bubbletea program against a non-terminal output.
	if !isTerminalWriter(out) {
		fmt.Fprintln(out, r.Info("No hay terminal interactiva disponible; usá `click install` o `click update` para aplicar los modelos por defecto."))
		return nil
	}

	claudeHome, err := installer.ResolveClaudeHome()
	if err != nil {
		return err
	}
	cfg := installer.Config{ClaudeHome: claudeHome}

	selection, cancelled, err := runModelSelectTUI(cmd)
	if err != nil {
		return err
	}
	if cancelled {
		fmt.Fprintln(out, r.Info("Selección cancelada."))
		return nil
	}

	if err := installer.SaveModels(cfg, selection); err != nil {
		return err
	}
	fmt.Fprintln(out, r.Info("Modelos por fase guardados."))
	return nil
}
