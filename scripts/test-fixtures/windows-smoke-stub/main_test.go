package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/installer"
)

func TestMergeClaudeMCPProducesPinnedPreflightCompatibleConfig(t *testing.T) {
	home := t.TempDir()
	configPath := filepath.Join(home, ".claude.json")
	original := []byte(`{"mcpServers":{"foreign":{"command":"foreign-command","args":["keep"]}},"other":"preserved"}`)
	if err := os.WriteFile(configPath, original, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := mergeClaudeMCP(configPath); err != nil {
		t.Fatalf("merge Claude fixture MCP registrations: %v", err)
	}
	if err := installer.PreflightGentleAIRegistrations(
		installer.Config{ClaudeHome: home},
		installer.TargetSelection{Claude: true},
	); err != nil {
		t.Fatalf("upstream-shaped Claude fixture registrations rejected by preflight: %v", err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatalf("decode merged Claude fixture config: %v", err)
	}
	servers := root["mcpServers"].(map[string]any)
	context7 := servers["context7"].(map[string]any)
	wantContext7 := map[string]any{
		"command": "npx",
		"args":    []any{"-y", "--package=@upstash/context7-mcp@2.2.5", "--", "context7-mcp"},
	}
	if !reflect.DeepEqual(context7, wantContext7) {
		t.Fatalf("Claude Context7 fixture = %#v, want pinned upstream shape %#v", context7, wantContext7)
	}
	if got := servers["foreign"]; !reflect.DeepEqual(got, map[string]any{"command": "foreign-command", "args": []any{"keep"}}) {
		t.Fatalf("Claude fixture changed foreign MCP entry: %#v", got)
	}
	if root["other"] != "preserved" {
		t.Fatalf("Claude fixture changed unrelated root data: %#v", root["other"])
	}
}
