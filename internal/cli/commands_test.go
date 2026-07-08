package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func execRoot(t *testing.T, claudeHome string, args ...string) (string, error) {
	t.Helper()
	t.Setenv("CLICK_CLAUDE_HOME", claudeHome)

	root := NewRootCommand()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs(args)

	err := root.Execute()
	return buf.String(), err
}

func TestInstallCommand_Succeeds(t *testing.T) {
	home := t.TempDir()

	out, err := execRoot(t, home, "install")
	if err != nil {
		t.Fatalf("install command error = %v, output:\n%s", err, out)
	}
	if !strings.Contains(out, "click-sdd") {
		t.Errorf("install output = %q, want it to mention the click-sdd plugin step", out)
	}
	if !strings.Contains(out, "click-memory") {
		t.Errorf("install output = %q, want it to mention the click-memory plugin step", out)
	}
	if !strings.Contains(out, "click-review") {
		t.Errorf("install output = %q, want it to mention the click-review plugin step", out)
	}
	if !strings.Contains(out, "CLAUDE.md") {
		t.Errorf("install output = %q, want it to mention the CLAUDE.md step", out)
	}
}

func TestDoctorCommand_AfterInstall_Succeeds(t *testing.T) {
	home := t.TempDir()

	if _, err := execRoot(t, home, "install"); err != nil {
		t.Fatalf("install command error = %v", err)
	}

	out, err := execRoot(t, home, "doctor")
	if err != nil {
		t.Fatalf("doctor command error = %v after a successful install, output:\n%s", err, out)
	}
}

func TestDoctorCommand_BeforeInstall_ReturnsError(t *testing.T) {
	home := t.TempDir()

	_, err := execRoot(t, home, "doctor")
	if err == nil {
		t.Fatal("doctor command on a never-installed ClaudeHome returned nil error, want a non-nil error (non-zero exit)")
	}
}

func TestUninstallCommand_ReversesInstall(t *testing.T) {
	home := t.TempDir()

	if _, err := execRoot(t, home, "install"); err != nil {
		t.Fatalf("install command error = %v", err)
	}
	if _, err := execRoot(t, home, "doctor"); err != nil {
		t.Fatalf("doctor command error = %v right after install", err)
	}

	if _, err := execRoot(t, home, "uninstall"); err != nil {
		t.Fatalf("uninstall command error = %v", err)
	}

	_, err := execRoot(t, home, "doctor")
	if err == nil {
		t.Fatal("doctor command after uninstall returned nil error, want a non-nil error (unhealthy)")
	}
}

func TestUninstallCommand_NoopWhenAlreadyUninstalled(t *testing.T) {
	home := t.TempDir()

	if _, err := execRoot(t, home, "uninstall"); err != nil {
		t.Fatalf("uninstall command on a never-installed ClaudeHome error = %v, want nil", err)
	}
}

func TestUpdateCommand_ResyncsPluginsAndWritesEngramPin(t *testing.T) {
	home := t.TempDir()
	binaryPath := filepath.Join(t.TempDir(), "engram.exe")
	if err := os.WriteFile(binaryPath, []byte("binary"), 0o644); err != nil {
		t.Fatalf("WriteFile(binary) error = %v", err)
	}
	t.Setenv("CLICK_ENGRAM_BINARY_PATH", binaryPath)

	if _, err := execRoot(t, home, "install"); err != nil {
		t.Fatalf("install command error = %v", err)
	}

	brokenPath := filepath.Join(home, "plugins", "click-sdd", "agents", "click-orchestrator.md")
	if err := os.WriteFile(brokenPath, []byte("broken"), 0o644); err != nil {
		t.Fatalf("WriteFile(brokenPath) error = %v", err)
	}

	out, err := execRoot(t, home, "update")
	if err != nil {
		t.Fatalf("update command error = %v", err)
	}
	if !strings.Contains(out, "click-review") {
		t.Errorf("update output = %q, want it to mention the click-review re-sync step", out)
	}
	if !strings.Contains(out, "Engram") {
		t.Errorf("update output = %q, want it to mention the Engram pin step", out)
	}

	updated, err := os.ReadFile(brokenPath)
	if err != nil {
		t.Fatalf("ReadFile(brokenPath) error = %v", err)
	}
	if string(updated) == "broken" {
		t.Fatal("update did not restore embedded plugin content")
	}

	if _, err := os.Stat(filepath.Join(home, "mcp", "engram.json")); err != nil {
		t.Fatalf("update did not write engram MCP config: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, "click-ai-devkit", "engram.json")); err != nil {
		t.Fatalf("update did not write engram pinned state: %v", err)
	}
}

func TestRootCommand_VersionDefaultsToDev(t *testing.T) {
	home := t.TempDir()
	out, err := execRoot(t, home, "--version")
	if err != nil {
		t.Fatalf("version command error = %v", err)
	}
	if !strings.Contains(out, "dev") {
		t.Fatalf("version output = %q, want it to contain dev", out)
	}
}

func TestRendererFor_NoColorFlagForcesPlain(t *testing.T) {
	root := NewRootCommand()
	root.SetArgs([]string{"install", "--no-color"})
	root.PersistentFlags().Parse([]string{"--no-color"})

	var buf bytes.Buffer
	r := rendererFor(root, &buf)
	if r.Color {
		t.Fatal("rendererFor() with --no-color parsed produced a color-enabled renderer")
	}
}
