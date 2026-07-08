package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"
)

func execRootWithIO(t *testing.T, claudeHome string, stdin string, args ...string) (string, string, error) {
	t.Helper()
	t.Setenv("CLICK_CLAUDE_HOME", claudeHome)

	root := NewRootCommand()
	var out bytes.Buffer
	var errBuf bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errBuf)
	root.SetIn(bytes.NewBufferString(stdin))
	root.SetArgs(args)

	err := root.Execute()
	return out.String(), errBuf.String(), err
}

func TestMemoryGuardCommand_AllowsBenignPayloadWithJSONOnlyStdout(t *testing.T) {
	stdin := `{"session_id":"sess-1","cwd":"C:/repo","hook_event_name":"PreToolUse","tool_name":"mcp__plugin_engram_engram__mem_save","tool_input":{"title":"ADR","content":"Store architecture decisions only"}}`

	stdout, stderr, err := execRootWithIO(t, t.TempDir(), stdin, "memory-guard")
	if err != nil {
		t.Fatalf("memory-guard error = %v, stderr=%q stdout=%q", err, stderr, stdout)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	if bytes.Contains([]byte(stdout), []byte("CLICK")) {
		t.Fatalf("stdout contains banner text: %q", stdout)
	}

	var got map[string]any
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\n%s", err, stdout)
	}
	decision := got["hookSpecificOutput"].(map[string]any)
	if decision["permissionDecision"] != "allow" {
		t.Fatalf("permissionDecision = %v, want allow", decision["permissionDecision"])
	}
}

func TestMemoryGuardCommand_DeniesForbiddenPayload(t *testing.T) {
	stdin := `{"session_id":"sess-2","cwd":"C:/repo","hook_event_name":"PreToolUse","tool_name":"mcp__plugin_engram_engram__mem_save","tool_input":{"content":"CUIT 20-12345678-3 del asegurado"}}`

	stdout, stderr, err := execRootWithIO(t, t.TempDir(), stdin, "memory-guard")
	if err != nil {
		t.Fatalf("memory-guard error = %v, stderr=%q stdout=%q", err, stderr, stdout)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}

	var got map[string]any
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\n%s", err, stdout)
	}
	decision := got["hookSpecificOutput"].(map[string]any)
	if decision["permissionDecision"] != "deny" {
		t.Fatalf("permissionDecision = %v, want deny", decision["permissionDecision"])
	}
	if decision["permissionDecisionReason"] == "" {
		t.Fatal("permissionDecisionReason = empty, want a short non-sensitive reason")
	}
}

func TestMemoryGuardCommand_InvalidJSONFailsClosedWithExitCode2(t *testing.T) {
	stdout, stderr, err := execRootWithIO(t, t.TempDir(), `{not-json`, "memory-guard")
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty on fail-closed", stdout)
	}
	if stderr == "" {
		t.Fatal("stderr = empty, want a short fail-closed reason")
	}
	var exitErr interface{ ExitCode() int }
	if !errors.As(err, &exitErr) {
		t.Fatalf("error = %T %v, want an exit-coded error", err, err)
	}
	if exitErr.ExitCode() != 2 {
		t.Fatalf("ExitCode() = %d, want 2", exitErr.ExitCode())
	}
}

func TestMemoryGuardCommand_PanicFailsClosedWithExitCode2(t *testing.T) {
	original := memoryGuardDecodeHook
	memoryGuardDecodeHook = func([]byte) (preToolUsePayload, error) {
		panic("boom")
	}
	t.Cleanup(func() { memoryGuardDecodeHook = original })

	stdout, stderr, err := execRootWithIO(t, t.TempDir(), `{}`, "memory-guard")
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty on fail-closed panic", stdout)
	}
	if stderr == "" {
		t.Fatal("stderr = empty, want a short panic reason")
	}
	var exitErr interface{ ExitCode() int }
	if !errors.As(err, &exitErr) {
		t.Fatalf("error = %T %v, want an exit-coded error", err, err)
	}
	if exitErr.ExitCode() != 2 {
		t.Fatalf("ExitCode() = %d, want 2", exitErr.ExitCode())
	}
}
