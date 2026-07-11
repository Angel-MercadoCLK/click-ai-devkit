package installer

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/modelconfig"
)

// CurrentModelsSchemaVersion is the schema_version SaveModels writes and LoadModels/IsStale
// compare against. It was introduced alongside the real-SDD-taxonomy realignment (the invented
// 5-phase taxonomy's models.json had no schema_version field at all — it was a bare flat map). Bump
// this whenever models.json's on-disk shape changes in a way old readers can't handle.
const CurrentModelsSchemaVersion = 2

// modelsFile is the on-disk shape SaveModels writes and LoadModels/IsStale read, wrapping the
// per-phase model map with a schema_version so a stale (pre-realignment or otherwise outdated)
// file can be detected without guessing from key names alone. Profile is an OPTIONAL top-level
// field (design D2): it carries the active orchestration profile name
// (modelconfig.ProfileName) so `click update`/`click doctor` can re-apply/report it, using the
// SAME file as the per-phase map rather than a second persistence path. It is intentionally
// omitempty and does NOT participate in schema_version — an existing v2 file written before this
// field existed still parses cleanly with Profile == "" (back-compat; see LoadModelsWithProfile).
type modelsFile struct {
	SchemaVersion int               `json:"schema_version"`
	Profile       string            `json:"profile,omitempty"`
	Models        map[string]string `json:"models"`
}

// SaveModels persists the resolved per-phase click-sdd model selection to cfg.ModelsPath(), so
// `click update` can re-apply the same --config flags and `click doctor` can report what's
// configured. It always writes the current schema_version and overwrites any previous file. It
// writes no profile field — equivalent to SaveModelsWithProfile(cfg, "", models). Kept alongside
// SaveModelsWithProfile so existing callers (install.go, update.go, configuremodels.go and their
// tests) keep compiling unchanged; PR2b/PR3 migrate them to the profile-aware form.
func SaveModels(cfg Config, models map[modelconfig.Phase]string) error {
	return SaveModelsWithProfile(cfg, "", models)
}

// SaveModelsWithProfile persists the resolved per-phase click-sdd model selection AND the active
// orchestration profile name to cfg.ModelsPath() in one file (design D2):
// {"schema_version":2,"profile":"cost-saver","models":{...}}. An empty profile name is written as
// an absent "profile" key (omitempty), matching what a pre-profile SaveModels call already
// produces.
func SaveModelsWithProfile(cfg Config, profile modelconfig.ProfileName, models map[modelconfig.Phase]string) error {
	data := modelsFile{
		SchemaVersion: CurrentModelsSchemaVersion,
		Profile:       string(profile),
		Models:        make(map[string]string, len(models)),
	}
	for phase, model := range models {
		data.Models[string(phase)] = model
	}
	if err := writeJSONFile(cfg.ModelsPath(), data); err != nil {
		return fmt.Errorf("installer: write models.json: %w", err)
	}
	return nil
}

// LoadModels reads the per-phase model selection written by SaveModels/SaveModelsWithProfile. It
// returns (nil, false, nil) when models.json doesn't exist yet (e.g. before the first
// `click install`), so callers can distinguish "never configured" from a real read/parse error.
// LoadModels does not itself detect or migrate a stale (pre-realignment) file — callers that care
// should check IsStale/MigrateIfStale first; a stale file simply round-trips through the current
// wrapper shape with an empty (or partial) Models map. It discards the profile field; callers that
// need it should use LoadModelsWithProfile.
func LoadModels(cfg Config) (map[modelconfig.Phase]string, bool, error) {
	_, models, found, err := LoadModelsWithProfile(cfg)
	return models, found, err
}

// LoadModelsWithProfile reads the per-phase model selection AND the active orchestration profile
// name written by SaveModelsWithProfile (or SaveModels, whose files simply have no profile key —
// design D2's back-compat guarantee: a pre-existing v2 file with no "profile" field loads cleanly
// with an empty ProfileName, which callers resolve to balanced via modelconfig.ResolveProfile("")).
// It returns (_, nil, false, nil) when models.json doesn't exist yet.
func LoadModelsWithProfile(cfg Config) (modelconfig.ProfileName, map[modelconfig.Phase]string, bool, error) {
	data, err := os.ReadFile(cfg.ModelsPath())
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil, false, nil
		}
		return "", nil, false, fmt.Errorf("installer: read models.json: %w", err)
	}
	var wrapper modelsFile
	if err := json.Unmarshal(data, &wrapper); err != nil {
		return "", nil, false, fmt.Errorf("installer: parse models.json: %w", err)
	}
	models := make(map[modelconfig.Phase]string, len(wrapper.Models))
	for k, v := range wrapper.Models {
		models[modelconfig.Phase(k)] = v
	}
	return modelconfig.ProfileName(wrapper.Profile), models, true, nil
}

// oldTaxonomyPhaseKeys are the five invented phase names models.json (and modelconfig.go) used
// before the real-SDD-taxonomy realignment. IsStale treats their presence as a staleness signal
// independent of schema_version, in case a future file ever carries a current-looking
// schema_version without having actually completed migration.
var oldTaxonomyPhaseKeys = map[string]bool{
	"orchestrator":   true,
	"prd_writer":     true,
	"architect":      true,
	"reviewer":       true,
	"memory_curator": true,
}

// IsStale reports whether cfg.ModelsPath() holds a pre-realignment or otherwise outdated
// models.json: either its schema_version is missing/lower than CurrentModelsSchemaVersion, or it
// still carries one of the old invented-taxonomy phase keys. A missing file is NOT stale — it just
// means defaults will be generated on the next install/update, which is a healthy state. This
// function never mutates the filesystem, so it is safe for `click doctor`'s read-only checks
// (NFR-012).
func IsStale(cfg Config) (bool, error) {
	data, err := os.ReadFile(cfg.ModelsPath())
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("installer: read models.json: %w", err)
	}
	var wrapper modelsFile
	if err := json.Unmarshal(data, &wrapper); err != nil {
		return false, fmt.Errorf("installer: parse models.json: %w", err)
	}
	if wrapper.SchemaVersion < CurrentModelsSchemaVersion {
		return true, nil
	}
	for key := range wrapper.Models {
		if oldTaxonomyPhaseKeys[key] {
			return true, nil
		}
	}
	return false, nil
}

// MigrateIfStale checks cfg.ModelsPath() and, if it holds a stale (pre-realignment or otherwise
// outdated) file, backs it up verbatim to models.json.bak FIRST, then fully regenerates
// models.json with modelconfig.Defaults() — confirmed migration behavior: old per-phase overrides
// are never preserved or merged, per D8 ("never clobber a working setup without a backup"). It is
// a safe no-op when models.json is absent or already current. Callers that must stay read-only
// (e.g. `click doctor`) should use IsStale instead — MigrateIfStale writes to disk.
func MigrateIfStale(cfg Config) (migrated bool, err error) {
	stale, err := IsStale(cfg)
	if err != nil {
		return false, err
	}
	if !stale {
		return false, nil
	}

	data, err := os.ReadFile(cfg.ModelsPath())
	if err != nil {
		return false, fmt.Errorf("installer: read stale models.json for backup: %w", err)
	}
	if err := os.WriteFile(cfg.ModelsPath()+".bak", data, 0o600); err != nil {
		return false, fmt.Errorf("installer: back up stale models.json: %w", err)
	}
	if err := SaveModels(cfg, modelconfig.Defaults()); err != nil {
		return false, fmt.Errorf("installer: regenerate models.json after migration: %w", err)
	}
	return true, nil
}
