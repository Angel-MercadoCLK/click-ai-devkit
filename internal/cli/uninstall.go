package cli

import (
	"fmt"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/installer"
	"github.com/spf13/cobra"
)

func newUninstallCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "uninstall",
		Short: "Reverse everything click install and click update wrote",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUninstall(cmd)
		},
	}
}

func runUninstall(cmd *cobra.Command) error {
	out := cmd.OutOrStdout()
	r := rendererFor(cmd, out)

	claudeHome, err := installer.ResolveClaudeHome()
	if err != nil {
		return err
	}
	cfg := installer.Config{ClaudeHome: claudeHome}

	if err := r.RunStep("Quitando plugins click-sdd, click-memory y click-review…", "Plugins eliminados de Claude Code", func() error {
		return installer.RemoveMarketplacePlugins()
	}); err != nil {
		return err
	}

	if err := r.RunStep("Limpiando CLAUDE.md…", "Bloque de CLAUDE.md eliminado", func() error {
		return installer.StripManagedBlock(cfg.ClaudeMDPath())
	}); err != nil {
		return err
	}

	if err := r.RunStep("Quitando memory-guard…", "memory-guard eliminado", func() error {
		return installer.UnregisterMemoryGuardHook(cfg)
	}); err != nil {
		return err
	}

	// RemoveEngramPlugin only reverses Engram when click's own state says click installed it —
	// a pre-existing developer setup is left running untouched.
	if err := r.RunStep("Quitando Engram (si click lo instaló)…", "Engram procesado", func() error {
		return installer.RemoveEngramPlugin(cfg)
	}); err != nil {
		return err
	}

	fmt.Fprintln(out, r.Info("Desinstalación completa."))
	return nil
}
