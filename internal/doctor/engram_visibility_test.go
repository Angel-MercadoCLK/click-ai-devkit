package doctor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/installer"
)

func TestCheckEngramSubagentVisibility_VerifiesPluginMCPAndAgentTools(t *testing.T) {
	cfg := installer.Config{ClaudeHome: t.TempDir()}
	engramPath := filepath.Join(cfg.ClaudeHome, "plugins", "cache", "engram", "engram", "0.1.1")
	clickSDDPath := filepath.Join(cfg.ClaudeHome, "plugins", "cache", "click-ai-devkit", "click-sdd", "0.1.0")
	writeInstalledPluginPaths(t, cfg, map[string]string{
		installer.EngramPluginID:   engramPath,
		installer.ClickSDDPluginID: clickSDDPath,
	})
	writeJSON(t, filepath.Join(engramPath, ".mcp.json"), map[string]any{
		"mcpServers": map[string]any{
			"engram": map[string]any{"command": "engram", "args": []string{"mcp", "--tools=agent"}},
		},
	})
	for agent, tools := range engramAgentToolRequirements {
		writeFile(t, filepath.Join(clickSDDPath, "agents", agent+".md"), "---\ntools: "+strings.Join(tools, ", ")+"\n---\n")
	}
	writeFile(t, cfg.SettingsPath(), `{"enabledPlugins":{"engram@engram":true}}`)

	check := checkEngramSubagentVisibility(cfg)
	if !check.Healthy {
		t.Fatalf("checkEngramSubagentVisibility() Healthy = false: %s", check.Detail)
	}
	if !strings.Contains(check.Detail, "propagación") || !strings.Contains(check.Detail, "sesión") {
		t.Fatalf("check detail = %q, want runtime propagation limitation", check.Detail)
	}
}

func TestCheckEngramSubagentVisibility_AcceptsGentleAIDirectMCPAndAgentTools(t *testing.T) {
	cfg := installer.Config{ClaudeHome: t.TempDir()}
	clickSDDPath := filepath.Join(cfg.ClaudeHome, "plugins", "cache", "click-ai-devkit", "click-sdd", "0.1.0")
	writeInstalledPluginPaths(t, cfg, map[string]string{installer.ClickSDDPluginID: clickSDDPath})
	writeJSON(t, cfg.Context7ConfigPath(), map[string]any{
		"mcpServers": map[string]any{
			"engram": map[string]any{"command": "engram", "args": []string{"mcp", "--tools=agent"}},
		},
	})
	for agent, tools := range engramAgentToolRequirements {
		directTools := make([]string, len(tools))
		for i, tool := range tools {
			directTools[i] = strings.Replace(tool, "mcp__plugin_engram_engram__", "mcp__engram__", 1)
		}
		writeFile(t, filepath.Join(clickSDDPath, "agents", agent+".md"), "---\ntools: "+strings.Join(directTools, ", ")+"\n---\n")
	}

	check := checkEngramSubagentVisibility(cfg)
	if !check.Healthy {
		t.Fatalf("checkEngramSubagentVisibility() Healthy = false for direct MCP: %s", check.Detail)
	}
}

func TestCheckEngramSubagentVisibility_RejectsAgentToolsForUnavailableTransport(t *testing.T) {
	for _, tt := range []struct {
		name        string
		useDirect   bool
		assetDirect bool
	}{
		{name: "direct registration with plugin-only tools", useDirect: true, assetDirect: false},
		{name: "plugin registration with direct-only tools", useDirect: false, assetDirect: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := installer.Config{ClaudeHome: t.TempDir()}
			clickSDDPath := filepath.Join(cfg.ClaudeHome, "plugins", "cache", "click-ai-devkit", "click-sdd", "0.1.0")
			engramPath := filepath.Join(cfg.ClaudeHome, "plugins", "cache", "engram", "engram", "0.1.1")
			paths := map[string]string{installer.ClickSDDPluginID: clickSDDPath}
			if tt.useDirect {
				writeJSON(t, cfg.Context7ConfigPath(), map[string]any{"mcpServers": map[string]any{
					"engram": map[string]any{"command": "engram", "args": []string{"mcp", "--tools=agent"}},
				}})
			} else {
				paths[installer.EngramPluginID] = engramPath
				writeFile(t, cfg.SettingsPath(), `{"enabledPlugins":{"engram@engram":true}}`)
				writeJSON(t, filepath.Join(engramPath, ".mcp.json"), map[string]any{"mcpServers": map[string]any{
					"engram": map[string]any{"command": "engram", "args": []string{"mcp", "--tools=agent"}},
				}})
			}
			writeInstalledPluginPaths(t, cfg, paths)
			for agent, tools := range engramAgentToolRequirements {
				agentTools := append([]string(nil), tools...)
				if tt.assetDirect {
					for i := range agentTools {
						agentTools[i] = strings.Replace(agentTools[i], "mcp__plugin_engram_engram__", "mcp__engram__", 1)
					}
				}
				writeFile(t, filepath.Join(clickSDDPath, "agents", agent+".md"), "---\ntools: "+strings.Join(agentTools, ", ")+"\n---\n")
			}

			check := checkEngramSubagentVisibility(cfg)
			if check.Healthy {
				t.Fatalf("checkEngramSubagentVisibility() Healthy = true; incompatible transport should fail: %s", check.Detail)
			}
		})
	}
}

func TestClickSDDAgentAssetsDeclareBothEngramTransports(t *testing.T) {
	for agent, requiredTools := range engramAgentToolRequirements {
		data, err := os.ReadFile(filepath.Join("..", "..", "plugins", "click-sdd", "agents", agent+".md"))
		if err != nil {
			t.Fatalf("read bundled agent %s: %v", agent, err)
		}
		tools := frontmatterTools(string(data))
		for _, pluginTool := range requiredTools {
			directTool := strings.Replace(pluginTool, "mcp__plugin_engram_engram__", "mcp__engram__", 1)
			if !containsString(tools, pluginTool) || !containsString(tools, directTool) {
				t.Errorf("agent %s tools missing transport pair %q and %q", agent, pluginTool, directTool)
			}
		}
	}
}

func TestCheckEngramSubagentVisibility_ReportsUnresolvedCachePath(t *testing.T) {
	cfg := installer.Config{ClaudeHome: t.TempDir()}
	writeInstalledPluginPaths(t, cfg, map[string]string{installer.EngramPluginID: ""})
	writeFile(t, cfg.SettingsPath(), `{"enabledPlugins":{"engram@engram":true}}`)

	check := checkEngramSubagentVisibility(cfg)
	if check.Healthy {
		t.Fatal("checkEngramSubagentVisibility() Healthy = true, want false when cache path is unavailable")
	}
	if !strings.Contains(check.Detail, "no se puede resolver de forma segura") {
		t.Fatalf("check detail = %q, want safe path limitation", check.Detail)
	}
}

func TestCheckEngramSubagentVisibility_DistinguishesStaleCacheFromMissingPackage(t *testing.T) {
	cfg := installer.Config{ClaudeHome: t.TempDir()}
	engramPath := filepath.Join(cfg.ClaudeHome, "plugins", "cache", "engram", "engram", "0.1.1")
	clickSDDPath := filepath.Join(cfg.ClaudeHome, "plugins", "cache", "click-ai-devkit", "click-sdd", "0.1.0")
	writeInstalledPluginPaths(t, cfg, map[string]string{installer.EngramPluginID: engramPath, installer.ClickSDDPluginID: clickSDDPath})
	if err := os.MkdirAll(clickSDDPath, 0o755); err != nil {
		t.Fatal(err)
	}
	writeJSON(t, filepath.Join(engramPath, ".mcp.json"), map[string]any{"mcpServers": map[string]any{"engram": map[string]any{"command": "engram", "args": []string{"mcp", "--tools=agent"}}}})
	writeFile(t, cfg.SettingsPath(), `{"enabledPlugins":{"engram@engram":true}}`)
	check := checkEngramSubagentVisibility(cfg)
	if check.Healthy || !strings.Contains(check.Detail, clickSDDPath) || !strings.Contains(check.Detail, "Actualice") {
		t.Fatalf("check = %#v, want stale cache path and update guidance", check)
	}
}

func writeInstalledPluginPaths(t *testing.T, cfg installer.Config, paths map[string]string) {
	t.Helper()
	plugins := map[string][]map[string]any{}
	for id, path := range paths {
		entry := map[string]any{}
		if path != "" {
			entry["installPath"] = path
		}
		plugins[id] = []map[string]any{entry}
	}
	writeJSON(t, cfg.InstalledPluginsPath(), map[string]any{"plugins": plugins})
}

func writeJSON(t *testing.T, path string, value any) {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	writeFile(t, path, string(raw))
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", path, err)
	}
}
