package installer

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/modelconfig"
)

func TestLoadModels_NoFileYet_ReturnsNotFound(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir()}

	models, found, err := LoadModels(cfg)
	if err != nil {
		t.Fatalf("LoadModels() error = %v, want nil", err)
	}
	if found {
		t.Fatal("LoadModels() found = true before any SaveModels() call, want false")
	}
	if models != nil {
		t.Fatalf("LoadModels() models = %#v, want nil when not found", models)
	}
}

func TestSaveModels_ThenLoadModels_RoundTrips(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir()}
	want := map[modelconfig.Phase]string{
		modelconfig.PhaseOrchestrator:  "sonnet",
		modelconfig.PhasePRDWriter:     "opus",
		modelconfig.PhaseArchitect:     "haiku",
		modelconfig.PhaseReviewer:      "opus",
		modelconfig.PhaseMemoryCurator: "sonnet",
	}

	if err := SaveModels(cfg, want); err != nil {
		t.Fatalf("SaveModels() error = %v", err)
	}

	got, found, err := LoadModels(cfg)
	if err != nil {
		t.Fatalf("LoadModels() error = %v", err)
	}
	if !found {
		t.Fatal("LoadModels() found = false right after SaveModels(), want true")
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("LoadModels() = %#v, want %#v", got, want)
	}
}

func TestSaveModels_WritesUnderClickAIDevkitDir(t *testing.T) {
	claudeHome := t.TempDir()
	cfg := Config{ClaudeHome: claudeHome}

	if err := SaveModels(cfg, modelconfig.Defaults()); err != nil {
		t.Fatalf("SaveModels() error = %v", err)
	}

	wantPath := filepath.Join(claudeHome, "click-ai-devkit", "models.json")
	if _, err := os.Stat(wantPath); err != nil {
		t.Fatalf("Stat(%s) error = %v, want models.json written there", wantPath, err)
	}
}

func TestSaveModels_Overwrites(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir()}

	first := map[modelconfig.Phase]string{modelconfig.PhaseOrchestrator: "opus", modelconfig.PhasePRDWriter: "opus", modelconfig.PhaseArchitect: "opus", modelconfig.PhaseReviewer: "opus", modelconfig.PhaseMemoryCurator: "sonnet"}
	if err := SaveModels(cfg, first); err != nil {
		t.Fatalf("SaveModels(first) error = %v", err)
	}

	second := map[modelconfig.Phase]string{modelconfig.PhaseOrchestrator: "haiku", modelconfig.PhasePRDWriter: "haiku", modelconfig.PhaseArchitect: "haiku", modelconfig.PhaseReviewer: "haiku", modelconfig.PhaseMemoryCurator: "haiku"}
	if err := SaveModels(cfg, second); err != nil {
		t.Fatalf("SaveModels(second) error = %v", err)
	}

	got, found, err := LoadModels(cfg)
	if err != nil {
		t.Fatalf("LoadModels() error = %v", err)
	}
	if !found {
		t.Fatal("LoadModels() found = false after two SaveModels() calls, want true")
	}
	if !reflect.DeepEqual(got, second) {
		t.Fatalf("LoadModels() = %#v, want the second saved map %#v", got, second)
	}
}

func TestConfig_ModelsPath(t *testing.T) {
	cfg := Config{ClaudeHome: filepath.Join("C:", "fake-home")}
	want := filepath.Join("C:", "fake-home", "click-ai-devkit", "models.json")
	if got := cfg.ModelsPath(); got != want {
		t.Fatalf("Config.ModelsPath() = %q, want %q", got, want)
	}
}
