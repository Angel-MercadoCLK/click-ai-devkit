package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/installer"
)

func TestConfigureOpenClawModelCommand_RetiredWithoutChangingExistingConfiguration(t *testing.T) {
	for _, args := range [][]string{{"configure-openclaw-model"}, {"configure-openclaw-model", "openai/example-model"}} {
		t.Run(strings.Join(args, "-"), func(t *testing.T) {
			claudeHome, openClawHome := t.TempDir(), t.TempDir()
			t.Setenv("CLICK_OPENCLAW_HOME", openClawHome)
			cfg := installer.Config{ClaudeHome: claudeHome, OpenClawHome: openClawHome}
			if err := os.MkdirAll(filepath.Dir(cfg.OpenClawMCPConfigPath()), 0o755); err != nil {
				t.Fatal(err)
			}
			before := []byte(`{"mcp":{"servers":{"foreign":{"command":"keep-me"}}},"agents":{"defaults":{"model":{"primary":"vendor/existing","fallbacks":["vendor/fallback"]}}},"custom":"sentinel"}`)
			if err := os.WriteFile(cfg.OpenClawMCPConfigPath(), before, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Dir(cfg.OpenClawModelProfilePath()), 0o755); err != nil {
				t.Fatal(err)
			}
			profileBefore := []byte(`{"schema_version":2,"profile":"legacy","models":{"default":"preserve-me"}}`)
			if err := os.WriteFile(cfg.OpenClawModelProfilePath(), profileBefore, 0o600); err != nil {
				t.Fatal(err)
			}
			runner := newTestCommandRunner(claudeHome)
			restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
			t.Cleanup(restoreRunner)
			lookupCalls := 0
			restoreLookup := installer.SetBinaryLookupFactoryForTests(func() installer.BinaryLookup {
				lookupCalls++
				return cliFakeBinaryLookup{resolved: map[string]string{}}
			})
			t.Cleanup(restoreLookup)

			cmd := newConfigureOpenClawModelCommand()
			var output bytes.Buffer
			cmd.SetOut(&output)
			cmd.SetErr(&output)
			cmd.SetIn(&bytes.Buffer{})
			cmd.SetArgs(args[1:])
			err := cmd.Execute()
			out := output.String()
			if err == nil {
				t.Fatalf("command error = nil, want a deprecation error; output=%q", out)
			}
			if !strings.Contains(err.Error(), "retirado") {
				t.Fatalf("command error = %q, want a Spanish retirement notice", err)
			}
			if len(runner.commands) != 0 {
				t.Fatalf("runner commands = %v, want none", runner.commands)
			}
			if lookupCalls != 0 {
				t.Fatalf("runtime lookup factory called %d times, want 0", lookupCalls)
			}
			after, readErr := os.ReadFile(cfg.OpenClawMCPConfigPath())
			if readErr != nil {
				t.Fatalf("ReadFile(existing OpenClaw config) error = %v", readErr)
			}
			if string(after) != string(before) {
				t.Fatalf("existing OpenClaw config changed: before=%q after=%q", before, after)
			}
			profileAfter, readErr := os.ReadFile(cfg.OpenClawModelProfilePath())
			if readErr != nil {
				t.Fatalf("ReadFile(existing OpenClaw model profile) error = %v", readErr)
			}
			if string(profileAfter) != string(profileBefore) {
				t.Fatalf("existing OpenClaw model profile changed: before=%q after=%q", profileBefore, profileAfter)
			}
		})
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
	configureCLIExecutionHomes(t, claudeHome, os.Getenv("CLICK_TEST_CODEX_FIXTURE"), openClawHome, args)
	t.Setenv("CLICK_STATE_HOME", t.TempDir())
	seedResolvableOpenClaw(t)

	root := NewRootCommand()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetIn(&bytes.Buffer{})
	root.SetArgs(args)

	err := root.Execute()
	if os.Getenv("CLICK_TEST_CLAUDE_FIXTURE") != "" {
		if syncErr := syncStandardTargetFixtures(); syncErr != nil {
			t.Errorf("sync target fixtures after command: %v", syncErr)
		}
	}
	return buf.String(), err
}

// TestInstallCommand_OpenClawDetected_WritesAgentsAndSoul is task 2.14/2.17's end-to-end test,
// adapted for the ownership transfer: with openclaw resolvable on PATH, `click install` must
// write AGENTS.md and SOUL.md, under the SAME single confirm as Claude Code's own writes
// (execRootWithOpenClaw's non-TTY buffer takes the non-interactive path, exactly like every other
// CLI test in this package, so this proves the writes actually run, not just that the plan lists
// them). gentle-ai owns the upstream MCP config in openclaw.json; Click writes only its guidance,
// model profile, and memory-guard assets around that registration.
func TestInstallCommand_OpenClawDetected_WritesAgentsAndSoul(t *testing.T) {
	claudeHome := t.TempDir()
	openClawHome := t.TempDir()
	cfg := installer.Config{ClaudeHome: claudeHome, OpenClawHome: openClawHome}
	if err := os.MkdirAll(filepath.Dir(cfg.OpenClawModelProfilePath()), 0o755); err != nil {
		t.Fatal(err)
	}
	legacyProfile := []byte("existing OpenClaw model metadata remains user-owned")
	if err := os.WriteFile(cfg.OpenClawModelProfilePath(), legacyProfile, 0o600); err != nil {
		t.Fatal(err)
	}
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
	upstreamConfig, err := os.ReadFile(cfg.OpenClawMCPConfigPath())
	if err != nil {
		t.Fatalf("ReadFile(openclaw.json) error = %v, want gentle-ai's selected MCP registrations", err)
	}
	var openClawConfig map[string]any
	if err := json.Unmarshal(upstreamConfig, &openClawConfig); err != nil {
		t.Fatalf("parse gentle-ai OpenClaw config: %v", err)
	}
	mcp := openClawConfig["mcp"].(map[string]any)
	servers := mcp["servers"].(map[string]any)
	for _, name := range []string{"engram", "context7"} {
		if _, ok := servers[name]; !ok {
			t.Fatalf("OpenClaw mcp.servers = %v, want upstream registration %q", servers, name)
		}
	}
	profileRaw, err := os.ReadFile(cfg.OpenClawModelProfilePath())
	if err != nil || !bytes.Equal(profileRaw, legacyProfile) {
		t.Fatalf("model-profile.json = %q (err %v), want legacy metadata preserved exactly", profileRaw, err)
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

// TestInstallCommand_OpenClawDetected_SyncsRetainedClickSkill verifies only the retained business
// skill is synchronized; retired workflow paths are not recreated during install.
func TestInstallCommand_OpenClawDetected_SyncsRetainedClickSkill(t *testing.T) {
	claudeHome := t.TempDir()
	openClawHome := t.TempDir()
	legacyPath := filepath.Join(openClawHome, "skills", "clickdev", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacyPath, []byte("custom legacy content\n"), 0o644); err != nil {
		t.Fatal(err)
	}
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
	if !strings.Contains(out, "Se conservó la skill heredada de OpenClaw clickdev/SKILL.md") {
		t.Errorf("install output = %q, want Spanish warning for modified legacy skill", out)
	}

	cfg := installer.Config{ClaudeHome: claudeHome, OpenClawHome: openClawHome}
	path := filepath.Join(cfg.OpenClawSkillsDir(), "clickhola", "SKILL.md")
	data, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(data), "name: clickhola") {
		t.Fatalf("clickhola content = %q, %v; want retained skill written", data, err)
	}
	got, err := os.ReadFile(legacyPath)
	if err != nil || string(got) != "custom legacy content\n" {
		t.Fatalf("clickdev legacy skill = %q, %v; want modified bytes preserved", got, err)
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

// TestUpdateCommand_OpenClawDetected_ResyncsAgentsAndSoul mirrors the install-side detection test
// for `click update` and keeps the upstream-owned MCP configuration present.
func TestUpdateCommand_OpenClawDetected_ResyncsAgentsAndSoul(t *testing.T) {
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
		t.Fatalf("Stat(openclaw.json) error = %v, want upstream-owned config retained", err)
	}
}

// TestInstallCommand_OpenClawDetected_PreservesUnprovenLegacyMCPConfig proves Click does not
// delete an unproven same-name registration while transferring MCP ownership to gentle-ai.
func TestInstallCommand_OpenClawDetected_PreservesUnprovenLegacyMCPConfig(t *testing.T) {
	claudeHome := t.TempDir()
	openClawHome := t.TempDir()
	runner := newTestCommandRunner(claudeHome)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()

	cfg := installer.Config{ClaudeHome: claudeHome, OpenClawHome: openClawHome}
	if err := os.MkdirAll(openClawHome, 0o755); err != nil {
		t.Fatalf("MkdirAll(openClawHome) error = %v", err)
	}
	corrupted := `{"someUserKey": {"kept": true}, "mcpServers": {"engram": {"command": "engram", "args": ["mcp", "--tools=agent"], "transport": "stdio"}}}`
	if err := os.WriteFile(cfg.OpenClawMCPConfigPath(), []byte(corrupted), 0o644); err != nil {
		t.Fatalf("WriteFile(openclaw.json) error = %v", err)
	}

	if _, err := execRootWithOpenClaw(t, claudeHome, openClawHome, "install"); err == nil {
		t.Fatal("install accepted an unproven same-name OpenClaw Engram registration")
	}
	if len(runner.commands) != 0 {
		t.Fatalf("Click runner calls = %v, want preflight to stop before Click writes", runner.commands)
	}

	got, err := os.ReadFile(cfg.OpenClawMCPConfigPath())
	if err != nil {
		t.Fatalf("ReadFile(openclaw.json) error = %v", err)
	}
	if string(got) != corrupted {
		t.Fatalf("openclaw.json = %s, want unproven legacy config preserved exactly", got)
	}
	if !strings.Contains(string(got), "someUserKey") {
		t.Fatalf("openclaw.json = %s, want unrelated top-level key preserved", got)
	}
}

// TestUpdateCommand_OpenClawDetected_PreservesModifiedRetiredSkill verifies update does not
// overwrite unknown content at a retired legacy path and reports the preservation warning.
func TestUpdateCommand_OpenClawDetected_PreservesModifiedRetiredSkill(t *testing.T) {
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
	if err := os.MkdirAll(filepath.Dir(tampered), 0o755); err != nil {
		t.Fatal(err)
	}
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
	if !strings.Contains(out, "Se conservó la skill heredada de OpenClaw clickdev/SKILL.md") {
		t.Errorf("update output = %q, want Spanish preservation warning", out)
	}

	got, err := os.ReadFile(tampered)
	if err != nil {
		t.Fatalf("ReadFile(clickdev SKILL.md) error = %v", err)
	}
	if string(got) != "tampered content" {
		t.Fatalf("clickdev SKILL.md = %q, want modified legacy content preserved exactly", got)
	}
}
