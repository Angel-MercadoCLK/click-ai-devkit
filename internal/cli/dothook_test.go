package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

type dotHookCapture struct {
	method string
	path   string
	header http.Header
	body   []byte
}

type dotHookRig struct {
	t        *testing.T
	srv      *httptest.Server
	file     string
	captured chan dotHookCapture
	release  chan struct{}
}

// newDotHookRig starts a fake Click Dot server and points the relay at it through the discovery-file
// override. Every process-level location (HOME, USERPROFILE, XDG_CONFIG_HOME, APPDATA) is isolated in
// t.TempDir() so nothing can reach the developer's real configuration.
func newDotHookRig(t *testing.T, handler func(w http.ResponseWriter, r *http.Request, body []byte)) *dotHookRig {
	t.Helper()
	isolateDotHookEnv(t)
	rig := &dotHookRig{t: t, captured: make(chan dotHookCapture, 8), release: make(chan struct{})}
	rig.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		rig.captured <- dotHookCapture{method: r.Method, path: r.URL.Path, header: r.Header.Clone(), body: body}
		handler(w, r, body)
	}))
	t.Cleanup(rig.srv.Close)
	t.Cleanup(func() { close(rig.release) })
	rig.file = filepath.Join(t.TempDir(), "agent-hook.json")
	rig.writeDiscovery(rig.srv.URL, "a1b2c3d4e5f60718293a4b5c6d7e8f90a1b2c3d4e5f60718293a4b5c6d7e8f90")
	t.Setenv("CLICK_DOT_AGENT_HOOK_FILE", rig.file)
	shortenDotHookTimeouts(t, 300*time.Millisecond, time.Second, time.Second)
	return rig
}

func isolateDotHookEnv(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))
	t.Setenv("CLICK_DOT_AGENT_HOOK_FILE", "")
}

func shortenDotHookTimeouts(t *testing.T, connect, event, permission time.Duration) {
	t.Helper()
	old := dotHookTimeouts
	dotHookTimeouts = dotHookTimeoutSet{connect: connect, event: event, permission: permission}
	t.Cleanup(func() { dotHookTimeouts = old })
}

func (r *dotHookRig) writeDiscovery(serverURL, token string) {
	r.t.Helper()
	parsed, err := url.Parse(serverURL)
	if err != nil {
		r.t.Fatal(err)
	}
	port, _ := strconv.Atoi(parsed.Port())
	content := `{"version":1,"port":` + strconv.Itoa(port) + `,"token":"` + token + `","pid":4242}`
	if err := os.WriteFile(r.file, []byte(content), 0o600); err != nil {
		r.t.Fatal(err)
	}
}

func (r *dotHookRig) request() dotHookCapture {
	r.t.Helper()
	select {
	case c := <-r.captured:
		return c
	case <-time.After(3 * time.Second):
		r.t.Fatal("the relay never reached the server")
		return dotHookCapture{}
	}
}

func execDotHook(t *testing.T, stdin string, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	root := NewRootCommand()
	var out, errBuf bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errBuf)
	root.SetIn(bytes.NewBufferString(stdin))
	root.SetArgs(append([]string{"dot-hook"}, args...))
	err = root.Execute()
	return out.String(), errBuf.String(), err
}

func respond(status int, body string) func(http.ResponseWriter, *http.Request, []byte) {
	return func(w http.ResponseWriter, _ *http.Request, _ []byte) {
		if body != "" {
			w.Header().Set("Content-Type", "application/json")
		}
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}
}

func assertFailOpen(t *testing.T, stdout, stderr string, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("the relay must exit 0 (nil error), got %v", err)
	}
	if stdout != "" {
		t.Fatalf("a fail-open relay must print nothing to stdout, got %q", stdout)
	}
	if stderr != "" {
		t.Fatalf("a fail-open relay must keep stderr quiet, got %q", stderr)
	}
}

const permissionBash = `{"session_id":"s1","cwd":"C:/work/app","hook_event_name":"PermissionRequest","tool_name":"Bash","tool_input":{"command":"npm test"},"permission_suggestions":[{"type":"addRules","rules":[{"toolName":"Bash","ruleContent":"npm test"}],"behavior":"allow","destination":"localSettings"}]}`

func decodeJSONMap(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("invalid JSON %q: %v", raw, err)
	}
	return out
}

func TestDotHookCommandIsHiddenAndRegistered(t *testing.T) {
	root := NewRootCommand()
	cmd, _, err := root.Find([]string{"dot-hook"})
	if err != nil || cmd == nil || cmd.Name() != "dot-hook" {
		t.Fatalf("dot-hook is not registered: %v", err)
	}
	if !cmd.Hidden {
		t.Fatal("dot-hook must be hidden")
	}
	var help bytes.Buffer
	root.SetOut(&help)
	root.SetArgs([]string{"--help"})
	_ = root.Execute()
	if strings.Contains(help.String(), "dot-hook") {
		t.Fatalf("dot-hook must not appear in help:\n%s", help.String())
	}
}

func TestDotHookRequestCarriesBearerNoOriginAndTheContractBody(t *testing.T) {
	rig := newDotHookRig(t, respond(http.StatusNoContent, ""))
	stdin := `{"session_id":"s1","transcript_path":"C:/secret/transcript.jsonl","hook_event_name":"PostToolUse","tool_name":"Bash","tool_input":{"command":"ls"},"tool_response":{"stdout":"secret output"}}`

	stdout, stderr, err := execDotHook(t, stdin, "PostToolUse")
	assertFailOpen(t, stdout, stderr, err)

	c := rig.request()
	if c.method != http.MethodPost || c.path != "/api/agent/hook" {
		t.Fatalf("request = %s %s", c.method, c.path)
	}
	if got := c.header.Get("Authorization"); got != "Bearer a1b2c3d4e5f60718293a4b5c6d7e8f90a1b2c3d4e5f60718293a4b5c6d7e8f90" {
		t.Fatalf("Authorization = %q", got)
	}
	if _, present := c.header["Origin"]; present {
		t.Fatalf("the request must carry no Origin header: %v", c.header)
	}
	if !strings.HasPrefix(c.header.Get("Content-Type"), "application/json") {
		t.Fatalf("Content-Type = %q", c.header.Get("Content-Type"))
	}
	body := decodeJSONMap(t, c.body)
	if body["version"] != float64(1) || body["agent"] != "claude-code" || body["event"] != "PostToolUse" {
		t.Fatalf("body envelope = %v", body)
	}
	payload, _ := body["payload"].(map[string]any)
	if payload["session_id"] != "s1" || payload["tool_name"] != "Bash" {
		t.Fatalf("payload = %v", payload)
	}
	if _, present := payload["transcript_path"]; present {
		t.Fatal("transcript_path must be dropped")
	}
	if _, present := payload["tool_response"]; present {
		t.Fatal("tool_response must be dropped")
	}
	if bytes.Contains(c.body, []byte("secret")) {
		t.Fatalf("dropped fields leaked into the body: %s", c.body)
	}
}

func TestDotHookNormalizationCapsStringsAtTwoThousandCharacters(t *testing.T) {
	rig := newDotHookRig(t, respond(http.StatusNoContent, ""))
	long := strings.Repeat("a", 5000)
	runes := strings.Repeat("é", 3000)
	stdin := `{"session_id":"s1","hook_event_name":"UserPromptSubmit","prompt":"` + long + `","tool_input":{"command":"` + long + `","nested":["` + runes + `"]}}`

	stdout, stderr, err := execDotHook(t, stdin, "UserPromptSubmit")
	assertFailOpen(t, stdout, stderr, err)

	payload := decodeJSONMap(t, rig.request().body)["payload"].(map[string]any)
	if got := payload["prompt"].(string); len(got) != 2000 {
		t.Fatalf("prompt length = %d, want 2000", len(got))
	}
	input := payload["tool_input"].(map[string]any)
	if got := input["command"].(string); len(got) != 2000 {
		t.Fatalf("tool_input.command length = %d, want 2000", len(got))
	}
	nested := input["nested"].([]any)[0].(string)
	if utf8.RuneCountInString(nested) != 2000 || !utf8.ValidString(nested) {
		t.Fatalf("multi-byte strings must be cut on a character boundary: %d runes", utf8.RuneCountInString(nested))
	}
}

func TestDotHookNormalizationKeepsEditWriteAndMultiEditInputUpToTwoHundredFiftySixKiB(t *testing.T) {
	for _, tool := range []string{"Edit", "MultiEdit", "Write"} {
		t.Run(tool, func(t *testing.T) {
			rig := newDotHookRig(t, respond(http.StatusNoContent, ""))
			big := strings.Repeat("x", 100*1024)
			stdin := `{"session_id":"s1","hook_event_name":"PreToolUse","tool_name":"` + tool + `","tool_input":{"file_path":"C:/a.ts","old_string":"` + big + `","new_string":"` + big + `"}}`

			stdout, stderr, err := execDotHook(t, stdin, "PreToolUse")
			assertFailOpen(t, stdout, stderr, err)

			input := decodeJSONMap(t, rig.request().body)["payload"].(map[string]any)["tool_input"].(map[string]any)
			if len(input["old_string"].(string)) != len(big) || len(input["new_string"].(string)) != len(big) {
				t.Fatalf("%s tool_input must be kept whole under 256 KiB", tool)
			}
		})
	}
}

func TestDotHookNormalizationCapsEditInputOverTwoHundredFiftySixKiB(t *testing.T) {
	rig := newDotHookRig(t, respond(http.StatusNoContent, ""))
	big := strings.Repeat("x", 200*1024)
	stdin := `{"session_id":"s1","hook_event_name":"PreToolUse","tool_name":"Write","tool_input":{"file_path":"C:/a.ts","content":"` + big + `","extra":"` + big + `"}}`

	stdout, stderr, err := execDotHook(t, stdin, "PreToolUse")
	assertFailOpen(t, stdout, stderr, err)

	c := rig.request()
	if len(c.body) > 512*1024 {
		t.Fatalf("a body over 256 KiB of tool_input must fall back to the 2,000 character cap, got %d bytes", len(c.body))
	}
	input := decodeJSONMap(t, c.body)["payload"].(map[string]any)["tool_input"].(map[string]any)
	if len(input["content"].(string)) != 2000 {
		t.Fatalf("content length = %d, want 2000", len(input["content"].(string)))
	}
}

func TestDotHookNormalizationNeverTruncatesAskUserQuestionInput(t *testing.T) {
	rig := newDotHookRig(t, respond(http.StatusNoContent, ""))
	longQuestion := strings.Repeat("q", 5000)
	longLabel := strings.Repeat("l", 3000)
	stdin := `{"session_id":"s1","hook_event_name":"PermissionRequest","tool_name":"AskUserQuestion","tool_input":{"questions":[{"question":"` + longQuestion + `","header":"h","options":[{"label":"` + longLabel + `","description":"d"}],"multiSelect":false}]}}`

	stdout, stderr, err := execDotHook(t, stdin, "PermissionRequest")
	assertFailOpen(t, stdout, stderr, err)

	questions := decodeJSONMap(t, rig.request().body)["payload"].(map[string]any)["tool_input"].(map[string]any)["questions"].([]any)
	q := questions[0].(map[string]any)
	if len(q["question"].(string)) != 5000 || len(q["options"].([]any)[0].(map[string]any)["label"].(string)) != 3000 {
		t.Fatal("AskUserQuestion tool_input must never be truncated")
	}
}

func TestDotHookNormalizationNeverTruncatesPermissionSuggestions(t *testing.T) {
	rig := newDotHookRig(t, respond(http.StatusNoContent, ""))
	longRule := strings.Repeat("r", 3000)
	stdin := `{"session_id":"s1","hook_event_name":"PermissionRequest","tool_name":"Bash","tool_input":{"command":"ls"},"permission_suggestions":[{"type":"addRules","rules":[{"toolName":"Bash","ruleContent":"` + longRule + `"}],"behavior":"allow","destination":"localSettings"}]}`

	stdout, stderr, err := execDotHook(t, stdin, "PermissionRequest")
	assertFailOpen(t, stdout, stderr, err)

	suggestion := decodeJSONMap(t, rig.request().body)["payload"].(map[string]any)["permission_suggestions"].([]any)[0].(map[string]any)
	if len(suggestion["rules"].([]any)[0].(map[string]any)["ruleContent"].(string)) != 3000 {
		t.Fatal("permission_suggestions must never be truncated, or Siempre could never match the original")
	}
}

func TestDotHookTimeoutsMatchTheWireContract(t *testing.T) {
	if dotHookTimeouts.connect != 300*time.Millisecond || dotHookTimeouts.event != 2*time.Second || dotHookTimeouts.permission != 110*time.Second {
		t.Fatalf("default timeouts = %+v, want connect 300ms, event 2s, permission 110s", dotHookTimeouts)
	}
	if dotHookTimeouts.totalFor("PermissionRequest") != 110*time.Second || dotHookTimeouts.totalFor("Stop") != 2*time.Second {
		t.Fatal("PermissionRequest waits 110 s; every other event 2 s")
	}
}

func TestDotHookDiscoveryFallsBackToTheUserConfigDirectory(t *testing.T) {
	isolateDotHookEnv(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = io.WriteString(w, `{"behavior":"allow"}`)
	}))
	t.Cleanup(srv.Close)
	configDir, err := os.UserConfigDir()
	if err != nil {
		t.Skip("no user config directory on this platform")
	}
	dir := filepath.Join(configDir, "Click Dot")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	parsed, _ := url.Parse(srv.URL)
	port, _ := strconv.Atoi(parsed.Port())
	if err := os.WriteFile(filepath.Join(dir, "agent-hook.json"), []byte(`{"version":1,"port":`+strconv.Itoa(port)+`,"token":"tok","pid":1}`), 0o600); err != nil {
		t.Fatal(err)
	}

	stdout, _, err := execDotHook(t, permissionBash, "PermissionRequest")
	if err != nil || !strings.Contains(stdout, `"behavior":"allow"`) {
		t.Fatalf("the default discovery file was not used: stdout=%q err=%v", stdout, err)
	}
}

func TestDotHookFailsOpenWhenDotIsUnavailableOrMisbehaves(t *testing.T) {
	type scenario struct {
		name    string
		setup   func(t *testing.T) string // returns stdin
		event   string
		handler func(http.ResponseWriter, *http.Request, []byte)
	}
	stdinOK := func(*testing.T) string { return permissionBash }
	cases := []scenario{
		{name: "204 means no decision", event: "PermissionRequest", setup: stdinOK, handler: respond(http.StatusNoContent, "")},
		{name: "500", event: "PermissionRequest", setup: stdinOK, handler: respond(http.StatusInternalServerError, `{"behavior":"allow"}`)},
		{name: "401", event: "PermissionRequest", setup: stdinOK, handler: respond(http.StatusUnauthorized, `{"behavior":"allow"}`)},
		{name: "413", event: "PermissionRequest", setup: stdinOK, handler: respond(http.StatusRequestEntityTooLarge, "")},
		{name: "202 is not a decision", event: "PermissionRequest", setup: stdinOK, handler: respond(http.StatusAccepted, `{"behavior":"allow"}`)},
		{name: "malformed JSON body", event: "PermissionRequest", setup: stdinOK, handler: respond(http.StatusOK, `{"behavior":`)},
		{name: "empty 200 body", event: "PermissionRequest", setup: stdinOK, handler: respond(http.StatusOK, ``)},
		{name: "body is not an object", event: "PermissionRequest", setup: stdinOK, handler: respond(http.StatusOK, `"allow"`)},
		{name: "unknown behavior", event: "PermissionRequest", setup: stdinOK, handler: respond(http.StatusOK, `{"behavior":"ask"}`)},
		{name: "missing behavior", event: "PermissionRequest", setup: stdinOK, handler: respond(http.StatusOK, `{}`)},
		{name: "behavior of the wrong type", event: "PermissionRequest", setup: stdinOK, handler: respond(http.StatusOK, `{"behavior":true}`)},
		{name: "redirect is never followed", event: "PermissionRequest", setup: stdinOK, handler: func(w http.ResponseWriter, r *http.Request, _ []byte) {
			http.Redirect(w, r, "/elsewhere", http.StatusFound)
		}},
		{name: "deny carrying answers is inconsistent", event: "PermissionRequest", setup: stdinOK, handler: respond(http.StatusOK, `{"behavior":"deny","answers":{"q":"a"}}`)},
		{name: "allow with empty updatedPermissions", event: "PermissionRequest", setup: stdinOK, handler: respond(http.StatusOK, `{"behavior":"allow","updatedPermissions":[]}`)},
		{name: "allow with non-array updatedPermissions", event: "PermissionRequest", setup: stdinOK, handler: respond(http.StatusOK, `{"behavior":"allow","updatedPermissions":{"a":1}}`)},
		{name: "invalid stdin JSON", event: "PermissionRequest", setup: func(*testing.T) string { return `{"hook_event_name":` }, handler: respond(http.StatusOK, `{"behavior":"allow"}`)},
		{name: "empty stdin", event: "PermissionRequest", setup: func(*testing.T) string { return `` }, handler: respond(http.StatusOK, `{"behavior":"allow"}`)},
		{name: "stdin is not an object", event: "PermissionRequest", setup: func(*testing.T) string { return `[1,2]` }, handler: respond(http.StatusOK, `{"behavior":"allow"}`)},
		{name: "a decision for a non-permission event is ignored", event: "Stop", setup: func(*testing.T) string {
			return `{"session_id":"s1","hook_event_name":"Stop"}`
		}, handler: respond(http.StatusOK, `{"behavior":"allow"}`)},
		{name: "a deny for PostToolUse is ignored", event: "PostToolUse", setup: func(*testing.T) string {
			return `{"session_id":"s1","hook_event_name":"PostToolUse","tool_name":"Bash"}`
		}, handler: respond(http.StatusOK, `{"behavior":"deny"}`)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			newDotHookRig(t, tc.handler)
			stdout, stderr, err := execDotHook(t, tc.setup(t), tc.event)
			assertFailOpen(t, stdout, stderr, err)
		})
	}
}

func TestDotHookFailsOpenWithoutAUsableDiscoveryFile(t *testing.T) {
	cases := map[string]func(t *testing.T) string{
		"missing file": func(t *testing.T) string { return filepath.Join(t.TempDir(), "absent.json") },
		"empty file":   func(t *testing.T) string { return writeDotHookDiscovery(t, ``) },
		"invalid JSON": func(t *testing.T) string { return writeDotHookDiscovery(t, `{"version":1,`) },
		"wrong version": func(t *testing.T) string {
			return writeDotHookDiscovery(t, `{"version":2,"port":47321,"token":"t","pid":1}`)
		},
		"missing token": func(t *testing.T) string { return writeDotHookDiscovery(t, `{"version":1,"port":47321,"pid":1}`) },
		"empty token": func(t *testing.T) string {
			return writeDotHookDiscovery(t, `{"version":1,"port":47321,"token":"","pid":1}`)
		},
		"port zero": func(t *testing.T) string {
			return writeDotHookDiscovery(t, `{"version":1,"port":0,"token":"t","pid":1}`)
		},
		"port out of range": func(t *testing.T) string {
			return writeDotHookDiscovery(t, `{"version":1,"port":70000,"token":"t","pid":1}`)
		},
		"port as a string": func(t *testing.T) string {
			return writeDotHookDiscovery(t, `{"version":1,"port":"47321","token":"t","pid":1}`)
		},
		"directory not file": func(t *testing.T) string { return t.TempDir() },
	}
	for name, path := range cases {
		t.Run(name, func(t *testing.T) {
			isolateDotHookEnv(t)
			t.Setenv("CLICK_DOT_AGENT_HOOK_FILE", path(t))
			stdout, stderr, err := execDotHook(t, permissionBash, "PermissionRequest")
			assertFailOpen(t, stdout, stderr, err)
		})
	}
}

func writeDotHookDiscovery(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "agent-hook.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestDotHookFailsOpenWhenTheConnectionIsRefused(t *testing.T) {
	rig := newDotHookRig(t, respond(http.StatusNoContent, ""))
	rig.srv.Close()
	started := time.Now()
	stdout, stderr, err := execDotHook(t, permissionBash, "PermissionRequest")
	assertFailOpen(t, stdout, stderr, err)
	if time.Since(started) > 2*time.Second {
		t.Fatal("a refused connection must fail fast")
	}
}

func TestDotHookFailsOpenWhenDotDoesNotAnswerInTime(t *testing.T) {
	for _, event := range []string{"Stop", "PermissionRequest"} {
		t.Run(event, func(t *testing.T) {
			rig := newDotHookRig(t, func(w http.ResponseWriter, r *http.Request, _ []byte) {
				select {
				case <-time.After(5 * time.Second):
				case <-r.Context().Done():
				}
				_, _ = io.WriteString(w, `{"behavior":"allow"}`)
			})
			shortenDotHookTimeouts(t, 300*time.Millisecond, 150*time.Millisecond, 150*time.Millisecond)
			_ = rig
			stdin := permissionBash
			if event == "Stop" {
				stdin = `{"session_id":"s1","hook_event_name":"Stop"}`
			}
			started := time.Now()
			stdout, stderr, err := execDotHook(t, stdin, event)
			assertFailOpen(t, stdout, stderr, err)
			if elapsed := time.Since(started); elapsed > 2*time.Second {
				t.Fatalf("the relay waited %v past its %s timeout", elapsed, event)
			}
		})
	}
}

func TestDotHookRecoversFromAPanicAndFailsOpen(t *testing.T) {
	newDotHookRig(t, respond(http.StatusNoContent, ""))
	old := dotHookPost
	dotHookPost = func(int, string, []byte, time.Duration) (dotHookReply, bool) { panic("boom") }
	t.Cleanup(func() { dotHookPost = old })
	stdout, stderr, err := execDotHook(t, permissionBash, "PermissionRequest")
	assertFailOpen(t, stdout, stderr, err)
}

func TestDotHookToleratesUnexpectedArguments(t *testing.T) {
	newDotHookRig(t, respond(http.StatusNoContent, ""))
	for _, args := range [][]string{nil, {"--bogus"}, {"PermissionRequest", "extra", "--flag=1"}} {
		stdout, stderr, err := execDotHook(t, permissionBash, args...)
		assertFailOpen(t, stdout, stderr, err)
	}
}

func TestDotHookMapsDenyAllowAndAlwaysExactly(t *testing.T) {
	suggestion := `{"type":"addRules","rules":[{"toolName":"Bash","ruleContent":"npm test"}],"behavior":"allow","destination":"localSettings"}`
	cases := []struct {
		name     string
		response string
		want     string
	}{
		{"deny", `{"behavior":"deny"}`, `{"hookSpecificOutput":{"hookEventName":"PermissionRequest","decision":{"behavior":"deny","message":"Denied from Click Dot"}}}`},
		{"allow", `{"behavior":"allow"}`, `{"hookSpecificOutput":{"hookEventName":"PermissionRequest","decision":{"behavior":"allow"}}}`},
		{"always applies a suggestion Claude Code itself sent", `{"behavior":"allow","updatedPermissions":[` + suggestion + `]}`, `{"hookSpecificOutput":{"hookEventName":"PermissionRequest","decision":{"behavior":"allow","updatedPermissions":[` + suggestion + `]}}}`},
		{"deny ignores nothing it was not given", `{"behavior":"deny","updatedPermissions":null}`, `{"hookSpecificOutput":{"hookEventName":"PermissionRequest","decision":{"behavior":"deny","message":"Denied from Click Dot"}}}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			newDotHookRig(t, respond(http.StatusOK, tc.response))
			stdout, stderr, err := execDotHook(t, permissionBash, "PermissionRequest")
			if err != nil || stderr != "" {
				t.Fatalf("err=%v stderr=%q", err, stderr)
			}
			if !jsonEqual(t, stdout, tc.want) {
				t.Fatalf("stdout = %s\nwant     %s", stdout, tc.want)
			}
			if !strings.Contains(tc.name, "always") && strings.TrimSpace(stdout) != tc.want {
				t.Fatalf("stdout bytes = %s\nwant         %s", stdout, tc.want)
			}
		})
	}
}

func jsonEqual(t *testing.T, got, want string) bool {
	t.Helper()
	var gotTree, wantTree any
	if err := json.Unmarshal([]byte(got), &gotTree); err != nil {
		t.Fatalf("stdout is not JSON: %q", got)
	}
	if err := json.Unmarshal([]byte(want), &wantTree); err != nil {
		t.Fatalf("bad expectation: %q", want)
	}
	return reflect.DeepEqual(gotTree, wantTree)
}

func TestDotHookRejectsPermissionSuggestionsClaudeCodeNeverSent(t *testing.T) {
	good := `{"type":"addRules","rules":[{"toolName":"Bash","ruleContent":"npm test"}],"behavior":"allow","destination":"localSettings"}`
	cases := map[string]string{
		"injected rule":                  `{"type":"addRules","rules":[{"toolName":"Bash","ruleContent":"rm -rf /"}],"behavior":"allow","destination":"localSettings"}`,
		"widened destination":            `{"type":"addRules","rules":[{"toolName":"Bash","ruleContent":"npm test"}],"behavior":"allow","destination":"userSettings"}`,
		"extra field":                    `{"type":"addRules","rules":[{"toolName":"Bash","ruleContent":"npm test"}],"behavior":"allow","destination":"localSettings","x":1}`,
		"entry that is not an object":    `"addRules"`,
		"one good entry plus one forged": good + `,{"type":"setMode","mode":"bypassPermissions","destination":"session"}`,
	}
	for name, entries := range cases {
		t.Run(name, func(t *testing.T) {
			newDotHookRig(t, respond(http.StatusOK, `{"behavior":"allow","updatedPermissions":[`+entries+`]}`))
			stdout, stderr, err := execDotHook(t, permissionBash, "PermissionRequest")
			assertFailOpen(t, stdout, stderr, err)
		})
	}

	t.Run("no suggestions in the original request", func(t *testing.T) {
		newDotHookRig(t, respond(http.StatusOK, `{"behavior":"allow","updatedPermissions":[`+good+`]}`))
		stdin := `{"session_id":"s1","hook_event_name":"PermissionRequest","tool_name":"Bash","tool_input":{"command":"npm test"}}`
		stdout, stderr, err := execDotHook(t, stdin, "PermissionRequest")
		assertFailOpen(t, stdout, stderr, err)
	})

	t.Run("a suggestion that was truncated on the wire never matches", func(t *testing.T) {
		long := strings.Repeat("c", 3000)
		original := `{"type":"addRules","rules":[{"toolName":"Bash","ruleContent":"` + long + `"}],"behavior":"allow","destination":"localSettings"}`
		truncated := `{"type":"addRules","rules":[{"toolName":"Bash","ruleContent":"` + long[:2000] + `"}],"behavior":"allow","destination":"localSettings"}`
		newDotHookRig(t, respond(http.StatusOK, `{"behavior":"allow","updatedPermissions":[`+truncated+`]}`))
		stdin := `{"session_id":"s1","hook_event_name":"PermissionRequest","tool_name":"Bash","tool_input":{"command":"x"},"permission_suggestions":[` + original + `]}`
		stdout, stderr, err := execDotHook(t, stdin, "PermissionRequest")
		assertFailOpen(t, stdout, stderr, err)
	})
}

const askSingleAndMulti = `{"session_id":"s1","cwd":"C:/work","hook_event_name":"PermissionRequest","tool_name":"AskUserQuestion","tool_input":{"questions":[` +
	`{"question":"Which database?","header":"DB","options":[{"label":"Postgres","description":"relational"},{"label":"SQLite","description":"embedded"}],"multiSelect":false},` +
	`{"question":"Which features?","header":"Feat","options":[{"label":"Auth","description":"a"},{"label":"Billing, invoices","description":"b"},{"label":"Search","description":"c"}],"multiSelect":true}` +
	`]}}`

func TestDotHookAnswersBecomeAnAllowWithTheOriginalUntruncatedInputPlusAnswers(t *testing.T) {
	longQuestion := strings.Repeat("q", 4000)
	stdin := `{"session_id":"s1","hook_event_name":"PermissionRequest","tool_name":"AskUserQuestion","tool_input":{"questions":[` +
		`{"question":"` + longQuestion + `","header":"H","options":[{"label":"Yes","description":"d"},{"label":"No","description":"d"}],"multiSelect":false}]}}`
	newDotHookRig(t, respond(http.StatusOK, `{"behavior":"allow","answers":{"`+longQuestion+`":"Yes"}}`))

	stdout, stderr, err := execDotHook(t, stdin, "PermissionRequest")
	if err != nil || stderr != "" {
		t.Fatalf("err=%v stderr=%q", err, stderr)
	}
	want := `{"hookSpecificOutput":{"hookEventName":"PermissionRequest","decision":{"behavior":"allow","updatedInput":{"answers":{"` + longQuestion + `":"Yes"},"questions":[` +
		`{"header":"H","multiSelect":false,"options":[{"description":"d","label":"Yes"},{"description":"d","label":"No"}],"question":"` + longQuestion + `"}]}}}}`
	var gotTree, wantTree any
	if err := json.Unmarshal([]byte(stdout), &gotTree); err != nil {
		t.Fatalf("stdout is not JSON: %q", stdout)
	}
	_ = json.Unmarshal([]byte(want), &wantTree)
	if !reflect.DeepEqual(gotTree, wantTree) {
		t.Fatalf("stdout = %s\nwant      %s", stdout, want)
	}
	decision := gotTree.(map[string]any)["hookSpecificOutput"].(map[string]any)["decision"].(map[string]any)
	if _, hasMessage := decision["message"]; hasMessage {
		t.Fatal("an allow carries no message")
	}
}

func TestDotHookMapsSingleAndMultiSelectAnswersExactly(t *testing.T) {
	newDotHookRig(t, respond(http.StatusOK, `{"behavior":"allow","answers":{"Which database?":"SQLite","Which features?":"Auth, Billing, invoices"}}`))
	stdout, stderr, err := execDotHook(t, askSingleAndMulti, "PermissionRequest")
	if err != nil || stderr != "" {
		t.Fatalf("err=%v stderr=%q", err, stderr)
	}
	root := decodeJSONMap(t, []byte(stdout))
	decision := root["hookSpecificOutput"].(map[string]any)["decision"].(map[string]any)
	if decision["behavior"] != "allow" {
		t.Fatalf("decision = %v", decision)
	}
	updated := decision["updatedInput"].(map[string]any)
	answers := updated["answers"].(map[string]any)
	if answers["Which database?"] != "SQLite" || answers["Which features?"] != "Auth, Billing, invoices" || len(answers) != 2 {
		t.Fatalf("answers = %v", answers)
	}
	if len(updated["questions"].([]any)) != 2 {
		t.Fatalf("updatedInput must keep the original questions: %v", updated)
	}
}

func TestDotHookDoesNotAnswerWhenAnswersDoNotMatchTheQuestions(t *testing.T) {
	cases := map[string]struct {
		stdin    string
		response string
	}{
		"unknown question text":                          {askSingleAndMulti, `{"behavior":"allow","answers":{"Which cache?":"Postgres"}}`},
		"question text differs by case":                  {askSingleAndMulti, `{"behavior":"allow","answers":{"which database?":"Postgres"}}`},
		"unknown option label":                           {askSingleAndMulti, `{"behavior":"allow","answers":{"Which database?":"MySQL"}}`},
		"label differs by case":                          {askSingleAndMulti, `{"behavior":"allow","answers":{"Which database?":"postgres"}}`},
		"label with surrounding space":                   {askSingleAndMulti, `{"behavior":"allow","answers":{"Which database?":" Postgres"}}`},
		"single select given two labels":                 {askSingleAndMulti, `{"behavior":"allow","answers":{"Which database?":"Postgres, SQLite"}}`},
		"multi select with a bad part":                   {askSingleAndMulti, `{"behavior":"allow","answers":{"Which features?":"Auth, Chat"}}`},
		"multi select joined without space":              {askSingleAndMulti, `{"behavior":"allow","answers":{"Which features?":"Auth,Search"}}`},
		"multi select with a repeated label":             {askSingleAndMulti, `{"behavior":"allow","answers":{"Which features?":"Auth, Auth"}}`},
		"multi select with a trailing comma":             {askSingleAndMulti, `{"behavior":"allow","answers":{"Which features?":"Auth, "}}`},
		"empty answer":                                   {askSingleAndMulti, `{"behavior":"allow","answers":{"Which database?":""}}`},
		"non-string answer":                              {askSingleAndMulti, `{"behavior":"allow","answers":{"Which database?":["Postgres"]}}`},
		"empty answers object":                           {askSingleAndMulti, `{"behavior":"allow","answers":{}}`},
		"answers is not an object":                       {askSingleAndMulti, `{"behavior":"allow","answers":["Postgres"]}`},
		"answers together with updatedPermissions":       {askSingleAndMulti, `{"behavior":"allow","answers":{"Which database?":"Postgres"},"updatedPermissions":[{"type":"addRules"}]}`},
		"answers for a tool that is not AskUserQuestion": {permissionBash, `{"behavior":"allow","answers":{"Which database?":"Postgres"}}`},
		"AskUserQuestion without questions":              {`{"session_id":"s1","hook_event_name":"PermissionRequest","tool_name":"AskUserQuestion","tool_input":{}}`, `{"behavior":"allow","answers":{"x":"y"}}`},
		"question without options":                       {`{"session_id":"s1","hook_event_name":"PermissionRequest","tool_name":"AskUserQuestion","tool_input":{"questions":[{"question":"Q?","multiSelect":false}]}}`, `{"behavior":"allow","answers":{"Q?":"y"}}`},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			newDotHookRig(t, respond(http.StatusOK, tc.response))
			stdout, stderr, err := execDotHook(t, tc.stdin, "PermissionRequest")
			assertFailOpen(t, stdout, stderr, err)
		})
	}
}

func TestDotHookPrintsOutputOnlyForPermissionRequest(t *testing.T) {
	for _, event := range []string{"SessionStart", "SessionEnd", "UserPromptSubmit", "PreToolUse", "PostToolUse", "PostToolUseFailure", "Notification", "Stop"} {
		t.Run(event, func(t *testing.T) {
			newDotHookRig(t, respond(http.StatusOK, `{"behavior":"allow"}`))
			stdin := `{"session_id":"s1","hook_event_name":"` + event + `","tool_name":"Bash","tool_input":{"command":"ls"}}`
			stdout, stderr, err := execDotHook(t, stdin, event)
			assertFailOpen(t, stdout, stderr, err)
		})
	}
}

func TestDotHookUsesTheEventNameItWasRegisteredFor(t *testing.T) {
	rig := newDotHookRig(t, respond(http.StatusNoContent, ""))
	stdout, stderr, err := execDotHook(t, `{"session_id":"s1"}`, "SessionStart")
	assertFailOpen(t, stdout, stderr, err)
	if got := decodeJSONMap(t, rig.request().body)["event"]; got != "SessionStart" {
		t.Fatalf("event = %v, want SessionStart", got)
	}
}
