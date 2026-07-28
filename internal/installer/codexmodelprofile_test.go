package installer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestCodexModelTierPresets_LowCost(t *testing.T) {
	tier, ok := CodexModelTierByName("low-cost")
	if !ok {
		t.Fatal("CodexModelTierByName(\"low-cost\") = false, want true")
	}
	if tier.Name != "low-cost" {
		t.Fatalf("tier.Name = %q, want \"low-cost\"", tier.Name)
	}
	want := map[string]CodexModelAssignment{
		"orquestador":  {Model: "gpt-5.6-sol", Effort: "low"},
		"razonamiento": {Model: "gpt-5.6-terra", Effort: "medium"},
		"codigo":       {Model: "gpt-5.6-terra", Effort: "medium"},
		"liviano":      {Model: "gpt-5.6-luna", Effort: "low"},
	}
	if !reflect.DeepEqual(tier.Roles, want) {
		t.Fatalf("low-cost roles = %#v, want %#v", tier.Roles, want)
	}
}

func TestCodexModelTierPresets_Recommended(t *testing.T) {
	tier, ok := CodexModelTierByName("recommended")
	if !ok {
		t.Fatal("CodexModelTierByName(\"recommended\") = false, want true")
	}
	want := map[string]CodexModelAssignment{
		"orquestador":  {Model: "gpt-5.6-sol", Effort: "low"},
		"razonamiento": {Model: "gpt-5.6-sol", Effort: "medium"},
		"codigo":       {Model: "gpt-5.6-terra", Effort: "medium"},
		"liviano":      {Model: "gpt-5.6-luna", Effort: "low"},
	}
	if !reflect.DeepEqual(tier.Roles, want) {
		t.Fatalf("recommended roles = %#v, want %#v", tier.Roles, want)
	}
}

func TestCodexModelTierPresets_Powerful(t *testing.T) {
	tier, ok := CodexModelTierByName("powerful")
	if !ok {
		t.Fatal("CodexModelTierByName(\"powerful\") = false, want true")
	}
	want := map[string]CodexModelAssignment{
		"orquestador":  {Model: "gpt-5.6-sol", Effort: "low"},
		"razonamiento": {Model: "gpt-5.6-sol", Effort: "high"},
		"codigo":       {Model: "gpt-5.6-terra", Effort: "high"},
		"liviano":      {Model: "gpt-5.6-luna", Effort: "low"},
	}
	if !reflect.DeepEqual(tier.Roles, want) {
		t.Fatalf("powerful roles = %#v, want %#v", tier.Roles, want)
	}
}

func TestConfig_CodexModelProfilePath(t *testing.T) {
	home := t.TempDir()
	cfg := Config{CodexHome: home}
	want := filepath.Join(home, "click-ai-devkit", "model-profile.json")
	if got := cfg.CodexModelProfilePath(); got != want {
		t.Fatalf("Config.CodexModelProfilePath() = %q, want %q", got, want)
	}
	if got := (Config{}).CodexModelProfilePath(); got != "" {
		t.Fatalf("Config{}.CodexModelProfilePath() = %q, want empty path", got)
	}
}

func TestSaveCodexModelProfile_EmptyHome_NoOp(t *testing.T) {
	cfg := Config{CodexHome: ""}
	if err := SaveCodexModelProfile(cfg, "recommended"); err != nil {
		t.Fatalf("SaveCodexModelProfile(empty home) error = %v, want nil", err)
	}
}

func TestSaveCodexModelProfile_UnknownTier_ReturnsError(t *testing.T) {
	cfg := Config{CodexHome: t.TempDir()}
	if err := SaveCodexModelProfile(cfg, "not-a-tier"); err == nil {
		t.Fatal("SaveCodexModelProfile(unknown tier) error = nil, want error")
	}
}

func TestSaveCodexModelProfile_WritesSchemaVersionTierAndRoles(t *testing.T) {
	cfg := Config{CodexHome: t.TempDir()}
	if err := SaveCodexModelProfile(cfg, "recommended"); err != nil {
		t.Fatalf("SaveCodexModelProfile() error = %v", err)
	}

	raw, err := os.ReadFile(cfg.CodexModelProfilePath())
	if err != nil {
		t.Fatalf("ReadFile(model-profile.json) error = %v, want file written", err)
	}

	var wrapper codexModelProfileFile
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		t.Fatalf("json.Unmarshal(model-profile.json) error = %v", err)
	}
	if wrapper.SchemaVersion != CurrentCodexModelProfileSchemaVersion {
		t.Fatalf("schema_version = %d, want %d", wrapper.SchemaVersion, CurrentCodexModelProfileSchemaVersion)
	}
	if wrapper.Tier != "recommended" {
		t.Fatalf("tier = %q, want \"recommended\"", wrapper.Tier)
	}

	want := CodexModelTierByNameOrDefault("recommended").Roles
	if !reflect.DeepEqual(wrapper.Roles, want) {
		t.Fatalf("roles = %#v, want %#v", wrapper.Roles, want)
	}
}

func TestRemoveCodexModelProfile_RemovesFileAndIsIdempotent(t *testing.T) {
	cfg := Config{CodexHome: t.TempDir()}
	if err := SaveCodexModelProfile(cfg, "recommended"); err != nil {
		t.Fatalf("SaveCodexModelProfile() error = %v", err)
	}
	if _, err := os.Stat(cfg.CodexModelProfilePath()); err != nil {
		t.Fatalf("model-profile.json should exist before remove: %v", err)
	}

	if err := RemoveCodexModelProfile(cfg); err != nil {
		t.Fatalf("RemoveCodexModelProfile() error = %v, want nil", err)
	}
	if _, err := os.Stat(cfg.CodexModelProfilePath()); !os.IsNotExist(err) {
		t.Fatalf("model-profile.json should be removed: %v", err)
	}

	if err := RemoveCodexModelProfile(cfg); err != nil {
		t.Fatalf("RemoveCodexModelProfile() second call error = %v, want nil (idempotent)", err)
	}
}

func TestRemoveCodexModelProfile_EmptyHome_NoOp(t *testing.T) {
	cfg := Config{CodexHome: ""}
	if err := RemoveCodexModelProfile(cfg); err != nil {
		t.Fatalf("RemoveCodexModelProfile(empty home) error = %v, want nil", err)
	}
}

func TestLoadCodexModelProfile_NoFileYet_ReturnsNotFound(t *testing.T) {
	cfg := Config{CodexHome: t.TempDir()}
	tier, found, err := LoadCodexModelProfile(cfg)
	if err != nil {
		t.Fatalf("LoadCodexModelProfile() error = %v, want nil", err)
	}
	if found {
		t.Fatal("LoadCodexModelProfile() found = true before any save, want false")
	}
	if tier != "" {
		t.Fatalf("LoadCodexModelProfile() tier = %q, want empty", tier)
	}
}

func TestLoadCodexModelProfile_AfterSave_RoundTrips(t *testing.T) {
	cfg := Config{CodexHome: t.TempDir()}
	if err := SaveCodexModelProfile(cfg, "low-cost"); err != nil {
		t.Fatalf("SaveCodexModelProfile() error = %v", err)
	}
	tier, found, err := LoadCodexModelProfile(cfg)
	if err != nil {
		t.Fatalf("LoadCodexModelProfile() error = %v", err)
	}
	if !found {
		t.Fatal("LoadCodexModelProfile() found = false after save, want true")
	}
	if tier != "low-cost" {
		t.Fatalf("LoadCodexModelProfile() tier = %q, want \"low-cost\"", tier)
	}
}

func TestLoadCodexModelProfile_UnknownTierFallback(t *testing.T) {
	cfg := Config{CodexHome: t.TempDir()}
	raw, err := json.Marshal(codexModelProfileFile{
		SchemaVersion: CurrentCodexModelProfileSchemaVersion,
		Tier:          "not-a-real-tier",
		Roles:         map[string]CodexModelAssignment{},
	})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(cfg.CodexModelProfilePath()), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(cfg.CodexModelProfilePath(), raw, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	tier, found, err := LoadCodexModelProfile(cfg)
	if err != nil {
		t.Fatalf("LoadCodexModelProfile() error = %v", err)
	}
	if !found {
		t.Fatal("LoadCodexModelProfile() found = false for existing file, want true")
	}
	if tier != "recommended" {
		t.Fatalf("LoadCodexModelProfile() tier = %q, want \"recommended\" fallback", tier)
	}
}
