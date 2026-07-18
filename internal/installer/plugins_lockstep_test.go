package installer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/modelconfig"
)

// phasesWithoutDedicatedSkill lists modelconfig.Phase values that are deliberately EXEMPT from
// having their own `plugins/click-sdd/skills/<phase>/SKILL.md` directory. "default" is a catch-all
// model assignment for delegation that isn't covered by a specific SDD phase workflow, and the
// review-* phases are review roles consumed by the review/orchestrator flow rather than standalone
// click-sdd skills. Any phase added to modelconfig.Phases in the future must either get a skill
// directory or be added here explicitly — this test never silently skips a phase.
var phasesWithoutDedicatedSkill = map[modelconfig.Phase]bool{
	modelconfig.PhaseReviewRisk:        true,
	modelconfig.PhaseReviewReadability: true,
	modelconfig.PhaseReviewReliability: true,
	modelconfig.PhaseReviewResilience:  true,
	modelconfig.PhaseReviewRefuter:     true,
	modelconfig.PhaseDefault:           true,
}

// TestClickSDDSkills_LockstepWithModelconfigPhases guards the taxonomy realignment (interactive-
// menu-and-model-taxonomy, Work Unit 3): every phase modelconfig.Phases lists must have a matching
// `plugins/click-sdd/skills/<phase>/SKILL.md` file, except phases explicitly exempted in
// phasesWithoutDedicatedSkill. This prevents modelconfig.go and the actual plugin skill content from
// drifting apart again the way the pre-realignment 5-phase taxonomy did.
func TestClickSDDSkills_LockstepWithModelconfigPhases(t *testing.T) {
	for _, phase := range modelconfig.Phases {
		if phasesWithoutDedicatedSkill[phase] {
			continue
		}
		skillPath := filepath.Join("..", "..", "plugins", "click-sdd", "skills", string(phase), "SKILL.md")
		data, err := os.ReadFile(skillPath)
		if err != nil {
			t.Errorf("phase %q: expected skill file %s to exist: %v", phase, skillPath, err)
			continue
		}
		if len(data) == 0 {
			t.Errorf("phase %q: skill file %s is empty", phase, skillPath)
		}
	}
}

// TestClickSDDSkills_NoOrphanPhaseDirectories is the inverse guard: every directory under
// plugins/click-sdd/skills/ that is named after a would-be phase string must correspond to a real
// entry in modelconfig.Phases (or be explicitly exempted). This catches a stale/renamed skill
// directory left behind after a future taxonomy change. Non-phase-shaped directories (like
// agent-builder, which is a general-purpose meta-skill, not an SDD phase) are allowed and skipped.
func TestClickSDDSkills_NoOrphanPhaseDirectories(t *testing.T) {
	knownPhases := map[string]bool{}
	for _, phase := range modelconfig.Phases {
		knownPhases[string(phase)] = true
	}
	// Directories that intentionally exist under skills/ but are not one of modelconfig.Phases.
	nonPhaseSkillDirs := map[string]bool{
		"agent-builder": true,
		// _shared holds cross-phase reference docs (e.g. result-contract.md), not a phase skill.
		"_shared": true,
	}

	skillsDir := filepath.Join("..", "..", "plugins", "click-sdd", "skills")
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		t.Fatalf("ReadDir(%s) error = %v", skillsDir, err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if nonPhaseSkillDirs[name] {
			continue
		}
		if !knownPhases[name] {
			t.Errorf("skills/%s does not correspond to any phase in modelconfig.Phases and is not listed in nonPhaseSkillDirs", name)
		}
	}
}

// TestClickSDDPluginJSON_ConfigKeysMatchModelconfigPhasesExactly guards the other half of the
// taxonomy lockstep: plugins/click-sdd/.claude-plugin/plugin.json's userConfig keys must be exactly
// the set of ConfigKey() values for modelconfig.Phases, PLUS modelconfig.ProfileConfigKey (design
// D3's "orchestration_profile" field, coexisting alongside the 18 per-phase fields as just another
// --config key) — no missing phase, no leftover/stale key from an old taxonomy or a name that isn't
// either the profile key or a real phase's ConfigKey(). TestSyncMarketplacePlugins_
// PassesPerPhaseConfigFlagsForClickSDD (in plugins_config_test.go) already guards that
// SyncMarketplacePlugins emits the right --config flags from modelconfig.Phases directly; it does
// not read plugin.json off disk, so this test is not a duplicate — it is the only test that verifies
// the on-disk plugin.json manifest itself stays in lockstep with modelconfig.Phases.
func TestClickSDDPluginJSON_ConfigKeysMatchModelconfigPhasesExactly(t *testing.T) {
	data := mustReadRepoFile(t, "plugins", "click-sdd", ".claude-plugin", "plugin.json")

	var manifest struct {
		UserConfig map[string]json.RawMessage `json:"userConfig"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("plugin.json parse error = %v", err)
	}

	wantKeys := map[string]bool{modelconfig.ProfileConfigKey: true}
	for _, phase := range modelconfig.Phases {
		wantKeys[phase.ConfigKey()] = true
	}

	for key := range manifest.UserConfig {
		if !wantKeys[key] {
			t.Errorf("plugin.json userConfig has stale/unknown key %q not produced by any modelconfig.Phases entry", key)
		}
	}
	for key := range wantKeys {
		if _, ok := manifest.UserConfig[key]; !ok {
			t.Errorf("plugin.json userConfig is missing key %q for a phase in modelconfig.Phases", key)
		}
	}
}
