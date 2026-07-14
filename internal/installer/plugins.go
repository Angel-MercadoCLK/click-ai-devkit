package installer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/modelconfig"
)

const (
	marketplaceName = "click-ai-devkit"
	pluginCLIBinary = "claude"

	// ClickSDDPluginID is the plugin@marketplace identifier Claude Code assigns to click-sdd once
	// installed via SyncMarketplacePlugins. It is exactly the key AppliedClickSDDPluginConfig looks
	// up in settings.json's pluginConfigs map.
	ClickSDDPluginID = "click-sdd@" + marketplaceName
)

var managedPlugins = []string{"click-sdd", "click-memory", "click-review", "click-skills"}

type pluginManifest struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

type commandInvocation struct {
	Name string
	Args []string
}

// CommandRunner abstracts `claude plugin ...` execution so unit tests can assert command order
// without calling the real CLI.
type CommandRunner interface {
	Run(name string, args ...string) error
	Output(name string, args ...string) ([]byte, error)
}

type execCommandRunner struct{ claudeConfigDirOverride string }

// commandEnv builds the environment for the spawned `claude` process. We set CLAUDE_CONFIG_DIR ONLY
// when CLICK_CLAUDE_HOME is explicitly set (tests / power-users) — to redirect the claude subprocess
// to the same throwaway dir click's own files use. In the real (no-override) case we leave it UNSET
// so claude uses its TRUE defaults. This matters: claude stores user-scope MCP servers in
// <home>/.claude.json (home root), NOT <config-dir>/.claude.json — so forcing CLAUDE_CONFIG_DIR=~/.claude
// would make `claude mcp add` (Context7) land where a normal Claude Code session never reads it.
// Plugins live in <config-dir>/plugins either way, so they were and remain unaffected.
func (r execCommandRunner) commandEnv() []string {
	if r.claudeConfigDirOverride == "" {
		return nil // real case: let claude resolve its own default config locations
	}
	return append(os.Environ(), "CLAUDE_CONFIG_DIR="+r.claudeConfigDirOverride)
}

func (r execCommandRunner) Run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = r.commandEnv()
	return cmd.Run()
}

func (r execCommandRunner) Output(name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	cmd.Env = r.commandEnv()
	return cmd.CombinedOutput()
}

var (
	commandRunnerFactory = func() CommandRunner {
		// Only the EXPLICIT override redirects the claude subprocess; a real run leaves
		// CLAUDE_CONFIG_DIR unset so claude uses its own defaults (see commandEnv).
		return execCommandRunner{claudeConfigDirOverride: os.Getenv(claudeHomeEnvOverride)}
	}
	marketplaceSource = defaultMarketplaceSource()
)

func defaultMarketplaceSource() string {
	return "https://github.com/Angel-MercadoCLK/click-ai-devkit"
}

// SetCommandRunnerFactoryForTests overrides the runner factory for tests and returns a restore
// function.
func SetCommandRunnerFactoryForTests(factory func() CommandRunner) func() {
	old := commandRunnerFactory
	commandRunnerFactory = factory
	return func() { commandRunnerFactory = old }
}

// SetMarketplaceSourceForTests overrides the marketplace source for tests and returns a restore
// function.
func SetMarketplaceSourceForTests(source string) func() {
	old := marketplaceSource
	marketplaceSource = source
	return func() { marketplaceSource = old }
}

// SyncMarketplacePlugins uses the official Claude Code plugin CLI to register the marketplace and
// install the click-managed plugins (see managedPlugins). This is the real activation path — copying loose plugin
// folders never loaded anything in Claude Code.
//
// models is the resolved per-phase click-sdd model selection (D25): it is always run through
// modelconfig.ResolveForProfile first, so a nil or partial map still installs click-sdd with a
// fully self-describing --config flag set (the profile's defaults fill any gap). click-memory and
// click-review never receive --config flags — they have no userConfig schema.
//
// profile is trailing-variadic, not a required second positional argument (design D3's literal
// signature would be SyncMarketplacePlugins(profile, models)): this is the single canonical sync
// function — Line B's separate SyncMarketplacePluginsForProfile wrapper is deliberately NOT
// reintroduced — but PR2b's scope explicitly forbids touching internal/cli/{install,update}.go,
// which both call SyncMarketplacePlugins(models) today. A trailing variadic keeps that exact 1-arg
// call compiling unchanged (defaulting to the balanced profile) while giving PR3 a real 2-arg call
// (SyncMarketplacePlugins(models, profile)) to wire the actual selected profile through once it
// migrates those two call sites. At most the first variadic value is used; an empty ProfileName
// behaves the same as omitting it.
func SyncMarketplacePlugins(models map[modelconfig.Phase]string, profile ...modelconfig.ProfileName) error {
	name := modelconfig.ProfileBalanced
	if len(profile) > 0 && profile[0] != "" {
		name = profile[0]
	}
	resolved := modelconfig.ResolveForProfile(string(name), models)
	runner := commandRunnerFactory()
	var sparsePaths []string
	if usesSparseCheckout(marketplaceSource) {
		sparsePaths = []string{".claude-plugin", "plugins"}
	}
	if err := addMarketplace(runner, marketplaceSource, sparsePaths); err != nil {
		return err
	}
	// updateMarketplace forces Claude Code to refresh its cached copy of the marketplace's
	// plugin.json/schema from the git source. This runs unconditionally, on EVERY sync — not just
	// the first-ever `marketplace add` — because addMarketplace's own "already on disk, declared
	// in user settings" no-op path (confirmed live on a re-run of `click update`) never refreshes
	// anything. Without this, `claude plugin install` treats an unversioned-bump plugin.json as
	// "already installed, nothing to check" and validates --config flags against a stale cached
	// schema, silently dropping newly-added userConfig keys (reproduced live: "--config not
	// applied: --config key \"review_risk_model\" isn't declared in this plugin's userConfig").
	// It must run BEFORE the install loop below — a refresh issued after install wouldn't help.
	if err := updateMarketplace(runner, marketplaceName); err != nil {
		return err
	}
	for _, plugin := range managedPlugins {
		var extraArgs []string
		if plugin == "click-sdd" {
			extraArgs = clickSDDConfigArgs(name, resolved)
		}
		if err := installMarketplacePlugin(runner, plugin, extraArgs...); err != nil {
			return err
		}
	}
	return nil
}

// clickSDDConfigArgs builds the `--config <key>=<value>` flag pairs for
// `claude plugin install click-sdd@click-ai-devkit`: FIRST the active orchestration profile
// (modelconfig.ProfileConfigKey, design D3), THEN one `--config <phase>_model=<alias>` pair per
// phase in modelconfig.Phases order (verified against the real CLI in Step 0: repeated
// `--config key=value` flags land in settings.json's
// pluginConfigs["click-sdd@click-ai-devkit"].options).
func clickSDDConfigArgs(profile modelconfig.ProfileName, resolved map[modelconfig.Phase]string) []string {
	args := make([]string, 0, len(modelconfig.Phases)*2+2)
	args = append(args, "--config", modelconfig.ProfileConfigKey+"="+string(profile))
	for _, phase := range modelconfig.Phases {
		model, ok := resolved[phase]
		if !ok || model == "" {
			continue
		}
		args = append(args, "--config", phase.ConfigKey()+"="+model)
	}
	return args
}

// RemoveMarketplacePlugins uninstalls all click-managed plugins (see managedPlugins) and removes the marketplace.
func RemoveMarketplacePlugins() error {
	runner := commandRunnerFactory()
	for _, plugin := range managedPlugins {
		if err := uninstallMarketplacePlugin(runner, plugin); err != nil {
			return err
		}
	}
	if err := removeMarketplace(runner, marketplaceName); err != nil {
		return err
	}
	return nil
}

// HasInstalledPlugin checks Claude Code's plugin registry files to verify one of click's own
// managed plugins (always under the click-ai-devkit marketplace) is actually installed and
// enabled, instead of just checking for a copied loose folder.
func HasInstalledPlugin(cfg Config, plugin string) (bool, error) {
	return HasInstalledPluginID(cfg, plugin+"@"+marketplaceName)
}

// HasInstalledPluginID is the general form behind HasInstalledPlugin: it checks an arbitrary
// plugin@marketplace identifier (e.g. "engram@engram"), not just click's own marketplace.
func HasInstalledPluginID(cfg Config, pluginID string) (bool, error) {
	registryData, err := os.ReadFile(cfg.InstalledPluginsPath())
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("installer: read installed plugins registry: %w", err)
	}
	type installedPluginsRegistry struct {
		Plugins map[string][]map[string]any `json:"plugins"`
	}
	var registry installedPluginsRegistry
	if err := json.Unmarshal(registryData, &registry); err != nil {
		return false, fmt.Errorf("installer: parse installed plugins registry: %w", err)
	}
	entries, ok := registry.Plugins[pluginID]
	if !ok || len(entries) == 0 {
		return false, nil
	}

	settingsData, err := os.ReadFile(cfg.SettingsPath())
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("installer: read settings for enabled plugins: %w", err)
	}
	type settings struct {
		EnabledPlugins map[string]bool `json:"enabledPlugins"`
	}
	var s settings
	if err := json.Unmarshal(settingsData, &s); err != nil {
		return false, fmt.Errorf("installer: parse settings for enabled plugins: %w", err)
	}
	return s.EnabledPlugins[pluginID], nil
}

// AppliedClickSDDPluginConfig reads the click-sdd plugin's ACTUALLY APPLIED --config options from
// cfg.SettingsPath()'s pluginConfigs[ClickSDDPluginID].options — i.e. what Claude Code itself
// accepted and wrote to settings.json, as opposed to what click computed it SHOULD have configured
// (modelconfig.Defaults/Resolve, what models.json holds). This distinction matters: a stale cached
// plugin.json schema on Claude Code's side can silently DROP newly-added --config keys during
// `claude plugin install` without click ever finding out (root-caused live,
// fix/marketplace-refresh-stale-schema — reproduced as "--config not applied: --config key ... isn't
// declared in this plugin's userConfig"). This is the read side `click doctor` uses to detect that
// blind spot. It returns (nil, false, nil) — not an error — when settings.json is missing, has no
// "pluginConfigs" key, has no click-sdd entry, or that entry has no "options" map: all "nothing
// applied yet" states a caller should treat as zero applied keys, not a hard failure. A genuinely
// malformed settings.json (invalid JSON) still returns an error, matching every other settings.json
// reader in this package.
func AppliedClickSDDPluginConfig(cfg Config) (map[string]string, bool, error) {
	settings, err := readSettingsFile(cfg.SettingsPath())
	if err != nil {
		return nil, false, err
	}
	pluginConfigs, ok := settings["pluginConfigs"].(map[string]any)
	if !ok {
		return nil, false, nil
	}
	entry, ok := pluginConfigs[ClickSDDPluginID].(map[string]any)
	if !ok {
		return nil, false, nil
	}
	optionsRaw, ok := entry["options"].(map[string]any)
	if !ok {
		return nil, false, nil
	}
	options := make(map[string]string, len(optionsRaw))
	for key, value := range optionsRaw {
		s, ok := value.(string)
		if !ok {
			continue
		}
		options[key] = s
	}
	return options, true, nil
}

// addMarketplace registers a plugin marketplace. sparsePaths, when non-empty, limits the checkout
// to those paths under the repo root (`--sparse <paths...>`) — only click-ai-devkit's own
// marketplace needs this (its plugins live under plugins/<name>, alongside .claude-plugin/).
// Engram's marketplace must NOT be sparse-checked-out: its plugin lives at plugin/claude-code/,
// not plugins/<name>, so a plugins/-scoped sparse checkout would silently miss it — confirmed
// against the real CLI in Step 0 (spike-e-engram-install.md).
func addMarketplace(runner CommandRunner, source string, sparsePaths []string) error {
	args := []string{"plugin", "marketplace", "add", source}
	if len(sparsePaths) > 0 {
		args = append(args, "--sparse")
		args = append(args, sparsePaths...)
	}
	if err := runner.Run(pluginCLIBinary, args...); err != nil {
		return fmt.Errorf("installer: add plugin marketplace %q: %w", source, err)
	}
	return nil
}

// updateMarketplace forces a refresh of Claude Code's cached copy of the named marketplace's
// plugin.json/schema from its git source (`claude plugin marketplace update <name>`, verified
// live). Errors are treated the same way addMarketplace's are (hard error, aborts the sync):
// the whole point of this step is correctness of the --config flags applied right after it, so a
// failed refresh means the sync can no longer be trusted to configure plugins correctly.
func updateMarketplace(runner CommandRunner, name string) error {
	if err := runner.Run(pluginCLIBinary, "plugin", "marketplace", "update", name); err != nil {
		return fmt.Errorf("installer: update plugin marketplace %q: %w", name, err)
	}
	return nil
}

func installMarketplacePlugin(runner CommandRunner, plugin string, extraArgs ...string) error {
	return installPluginID(runner, plugin, marketplaceName, extraArgs...)
}

func uninstallMarketplacePlugin(runner CommandRunner, plugin string) error {
	return uninstallPluginID(runner, plugin, marketplaceName)
}

// installPluginID installs plugin@marketplace, the general form behind installMarketplacePlugin
// (always click-ai-devkit) and SyncEngramPlugin (always engram).
func installPluginID(runner CommandRunner, plugin, marketplace string, extraArgs ...string) error {
	args := append([]string{"plugin", "install", plugin + "@" + marketplace}, extraArgs...)
	if err := runner.Run(pluginCLIBinary, args...); err != nil {
		return fmt.Errorf("installer: install plugin %s@%s: %w", plugin, marketplace, err)
	}
	return nil
}

// uninstallPluginID is the general form behind uninstallMarketplacePlugin and RemoveEngramPlugin.
func uninstallPluginID(runner CommandRunner, plugin, marketplace string) error {
	if err := runner.Run(pluginCLIBinary, "plugin", "uninstall", plugin+"@"+marketplace); err != nil {
		return fmt.Errorf("installer: uninstall plugin %s@%s: %w", plugin, marketplace, err)
	}
	return nil
}

func removeMarketplace(runner CommandRunner, name string) error {
	if err := runner.Run(pluginCLIBinary, "plugin", "marketplace", "remove", name); err != nil {
		return fmt.Errorf("installer: remove marketplace %s: %w", name, err)
	}
	return nil
}

func usesSparseCheckout(source string) bool {
	if filepath.IsAbs(source) || strings.HasPrefix(source, ".") {
		return false
	}
	return true
}

type fakeCommandRunner struct {
	cfg          Config
	commands     []commandInvocation
	plugins      map[string]bool
	marketplaces map[string]bool
	lookup       map[string][]byte
}

func newFakeCommandRunner(cfg Config) *fakeCommandRunner {
	return &fakeCommandRunner{cfg: cfg, plugins: map[string]bool{}, marketplaces: map[string]bool{}, lookup: map[string][]byte{}}
}

// Run simulates the `claude` CLI well enough for unit tests: it derives the marketplace's
// registry key from the source the same way the real CLI does (the repo/dir basename — e.g.
// "click-ai-devkit" from .../click-ai-devkit, "engram" from .../engram, both verified against the
// real CLI in Step 0), so more than one marketplace (click-ai-devkit AND engram) can coexist in
// the same fake registry within a single test, matching what `click install` now does for real.
func (f *fakeCommandRunner) Run(name string, args ...string) error {
	f.commands = append(f.commands, commandInvocation{Name: name, Args: append([]string(nil), args...)})
	if name != pluginCLIBinary || len(args) < 2 {
		return nil
	}
	if len(args) >= 4 && args[0] == "plugin" && args[1] == "marketplace" && args[2] == "add" {
		source := args[3]
		key := marketplaceKeyFromSource(source)
		f.marketplaces[key] = true
		return f.writeMarketplaceRegistry(key, source)
	}
	if len(args) >= 4 && args[0] == "plugin" && args[1] == "marketplace" && args[2] == "remove" {
		key := args[3]
		delete(f.marketplaces, key)
		return f.removeMarketplaceRegistryEntry(key)
	}
	if len(args) >= 3 && args[0] == "plugin" && args[1] == "install" {
		pluginID := args[2]
		f.plugins[pluginID] = true
		if err := f.upsertPluginRegistry(pluginID, true); err != nil {
			return err
		}
		// Mirrors the real `claude` CLI: `--config key=value` flags land in settings.json's
		// pluginConfigs[pluginID].options (verified against the real CLI in Step 0) — this is what
		// installer.AppliedClickSDDPluginConfig / doctor's checkAppliedPluginConfig read back.
		if options := parseConfigFlags(args[3:]); len(options) > 0 {
			return f.upsertPluginConfig(pluginID, options)
		}
		return nil
	}
	if len(args) >= 3 && args[0] == "plugin" && args[1] == "uninstall" {
		pluginID := args[2]
		delete(f.plugins, pluginID)
		return f.upsertPluginRegistry(pluginID, false)
	}
	if len(args) >= 3 && args[0] == "mcp" && args[1] == "add" {
		// args tail is always "<name> <url>" regardless of transport/scope flags in between —
		// mirrors the exact `claude mcp add --transport http --scope user <name> <url>` shape.
		mcpName := args[len(args)-2]
		mcpURL := args[len(args)-1]
		return f.upsertMCPServer(mcpName, mcpURL)
	}
	if len(args) >= 3 && args[0] == "mcp" && args[1] == "remove" {
		mcpName := args[2]
		return f.removeMCPServer(mcpName)
	}
	return nil
}

// parseConfigFlags extracts the "--config key=value" pairs from a `plugin install` argument tail,
// mirroring how the real `claude` CLI turns repeated --config flags into
// pluginConfigs[pluginID].options entries.
func parseConfigFlags(args []string) map[string]string {
	options := map[string]string{}
	for i := 0; i < len(args)-1; i++ {
		if args[i] != "--config" {
			continue
		}
		key, value, ok := strings.Cut(args[i+1], "=")
		if !ok {
			continue
		}
		options[key] = value
	}
	return options
}

// upsertPluginConfig simulates `claude plugin install ... --config k=v ...`'s effect on Claude
// Code's own settings.json pluginConfigs[pluginID].options — the same shape
// AppliedClickSDDPluginConfig reads for real — so doctor's checkAppliedPluginConfig can observe it
// deterministically in fake-runner-backed tests without shelling out.
func (f *fakeCommandRunner) upsertPluginConfig(pluginID string, options map[string]string) error {
	settings, err := readSettingsFile(f.cfg.SettingsPath())
	if err != nil {
		return err
	}
	pluginConfigs, ok := settings["pluginConfigs"].(map[string]any)
	if !ok || pluginConfigs == nil {
		pluginConfigs = map[string]any{}
	}
	optionsAny := make(map[string]any, len(options))
	for k, v := range options {
		optionsAny[k] = v
	}
	pluginConfigs[pluginID] = map[string]any{"options": optionsAny}
	settings["pluginConfigs"] = pluginConfigs
	return writeSettingsFile(f.cfg.SettingsPath(), settings)
}

// marketplaceKeyFromSource mirrors how the real `claude` CLI names a marketplace after `plugin
// marketplace add <source>` with no explicit name: the source's basename, minus a trailing
// ".git" — verified in Step 0 against the real CLI ("Angel-MercadoCLK/click-ai-devkit" ->
// "click-ai-devkit", "Gentleman-Programming/engram" -> "engram").
func marketplaceKeyFromSource(source string) string {
	base := source
	if idx := strings.LastIndexAny(base, "/\\"); idx != -1 {
		base = base[idx+1:]
	}
	return strings.TrimSuffix(base, ".git")
}

func (f *fakeCommandRunner) Output(name string, args ...string) ([]byte, error) {
	f.commands = append(f.commands, commandInvocation{Name: name, Args: append([]string(nil), args...)})
	key := name + " " + strings.Join(args, " ")
	if out, ok := f.lookup[key]; ok {
		return out, nil
	}
	return []byte{}, nil
}

func (f *fakeCommandRunner) writeMarketplaceRegistry(key, source string) error {
	data := map[string]any{}
	if existing, err := os.ReadFile(f.cfg.KnownMarketplacesPath()); err == nil {
		_ = json.Unmarshal(existing, &data)
	}
	data[key] = map[string]any{
		"source": map[string]any{
			"source":             chooseSourceKind(source),
			pathOrURLKey(source): source,
		},
		"installLocation": source,
	}
	return writeJSONFile(f.cfg.KnownMarketplacesPath(), data)
}

func (f *fakeCommandRunner) removeMarketplaceRegistryEntry(key string) error {
	data := map[string]any{}
	existing, err := os.ReadFile(f.cfg.KnownMarketplacesPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if err := json.Unmarshal(existing, &data); err != nil {
		return err
	}
	delete(data, key)
	if len(data) == 0 {
		return removeIfExists(f.cfg.KnownMarketplacesPath())
	}
	return writeJSONFile(f.cfg.KnownMarketplacesPath(), data)
}

// upsertPluginRegistry adds or removes exactly pluginID, preserving every other entry already on
// disk. This matters for tests that seed a pre-existing plugin (e.g. an Engram install a
// developer already had) directly on disk before exercising Install()/Uninstall(): a naive
// "rebuild the whole registry from what this fake instance has seen" would silently erase that
// seeded entry the moment any other plugin gets installed through the same runner.
func (f *fakeCommandRunner) upsertPluginRegistry(pluginID string, installed bool) error {
	type pluginsRegistry struct {
		Version int                         `json:"version"`
		Plugins map[string][]map[string]any `json:"plugins"`
	}
	reg := pluginsRegistry{Version: 2, Plugins: map[string][]map[string]any{}}
	if data, err := os.ReadFile(f.cfg.InstalledPluginsPath()); err == nil {
		_ = json.Unmarshal(data, &reg)
		if reg.Plugins == nil {
			reg.Plugins = map[string][]map[string]any{}
		}
	}

	if installed {
		pluginName, marketplace, _ := strings.Cut(pluginID, "@")
		reg.Plugins[pluginID] = []map[string]any{{
			"scope":       "user",
			"installPath": filepath.Join(f.cfg.ClaudeHome, "plugins", "cache", marketplace, pluginName, "0.1.0"),
			"version":     "0.1.0",
		}}
	} else {
		delete(reg.Plugins, pluginID)
	}

	plugins := map[string]any{}
	for id, entries := range reg.Plugins {
		plugins[id] = entries
	}
	if err := writeJSONFile(f.cfg.InstalledPluginsPath(), map[string]any{"version": 2, "plugins": plugins}); err != nil {
		return err
	}

	settings, _ := readSettingsFile(f.cfg.SettingsPath())
	enabled := map[string]bool{}
	if raw, ok := settings["enabledPlugins"].(map[string]any); ok {
		for id, v := range raw {
			if b, ok := v.(bool); ok {
				enabled[id] = b
			}
		}
	}
	if installed {
		enabled[pluginID] = true
	} else {
		delete(enabled, pluginID)
	}
	settings["enabledPlugins"] = enabled
	return writeSettingsFile(f.cfg.SettingsPath(), settings)
}

// upsertMCPServer simulates `claude mcp add ... <name> <url>`'s effect on Claude Code's own
// user-scope config file (cfg.Context7ConfigPath — CLAUDE_CONFIG_DIR/.claude.json's top-level
// mcpServers key, confirmed against the real CLI in Step 0 of this slice), so HasContext7's
// pure-file-read can observe it deterministically in tests without ever shelling out.
func (f *fakeCommandRunner) upsertMCPServer(name, url string) error {
	data := map[string]any{}
	if existing, err := os.ReadFile(f.cfg.Context7ConfigPath()); err == nil {
		_ = json.Unmarshal(existing, &data)
	}
	servers, _ := data["mcpServers"].(map[string]any)
	if servers == nil {
		servers = map[string]any{}
	}
	servers[name] = map[string]any{"type": "http", "url": url}
	data["mcpServers"] = servers
	return writeJSONFile(f.cfg.Context7ConfigPath(), data)
}

// removeMCPServer simulates `claude mcp remove <name>`'s effect on the same user-scope config
// file upsertMCPServer writes.
func (f *fakeCommandRunner) removeMCPServer(name string) error {
	data := map[string]any{}
	existing, err := os.ReadFile(f.cfg.Context7ConfigPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if err := json.Unmarshal(existing, &data); err != nil {
		return err
	}
	if servers, ok := data["mcpServers"].(map[string]any); ok {
		delete(servers, name)
		data["mcpServers"] = servers
	}
	return writeJSONFile(f.cfg.Context7ConfigPath(), data)
}

func chooseSourceKind(source string) string {
	if filepath.IsAbs(source) || strings.HasPrefix(source, ".") {
		return "directory"
	}
	return "github"
}

func pathOrURLKey(source string) string {
	if filepath.IsAbs(source) || strings.HasPrefix(source, ".") {
		return "path"
	}
	return "url"
}

func (f *fakeCommandRunner) commandStrings() []string {
	lines := make([]string, 0, len(f.commands))
	for _, cmd := range f.commands {
		var buf bytes.Buffer
		buf.WriteString(cmd.Name)
		for _, arg := range cmd.Args {
			buf.WriteByte(' ')
			buf.WriteString(arg)
		}
		lines = append(lines, buf.String())
	}
	return lines
}
