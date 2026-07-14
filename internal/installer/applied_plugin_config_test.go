package installer

import (
	"os"
	"testing"
)

// TestAppliedClickSDDPluginConfig_AllKeysPresent guards the happy path: every key Claude Code
// actually wrote under pluginConfigs[ClickSDDPluginID].options round-trips as a plain
// map[string]string, and found is true.
func TestAppliedClickSDDPluginConfig_AllKeysPresent(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir()}
	seedAppliedPluginConfig(t, cfg, map[string]string{
		"orchestration_profile": "balanced",
		"explore_model":         "sonnet",
		"apply_model":           "sonnet",
	})

	options, found, err := AppliedClickSDDPluginConfig(cfg)
	if err != nil {
		t.Fatalf("AppliedClickSDDPluginConfig() error = %v", err)
	}
	if !found {
		t.Fatal("AppliedClickSDDPluginConfig() found = false, want true")
	}
	if len(options) != 3 || options["orchestration_profile"] != "balanced" || options["explore_model"] != "sonnet" || options["apply_model"] != "sonnet" {
		t.Fatalf("AppliedClickSDDPluginConfig() options = %#v, want the 3 seeded keys", options)
	}
}

// TestAppliedClickSDDPluginConfig_SettingsMissing guards the "click install never ran" state:
// no settings.json at all must be a graceful (nil, false, nil), never an error or panic.
func TestAppliedClickSDDPluginConfig_SettingsMissing(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir()}

	options, found, err := AppliedClickSDDPluginConfig(cfg)
	if err != nil {
		t.Fatalf("AppliedClickSDDPluginConfig() error = %v, want nil", err)
	}
	if found {
		t.Fatal("AppliedClickSDDPluginConfig() found = true on a fresh ClaudeHome, want false")
	}
	if options != nil {
		t.Fatalf("AppliedClickSDDPluginConfig() options = %#v, want nil", options)
	}
}

// TestAppliedClickSDDPluginConfig_PluginConfigsAbsent guards a settings.json that exists (e.g. it
// has enabledPlugins) but has never gotten a pluginConfigs key at all — still graceful, not found.
func TestAppliedClickSDDPluginConfig_PluginConfigsAbsent(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir()}
	if err := writeJSONFile(cfg.SettingsPath(), map[string]any{
		"enabledPlugins": map[string]bool{ClickSDDPluginID: true},
	}); err != nil {
		t.Fatalf("writeJSONFile(SettingsPath) error = %v", err)
	}

	options, found, err := AppliedClickSDDPluginConfig(cfg)
	if err != nil {
		t.Fatalf("AppliedClickSDDPluginConfig() error = %v, want nil", err)
	}
	if found {
		t.Fatal("AppliedClickSDDPluginConfig() found = true with no pluginConfigs key, want false")
	}
	if options != nil {
		t.Fatalf("AppliedClickSDDPluginConfig() options = %#v, want nil", options)
	}
}

// TestAppliedClickSDDPluginConfig_ClickSDDEntryAbsent guards a pluginConfigs map that exists but
// holds no click-sdd entry (e.g. only click-memory's config, which has no userConfig schema at
// all so it would never realistically appear, but the shape must still degrade gracefully).
func TestAppliedClickSDDPluginConfig_ClickSDDEntryAbsent(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir()}
	if err := writeJSONFile(cfg.SettingsPath(), map[string]any{
		"pluginConfigs": map[string]any{
			"some-other-plugin@click-ai-devkit": map[string]any{"options": map[string]any{"foo": "bar"}},
		},
	}); err != nil {
		t.Fatalf("writeJSONFile(SettingsPath) error = %v", err)
	}

	options, found, err := AppliedClickSDDPluginConfig(cfg)
	if err != nil {
		t.Fatalf("AppliedClickSDDPluginConfig() error = %v, want nil", err)
	}
	if found {
		t.Fatal("AppliedClickSDDPluginConfig() found = true with no click-sdd pluginConfigs entry, want false")
	}
	if options != nil {
		t.Fatalf("AppliedClickSDDPluginConfig() options = %#v, want nil", options)
	}
}

// TestAppliedClickSDDPluginConfig_MalformedJSON guards against a panic on a corrupt settings.json:
// it must return a real error, matching every other settings.json reader in this package.
func TestAppliedClickSDDPluginConfig_MalformedJSON(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir()}
	if err := os.MkdirAll(cfg.ClaudeHome, 0o755); err != nil {
		t.Fatalf("MkdirAll(ClaudeHome) error = %v", err)
	}
	if err := os.WriteFile(cfg.SettingsPath(), []byte("{not valid json"), 0o600); err != nil {
		t.Fatalf("WriteFile(SettingsPath) error = %v", err)
	}

	_, _, err := AppliedClickSDDPluginConfig(cfg)
	if err == nil {
		t.Fatal("AppliedClickSDDPluginConfig() error = nil for malformed settings.json, want error")
	}
}

// seedAppliedPluginConfig writes settings.json with exactly the given options under
// pluginConfigs[ClickSDDPluginID].options — the shape `claude plugin install
// click-sdd@click-ai-devkit --config k=v ...` produces for real.
func seedAppliedPluginConfig(t *testing.T, cfg Config, options map[string]string) {
	t.Helper()
	optionsAny := make(map[string]any, len(options))
	for k, v := range options {
		optionsAny[k] = v
	}
	data := map[string]any{
		"pluginConfigs": map[string]any{
			ClickSDDPluginID: map[string]any{"options": optionsAny},
		},
	}
	if err := writeJSONFile(cfg.SettingsPath(), data); err != nil {
		t.Fatalf("writeJSONFile(SettingsPath) error = %v", err)
	}
}
