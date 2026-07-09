package installer

import (
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
