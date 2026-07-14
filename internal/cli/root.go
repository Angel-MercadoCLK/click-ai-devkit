// Package cli wires click's cobra command tree. It is intentionally thin: it dispatches to
// internal/installer, internal/doctor, and internal/ui for real logic — click is a thin
// installer/manager, not the orchestration brain (tech-spec.md §1).
package cli

import (
	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/version"
	"github.com/spf13/cobra"
)

// noColorFlag is click's global --no-color flag: it forces plain, ANSI-free output regardless
// of TTY detection or the NO_COLOR env var (ui.Renderer already checks NO_COLOR/TTY on its own;
// this flag is the explicit, highest-priority override).
const noColorFlag = "no-color"

// NewRootCommand builds click's root cobra command with every Slice 1 subcommand wired in, plus
// a default no-arg action: bare `click` on an interactive TTY launches the standing menu
// (internal/menu); otherwise (non-TTY, CI, or --no-interactive) it prints help and exits 0 — see
// rootdefault.go's interactive() gate.
func NewRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:     "click",
		Short:   "click-ai-devkit installer/manager for Claude Code plugins + Engram",
		Version: version.Version,
		RunE:    runRootDefault,
	}

	// Usage dumps belong to genuine usage errors (unknown flags, bad flag values), never to runtime
	// failures — and cobra's own generic post-execute usage print (ExecuteC in command.go) can't
	// tell the two apart: it gates solely on SilenceUsage, with no visibility into *why* execute()
	// failed. Silencing it globally here handles the runtime-failure case (e.g. a failed `click
	// install`), so no irrelevant block of root usage text gets dumped after it — this applies to
	// every root built by NewRootCommand, including the fresh one dispatch() spins up per menu
	// action (rootdefault.go), so a menu-dispatched runtime failure never prints one either.
	root.SilenceUsage = true

	// FlagErrorFunc runs precisely (and only) when pflag fails to parse the command line — unknown
	// flag, missing value, bad value type — before RunE ever executes (command.go's execute():
	// `err = c.ParseFlags(a); if err != nil { return c.FlagErrorFunc()(c, err) }`). That makes it
	// the correct discrimination point for restoring the pre-SilenceUsage usage dump on genuine
	// usage errors without reintroducing it for runtime failures: the generic SilenceUsage gate
	// above still swallows cobra's own post-execute usage print for every error, so this explicitly
	// writes the usage block itself for flag-parse errors only, then returns the original error
	// unchanged so cobra's normal single "Error: ..." line still prints via its usual path.
	// FlagErrorFunc is inherited by every child command that doesn't set its own (cobra walks up
	// via cmd.parent.FlagErrorFunc(), see command.go), so this covers `click install
	// --bad-flag` and every other subcommand, not just the bare root.
	root.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error {
		cmd.Println(cmd.UsageString())
		return err
	})

	root.PersistentFlags().Bool(noColorFlag, false, "Disable colored output (also honors the NO_COLOR env var)")
	root.Flags().Bool(noInteractiveFlag, false, "Skip the interactive menu on bare `click`; print help instead")

	root.AddCommand(
		newInstallCommand(),
		newUpdateCommand(),
		newDoctorCommand(),
		newUninstallCommand(),
		newMemoryGuardCommand(),
		newConfigureModelsCommand(),
		newAgentBuilderCommand(),
	)

	return root
}
