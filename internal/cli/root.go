// Package cli wires click's cobra command tree. It is intentionally thin: it dispatches to
// internal/installer, internal/doctor, and internal/ui for real logic — click is a thin
// installer/manager, not the orchestration brain (tech-spec.md §1).
package cli

import (
	"github.com/spf13/cobra"
)

// version is the click CLI version. It is a plain var (not a const) so a future release
// pipeline can inject the real value at build time via -ldflags, per tech-spec.md §2.5
// (internal/version). Slice 1 hardcodes a dev placeholder.
var version = "0.1.0-dev"

// noColorFlag is click's global --no-color flag: it forces plain, ANSI-free output regardless
// of TTY detection or the NO_COLOR env var (ui.Renderer already checks NO_COLOR/TTY on its own;
// this flag is the explicit, highest-priority override).
const noColorFlag = "no-color"

// NewRootCommand builds click's root cobra command with every Slice 1 subcommand wired in.
func NewRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:     "click",
		Short:   "click-ai-devkit installer/manager for Claude Code plugins + Engram",
		Version: version,
	}

	root.PersistentFlags().Bool(noColorFlag, false, "Disable colored output (also honors the NO_COLOR env var)")

	root.AddCommand(
		newInstallCommand(),
		newUpdateCommand(),
		newDoctorCommand(),
		newUninstallCommand(),
		newMemoryGuardCommand(),
	)

	return root
}
