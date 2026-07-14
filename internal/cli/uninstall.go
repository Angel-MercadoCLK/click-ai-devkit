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

	if err := r.RunStep("Quitando plugins click-sdd, click-memory, click-review y click-skills…", "Plugins eliminados de Claude Code", func() error {
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

	// RemoveContext7 mirrors RemoveEngramPlugin's exact respect-ownership contract: only removes
	// Context7 when click's own state says click registered it.
	if err := r.RunStep("Quitando Context7 (si click lo instaló)…", "Context7 procesado", func() error {
		return installer.RemoveContext7(cfg)
	}); err != nil {
		return err
	}

	fmt.Fprintln(out, r.Info("Desinstalación completa."))
	return nil
}
