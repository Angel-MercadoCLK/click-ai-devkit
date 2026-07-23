package doctor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/installer"
)

const clickPluginRegistryCheckName = "Registros de plugins de Claude"

func TestCheckClickPluginRegistries_CompleteStateReportsHealthy(t *testing.T) {
	cfg := installer.Config{ClaudeHome: t.TempDir()}
	seedClickPluginRegistries(t, cfg, true, "https://github.com/Angel-MercadoCLK/click-ai-devkit.git", installerClickPluginIDs())

	check := checkClickPluginRegistries(cfg)

	if !check.Healthy {
		t.Fatalf("checkClickPluginRegistries() Healthy = false, want true: %s", check.Detail)
	}
	if !strings.Contains(check.Detail, "4 plugins") {
		t.Fatalf("checkClickPluginRegistries() Detail = %q, want installed count", check.Detail)
	}
}

func TestCheckClickPluginRegistries_MissingMarketplaceRegistryReportsActionableWarning(t *testing.T) {
	cfg := installer.Config{ClaudeHome: t.TempDir()}
	seedClickPluginRegistries(t, cfg, false, "", installerClickPluginIDs())

	check := checkClickPluginRegistries(cfg)

	if check.Healthy {
		t.Fatal("checkClickPluginRegistries() Healthy = true, want false")
	}
	if !strings.Contains(check.Detail, "known_marketplaces.json") || !strings.Contains(check.Detail, "click update") {
		t.Fatalf("checkClickPluginRegistries() Detail = %q, want missing registry and click update", check.Detail)
	}
}

func TestCheckClickPluginRegistries_MissingInstalledRegistryReportsActionableWarning(t *testing.T) {
	cfg := installer.Config{ClaudeHome: t.TempDir()}
	seedClickPluginRegistries(t, cfg, true, "https://github.com/Angel-MercadoCLK/click-ai-devkit", installerClickPluginIDs())
	if err := os.Remove(cfg.InstalledPluginsPath()); err != nil {
		t.Fatalf("Remove(installed plugins) error = %v", err)
	}

	check := checkClickPluginRegistries(cfg)

	if check.Healthy {
		t.Fatal("checkClickPluginRegistries() Healthy = true, want false")
	}
	if !strings.Contains(check.Detail, "installed_plugins.json") || !strings.Contains(check.Detail, "click update") {
		t.Fatalf("checkClickPluginRegistries() Detail = %q, want missing registry and click update", check.Detail)
	}
}

func TestCheckClickPluginRegistries_SourceMismatchReportsActionableWarning(t *testing.T) {
	cfg := installer.Config{ClaudeHome: t.TempDir()}
	seedClickPluginRegistries(t, cfg, true, "https://example.invalid/not-click", installerClickPluginIDs())

	check := checkClickPluginRegistries(cfg)

	if check.Healthy {
		t.Fatal("checkClickPluginRegistries() Healthy = true, want false")
	}
	if !strings.Contains(check.Detail, "fuente") || !strings.Contains(check.Detail, "click update") {
		t.Fatalf("checkClickPluginRegistries() Detail = %q, want source mismatch and click update", check.Detail)
	}
}

func TestCheckClickPluginRegistries_IncompletePluginSetReportsMissingIDs(t *testing.T) {
	cfg := installer.Config{ClaudeHome: t.TempDir()}
	ids := installerClickPluginIDs()[:2]
	seedClickPluginRegistries(t, cfg, true, "https://github.com/Angel-MercadoCLK/click-ai-devkit", ids)

	check := checkClickPluginRegistries(cfg)

	if check.Healthy {
		t.Fatal("checkClickPluginRegistries() Healthy = true, want false")
	}
	for _, plugin := range []string{"click-review@click-ai-devkit", "click-skills@click-ai-devkit"} {
		if !strings.Contains(check.Detail, plugin) {
			t.Errorf("checkClickPluginRegistries() Detail = %q, want missing plugin %q", check.Detail, plugin)
		}
	}
	if !strings.Contains(check.Detail, "click update") {
		t.Fatalf("checkClickPluginRegistries() Detail = %q, want click update", check.Detail)
	}
}

func TestCheckClickPluginRegistries_MalformedJSONReportsActionableWarning(t *testing.T) {
	cfg := installer.Config{ClaudeHome: t.TempDir()}
	if err := os.MkdirAll(filepath.Dir(cfg.KnownMarketplacesPath()), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(cfg.KnownMarketplacesPath(), []byte("{not json"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	check := checkClickPluginRegistries(cfg)

	if check.Healthy {
		t.Fatal("checkClickPluginRegistries() Healthy = true, want false")
	}
	if !strings.Contains(check.Detail, "malformado") || !strings.Contains(check.Detail, "click update") {
		t.Fatalf("checkClickPluginRegistries() Detail = %q, want malformed warning and click update", check.Detail)
	}
}

func installerClickPluginIDs() []string {
	return []string{
		"click-sdd@click-ai-devkit",
		"click-memory@click-ai-devkit",
		"click-review@click-ai-devkit",
		"click-skills@click-ai-devkit",
	}
}

func seedClickPluginRegistries(t *testing.T, cfg installer.Config, marketplace bool, source string, pluginIDs []string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(cfg.KnownMarketplacesPath()), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	known := map[string]any{}
	if marketplace {
		known["click-ai-devkit"] = map[string]any{
			"source":          map[string]any{"source": "git", "url": source},
			"installLocation": filepath.Join(cfg.ClaudeHome, "plugins", "marketplaces", "click-ai-devkit"),
		}
	}
	knownData, err := json.Marshal(known)
	if err != nil {
		t.Fatalf("json.Marshal(known) error = %v", err)
	}
	if err := os.WriteFile(cfg.KnownMarketplacesPath(), knownData, 0o600); err != nil {
		t.Fatalf("WriteFile(known marketplaces) error = %v", err)
	}

	plugins := map[string][]map[string]any{}
	for _, id := range pluginIDs {
		plugins[id] = []map[string]any{{}}
	}
	installedData, err := json.Marshal(map[string]any{"version": 2, "plugins": plugins})
	if err != nil {
		t.Fatalf("json.Marshal(installed) error = %v", err)
	}
	if err := os.WriteFile(cfg.InstalledPluginsPath(), installedData, 0o600); err != nil {
		t.Fatalf("WriteFile(installed plugins) error = %v", err)
	}
}
