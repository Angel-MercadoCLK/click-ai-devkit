package installer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/modelconfig"
)

var artifactNamePattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$`)

// MarkdownAgent describes the deterministic Claude Code sub-agent markdown emitted by the profile
// builder path. It intentionally mirrors the repository's existing agent frontmatter convention:
// name/description/model/tools followed by structured prompt sections.
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

// SaveProfileArtifact writes a full custom orchestration profile descriptor under
// click-ai-devkit/profiles/<name>/profile.json. SaveProfile still controls which profile is active;
// this artifact stores the reusable profile-level configuration the builder creates.
func SaveProfileArtifact(cfg Config, profile modelconfig.RuntimeProfile) error {
	if err := validateArtifactName("profile", string(profile.Name)); err != nil {
		return err
	}
	if err := writeJSONFile(cfg.ProfileArtifactPath(profile.Name), profile); err != nil {
		return fmt.Errorf("installer: write profile artifact: %w", err)
	}
	return nil
}

// LoadProfileArtifact reads a custom orchestration profile descriptor previously written by
// SaveProfileArtifact. Built-in profiles remain resolved by modelconfig.ResolveProfile.
func LoadProfileArtifact(cfg Config, name modelconfig.ProfileName) (modelconfig.RuntimeProfile, error) {
	if err := validateArtifactName("profile", string(name)); err != nil {
		return modelconfig.RuntimeProfile{}, err
	}
	data, err := os.ReadFile(cfg.ProfileArtifactPath(name))
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
func RenderMarkdownAgent(agent MarkdownAgent) (string, error) {
	if err := validateArtifactName("agent", agent.Name); err != nil {
		return "", err
	}
	var b bytes.Buffer
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

// SaveMarkdownAgent writes a markdown sub-agent into the selected profile's agent artifact folder.
// The profile name is part of the path so a future menu/builder can keep custom agents grouped with
// the profile that introduced them without changing the base click-sdd phase chain.
func SaveMarkdownAgent(cfg Config, profile modelconfig.ProfileName, agent MarkdownAgent) (string, error) {
	if err := validateArtifactName("profile", string(profile)); err != nil {
		return "", err
	}
	content, err := RenderMarkdownAgent(agent)
	if err != nil {
		return "", err
	}
	path := cfg.ProfileAgentsDir(profile) + string(os.PathSeparator) + agent.Name + ".md"
	if err := os.MkdirAll(cfg.ProfileAgentsDir(profile), 0o755); err != nil {
		return "", fmt.Errorf("installer: create profile agents dir: %w", err)
	}
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

func writeAgentSection(b *bytes.Buffer, title, body string) {
	b.WriteString("# ")
	b.WriteString(title)
	b.WriteString("\n")
	b.WriteString(strings.TrimSpace(body))
	b.WriteString("\n\n")
}
