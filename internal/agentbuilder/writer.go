package agentbuilder

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

var agentNamePattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$`)

const clickSDDPluginSource = "./plugins/click-sdd"

type FileWriter interface {
	MkdirAll(path string, perm os.FileMode) error
	ReadFile(path string) ([]byte, error)
	WriteFile(path string, data []byte, perm os.FileMode) error
	Stat(path string) (os.FileInfo, error)
}

func RenderAgentMarkdown(spec AgentSpec) (string, error) {
	if err := validateAgentName(spec.Name); err != nil {
		return "", err
	}
	if err := validateFrontmatterScalar("description", spec.Description); err != nil {
		return "", err
	}
	if err := validateFrontmatterScalar("model", spec.Model); err != nil {
		return "", err
	}
	if err := validateFrontmatterScalar("tools", spec.Tools); err != nil {
		return "", err
	}

	var b strings.Builder
	b.WriteString("---\n")
	writeFrontmatterScalar(&b, "name", spec.Name)
	writeFrontmatterScalar(&b, "description", spec.Description)
	writeFrontmatterScalar(&b, "model", spec.Model)
	writeFrontmatterScalar(&b, "tools", spec.Tools)
	b.WriteString("---\n\n")
	writeMarkdownSection(&b, "#", "Role", spec.Purpose)
	writeMarkdownSection(&b, "##", "Tasks", spec.Tasks)
	writeMarkdownSection(&b, "##", "Triggers", spec.Triggers)
	writeMarkdownSection(&b, "##", "Hard Rules", spec.Rules)
	writeSDDIntegrationSection(&b, spec)
	writeMarkdownSection(&b, "##", "Tone", spec.Tone)
	writeMarkdownSection(&b, "##", "Domain Knowledge", spec.Domain)
	writeMarkdownSection(&b, "##", "Good Output", spec.GoodOutput)
	return b.String(), nil
}

func TargetPath(spec AgentSpec, claudeHome, repoRoot string) (string, error) {
	if err := validateAgentName(spec.Name); err != nil {
		return "", err
	}

	switch spec.Placement {
	case PlacementPersonal:
		home, err := resolveClaudeHome(claudeHome)
		if err != nil {
			return "", err
		}
		engine := spec.Engine
		if engine.AgentsDir == nil {
			engine = ClaudeCode
		}
		return filepath.Join(engine.AgentsDir(home), spec.Name+".md"), nil
	case PlacementShareable:
		if strings.TrimSpace(repoRoot) == "" {
			return "", fmt.Errorf("agentbuilder: repo root is required for shareable placement")
		}
		return shareableTargetPath(spec, repoRoot)
	default:
		return "", fmt.Errorf("agentbuilder: unsupported placement %q", spec.Placement)
	}
}

func Install(spec AgentSpec, claudeHome, repoRoot string, w FileWriter) (string, error) {
	if w == nil {
		w = osFileWriter{}
	}
	content, err := RenderAgentMarkdown(spec)
	if err != nil {
		return "", err
	}
	return installContent(spec, []byte(content), claudeHome, repoRoot, w)
}

// InstallFinalMarkdown installs the wizard-confirmed markdown bytes exactly, while preserving the
// same target-path and shareable-plugin scaffolding behavior as Install.
func InstallFinalMarkdown(spec AgentSpec, finalMarkdown, claudeHome, repoRoot string, w FileWriter) (string, error) {
	if w == nil {
		w = osFileWriter{}
	}
	if strings.TrimSpace(finalMarkdown) == "" {
		return "", fmt.Errorf("agentbuilder: final markdown is required")
	}
	return installContent(spec, []byte(finalMarkdown), claudeHome, repoRoot, w)
}

func installContent(spec AgentSpec, content []byte, claudeHome, repoRoot string, w FileWriter) (string, error) {
	path, err := installTargetPath(spec, claudeHome, repoRoot, w)
	if err != nil {
		return "", err
	}
	standalonePluginPath := filepath.Join(repoRoot, "plugins", standalonePluginName(spec), "agents", spec.Name+".md")
	if spec.Placement == PlacementShareable && path == standalonePluginPath {
		if err := ensureShareablePluginNameAvailable(spec, repoRoot, w); err != nil {
			return "", err
		}
	}
	if err := w.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("agentbuilder: create agent directory: %w", err)
	}
	if exists, err := fileExists(w, path); err != nil {
		return "", fmt.Errorf("agentbuilder: inspect target agent: %w", err)
	} else if exists {
		return "", fmt.Errorf("agentbuilder: target agent already exists at %s; choose a different agent name or remove the existing agent before installing", path)
	}
	if err := w.WriteFile(path, content, 0o600); err != nil {
		return "", fmt.Errorf("agentbuilder: write agent: %w", err)
	}
	if spec.Placement == PlacementShareable && path == standalonePluginPath {
		if err := scaffoldShareablePlugin(spec, repoRoot, w); err != nil {
			return "", err
		}
	}
	return path, nil
}

func installTargetPath(spec AgentSpec, claudeHome, repoRoot string, w FileWriter) (string, error) {
	if spec.Placement != PlacementShareable {
		return TargetPath(spec, claudeHome, repoRoot)
	}
	if err := validateAgentName(spec.Name); err != nil {
		return "", err
	}
	if strings.TrimSpace(repoRoot) == "" {
		return "", fmt.Errorf("agentbuilder: repo root is required for shareable placement")
	}
	if spec.SDDMode == SDDPhaseSupport {
		ok, err := hasLoadableClickSDDPlugin(repoRoot, w)
		if err != nil {
			return "", err
		}
		if ok {
			return filepath.Join(repoRoot, "plugins", "click-sdd", "agents", spec.Name+".md"), nil
		}
	}
	return filepath.Join(repoRoot, "plugins", standalonePluginName(spec), "agents", spec.Name+".md"), nil
}

func resolveClaudeHome(claudeHome string) (string, error) {
	if claudeHome != "" {
		return claudeHome, nil
	}
	if configDir := os.Getenv("CLAUDE_CONFIG_DIR"); configDir != "" {
		return configDir, nil
	}
	if clickClaudeHome := os.Getenv("CLICK_CLAUDE_HOME"); clickClaudeHome != "" {
		return clickClaudeHome, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("agentbuilder: resolve claude home: %w", err)
	}
	return filepath.Join(home, ".claude"), nil
}

func shareableTargetPath(spec AgentSpec, repoRoot string) (string, error) {
	if spec.SDDMode == SDDPhaseSupport {
		ok, err := hasLoadableClickSDDPlugin(repoRoot, osFileWriter{})
		if err != nil {
			return "", err
		}
		if ok {
			return filepath.Join(repoRoot, "plugins", "click-sdd", "agents", spec.Name+".md"), nil
		}
	}
	return filepath.Join(repoRoot, "plugins", "click-"+spec.Name, "agents", spec.Name+".md"), nil
}

func hasLoadableClickSDDPlugin(repoRoot string, w FileWriter) (bool, error) {
	marketplacePath := filepath.Join(repoRoot, ".claude-plugin", "marketplace.json")
	marketplace, err := loadMarketplaceManifest(w, marketplacePath)
	if err != nil {
		return false, err
	}
	if !marketplace.HasPluginSource("click-sdd", clickSDDPluginSource) {
		return false, nil
	}
	pluginManifestPath := filepath.Join(repoRoot, "plugins", "click-sdd", ".claude-plugin", "plugin.json")
	manifestData, err := w.ReadFile(pluginManifestPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("agentbuilder: inspect click-sdd plugin manifest: %w", err)
	}
	return isLoadablePluginManifest(manifestData, "click-sdd"), nil
}

func isLoadablePluginManifest(data []byte, name string) bool {
	var manifest pluginManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return false
	}
	return strings.TrimSpace(manifest.Name) == name &&
		strings.TrimSpace(manifest.Version) != "" &&
		strings.TrimSpace(manifest.Description) != "" &&
		strings.TrimSpace(manifest.Author.Name) != ""
}

func validateAgentName(name string) error {
	if !agentNamePattern.MatchString(name) {
		return fmt.Errorf("agentbuilder: invalid agent name %q", name)
	}
	return nil
}

func validateFrontmatterScalar(field, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("agentbuilder: agent frontmatter field %s is required", field)
	}
	if strings.ContainsAny(value, "\n\r") {
		return fmt.Errorf("agentbuilder: agent frontmatter field %s contains a newline", field)
	}
	return nil
}

func writeFrontmatterScalar(b *strings.Builder, field, value string) {
	b.WriteString(field)
	b.WriteString(": ")
	b.WriteString(quoteYAMLScalar(strings.TrimSpace(value)))
	b.WriteString("\n")
}

func quoteYAMLScalar(value string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range value {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if r < 0x20 {
				b.WriteString(fmt.Sprintf(`\u%04x`, r))
				continue
			}
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

func scaffoldShareablePlugin(spec AgentSpec, repoRoot string, w FileWriter) error {
	pluginName := standalonePluginName(spec)
	pluginDir := filepath.Join(repoRoot, "plugins", pluginName)
	pluginManifestDir := filepath.Join(pluginDir, ".claude-plugin")
	if err := w.MkdirAll(pluginManifestDir, 0o755); err != nil {
		return fmt.Errorf("agentbuilder: create plugin manifest directory: %w", err)
	}
	pluginManifest, err := json.MarshalIndent(newPluginManifest(pluginName, spec.Description), "", "  ")
	if err != nil {
		return fmt.Errorf("agentbuilder: render plugin manifest: %w", err)
	}
	pluginManifest = append(pluginManifest, '\n')
	if err := w.WriteFile(filepath.Join(pluginManifestDir, "plugin.json"), pluginManifest, 0o600); err != nil {
		return fmt.Errorf("agentbuilder: write plugin manifest: %w", err)
	}

	marketplacePath := filepath.Join(repoRoot, ".claude-plugin", "marketplace.json")
	marketplace, err := loadMarketplaceManifest(w, marketplacePath)
	if err != nil {
		return err
	}
	marketplace.UpsertPlugin(newMarketplacePlugin(pluginName, spec.Description))
	marketplaceData, err := json.MarshalIndent(marketplace, "", "  ")
	if err != nil {
		return fmt.Errorf("agentbuilder: render marketplace manifest: %w", err)
	}
	marketplaceData = append(marketplaceData, '\n')
	if err := w.MkdirAll(filepath.Dir(marketplacePath), 0o755); err != nil {
		return fmt.Errorf("agentbuilder: create marketplace directory: %w", err)
	}
	if err := w.WriteFile(marketplacePath, marketplaceData, 0o600); err != nil {
		return fmt.Errorf("agentbuilder: write marketplace manifest: %w", err)
	}
	return nil
}

func ensureShareablePluginNameAvailable(spec AgentSpec, repoRoot string, w FileWriter) error {
	pluginName := standalonePluginName(spec)
	pluginManifestPath := filepath.Join(repoRoot, "plugins", pluginName, ".claude-plugin", "plugin.json")
	exists, err := fileExists(w, pluginManifestPath)
	if err != nil {
		return fmt.Errorf("agentbuilder: inspect plugin manifest collision: %w", err)
	}
	if exists {
		return fmt.Errorf("agentbuilder: shareable plugin %q already exists at %s; choose a different agent name or remove the existing plugin metadata before installing", pluginName, pluginManifestPath)
	}

	marketplacePath := filepath.Join(repoRoot, ".claude-plugin", "marketplace.json")
	marketplace, err := loadMarketplaceManifest(w, marketplacePath)
	if err != nil {
		return err
	}
	if marketplace.HasPlugin(pluginName) {
		return fmt.Errorf("agentbuilder: shareable plugin %q already exists in %s; choose a different agent name or remove the existing marketplace entry before installing", pluginName, marketplacePath)
	}
	return nil
}

func fileExists(w FileWriter, path string) (bool, error) {
	_, err := w.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func standalonePluginName(spec AgentSpec) string {
	return "click-" + spec.Name
}

type pluginManifest struct {
	Name        string           `json:"name"`
	Version     string           `json:"version"`
	Description string           `json:"description"`
	Author      pluginAuthor     `json:"author"`
	UserConfig  *json.RawMessage `json:"userConfig,omitempty"`
}

type pluginAuthor struct {
	Name string `json:"name"`
}

func newPluginManifest(name, description string) pluginManifest {
	return pluginManifest{
		Name:        name,
		Version:     "0.1.0",
		Description: strings.TrimSpace(description),
		Author:      pluginAuthor{Name: "Click AI Devkit"},
	}
}

// marketplaceManifest models only the subset of the Claude Code marketplace.json schema
// this package needs to read/write directly. It round-trips through UnmarshalJSON/
// MarshalJSON below so that any field outside this subset (e.g. strict, commands,
// keywords, top-level metadata) survives a rewrite untouched instead of being silently
// dropped (R4-001).
type marketplaceManifest struct {
	Schema      string              `json:"$schema,omitempty"`
	Name        string              `json:"name"`
	Description string              `json:"description,omitempty"`
	Owner       *marketplaceOwner   `json:"owner,omitempty"`
	Plugins     []marketplacePlugin `json:"plugins"`

	extra map[string]json.RawMessage
}

var marketplaceManifestKnownFields = map[string]bool{
	"$schema":     true,
	"name":        true,
	"description": true,
	"owner":       true,
	"plugins":     true,
}

func (m *marketplaceManifest) UnmarshalJSON(data []byte) error {
	type alias marketplaceManifest
	var a alias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	extra := make(map[string]json.RawMessage, len(raw))
	for k, v := range raw {
		if marketplaceManifestKnownFields[k] {
			continue
		}
		extra[k] = v
	}
	*m = marketplaceManifest(a)
	m.extra = extra
	return nil
}

func (m marketplaceManifest) MarshalJSON() ([]byte, error) {
	return mergeExtraFields(m.extra, marketplaceManifestAlias(m))
}

type marketplaceManifestAlias marketplaceManifest

type marketplaceOwner struct {
	Name  string `json:"name"`
	Email string `json:"email,omitempty"`
}

// marketplacePlugin models only the subset of a marketplace plugin entry this package
// needs. It preserves unknown per-plugin fields (e.g. keywords, license) the same way
// marketplaceManifest does (R4-001). Source is raw JSON so both a string source and an
// object-form source (schema-valid alternatives) round-trip without erroring.
type marketplacePlugin struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Version     string          `json:"version"`
	Author      pluginAuthor    `json:"author"`
	Source      json.RawMessage `json:"source"`
	Category    string          `json:"category,omitempty"`
	Homepage    string          `json:"homepage,omitempty"`

	extra map[string]json.RawMessage
}

var marketplacePluginKnownFields = map[string]bool{
	"name":        true,
	"description": true,
	"version":     true,
	"author":      true,
	"source":      true,
	"category":    true,
	"homepage":    true,
}

func (p *marketplacePlugin) UnmarshalJSON(data []byte) error {
	type alias marketplacePlugin
	var a alias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	extra := make(map[string]json.RawMessage, len(raw))
	for k, v := range raw {
		if marketplacePluginKnownFields[k] {
			continue
		}
		extra[k] = v
	}
	*p = marketplacePlugin(a)
	p.extra = extra
	return nil
}

func (p marketplacePlugin) MarshalJSON() ([]byte, error) {
	return mergeExtraFields(p.extra, marketplacePluginAlias(p))
}

type marketplacePluginAlias marketplacePlugin

// mergeExtraFields marshals known (typed) fields via alias, then merges any preserved
// unknown fields back in before the final marshal. Known fields always win on key
// collision. Key order is not preserved (Go map marshaling sorts keys); content is.
func mergeExtraFields(extra map[string]json.RawMessage, known any) ([]byte, error) {
	knownBytes, err := json.Marshal(known)
	if err != nil {
		return nil, err
	}
	merged := make(map[string]json.RawMessage, len(extra))
	for k, v := range extra {
		merged[k] = v
	}
	var knownMap map[string]json.RawMessage
	if err := json.Unmarshal(knownBytes, &knownMap); err != nil {
		return nil, err
	}
	for k, v := range knownMap {
		merged[k] = v
	}
	return json.Marshal(merged)
}

func newMarketplacePlugin(name, description string) marketplacePlugin {
	sourceJSON, err := json.Marshal("./" + path.Join("plugins", name))
	if err != nil {
		// "./"+path.Join(...) is always a valid string; Marshal of a string never fails.
		sourceJSON = []byte(`""`)
	}
	return marketplacePlugin{
		Name:        name,
		Description: strings.TrimSpace(description),
		Version:     "0.1.0",
		Author:      pluginAuthor{Name: "Click AI Devkit"},
		Source:      json.RawMessage(sourceJSON),
		Category:    "productivity",
	}
}

func loadMarketplaceManifest(w FileWriter, marketplacePath string) (marketplaceManifest, error) {
	data, err := w.ReadFile(marketplacePath)
	if err != nil {
		if os.IsNotExist(err) {
			return marketplaceManifest{
				Schema:      "https://anthropic.com/claude-code/marketplace.schema.json",
				Name:        "click-ai-devkit",
				Description: "Click AI Devkit Claude Code plugin marketplace.",
				Owner:       &marketplaceOwner{Name: "Click AI Devkit"},
				Plugins:     []marketplacePlugin{},
			}, nil
		}
		return marketplaceManifest{}, fmt.Errorf("agentbuilder: read marketplace manifest: %w", err)
	}
	var marketplace marketplaceManifest
	if err := json.Unmarshal(data, &marketplace); err != nil {
		return marketplaceManifest{}, fmt.Errorf("agentbuilder: parse marketplace manifest: %w", err)
	}
	return marketplace, nil
}

func (m *marketplaceManifest) UpsertPlugin(plugin marketplacePlugin) {
	for i := range m.Plugins {
		if m.Plugins[i].Name == plugin.Name {
			m.Plugins[i] = plugin
			return
		}
	}
	m.Plugins = append(m.Plugins, plugin)
}

func (m marketplaceManifest) HasPlugin(name string) bool {
	for _, plugin := range m.Plugins {
		if plugin.Name == name {
			return true
		}
	}
	return false
}

func (m marketplaceManifest) HasPluginSource(name, source string) bool {
	for _, plugin := range m.Plugins {
		if plugin.Name != name {
			continue
		}
		var pluginSource string
		if err := json.Unmarshal(plugin.Source, &pluginSource); err != nil {
			// Object-form (or otherwise non-string) sources never match a plain string
			// source comparison; they simply don't match this string-source check.
			continue
		}
		if strings.TrimSpace(pluginSource) == source {
			return true
		}
	}
	return false
}

func writeMarkdownSection(b *strings.Builder, level, title, body string) {
	b.WriteString(level)
	b.WriteString(" ")
	b.WriteString(title)
	b.WriteString("\n")
	b.WriteString(strings.TrimSpace(body))
	b.WriteString("\n\n")
}

func writeSDDIntegrationSection(b *strings.Builder, spec AgentSpec) {
	b.WriteString("## SDD Integration\n")
	b.WriteString("Mode: ")
	b.WriteString(string(spec.SDDMode))
	b.WriteString("\n")
	if spec.SDDMode == SDDPhaseSupport && spec.Phase != "" {
		b.WriteString("Phase: ")
		b.WriteString(string(spec.Phase))
		b.WriteString("\n")
	}
	b.WriteString("\n")
}

type osFileWriter struct{}

func (osFileWriter) MkdirAll(path string, perm os.FileMode) error {
	return os.MkdirAll(path, perm)
}

func (osFileWriter) WriteFile(path string, data []byte, perm os.FileMode) error {
	return os.WriteFile(path, data, perm)
}

func (osFileWriter) ReadFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

func (osFileWriter) Stat(path string) (os.FileInfo, error) {
	return os.Stat(path)
}
