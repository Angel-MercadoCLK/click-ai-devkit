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
