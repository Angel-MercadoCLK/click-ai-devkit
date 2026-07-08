package cli

import (
	"fmt"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/installer"
	"github.com/spf13/cobra"
)

func newInstallCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "install",
		Short: "Install click-ai-devkit's click-sdd plugin and CLAUDE.md block into Claude Code",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInstall(cmd)
		},
	}
}

func runInstall(cmd *cobra.Command) error {
	out := cmd.OutOrStdout()
	r := rendererFor(cmd, out)

	fmt.Fprintln(out, r.Banner())

	claudeHome, err := installer.ResolveClaudeHome()
	if err != nil {
		return err
	}
	cfg := installer.Config{ClaudeHome: claudeHome}

	if err := r.RunStep("Copiando plugin click-sdd…", "Plugin click-sdd copiado", func() error {
		return installer.CopyClickSDDPlugin(cfg)
	}); err != nil {
		return err
	}

	if err := r.RunStep("Actualizando CLAUDE.md…", "CLAUDE.md actualizado", func() error {
		return installer.WriteManagedBlock(cfg.ClaudeMDPath(), installer.DefaultManagedContent)
	}); err != nil {
		return err
	}

	if err := r.RunStep("Registrando memory-guard…", "memory-guard registrado", func() error {
		return installer.RegisterMemoryGuardHook(cfg)
	}); err != nil {
		return err
	}

	fmt.Fprintln(out, r.Info("Instalación completa."))
	return nil
}
