package cli

import (
	"fmt"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/installer"
	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/manifest"
	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/modelconfig"
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

	// Re-apply whatever per-phase models `click install` saved (D25), so `click update` never
	// silently resets a developer's choice back to defaults. A models.json-less home (installed
	// before this feature existed, or never installed) falls back to defaults.
	models, found, err := installer.LoadModels(cfg)
	if err != nil {
		return err
	}
	if !found {
		models = modelconfig.Defaults()
	}

	if err := r.RunStep("Re-sincronizando plugins click-sdd, click-memory y click-review…", "Plugins sincronizados en Claude Code", func() error {
		return installer.SyncMarketplacePlugins(models)
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
	if err := r.RunStep(fmt.Sprintf("Sincronizando Engram (pin %s)…", m.Engram.Version), "Engram sincronizado", func() error {
		_, err := installer.SyncEngram(cfg, m)
		return err
	}); err != nil {
		return err
	}

	fmt.Fprintln(out, r.Info("Update completo."))
	return nil
}
