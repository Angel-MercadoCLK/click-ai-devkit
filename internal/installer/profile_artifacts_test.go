package installer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/modelconfig"
)

// TestSaveLoadProfileArtifact_RoundTrips guards the generic RuntimeProfile JSON artifact this file
// salvages as substrate for the separate agent-builder-flow change (design's profile_artifacts.go,
// re-keyed onto PR2a's RuntimeProfile{Name, Models} shape). It is UNWIRED to any UI in this change.
func TestSaveLoadProfileArtifact_RoundTrips(t *testing.T) {
	t.Setenv("CLICK_CLAUDE_HOME", t.TempDir())
	claudeHome, err := ResolveClaudeHome()
	if err != nil {
		t.Fatalf("ResolveClaudeHome() error = %v", err)
	}
	cfg := Config{ClaudeHome: claudeHome}

	profile := modelconfig.RuntimeProfile{
		Name: "my-custom-profile",
		Models: map[modelconfig.Phase]string{
			modelconfig.PhaseExplore: "opus",
			modelconfig.PhaseApply:   "haiku",
		},
	}

	if err := SaveProfileArtifact(cfg, profile); err != nil {
		t.Fatalf("SaveProfileArtifact() error = %v", err)
	}

	path := cfg.ProfileArtifactPath(string(profile.Name))
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected artifact file at %s: %v", path, err)
	}

	got, err := LoadProfileArtifact(cfg, profile.Name)
	if err != nil {
		t.Fatalf("LoadProfileArtifact() error = %v", err)
	}
	if got.Name != profile.Name {
		t.Errorf("Name = %q, want %q", got.Name, profile.Name)
	}
	if len(got.Models) != len(profile.Models) {
		t.Fatalf("Models = %#v, want %#v", got.Models, profile.Models)
	}
	for phase, model := range profile.Models {
		if got.Models[phase] != model {
			t.Errorf("Models[%q] = %q, want %q", phase, got.Models[phase], model)
		}
	}
}

// TestLoadProfileArtifact_MissingFile confirms a never-saved profile artifact surfaces a real error
// (this is not a "found/not found" bool API like LoadModelsWithProfile — callers only ask for an
// artifact they expect to already exist).
func TestLoadProfileArtifact_MissingFile(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir()}
	if _, err := LoadProfileArtifact(cfg, "never-saved"); err == nil {
		t.Fatal("LoadProfileArtifact() error = nil, want non-nil for a never-saved artifact")
	}
}

// TestLoadProfileArtifact_NameMismatchIsRejected guards against a hand-edited or corrupted artifact
// file whose embedded name doesn't match the filename it was loaded from.
func TestLoadProfileArtifact_NameMismatchIsRejected(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir()}
	if err := SaveProfileArtifact(cfg, modelconfig.RuntimeProfile{Name: "alpha", Models: map[modelconfig.Phase]string{}}); err != nil {
		t.Fatalf("SaveProfileArtifact() error = %v", err)
	}
	// Overwrite on disk with a mismatched embedded name, simulating a corrupted/hand-edited file.
	path := cfg.ProfileArtifactPath("alpha")
	if err := os.WriteFile(path, []byte(`{"Name":"beta","Models":{}}`), 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}
	if _, err := LoadProfileArtifact(cfg, "alpha"); err == nil {
		t.Fatal("LoadProfileArtifact() error = nil, want non-nil for a name/filename mismatch")
	}
}

// TestValidateArtifactName_RejectsBadNames guards SaveProfileArtifact/SaveMarkdownAgent/
// LoadProfileArtifact against path-traversal or otherwise unsafe artifact names, since the name
// becomes part of a filesystem path (cfg.ProfileArtifactPath/cfg.ProfileAgentsDir).
func TestValidateArtifactName_RejectsBadNames(t *testing.T) {
	badNames := []string{"", "..", "../escape", "has spaces", "Has-Upper", "trailing-", "-leading", "a/b", `a\b`}
	for _, name := range badNames {
		if err := validateArtifactName("profile", name); err == nil {
			t.Errorf("validateArtifactName(%q) error = nil, want non-nil", name)
		}
	}
	goodNames := []string{"a", "balanced", "cost-saver", "my-profile-2"}
	for _, name := range goodNames {
		if err := validateArtifactName("profile", name); err != nil {
			t.Errorf("validateArtifactName(%q) error = %v, want nil", name, err)
		}
	}
}

// TestRenderMarkdownAgent_GoldenOutput guards RenderMarkdownAgent's deterministic Claude Code
// sub-agent markdown shape for a fixed input — the builder UI/LLM skill owns content quality, this
// only owns syntax/identity determinism.
func TestRenderMarkdownAgent_GoldenOutput(t *testing.T) {
	agent := MarkdownAgent{
		Name:         "release-notes-writer",
		Description:  "Drafts release notes from merged PRs.",
		Model:        "sonnet",
		Tools:        "Read, Grep",
		Role:         "You summarize merged PRs into release notes.",
		Workflow:     "1. Read the PR list.\n2. Group by area.\n3. Write the notes.",
		HardRules:    "Never invent a PR that wasn't merged.",
		OutputFormat: "Markdown bullet list grouped by area.",
	}

	got, err := RenderMarkdownAgent(agent)
	if err != nil {
		t.Fatalf("RenderMarkdownAgent() error = %v", err)
	}

	want := "---\n" +
		"name: release-notes-writer\n" +
		"description: Drafts release notes from merged PRs.\n" +
		"model: sonnet\n" +
		"tools: Read, Grep\n" +
		"---\n\n" +
		"# Role\n" +
		"You summarize merged PRs into release notes.\n\n" +
		"# Workflow\n" +
		"1. Read the PR list.\n2. Group by area.\n3. Write the notes.\n\n" +
		"# Hard Rules\n" +
		"Never invent a PR that wasn't merged.\n\n" +
		"# Output Format\n" +
		"Markdown bullet list grouped by area.\n\n"

	if got != want {
		t.Fatalf("RenderMarkdownAgent() =\n%q\nwant\n%q", got, want)
	}
}

// TestRenderMarkdownAgent_RejectsBadName guards the same name validation SaveProfileArtifact uses.
func TestRenderMarkdownAgent_RejectsBadName(t *testing.T) {
	if _, err := RenderMarkdownAgent(MarkdownAgent{Name: "Not Valid"}); err == nil {
		t.Fatal("RenderMarkdownAgent() error = nil, want non-nil for an invalid agent name")
	}
}

// TestSaveMarkdownAgent_WritesUnderProfileAgentsDir guards the substrate path SaveMarkdownAgent
// writes to (cfg.ProfileAgentsDir), unwired to any UI in this change.
func TestSaveMarkdownAgent_WritesUnderProfileAgentsDir(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir()}
	agent := MarkdownAgent{
		Name:         "helper",
		Description:  "d",
		Model:        "haiku",
		Tools:        "Read",
		Role:         "r",
		Workflow:     "w",
		HardRules:    "h",
		OutputFormat: "o",
	}

	path, err := SaveMarkdownAgent(cfg, "cost-saver", agent)
	if err != nil {
		t.Fatalf("SaveMarkdownAgent() error = %v", err)
	}

	wantPath := filepath.Join(cfg.ProfileAgentsDir("cost-saver"), "helper.md")
	if path != wantPath {
		t.Fatalf("path = %q, want %q", path, wantPath)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file at %s: %v", path, err)
	}
}
