package cli

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/audit"
	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/guard"
	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/installer"
	"github.com/spf13/cobra"
)

type preToolUsePayload struct {
	SessionID     string `json:"session_id"`
	CWD           string `json:"cwd"`
	HookEventName string `json:"hook_event_name"`
	ToolName      string `json:"tool_name"`
	ToolInput     any    `json:"tool_input"`
}

var memoryGuardDecodeHook = decodePreToolUsePayload

func newMemoryGuardCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "memory-guard",
		Short:  "Internal PreToolUse hook for Engram mem_save",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMemoryGuard(cmd)
		},
	}
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	return cmd
}

func runMemoryGuard(cmd *cobra.Command) (err error) {
	stdout := cmd.OutOrStdout()
	stderr := cmd.ErrOrStderr()

	defer func() {
		if recovered := recover(); recovered != nil {
			_, _ = fmt.Fprintln(stderr, "memory-guard panic")
			err = &exitCodeError{code: 2, msg: "memory-guard panic"}
		}
	}()

	raw, readErr := io.ReadAll(cmd.InOrStdin())
	if readErr != nil {
		return failClosed(stderr, "memory-guard read failure")
	}
	payload, decodeErr := memoryGuardDecodeHook(raw)
	if decodeErr != nil {
		return failClosed(stderr, "memory-guard invalid payload")
	}
	canonicalInput, marshalErr := canonicalToolInput(payload.ToolInput)
	if marshalErr != nil {
		return failClosed(stderr, "memory-guard invalid tool_input")
	}

	decision, scanErr := evaluateMemoryGuard(payload.ToolName, canonicalInput)
	if scanErr != nil {
		return failClosed(stderr, "memory-guard scan failure")
	}

	logPath, logPathErr := audit.ResolveLogPath()
	if logPathErr != nil {
		return failClosed(stderr, "memory-guard audit path failure")
	}
	if err := audit.Append(logPath, audit.Entry{
		Decision:      permissionDecision(decision),
		Category:      decision.Category,
		ContentSHA256: audit.SHA256(canonicalInput),
		SessionID:     payload.SessionID,
	}); err != nil {
		return failClosed(stderr, "memory-guard audit failure")
	}

	response := map[string]any{
		"hookSpecificOutput": map[string]any{
			"hookEventName":      "PreToolUse",
			"permissionDecision": permissionDecision(decision),
		},
	}
	if decision.Blocked {
		response["hookSpecificOutput"].(map[string]any)["permissionDecisionReason"] = decision.Reason
	}
	if err := json.NewEncoder(stdout).Encode(response); err != nil {
		return failClosed(stderr, "memory-guard write failure")
	}
	return nil
}

func decodePreToolUsePayload(raw []byte) (preToolUsePayload, error) {
	var payload preToolUsePayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return preToolUsePayload{}, err
	}
	return payload, nil
}

func canonicalToolInput(input any) (string, error) {
	data, err := json.Marshal(input)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func evaluateMemoryGuard(toolName, payload string) (guard.Decision, error) {
	if toolName != installer.MemoryGuardToolMatcher {
		return guard.Decision{}, nil
	}
	return guard.ScanWithError(payload)
}

func permissionDecision(decision guard.Decision) string {
	if decision.Blocked {
		return "deny"
	}
	return "allow"
}

func failClosed(stderr io.Writer, reason string) error {
	_, _ = fmt.Fprintln(stderr, reason)
	return &exitCodeError{code: 2, msg: reason}
}
