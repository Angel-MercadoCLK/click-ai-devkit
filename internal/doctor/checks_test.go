package doctor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/installer"
	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/manifest"
)

func TestRun_BeforeInstall_ReportsUnhealthy(t *testing.T) {
	cfg := installer.Config{ClaudeHome: t.TempDir()}

	report := Run(cfg)

	if report.Healthy() {
		t.Fatal("Run() on a fresh, never-installed ClaudeHome reports healthy, want unhealthy")
	}
	if len(report.Checks) == 0 {
		t.Fatal("Run() returned zero checks")
	}
}

func TestRun_AfterInstall_ReportsHealthy(t *testing.T) {
	cfg := installer.Config{ClaudeHome: t.TempDir()}

	seedInstalledState(t, cfg)
	if err := installer.WriteManagedBlock(cfg.ClaudeMDPath(), installer.DefaultManagedContent); err != nil {
		t.Fatalf("WriteManagedBlock() error = %v", err)
	}
	if err := installer.RegisterMemoryGuardHook(cfg); err != nil {
		t.Fatalf("RegisterMemoryGuardHook() error = %v", err)
	}
	binaryPath := filepath.Join(t.TempDir(), "engram.exe")
	if err := os.WriteFile(binaryPath, []byte("binary"), 0o644); err != nil {
		t.Fatalf("WriteFile(binary) error = %v", err)
	}
	t.Setenv("CLICK_ENGRAM_BINARY_PATH", binaryPath)
	seedContext7Registered(t, cfg)

	report := Run(cfg)

	if !report.Healthy() {
		t.Fatalf("Run() after Install() reports unhealthy, want healthy: %+v", report.Checks)
	}
	for _, c := range report.Checks {
		if !c.Healthy {
			t.Errorf("check %q reported unhealthy after Install(): %s", c.Name, c.Detail)
		}
	}
}

func TestRun_AfterUninstall_ReportsUnhealthy(t *testing.T) {
	cfg := installer.Config{ClaudeHome: t.TempDir()}

	seedInstalledState(t, cfg)
	if err := installer.WriteManagedBlock(cfg.ClaudeMDPath(), installer.DefaultManagedContent); err != nil {
		t.Fatalf("WriteManagedBlock() error = %v", err)
	}
	if err := installer.RegisterMemoryGuardHook(cfg); err != nil {
		t.Fatalf("RegisterMemoryGuardHook() error = %v", err)
	}
	if err := installer.StripManagedBlock(cfg.ClaudeMDPath()); err != nil {
		t.Fatalf("StripManagedBlock() error = %v", err)
	}
	if err := os.Remove(cfg.InstalledPluginsPath()); err != nil {
		t.Fatalf("Remove(installed_plugins.json) error = %v", err)
	}
	if err := installer.UnregisterMemoryGuardHook(cfg); err != nil {
		t.Fatalf("UnregisterMemoryGuardHook() error = %v", err)
	}

	report := Run(cfg)

	if report.Healthy() {
		t.Fatal("Run() after Uninstall() reports healthy, want unhealthy")
	}
}

func TestRun_ChecksHavePluginAndClaudeMD(t *testing.T) {
	cfg := installer.Config{ClaudeHome: t.TempDir()}
	report := Run(cfg)

	const wantChecks = 5 + EngramChecksCount + Context7ChecksCount
	if len(report.Checks) != wantChecks {
		t.Fatalf("Run() returned %d checks, want %d (click-sdd plugin, click-memory plugin, click-review plugin, CLAUDE.md, memory-guard hook, engram plugin, engram binary, context7 MCP)", len(report.Checks), wantChecks)
	}
}

// TestCheckEngramBinary_ReportsRemediationWhenMissing guards the shared-message contract from
// Slice 3b: when the engram binary is missing, doctor's Detail must include the exact same `go
// install` remediation command that `click install`'s own non-fatal provisioning fallback shows
// (installer.EngramBinaryRemediationMessage) — not a bare "missing" note that leaves the developer
// guessing the next step.
func TestCheckEngramBinary_ReportsRemediationWhenMissing(t *testing.T) {
	cfg := installer.Config{ClaudeHome: t.TempDir()}
	t.Setenv("CLICK_ENGRAM_BINARY_PATH", filepath.Join(t.TempDir(), "does-not-exist", "engram.exe"))

	m, err := manifest.Load()
	if err != nil {
		t.Fatalf("manifest.Load() error = %v", err)
	}

	report := Run(cfg)

	var checked bool
	for _, c := range report.Checks {
		if c.Name != "engram binary" {
			continue
		}
		checked = true
		if c.Healthy {
			t.Fatal("checkEngramBinary reports healthy for a binary path that does not exist on disk")
		}
		wantCmd := installer.EngramInstallCommand(m.Engram.Version)
		if !strings.Contains(c.Detail, wantCmd) {
			t.Fatalf("checkEngramBinary Detail = %q, want it to contain the remediation command %q", c.Detail, wantCmd)
		}
	}
	if !checked {
		t.Fatal(`Run() did not include an "engram binary" check`)
	}
}

// TestCheckContext7_ReportsMissingByDefault guards the "not registered yet" case: a fresh
// ClaudeHome where `click install` never ran (or context7 was never synced) must report
// unhealthy, not silently skip the check.
func TestCheckContext7_ReportsMissingByDefault(t *testing.T) {
	cfg := installer.Config{ClaudeHome: t.TempDir()}

	report := Run(cfg)

	var checked bool
	for _, c := range report.Checks {
		if c.Name != "context7 MCP" {
			continue
		}
		checked = true
		if c.Healthy {
			t.Fatal("checkContext7 reports healthy on a fresh home with no context7 registration")
		}
	}
	if !checked {
		t.Fatal(`Run() did not include a "context7 MCP" check`)
	}
}

// TestCheckContext7_ReportsHealthyWhenRegistered covers the "registered" case, reading directly
// from Claude Code's own user config file (the same file `claude mcp add --scope user` writes,
// per documentacion/spikes/spike-g-context7.md) rather than shelling out — matching every other
// check in this package.
func TestCheckContext7_ReportsHealthyWhenRegistered(t *testing.T) {
	cfg := installer.Config{ClaudeHome: t.TempDir()}
	seedContext7Registered(t, cfg)

	report := Run(cfg)

	var checked bool
	for _, c := range report.Checks {
		if c.Name != "context7 MCP" {
			continue
		}
		checked = true
		if !c.Healthy {
			t.Fatalf("checkContext7 reports unhealthy for a registered context7: %s", c.Detail)
		}
	}
	if !checked {
		t.Fatal(`Run() did not include a "context7 MCP" check`)
	}
}

func seedContext7Registered(t *testing.T, cfg installer.Config) {
	t.Helper()
	data := map[string]any{
		"mcpServers": map[string]any{
			"context7": map[string]any{"type": "http", "url": "https://mcp.context7.com/mcp"},
		},
	}
	raw, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("json.Marshal(context7 config) error = %v", err)
	}
	if err := os.WriteFile(cfg.Context7ConfigPath(), raw, 0o600); err != nil {
		t.Fatalf("WriteFile(Context7ConfigPath) error = %v", err)
	}
}

func seedInstalledState(t *testing.T, cfg installer.Config) {
	t.Helper()
	type pluginsRegistry struct {
		Version int                         `json:"version"`
		Plugins map[string][]map[string]any `json:"plugins"`
	}
	registry := pluginsRegistry{
		Version: 2,
		Plugins: map[string][]map[string]any{
			"click-sdd@click-ai-devkit":    {{}},
			"click-memory@click-ai-devkit": {{}},
			"click-review@click-ai-devkit": {{}},
			installer.EngramPluginID:       {{}},
		},
	}
	data, err := json.Marshal(registry)
	if err != nil {
		t.Fatalf("json.Marshal(registry) error = %v", err)
	}
	if err := os.MkdirAll(filepathDir(cfg.InstalledPluginsPath()), 0o755); err != nil {
		t.Fatalf("MkdirAll(installed plugins dir) error = %v", err)
	}
	if err := os.WriteFile(cfg.InstalledPluginsPath(), data, 0o600); err != nil {
		t.Fatalf("WriteFile(installed_plugins.json) error = %v", err)
	}
	settings := map[string]any{
		"enabledPlugins": map[string]bool{
			"click-sdd@click-ai-devkit":    true,
			"click-memory@click-ai-devkit": true,
			"click-review@click-ai-devkit": true,
			installer.EngramPluginID:       true,
		},
	}
	settingsData, err := json.Marshal(settings)
	if err != nil {
		t.Fatalf("json.Marshal(settings) error = %v", err)
	}
	if err := os.WriteFile(cfg.SettingsPath(), settingsData, 0o600); err != nil {
		t.Fatalf("WriteFile(settings.json) error = %v", err)
	}
}

func filepathDir(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '\\' || path[i] == '/' {
			return path[:i]
		}
	}
	return "."
}
