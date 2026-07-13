package cli

import (
	"fmt"
	"io"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/installer"
	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/modelconfig"
	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/ui"
	"github.com/spf13/cobra"
)

// newConfigureModelsCommand backs the menu's "Configure models" item: it reuses install.go's
// existing runModelSelectTUI (the same internal/ui.ModelSelectModel screen `click install`
// already drives — not rebuilt here) and persists the result via resolveAndSaveConfiguredModels,
// which preserves the currently persisted orchestration profile label (C1 fix) instead of
// silently dropping it. Hidden from `click --help` since it's primarily reached through the
// standing menu, but still directly runnable (`click configure-models`) for scripts or developers
// who want to skip the menu.
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

	return resolveAndSaveConfiguredModels(cmd, out, r, cfg, runModelSelectTUI)
}

// configureModelsSelector matches runModelSelectTUI's signature so tests can drive
// resolveAndSaveConfiguredModels with a fake TUI result (a real bubbletea program can't be
// exercised headlessly).
type configureModelsSelector func(cmd *cobra.Command) (map[modelconfig.Phase]string, bool, error)

// resolveAndSaveConfiguredModels drives the per-phase model picker and persists the result while
// preserving the label-consistency rule (C1 fix): it loads the currently persisted profile,
// computes the effective label for the newly selected map via modelconfig.EffectiveProfileName
// (the preset name itself if selection still byte-equals that preset, "custom" otherwise), and
// saves both together via installer.SaveModelsWithProfile — never the profile-dropping
// installer.SaveModels this used to call.
func resolveAndSaveConfiguredModels(cmd *cobra.Command, out io.Writer, r *ui.Renderer, cfg installer.Config, selector configureModelsSelector) error {
	selection, cancelled, err := selector(cmd)
	if err != nil {
		return err
	}
	if cancelled {
		fmt.Fprintln(out, r.Info("Selección cancelada."))
		return nil
	}

	loadedProfile, _, _, err := installer.LoadModelsWithProfile(cfg)
	if err != nil {
		return err
	}
	effectiveLabel := modelconfig.EffectiveProfileName(loadedProfile, selection)

	if err := installer.SaveModelsWithProfile(cfg, effectiveLabel, selection); err != nil {
		return err
	}
	fmt.Fprintln(out, r.Info("Modelos por fase guardados."))
	return nil
}
