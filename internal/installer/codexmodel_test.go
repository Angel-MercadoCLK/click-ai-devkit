package installer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigureCodexModel_ExplicitlyUpdatesRootModelOnly(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "config.toml")
	before := strings.Join([]string{
		"# Codex native config",
		"model = \"gpt-5.5\"",
		"approval_policy = \"on-request\"",
		"",
		"[profiles.fast]",
		"model = \"table-model\"",
		"sandbox_mode = \"read-only\"",
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(before), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ConfigureCodexModel(home, "gpt-5.6"); err != nil {
		t.Fatalf("ConfigureCodexModel() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	if !strings.Contains(got, "model = \"gpt-5.6\"") {
		t.Fatalf("config.toml = %q, root model not updated", got)
	}
	if !strings.Contains(got, "approval_policy = \"on-request\"") {
		t.Fatalf("config.toml = %q, unrelated root config missing", got)
	}
	if !strings.Contains(got, "[profiles.fast]\nmodel = \"table-model\"") {
		t.Fatalf("config.toml = %q, table-scoped model should stay unchanged", got)
	}
	if strings.Count(got, "model =") != 2 {
		t.Fatalf("config.toml = %q, expected exactly two model keys (root + table)", got)
	}
}

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
