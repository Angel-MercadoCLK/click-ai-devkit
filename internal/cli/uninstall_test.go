package cli

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/installer"
)

func execRootWithLookupAndState(t *testing.T, claudeHome, stateHome string, lookup installer.BinaryLookup, args ...string) (string, error) {
	t.Helper()
	t.Setenv("CLICK_CLAUDE_HOME", claudeHome)
	t.Setenv("CLICK_STATE_HOME", stateHome)
	restore := installer.SetBinaryLookupFactoryForTests(func() installer.BinaryLookup { return lookup })
	defer restore()

	root := NewRootCommand()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetIn(&bytes.Buffer{})
	root.SetArgs(args)

	err := root.Execute()
	return buf.String(), err
}

// failingClaudeRunner simulates a machine where the `claude` CLI genuinely cannot run: every
// invocation with name=="claude" fails, mirroring the raw, unwrapped exec error a real machine with
// claude removed from PATH would produce once a claude-dependent step actually tries to shell out.
// Used to prove Finding 2(b)'s resilience contract (every OTHER step still runs to completion) and
// Finding 2(a)'s friendly-message wrapping (the raw failure gets ClaudeMissingMessage attached) end
// to end, without depending on a real missing claude binary.
type failingClaudeRunner struct{ commands []string }

func (r *failingClaudeRunner) Run(name string, args ...string) error {
	r.commands = append(r.commands, name+" "+strings.Join(args, " "))
	if name == "claude" {
		return fmt.Errorf("exec: %q: executable file not found in $PATH", name)
	}
	return nil
}

func (r *failingClaudeRunner) Output(name string, args ...string) ([]byte, error) {
	r.commands = append(r.commands, name+" "+strings.Join(args, " "))
	return []byte{}, nil
}

// TestUninstallCommand_SurfacesEngramPathWarning_WarningOnlyNoError is the pre-existing-shaped half
// of the T4-1 follow-up regression coverage: a warning-only RemoveEngramPlugin result (err == nil)
// must surface via surfacePathWarning and `click uninstall` must still succeed. This mirrors the
// success path runUninstall already exercised before the fix — kept here as an explicit CLI-level
// case alongside the double-failure test below, so both branches of the same `if err != nil` split
// are covered from the same seam.
func TestUninstallCommand_SurfacesEngramPathWarning_WarningOnlyNoError(t *testing.T) {
	home := t.TempDir()
	runner := newTestCommandRunner(home)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()

	wantWarning := "no se pudo quitar C:\\fake\\gobin del PATH persistente: acceso denegado"
	restoreRemove := SetRemoveEngramPluginFuncForTests(func(cfg installer.Config) (string, error) {
		return wantWarning, nil
	})
	defer restoreRemove()

	out, err := execRoot(t, home, "uninstall")
	if err != nil {
		t.Fatalf("uninstall command error = %v, want nil when RemoveEngramPlugin only returns a pathWarning", err)
	}
	if !strings.Contains(out, wantWarning) {
		t.Fatalf("uninstall output = %q, want it to contain the PATH warning %q", out, wantWarning)
	}
}

// TestUninstallCommand_SurfacesEngramPathWarning_EvenOnFatalError is the actual regression this
// fix closes: RemoveEngramPlugin returning BOTH a non-empty pathWarning AND a fatal error in the
// same call (e.g. one PATH entry failed to be removed AND a later, unrelated step such as
// uninstalling the plugin itself also failed) used to silently drop the pathWarning — runUninstall
// captured it into engramPathWarning but returned early on `err != nil`, before ever reaching
// surfacePathWarning. The fatal error itself still surfaced (via cobra), but the PATH-specific
// detail vanished. The fix surfaces engramPathWarning on BOTH the error path and the success path.
func TestUninstallCommand_SurfacesEngramPathWarning_EvenOnFatalError(t *testing.T) {
	home := t.TempDir()
	runner := newTestCommandRunner(home)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()

	wantWarning := "no se pudo quitar C:\\fake\\gobin del PATH persistente: acceso denegado"
	wantErr := errors.New("no se pudo desinstalar el plugin engram@engram")
	restoreRemove := SetRemoveEngramPluginFuncForTests(func(cfg installer.Config) (string, error) {
		return wantWarning, wantErr
	})
	defer restoreRemove()

	out, err := execRoot(t, home, "uninstall")
	if err == nil {
		t.Fatalf("uninstall command error = nil, want a non-nil error when RemoveEngramPlugin also fails, output:\n%s", out)
	}
	if !strings.Contains(out, wantWarning) {
		t.Fatalf("uninstall output = %q, want it to contain the PATH warning %q even though RemoveEngramPlugin also returned a fatal error", out, wantWarning)
	}
}

// TestUninstallCommand_ContinuesEveryStepAfterAnEarlierOneFails is Finding 2(b)'s core regression
// test: `click uninstall` must be RESILIENT, not fail-fast. Before this fix, runUninstall returned
// immediately on step 1's error (`return err`) — since step 1 (RemoveMarketplacePlugins) shells out
// to `claude`, a realistic uninstall scenario (claude already removed as part of tearing the setup
// down) meant CLAUDE.md and the memory-guard hook NEVER got cleaned up, contradicting installer's own
// doc comment that Uninstall "reverses everything Install can have written". This proves every LATER
// step still runs to completion — and its own state change lands on disk — even though the FIRST
// step was forced to fail.
func TestUninstallCommand_ContinuesEveryStepAfterAnEarlierOneFails(t *testing.T) {
	home := t.TempDir()
	cfg := installer.Config{ClaudeHome: home}

	// Stand in for a real prior `click install`: a managed CLAUDE.md block and a registered
	// memory-guard hook, both of which must still get cleaned up below despite step 1 failing.
	if err := installer.WriteManagedBlock(cfg.ClaudeMDPath(), installer.DefaultManagedContent); err != nil {
		t.Fatalf("WriteManagedBlock() error = %v", err)
	}
	if err := installer.RegisterMemoryGuardHook(cfg); err != nil {
		t.Fatalf("RegisterMemoryGuardHook() error = %v", err)
	}

	runner := &failingClaudeRunner{}
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()

	out, err := execRoot(t, home, "uninstall")
	if err == nil {
		t.Fatalf("uninstall command error = nil, want non-nil — RemoveMarketplacePlugins was forced to fail, output:\n%s", out)
	}

	md, readErr := os.ReadFile(cfg.ClaudeMDPath())
	if readErr != nil {
		t.Fatalf("ReadFile(ClaudeMDPath) error = %v", readErr)
	}
	if !strings.Contains(string(md), "click-ai-devkit (managed)") {
		t.Fatalf("CLAUDE.md = %q, want rollback to restore the managed block after the failed first step", md)
	}

	hasHook, hookErr := installer.HasMemoryGuardHook(cfg)
	if hookErr != nil {
		t.Fatalf("HasMemoryGuardHook() error = %v", hookErr)
	}
	if !hasHook {
		t.Fatal("memory-guard hook missing after uninstall failure, want rollback to restore it")
	}

	// The failure must be reported, not silently swallowed (Finding 2(b)): the overall summary must
	// name the step that failed.
	if !strings.Contains(out, "Plugins de Claude Code") {
		t.Fatalf("uninstall output = %q, want the final summary to name the failed \"Plugins de Claude Code\" step", out)
	}
}

// TestUninstallCommand_ClaudeMissing_StillRunsEveryStepAndReportsFriendlyMessage is Finding 2(a)'s
// regression test: unlike install.go/update.go (which abort BEFORE issuing any command when claude
// is missing), `click uninstall` must still run every cleanup step when claude is missing — and the
// claude-dependent steps' failures must report the SAME actionable ClaudeMissingMessage text
// install/update already show, not a bare unwrapped exec error.
func TestUninstallCommand_ClaudeMissing_StillRunsEveryStepAndReportsFriendlyMessage(t *testing.T) {
	home := t.TempDir()
	cfg := installer.Config{ClaudeHome: home}
	if err := installer.WriteManagedBlock(cfg.ClaudeMDPath(), installer.DefaultManagedContent); err != nil {
		t.Fatalf("WriteManagedBlock() error = %v", err)
	}

	stateHome := t.TempDir()
	t.Setenv("CLICK_STATE_HOME", stateHome)
	if err := installer.SaveTargetSelection(installer.Config{ClaudeHome: home, ClickStateHome: stateHome}, installer.TargetSelection{Configured: true, Claude: true}); err != nil {
		t.Fatalf("SaveTargetSelection() error = %v", err)
	}

	runner := &failingClaudeRunner{}
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()
	missingClaude := cliFakeBinaryLookup{resolved: map[string]string{"git": "/usr/bin/git"}}

	out, err := execRootWithLookupAndState(t, home, stateHome, missingClaude, "uninstall")
	if err == nil {
		t.Fatalf("uninstall command error = nil when claude is missing, want non-nil, output:\n%s", out)
	}
	if !strings.Contains(out, installer.ClaudeMissingMessage) && !strings.Contains(err.Error(), installer.ClaudeMissingMessage) {
		t.Fatalf("uninstall output/error did not contain the actionable ClaudeMissingMessage; output:\n%s\nerror: %v", out, err)
	}

	// Resilience: CLAUDE.md must still have been stripped even though claude is missing and every
	// claude-dependent step failed.
	md, readErr := os.ReadFile(cfg.ClaudeMDPath())
	if readErr != nil {
		t.Fatalf("ReadFile(ClaudeMDPath) error = %v", readErr)
	}
	if !strings.Contains(string(md), "click-ai-devkit (managed)") {
		t.Fatalf("CLAUDE.md = %q, want rollback to restore the managed block when claude-dependent teardown fails", md)
	}
}

// TestUninstallCommand_RemovesOpenClawMemoryGuardPlugin is task 3.12's RED test: `click uninstall`
// must remove the click-memory-guard OpenClaw plugin directory (parity with how the Claude Code
// memory-guard hook gets unregistered a few lines above it in runUninstall).
func TestUninstallCommand_RemovesOpenClawMemoryGuardPlugin(t *testing.T) {
	claudeHome := t.TempDir()
	openClawHome := t.TempDir()
	cfg := installer.Config{ClaudeHome: claudeHome, OpenClawHome: openClawHome}
	restoreExec := installer.SetOSExecutableForTests(func() (string, error) { return "/opt/click/bin/click", nil })
	if err := installer.SyncOpenClawPlugin(cfg); err != nil {
		restoreExec()
		t.Fatalf("SyncOpenClawPlugin() error = %v", err)
	}
	restoreExec()
	if _, err := os.Stat(cfg.OpenClawPluginDir()); err != nil {
		t.Fatalf("Stat(plugin dir) before uninstall error = %v, want it to exist first", err)
	}

	runner := newTestCommandRunner(claudeHome)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()

	out, err := execRootWithOpenClaw(t, claudeHome, openClawHome, "uninstall")
	if err != nil {
		t.Fatalf("uninstall command error = %v, output:\n%s", err, out)
	}
	if _, err := os.Stat(cfg.OpenClawPluginDir()); !os.IsNotExist(err) {
		t.Fatalf("Stat(plugin dir) after uninstall error = %v, want os.IsNotExist", err)
	}
}

// TestUninstallCommand_OpenClawNeverInstalled_NoOpNoError guards the "OpenClaw never touched this
// machine" case — `click uninstall` must succeed without error even when
// <OpenClawHome>/plugins/click-memory-guard was never created.
func TestUninstallCommand_OpenClawNeverInstalled_NoOpNoError(t *testing.T) {
	home := t.TempDir()
	runner := newTestCommandRunner(home)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()

	out, err := execRoot(t, home, "uninstall")
	if err != nil {
		t.Fatalf("uninstall command error = %v, want nil when OpenClaw was never installed, output:\n%s", err, out)
	}
}

// TestUninstallCommand_RemovesOpenClawSkillsAndPreservesSiblings is PR4's RED test: `click
// uninstall` must remove the click-owned OpenClaw skill directories (clickhola, clickdev) while
// leaving any user-created sibling skill directories untouched.
func TestUninstallCommand_RemovesOpenClawSkillsAndPreservesSiblings(t *testing.T) {
	claudeHome := t.TempDir()
	openClawHome := t.TempDir()
	cfg := installer.Config{ClaudeHome: claudeHome, OpenClawHome: openClawHome}

	restoreExec := installer.SetOSExecutableForTests(func() (string, error) { return "/opt/click/bin/click", nil })
	if err := installer.SyncOpenClawPlugin(cfg); err != nil {
		restoreExec()
		t.Fatalf("SyncOpenClawPlugin() error = %v", err)
	}
	if err := installer.SyncOpenClawSkills(cfg); err != nil {
		restoreExec()
		t.Fatalf("SyncOpenClawSkills() error = %v", err)
	}
	restoreExec()

	// Simulate a user-created sibling skill.
	sibling := filepath.Join(cfg.OpenClawSkillsDir(), "user-skill")
	if err := os.MkdirAll(sibling, 0o755); err != nil {
		t.Fatalf("MkdirAll(sibling) error = %v", err)
	}

	runner := newTestCommandRunner(claudeHome)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()

	out, err := execRootWithOpenClaw(t, claudeHome, openClawHome, "uninstall")
	if err != nil {
		t.Fatalf("uninstall command error = %v, output:\n%s", err, out)
	}

	for _, owned := range []string{"clickhola", "clickdev"} {
		path := filepath.Join(cfg.OpenClawSkillsDir(), owned)
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("Stat(%s) after uninstall error = %v, want os.IsNotExist", owned, err)
		}
	}
	if _, err := os.Stat(sibling); err != nil {
		t.Fatalf("Stat(user-skill) after uninstall error = %v, want sibling preserved", err)
	}
}

// TestUninstallCommand_RemoveOpenClawSkillsError_ContinuesTeardown is PR4's resilience RED test: a
// failure removing the click-owned OpenClaw skill directories must be recorded and reported, but
// every other teardown step must still run to completion.
func TestUninstallCommand_RemoveOpenClawSkillsError_ContinuesTeardown(t *testing.T) {
	claudeHome := t.TempDir()
	openClawHome := t.TempDir()
	cfg := installer.Config{ClaudeHome: claudeHome, OpenClawHome: openClawHome}

	if err := installer.WriteManagedBlock(cfg.ClaudeMDPath(), installer.DefaultManagedContent); err != nil {
		t.Fatalf("WriteManagedBlock() error = %v", err)
	}
	if err := installer.RegisterMemoryGuardHook(cfg); err != nil {
		t.Fatalf("RegisterMemoryGuardHook() error = %v", err)
	}

	injectedErr := errors.New("injected remove openclaw skills failure")
	restoreSkills := SetRemoveOpenClawSkillsFuncForTests(func(c installer.Config) error { return injectedErr })
	defer restoreSkills()

	runner := newTestCommandRunner(claudeHome)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()

	out, err := execRootWithOpenClaw(t, claudeHome, openClawHome, "uninstall")
	if err == nil {
		t.Fatalf("uninstall command error = nil, want non-nil when RemoveOpenClawSkills fails, output:\n%s", out)
	}

	md, readErr := os.ReadFile(cfg.ClaudeMDPath())
	if readErr != nil {
		t.Fatalf("ReadFile(ClaudeMDPath) error = %v", readErr)
	}
	if !strings.Contains(string(md), "click-ai-devkit (managed)") {
		t.Fatalf("CLAUDE.md = %q, want rollback to restore the managed block after RemoveOpenClawSkills fails", md)
	}

	hasHook, hookErr := installer.HasMemoryGuardHook(cfg)
	if hookErr != nil {
		t.Fatalf("HasMemoryGuardHook() error = %v", hookErr)
	}
	if !hasHook {
		t.Fatal("memory-guard hook missing after RemoveOpenClawSkills failure, want rollback to restore it")
	}

	if !strings.Contains(out, "skills") {
		t.Fatalf("uninstall output = %q, want the final summary to name the failed OpenClaw skills step", out)
	}
}

// TestUninstallCommand_CorruptedEngramState_SucceedsWithWarning is Finding 3's CLI-level regression
// test: a truncated/corrupted engram.json must not abort `click uninstall` — RemoveEngramPlugin now
// reports it as a warning (installer.RemoveEngramPlugin's own corrupted-state handling), so with
// every other step healthy the overall command must still succeed, and the warning must be visible
// in the output rather than silently dropped.
func TestUninstallCommand_CorruptedEngramState_SucceedsWithWarning(t *testing.T) {
	home := t.TempDir()
	runner := newTestCommandRunner(home)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()

	cfg := installer.Config{ClaudeHome: home}
	statePath := cfg.EngramStatePath()
	if err := os.MkdirAll(filepath.Dir(statePath), 0o755); err != nil {
		t.Fatalf("MkdirAll(engram state dir) error = %v", err)
	}
	if err := os.WriteFile(statePath, []byte("{not valid json"), 0o600); err != nil {
		t.Fatalf("WriteFile(engram.json) error = %v", err)
	}

	out, err := execRoot(t, home, "uninstall")
	if err != nil {
		t.Fatalf("uninstall command error = %v, want nil — a corrupted engram.json must be a warning, not a fatal failure, output:\n%s", err, out)
	}
	if !strings.Contains(out, "dañado") {
		t.Fatalf("uninstall output = %q, want it to contain the corrupted-state warning", out)
	}
}

func TestUninstallCommand_CodexOnlySelection_DoesNotRequireClaudeAndRemovesNeutralStateLast(t *testing.T) {
	claudeHome := t.TempDir()
	stateHome := t.TempDir()
	codexHome := t.TempDir()
	t.Setenv("CLICK_STATE_HOME", stateHome)
	t.Setenv("CODEX_HOME", codexHome)

	selectionCfg := installer.Config{ClaudeHome: claudeHome, ClickStateHome: stateHome}
	if err := installer.SaveTargetSelection(selectionCfg, installer.TargetSelection{Configured: true, Codex: true}); err != nil {
		t.Fatalf("SaveTargetSelection() error = %v", err)
	}
	codexCfg := installer.Config{CodexHome: codexHome}
	if err := installer.WriteManagedBlock(codexCfg.CodexAgentsMDPath(), installer.DefaultCodexAgentsContent); err != nil {
		t.Fatalf("WriteManagedBlock(CodexAgentsMDPath) error = %v", err)
	}

	lookup := cliFakeBinaryLookup{resolved: map[string]string{"codex": "/usr/bin/codex"}}
	out, err := execRootWithLookupAndState(t, claudeHome, stateHome, lookup, "uninstall")
	if err != nil {
		t.Fatalf("uninstall command error = %v, want nil for a Codex-only teardown, output:\n%s", err, out)
	}
	if _, err := os.Stat(codexCfg.CodexAgentsMDPath()); !os.IsNotExist(err) {
		t.Fatalf("Stat(CodexAgentsMDPath) error = %v, want Codex guidance removed", err)
	}
	if _, err := os.Stat(filepath.Join(stateHome, "targets.json")); !os.IsNotExist(err) {
		t.Fatalf("Stat(targets.json) error = %v, want neutral state removed after successful teardown", err)
	}
	if strings.Contains(out, installer.ClaudeMissingMessage) {
		t.Fatalf("uninstall output = %q, want no Claude dependency warning in a Codex-only teardown", out)
	}
}

func TestUninstallCommand_FailedCodexTeardown_RestoresSnapshotAndKeepsNeutralState(t *testing.T) {
	claudeHome := t.TempDir()
	stateHome := t.TempDir()
	codexHome := t.TempDir()
	t.Setenv("CLICK_STATE_HOME", stateHome)
	t.Setenv("CODEX_HOME", codexHome)

	selectionCfg := installer.Config{ClaudeHome: claudeHome, ClickStateHome: stateHome}
	if err := installer.SaveTargetSelection(selectionCfg, installer.TargetSelection{Configured: true, Codex: true}); err != nil {
		t.Fatalf("SaveTargetSelection() error = %v", err)
	}
	codexCfg := installer.Config{CodexHome: codexHome}
	if err := installer.WriteManagedBlock(codexCfg.CodexAgentsMDPath(), installer.DefaultCodexAgentsContent); err != nil {
		t.Fatalf("WriteManagedBlock(CodexAgentsMDPath) error = %v", err)
	}
	original, readOriginalErr := os.ReadFile(codexCfg.CodexAgentsMDPath())
	if readOriginalErr != nil {
		t.Fatalf("ReadFile(CodexAgentsMDPath) error = %v", readOriginalErr)
	}
	unrelated := filepath.Join(codexHome, "notes.txt")
	if err := os.WriteFile(unrelated, []byte("keep me\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(unrelated) error = %v", err)
	}

	injectedErr := errors.New("injected codex teardown failure")
	restoreStrip := SetStripCodexGuidanceFuncForTests(func(cfg installer.Config) error {
		if err := os.Remove(cfg.CodexAgentsMDPath()); err != nil {
			return err
		}
		return injectedErr
	})
	defer restoreStrip()

	lookup := cliFakeBinaryLookup{resolved: map[string]string{"codex": "/usr/bin/codex"}}
	out, err := execRootWithLookupAndState(t, claudeHome, stateHome, lookup, "uninstall")
	if err == nil {
		t.Fatalf("uninstall command error = nil, want rollback-triggering failure, output:\n%s", out)
	}
	got, readErr := os.ReadFile(codexCfg.CodexAgentsMDPath())
	if readErr != nil {
		t.Fatalf("ReadFile(CodexAgentsMDPath) error = %v, want rollback to restore the captured file", readErr)
	}
	if string(got) != string(original) {
		t.Fatalf("Codex guidance after rollback = %q, want %q", got, original)
	}
	if _, err := os.Stat(filepath.Join(stateHome, "targets.json")); err != nil {
		t.Fatalf("Stat(targets.json) error = %v, want neutral state preserved when teardown rolls back", err)
	}
	if keep, err := os.ReadFile(unrelated); err != nil || string(keep) != "keep me\n" {
		t.Fatalf("unrelated file = %q, err = %v, want unrelated content untouched", keep, err)
	}
	if !strings.Contains(out, "Codex") {
		t.Fatalf("uninstall output = %q, want the failed Codex step reported", out)
	}
}
