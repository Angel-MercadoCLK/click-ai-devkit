package cli

import (
	"fmt"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/installer"
	"github.com/spf13/cobra"
)

func newUninstallCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "uninstall",
		Short: "Reverse click install exactly",
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

	if err := r.RunStep("Quitando plugin click-sdd…", "Plugin click-sdd eliminado", func() error {
		return installer.RemoveClickSDDPlugin(cfg)
	}); err != nil {
		return err
	}

	if err := r.RunStep("Quitando plugin click-memory…", "Plugin click-memory eliminado", func() error {
		return installer.RemoveClickMemoryPlugin(cfg)
	}); err != nil {
		return err
	}

	if err := r.RunStep("Quitando plugin click-review…", "Plugin click-review eliminado", func() error {
		return installer.RemoveClickReviewPlugin(cfg)
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

	fmt.Fprintln(out, r.Info("Desinstalación completa."))
	return nil
}
