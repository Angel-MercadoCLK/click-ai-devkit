package cli

import (
	"errors"

	"github.com/spf13/cobra"
)

func newConfigureOpenClawModelCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "configure-openclaw-model <provider/model> [fallback-provider/model...]",
		Short: "Comando retirado; configure el modelo con las herramientas nativas de OpenClaw",
		RunE: func(cmd *cobra.Command, args []string) error {
			return errors.New("el selector de modelos de OpenClaw de Click fue retirado; configure proveedor y modelo con las herramientas nativas de OpenClaw")
		},
	}
	cmd.SilenceUsage = true
	return cmd
}
