package cli

import (
	"errors"
	"fmt"
	"io"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/installer"
	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/modelconfig"
	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/ui"
	"github.com/spf13/cobra"
)

// newConfigureModelsCommand remains as a compatibility entrypoint for scripts that used the
// retired Click-owned model selector. It deliberately performs no prompt or configuration write.
func newConfigureModelsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "configure-models",
		Short:  "Comando retirado; la selección de modelos compatibles pertenece a gentle-ai",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfigureModels(cmd)
		},
	}
	return cmd
}

func runConfigureModels(_ *cobra.Command) error {
	return errors.New("el selector de modelos de Click fue retirado; para roles compatibles, use la configuración interactiva de Configure Models de gentle-ai")
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
