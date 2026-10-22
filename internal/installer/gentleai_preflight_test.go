package installer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPreflightGentleAIRegistrations_RejectsUnprovenSameNameWithoutMutation(t *testing.T) {
	codexHome := t.TempDir()
	configPath := filepath.Join(codexHome, "config.toml")
	original := "[mcp_servers.engram]\ncommand = \"custom-memory\"\nargs = [\"mcp\"]\n"
	if err := os.WriteFile(configPath, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	err := PreflightGentleAIRegistrations(Config{CodexHome: codexHome}, TargetSelection{Codex: true})
	if err == nil || !strings.Contains(err.Error(), "engram") {
		t.Fatalf("PreflightGentleAIRegistrations() error = %v, want actionable conflict for engram", err)
	}
	got, readErr := os.ReadFile(configPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != original {
		t.Fatalf("preflight changed config.toml: got %q, want original %q", got, original)
	}
}

func TestPreflightGentleAIRegistrations_SkipsUnselectedTargets(t *testing.T) {
	codexHome := t.TempDir()
	claudeHome := t.TempDir()
	if err := os.WriteFile(filepath.Join(codexHome, "config.toml"), []byte("[mcp_servers.engram]\ncommand = \"custom-memory\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := PreflightGentleAIRegistrations(Config{ClaudeHome: claudeHome, CodexHome: codexHome, OpenClawHome: t.TempDir()}, TargetSelection{Claude: true}); err != nil {
		t.Fatalf("preflight rejected unselected Codex config: %v", err)
	}
}

func TestPreflightGentleAIRegistrations_RejectsSameNameEntriesWithExtraState(t *testing.T) {
	cases := []struct {
		name string
		path string
		data string
		pick TargetSelection
	}{
		{"Claude custom env", "claude", `{"mcpServers":{"engram":{"command":"engram","args":["mcp","--tools=agent"],"env":{"TOKEN":"user-owned"}}}}`, TargetSelection{Claude: true}},
		{"Claude custom env with stdio type", "claude", `{"mcpServers":{"engram":{"type":"stdio","command":"engram","args":["mcp","--tools=agent"],"env":{"TOKEN":"user-owned"}}}}`, TargetSelection{Claude: true}},
		{"Claude non-stdio type", "claude", `{"mcpServers":{"engram":{"type":"http","command":"engram","args":["mcp","--tools=agent"]}}}`, TargetSelection{Claude: true}},
		{"Claude null env", "claude", `{"mcpServers":{"engram":{"command":"engram","args":["mcp","--tools=agent"],"env":null}}}`, TargetSelection{Claude: true}},
		{"Codex custom args", "codex", "[mcp_servers.context7]\nurl = \"https://mcp.context7.com/mcp\"\nheaders = { authorization = \"user-owned\" }\n", TargetSelection{Codex: true}},
		{"OpenClaw custom args", "openclaw", `{"mcp":{"servers":{"context7":{"type":"remote","url":"https://mcp.context7.com/mcp","headers":{"Authorization":"user-owned"}}}}}`, TargetSelection{OpenClaw: true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			claudeHome, codexHome, openClawHome := t.TempDir(), t.TempDir(), t.TempDir()
			cfg := Config{ClaudeHome: claudeHome, CodexHome: codexHome, OpenClawHome: openClawHome}
			var path string
			switch tc.path {
			case "claude":
				path = cfg.Context7ConfigPath()
			case "codex":
				path = cfg.CodexConfigPath()
			case "openclaw":
				path = cfg.OpenClawMCPConfigPath()
			}
			if err := os.WriteFile(path, []byte(tc.data), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := PreflightGentleAIRegistrations(cfg, tc.pick); err == nil {
				t.Fatal("preflight accepted same-name entry with user-owned extra state")
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tc.data {
				t.Fatalf("preflight changed user-owned registration: got %q, want %q", got, tc.data)
			}
		})
	}
}

func TestPreflightGentleAIRegistrations_AllowsExactExistingManagedShapes(t *testing.T) {
	claudeHome, codexHome := t.TempDir(), t.TempDir()
	claude := Config{ClaudeHome: claudeHome}
	if err := os.WriteFile(claude.Context7ConfigPath(), marshalMCPConfig(t, map[string]any{"mcpServers": map[string]any{"engram": engramEntry("engram"), "context7": legacyContext7HTTPEntry()}}), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(claude.Context7StatePath()), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(claude.Context7StatePath(), []byte(`{"installed_by_click":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	codexConfig := "[mcp_servers.engram]\ncommand = \"engram\"\nargs = [\"mcp\", \"--tools=agent\"]\n\n[mcp_servers.context7]\nurl = \"https://mcp.context7.com/mcp\"\n"
	if err := os.WriteFile((Config{CodexHome: codexHome}).CodexConfigPath(), []byte(codexConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := PreflightGentleAIRegistrations(Config{ClaudeHome: claudeHome, CodexHome: codexHome}, TargetSelection{Claude: true, Codex: true}); err != nil {
		t.Fatalf("preflight rejected exact upstream-compatible existing entries: %v", err)
	}
}

// Claude Code stores a server added with `claude mcp add` (as engram's own setup does) with an
// explicit stdio type and an empty env. That is the same registration gentle-ai merges into.
func TestPreflightGentleAIRegistrations_AllowsClaudeCodeNormalizedEngramShape(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "engram", "bin", "engram.exe")
	defer setEngramLookPathForTest(t, bin, nil)()
	cases := []struct {
		name string
		data string
	}{
		{"stdio type and empty env", `{"mcpServers":{"engram":{"type":"stdio","command":` + jsonString(t, bin) + `,"args":["mcp","--tools=agent"],"env":{}}}}`},
		{"stdio type only", `{"mcpServers":{"engram":{"type":"stdio","command":"engram","args":["mcp","--tools=agent"]}}}`},
		{"empty env only", `{"mcpServers":{"engram":{"command":"engram","args":["mcp","--tools=agent"],"env":{}}}}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := Config{ClaudeHome: t.TempDir()}
			path := cfg.Context7ConfigPath()
			if err := os.WriteFile(path, []byte(tc.data), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := PreflightGentleAIRegistrations(cfg, TargetSelection{Claude: true}); err != nil {
				t.Fatalf("preflight rejected Claude Code's normalized engram registration: %v", err)
			}
			found, err := HasGentleAIEngramMCP(cfg)
			if err != nil || !found {
				t.Fatalf("HasGentleAIEngramMCP() = %v, %v; want true, nil", found, err)
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tc.data {
				t.Fatalf("preflight changed the registration: got %q, want %q", got, tc.data)
			}
		})
	}
}

func jsonString(t *testing.T, s string) string {
	t.Helper()
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestPreflightGentleAIRegistrations_AllowsPinnedContext7Shapes(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
		data any
		pick TargetSelection
	}{
		{"Claude local npx", Config{ClaudeHome: t.TempDir()}, map[string]any{"mcpServers": map[string]any{"context7": context7LocalEntry()}}, TargetSelection{Claude: true}},
		{"OpenClaw local npx", Config{OpenClawHome: t.TempDir()}, map[string]any{"mcp": map[string]any{"servers": map[string]any{"context7": context7LocalEntry()}}}, TargetSelection{OpenClaw: true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := tc.cfg.Context7ConfigPath()
			if tc.pick.OpenClaw {
				path = tc.cfg.OpenClawMCPConfigPath()
			}
			if err := os.WriteFile(path, marshalMCPConfig(t, tc.data), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := PreflightGentleAIRegistrations(tc.cfg, tc.pick); err != nil {
				t.Fatalf("preflight rejected pinned v4.0.0 shape: %v", err)
			}
		})
	}
}

func TestPreflightGentleAIRegistrations_AllowsOnlyStateOwnedLegacyHTTPMigration(t *testing.T) {
	for _, tc := range []struct {
		name      string
		owned     bool
		entry     map[string]any
		wantError bool
	}{
		{"owned legacy", true, legacyContext7HTTPEntry(), false},
		{"owned post-upsert hybrid", true, map[string]any{"type": "http", "url": context7MCPURL, "command": "npx", "args": context7LocalEntry()["args"]}, false},
		{"independent legacy", false, legacyContext7HTTPEntry(), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := Config{ClaudeHome: t.TempDir()}
			if err := os.MkdirAll(filepath.Dir(cfg.Context7StatePath()), 0o700); err != nil {
				t.Fatal(err)
			}
			state, _ := json.Marshal(context7State{InstalledByClick: tc.owned})
			if err := os.WriteFile(cfg.Context7StatePath(), state, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(cfg.Context7ConfigPath(), marshalMCPConfig(t, map[string]any{"mcpServers": map[string]any{"context7": tc.entry}}), 0o600); err != nil {
				t.Fatal(err)
			}
			err := PreflightGentleAIRegistrations(cfg, TargetSelection{Claude: true})
			if (err != nil) != tc.wantError {
				t.Fatalf("preflight error = %v, wantError=%t", err, tc.wantError)
			}
		})
	}
}

func TestPreflightGentleAIRegistrations_ClassifiesOpenClawLegacyAndNestedMaps(t *testing.T) {
	tests := []struct {
		name string
		root map[string]any
	}{
		{"malformed legacy map", map[string]any{"mcpServers": "not-a-map"}},
		{"malformed mcp", map[string]any{"mcp": []any{}}},
		{"malformed nested servers", map[string]any{"mcp": map[string]any{"servers": true}}},
		{"conflicting same-name migration", map[string]any{
			"mcpServers": map[string]any{"context7": legacyContext7HTTPEntry()},
			"mcp":        map[string]any{"servers": map[string]any{"context7": map[string]any{"command": "custom"}}},
		}},
		{"unproven legacy registration", map[string]any{"mcpServers": map[string]any{"engram": map[string]any{"command": "custom"}}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := Config{OpenClawHome: t.TempDir()}
			path := cfg.OpenClawMCPConfigPath()
			original := marshalMCPConfig(t, tc.root)
			if err := os.WriteFile(path, original, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := PreflightGentleAIRegistrations(cfg, TargetSelection{OpenClaw: true}); err == nil {
				t.Fatal("preflight accepted unclassifiable or destructive OpenClaw migration state")
			}
			got, err := os.ReadFile(path)
			if err != nil || string(got) != string(original) {
				t.Fatalf("preflight changed OpenClaw config: error=%v got=%q want=%q", err, got, original)
			}
		})
	}
}

func TestPreflightGentleAIRegistrations_RejectsMissingSelectedHome(t *testing.T) {
	if err := PreflightGentleAIRegistrations(Config{}, TargetSelection{Claude: true}); err == nil {
		t.Fatal("preflight accepted selected target with no isolated home")
	}
}

func TestValidateGentleAIDestinationRoots_RejectsUnsupportedClickOverrides(t *testing.T) {
	home := t.TempDir()
	cfg := Config{ClaudeHome: filepath.Join(home, ".claude"), CodexHome: filepath.Join(home, ".codex"), OpenClawHome: filepath.Join(home, ".openclaw")}
	if err := validateGentleAIDestinationRoots(cfg, TargetSelection{Claude: true, Codex: true, OpenClaw: true}, home); err != nil {
		t.Fatalf("standard roots rejected: %v", err)
	}
	cfg.OpenClawHome = t.TempDir()
	if err := validateGentleAIDestinationRoots(cfg, TargetSelection{Claude: true, OpenClaw: true}, home); err == nil {
		t.Fatal("root preflight accepted unsupported isolated OpenClaw override")
	}
	if err := validateGentleAIDestinationRoots(cfg, TargetSelection{Claude: true}, home); err != nil {
		t.Fatalf("unselected override incorrectly blocked Claude-only install: %v", err)
	}
}

func marshalMCPConfig(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func engramEntry(command string) map[string]any {
	return map[string]any{"command": command, "args": []string{"mcp", "--tools=agent"}}
}

func context7LocalEntry() map[string]any {
	return map[string]any{"command": "npx", "args": []string{"-y", "--package=@upstash/context7-mcp@2.2.5", "--", "context7-mcp"}}
}

func legacyContext7HTTPEntry() map[string]any {
	return map[string]any{"type": "http", "url": context7MCPURL}
}
