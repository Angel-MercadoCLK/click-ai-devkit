package doctor

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/installer"
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

	if len(report.Checks) != 5 {
		t.Fatalf("Run() returned %d checks, want 5 (click-sdd plugin, click-memory plugin, click-review plugin, CLAUDE.md, memory-guard hook)", len(report.Checks))
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
