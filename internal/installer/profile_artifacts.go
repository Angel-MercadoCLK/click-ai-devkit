package installer

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/modelconfig"
)

// artifactNamePattern is a deliberately conservative filesystem-safe name (lowercase, digits,
// internal hyphens only) since every artifact name here becomes part of a path
// (cfg.ProfileArtifactPath/cfg.ProfileAgentsDir) — salvaged verbatim from Line B's
// profile_artifacts.go.
var artifactNamePattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$`)

// MarkdownAgent describes the deterministic Claude Code sub-agent markdown emitted by the profile
// builder path. It intentionally mirrors the repository's existing agent frontmatter convention:
// name/description/model/tools followed by structured prompt sections. Salvaged from Line B's
// profile_artifacts.go, unchanged (no PR2a type it depends on).
type MarkdownAgent struct {
	Name         string
	Description  string
	Model        string
	Tools        string
	Role         string
	Workflow     string
	HardRules    string
	OutputFormat string
}

// SaveProfileArtifact writes a named orchestration profile's generic RuntimeProfile JSON artifact
// to cfg.ProfileArtifactPath(name) — substrate for the separate agent-builder-flow change. This is
// NOT the active-profile store: that's models.json (installer.SaveModelsWithProfile), which is what
// `click install`/`click update`/`click doctor` actually read. This function and the rest of this
// file are UNWIRED to any CLI/UI code in this change.
//
// Re-keyed from Line B's version onto PR2a's actual RuntimeProfile{Name ProfileName, Models
// map[Phase]string} shape (Line B's RuntimeProfile carried different fields, e.g. PhaseChain, that
// don't exist on this branch).
func SaveProfileArtifact(cfg Config, profile modelconfig.RuntimeProfile) error {
	if err := validateArtifactName("profile", string(profile.Name)); err != nil {
		return err
	}
	if err := writeJSONFile(cfg.ProfileArtifactPath(string(profile.Name)), profile); err != nil {
		return fmt.Errorf("installer: write profile artifact: %w", err)
	}
	return nil
}

// LoadProfileArtifact reads a profile artifact previously written by SaveProfileArtifact. Built-in
// profiles (balanced/cost-saver/quality) remain resolved by modelconfig.ResolveProfile — this is
// only for named, saved artifacts (agent-builder-flow substrate).
func LoadProfileArtifact(cfg Config, name modelconfig.ProfileName) (modelconfig.RuntimeProfile, error) {
	if err := validateArtifactName("profile", string(name)); err != nil {
		return modelconfig.RuntimeProfile{}, err
	}
	data, err := os.ReadFile(cfg.ProfileArtifactPath(string(name)))
	if err != nil {
		return modelconfig.RuntimeProfile{}, fmt.Errorf("installer: read profile artifact %q: %w", name, err)
	}
	var profile modelconfig.RuntimeProfile
	if err := json.Unmarshal(data, &profile); err != nil {
		return modelconfig.RuntimeProfile{}, fmt.Errorf("installer: parse profile artifact %q: %w", name, err)
	}
	if profile.Name == "" {
		profile.Name = name
	}
	if profile.Name != name {
		return modelconfig.RuntimeProfile{}, fmt.Errorf("installer: profile artifact %q contains name %q", name, profile.Name)
	}
	return profile, nil
}

// RenderMarkdownAgent returns a deterministic Claude Code sub-agent markdown document. It performs
// only syntax/identity validation; the builder UI or LLM skill owns content quality decisions.
//
// R1-001 fix: Description, Model, and Tools are rejected if they contain a newline (\n or \r).
// Without this, a value like "x\ntools: Bash(*)" would inject an attacker-chosen extra YAML key
// into the frontmatter block once this renderer is wired to free text by the future
// agent-builder-flow change — a tool-permission escalation. Rejecting loudly (matching
// validateArtifactName's error style) is preferred over silently stripping/escaping so bad input
// surfaces immediately instead of being silently altered.
func RenderMarkdownAgent(agent MarkdownAgent) (string, error) {
	if err := validateArtifactName("agent", agent.Name); err != nil {
		return "", err
	}
	if err := validateFrontmatterScalar("description", agent.Description); err != nil {
		return "", err
	}
	if err := validateFrontmatterScalar("model", agent.Model); err != nil {
		return "", err
	}
	if err := validateFrontmatterScalar("tools", agent.Tools); err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("name: ")
	b.WriteString(agent.Name)
	b.WriteString("\n")
	b.WriteString("description: ")
	b.WriteString(strings.TrimSpace(agent.Description))
	b.WriteString("\n")
	b.WriteString("model: ")
	b.WriteString(strings.TrimSpace(agent.Model))
	b.WriteString("\n")
	b.WriteString("tools: ")
	b.WriteString(strings.TrimSpace(agent.Tools))
	b.WriteString("\n---\n\n")
	writeAgentSection(&b, "Role", agent.Role)
	writeAgentSection(&b, "Workflow", agent.Workflow)
	writeAgentSection(&b, "Hard Rules", agent.HardRules)
	writeAgentSection(&b, "Output Format", agent.OutputFormat)
	return b.String(), nil
}

// SaveMarkdownAgent writes a markdown sub-agent into the named profile's agent artifact folder
// (cfg.ProfileAgentsDir(profile)). The profile name is part of the path so a future menu/builder can
// keep custom agents grouped with the profile that introduced them without changing the base
// click-sdd phase chain. Salvaged from Line B's profile_artifacts.go, re-keyed to
// modelconfig.ProfileName.
func SaveMarkdownAgent(cfg Config, profile modelconfig.ProfileName, agent MarkdownAgent) (string, error) {
	if err := validateArtifactName("profile", string(profile)); err != nil {
		return "", err
	}
	content, err := RenderMarkdownAgent(agent)
	if err != nil {
		return "", err
	}
	dir := cfg.ProfileAgentsDir(string(profile))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("installer: create profile agents dir: %w", err)
	}
	path := dir + string(os.PathSeparator) + agent.Name + ".md"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return "", fmt.Errorf("installer: write markdown agent: %w", err)
	}
	return path, nil
}

func validateArtifactName(kind, name string) error {
	if !artifactNamePattern.MatchString(name) {
		return fmt.Errorf("installer: invalid %s artifact name %q", kind, name)
	}
	return nil
}

// validateFrontmatterScalar rejects a YAML frontmatter scalar field that contains a newline (\n or
// \r) — see RenderMarkdownAgent's R1-001 fix comment for why this must be rejected rather than
// silently stripped/escaped.
func validateFrontmatterScalar(field, value string) error {
	if strings.ContainsAny(value, "\n\r") {
		return fmt.Errorf("installer: agent frontmatter field %s contains a newline", field)
	}
	return nil
}

func writeAgentSection(b *strings.Builder, title, body string) {
	b.WriteString("# ")
	b.WriteString(title)
	b.WriteString("\n")
	b.WriteString(strings.TrimSpace(body))
	b.WriteString("\n\n")
}
