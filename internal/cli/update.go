package cli

import (
	"fmt"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/installer"
	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/manifest"
	"github.com/spf13/cobra"
)

func newUpdateCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "update",
		Short: "Re-sync plugins and the Engram pin to the currently installed click binary",
		RunE:  runUpdate,
	}
}

func runUpdate(cmd *cobra.Command, args []string) error {
	out := cmd.OutOrStdout()
	r := rendererFor(cmd, out)

	claudeHome, err := installer.ResolveClaudeHome()
	if err != nil {
		return err
	}
	cfg := installer.Config{ClaudeHome: claudeHome}
	m, err := manifest.Load()
	if err != nil {
		return err
	}

	if err := r.RunStep("Re-sincronizando plugin click-sdd…", "Plugin click-sdd sincronizado", func() error {
		return installer.CopyClickSDDPlugin(cfg)
	}); err != nil {
		return err
	}
	if err := r.RunStep("Re-sincronizando plugin click-memory…", "Plugin click-memory sincronizado", func() error {
		return installer.CopyClickMemoryPlugin(cfg)
	}); err != nil {
		return err
	}
	if err := r.RunStep("Re-sincronizando plugin click-review…", "Plugin click-review sincronizado", func() error {
		return installer.CopyClickReviewPlugin(cfg)
	}); err != nil {
		return err
	}
	if err := r.RunStep("Actualizando CLAUDE.md…", "CLAUDE.md sincronizado", func() error {
		return installer.WriteManagedBlock(cfg.ClaudeMDPath(), installer.DefaultManagedContent)
	}); err != nil {
		return err
	}
	if err := r.RunStep("Re-registrando memory-guard…", "memory-guard sincronizado", func() error {
		return installer.RegisterMemoryGuardHook(cfg)
	}); err != nil {
		return err
	}
	if err := r.RunStep(fmt.Sprintf("Actualizando pin de Engram a %s…", m.Engram.Version), "Engram sincronizado", func() error {
		return installer.ConfigureEngramMCP(cfg, m)
	}); err != nil {
		return err
	}

	fmt.Fprintln(out, r.Info("Update completo."))
	return nil
}
