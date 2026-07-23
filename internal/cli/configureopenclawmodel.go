package cli

import (
	"fmt"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/installer"
	"github.com/spf13/cobra"
)

func newConfigureOpenClawModelCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "configure-openclaw-model <provider/model> [fallback-provider/model...]",
		Short: "Configura el modelo nativo de OpenClaw mediante su CLI oficial",
		RunE: func(cmd *cobra.Command, args []string) error {
			// The "need a primary model ref" requirement lives here in RunE, NOT in cobra's Args:
			// an Args error would propagate up through the standing menu's runMenuLoop, which
			// returns on any dispatch failure and thereby TERMINATES the whole interactive menu
			// session. Selecting this row from the menu supplies zero args (menu.ActionArgs maps it
			// to just ["configure-openclaw-model"]), so an Args-level error made the item impossible
			// to run from the menu without crashing it.
			//
			// With no args we degrade gracefully instead of erroring. There is no simple reusable
			// text-input widget in internal/ui to collect the ref interactively — internal/ui's
			// agentbuilder input is a bespoke multi-step wizard, not a standalone component — so per
			// the fix's guidance we do NOT hand-roll a fragile one-off bubbletea input here. On both
			// the non-TTY path (mirroring runConfigureTargets' isTerminalWriter guard) and the TTY
			// path we print the same Spanish guidance line and return nil: no crash, no error.
			if len(args) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "Indique el modelo con: click configure-openclaw-model <provider/model> [fallbacks...]")
				return nil
			}
			if err := installer.ConfigureOpenClawModels(args[0], args[1:]); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Configuración nativa de modelos de OpenClaw guardada.")
			return nil
		},
	}
	cmd.SilenceUsage = true
	return cmd
}
