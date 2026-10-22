package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/spf13/cobra"
)

// `click dot-hook <Event>` is the hidden Claude Code hook that relays an event to Click Dot, the
// Windows desktop companion (odd/tasks/dot-agent-companion.md, S3-S5 and S7 and the wire contract).
//
// It fails open ALWAYS: any problem (no discovery file, refused connection, timeout, a status other
// than 200, a malformed or unacceptable reply, a panic) prints nothing to stdout, keeps stderr quiet
// and exits 0, so Claude Code falls back to its own terminal prompt. It never allows or denies
// without an explicit decision from Dot, and only PermissionRequest may produce output.

const (
	dotHookDiscoveryEnv      = "CLICK_DOT_AGENT_HOOK_FILE"
	dotHookDiscoveryDirName  = "Click Dot"
	dotHookDiscoveryFileName = "agent-hook.json"
	dotHookEndpointPath      = "/api/agent/hook"

	dotHookStringCap       = 2000
	dotHookEditInputCap    = 256 << 10
	dotHookMaxStdinBytes   = 32 << 20
	dotHookMaxReplyBytes   = 1 << 20
	dotHookMaxDiscoveryLen = 64 << 10

	dotHookDenyMessage = "Denied from Click Dot"
)

// dotHookEvents are the only events the relay forwards; they match the entries `click install`
// registers (installer.dotHookEvents).
var dotHookKnownEvents = map[string]bool{
	"PermissionRequest": true, "SessionStart": true, "SessionEnd": true, "UserPromptSubmit": true,
	"PreToolUse": true, "PostToolUse": true, "PostToolUseFailure": true, "Notification": true, "Stop": true,
}

// dotHookTimeoutSet holds the relay timeouts. Tests shorten them; production values are the wire
// contract: connect 300 ms, 2 s in total for non-permission events, 110 s for PermissionRequest.
type dotHookTimeoutSet struct {
	connect, event, permission time.Duration
}

var dotHookTimeouts = dotHookTimeoutSet{connect: 300 * time.Millisecond, event: 2 * time.Second, permission: 110 * time.Second}

func (t dotHookTimeoutSet) totalFor(event string) time.Duration {
	if event == "PermissionRequest" {
		return t.permission
	}
	return t.event
}

// dotHookReply is Dot's HTTP reply: the status code and the (bounded) body.
type dotHookReply struct {
	status int
	body   []byte
}

// dotHookPost is the transport seam: it POSTs body to Dot on loopback and reports ok=false on any
// transport failure.
var dotHookPost = postToDot

func newDotHookCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "dot-hook <Event>",
		Short:  "Internal Claude Code hook that relays agent events to Click Dot",
		Hidden: true,
		// Hooks must never fail on unexpected input: flags and extra arguments are tolerated.
		DisableFlagParsing: true,
		Args:               cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			runDotHook(cmd, args)
			return nil
		},
	}
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	return cmd
}

// runDotHook never returns an error and never panics: fail open is the only failure mode.
func runDotHook(cmd *cobra.Command, args []string) {
	defer func() { _ = recover() }()

	raw, err := io.ReadAll(io.LimitReader(cmd.InOrStdin(), dotHookMaxStdinBytes+1))
	if err != nil || len(raw) > dotHookMaxStdinBytes {
		return
	}
	original, ok := decodeJSONObject(raw)
	if !ok {
		return
	}
	event := resolveDotHookEvent(args, original)
	if event == "" {
		return
	}
	discovery, ok := readDotHookDiscovery()
	if !ok {
		return
	}
	body, err := json.Marshal(dotHookRequest{Version: 1, Agent: "claude-code", Event: event, Payload: normalizeDotHookPayload(original)})
	if err != nil {
		return
	}
	reply, ok := dotHookPost(discovery.Port, discovery.Token, body, dotHookTimeouts.totalFor(event))
	if !ok || reply.status != http.StatusOK || event != "PermissionRequest" {
		return
	}
	decision, ok := mapDotHookDecision(reply.body, original)
	if !ok {
		return
	}
	out, err := encodeDotHookOutput(decision)
	if err != nil {
		return
	}
	_, _ = cmd.OutOrStdout().Write(out)
}

// resolveDotHookEvent prefers the event the hook was registered for (the command argument) and falls
// back to the payload's hook_event_name; an event the devkit never registers is not forwarded.
func resolveDotHookEvent(args []string, payload map[string]any) string {
	if len(args) > 0 && dotHookKnownEvents[args[0]] {
		return args[0]
	}
	if name, _ := payload["hook_event_name"].(string); dotHookKnownEvents[name] {
		return name
	}
	return ""
}

type dotHookRequest struct {
	Version int            `json:"version"`
	Agent   string         `json:"agent"`
	Event   string         `json:"event"`
	Payload map[string]any `json:"payload"`
}

type dotHookDiscovery struct {
	Version int    `json:"version"`
	Port    int    `json:"port"`
	Token   string `json:"token"`
}

func dotHookDiscoveryPath() (string, bool) {
	if override := os.Getenv(dotHookDiscoveryEnv); override != "" {
		return override, true
	}
	configDir, err := os.UserConfigDir()
	if err != nil || configDir == "" {
		return "", false
	}
	return filepath.Join(configDir, dotHookDiscoveryDirName, dotHookDiscoveryFileName), true
}

func readDotHookDiscovery() (dotHookDiscovery, bool) {
	path, ok := dotHookDiscoveryPath()
	if !ok {
		return dotHookDiscovery{}, false
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > dotHookMaxDiscoveryLen {
		return dotHookDiscovery{}, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return dotHookDiscovery{}, false
	}
	var discovery dotHookDiscovery
	if err := json.Unmarshal(data, &discovery); err != nil {
		return dotHookDiscovery{}, false
	}
	if discovery.Version != 1 || discovery.Port < 1 || discovery.Port > 65535 || discovery.Token == "" || len(discovery.Token) > 256 {
		return dotHookDiscovery{}, false
	}
	for _, r := range discovery.Token {
		if r <= ' ' || r == 0x7f {
			return dotHookDiscovery{}, false
		}
	}
	return discovery, true
}

func postToDot(port int, token string, body []byte, total time.Duration) (dotHookReply, bool) {
	dialer := &net.Dialer{Timeout: dotHookTimeouts.connect}
	client := &http.Client{
		Timeout: total,
		Transport: &http.Transport{
			// Never route a loopback call through an ambient proxy.
			Proxy:             nil,
			DialContext:       dialer.DialContext,
			DisableKeepAlives: true,
		},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	req, err := http.NewRequest(http.MethodPost, "http://127.0.0.1:"+strconv.Itoa(port)+dotHookEndpointPath, bytes.NewReader(body))
	if err != nil {
		return dotHookReply{}, false
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return dotHookReply{}, false
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, dotHookMaxReplyBytes))
	if err != nil {
		return dotHookReply{}, false
	}
	return dotHookReply{status: resp.StatusCode, body: data}, true
}

// decodeJSONObject parses exactly one JSON object, keeping numbers verbatim (json.Number) so values
// compare and re-encode without float rounding.
func decodeJSONObject(raw []byte) (map[string]any, bool) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, false
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, false
	}
	object, ok := value.(map[string]any)
	return object, ok && object != nil
}

// normalizeDotHookPayload returns a copy of the hook payload ready to leave the machine: it drops
// transcript_path and tool_response and caps every string at 2,000 characters. The exceptions are
// the tool_input of Edit, MultiEdit and Write (kept whole while it serializes to at most 256 KiB, so
// Dot can count added and removed lines) and of AskUserQuestion (never truncated: Dot must answer
// against the exact question and option texts). permission_suggestions are never truncated either:
// Siempre echoes one back and the relay accepts it only when it equals the original exactly. The
// input map is never modified.
func normalizeDotHookPayload(payload map[string]any) map[string]any {
	tool, _ := payload["tool_name"].(string)
	out := make(map[string]any, len(payload))
	for key, value := range payload {
		switch key {
		case "transcript_path", "tool_response":
			continue
		case "permission_suggestions":
			out[key] = value
		case "tool_input":
			out[key] = normalizeDotHookToolInput(tool, value)
		default:
			out[key] = capDotHookStrings(value)
		}
	}
	return out
}

func normalizeDotHookToolInput(tool string, input any) any {
	switch tool {
	case "AskUserQuestion":
		return input
	case "Edit", "MultiEdit", "Write":
		if encoded, err := json.Marshal(input); err == nil && len(encoded) <= dotHookEditInputCap {
			return input
		}
	}
	return capDotHookStrings(input)
}

func capDotHookStrings(value any) any {
	switch v := value.(type) {
	case string:
		return capDotHookString(v)
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = capDotHookStrings(item)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, item := range v {
			out[key] = capDotHookStrings(item)
		}
		return out
	}
	return value
}

func capDotHookString(s string) string {
	if len(s) <= dotHookStringCap {
		return s
	}
	count := 0
	for i := range s {
		if count == dotHookStringCap {
			return s[:i]
		}
		count++
	}
	return s
}

type dotHookDecision struct {
	Behavior           string         `json:"behavior"`
	Message            string         `json:"message,omitempty"`
	UpdatedInput       map[string]any `json:"updatedInput,omitempty"`
	UpdatedPermissions []any          `json:"updatedPermissions,omitempty"`
}

type dotHookSpecificOutput struct {
	HookEventName string          `json:"hookEventName"`
	Decision      dotHookDecision `json:"decision"`
}

type dotHookOutput struct {
	HookSpecificOutput dotHookSpecificOutput `json:"hookSpecificOutput"`
}

func encodeDotHookOutput(decision dotHookDecision) ([]byte, error) {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	err := encoder.Encode(dotHookOutput{HookSpecificOutput: dotHookSpecificOutput{HookEventName: "PermissionRequest", Decision: decision}})
	return buf.Bytes(), err
}

// mapDotHookDecision turns Dot's 200 reply into the Claude Code decision, or reports ok=false when
// the reply is malformed or asks for something Claude Code never offered (fail open).
func mapDotHookDecision(reply []byte, original map[string]any) (dotHookDecision, bool) {
	response, ok := decodeJSONObject(reply)
	if !ok {
		return dotHookDecision{}, false
	}
	behavior, _ := response["behavior"].(string)
	answers, hasAnswers := response["answers"]
	hasAnswers = hasAnswers && answers != nil
	permissions, hasPermissions := response["updatedPermissions"]
	hasPermissions = hasPermissions && permissions != nil

	switch behavior {
	case "deny":
		if hasAnswers || hasPermissions {
			return dotHookDecision{}, false
		}
		return dotHookDecision{Behavior: "deny", Message: dotHookDenyMessage}, true
	case "allow":
		switch {
		case hasAnswers && hasPermissions:
			return dotHookDecision{}, false
		case hasAnswers:
			updated, ok := answeredToolInput(answers, original)
			if !ok {
				return dotHookDecision{}, false
			}
			return dotHookDecision{Behavior: "allow", UpdatedInput: updated}, true
		case hasPermissions:
			entries, ok := suggestedPermissions(permissions, original)
			if !ok {
				return dotHookDecision{}, false
			}
			return dotHookDecision{Behavior: "allow", UpdatedPermissions: entries}, true
		}
		return dotHookDecision{Behavior: "allow"}, true
	}
	return dotHookDecision{}, false
}

// suggestedPermissions accepts updatedPermissions only when every entry deep-equals an entry of the
// permission_suggestions Claude Code itself sent in the original (untruncated) request.
func suggestedPermissions(value any, original map[string]any) ([]any, bool) {
	entries, ok := value.([]any)
	if !ok || len(entries) == 0 {
		return nil, false
	}
	suggestions, _ := original["permission_suggestions"].([]any)
	accepted := make([]any, 0, len(entries))
	for _, entry := range entries {
		matched := false
		for _, suggestion := range suggestions {
			if reflect.DeepEqual(entry, suggestion) {
				accepted = append(accepted, suggestion)
				matched = true
				break
			}
		}
		if !matched {
			return nil, false
		}
	}
	return accepted, true
}

type dotHookQuestion struct {
	labels []string
	multi  bool
}

// answeredToolInput builds updatedInput = the original untruncated tool_input plus answers, only for
// AskUserQuestion and only when every answer key is a question text and every value is a label of
// that question's options (multi-select: distinct labels joined with ", ").
func answeredToolInput(value any, original map[string]any) (map[string]any, bool) {
	if tool, _ := original["tool_name"].(string); tool != "AskUserQuestion" {
		return nil, false
	}
	answers, ok := value.(map[string]any)
	if !ok || len(answers) == 0 {
		return nil, false
	}
	toolInput, ok := original["tool_input"].(map[string]any)
	if !ok {
		return nil, false
	}
	questions, ok := parseDotHookQuestions(toolInput)
	if !ok {
		return nil, false
	}
	for text, raw := range answers {
		question, known := questions[text]
		answer, isString := raw.(string)
		if !known || !isString || answer == "" {
			return nil, false
		}
		if question.multi {
			if !joinsDistinctLabels(answer, question.labels, map[int]bool{}) {
				return nil, false
			}
		} else if !containsString(question.labels, answer) {
			return nil, false
		}
	}
	updated := make(map[string]any, len(toolInput)+1)
	for key, item := range toolInput {
		updated[key] = item
	}
	updated["answers"] = answers
	return updated, true
}

func parseDotHookQuestions(toolInput map[string]any) (map[string]dotHookQuestion, bool) {
	list, ok := toolInput["questions"].([]any)
	if !ok || len(list) == 0 {
		return nil, false
	}
	questions := make(map[string]dotHookQuestion, len(list))
	for _, raw := range list {
		entry, ok := raw.(map[string]any)
		if !ok {
			return nil, false
		}
		text, ok := entry["question"].(string)
		options, hasOptions := entry["options"].([]any)
		if !ok || text == "" || !hasOptions || len(options) == 0 {
			return nil, false
		}
		if _, duplicate := questions[text]; duplicate {
			return nil, false
		}
		question := dotHookQuestion{}
		question.multi, _ = entry["multiSelect"].(bool)
		for _, rawOption := range options {
			option, ok := rawOption.(map[string]any)
			if !ok {
				return nil, false
			}
			label, ok := option["label"].(string)
			if !ok || label == "" {
				return nil, false
			}
			question.labels = append(question.labels, label)
		}
		questions[text] = question
	}
	return questions, true
}

func containsString(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}

// joinsDistinctLabels reports whether value is a sequence of distinct option labels joined by ", ".
// Labels may themselves contain ", ", so it backtracks over the label set instead of splitting.
func joinsDistinctLabels(value string, labels []string, used map[int]bool) bool {
	if !utf8.ValidString(value) {
		return false
	}
	for i, label := range labels {
		if used[i] {
			continue
		}
		if value == label {
			return true
		}
		if strings.HasPrefix(value, label+", ") {
			used[i] = true
			if joinsDistinctLabels(value[len(label)+2:], labels, used) {
				return true
			}
			used[i] = false
		}
	}
	return false
}
