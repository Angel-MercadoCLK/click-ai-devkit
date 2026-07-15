package cli

import (
	"fmt"
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

// surfacePathWarning prints pathWarning via r.Warn to out, or does nothing when pathWarning is
// empty (no PATH-persistence attempt was made, or it succeeded). Shared by runInstall and
// runUpdate — both call installer.SyncEngram and must surface its pathWarning return the same way
// (design D-5, sdd/engram-mcp-resolution obs #1436) — this closes the original bug's root failure
// mode: a PATH-persistence failure silently vanishing behind an otherwise-successful install.
//
// Kept as its own small function so this wiring stays unit-testable directly, without needing to
// drive a real pathStore through installer.SyncEngram: pathStore only ever activates when the
// resolved Engram binary sits inside a real `go env`-derived GoBinDir, and CLI-level tests
// deliberately never stub `go env` — so this package would otherwise have no safe, hermetic way to
// exercise the "pathWarning is non-empty" branch of install.go/update.go's own wiring at all.
func surfacePathWarning(out io.Writer, r *ui.Renderer, pathWarning string) {
	if pathWarning == "" {
		return
	}
	fmt.Fprintln(out, r.Warn(pathWarning))
}
