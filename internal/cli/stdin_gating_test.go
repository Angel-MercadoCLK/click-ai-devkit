package cli

import (
	"bytes"
	"io"
	"testing"
)

// forceTerminalDetection overrides the shared isTerminalWriter/isTerminalReader seams
// (rootdefault.go) for one test and restores them afterward. A real TTY cannot be faked with a
// bytes.Buffer, so this is the only way to exercise the "stdout IS a terminal, stdin is NOT" path
// deterministically.
func forceTerminalDetection(t *testing.T, writerIsTTY, readerIsTTY bool) {
	t.Helper()
	prevWriter := isTerminalWriter
	prevReader := isTerminalReader
	isTerminalWriter = func(io.Writer) bool { return writerIsTTY }
	isTerminalReader = func(io.Reader) bool { return readerIsTTY }
	t.Cleanup(func() {
		isTerminalWriter = prevWriter
		isTerminalReader = prevReader
	})
}

// TestIsNonInteractiveInstall_TTYStdoutNonTTYStdin_IsNonInteractive is the FIX 3 regression: when
// stdout is a real terminal but stdin is NOT (piped/redirected/service), `click install` must be
// treated as non-interactive so it never launches the alt-screen wizard (runInstallSelectTUI) or
// blocks forever on confirmProceed's ReadString. Before the fix, isNonInteractiveInstall checked
// only stdout, so this exact combination hung.
func TestIsNonInteractiveInstall_TTYStdoutNonTTYStdin_IsNonInteractive(t *testing.T) {
	forceTerminalDetection(t, true /* stdout is a TTY */, false /* stdin is NOT a TTY */)

	cmd := newInstallCommand()
	cmd.SetIn(&bytes.Buffer{})

	if !isNonInteractiveInstall(cmd, &bytes.Buffer{}) {
		t.Fatal("isNonInteractiveInstall = false for TTY stdout + non-TTY stdin, want true (must not launch the wizard or block on ReadString)")
	}
}

// TestIsNonInteractiveInstall_BothStreamsTTY_IsInteractive is the complementary proof that the
// stdout-only regression was not "fixed" by simply forcing non-interactive always: with BOTH streams
// real terminals and no --yes flag, isNonInteractiveInstall must return false (the interactive path).
func TestIsNonInteractiveInstall_BothStreamsTTY_IsInteractive(t *testing.T) {
	forceTerminalDetection(t, true, true)

	cmd := newInstallCommand()
	cmd.SetIn(&bytes.Buffer{})

	if isNonInteractiveInstall(cmd, &bytes.Buffer{}) {
		t.Fatal("isNonInteractiveInstall = true when BOTH streams are terminals and no --yes flag, want false (interactive)")
	}
}
