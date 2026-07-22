package installer

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- Task 3.9's GREEN target, tested first as RED (SyncOpenClawPlugin/RemoveOpenClawPlugin/
// osExecutable/SetOSExecutableForTests do not exist until openclawplugin.go's GREEN change) ---

// TestSyncOpenClawPlugin_WritesHooksJSAndManifest_WithTemplatedClickBinPath is task 3.9's core RED
// test: with cfg.OpenClawHome populated, SyncOpenClawPlugin must write hooks.js and plugin.json
// under cfg.OpenClawPluginDir(), with {{CLICK_BIN}} replaced by the resolved absolute click path.
func TestSyncOpenClawPlugin_WritesHooksJSAndManifest_WithTemplatedClickBinPath(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir(), OpenClawHome: t.TempDir()}
	// Deliberately forward-slash-only (not filepath.Join, which would emit backslashes on Windows —
	// this session's own platform) so this general test can assert a plain substring match without
	// getting entangled with escapeForJSSingleQuotedString's backslash-doubling behavior; THAT
	// behavior has its own dedicated test below
	// (TestSyncOpenClawPlugin_WindowsBackslashPath_EscapedForJSStringLiteral).
	wantBin := "/opt/click/bin/click"
	restore := SetOSExecutableForTests(func() (string, error) { return wantBin, nil })
	defer restore()

	if err := SyncOpenClawPlugin(cfg); err != nil {
		t.Fatalf("SyncOpenClawPlugin() error = %v", err)
	}

	hooksPath := filepath.Join(cfg.OpenClawPluginDir(), "plugins", "hooks.js")
	data, err := os.ReadFile(hooksPath)
	if err != nil {
		t.Fatalf("ReadFile(hooks.js) error = %v, want it written when OpenClaw is detected", err)
	}
	content := string(data)
	if strings.Contains(content, "{{CLICK_BIN}}") {
		t.Fatalf("hooks.js content still contains the {{CLICK_BIN}} placeholder, want it templated")
	}
	if !strings.Contains(content, wantBin) {
		t.Fatalf("hooks.js content = %q, want it to contain the resolved click binary path %q", content, wantBin)
	}

	manifestPath := filepath.Join(cfg.OpenClawPluginDir(), "plugin.json")
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("ReadFile(plugin.json) error = %v, want it written alongside hooks.js", err)
	}
	if !strings.Contains(string(manifestData), "click-memory-guard") {
		t.Fatalf("plugin.json content = %q, want it to describe the click-memory-guard plugin", manifestData)
	}
}

// TestSyncOpenClawPlugin_Absent_NoOp mirrors SyncOpenClawWorkspace/SyncOpenClawMCPConfig's own
// absent-guard tests: cfg.OpenClawHome == "" must write nothing anywhere.
func TestSyncOpenClawPlugin_Absent_NoOp(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir()}
	if err := SyncOpenClawPlugin(cfg); err != nil {
		t.Fatalf("SyncOpenClawPlugin() error = %v, want nil no-op when OpenClawHome is empty", err)
	}
}

// TestSyncOpenClawPlugin_ExecutableResolutionFails_FallsBackToBareClick covers OCG-5's documented
// fallback branch: when osExecutable fails, hooks.js must be templated with the bare command name
// "click", not left with the placeholder or an error.
func TestSyncOpenClawPlugin_ExecutableResolutionFails_FallsBackToBareClick(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir(), OpenClawHome: t.TempDir()}
	restore := SetOSExecutableForTests(func() (string, error) {
		return "", errors.New("os.Executable: not supported on this platform")
	})
	defer restore()

	if err := SyncOpenClawPlugin(cfg); err != nil {
		t.Fatalf("SyncOpenClawPlugin() error = %v", err)
	}
	data, err := os.ReadFile(filepath.Join(cfg.OpenClawPluginDir(), "plugins", "hooks.js"))
	if err != nil {
		t.Fatalf("ReadFile(hooks.js) error = %v", err)
	}
	if !strings.Contains(string(data), "CLICK_BIN = 'click';") {
		t.Fatalf("hooks.js content = %q, want the bare \"click\" fallback when os.Executable() fails", data)
	}
}

// TestSyncOpenClawPlugin_WindowsBackslashPath_EscapedForJSStringLiteral is a deviation beyond the
// original 14 tasks, added after discovering a real correctness bug while implementing OCG-5's
// templating: an unescaped backslash inside hooks.js's single-quoted CLICK_BIN string literal is
// silently DROPPED by JS string parsing (e.g. `\U` -> `U`), which would corrupt a Windows click
// binary path (this session's own platform) the moment Node loads hooks.js. Backslashes must be
// doubled before substitution.
func TestSyncOpenClawPlugin_WindowsBackslashPath_EscapedForJSStringLiteral(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir(), OpenClawHome: t.TempDir()}
	winPath := `C:\Users\CLK090\bin\click.exe`
	restore := SetOSExecutableForTests(func() (string, error) { return winPath, nil })
	defer restore()

	if err := SyncOpenClawPlugin(cfg); err != nil {
		t.Fatalf("SyncOpenClawPlugin() error = %v", err)
	}
	data, err := os.ReadFile(filepath.Join(cfg.OpenClawPluginDir(), "plugins", "hooks.js"))
	if err != nil {
		t.Fatalf("ReadFile(hooks.js) error = %v", err)
	}
	content := string(data)
	wantEscaped := `C:\\Users\\CLK090\\bin\\click.exe`
	if !strings.Contains(content, wantEscaped) {
		t.Fatalf("hooks.js content = %q, want doubled backslashes %q for a valid JS single-quoted string literal", content, wantEscaped)
	}
	if strings.Contains(content, `'C:\Users`) {
		t.Fatalf("hooks.js content = %q, contains an UNESCAPED backslash right after the opening quote — this silently corrupts the path when Node parses the string literal (e.g. \\U -> U)", content)
	}
}

// TestSyncOpenClawPlugin_Rerun_ByteIdenticalOutput is task 3.6's idempotency half: re-running
// SyncOpenClawPlugin with an unchanged resolved click path must produce byte-identical output.
func TestSyncOpenClawPlugin_Rerun_ByteIdenticalOutput(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir(), OpenClawHome: t.TempDir()}
	restore := SetOSExecutableForTests(func() (string, error) { return "/opt/click/bin/click", nil })
	defer restore()

	hooksPath := filepath.Join(cfg.OpenClawPluginDir(), "plugins", "hooks.js")

	if err := SyncOpenClawPlugin(cfg); err != nil {
		t.Fatalf("SyncOpenClawPlugin() 1st run error = %v", err)
	}
	first, err := os.ReadFile(hooksPath)
	if err != nil {
		t.Fatalf("ReadFile(hooks.js) after 1st run error = %v", err)
	}

	if err := SyncOpenClawPlugin(cfg); err != nil {
		t.Fatalf("SyncOpenClawPlugin() 2nd run error = %v", err)
	}
	second, err := os.ReadFile(hooksPath)
	if err != nil {
		t.Fatalf("ReadFile(hooks.js) after 2nd run error = %v", err)
	}

	if string(first) != string(second) {
		t.Fatalf("hooks.js content changed across re-run:\n1st=%q\n2nd=%q\nwant byte-identical (idempotent re-copy)", first, second)
	}
}

// TestOpenClawPlugin_SnapshotAndRestore_RestoresByteForByte is task 3.6's rollback half: a
// run-start snapshot taken after the plugin is installed, followed by a hand-edit (simulating drift
// or corruption) and a RestoreRun, must bring hooks.js back byte-for-byte to its pre-tamper content
// — proving the plugin's files ride PR-B's existing per-target snapshot/rollback mechanism for free,
// with zero snapshot.go logic beyond the snapshotSources entries this batch adds.
func TestOpenClawPlugin_SnapshotAndRestore_RestoresByteForByte(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir(), OpenClawHome: t.TempDir()}
	restore := SetOSExecutableForTests(func() (string, error) { return "/opt/click/bin/click", nil })
	defer restore()

	if err := SyncOpenClawPlugin(cfg); err != nil {
		t.Fatalf("SyncOpenClawPlugin() error = %v", err)
	}
	hooksPath := filepath.Join(cfg.OpenClawPluginDir(), "plugins", "hooks.js")
	original, err := os.ReadFile(hooksPath)
	if err != nil {
		t.Fatalf("ReadFile(hooks.js) error = %v", err)
	}

	if err := SnapshotRun(cfg); err != nil {
		t.Fatalf("SnapshotRun() error = %v", err)
	}

	if err := os.WriteFile(hooksPath, []byte("tampered content, not what SyncOpenClawPlugin wrote"), 0o644); err != nil {
		t.Fatalf("WriteFile(tamper) error = %v", err)
	}

	if err := RestoreRun(cfg); err != nil {
		t.Fatalf("RestoreRun() error = %v", err)
	}

	restored, err := os.ReadFile(hooksPath)
	if err != nil {
		t.Fatalf("ReadFile(hooks.js) after restore error = %v", err)
	}
	if string(restored) != string(original) {
		t.Fatalf("restored hooks.js = %q, want byte-identical to pre-tamper content %q", restored, original)
	}
}

// --- Task 3.13's supporting RED coverage (RemoveOpenClawPlugin) ---

func TestRemoveOpenClawPlugin_RemovesEntireDir(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir(), OpenClawHome: t.TempDir()}
	restore := SetOSExecutableForTests(func() (string, error) { return "/opt/click/bin/click", nil })
	defer restore()

	if err := SyncOpenClawPlugin(cfg); err != nil {
		t.Fatalf("SyncOpenClawPlugin() error = %v", err)
	}
	if _, err := os.Stat(cfg.OpenClawPluginDir()); err != nil {
		t.Fatalf("Stat(plugin dir) before removal error = %v, want it to exist first", err)
	}

	if err := RemoveOpenClawPlugin(cfg); err != nil {
		t.Fatalf("RemoveOpenClawPlugin() error = %v", err)
	}
	if _, err := os.Stat(cfg.OpenClawPluginDir()); !os.IsNotExist(err) {
		t.Fatalf("Stat(plugin dir) after removal error = %v, want os.IsNotExist", err)
	}
}

func TestRemoveOpenClawPlugin_AbsentDir_NoOp(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir(), OpenClawHome: t.TempDir()}
	if err := RemoveOpenClawPlugin(cfg); err != nil {
		t.Fatalf("RemoveOpenClawPlugin() error = %v, want nil when the plugin was never installed", err)
	}
}

func TestRemoveOpenClawPlugin_OpenClawHomeEmpty_NoOp(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir()}
	if err := RemoveOpenClawPlugin(cfg); err != nil {
		t.Fatalf("RemoveOpenClawPlugin() error = %v, want nil no-op when OpenClawHome is empty", err)
	}
}
