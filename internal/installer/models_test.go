package installer

import (
	"encoding/json"
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
		modelconfig.PhaseExplore: "sonnet",
		modelconfig.PhasePropose: "opus",
		modelconfig.PhaseDesign:  "haiku",
		modelconfig.PhaseVerify:  "opus",
		modelconfig.PhaseApply:   "sonnet",
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

	first := map[modelconfig.Phase]string{modelconfig.PhaseExplore: "opus", modelconfig.PhasePropose: "opus", modelconfig.PhaseDesign: "opus", modelconfig.PhaseVerify: "opus", modelconfig.PhaseApply: "sonnet"}
	if err := SaveModels(cfg, first); err != nil {
		t.Fatalf("SaveModels(first) error = %v", err)
	}

	second := map[modelconfig.Phase]string{modelconfig.PhaseExplore: "haiku", modelconfig.PhasePropose: "haiku", modelconfig.PhaseDesign: "haiku", modelconfig.PhaseVerify: "haiku", modelconfig.PhaseApply: "haiku"}
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

// TestSaveModels_WritesCurrentSchemaVersion guards the versioned-schema requirement: every file
// SaveModels writes must self-report schema_version so a later stale/current check never has to
// guess.
func TestSaveModels_WritesCurrentSchemaVersion(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir()}
	if err := SaveModels(cfg, modelconfig.Defaults()); err != nil {
		t.Fatalf("SaveModels() error = %v", err)
	}

	raw, err := os.ReadFile(cfg.ModelsPath())
	if err != nil {
		t.Fatalf("ReadFile(models.json) error = %v", err)
	}
	var wrapper struct {
		SchemaVersion int `json:"schema_version"`
	}
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		t.Fatalf("json.Unmarshal(models.json) error = %v", err)
	}
	if wrapper.SchemaVersion != CurrentModelsSchemaVersion {
		t.Fatalf("models.json schema_version = %d, want %d", wrapper.SchemaVersion, CurrentModelsSchemaVersion)
	}
}

// TestIsStale_NoFileYet_ReportsNotStale guards the "absent = healthy" contract: a home where
// `click install` never ran must not be flagged stale — it just means defaults will be generated
// on next install/update.
func TestIsStale_NoFileYet_ReportsNotStale(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir()}

	stale, err := IsStale(cfg)
	if err != nil {
		t.Fatalf("IsStale() error = %v, want nil", err)
	}
	if stale {
		t.Fatal("IsStale() = true for a home with no models.json, want false")
	}
}

// TestIsStale_CurrentSchema_ReportsNotStale guards the happy path: a file SaveModels just wrote
// (current schema_version, new-taxonomy keys) must never be flagged stale.
func TestIsStale_CurrentSchema_ReportsNotStale(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir()}
	if err := SaveModels(cfg, modelconfig.Defaults()); err != nil {
		t.Fatalf("SaveModels() error = %v", err)
	}

	stale, err := IsStale(cfg)
	if err != nil {
		t.Fatalf("IsStale() error = %v, want nil", err)
	}
	if stale {
		t.Fatal("IsStale() = true right after SaveModels() with current defaults, want false")
	}
}

// TestIsStale_LegacyFlatFileNoSchemaVersion_ReportsStale guards migration detection for a
// pre-realignment models.json: the OLD format was a bare flat map (phase -> model) with no
// schema_version wrapper at all, written by the invented 5-phase taxonomy.
func TestIsStale_LegacyFlatFileNoSchemaVersion_ReportsStale(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir()}
	legacy := map[string]string{
		"orchestrator":   "opus",
		"prd_writer":     "opus",
		"architect":      "opus",
		"reviewer":       "opus",
		"memory_curator": "sonnet",
	}
	writeLegacyModelsFile(t, cfg, legacy)

	stale, err := IsStale(cfg)
	if err != nil {
		t.Fatalf("IsStale() error = %v, want nil", err)
	}
	if !stale {
		t.Fatal("IsStale() = false for a legacy flat-map models.json with no schema_version, want true")
	}
}

func TestIsStale_LowerSchemaVersion_ReportsStale(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir()}
	raw, err := json.Marshal(map[string]any{
		"schema_version": 1,
		"models":         map[string]string{"explore": "sonnet"},
	})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(cfg.ModelsPath()), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(cfg.ModelsPath(), raw, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	stale, err := IsStale(cfg)
	if err != nil {
		t.Fatalf("IsStale() error = %v, want nil", err)
	}
	if !stale {
		t.Fatalf("IsStale() = false for schema_version 1 (< current %d), want true", CurrentModelsSchemaVersion)
	}
}

// TestMigrateIfStale_NoFileYet_NoOp guards that migration never creates a models.json out of thin
// air — a never-installed home stays untouched.
func TestMigrateIfStale_NoFileYet_NoOp(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir()}

	migrated, err := MigrateIfStale(cfg)
	if err != nil {
		t.Fatalf("MigrateIfStale() error = %v, want nil", err)
	}
	if migrated {
		t.Fatal("MigrateIfStale() migrated = true for a home with no models.json, want false")
	}
	if _, err := os.Stat(cfg.ModelsPath()); !os.IsNotExist(err) {
		t.Fatalf("MigrateIfStale() created models.json on a never-installed home, want it to stay absent (err = %v)", err)
	}
}

// TestMigrateIfStale_CurrentSchema_NoOp guards that migration never touches an already-current
// file (no needless backup churn).
func TestMigrateIfStale_CurrentSchema_NoOp(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir()}
	if err := SaveModels(cfg, modelconfig.Defaults()); err != nil {
		t.Fatalf("SaveModels() error = %v", err)
	}

	migrated, err := MigrateIfStale(cfg)
	if err != nil {
		t.Fatalf("MigrateIfStale() error = %v, want nil", err)
	}
	if migrated {
		t.Fatal("MigrateIfStale() migrated = true for an already-current models.json, want false")
	}
	if _, err := os.Stat(cfg.ModelsPath() + ".bak"); !os.IsNotExist(err) {
		t.Fatalf("MigrateIfStale() created a .bak backup for an already-current file, want none (err = %v)", err)
	}
}

// TestMigrateIfStale_StaleFile_BacksUpThenRegenerates guards the confirmed migration contract: a
// stale models.json is backed up verbatim to models.json.bak FIRST, then fully regenerated with
// new-taxonomy defaults — old per-phase overrides are never preserved/merged (D8: never clobber a
// working setup without a backup, but also never silently trust stale per-phase data).
func TestMigrateIfStale_StaleFile_BacksUpThenRegenerates(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir()}
	legacy := map[string]string{
		"orchestrator":   "haiku", // a real developer override we must NOT carry forward
		"prd_writer":     "opus",
		"architect":      "opus",
		"reviewer":       "opus",
		"memory_curator": "sonnet",
	}
	writeLegacyModelsFile(t, cfg, legacy)

	migrated, err := MigrateIfStale(cfg)
	if err != nil {
		t.Fatalf("MigrateIfStale() error = %v, want nil", err)
	}
	if !migrated {
		t.Fatal("MigrateIfStale() migrated = false for a stale legacy file, want true")
	}

	backupRaw, err := os.ReadFile(cfg.ModelsPath() + ".bak")
	if err != nil {
		t.Fatalf("ReadFile(models.json.bak) error = %v, want the stale file backed up", err)
	}
	var backedUp map[string]string
	if err := json.Unmarshal(backupRaw, &backedUp); err != nil {
		t.Fatalf("json.Unmarshal(models.json.bak) error = %v", err)
	}
	if !reflect.DeepEqual(backedUp, legacy) {
		t.Fatalf("models.json.bak = %#v, want the exact legacy content %#v", backedUp, legacy)
	}

	got, found, err := LoadModels(cfg)
	if err != nil {
		t.Fatalf("LoadModels() error = %v", err)
	}
	if !found {
		t.Fatal("LoadModels() found = false right after migration, want true")
	}
	if !reflect.DeepEqual(got, modelconfig.Defaults()) {
		t.Fatalf("LoadModels() after migration = %#v, want fresh new-taxonomy defaults %#v (old overrides must NOT be preserved)", got, modelconfig.Defaults())
	}

	stale, err := IsStale(cfg)
	if err != nil {
		t.Fatalf("IsStale() error = %v", err)
	}
	if stale {
		t.Fatal("IsStale() = true right after MigrateIfStale(), want false")
	}
}

// TestSaveModelsWithProfile_ThenLoadModelsWithProfile_RoundTrips guards design D2's persistence
// shape: the active profile name rides IN models.json as one optional top-level field, alongside
// the existing per-phase map — no second file, no schema_version bump.
func TestSaveModelsWithProfile_ThenLoadModelsWithProfile_RoundTrips(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir()}
	models := modelconfig.ResolveProfile(string(modelconfig.ProfileCostSaver)).Models

	if err := SaveModelsWithProfile(cfg, modelconfig.ProfileCostSaver, models); err != nil {
		t.Fatalf("SaveModelsWithProfile() error = %v", err)
	}

	gotProfile, gotModels, found, err := LoadModelsWithProfile(cfg)
	if err != nil {
		t.Fatalf("LoadModelsWithProfile() error = %v", err)
	}
	if !found {
		t.Fatal("LoadModelsWithProfile() found = false right after SaveModelsWithProfile(), want true")
	}
	if gotProfile != modelconfig.ProfileCostSaver {
		t.Fatalf("LoadModelsWithProfile() profile = %q, want %q", gotProfile, modelconfig.ProfileCostSaver)
	}
	if !reflect.DeepEqual(gotModels, models) {
		t.Fatalf("LoadModelsWithProfile() models = %#v, want %#v", gotModels, models)
	}
}

// TestSaveModelsWithProfile_WritesProfileField guards the exact on-disk shape:
// {"schema_version":2,"profile":"cost-saver","models":{...}}.
func TestSaveModelsWithProfile_WritesProfileField(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir()}
	if err := SaveModelsWithProfile(cfg, modelconfig.ProfileCostSaver, modelconfig.Defaults()); err != nil {
		t.Fatalf("SaveModelsWithProfile() error = %v", err)
	}

	raw, err := os.ReadFile(cfg.ModelsPath())
	if err != nil {
		t.Fatalf("ReadFile(models.json) error = %v", err)
	}
	var wrapper struct {
		SchemaVersion int    `json:"schema_version"`
		Profile       string `json:"profile"`
	}
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		t.Fatalf("json.Unmarshal(models.json) error = %v", err)
	}
	if wrapper.SchemaVersion != CurrentModelsSchemaVersion {
		t.Fatalf("models.json schema_version = %d, want %d (adding profile must NOT bump schema_version)", wrapper.SchemaVersion, CurrentModelsSchemaVersion)
	}
	if wrapper.Profile != string(modelconfig.ProfileCostSaver) {
		t.Fatalf("models.json profile = %q, want %q", wrapper.Profile, modelconfig.ProfileCostSaver)
	}
}

// TestSaveModels_OmitsProfileField guards that the existing (profile-unaware) SaveModels callers
// keep writing a file with no "profile" key at all, rather than an empty-string one — the field is
// omitempty, so a plain SaveModels() call must round-trip through LoadModelsWithProfile() as an
// absent profile.
func TestSaveModels_OmitsProfileField(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir()}
	if err := SaveModels(cfg, modelconfig.Defaults()); err != nil {
		t.Fatalf("SaveModels() error = %v", err)
	}

	raw, err := os.ReadFile(cfg.ModelsPath())
	if err != nil {
		t.Fatalf("ReadFile(models.json) error = %v", err)
	}
	var wrapper map[string]json.RawMessage
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		t.Fatalf("json.Unmarshal(models.json) error = %v", err)
	}
	if _, present := wrapper["profile"]; present {
		t.Fatalf("models.json has a %q key after plain SaveModels(), want it omitted entirely", "profile")
	}
}

// TestLoadModelsWithProfile_V2FileWithoutProfileField_ResolvesToBalanced_NoError guards back-compat
// (design D2): a valid v2 models.json written before this change (no "profile" key at all) must
// load cleanly with an empty ProfileName, which callers resolve to balanced via
// modelconfig.ResolveProfile("") — no error, and critically no migration/staleness is triggered by
// the missing field.
func TestLoadModelsWithProfile_V2FileWithoutProfileField_ResolvesToBalanced_NoError(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir()}
	// Simulate a pre-existing v2 file written by the OLD (profile-unaware) SaveModels — no
	// "profile" key present at all, exactly like models written before this change.
	if err := SaveModels(cfg, modelconfig.Defaults()); err != nil {
		t.Fatalf("SaveModels() error = %v", err)
	}

	gotProfile, gotModels, found, err := LoadModelsWithProfile(cfg)
	if err != nil {
		t.Fatalf("LoadModelsWithProfile() error = %v, want nil for a v2 file with no profile field", err)
	}
	if !found {
		t.Fatal("LoadModelsWithProfile() found = false for an existing v2 file, want true")
	}
	if resolved := modelconfig.ResolveProfile(string(gotProfile)); resolved.Name != modelconfig.ProfileBalanced {
		t.Fatalf("modelconfig.ResolveProfile(%q).Name = %q, want %q (missing profile field must resolve to balanced)", gotProfile, resolved.Name, modelconfig.ProfileBalanced)
	}
	if !reflect.DeepEqual(gotModels, modelconfig.Defaults()) {
		t.Fatalf("LoadModelsWithProfile() models = %#v, want %#v", gotModels, modelconfig.Defaults())
	}
}

// TestIsStale_ExistingV2FileWithoutProfileField_NotStale_NoBackup is the explicit regression guard
// the task calls for: adding the optional profile field must NOT make IsStale/MigrateIfStale treat
// a pre-existing v2-without-profile file as stale, and MigrateIfStale must not touch it (no .bak
// written).
func TestIsStale_ExistingV2FileWithoutProfileField_NotStale_NoBackup(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir()}
	if err := SaveModels(cfg, modelconfig.Defaults()); err != nil {
		t.Fatalf("SaveModels() error = %v", err)
	}

	stale, err := IsStale(cfg)
	if err != nil {
		t.Fatalf("IsStale() error = %v, want nil", err)
	}
	if stale {
		t.Fatal("IsStale() = true for an existing v2-without-profile models.json, want false")
	}

	migrated, err := MigrateIfStale(cfg)
	if err != nil {
		t.Fatalf("MigrateIfStale() error = %v, want nil", err)
	}
	if migrated {
		t.Fatal("MigrateIfStale() migrated = true for an existing v2-without-profile models.json, want false")
	}
	if _, err := os.Stat(cfg.ModelsPath() + ".bak"); !os.IsNotExist(err) {
		t.Fatalf("MigrateIfStale() created a .bak backup for a v2-without-profile file, want none (err = %v)", err)
	}
}

func writeLegacyModelsFile(t *testing.T, cfg Config, legacy map[string]string) {
	t.Helper()
	raw, err := json.Marshal(legacy)
	if err != nil {
		t.Fatalf("json.Marshal(legacy) error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(cfg.ModelsPath()), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(cfg.ModelsPath(), raw, 0o600); err != nil {
		t.Fatalf("WriteFile(legacy models.json) error = %v", err)
	}
}
