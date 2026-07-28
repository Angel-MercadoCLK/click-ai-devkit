package installer

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// CurrentCodexModelProfileSchemaVersion is the schema_version SaveCodexModelProfile writes and
// future loaders can compare against. Bump it whenever the on-disk shape of Codex's model-profile.json
// changes in a way old readers cannot handle.
const CurrentCodexModelProfileSchemaVersion = 1

// CodexModelAssignment is one role bucket's resolved {model, effort} pair.
type CodexModelAssignment struct {
	Model  string `json:"model"`
	Effort string `json:"effort"`
}

// CodexModelTier is a named preset: a tier name plus the four role buckets and their assignments.
type CodexModelTier struct {
	Name  string                          `json:"name"`
	Roles map[string]CodexModelAssignment `json:"roles"`
}

// codexModelProfileFile is the on-disk shape written by SaveCodexModelProfile. It deliberately mirrors
// the versioned wrapper convention used by models.json (models.go) while carrying Codex-specific
// tier/role data instead of per-phase model names.
type codexModelProfileFile struct {
	SchemaVersion int                             `json:"schema_version"`
	Tier          string                          `json:"tier"`
	Roles         map[string]CodexModelAssignment `json:"roles"`
}

// codexModelTiers holds the three v1 preset tiers confirmed by design. It is package data so tests and
// the UI can read the same canonical assignments.
var codexModelTiers = map[string]CodexModelTier{
	"low-cost": {
		Name: "low-cost",
		Roles: map[string]CodexModelAssignment{
			"orquestador":  {Model: "gpt-5.6-sol", Effort: "low"},
			"razonamiento": {Model: "gpt-5.6-terra", Effort: "medium"},
			"codigo":       {Model: "gpt-5.6-terra", Effort: "medium"},
			"liviano":      {Model: "gpt-5.6-luna", Effort: "low"},
		},
	},
	"recommended": {
		Name: "recommended",
		Roles: map[string]CodexModelAssignment{
			"orquestador":  {Model: "gpt-5.6-sol", Effort: "low"},
			"razonamiento": {Model: "gpt-5.6-sol", Effort: "medium"},
			"codigo":       {Model: "gpt-5.6-terra", Effort: "medium"},
			"liviano":      {Model: "gpt-5.6-luna", Effort: "low"},
		},
	},
	"powerful": {
		Name: "powerful",
		Roles: map[string]CodexModelAssignment{
			"orquestador":  {Model: "gpt-5.6-sol", Effort: "low"},
			"razonamiento": {Model: "gpt-5.6-sol", Effort: "high"},
			"codigo":       {Model: "gpt-5.6-terra", Effort: "high"},
			"liviano":      {Model: "gpt-5.6-luna", Effort: "low"},
		},
	},
}

// CodexModelTierNames returns the three preset tier names in a stable order (low-cost, recommended,
// powerful) for UI rendering.
func CodexModelTierNames() []string {
	return []string{"low-cost", "recommended", "powerful"}
}

// CodexModelTierByName returns the preset tier with the given name and true, or false if the name is
// not one of the three presets.
func CodexModelTierByName(name string) (CodexModelTier, bool) {
	tier, ok := codexModelTiers[name]
	return tier, ok
}

// CodexModelTierByNameOrDefault returns the named preset tier, falling back to "recommended" when the
// name is empty or unrecognized. The default matches the non-interactive install path.
func CodexModelTierByNameOrDefault(name string) CodexModelTier {
	if tier, ok := codexModelTiers[name]; ok {
		return tier
	}
	return codexModelTiers["recommended"]
}

// CodexModelProfilePath stores Click's portable model/profile recommendation for Codex. It is
// deliberately outside Codex's native configuration files (config.toml / sdd-*.config.toml profiles):
// Codex model routing is not established by this repository, so this artifact is reference data only,
// not active configuration. It returns empty when CodexHome is unset so callers can no-op safely.
func (c Config) CodexModelProfilePath() string {
	if c.CodexHome == "" {
		return ""
	}
	return filepath.Join(c.CodexHome, "click-ai-devkit", "model-profile.json")
}

// SaveCodexModelProfile writes Click's resolved Codex tier recommendation under CodexHome. It validates
// the tier name, then persists a versioned JSON file containing the tier and its resolved role
// assignments. It is a no-op when CodexHome is empty and never touches Codex's own config.toml or any
// sdd-*.config.toml profile — this file is reference data only, not active configuration.
func SaveCodexModelProfile(cfg Config, tier string) error {
	if cfg.CodexHome == "" {
		return nil
	}
	resolved, ok := codexModelTiers[tier]
	if !ok {
		return fmt.Errorf("installer: unknown Codex model tier %q", tier)
	}
	data := codexModelProfileFile{
		SchemaVersion: CurrentCodexModelProfileSchemaVersion,
		Tier:          tier,
		Roles:         resolved.Roles,
	}
	if err := writeJSONFile(cfg.CodexModelProfilePath(), data); err != nil {
		return fmt.Errorf("installer: write Codex model profile: %w", err)
	}
	return nil
}

// LoadCodexModelProfile reads the previously saved Codex tier recommendation from CodexHome. It
// returns ("", false, nil) when the file does not exist yet, so callers can distinguish "never
// configured" from a real read/parse error. The returned tier is validated against the known presets;
// an unrecognized persisted tier falls back to "recommended" with found=true so a corrupt file never
// silently blocks update re-application.
func LoadCodexModelProfile(cfg Config) (string, bool, error) {
	if cfg.CodexHome == "" {
		return "", false, nil
	}
	data, err := os.ReadFile(cfg.CodexModelProfilePath())
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("installer: read Codex model profile: %w", err)
	}
	var wrapper codexModelProfileFile
	if err := json.Unmarshal(data, &wrapper); err != nil {
		return "", false, fmt.Errorf("installer: parse Codex model profile: %w", err)
	}
	if _, ok := codexModelTiers[wrapper.Tier]; !ok {
		return "recommended", true, nil
	}
	return wrapper.Tier, true, nil
}

// RemoveCodexModelProfile removes Click's own portable model-profile.json for Codex — the only file
// SaveCodexModelProfile writes. It is deliberately offline and idempotent: a missing file (or an
// installer with no CodexHome) is a silent no-op, mirroring RemoveOpenClawModelProfile's reversal
// contract. It never touches Codex's OWN native configuration (config.toml / the user's
// ~/.codex/sdd-*.config.toml profiles): those are user-owned and out of scope for uninstall.
func RemoveCodexModelProfile(cfg Config) error {
	if cfg.CodexHome == "" {
		return nil
	}
	if err := os.Remove(cfg.CodexModelProfilePath()); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("installer: remove Codex model profile: %w", err)
	}
	return nil
}
