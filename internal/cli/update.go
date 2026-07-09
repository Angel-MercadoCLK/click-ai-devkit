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
	// Symmetric with `click install`: re-applying the per-phase model routing config without also
	// re-persisting it to disk left models.json stale (or entirely absent, on a home where it was
	// never written) after `click update` — a real asymmetry bug between the two commands.
	if err := r.RunStep("Guardando modelos por fase de click-sdd…", "Modelos por fase guardados", func() error {
		return installer.SaveModels(cfg, models)
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
	// Same non-fatal binary-provisioning report as `click install` (Slice 3b): SyncEngram already
	// attempted `go install` internally when needed; this just surfaces the resulting state.
	if _, resolvable, err := installer.EngramBinaryResolvable(cfg); err != nil {
		return err
	} else if !resolvable {
		fmt.Fprintln(out, r.Info(installer.EngramBinaryRemediationMessage(m.Engram.Version)))
	}

	context7AlreadyPresent := false
	if err := r.RunStep("Sincronizando Context7 (documentación de librerías)…", "Context7 sincronizado", func() error {
		var syncErr error
		context7AlreadyPresent, syncErr = installer.SyncContext7(cfg)
		return syncErr
	}); err != nil {
		return err
	}
	if context7AlreadyPresent {
		fmt.Fprintln(out, r.Info("Context7 ya estaba configurado — se dejó como está, sin reinstalar."))
	}

	fmt.Fprintln(out, r.Info("Update completo."))
	return nil
}
