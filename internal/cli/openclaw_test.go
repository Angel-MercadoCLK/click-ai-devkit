package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/installer"
)

// TestConfigureOpenClawModelCommand_NoArgs_PrintsGuidanceAndReturnsNil is the FIX 1 contract:
// running `click configure-openclaw-model` with NO args (exactly what the standing menu dispatches)
// must NEVER return an error — an error here would propagate through runMenuLoop and terminate the
// whole interactive menu session. It must instead print the Spanish guidance line and exit cleanly.
func TestConfigureOpenClawModelCommand_NoArgs_PrintsGuidanceAndReturnsNil(t *testing.T) {
	home := t.TempDir()
	out, err := execRoot(t, home, "configure-openclaw-model")
	if err != nil {
		t.Fatalf("`click configure-openclaw-model` with no args error = %v, want nil (must never crash the menu)", err)
	}
	if !strings.Contains(out, "Indique el modelo con: click configure-openclaw-model <provider/model>") {
		t.Fatalf("no-args output = %q, want the Spanish guidance line", out)
	}
}

// seedResolvableOpenClaw extends seedResolvableGit's fake BinaryLookup (commands_test.go) so
// "openclaw" also resolves on PATH, letting install/update-driven CLI tests exercise the
// OpenClaw-detected path deterministically, regardless of whether the real test machine has
// openclaw on PATH.
func seedResolvableOpenClaw(t *testing.T) {
	t.Helper()
	restore := installer.SetBinaryLookupFactoryForTests(func() installer.BinaryLookup {
		return cliFakeBinaryLookup{resolved: map[string]string{
			"git": "/usr/bin/git", "claude": "/usr/bin/claude", "openclaw": "/usr/bin/openclaw",
		}}
	})
	t.Cleanup(restore)
}

// execRootWithOpenClaw is execRoot's exact shape, but with openclaw ALSO resolvable on PATH
// (seedResolvableOpenClaw) and CLICK_OPENCLAW_HOME pointed at openClawHome — used exclusively by
// the OpenClaw-detected CLI tests below, where the whole point is to simulate openclaw being
// present.
func execRootWithOpenClaw(t *testing.T, claudeHome, openClawHome string, args ...string) (string, error) {
	t.Helper()
	t.Setenv("CLICK_CLAUDE_HOME", claudeHome)
	t.Setenv("CLICK_OPENCLAW_HOME", openClawHome)
	seedResolvableOpenClaw(t)

	root := NewRootCommand()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetIn(&bytes.Buffer{})
	root.SetArgs(args)

	err := root.Execute()
	return buf.String(), err
}

// TestInstallCommand_OpenClawDetected_WritesAgentsAndSoulAndMCPConfig is task 2.14/2.17's
// end-to-end RED test: with openclaw resolvable on PATH, `click install` must write AGENTS.md,
// SOUL.md, and register the engram MCP entry in openclaw.json — under the SAME single confirm as
// Claude Code's own writes (execRootWithOpenClaw's non-TTY buffer takes the non-interactive path,
// exactly like every other CLI test in this package, so this proves the writes actually run, not
// just that the plan lists them).
func TestInstallCommand_OpenClawDetected_WritesAgentsAndSoulAndMCPConfig(t *testing.T) {
	claudeHome := t.TempDir()
	openClawHome := t.TempDir()
	runner := newTestCommandRunner(claudeHome)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()

	out, err := execRootWithOpenClaw(t, claudeHome, openClawHome, "install")
	if err != nil {
		t.Fatalf("install command error = %v, output:\n%s", err, out)
	}
	if !strings.Contains(out, "OpenClaw") {
		t.Errorf("install output = %q, want it to mention OpenClaw when detected", out)
	}

	cfg := installer.Config{ClaudeHome: claudeHome, OpenClawHome: openClawHome}
	agentsRaw, err := os.ReadFile(cfg.OpenClawAgentsMDPath())
	if err != nil {
		t.Fatalf("ReadFile(AGENTS.md) error = %v, want it written when OpenClaw is detected", err)
	}
	if !strings.Contains(string(agentsRaw), "click-ai-devkit (managed)") {
		t.Fatalf("AGENTS.md content = %q, want it to contain the managed block markers", agentsRaw)
	}
	soulRaw, err := os.ReadFile(cfg.OpenClawSoulMDPath())
	if err != nil {
		t.Fatalf("ReadFile(SOUL.md) error = %v, want it written when OpenClaw is detected", err)
	}
	if !strings.Contains(string(soulRaw), "click-ai-devkit (managed)") {
		t.Fatalf("SOUL.md content = %q, want it to contain the managed block markers", soulRaw)
	}
	mcpRaw, err := os.ReadFile(cfg.OpenClawMCPConfigPath())
	if err != nil {
		t.Fatalf("ReadFile(openclaw.json) error = %v, want it written when OpenClaw is detected", err)
	}
	if !strings.Contains(string(mcpRaw), `"engram"`) {
		t.Fatalf("openclaw.json content = %q, want it to contain the engram MCP entry", mcpRaw)
	}
	profileRaw, err := os.ReadFile(cfg.OpenClawModelProfilePath())
	if err != nil {
		t.Fatalf("ReadFile(model-profile.json) error = %v, want portable recommendation written", err)
	}
	if !strings.Contains(string(profileRaw), `"schema_version": 2`) || !strings.Contains(string(profileRaw), `"profile": "balanced"`) {
		t.Fatalf("model-profile.json content = %q, want the shared versioned balanced profile", profileRaw)
	}
}

// TestInstallCommand_OpenClawAbsent_NoOpenClawMentionOrWrites is task 2.15's RED test: OpenClaw not
// detected -> zero new prompts, zero OpenClaw writes, output identical in shape to a pre-OpenClaw
// install (regression guard for the "zero behavior change" requirement).
func TestInstallCommand_OpenClawAbsent_NoOpenClawMentionOrWrites(t *testing.T) {
	claudeHome := t.TempDir()
	runner := newTestCommandRunner(claudeHome)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()

	out, err := execRoot(t, claudeHome, "install")
	if err != nil {
		t.Fatalf("install command error = %v, output:\n%s", err, out)
	}
	if strings.Contains(out, "OpenClaw") {
		t.Fatalf("install output = %q, want no OpenClaw mention when openclaw is not detected", out)
	}
}

// TestInstallCommand_SkipOpenClawFlag_ForcesClaudeOnlyEvenWhenDetected is task 2.16's RED test:
// --skip-openclaw must force a Claude-only install even when openclaw IS resolvable on PATH.
func TestInstallCommand_SkipOpenClawFlag_ForcesClaudeOnlyEvenWhenDetected(t *testing.T) {
	claudeHome := t.TempDir()
	openClawHome := t.TempDir()
	runner := newTestCommandRunner(claudeHome)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()

	out, err := execRootWithOpenClaw(t, claudeHome, openClawHome, "install", "--skip-openclaw")
	if err != nil {
		t.Fatalf("install command error = %v, output:\n%s", err, out)
	}
	if strings.Contains(out, "OpenClaw") {
		t.Fatalf("install output = %q, want no OpenClaw mention when --skip-openclaw is set even though openclaw is detected", out)
	}

	cfg := installer.Config{ClaudeHome: claudeHome, OpenClawHome: openClawHome}
	if _, err := os.Stat(cfg.OpenClawAgentsMDPath()); !os.IsNotExist(err) {
		t.Fatalf("Stat(AGENTS.md) error = %v, want no OpenClaw files written when --skip-openclaw is set", err)
	}
	if _, err := os.Stat(cfg.OpenClawModelProfilePath()); !os.IsNotExist(err) {
		t.Fatalf("Stat(model-profile.json) error = %v, want no OpenClaw artifact when --skip-openclaw is set", err)
	}
}

// TestInstallCommand_OpenClawDetected_InstallsMemoryGuardPlugin is PR-C's (design #1666's OCG-1..6)
// end-to-end RED test: with openclaw resolvable on PATH, `click install` must also write the
// click-memory-guard plugin (hooks.js + plugin.json) under OpenClawPluginDir(), with the
// {{CLICK_BIN}} placeholder templated away.
func TestInstallCommand_OpenClawDetected_InstallsMemoryGuardPlugin(t *testing.T) {
	claudeHome := t.TempDir()
	openClawHome := t.TempDir()
	runner := newTestCommandRunner(claudeHome)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()

	out, err := execRootWithOpenClaw(t, claudeHome, openClawHome, "install")
	if err != nil {
		t.Fatalf("install command error = %v, output:\n%s", err, out)
	}
	if !strings.Contains(out, "memory-guard") {
		t.Errorf("install output = %q, want it to mention installing the memory-guard plugin", out)
	}

	cfg := installer.Config{ClaudeHome: claudeHome, OpenClawHome: openClawHome}
	hooksRaw, err := os.ReadFile(filepath.Join(cfg.OpenClawPluginDir(), "plugins", "hooks.js"))
	if err != nil {
		t.Fatalf("ReadFile(hooks.js) error = %v, want the plugin installed when OpenClaw is detected", err)
	}
	if strings.Contains(string(hooksRaw), "{{CLICK_BIN}}") {
		t.Fatalf("hooks.js content = %q, want the {{CLICK_BIN}} placeholder templated away", hooksRaw)
	}
	if _, err := os.Stat(filepath.Join(cfg.OpenClawPluginDir(), "plugin.json")); err != nil {
		t.Fatalf("Stat(plugin.json) error = %v, want it written alongside hooks.js", err)
	}
}

// TestInstallCommand_SkipOpenClawFlag_NoMemoryGuardPluginWritten guards the plugin write against
// the same --skip-openclaw escape hatch every other OpenClaw write already respects.
func TestInstallCommand_SkipOpenClawFlag_NoMemoryGuardPluginWritten(t *testing.T) {
	claudeHome := t.TempDir()
	openClawHome := t.TempDir()
	runner := newTestCommandRunner(claudeHome)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()

	if _, err := execRootWithOpenClaw(t, claudeHome, openClawHome, "install", "--skip-openclaw"); err != nil {
		t.Fatalf("install command error = %v", err)
	}

	cfg := installer.Config{ClaudeHome: claudeHome, OpenClawHome: openClawHome}
	if _, err := os.Stat(cfg.OpenClawPluginDir()); !os.IsNotExist(err) {
		t.Fatalf("Stat(plugin dir) error = %v, want no plugin dir written when --skip-openclaw is set", err)
	}
}

// TestInstallCommand_OpenClawDetected_SyncsClickSkills is PR4's RED test: with openclaw resolvable
// on PATH, `click install` must synchronize the click-owned skill manifests under OpenClawSkillsDir().
func TestInstallCommand_OpenClawDetected_SyncsClickSkills(t *testing.T) {
	claudeHome := t.TempDir()
	openClawHome := t.TempDir()
	runner := newTestCommandRunner(claudeHome)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()

	out, err := execRootWithOpenClaw(t, claudeHome, openClawHome, "install")
	if err != nil {
		t.Fatalf("install command error = %v, output:\n%s", err, out)
	}
	if !strings.Contains(out, "Sincronizando skills de Click en OpenClaw") {
		t.Errorf("install output = %q, want it to mention the OpenClaw skill sync step", out)
	}

	cfg := installer.Config{ClaudeHome: claudeHome, OpenClawHome: openClawHome}
	for _, name := range []string{"clickhola", "clickdev"} {
		path := filepath.Join(cfg.OpenClawSkillsDir(), name, "SKILL.md")
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile(%s) error = %v, want it written when OpenClaw is detected", path, err)
		}
		if !strings.Contains(string(data), "name: "+name) {
			t.Fatalf("%s content = %q, want frontmatter name %q", path, data, name)
		}
	}
}

// TestInstallCommand_OpenClawAbsent_NoSkillWrites is PR4's zero-behavior-change guard: when OpenClaw
// is not detected, `click install` must not write any skill files.
func TestInstallCommand_OpenClawAbsent_NoSkillWrites(t *testing.T) {
	claudeHome := t.TempDir()
	runner := newTestCommandRunner(claudeHome)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()

	out, err := execRoot(t, claudeHome, "install")
	if err != nil {
		t.Fatalf("install command error = %v, output:\n%s", err, out)
	}
	if strings.Contains(out, "Sincronizando skills de Click en OpenClaw") {
		t.Fatalf("install output = %q, want no OpenClaw skill mention when openclaw is not detected", out)
	}
}

// TestUpdateCommand_OpenClawDetected_ResyncsAgentsAndSoulAndMCPConfig mirrors the install-side
// detection test for `click update`.
func TestUpdateCommand_OpenClawDetected_ResyncsAgentsAndSoulAndMCPConfig(t *testing.T) {
	claudeHome := t.TempDir()
	openClawHome := t.TempDir()
	runner := newTestCommandRunner(claudeHome)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()

	if _, err := execRootWithOpenClaw(t, claudeHome, openClawHome, "install"); err != nil {
		t.Fatalf("install command error = %v", err)
	}

	out, err := execRootWithOpenClaw(t, claudeHome, openClawHome, "update")
	if err != nil {
		t.Fatalf("update command error = %v, output:\n%s", err, out)
	}
	if !strings.Contains(out, "OpenClaw") {
		t.Errorf("update output = %q, want it to mention OpenClaw when detected", out)
	}

	cfg := installer.Config{ClaudeHome: claudeHome, OpenClawHome: openClawHome}
	if _, err := os.Stat(cfg.OpenClawMCPConfigPath()); err != nil {
		t.Fatalf("Stat(openclaw.json) error = %v, want it present after update re-syncs OpenClaw", err)
	}
}

// TestUpdateCommand_OpenClawDetected_ResyncsClickSkills is PR4's RED test: `click update` must re-
// synchronize the click-owned OpenClaw skill manifests, restoring them if they drifted.
func TestUpdateCommand_OpenClawDetected_ResyncsClickSkills(t *testing.T) {
	claudeHome := t.TempDir()
	openClawHome := t.TempDir()
	runner := newTestCommandRunner(claudeHome)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()

	if _, err := execRootWithOpenClaw(t, claudeHome, openClawHome, "install"); err != nil {
		t.Fatalf("install command error = %v", err)
	}

	cfg := installer.Config{ClaudeHome: claudeHome, OpenClawHome: openClawHome}
	tampered := filepath.Join(cfg.OpenClawSkillsDir(), "clickdev", "SKILL.md")
	if err := os.WriteFile(tampered, []byte("tampered content"), 0o644); err != nil {
		t.Fatalf("WriteFile(tamper) error = %v", err)
	}

	out, err := execRootWithOpenClaw(t, claudeHome, openClawHome, "update")
	if err != nil {
		t.Fatalf("update command error = %v, output:\n%s", err, out)
	}
	if !strings.Contains(out, "Sincronizando skills de Click en OpenClaw") {
		t.Errorf("update output = %q, want it to mention the OpenClaw skill sync step", out)
	}

	got, err := os.ReadFile(tampered)
	if err != nil {
		t.Fatalf("ReadFile(clickdev SKILL.md) error = %v", err)
	}
	if strings.Contains(string(got), "tampered content") {
		t.Fatalf("clickdev SKILL.md still contains tampered content = %q, want resynced embedded bytes", got)
	}
	if !strings.Contains(string(got), "name: clickdev") {
		t.Fatalf("clickdev SKILL.md = %q, want resynced embedded bytes with frontmatter name", got)
	}
}
