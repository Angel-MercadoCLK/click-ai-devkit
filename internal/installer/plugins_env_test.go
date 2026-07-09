package installer

import (
	"os"
	"path/filepath"
	"testing"
)

// TestExecRunnerBridgesClickHomeToClaudeConfigDir guards the isolation rule: when click is
// redirected via CLICK_CLAUDE_HOME, the `claude` subprocess it spawns MUST also be redirected
// (via CLAUDE_CONFIG_DIR) so plugin registration lands in the same place — otherwise a test or
// override run would silently install plugins into the developer's REAL ~/.claude.
func TestExecRunnerBridgesClickHomeToClaudeConfigDir(t *testing.T) {
	override := filepath.Join(t.TempDir(), "throwaway-claude")
	t.Setenv("CLICK_CLAUDE_HOME", override)

	runner, ok := commandRunnerFactory().(execCommandRunner)
	if !ok {
		t.Fatalf("expected execCommandRunner, got %T", commandRunnerFactory())
	}

	env := runner.commandEnv()
	want := "CLAUDE_CONFIG_DIR=" + override
	for _, e := range env {
		if e == want {
			return // pass: the override is propagated to the claude subprocess
		}
	}
	t.Fatalf("claude subprocess env is missing %q, so a CLICK_CLAUDE_HOME override would leak plugin installs into the real ~/.claude; got %v", want, env)
}

// TestExecRunnerRealRunLeavesClaudeConfigDirUnset guards the OTHER half: on a real run (no
// CLICK_CLAUDE_HOME override), click must NOT force CLAUDE_CONFIG_DIR. Forcing it to ~/.claude
// silently redirected `claude mcp add` (Context7) to ~/.claude/.claude.json — a file a normal
// Claude Code session never reads — so the MCP install was invisible in real use.
func TestExecRunnerRealRunLeavesClaudeConfigDirUnset(t *testing.T) {
	t.Setenv("CLICK_CLAUDE_HOME", "") // real run: no explicit override

	runner, ok := commandRunnerFactory().(execCommandRunner)
	if !ok {
		t.Fatalf("expected execCommandRunner, got %T", commandRunnerFactory())
	}
	if env := runner.commandEnv(); env != nil {
		t.Fatalf("a real run must NOT force CLAUDE_CONFIG_DIR (it misdirects `claude mcp add` for Context7); got %v", env)
	}
}

// TestContext7ConfigPathMirrorsWhereClaudeWrites: HasContext7 must read the SAME .claude.json the
// runner's `claude mcp add` writes — <override>/.claude.json under CLICK_CLAUDE_HOME, else the OS
// home ROOT's .claude.json (NOT <ClaudeHome>/.claude.json, i.e. not ~/.claude/.claude.json).
func TestContext7ConfigPathMirrorsWhereClaudeWrites(t *testing.T) {
	override := t.TempDir()
	t.Setenv("CLICK_CLAUDE_HOME", override)
	if got, want := (Config{ClaudeHome: override}).Context7ConfigPath(), filepath.Join(override, ".claude.json"); got != want {
		t.Fatalf("override case: got %q want %q", got, want)
	}

	t.Setenv("CLICK_CLAUDE_HOME", "")
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("no user home: %v", err)
	}
	cfg := Config{ClaudeHome: filepath.Join(home, ".claude")}
	if got, want := cfg.Context7ConfigPath(), filepath.Join(home, ".claude.json"); got != want {
		t.Fatalf("real case: got %q want %q (must be home ROOT, not inside ~/.claude)", got, want)
	}
}
