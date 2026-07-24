package cli

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/installer"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-isatty"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/menu"
	"github.com/spf13/cobra"
)

// noInteractiveFlag lets a developer/script force bare `click`'s default action to skip the
// standing menu and print help instead, without needing to fake a non-TTY environment.
const noInteractiveFlag = "no-interactive"

// runRootDefault is root's RunE: it only ever runs when no registered subcommand matched (bare
// `click`, or `click` followed by an unrecognized token). A non-empty args here means the first
// token wasn't a known subcommand — report it exactly like cobra's own unknown-command error
// rather than silently launching the menu or printing help, so typos stay loud.
func runRootDefault(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("unknown command %q for %q", args[0], cmd.CommandPath())
	}

	out := cmd.OutOrStdout()
	in := cmd.InOrStdin()
	noInteractive, _ := cmd.Flags().GetBool(noInteractiveFlag)

	if !interactive(noInteractive, out, in) {
		return cmd.Help()
	}

	launchMenu := func() (string, error) {
		program := tea.NewProgram(menu.NewModelWithItems(rootMenuItems()), tea.WithInput(in), tea.WithOutput(out))
		finalModel, err := program.Run()
		if err != nil {
			return "", fmt.Errorf("cli: run menu TUI: %w", err)
		}
		return finalModel.(menu.Model).Chosen, nil
	}
	dispatchFn := func(args []string) error {
		return dispatch(cmd, args)
	}
	err := runMenuLoop(launchMenu, dispatchFn)
	// A dispatch-originated failure has already been shown to the user exactly once (either
	// cobra's own single "Error: ..." line from dispatch()'s inner Execute() call, or the
	// subcommand's own self-silenced report, e.g. `doctor`'s Fail lines). Without this, the outer
	// live root's own Execute() would print the same error a second, confusing time once it
	// propagates back up through this RunE (R4-001).
	silenceIfAlreadyReported(cmd, err)
	return err
}

func rootMenuItems() []menu.Item {
	items := menu.DefaultItems()
	status := installer.OpenClawNativeModelActionStatus()
	if status.Available {
		return items
	}
	for i := range items {
		if items[i].Action != menu.ActionConfigureOpenClawModel {
			continue
		}
		items[i].Active = false
		items[i].Hint = status.Detail
		items[i].InactiveLabel = "no disponible"
		break
	}
	return items
}

// errMenuDispatchFailed wraps an error already surfaced to the user by dispatch()'s inner
// Execute() call, so runRootDefault can tell it apart from an error nobody has reported yet (e.g.
// a launchMenu/bubbletea failure) and silence the outer live root's own redundant auto-print.
type errMenuDispatchFailed struct{ err error }

func (e *errMenuDispatchFailed) Error() string { return e.err.Error() }
func (e *errMenuDispatchFailed) Unwrap() error { return e.err }

// silenceIfAlreadyReported sets cmd.SilenceErrors when err was already shown to the user once by
// dispatch(), so the outer live root's own ExecuteC doesn't print it again. Any other error (e.g.
// launchMenu failing before ever reaching dispatch) is left alone: it still needs cobra's normal
// single auto-print, since nothing else has reported it yet.
func silenceIfAlreadyReported(cmd *cobra.Command, err error) {
	var dispatchErr *errMenuDispatchFailed
	if errors.As(err, &dispatchErr) {
		cmd.SilenceErrors = true
	}
}

// runMenuLoop is the standing menu's control-flow: it launches the menu, and — if the user chose
// a dispatchable active item — dispatches it and re-launches the menu again, repeating until the
// user quits (menu.ActionQuit, or any chosen value that maps to no dispatch args) or an error
// occurs. Only quitting (or an error) ends the loop; a completed dispatch always returns control
// to the menu instead of exiting the process. launchMenu and dispatchFn are injected so this
// control-flow can be unit-tested without a real bubbletea program.
func runMenuLoop(launchMenu func() (string, error), dispatchFn func([]string) error) error {
	for {
		chosen, err := launchMenu()
		if err != nil {
			return err
		}
		actionArgs := menu.ActionArgs(chosen)
		if len(actionArgs) == 0 {
			return nil
		}
		if err := dispatchFn(actionArgs); err != nil {
			return err
		}
	}
}

// dispatch runs a chosen menu action through a brand-new cobra command tree — NOT the live root
// that's already executing — so subcommand flag state never re-enters the running Execute()
// call. It's attached to the caller's own streams (falling back to the real os.Stdout/os.Stdin
// only when the caller never set any, exactly like any other directly-invoked click command).
func dispatch(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return nil
	}
	fresh := NewRootCommand()
	fresh.SetOut(cmd.OutOrStdout())
	fresh.SetErr(cmd.ErrOrStderr())
	fresh.SetIn(cmd.InOrStdin())
	fresh.SetArgs(args)
	if err := fresh.Execute(); err != nil {
		return &errMenuDispatchFailed{err: err}
	}
	return nil
}

// interactive decides whether bare `click` should launch the standing menu. It must resolve to
// false — safe, never hanging — whenever: --no-interactive was passed, the CI env var is set, or
// either stdout or stdin is not a real terminal. Both streams are checked independently because
// bubbletea starves on piped stdin even when stdout is a real TTY. Takes io.Writer/io.Reader
// (never touching os.Stdout/os.Stdin directly) so tests can force the non-TTY branch
// deterministically with bytes.Buffer.
func interactive(noInteractive bool, out io.Writer, in io.Reader) bool {
	if noInteractive {
		return false
	}
	if os.Getenv("CI") != "" {
		return false
	}
	if !isTerminalWriter(out) {
		return false
	}
	return isTerminalReader(in)
}

// isTerminalWriter reports whether out is a real terminal: the same *os.File + isatty pattern
// used by ui.shouldUseColor and cli.isNonInteractiveInstall (install.go). It is a package-level var
// (not a plain func) purely so tests can override the terminal detection deterministically — a real
// TTY cannot be faked with a bytes.Buffer.
var isTerminalWriter = func(out io.Writer) bool {
	f, ok := out.(*os.File)
	if !ok {
		return false
	}
	return isatty.IsTerminal(f.Fd()) || isatty.IsCygwinTerminal(f.Fd())
}

// isTerminalReader mirrors isTerminalWriter for stdin: bubbletea needs a real terminal on both
// ends, and piped/redirected stdin (even with a TTY stdout) must never launch the menu / wizard or
// block on a confirmation read. Also a var for the same test-override reason as isTerminalWriter.
var isTerminalReader = func(in io.Reader) bool {
	f, ok := in.(*os.File)
	if !ok {
		return false
	}
	return isatty.IsTerminal(f.Fd()) || isatty.IsCygwinTerminal(f.Fd())
}
