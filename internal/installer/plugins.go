package installer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	marketplaceName = "click-ai-devkit"
	pluginCLIBinary = "claude"
)

var managedPlugins = []string{"click-sdd", "click-memory", "click-review"}

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

type execCommandRunner struct{ claudeHome string }

// commandEnv builds the environment for the spawned `claude` process. When click is redirected to
// a non-default Claude home (via CLICK_CLAUDE_HOME), it MUST propagate that to the claude CLI via
// CLAUDE_CONFIG_DIR so plugin registration lands in the same directory as click's own files — and,
// critically, so a test/override run never installs plugins into the developer's real ~/.claude.
// In production the resolved home is ~/.claude, which is already claude's default: a harmless no-op.
func (r execCommandRunner) commandEnv() []string {
	if r.claudeHome == "" {
		return nil // inherit the parent environment unchanged
	}
	return append(os.Environ(), "CLAUDE_CONFIG_DIR="+r.claudeHome)
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
		home, _ := ResolveClaudeHome()
		return execCommandRunner{claudeHome: home}
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
// install the three Click-managed plugins. This is the real activation path — copying loose plugin
// folders never loaded anything in Claude Code.
func SyncMarketplacePlugins() error {
	runner := commandRunnerFactory()
	if err := addMarketplace(runner, marketplaceSource); err != nil {
		return err
	}
	for _, plugin := range managedPlugins {
		if err := installMarketplacePlugin(runner, plugin); err != nil {
			return err
		}
	}
	return nil
}

// RemoveMarketplacePlugins uninstalls the three Click-managed plugins and removes the marketplace.
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

// HasInstalledPlugin checks Claude Code's plugin registry files to verify a plugin is actually
// installed and enabled, instead of just checking for a copied loose folder.
func HasInstalledPlugin(cfg Config, plugin string) (bool, error) {
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
	pluginID := plugin + "@" + marketplaceName
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

func addMarketplace(runner CommandRunner, source string) error {
	args := []string{"plugin", "marketplace", "add", source}
	if usesSparseCheckout(source) {
		args = append(args, "--sparse", ".claude-plugin", "plugins")
	}
	if err := runner.Run(pluginCLIBinary, args...); err != nil {
		return fmt.Errorf("installer: add plugin marketplace %q: %w", source, err)
	}
	return nil
}

func installMarketplacePlugin(runner CommandRunner, plugin string) error {
	if err := runner.Run(pluginCLIBinary, "plugin", "install", plugin+"@"+marketplaceName); err != nil {
		return fmt.Errorf("installer: install plugin %s: %w", plugin, err)
	}
	return nil
}

func uninstallMarketplacePlugin(runner CommandRunner, plugin string) error {
	if err := runner.Run(pluginCLIBinary, "plugin", "uninstall", plugin+"@"+marketplaceName); err != nil {
		return fmt.Errorf("installer: uninstall plugin %s: %w", plugin, err)
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
	cfg         Config
	commands    []commandInvocation
	plugins     map[string]bool
	marketplace string
	lookup      map[string][]byte
}

func newFakeCommandRunner(cfg Config) *fakeCommandRunner {
	return &fakeCommandRunner{cfg: cfg, plugins: map[string]bool{}, lookup: map[string][]byte{}}
}

func (f *fakeCommandRunner) Run(name string, args ...string) error {
	f.commands = append(f.commands, commandInvocation{Name: name, Args: append([]string(nil), args...)})
	if name != pluginCLIBinary || len(args) < 2 {
		return nil
	}
	switch strings.Join(args[:3], " ") {
	case "plugin marketplace add":
		f.marketplace = marketplaceName
		return f.writeMarketplaceRegistry(args[3])
	case "plugin marketplace remove":
		f.marketplace = ""
		_ = os.Remove(f.cfg.KnownMarketplacesPath())
		return nil
	}
	if len(args) >= 3 && args[0] == "plugin" && args[1] == "install" {
		pluginID := args[2]
		f.plugins[pluginID] = true
		return f.writePluginRegistry()
	}
	if len(args) >= 3 && args[0] == "plugin" && args[1] == "uninstall" {
		delete(f.plugins, args[2])
		return f.writePluginRegistry()
	}
	return nil
}

func (f *fakeCommandRunner) Output(name string, args ...string) ([]byte, error) {
	f.commands = append(f.commands, commandInvocation{Name: name, Args: append([]string(nil), args...)})
	key := name + " " + strings.Join(args, " ")
	if out, ok := f.lookup[key]; ok {
		return out, nil
	}
	return []byte{}, nil
}

func (f *fakeCommandRunner) writeMarketplaceRegistry(source string) error {
	data := map[string]any{
		marketplaceName: map[string]any{
			"source": map[string]any{
				"source":             chooseSourceKind(source),
				pathOrURLKey(source): source,
			},
			"installLocation": source,
		},
	}
	return writeJSONFile(f.cfg.KnownMarketplacesPath(), data)
}

func (f *fakeCommandRunner) writePluginRegistry() error {
	plugins := map[string]any{}
	enabled := map[string]bool{}
	for pluginID := range f.plugins {
		parts := strings.Split(pluginID, "@")
		pluginName := parts[0]
		plugins[pluginID] = []map[string]any{{
			"scope":       "user",
			"installPath": filepath.Join(f.cfg.ClaudeHome, "plugins", "cache", marketplaceName, pluginName, "0.1.0"),
			"version":     "0.1.0",
		}}
		enabled[pluginID] = true
	}
	if err := writeJSONFile(f.cfg.InstalledPluginsPath(), map[string]any{"version": 2, "plugins": plugins}); err != nil {
		return err
	}
	settings, _ := readSettingsFile(f.cfg.SettingsPath())
	settings["enabledPlugins"] = enabled
	return writeSettingsFile(f.cfg.SettingsPath(), settings)
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
