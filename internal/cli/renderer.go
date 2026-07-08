package cli

import (
	"io"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/ui"
	"github.com/spf13/cobra"
)

// rendererFor builds a ui.Renderer for cmd's --no-color flag (inherited from the persistent
// flag registered on the root command) writing to out.
func rendererFor(cmd *cobra.Command, out io.Writer) *ui.Renderer {
	noColor, _ := cmd.Flags().GetBool(noColorFlag)
	return ui.NewRenderer(out, noColor)
}
