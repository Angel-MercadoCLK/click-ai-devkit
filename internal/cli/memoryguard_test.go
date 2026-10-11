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

// blockedContent trips the guard's pii/cuit-cuil rule wherever it appears in a payload.
// click:allow-secret synthetic sequential-placeholder CUIT fixture for guard tests; not real PII.
const blockedContent = "CUIT 20-12345678-3 del asegurado"

func TestEvaluateMemoryGuard_DeniesEveryPersistingEngramToolInBothNameForms(t *testing.T) {
	// field is the content-bearing input field each persisting tool accepts.
	cases := map[string]string{
		"mem_save":            "content",
		"mem_update":          "content",
		"mem_save_prompt":     "content",
		"mem_session_summary": "content",
		"mem_session_end":     "summary",
		"mem_capture_passive": "content",
		"mem_judge":           "reason",
		"mem_compare":         "reasoning",
	}
	for tool, field := range cases {
		for _, prefix := range []string{"mcp__plugin_engram_engram__", "mcp__engram__"} {
			name := prefix + tool
			t.Run(name, func(t *testing.T) {
				input, err := canonicalToolInput(map[string]any{field: blockedContent})
				if err != nil {
					t.Fatalf("canonicalToolInput() error = %v", err)
				}
				decision, err := evaluateMemoryGuard(name, input)
				if err != nil {
					t.Fatalf("evaluateMemoryGuard() error = %v", err)
				}
				if !decision.Blocked {
					t.Fatalf("evaluateMemoryGuard(%q) allowed a blocked payload", name)
				}
			})
		}
	}
}

func TestEvaluateMemoryGuard_ScansEveryFieldOfMemUpdate(t *testing.T) {
	for _, field := range []string{"title", "content", "topic_key", "find", "replace"} {
		t.Run(field, func(t *testing.T) {
			input, err := canonicalToolInput(map[string]any{"id": 7, field: blockedContent})
			if err != nil {
				t.Fatalf("canonicalToolInput() error = %v", err)
			}
			decision, err := evaluateMemoryGuard("mcp__plugin_engram_engram__mem_update", input)
			if err != nil {
				t.Fatalf("evaluateMemoryGuard() error = %v", err)
			}
			if !decision.Blocked {
				t.Fatalf("mem_update field %q was not scanned", field)
			}
		})
	}
}

func TestEvaluateMemoryGuard_AllowsReadOnlyAndUnrelatedToolsWithoutScanning(t *testing.T) {
	names := []string{
		"mcp__plugin_engram_engram__mem_search",
		"mcp__plugin_engram_engram__mem_context",
		"mcp__plugin_engram_engram__mem_get_observation",
		"mcp__plugin_engram_engram__mem_current_project",
		"mcp__engram__mem_search",
		"mcp__engram__mem_timeline",
		"mcp__engram__mem_stats",
		"mcp__other_server__mem_save",
		"Bash",
		"",
	}
	input, err := canonicalToolInput(map[string]any{"query": blockedContent})
	if err != nil {
		t.Fatalf("canonicalToolInput() error = %v", err)
	}
	for _, name := range names {
		decision, err := evaluateMemoryGuard(name, input)
		if err != nil {
			t.Fatalf("evaluateMemoryGuard(%q) error = %v", name, err)
		}
		if decision.Blocked {
			t.Fatalf("evaluateMemoryGuard(%q) blocked a tool that never persists content", name)
		}
	}
}

func TestMemoryGuardCommand_DeniesMemUpdateAndStandaloneForms(t *testing.T) {
	for _, toolName := range []string{"mcp__plugin_engram_engram__mem_update", "mcp__engram__mem_save", "mcp__engram__mem_update"} {
		t.Run(toolName, func(t *testing.T) {
			// click:allow-secret synthetic sequential-placeholder CUIT input for the memory-guard unit test; not user data.
			stdin := `{"session_id":"sess-3","cwd":"/repo","hook_event_name":"PreToolUse","tool_name":"` + toolName + `","tool_input":{"id":1,"content":"CUIT 20-12345678-3 del asegurado"}}`
			stdout, stderr, err := execRootWithIO(t, t.TempDir(), stdin, "memory-guard")
			if err != nil {
				t.Fatalf("memory-guard error = %v, stderr=%q stdout=%q", err, stderr, stdout)
			}
			var got map[string]any
			if err := json.Unmarshal([]byte(stdout), &got); err != nil {
				t.Fatalf("stdout is not valid JSON: %v\n%s", err, stdout)
			}
			if got["hookSpecificOutput"].(map[string]any)["permissionDecision"] != "deny" {
				t.Fatalf("permissionDecision = %v, want deny", got["hookSpecificOutput"])
			}
		})
	}
}
