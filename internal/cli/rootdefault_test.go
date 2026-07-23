package cli

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/menu"
)

// --- interactive() gate: every branch that must resolve to false (safe, non-hanging). ---

func TestInteractive_NoInteractiveFlagForcesFalse(t *testing.T) {
	if interactive(true, &bytes.Buffer{}, &bytes.Buffer{}) {
		t.Fatal("interactive(noInteractive=true, ...) = true, want false")
	}
}

func TestInteractive_CIEnvForcesFalse(t *testing.T) {
	t.Setenv("CI", "1")
	if interactive(false, &bytes.Buffer{}, &bytes.Buffer{}) {
		t.Fatal("interactive() with CI set = true, want false")
	}
}

func TestInteractive_NonFileOutForcesFalse(t *testing.T) {
	t.Setenv("CI", "")
	if interactive(false, &bytes.Buffer{}, &bytes.Buffer{}) {
		t.Fatal("interactive() with a bytes.Buffer out = true, want false")
	}
}

func TestInteractive_FileOutButNotATerminal_ForcesFalse(t *testing.T) {
	t.Setenv("CI", "")
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error = %v", err)
	}
	defer r.Close()
	defer w.Close()
	if interactive(false, w, &bytes.Buffer{}) {
		t.Fatal("interactive() with a pipe (non-terminal *os.File) out = true, want false")
	}
}

// --- isTerminalWriter / isTerminalReader helpers: direct coverage of the *os.File gate. ---

func TestIsTerminalWriter_NonFileIsFalse(t *testing.T) {
	if isTerminalWriter(&bytes.Buffer{}) {
		t.Fatal("isTerminalWriter(bytes.Buffer) = true, want false")
	}
}

func TestIsTerminalWriter_PipeIsFalse(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error = %v", err)
	}
	defer r.Close()
	defer w.Close()
	if isTerminalWriter(w) {
		t.Fatal("isTerminalWriter(pipe) = true, want false")
	}
}

func TestIsTerminalReader_NonFileIsFalse(t *testing.T) {
	if isTerminalReader(&bytes.Buffer{}) {
		t.Fatal("isTerminalReader(bytes.Buffer) = true, want false")
	}
}

func TestIsTerminalReader_PipeIsFalse(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error = %v", err)
	}
	defer r.Close()
	defer w.Close()
	if isTerminalReader(r) {
		t.Fatal("isTerminalReader(pipe) = true, want false")
	}
}

// --- root command wiring: bare `click` on non-TTY must never hang and must exit 0 with help. ---

func TestRootCommand_NoArgs_NonTTY_PrintsHelpAndExitsCleanly(t *testing.T) {
	home := t.TempDir()
	out, err := execRoot(t, home)
	if err != nil {
		t.Fatalf("bare `click` on non-TTY error = %v, want nil (help + exit 0)", err)
	}
	if !strings.Contains(out, "Usage:") {
		t.Errorf("bare `click` on non-TTY output = %q, want cobra help text (Usage:)", out)
	}
}

func TestRootCommand_NoInteractiveFlag_PrintsHelpAndExitsCleanly(t *testing.T) {
	home := t.TempDir()
	out, err := execRoot(t, home, "--no-interactive")
	if err != nil {
		t.Fatalf("`click --no-interactive` error = %v, want nil", err)
	}
	if !strings.Contains(out, "Usage:") {
		t.Errorf("`click --no-interactive` output = %q, want cobra help text (Usage:)", out)
	}
}

func TestRootCommand_UnknownSubcommand_ReturnsError(t *testing.T) {
	home := t.TempDir()
	if _, err := execRoot(t, home, "totally-bogus-subcommand"); err == nil {
		t.Fatal("`click totally-bogus-subcommand` error = nil, want an error (typo should not silently launch help)")
	}
}

// --- explicit subcommands keep working: dispatching one no-arg command each is enough to guard
// --- against the RunE-on-root change accidentally breaking normal subcommand routing. ---

func TestRootCommand_ExplicitSubcommands_StillDispatch(t *testing.T) {
	home := t.TempDir()
	if _, err := execRoot(t, home, "doctor"); err == nil {
		t.Fatal("`click doctor` on an empty home error = nil, want the expected unhealthy error (proves doctor's own RunE still runs, not the menu)")
	}
}

// --- error/usage printing: a failing command must be reported to the user exactly once. Runtime
// --- failures (RunE returns an error) must never get an irrelevant usage dump — whether invoked
// --- directly or dispatched from the standing menu. Genuine flag-parse/usage errors (unknown
// --- flag, bad flag value) must still show the Usage: block, exactly like pre-SilenceUsage cobra.
// --- Regression tests for R4-001 (double "Error:" print + irrelevant root usage dump on
// --- menu-dispatched runtime failures) and RR1-001 (global SilenceUsage over-suppressed genuine
// --- flag-parse/usage errors too, which must keep showing Usage:).

func TestRootCommand_UnknownFlag_PrintsErrorAndUsage(t *testing.T) {
	home := t.TempDir()
	out, err := execRoot(t, home, "install", "--this-flag-does-not-exist")
	if err == nil {
		t.Fatal("`click install --this-flag-does-not-exist` error = nil, want a non-nil error")
	}
	if n := strings.Count(out, "Error:"); n != 1 {
		t.Fatalf("output contains %d \"Error:\" line(s), want exactly 1:\n%s", n, out)
	}
	if !strings.Contains(out, "Usage:") {
		t.Fatalf("output contains no Usage: block, want one on a genuine flag-parse error:\n%s", out)
	}
}

func TestRootCommand_BadFlagValue_PrintsErrorAndUsage(t *testing.T) {
	home := t.TempDir()
	out, err := execRoot(t, home, "--no-color=not-a-bool")
	if err == nil {
		t.Fatal("`click --no-color=not-a-bool` error = nil, want a non-nil error (bad bool value)")
	}
	if n := strings.Count(out, "Error:"); n != 1 {
		t.Fatalf("output contains %d \"Error:\" line(s), want exactly 1:\n%s", n, out)
	}
	if !strings.Contains(out, "Usage:") {
		t.Fatalf("output contains no Usage: block, want one on a genuine flag-parse error:\n%s", out)
	}
}

// doctor.go self-silences (cmd.SilenceErrors = true) and reports failure via its own Fail lines
// instead of cobra's generic "Error: ..." print — deliberate, pre-existing, and out of scope here
// — so these two tests assert the two things this fix actually governs: the failure is genuinely
// reported (non-nil error, non-empty output) and no irrelevant Usage: block leaks through.

func TestRootCommand_DirectSubcommandRuntimeFailure_PrintsErrorExactlyOnceNoUsage(t *testing.T) {
	home := t.TempDir()
	out, err := execRoot(t, home, "doctor")
	if err == nil {
		t.Fatal("`click doctor` on an empty home error = nil, want the expected unhealthy error")
	}
	if !strings.Contains(out, "click-ai-devkit no está instalado correctamente") {
		t.Fatalf("output missing doctor's own unhealthy report, want exactly one failure report:\n%s", out)
	}
	if strings.Contains(out, "Usage:") {
		t.Fatalf("output contains a Usage: dump, want none on a runtime failure:\n%s", out)
	}
}

func TestDispatch_SubcommandRuntimeFailure_PrintsErrorExactlyOnceNoUsageDump(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CLICK_CLAUDE_HOME", home)

	parent := &cobra.Command{}
	var buf bytes.Buffer
	parent.SetOut(&buf)
	parent.SetErr(&buf)
	parent.SetIn(&bytes.Buffer{})

	err := dispatch(parent, []string{"doctor"})
	if err == nil {
		t.Fatal("dispatch() error = nil, want the expected unhealthy error from `doctor` on an empty home")
	}
	out := buf.String()
	if !strings.Contains(out, "click-ai-devkit no está instalado correctamente") {
		t.Fatalf("dispatch() output missing doctor's own unhealthy report, want exactly one failure report:\n%s", out)
	}
	if strings.Contains(out, "Usage:") {
		t.Fatalf("dispatch() output contains a Usage: dump, want none on a runtime failure:\n%s", out)
	}
}

func TestDispatch_FlagError_PrintsUsage(t *testing.T) {
	parent := &cobra.Command{}
	var buf bytes.Buffer
	parent.SetOut(&buf)
	parent.SetErr(&buf)
	parent.SetIn(&bytes.Buffer{})

	err := dispatch(parent, []string{"install", "--this-flag-does-not-exist"})
	if err == nil {
		t.Fatal("dispatch() error = nil, want a non-nil error for an unknown flag")
	}
	out := buf.String()
	if !strings.Contains(out, "Usage:") {
		t.Fatalf("dispatch() output contains no Usage: block, want one on a genuine flag-parse error:\n%s", out)
	}
}

func TestSilenceIfAlreadyReported_DispatchError_SilencesRoot(t *testing.T) {
	cmd := &cobra.Command{}
	err := &errMenuDispatchFailed{err: errors.New("boom")}
	silenceIfAlreadyReported(cmd, err)
	if !cmd.SilenceErrors {
		t.Fatal("cmd.SilenceErrors = false after a dispatch-originated error, want true (avoid a second, redundant print by the outer live root's own Execute())")
	}
}

func TestSilenceIfAlreadyReported_OtherError_LeavesRootUnsilenced(t *testing.T) {
	cmd := &cobra.Command{}
	silenceIfAlreadyReported(cmd, errors.New("launch menu failed"))
	if cmd.SilenceErrors {
		t.Fatal("cmd.SilenceErrors = true for a non-dispatch error, want false (this error was never shown yet, so cobra's own single auto-print must still run)")
	}
}

func TestErrMenuDispatchFailed_UnwrapsToOriginalError(t *testing.T) {
	inner := errors.New("boom")
	wrapped := &errMenuDispatchFailed{err: inner}
	if !errors.Is(wrapped, inner) {
		t.Fatal("errors.Is(wrapped, inner) = false, want true (Unwrap must expose the original error)")
	}
	if wrapped.Error() != inner.Error() {
		t.Fatalf("wrapped.Error() = %q, want %q", wrapped.Error(), inner.Error())
	}
}

// --- dispatch(): pure-ish helper that runs a fresh command tree for the chosen menu action. ---

func TestDispatch_RunsFreshCommandTreeAgainstProvidedStreams(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CLICK_CLAUDE_HOME", home)

	parent := &cobra.Command{}
	var buf bytes.Buffer
	parent.SetOut(&buf)
	parent.SetErr(&buf)
	parent.SetIn(&bytes.Buffer{})

	// doctor on an empty home reports unhealthy (non-nil error) but must still write its report
	// to the streams dispatch forwarded from parent — proving dispatch attaches a fresh command
	// tree to the caller's own I/O rather than falling back to the real os.Stdout/os.Stdin.
	_ = dispatch(parent, []string{"doctor"})
	if buf.Len() == 0 {
		t.Fatal("dispatch(parent, [doctor]) wrote nothing to parent's Out, want the doctor report")
	}
}

func TestDispatch_EmptyArgsIsANoOp(t *testing.T) {
	parent := &cobra.Command{}
	var buf bytes.Buffer
	parent.SetOut(&buf)
	parent.SetErr(&buf)
	parent.SetIn(&bytes.Buffer{})

	if err := dispatch(parent, nil); err != nil {
		t.Fatalf("dispatch(parent, nil) error = %v, want nil (no-op)", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("dispatch(parent, nil) wrote %q, want nothing", buf.String())
	}
}

// --- configure-models: must never spin up its TUI on non-TTY output. ---

func TestConfigureModelsCommand_NonTTY_NoHangPrintsMessage(t *testing.T) {
	home := t.TempDir()
	out, err := execRoot(t, home, "configure-models")
	if err != nil {
		t.Fatalf("`click configure-models` on non-TTY error = %v, want nil", err)
	}
	if out == "" {
		t.Fatal("`click configure-models` on non-TTY printed nothing, want a no-terminal message")
	}
}

// --- runMenuLoop: the menu must return control to itself after dispatching an active item,
// --- instead of exiting the process. launchMenu/dispatchFn are injected so this control-flow is
// --- unit-tested without a real bubbletea program (mirrors dispatch()'s own testing pattern).

func TestRunMenuLoop_QuitImmediatelyReturnsNilWithoutDispatching(t *testing.T) {
	launchCalls := 0
	launchMenu := func() (string, error) {
		launchCalls++
		return menu.ActionQuit, nil
	}
	dispatchCalls := 0
	dispatchFn := func(args []string) error {
		dispatchCalls++
		return nil
	}

	if err := runMenuLoop(launchMenu, dispatchFn); err != nil {
		t.Fatalf("runMenuLoop(quit) error = %v, want nil", err)
	}
	if launchCalls != 1 {
		t.Fatalf("launchMenu called %d times, want 1 (no loop back after quit)", launchCalls)
	}
	if dispatchCalls != 0 {
		t.Fatalf("dispatchFn called %d times, want 0 (quit never dispatches)", dispatchCalls)
	}
}

func TestRunMenuLoop_DispatchesThenLoopsBackToMenuUntilQuit(t *testing.T) {
	chosen := []string{menu.ActionInstall, menu.ActionDoctor, menu.ActionQuit}
	launchCalls := 0
	launchMenu := func() (string, error) {
		got := chosen[launchCalls]
		launchCalls++
		return got, nil
	}
	var dispatched [][]string
	dispatchFn := func(args []string) error {
		dispatched = append(dispatched, args)
		return nil
	}

	if err := runMenuLoop(launchMenu, dispatchFn); err != nil {
		t.Fatalf("runMenuLoop error = %v, want nil", err)
	}
	if launchCalls != 3 {
		t.Fatalf("launchMenu called %d times, want 3 (menu re-launched after each dispatched action)", launchCalls)
	}
	if len(dispatched) != 2 {
		t.Fatalf("dispatchFn called %d times, want 2 (install, doctor — not quit)", len(dispatched))
	}
	if len(dispatched[0]) != 1 || dispatched[0][0] != "install" {
		t.Fatalf("first dispatch = %v, want [install]", dispatched[0])
	}
	if len(dispatched[1]) != 1 || dispatched[1][0] != "doctor" {
		t.Fatalf("second dispatch = %v, want [doctor]", dispatched[1])
	}
}

func TestRunMenuLoop_LaunchMenuErrorStopsLoopWithoutDispatching(t *testing.T) {
	wantErr := errors.New("boom")
	launchMenu := func() (string, error) {
		return "", wantErr
	}
	dispatchCalls := 0
	dispatchFn := func(args []string) error {
		dispatchCalls++
		return nil
	}

	if err := runMenuLoop(launchMenu, dispatchFn); !errors.Is(err, wantErr) {
		t.Fatalf("runMenuLoop error = %v, want %v", err, wantErr)
	}
	if dispatchCalls != 0 {
		t.Fatalf("dispatchFn called %d times, want 0 (launchMenu failed before any dispatch)", dispatchCalls)
	}
}

func TestRunMenuLoop_DispatchErrorStopsLoopWithoutRelaunching(t *testing.T) {
	wantErr := errors.New("dispatch failed")
	launchCalls := 0
	launchMenu := func() (string, error) {
		launchCalls++
		return menu.ActionInstall, nil
	}
	dispatchFn := func(args []string) error {
		return wantErr
	}

	if err := runMenuLoop(launchMenu, dispatchFn); !errors.Is(err, wantErr) {
		t.Fatalf("runMenuLoop error = %v, want %v", err, wantErr)
	}
	if launchCalls != 1 {
		t.Fatalf("launchMenu called %d times, want 1 (loop must stop after dispatch error, not relaunch)", launchCalls)
	}
}

// TestRunMenuLoop_ConfigureOpenClawModelDispatchDoesNotCrashMenu reproduces the exact FIX 1 crash
// scenario end-to-end: selecting the "Configurar modelo nativo de OpenClaw" menu row dispatches
// `configure-openclaw-model` with no args. Before the fix this errored out of runMenuLoop and killed
// the whole menu. Now the loop must survive it: dispatch returns nil, the menu relaunches, and only
// the following Quit ends the loop (launchMenu called twice, not one).
func TestRunMenuLoop_ConfigureOpenClawModelDispatchDoesNotCrashMenu(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CLICK_CLAUDE_HOME", home)
	t.Setenv("CLICK_OPENCLAW_HOME", t.TempDir())
	seedResolvableGit(t)

	parent := &cobra.Command{}
	var buf bytes.Buffer
	parent.SetOut(&buf)
	parent.SetErr(&buf)
	parent.SetIn(&bytes.Buffer{})

	chosen := []string{menu.ActionConfigureOpenClawModel, menu.ActionQuit}
	launchCalls := 0
	launchMenu := func() (string, error) {
		got := chosen[launchCalls]
		launchCalls++
		return got, nil
	}
	dispatchFn := func(args []string) error { return dispatch(parent, args) }

	if err := runMenuLoop(launchMenu, dispatchFn); err != nil {
		t.Fatalf("runMenuLoop dispatching ActionConfigureOpenClawModel error = %v, want nil (menu must survive the no-args item)", err)
	}
	if launchCalls != 2 {
		t.Fatalf("launchMenu called %d times, want 2 (menu relaunched after the item, then quit) — a dispatch error would have stopped the loop early", launchCalls)
	}
	if !strings.Contains(buf.String(), "Indique el modelo con") {
		t.Fatalf("dispatched output = %q, want the no-args guidance line", buf.String())
	}
}

func TestRunMenuLoop_EmptyChosenReturnsNil(t *testing.T) {
	launchMenu := func() (string, error) {
		return "", nil
	}
	dispatchCalls := 0
	dispatchFn := func(args []string) error {
		dispatchCalls++
		return nil
	}

	if err := runMenuLoop(launchMenu, dispatchFn); err != nil {
		t.Fatalf("runMenuLoop(empty chosen) error = %v, want nil", err)
	}
	if dispatchCalls != 0 {
		t.Fatalf("dispatchFn called %d times, want 0 (empty Chosen means nothing to dispatch)", dispatchCalls)
	}
}
