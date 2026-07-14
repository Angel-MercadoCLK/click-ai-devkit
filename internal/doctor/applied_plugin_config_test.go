package doctor

import (
	"encoding/json"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/installer"
	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/modelconfig"
)

const appliedPluginConfigCheckName = "click-sdd applied plugin config"

// allExpectedAppliedOptions builds a fully-applied pluginConfigs options map — one entry per
// modelconfig.Phases ConfigKey() plus modelconfig.ProfileConfigKey — using modelconfig.Defaults()
// as filler values. It intentionally does NOT hardcode a key count so it stays correct if the
// phase taxonomy ever changes.
func allExpectedAppliedOptions() map[string]string {
	defaults := modelconfig.Defaults()
	options := map[string]string{modelconfig.ProfileConfigKey: "balanced"}
	for _, phase := range modelconfig.Phases {
		options[phase.ConfigKey()] = defaults[phase]
	}
	return options
}

func findCheck(t *testing.T, report Report, name string) CheckResult {
	t.Helper()
	for _, c := range report.Checks {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("Run() did not include a %q check", name)
	return CheckResult{}
}

// TestCheckAppliedPluginConfig_AllKeysPresent_ReportsHealthy covers the fully-healthy case: every
// expected key (derived from modelconfig.Phases + modelconfig.ProfileConfigKey) is present in the
// applied pluginConfigs options.
func TestCheckAppliedPluginConfig_AllKeysPresent_ReportsHealthy(t *testing.T) {
	cfg := installer.Config{ClaudeHome: t.TempDir()}
	seedAppliedPluginConfigOptions(t, cfg, allExpectedAppliedOptions())

	report := Run(cfg)

	check := findCheck(t, report, appliedPluginConfigCheckName)
	if !check.Healthy {
		t.Fatalf("checkAppliedPluginConfig reports unhealthy with all keys applied: %s", check.Detail)
	}
}

// TestCheckAppliedPluginConfig_IncidentReplay_MissingReviewKeys_ReportsUnhealthy replays the real
// incident this check closes live: click-sdd's plugin.json grew from 13 to 18 model-config phases
// (adding the 5 review_*_model userConfig keys), but because the plugin version never bumped,
// Claude Code cached a stale schema and silently DROPPED those 5 --config keys during sync — so
// the developer's real settings.json ended up with only 14 of the 19 expected keys, while
// checkModelsConfig (validating models.json, not the applied side) kept reporting healthy the
// whole time. This test seeds exactly that 14-of-19 state and asserts the new check catches it:
// FAIL, naming the missing keys, and pointing at `click update`.
func TestCheckAppliedPluginConfig_IncidentReplay_MissingReviewKeys_ReportsUnhealthy(t *testing.T) {
	cfg := installer.Config{ClaudeHome: t.TempDir()}

	applied := allExpectedAppliedOptions()
	missingKeys := []string{
		modelconfig.PhaseReviewRisk.ConfigKey(),
		modelconfig.PhaseReviewReadability.ConfigKey(),
		modelconfig.PhaseReviewReliability.ConfigKey(),
		modelconfig.PhaseReviewResilience.ConfigKey(),
		modelconfig.PhaseReviewRefuter.ConfigKey(),
	}
	for _, key := range missingKeys {
		delete(applied, key)
	}
	if len(applied) != 14 {
		t.Fatalf("test fixture holds %d applied keys, want 14 (19 expected - 5 dropped review keys)", len(applied))
	}
	seedAppliedPluginConfigOptions(t, cfg, applied)

	report := Run(cfg)

	check := findCheck(t, report, appliedPluginConfigCheckName)
	if check.Healthy {
		t.Fatal("checkAppliedPluginConfig reports healthy with 5 review_*_model keys missing, want unhealthy")
	}
	for _, key := range missingKeys {
		if !strings.Contains(check.Detail, key) {
			t.Errorf("checkAppliedPluginConfig Detail = %q, want it to name missing key %q", check.Detail, key)
		}
	}
	if !strings.Contains(check.Detail, "click update") {
		t.Errorf("checkAppliedPluginConfig Detail = %q, want it to suggest `click update`", check.Detail)
	}
}

// TestCheckAppliedPluginConfig_SettingsMissing_ReportsUnhealthy guards the "click install never
// ran" state: a fresh ClaudeHome with no settings.json at all must report unhealthy (not applied
// yet) without panicking.
func TestCheckAppliedPluginConfig_SettingsMissing_ReportsUnhealthy(t *testing.T) {
	cfg := installer.Config{ClaudeHome: t.TempDir()}

	report := Run(cfg)

	check := findCheck(t, report, appliedPluginConfigCheckName)
	if check.Healthy {
		t.Fatal("checkAppliedPluginConfig reports healthy on a fresh ClaudeHome with no settings.json, want unhealthy")
	}
}

// TestCheckAppliedPluginConfig_PluginConfigsAbsent_ReportsUnhealthy guards a settings.json that
// exists but has never gotten a pluginConfigs key at all.
func TestCheckAppliedPluginConfig_PluginConfigsAbsent_ReportsUnhealthy(t *testing.T) {
	cfg := installer.Config{ClaudeHome: t.TempDir()}
	if err := os.MkdirAll(cfg.ClaudeHome, 0o755); err != nil {
		t.Fatalf("MkdirAll(ClaudeHome) error = %v", err)
	}
	if err := os.WriteFile(cfg.SettingsPath(), []byte(`{"enabledPlugins":{}}`), 0o600); err != nil {
		t.Fatalf("WriteFile(SettingsPath) error = %v", err)
	}

	report := Run(cfg)

	check := findCheck(t, report, appliedPluginConfigCheckName)
	if check.Healthy {
		t.Fatal("checkAppliedPluginConfig reports healthy with no pluginConfigs key, want unhealthy")
	}
}

// TestCheckAppliedPluginConfig_MalformedJSON_ReportsUnhealthy guards against a panic on a corrupt
// settings.json: it must degrade to an unhealthy CheckResult, matching every other check.
func TestCheckAppliedPluginConfig_MalformedJSON_ReportsUnhealthy(t *testing.T) {
	cfg := installer.Config{ClaudeHome: t.TempDir()}
	if err := os.MkdirAll(cfg.ClaudeHome, 0o755); err != nil {
		t.Fatalf("MkdirAll(ClaudeHome) error = %v", err)
	}
	if err := os.WriteFile(cfg.SettingsPath(), []byte("{not valid json"), 0o600); err != nil {
		t.Fatalf("WriteFile(SettingsPath) error = %v", err)
	}

	report := Run(cfg)

	check := findCheck(t, report, appliedPluginConfigCheckName)
	if check.Healthy {
		t.Fatal("checkAppliedPluginConfig reports healthy for a malformed settings.json, want unhealthy")
	}
	if check.Detail == "" {
		t.Fatal("checkAppliedPluginConfig Detail is empty for a malformed settings.json")
	}
}

// TestExpectedClickSDDConfigKeys_DerivedFromModelconfigPhases guards against hardcoding the
// expected-key count (currently 19): it independently recomputes the expected set from
// modelconfig.Phases + modelconfig.ProfileConfigKey and asserts expectedClickSDDConfigKeys()
// returns exactly that set (order-independent). If a phase is ever added or removed from
// modelconfig.Phases without updating a hypothetical hardcoded literal, this test breaks —
// expectedClickSDDConfigKeys() must NOT hardcode a literal count or key list.
func TestExpectedClickSDDConfigKeys_DerivedFromModelconfigPhases(t *testing.T) {
	want := make([]string, 0, len(modelconfig.Phases)+1)
	want = append(want, modelconfig.ProfileConfigKey)
	for _, phase := range modelconfig.Phases {
		want = append(want, phase.ConfigKey())
	}
	sort.Strings(want)

	got := expectedClickSDDConfigKeys()
	sort.Strings(got)

	if len(got) != len(want) {
		t.Fatalf("expectedClickSDDConfigKeys() has %d keys, want %d (derived from modelconfig.Phases)", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expectedClickSDDConfigKeys() = %v, want %v", got, want)
		}
	}
}

// seedAppliedPluginConfigOptions writes settings.json with exactly the given options under
// pluginConfigs[installer.ClickSDDPluginID].options.
func seedAppliedPluginConfigOptions(t *testing.T, cfg installer.Config, options map[string]string) {
	t.Helper()
	optionsAny := make(map[string]any, len(options))
	for k, v := range options {
		optionsAny[k] = v
	}
	if err := os.MkdirAll(cfg.ClaudeHome, 0o755); err != nil {
		t.Fatalf("MkdirAll(ClaudeHome) error = %v", err)
	}
	data := map[string]any{
		"pluginConfigs": map[string]any{
			installer.ClickSDDPluginID: map[string]any{"options": optionsAny},
		},
	}
	raw, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("json.Marshal(pluginConfigs fixture) error = %v", err)
	}
	if err := os.WriteFile(cfg.SettingsPath(), raw, 0o600); err != nil {
		t.Fatalf("WriteFile(SettingsPath) error = %v", err)
	}
}
