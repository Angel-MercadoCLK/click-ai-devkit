package installer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	toml "github.com/pelletier/go-toml/v2"
)

// parseCodexConfig round-trip-parses the on-disk config.toml back into a map so tests can assert on
// SEMANTICS (the effective key/value tree) rather than exact formatting or quote style — the whole
// point of moving to a real TOML library is that output is always valid TOML, even if its
// whitespace/quote style differs from the input. A parse failure here means ConfigureCodexModel
// produced invalid TOML, which is itself a test failure.
func parseCodexConfig(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	config := map[string]any{}
	if err := toml.Unmarshal(data, &config); err != nil {
		t.Fatalf("config.toml is not valid TOML after ConfigureCodexModel: %v\ncontent:\n%s", err, data)
	}
	return config
}

// TestConfigureCodexModel_EmptyFile_CreatesValidTOMLWithModel proves the empty/absent case: with no
// config.toml at all, ConfigureCodexModel creates one that is valid TOML and has model set.
func TestConfigureCodexModel_EmptyFile_CreatesValidTOMLWithModel(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "config.toml")

	if err := ConfigureCodexModel(home, "gpt-5.6"); err != nil {
		t.Fatalf("ConfigureCodexModel() error = %v", err)
	}
	config := parseCodexConfig(t, path)
	if config["model"] != "gpt-5.6" {
		t.Fatalf("model = %#v, want %q", config["model"], "gpt-5.6")
	}
}

// TestConfigureCodexModel_OnlyTables_AddsRootModelAndPreservesTables proves a file with only
// [tables] (no root model) gains a valid root model while every table survives.
func TestConfigureCodexModel_OnlyTables_AddsRootModelAndPreservesTables(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "config.toml")
	before := "[profiles.fast]\nmodel = \"table-model\"\nsandbox_mode = \"read-only\"\n"
	if err := os.WriteFile(path, []byte(before), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := ConfigureCodexModel(home, "gpt-5.6"); err != nil {
		t.Fatalf("ConfigureCodexModel() error = %v", err)
	}

	config := parseCodexConfig(t, path)
	if config["model"] != "gpt-5.6" {
		t.Fatalf("root model = %#v, want %q", config["model"], "gpt-5.6")
	}
	fast := nestedTable(t, config, "profiles", "fast")
	if fast["model"] != "table-model" {
		t.Fatalf("[profiles.fast] model = %#v, want %q preserved", fast["model"], "table-model")
	}
	if fast["sandbox_mode"] != "read-only" {
		t.Fatalf("[profiles.fast] sandbox_mode = %#v, want %q preserved", fast["sandbox_mode"], "read-only")
	}
}

// TestConfigureCodexModel_ReplacesRootModel_PreservesUnrelatedKeysAndTables proves the common case:
// an existing root model line is replaced, and unrelated root keys AND a table-scoped model are
// left untouched.
func TestConfigureCodexModel_ReplacesRootModel_PreservesUnrelatedKeysAndTables(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "config.toml")
	before := strings.Join([]string{
		"# Codex native config",
		"model = \"gpt-5.5\"",
		"approval_policy = \"on-request\"",
		"",
		"[profiles.fast]",
		"model = \"table-model\"",
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(before), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := ConfigureCodexModel(home, "gpt-5.6"); err != nil {
		t.Fatalf("ConfigureCodexModel() error = %v", err)
	}

	config := parseCodexConfig(t, path)
	if config["model"] != "gpt-5.6" {
		t.Fatalf("root model = %#v, want %q", config["model"], "gpt-5.6")
	}
	if config["approval_policy"] != "on-request" {
		t.Fatalf("approval_policy = %#v, want %q preserved", config["approval_policy"], "on-request")
	}
	fast := nestedTable(t, config, "profiles", "fast")
	if fast["model"] != "table-model" {
		t.Fatalf("[profiles.fast] model = %#v, want %q (table-scoped model must NOT be touched as root)", fast["model"], "table-model")
	}
}

// TestConfigureCodexModel_TableScopedModel_NotTouchedAsRoot proves a `model =` that exists ONLY
// inside a [table] (with no root model) is never mistaken for the root model: setting the root model
// adds a distinct root key and leaves the table's model alone.
func TestConfigureCodexModel_TableScopedModel_NotTouchedAsRoot(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "config.toml")
	before := "[server]\nmodel = \"do-not-touch\"\n"
	if err := os.WriteFile(path, []byte(before), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := ConfigureCodexModel(home, "gpt-5.6"); err != nil {
		t.Fatalf("ConfigureCodexModel() error = %v", err)
	}

	config := parseCodexConfig(t, path)
	if config["model"] != "gpt-5.6" {
		t.Fatalf("root model = %#v, want %q", config["model"], "gpt-5.6")
	}
	server := nestedTable(t, config, "server")
	if server["model"] != "do-not-touch" {
		t.Fatalf("[server] model = %#v, want %q untouched", server["model"], "do-not-touch")
	}
}

// TestConfigureCodexModel_MultiLineArray_NoErrorAndPreserved proves the previous hand-rolled editor's
// hard-failure case (a multi-line array) is now handled gracefully: no error, and the array is
// preserved intact while the root model is updated.
func TestConfigureCodexModel_MultiLineArray_NoErrorAndPreserved(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "config.toml")
	before := strings.Join([]string{
		"model = \"gpt-5.5\"",
		"trusted = [",
		"  \"one\",",
		"  \"two\",",
		"]",
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(before), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := ConfigureCodexModel(home, "gpt-5.6"); err != nil {
		t.Fatalf("ConfigureCodexModel() error = %v, want a multi-line array to be handled without error", err)
	}

	config := parseCodexConfig(t, path)
	if config["model"] != "gpt-5.6" {
		t.Fatalf("root model = %#v, want %q", config["model"], "gpt-5.6")
	}
	trusted, ok := config["trusted"].([]any)
	if !ok || len(trusted) != 2 || trusted[0] != "one" || trusted[1] != "two" {
		t.Fatalf("trusted = %#v, want [\"one\" \"two\"] preserved", config["trusted"])
	}
}

// TestConfigureCodexModel_QuotedKeyModel_SingleDefinitionNoDuplicate proves that a quoted-key
// existing `"model"` results in a SINGLE model definition, not an invalid duplicate: TOML treats
// `"model"` and `model` as the same key, so the round-tripped output has exactly one, and it parses
// (a duplicate key would make the file invalid TOML and fail parseCodexConfig).
func TestConfigureCodexModel_QuotedKeyModel_SingleDefinitionNoDuplicate(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "config.toml")
	before := "\"model\" = \"quoted-old\"\napproval_policy = \"on-request\"\n"
	if err := os.WriteFile(path, []byte(before), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := ConfigureCodexModel(home, "gpt-5.6"); err != nil {
		t.Fatalf("ConfigureCodexModel() error = %v", err)
	}

	config := parseCodexConfig(t, path)
	if config["model"] != "gpt-5.6" {
		t.Fatalf("model = %#v, want a single %q definition", config["model"], "gpt-5.6")
	}
	if config["approval_policy"] != "on-request" {
		t.Fatalf("approval_policy = %#v, want %q preserved", config["approval_policy"], "on-request")
	}
}

// TestConfigureCodexModel_EmptyModelReturnsGuidanceAndLeavesFileUntouched keeps the non-empty-model
// validation contract: an empty model is rejected with actionable guidance and never touches the
// file.
func TestConfigureCodexModel_EmptyModelReturnsGuidanceAndLeavesFileUntouched(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "config.toml")
	before := "model = \"gpt-5.5\"\napproval_policy = \"on-request\"\n"
	if err := os.WriteFile(path, []byte(before), 0o600); err != nil {
		t.Fatal(err)
	}

	err := ConfigureCodexModel(home, "")
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "codex") || !strings.Contains(strings.ToLower(err.Error()), "explicit") {
		t.Fatalf("error = %v, want actionable explicit-model guidance", err)
	}
	after, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(after) != before {
		t.Fatalf("config.toml mutated on empty model\nbefore: %q\nafter:  %q", before, string(after))
	}
}

// TestConfigureCodexModel_ModelWithLineBreakRejected keeps the reject-line-breaks validation.
func TestConfigureCodexModel_ModelWithLineBreakRejected(t *testing.T) {
	home := t.TempDir()
	err := ConfigureCodexModel(home, "gpt-5.6\ninjected = true")
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "line break") {
		t.Fatalf("error = %v, want a line-break rejection", err)
	}
	if _, statErr := os.Stat(filepath.Join(home, "config.toml")); !os.IsNotExist(statErr) {
		t.Fatalf("Stat(config.toml) = %v, want no file written on rejected model", statErr)
	}
}

// TestConfigureCodexModel_InvalidTOMLIsAtomic proves genuinely invalid input (an unterminated basic
// string) is rejected with a TOML parse error and leaves the file byte-for-byte untouched — the
// library never partially writes.
func TestConfigureCodexModel_InvalidTOMLIsAtomic(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "config.toml")
	before := "model = \"gpt-5.5\n[profiles.fast]\nsandbox_mode = \"read-only\"\n"
	if err := os.WriteFile(path, []byte(before), 0o600); err != nil {
		t.Fatal(err)
	}

	err := ConfigureCodexModel(home, "gpt-5.6")
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "toml") {
		t.Fatalf("error = %v, want TOML parse failure", err)
	}
	after, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(after) != before {
		t.Fatalf("config.toml mutated after parse failure\nbefore: %q\nafter:  %q", before, string(after))
	}
}

// nestedTable descends config[keys[0]][keys[1]]... asserting each level is a table
// (map[string]any). go-toml/v2 unmarshals nested tables into map[string]any, so this mirrors how the
// round-tripped config is shaped.
func nestedTable(t *testing.T, config map[string]any, keys ...string) map[string]any {
	t.Helper()
	current := config
	for _, key := range keys {
		next, ok := current[key].(map[string]any)
		if !ok {
			t.Fatalf("expected table at %q, got %#v", key, current[key])
		}
		current = next
	}
	return current
}
