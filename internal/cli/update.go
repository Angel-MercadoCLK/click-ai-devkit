package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newUpdateCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "update",
		Short: "Re-sync plugins and the Engram pin to the currently installed click binary",
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			r := rendererFor(cmd, out)
			fmt.Fprintln(out, r.Info("update: coming in a later slice"))
			return nil
		},
	}
}
