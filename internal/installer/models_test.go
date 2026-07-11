package installer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
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
		modelconfig.PhaseOrchestrator:  "sonnet",
		modelconfig.PhasePRDWriter:     "opus",
		modelconfig.PhaseArchitect:     "haiku",
		modelconfig.PhaseReviewer:      "opus",
		modelconfig.PhaseMemoryCurator: "sonnet",
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

func TestSaveProfile_ThenLoadProfile_RoundTripsDefault(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir()}
	profile := modelconfig.DefaultProfile()

	if err := SaveProfile(cfg, profile); err != nil {
		t.Fatalf("SaveProfile() error = %v", err)
	}

	got, found, err := LoadProfile(cfg)
	if err != nil {
		t.Fatalf("LoadProfile() error = %v", err)
	}
	if !found {
		t.Fatal("LoadProfile() found = false right after SaveProfile(), want true")
	}
	if got.Name != modelconfig.ProfileDefault {
		t.Fatalf("LoadProfile().Name = %q, want %q", got.Name, modelconfig.ProfileDefault)
	}
}

func TestLoadProfile_NoFileYet_ReturnsDefaultNotFound(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir()}

	profile, found, err := LoadProfile(cfg)
	if err != nil {
		t.Fatalf("LoadProfile() error = %v, want nil", err)
	}
	if found {
		t.Fatal("LoadProfile() found = true before any SaveProfile() call, want false")
	}
	if profile.Name != modelconfig.ProfileDefault {
		t.Fatalf("LoadProfile() fallback profile = %q, want %q", profile.Name, modelconfig.ProfileDefault)
	}
}

func TestSaveModels_Overwrites(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir()}

	first := map[modelconfig.Phase]string{modelconfig.PhaseOrchestrator: "opus", modelconfig.PhasePRDWriter: "opus", modelconfig.PhaseArchitect: "opus", modelconfig.PhaseReviewer: "opus", modelconfig.PhaseMemoryCurator: "sonnet"}
	if err := SaveModels(cfg, first); err != nil {
		t.Fatalf("SaveModels(first) error = %v", err)
	}

	second := map[modelconfig.Phase]string{modelconfig.PhaseOrchestrator: "haiku", modelconfig.PhasePRDWriter: "haiku", modelconfig.PhaseArchitect: "haiku", modelconfig.PhaseReviewer: "haiku", modelconfig.PhaseMemoryCurator: "haiku"}
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

func TestConfig_ProfilePath(t *testing.T) {
	cfg := Config{ClaudeHome: filepath.Join("C:", "fake-home")}
	want := filepath.Join("C:", "fake-home", "click-ai-devkit", "profile.json")
	if got := cfg.ProfilePath(); got != want {
		t.Fatalf("Config.ProfilePath() = %q, want %q", got, want)
	}
}

func TestSaveProfileArtifact_ThenLoadProfile_ResolvesCustomProfile(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir()}
	profile := modelconfig.RuntimeProfile{
		Name:        modelconfig.ProfileName("claims-review"),
		Title:       "Claims review",
		Description: "Custom profile for claims review work.",
		Models: map[modelconfig.Phase]string{
			modelconfig.PhaseOrchestrator:  "sonnet",
			modelconfig.PhasePRDWriter:     "haiku",
			modelconfig.PhaseArchitect:     "opus",
			modelconfig.PhaseReviewer:      "sonnet",
			modelconfig.PhaseMemoryCurator: "sonnet",
		},
		Delegation: modelconfig.DelegationPolicy{
			SimpleInlineAllowed: false,
			EngramRequired:      true,
			MandatoryDelegationTriggers: []string{
				"claims_context",
				"multi_file_implementation",
			},
		},
		PhaseChain: []string{"explore", "prd", "design", "tasks", "code", "review", "memory"},
	}

	if err := SaveProfileArtifact(cfg, profile); err != nil {
		t.Fatalf("SaveProfileArtifact() error = %v", err)
	}
	if err := SaveProfile(cfg, profile); err != nil {
		t.Fatalf("SaveProfile() error = %v", err)
	}

	got, found, err := LoadProfile(cfg)
	if err != nil {
		t.Fatalf("LoadProfile() error = %v", err)
	}
	if !found {
		t.Fatal("LoadProfile() found = false after custom profile was saved and activated, want true")
	}
	if got.Name != profile.Name {
		t.Fatalf("LoadProfile().Name = %q, want %q", got.Name, profile.Name)
	}
	if got.Models[modelconfig.PhasePRDWriter] != "haiku" {
		t.Fatalf("LoadProfile().Models[prd_writer] = %q, want haiku", got.Models[modelconfig.PhasePRDWriter])
	}
	if got.Delegation.SimpleInlineAllowed {
		t.Fatal("LoadProfile().Delegation.SimpleInlineAllowed = true, want custom false")
	}
	if len(got.Delegation.MandatoryDelegationTriggers) != 2 {
		t.Fatalf("LoadProfile().Delegation.MandatoryDelegationTriggers = %#v, want two custom triggers", got.Delegation.MandatoryDelegationTriggers)
	}

	raw, err := os.ReadFile(cfg.ProfileArtifactPath(profile.Name))
	if err != nil {
		t.Fatalf("ReadFile(profile artifact) error = %v", err)
	}
	var persisted map[string]any
	if err := json.Unmarshal(raw, &persisted); err != nil {
		t.Fatalf("profile artifact is not JSON: %v", err)
	}
	if persisted["name"] != string(profile.Name) {
		t.Fatalf("persisted profile name = %#v, want %q", persisted["name"], profile.Name)
	}
}

func TestSaveMarkdownAgent_WritesProfileScopedAgentFile(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir()}
	agent := MarkdownAgent{
		Name:         "claims-triage",
		Description:  "Triage claims-related engineering work.",
		Model:        "sonnet",
		Tools:        "Read, Grep, Glob",
		Role:         "You triage claims-related engineering tasks.",
		Workflow:     "Classify the request, inspect relevant context, then return a focused plan.",
		HardRules:    "Do not persist customer data or policy identifiers.",
		OutputFormat: "Return findings, risks, and next action.",
	}

	path, err := SaveMarkdownAgent(cfg, modelconfig.ProfileName("claims-review"), agent)
	if err != nil {
		t.Fatalf("SaveMarkdownAgent() error = %v", err)
	}

	wantPath := filepath.Join(cfg.ProfileAgentsDir("claims-review"), "claims-triage.md")
	if path != wantPath {
		t.Fatalf("SaveMarkdownAgent() path = %q, want %q", path, wantPath)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(agent) error = %v", err)
	}
	content := string(raw)
	for _, fragment := range []string{
		"name: claims-triage",
		"description: Triage claims-related engineering work.",
		"model: sonnet",
		"tools: Read, Grep, Glob",
		"# Role\nYou triage claims-related engineering tasks.",
		"# Hard Rules\nDo not persist customer data or policy identifiers.",
	} {
		if !strings.Contains(content, fragment) {
			t.Fatalf("agent markdown = %q, want fragment %q", content, fragment)
		}
	}
}

func TestSaveMarkdownAgent_RejectsUnsafeNames(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir()}
	agent := MarkdownAgent{Name: "../escape", Description: "bad", Model: "sonnet", Tools: "Read"}

	if _, err := SaveMarkdownAgent(cfg, modelconfig.ProfileName("claims-review"), agent); err == nil {
		t.Fatal("SaveMarkdownAgent() error = nil for unsafe agent name, want error")
	}
	if _, err := SaveMarkdownAgent(cfg, modelconfig.ProfileName("../escape"), MarkdownAgent{Name: "safe-agent", Description: "ok", Model: "sonnet", Tools: "Read"}); err == nil {
		t.Fatal("SaveMarkdownAgent() error = nil for unsafe profile name, want error")
	}
}
