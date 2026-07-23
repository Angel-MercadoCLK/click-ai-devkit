package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPluginsCommand_ListsManagedStateAndStagingWithoutWriting(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CLICK_CLAUDE_HOME", home)
	before, err := os.ReadDir(home)
	if err != nil {
		t.Fatal(err)
	}

	out, err := execRoot(t, home, "plugins")
	if err != nil {
		t.Fatalf("plugins error = %v, output = %s", err, out)
	}
	for _, want := range []string{
		"Plugins gestionados por Click",
		"click-sdd: no conocido en el registro; no instalado",
		"Staging local para futuros plugins: " + filepath.Join(home, "click-ai-devkit", "plugins"),
		"Agregar un plugin al staging no lo instala ni lo activa",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("plugins output missing %q:\n%s", want, out)
		}
	}
	after, err := os.ReadDir(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before) {
		t.Fatalf("plugins changed home directory entries: before=%d after=%d", len(before), len(after))
	}
}

// TestPluginsCommand_DistinguishesEnabledFromDisabled guards the user-facing half of FIX 4: a
// managed plugin registered in installed_plugins.json but disabled in settings.json must render as
// "registrado pero deshabilitado", never as plainly "instalado" — so `click plugins` can never
// contradict `click doctor`'s enabled-only view.
func TestPluginsCommand_DistinguishesEnabledFromDisabled(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, "plugins"), 0o755); err != nil {
		t.Fatal(err)
	}
	sddID := "click-sdd@click-ai-devkit"
	memID := "click-memory@click-ai-devkit"
	writeJSONFileForTest(t, filepath.Join(home, "plugins", "installed_plugins.json"), map[string]any{
		"plugins": map[string]any{
			sddID: []map[string]any{{"installPath": ""}},
			memID: []map[string]any{{"installPath": ""}},
		},
	})
	writeJSONFileForTest(t, filepath.Join(home, "settings.json"), map[string]any{
		"enabledPlugins": map[string]bool{sddID: true, memID: false},
	})

	out, err := execRoot(t, home, "plugins")
	if err != nil {
		t.Fatalf("plugins error = %v, output = %s", err, out)
	}
	if !strings.Contains(out, "click-sdd: no conocido en el registro; instalado y habilitado") {
		t.Errorf("plugins output missing enabled status for click-sdd:\n%s", out)
	}
	if !strings.Contains(out, "click-memory: no conocido en el registro; registrado pero deshabilitado — ejecute `click update`") {
		t.Errorf("plugins output missing registered-but-disabled status for click-memory:\n%s", out)
	}
}

func writeJSONFileForTest(t *testing.T, path string, data any) {
	t.Helper()
	raw, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}
