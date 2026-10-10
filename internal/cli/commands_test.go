package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	toml "github.com/pelletier/go-toml/v2"

	"github.com/spf13/cobra"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/doctor"
	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/installer"
	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/modelconfig"
)

// seedResolvableEngram makes EnsureEngramBinary see Engram as already resolvable, so install-driven
// CLI tests are deterministic regardless of the host PATH. Without it, install issues a `go install`
// on any host with `go` but no `engram` (e.g. CI) — passing locally (dev has engram) but failing CI.
func seedResolvableEngram(t *testing.T) {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "engram")
	if err := os.WriteFile(bin, []byte("stub"), 0o755); err != nil {
		t.Fatalf("seed engram binary: %v", err)
	}
	t.Setenv("CLICK_ENGRAM_BINARY_PATH", bin)

	// checkEngramPath (`click doctor`'s PERSISTED-vs-LIVE PATH drift diagnosis, sdd/engram-mcp-
	// resolution obs #1436) reads the REAL platform pathStore (Windows registry / POSIX shell rc)
	// and the REAL live process PATH by default — neither of which this fake, hermetic CLI test
	// suite controls or wants to depend on. Force both probes to report a healthy (persisted AND
	// live) state so `click doctor` after a successful install is deterministic here regardless of
	// the actual test machine's real PATH/registry state.
	t.Cleanup(installer.SetPathPersistedProbeForTests(func(dir string) (bool, error) { return true, nil }))
	t.Cleanup(installer.SetLivePathContainsProbeForTests(func(dir string) bool { return true }))
}

// seedResolvableClickBinary makes `click doctor`'s checkClickBinary check (internal/doctor/checks.go)
// see the click binary as resolvable on PATH, via doctor.SetClickBinaryLookupForTests, so
// install-then-doctor CLI tests are deterministic regardless of whether the real test machine
// actually has click installed to PATH. Without this, these tests only pass by accident of the host
// environment: a developer machine with click on PATH (e.g. via scoop) is healthy, but CI — which
// never installs the binary it just built to PATH — reports checkClickBinary FAIL and the whole
// doctor report unhealthy, exactly mirroring seedResolvableEngram's rationale above for the engram
// binary/PATH checks.
func seedResolvableClickBinary(t *testing.T) {
	t.Helper()
	t.Cleanup(doctor.SetClickBinaryLookupForTests(func(string) (string, error) { return "/usr/bin/click", nil }))
}

// cliFakeBinaryLookup fakes PATH resolution for installer.BinaryLookup, mirroring the same
// injectable pattern already used for the Engram binary/Go toolchain lookups
// (internal/installer/engram_test.go's fakeBinaryLookup) so CLI-level tests never depend on
// whether the real test machine happens to have git on PATH.
type cliFakeBinaryLookup struct {
	resolved map[string]string
}

func (f cliFakeBinaryLookup) LookPath(name string) (string, error) {
	if path, ok := f.resolved[name]; ok {
		return path, nil
	}
	return "", fmt.Errorf("cliFakeBinaryLookup: not found: %s", name)
}

// seedResolvableGit makes installer.GitAvailable/GitPath AND installer.ClaudeAvailable/ClaudePath
// (and therefore runInstall/runUpdate's PreflightClaude + PreflightGit) see both binaries as
// resolvable, so `execRoot`'s install/update-driven CLI tests are deterministic regardless of the
// host PATH — the same rationale as seedResolvableEngram above. execRoot calls this by default;
// tests that specifically exercise a "missing" preflight path build their own cobra command
// instead of going through execRoot (see TestInstallCommand_GitMissing_AbortsBeforeMarketplace-
// Registration / TestInstallCommand_ClaudeMissing_AbortsBeforeMarketplaceRegistration).
func seedResolvableGit(t *testing.T) {
	t.Helper()
	restore := installer.SetBinaryLookupFactoryForTests(func() installer.BinaryLookup {
		return cliFakeBinaryLookup{resolved: map[string]string{"git": "/usr/bin/git", "claude": "/usr/bin/claude"}}
	})
	t.Cleanup(restore)
}

// isolateStandardTargetHomes gives the CLI the standard target paths expected by the pinned
// gentle-ai installer while keeping each target's contents in test-owned temporary directories.
// The symlinked target trees let existing assertions keep using their fixture paths; gentle-ai
// itself still resolves and mutates only <isolated HOME>/.claude, .codex, and .openclaw.
func isolateStandardTargetHomes(t *testing.T, claudeFixture, codexFixture, openClawFixture string) (claudeHome, codexHome, openClawHome string) {
	t.Helper()
	if claudeFixture == "" {
		claudeFixture = os.Getenv("CLICK_TEST_CLAUDE_FIXTURE")
	}
	if codexFixture == "" {
		codexFixture = os.Getenv("CLICK_TEST_CODEX_FIXTURE")
	}
	if openClawFixture == "" {
		openClawFixture = os.Getenv("CLICK_TEST_OPENCLAW_FIXTURE")
	}
	processHome := filepath.Join(filepath.Dir(claudeFixture), ".gentle-ai-test-home-"+filepath.Base(claudeFixture))
	if err := os.MkdirAll(processHome, 0o755); err != nil {
		t.Fatalf("create isolated process home: %v", err)
	}
	t.Setenv("HOME", processHome)
	t.Setenv("USERPROFILE", processHome)

	fixtures := []struct {
		name string
		path string
		into *string
	}{
		{name: ".claude", path: claudeFixture, into: &claudeHome},
		{name: ".codex", path: codexFixture, into: &codexHome},
		{name: ".openclaw", path: openClawFixture, into: &openClawHome},
	}
	for _, fixture := range fixtures {
		if fixture.path == "" {
			fixture.path = t.TempDir()
		}
		if err := os.MkdirAll(fixture.path, 0o755); err != nil {
			t.Fatalf("create %s fixture root: %v", fixture.name, err)
		}
		target := filepath.Join(processHome, fixture.name)
		if err := replaceFixtureTree(fixture.path, target); err != nil {
			t.Fatalf("prepare isolated %s target: %v", fixture.name, err)
		}
		*fixture.into = target
	}
	if err := copyFixtureFile(filepath.Join(claudeFixture, ".claude.json"), filepath.Join(processHome, ".claude.json")); err != nil {
		t.Fatalf("prepare isolated Claude MCP config: %v", err)
	}
	t.Setenv("CLICK_CLAUDE_HOME", claudeHome)
	t.Setenv("CODEX_HOME", codexHome)
	t.Setenv("CLICK_OPENCLAW_HOME", openClawHome)
	t.Setenv("CLICK_TEST_CLAUDE_FIXTURE", claudeFixture)
	t.Setenv("CLICK_TEST_CODEX_FIXTURE", codexFixture)
	t.Setenv("CLICK_TEST_OPENCLAW_FIXTURE", openClawFixture)
	t.Cleanup(func() {
		for _, fixture := range fixtures {
			if fixture.path == "" {
				continue
			}
			if err := replaceFixtureTree(filepath.Join(processHome, fixture.name), fixture.path); err != nil {
				t.Errorf("sync isolated %s fixture: %v", fixture.name, err)
			}
		}
		if err := copyFixtureFile(filepath.Join(processHome, ".claude.json"), filepath.Join(claudeFixture, ".claude.json")); err != nil {
			t.Errorf("sync isolated Claude MCP config: %v", err)
		}
	})
	return claudeHome, codexHome, openClawHome
}

func syncStandardTargetFixtures() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	fixtures := []struct {
		name string
		path string
	}{
		{name: ".claude", path: os.Getenv("CLICK_TEST_CLAUDE_FIXTURE")},
		{name: ".codex", path: os.Getenv("CLICK_TEST_CODEX_FIXTURE")},
		{name: ".openclaw", path: os.Getenv("CLICK_TEST_OPENCLAW_FIXTURE")},
	}
	for _, fixture := range fixtures {
		if fixture.path == "" {
			continue
		}
		if err := replaceFixtureTree(filepath.Join(home, fixture.name), fixture.path); err != nil {
			return fmt.Errorf("sync %s test fixture: %w", fixture.name, err)
		}
	}
	if err := copyFixtureFile(filepath.Join(home, ".claude.json"), filepath.Join(os.Getenv("CLICK_TEST_CLAUDE_FIXTURE"), ".claude.json")); err != nil {
		return fmt.Errorf("sync Claude MCP test fixture: %w", err)
	}
	return nil
}

func configureCLIExecutionHomes(t *testing.T, claudeFixture, codexFixture, openClawFixture string, args []string) {
	t.Helper()
	usesUpstreamInstall := len(args) > 0 && (args[0] == "install" || args[0] == "update")
	continuesStandardHome := os.Getenv("CLICK_TEST_CLAUDE_FIXTURE") == claudeFixture && claudeFixture != ""
	if usesUpstreamInstall || continuesStandardHome {
		isolateStandardTargetHomes(t, claudeFixture, codexFixture, openClawFixture)
		return
	}
	processHome := t.TempDir()
	t.Setenv("HOME", processHome)
	t.Setenv("USERPROFILE", processHome)
	t.Setenv("CLICK_CLAUDE_HOME", claudeFixture)
	if codexFixture == "" {
		codexFixture = t.TempDir()
	}
	if openClawFixture == "" {
		openClawFixture = t.TempDir()
	}
	t.Setenv("CODEX_HOME", codexFixture)
	t.Setenv("CLICK_OPENCLAW_HOME", openClawFixture)
	t.Setenv("CLICK_TEST_CLAUDE_FIXTURE", "")
	t.Setenv("CLICK_TEST_CODEX_FIXTURE", "")
	t.Setenv("CLICK_TEST_OPENCLAW_FIXTURE", "")
}

func copyFixtureFile(source, destination string) error {
	data, err := os.ReadFile(source)
	if os.IsNotExist(err) {
		if removeErr := os.Remove(destination); removeErr != nil && !os.IsNotExist(removeErr) {
			return removeErr
		}
		return nil
	}
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	return os.WriteFile(destination, data, 0o600)
}

func replaceFixtureTree(source, destination string) error {
	if err := os.RemoveAll(destination); err != nil {
		return err
	}
	if err := os.MkdirAll(destination, 0o755); err != nil {
		return err
	}
	return filepath.WalkDir(source, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == source {
			return nil
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		out := filepath.Join(destination, rel)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.IsDir() {
			return os.MkdirAll(out, info.Mode().Perm())
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("unsupported test fixture entry %q", path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return err
		}
		return os.WriteFile(out, data, info.Mode().Perm())
	})
}

func execRoot(t *testing.T, claudeHome string, args ...string) (string, error) {
	t.Helper()
	return execRootWithStateHome(t, claudeHome, t.TempDir(), args...)
}

// execRootWithStateHome is execRoot for tests that must pin CLICK_STATE_HOME themselves. Anything
// that seeds state with its own installer.Config and then expects the command to find it needs
// this: BackupDir() is rooted at ClickStateHome, so a test that let execRoot pick a fresh throwaway
// state home would write its snapshot somewhere the command never looks.
func execRootWithStateHome(t *testing.T, claudeHome, stateHome string, args ...string) (string, error) {
	t.Helper()
	codexFixture, openClawFixture := os.Getenv("CLICK_TEST_CODEX_FIXTURE"), os.Getenv("CLICK_TEST_OPENCLAW_FIXTURE")
	configureCLIExecutionHomes(t, claudeHome, codexFixture, openClawFixture, args)
	t.Setenv("CLICK_STATE_HOME", stateHome)
	// openclaw-target-support (PR-C safety fix): `click uninstall` now unconditionally resolves
	// OpenClawHome (installer.ResolveOpenClawHome) to remove the memory-guard plugin dir, regardless
	// of whether openclaw itself is currently resolvable on PATH. Without this override, every
	// execRoot-based test (this helper is used across install/update/uninstall/doctor tests, most of
	// which never touch OpenClaw at all) would resolve ResolveOpenClawHome() to the REAL
	// <user home>/.openclaw on whatever machine runs `go test` — violating this codebase's own hard
	// safety rule (config.go's openClawHomeEnvOverride doc comment: "never the real home directory").
	// Pointing it at a throwaway t.TempDir() here makes every execRoot-based test safe by default.
	seedResolvableGit(t)

	root := NewRootCommand()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	// Pinned to an empty buffer (not left to default to the real os.Stdin) so every CLI test —
	// including the default no-arg action's interactive() TTY gate — is deterministic regardless
	// of what stdin happens to be under the test runner.
	root.SetIn(&bytes.Buffer{})
	root.SetArgs(args)

	err := root.Execute()
	if os.Getenv("CLICK_TEST_CLAUDE_FIXTURE") != "" {
		if syncErr := syncStandardTargetFixtures(); syncErr != nil {
			t.Errorf("sync target fixtures after command: %v", syncErr)
		}
	}
	return buf.String(), err
}

type testCommandRunner struct {
	home          string
	commands      []string
	plugins       map[string]bool
	pluginConfigs map[string]map[string]string
}

func newTestCommandRunner(home string) *testCommandRunner {
	return &testCommandRunner{home: home, plugins: map[string]bool{}, pluginConfigs: map[string]map[string]string{}}
}

func (r *testCommandRunner) homeDir() string {
	if home := os.Getenv("CLICK_CLAUDE_HOME"); home != "" {
		return home
	}
	return r.home
}

func (r *testCommandRunner) Run(name string, args ...string) error {
	r.commands = append(r.commands, name+" "+strings.Join(args, " "))
	if name != "claude" || len(args) < 2 {
		return nil
	}
	switch {
	case len(args) >= 4 && args[0] == "plugin" && args[1] == "marketplace" && args[2] == "add":
		if strings.Contains(args[3], "click-ai-devkit") {
			if err := r.writeKnownMarketplace(args[3]); err != nil {
				return err
			}
		}
		return r.writeSettings()
	case len(args) >= 3 && args[0] == "plugin" && args[1] == "install":
		r.plugins[args[2]] = true
		// Mirrors the real `claude` CLI: repeated `--config key=value` flags land in settings.json's
		// pluginConfigs[pluginID].options — the shape installer.AppliedClickSDDPluginConfig / doctor's
		// checkAppliedPluginConfig read back.
		if options := parseTestConfigFlags(args[3:]); len(options) > 0 {
			r.pluginConfigs[args[2]] = options
		}
		return r.writeSettings()
	case len(args) >= 3 && args[0] == "plugin" && args[1] == "uninstall":
		delete(r.plugins, args[2])
		return r.writeSettings()
	case len(args) >= 4 && args[0] == "plugin" && args[1] == "marketplace" && args[2] == "remove":
		return nil
	case len(args) >= 3 && args[0] == "mcp" && args[1] == "add":
		return r.upsertMCPServer(args[len(args)-2], args[len(args)-1])
	case len(args) >= 3 && args[0] == "mcp" && args[1] == "remove":
		return r.removeMCPServer(args[2])
	default:
		return nil
	}
}

func (r *testCommandRunner) writeKnownMarketplace(source string) error {
	home := r.homeDir()
	data, err := json.Marshal(map[string]any{
		installer.ClickMarketplaceName(): map[string]any{
			"source": map[string]any{"source": "git", "url": source},
		},
	})
	if err != nil {
		return err
	}
	path := filepath.Join(home, "plugins", "known_marketplaces.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// upsertMCPServer/removeMCPServer simulate `claude mcp add/remove`'s effect on Claude Code's own
// user-scope config file — the same file installer.HasContext7 reads directly (confirmed against
// the real CLI in documentacion/spikes/spike-g-context7.md), mirroring the equivalent simulation
// already used by installer package's own fakeCommandRunner (internal/installer/plugins.go).
func (r *testCommandRunner) context7ConfigPath() string {
	return filepath.Join(r.homeDir(), ".claude.json")
}

func (r *testCommandRunner) upsertMCPServer(name, url string) error {
	data := map[string]any{}
	if existing, err := os.ReadFile(r.context7ConfigPath()); err == nil {
		_ = json.Unmarshal(existing, &data)
	}
	servers, _ := data["mcpServers"].(map[string]any)
	if servers == nil {
		servers = map[string]any{}
	}
	servers[name] = map[string]any{"type": "http", "url": url}
	data["mcpServers"] = servers
	raw, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return os.WriteFile(r.context7ConfigPath(), raw, 0o600)
}

func (r *testCommandRunner) removeMCPServer(name string) error {
	data := map[string]any{}
	existing, err := os.ReadFile(r.context7ConfigPath())
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
	raw, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return os.WriteFile(r.context7ConfigPath(), raw, 0o600)
}

// Output answers `go env GOBIN`/`go env GOPATH` with a realistic, deterministic result — GOBIN
// unset (the common case, empty string, same as a real machine that never ran `go env -w
// GOBIN=...`) and GOPATH resolving under the fake ClaudeHome — so installer.GoBinDir (consulted by
// EnsureEngramBinary's persistPathToBinaryDir and by `click doctor`'s checkEngramPath,
// sdd/engram-mcp-resolution obs #1436) always resolves successfully here, instead of erroring on
// the previous unconditional empty-bytes stub. Every other command still gets the previous
// no-op/empty-bytes behavior.
func (r *testCommandRunner) Output(name string, args ...string) ([]byte, error) {
	home := r.homeDir()
	r.commands = append(r.commands, name+" "+strings.Join(args, " "))
	if name == "go" && len(args) == 2 && args[0] == "env" {
		switch args[1] {
		case "GOBIN":
			return []byte("\n"), nil
		case "GOPATH":
			return []byte(filepath.Join(home, "go") + "\n"), nil
		}
	}
	if filepath.Base(name) == "openclaw" && len(args) == 3 && args[0] == "config" && args[1] == "set" && args[2] == "--help" {
		return []byte("config set agents.defaults.model.primary agents.defaults.model.fallbacks --strict-json"), nil
	}
	return []byte{}, nil
}

// parseTestConfigFlags extracts the "--config key=value" pairs from a `plugin install` argument
// tail, mirroring how the real `claude` CLI turns repeated --config flags into
// pluginConfigs[pluginID].options entries.
func parseTestConfigFlags(args []string) map[string]string {
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

func (r *testCommandRunner) writeSettings() error {
	home := r.homeDir()
	plugins := map[string]any{}
	enabled := map[string]bool{}
	for pluginID := range r.plugins {
		pluginName, marketplace, _ := strings.Cut(pluginID, "@")
		installPath := filepath.Join(home, "plugins", "cache", marketplace, pluginName, "0.1.0")
		plugins[pluginID] = []map[string]any{{"scope": "user", "version": "0.1.0", "installPath": installPath}}
		enabled[pluginID] = true
		if pluginID == installer.EngramPluginID {
			writeTestFile(filepath.Join(installPath, ".mcp.json"), `{"mcpServers":{"engram":{"command":"engram","args":["mcp","--tools=agent"]}}}`)
		}
		if pluginID == installer.ClickSDDPluginID {
			for _, agent := range []string{"click-apply", "click-architect", "click-archive", "click-explore", "click-jd-fix-agent", "click-jd-judge-a", "click-jd-judge-b", "click-memory-curator", "click-onboard", "click-orchestrator", "click-prd-writer", "click-reviewer"} {
				assetPath := filepath.Join("..", "..", "plugins", "click-sdd", "agents", agent+".md")
				data, err := os.ReadFile(assetPath)
				if err != nil {
					return fmt.Errorf("read bundled Click agent asset %s: %w", agent, err)
				}
				assetDestination := filepath.Join(installPath, "agents", agent+".md")
				if err := os.MkdirAll(filepath.Dir(assetDestination), 0o755); err != nil {
					return err
				}
				if err := os.WriteFile(assetDestination, data, 0o600); err != nil {
					return err
				}
			}
		}
	}
	pluginsData, err := json.Marshal(map[string]any{"version": 2, "plugins": plugins})
	if err != nil {
		return err
	}
	pluginsPath := filepath.Join(home, "plugins", "installed_plugins.json")
	if err := os.MkdirAll(filepath.Dir(pluginsPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(pluginsPath, pluginsData, 0o600); err != nil {
		return err
	}
	pluginConfigs := map[string]any{}
	for pluginID, options := range r.pluginConfigs {
		optionsAny := make(map[string]any, len(options))
		for k, v := range options {
			optionsAny[k] = v
		}
		pluginConfigs[pluginID] = map[string]any{"options": optionsAny}
	}

	// MERGE: read existing settings.json if present, preserve foreign keys, set/replace only the keys we own
	settingsPath := filepath.Join(home, "settings.json")
	existingSettings := map[string]any{}
	if data, err := os.ReadFile(settingsPath); err == nil {
		if err := json.Unmarshal(data, &existingSettings); err != nil {
			// If file exists but is corrupt, overwrite with fresh data (conservative fallback)
			existingSettings = map[string]any{}
		}
	}
	// This fake models the external CLI's plugin-config handling as a complete replacement.
	// Tests must not infer that legacy plugin-owned options survive real marketplace operations.
	existingSettings["enabledPlugins"] = enabled
	existingSettings["pluginConfigs"] = pluginConfigs

	settingsData, err := json.MarshalIndent(existingSettings, "", "  ")
	if err != nil {
		return err
	}
	settingsData = append(settingsData, '\n')
	return os.WriteFile(settingsPath, settingsData, 0o600)
}

func writeTestFile(path, content string) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		panic("write test fixture directory: " + err.Error())
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		panic("write test fixture: " + err.Error())
	}
}

func TestInstallCommand_Succeeds(t *testing.T) {
	home := t.TempDir()
	writeJSONFileForTest(t, filepath.Join(home, ".claude.json"), map[string]any{
		"mcpServers":  map[string]any{"private-server": map[string]any{"command": "private-tool"}},
		"userSetting": "preserved",
	})
	runner := newTestCommandRunner(home)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()
	restoreSource := installer.SetMarketplaceSourceForTests("https://github.com/Angel-MercadoCLK/click-ai-devkit")
	defer restoreSource()

	out, err := execRoot(t, home, "install")
	if err != nil {
		t.Fatalf("install command error = %v, output:\n%s", err, out)
	}
	if !strings.Contains(out, "click-sdd") {
		t.Errorf("install output = %q, want it to mention the click-sdd plugin step", out)
	}
	if !strings.Contains(out, "click-memory") {
		t.Errorf("install output = %q, want it to mention the click-memory plugin step", out)
	}
	if !strings.Contains(out, "click-review") {
		t.Errorf("install output = %q, want it to mention the click-review plugin step", out)
	}
	if !strings.Contains(out, "CLAUDE.md") {
		t.Errorf("install output = %q, want it to mention the CLAUDE.md step", out)
	}
	configBytes, err := os.ReadFile(filepath.Join(home, ".claude.json"))
	if err != nil {
		t.Fatalf("ReadFile(.claude.json) error = %v", err)
	}
	var config map[string]any
	if err := json.Unmarshal(configBytes, &config); err != nil {
		t.Fatalf("parse .claude.json: %v", err)
	}
	if config["userSetting"] != "preserved" {
		t.Fatalf(".claude.json userSetting = %v, want existing unrelated value preserved", config["userSetting"])
	}
	servers := config["mcpServers"].(map[string]any)
	for _, name := range []string{"private-server", "engram", "context7"} {
		if _, ok := servers[name]; !ok {
			t.Fatalf(".claude.json mcpServers = %v, want %q present", servers, name)
		}
	}
	if !reflect.DeepEqual(servers["engram"], map[string]any{"command": "engram", "args": []any{"mcp", "--tools=agent"}}) {
		t.Fatalf("upstream Engram registration = %#v, want pinned gentle-ai shape", servers["engram"])
	}
	if !reflect.DeepEqual(servers["context7"], map[string]any{"command": "npx", "args": []any{"-y", "--package=@upstash/context7-mcp@2.2.5", "--", "context7-mcp"}}) {
		t.Fatalf("upstream Context7 registration = %#v, want pinned gentle-ai shape", servers["context7"])
	}
}

func TestResolveInstallTargetSelection_NonInteractiveDoesNotPersistBeforeConfirmation(t *testing.T) {
	stateHome := t.TempDir()
	restoreLookup := installer.SetBinaryLookupFactoryForTests(func() installer.BinaryLookup {
		return cliFakeBinaryLookup{resolved: map[string]string{"codex": "/usr/bin/codex"}}
	})
	defer restoreLookup()

	cmd := NewRootCommand()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetIn(&bytes.Buffer{})

	selection, err := resolveInstallTargetSelection(cmd, false, &buf, rendererFor(cmd, &buf))
	if err != nil {
		t.Fatalf("resolveInstallTargetSelection() error = %v", err)
	}
	if !selection.Codex || selection.Claude || selection.OpenClaw {
		t.Fatalf("selection = %+v, want Codex-only detected selection", selection)
	}
	if _, err := os.Stat(filepath.Join(stateHome, "targets.json")); !os.IsNotExist(err) {
		t.Fatalf("Stat(targets.json) error = %v, want no persisted mutation before final confirmation", err)
	}
}

// execRootWithGitLookup is execRoot's shape, but wired with an explicit installer.BinaryLookup
// instead of execRoot's own default seedResolvableGit — used exclusively by the "git missing"
// preflight tests below, where the whole point is to simulate git being absent from PATH.
func execRootWithGitLookup(t *testing.T, claudeHome string, lookup installer.BinaryLookup, args ...string) (string, error) {
	t.Helper()
	codexFixture, openClawFixture := os.Getenv("CLICK_TEST_CODEX_FIXTURE"), os.Getenv("CLICK_TEST_OPENCLAW_FIXTURE")
	configureCLIExecutionHomes(t, claudeHome, codexFixture, openClawFixture, args)
	t.Setenv("CLICK_STATE_HOME", t.TempDir())
	// Same CLICK_OPENCLAW_HOME safety override as execRoot above — see its comment for the full
	// rationale (runUninstall's unconditional ResolveOpenClawHome call).
	restore := installer.SetBinaryLookupFactoryForTests(func() installer.BinaryLookup { return lookup })
	defer restore()

	root := NewRootCommand()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetIn(&bytes.Buffer{})
	root.SetArgs(args)

	err := root.Execute()
	if os.Getenv("CLICK_TEST_CLAUDE_FIXTURE") != "" {
		if syncErr := syncStandardTargetFixtures(); syncErr != nil {
			t.Errorf("sync target fixtures after command: %v", syncErr)
		}
	}
	return buf.String(), err
}

func execRootWithHomesAndLookup(t *testing.T, claudeHome, stateHome, openClawHome, codexHome string, lookup installer.BinaryLookup, args ...string) (string, error) {
	t.Helper()
	configureCLIExecutionHomes(t, claudeHome, codexHome, openClawHome, args)
	t.Setenv("CLICK_STATE_HOME", stateHome)
	restore := installer.SetBinaryLookupFactoryForTests(func() installer.BinaryLookup { return lookup })
	defer restore()

	root := NewRootCommand()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetIn(&bytes.Buffer{})
	root.SetArgs(args)

	err := root.Execute()
	if os.Getenv("CLICK_TEST_CLAUDE_FIXTURE") != "" {
		if syncErr := syncStandardTargetFixtures(); syncErr != nil {
			t.Errorf("sync target fixtures after command: %v", syncErr)
		}
	}
	return buf.String(), err
}

// TestInstallCommand_GitMissing_AbortsBeforeMarketplaceRegistration reproduces the exact bug
// fixed here — `click install` on a machine with no git on PATH used to fail deep inside plugin
// registration with a cryptic "Command git not found or is in an unsafe location" error, well
// after the interactive model-selection TUI had already run. With PreflightGit wired in at the top
// of runInstall, a missing git must now abort BEFORE any marketplace/plugin registration command is
// ever issued — verified here via the fake CommandRunner recording zero commands at all.
func TestInstallCommand_GitMissing_AbortsBeforeMarketplaceRegistration(t *testing.T) {
	home := t.TempDir()
	runner := newTestCommandRunner(home)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()
	// claude resolvable so PreflightClaude (which now runs first) doesn't mask the git-missing
	// scenario this test targets.
	missingGit := cliFakeBinaryLookup{resolved: map[string]string{"claude": "/usr/bin/claude"}}

	out, err := execRootWithGitLookup(t, home, missingGit, "install")
	if err == nil {
		t.Fatalf("install command error = nil when git is missing from PATH, want a non-nil actionable error, output:\n%s", out)
	}
	if !strings.Contains(err.Error(), installer.GitMissingMessage) {
		t.Fatalf("install command error = %q, want it to contain the actionable message %q", err.Error(), installer.GitMissingMessage)
	}
	if len(runner.commands) != 0 {
		t.Fatalf("runner.commands = %#v, want zero commands issued — install must abort before touching the marketplace when git is missing", runner.commands)
	}
}

// TestInstallCommand_ClaudeMissing_AbortsBeforeMarketplaceRegistration mirrors the git-missing test
// above for T1-2's PreflightClaude: on a machine with no Claude Code installed, `click install`
// used to run its whole interactive TUI and then die with a raw Go dump ("exec: \"claude\":
// executable file not found in %PATH%") deep inside SyncMarketplacePlugins. With PreflightClaude
// wired in at the very top of runInstall (before even PreflightGit), a missing claude must abort
// BEFORE any marketplace/plugin registration command is ever issued.
func TestInstallCommand_ClaudeMissing_AbortsBeforeMarketplaceRegistration(t *testing.T) {
	home := t.TempDir()
	runner := newTestCommandRunner(home)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()
	// git resolvable so this test isolates the claude-missing scenario specifically.
	missingClaude := cliFakeBinaryLookup{resolved: map[string]string{"git": "/usr/bin/git"}}

	out, err := execRootWithGitLookup(t, home, missingClaude, "install")
	if err == nil {
		t.Fatalf("install command error = nil when claude is missing from PATH, want a non-nil actionable error, output:\n%s", out)
	}
	if !strings.Contains(err.Error(), installer.ClaudeMissingMessage) {
		t.Fatalf("install command error = %q, want it to contain the actionable message %q", err.Error(), installer.ClaudeMissingMessage)
	}
	if len(runner.commands) != 0 {
		t.Fatalf("runner.commands = %#v, want zero commands issued — install must abort before touching the marketplace when claude is missing", runner.commands)
	}
}

// TestInstallCommand_GitPresent_ProceedsToMarketplaceRegistration is the GREEN counterpart: with
// git resolvable, PreflightGit must not block the install, and marketplace registration proceeds
// exactly as before this fix.
func TestInstallCommand_GitPresent_ProceedsToMarketplaceRegistration(t *testing.T) {
	home := t.TempDir()
	runner := newTestCommandRunner(home)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()
	presentGit := cliFakeBinaryLookup{resolved: map[string]string{"git": "/usr/bin/git", "claude": "/usr/bin/claude"}}

	out, err := execRootWithGitLookup(t, home, presentGit, "install")
	if err != nil {
		t.Fatalf("install command error = %v when git is present, output:\n%s", err, out)
	}
	if len(runner.commands) == 0 {
		t.Fatal("runner.commands is empty, want install to have issued marketplace/plugin registration commands when git is present")
	}
	wantCommand := "claude plugin marketplace add"
	found := false
	for _, c := range runner.commands {
		if strings.HasPrefix(c, wantCommand) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("runner.commands = %#v, want it to contain a %q command", runner.commands, wantCommand)
	}
}

// TestUpdateCommand_GitMissing_AbortsBeforeMarketplaceRegistration mirrors the install preflight
// test above for `click update`, which also re-syncs the marketplace (SyncMarketplacePlugins) and
// therefore also needs git.
func TestUpdateCommand_GitMissing_AbortsBeforeMarketplaceRegistration(t *testing.T) {
	home := t.TempDir()
	runner := newTestCommandRunner(home)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()
	// claude resolvable so PreflightClaude (which now runs first) doesn't mask the git-missing
	// scenario this test targets.
	missingGit := cliFakeBinaryLookup{resolved: map[string]string{"claude": "/usr/bin/claude"}}

	out, err := execRootWithGitLookup(t, home, missingGit, "update")
	if err == nil {
		t.Fatalf("update command error = nil when git is missing from PATH, want a non-nil actionable error, output:\n%s", out)
	}
	if !strings.Contains(err.Error(), installer.GitMissingMessage) {
		t.Fatalf("update command error = %q, want it to contain the actionable message %q", err.Error(), installer.GitMissingMessage)
	}
	if len(runner.commands) != 0 {
		t.Fatalf("runner.commands = %#v, want zero commands issued — update must abort before touching the marketplace when git is missing", runner.commands)
	}
}

// TestUpdateCommand_ClaudeMissing_AbortsBeforeMarketplaceRegistration mirrors the install-side
// claude-missing test above for `click update`, which also re-syncs the marketplace
// (SyncMarketplacePlugins) via the claude CLI and therefore also needs PreflightClaude wired in.
func TestUpdateCommand_ClaudeMissing_AbortsBeforeMarketplaceRegistration(t *testing.T) {
	home := t.TempDir()
	runner := newTestCommandRunner(home)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()
	// git resolvable so this test isolates the claude-missing scenario specifically.
	missingClaude := cliFakeBinaryLookup{resolved: map[string]string{"git": "/usr/bin/git"}}

	out, err := execRootWithGitLookup(t, home, missingClaude, "update")
	if err == nil {
		t.Fatalf("update command error = nil when claude is missing from PATH, want a non-nil actionable error, output:\n%s", out)
	}
	if !strings.Contains(err.Error(), installer.ClaudeMissingMessage) {
		t.Fatalf("update command error = %q, want it to contain the actionable message %q", err.Error(), installer.ClaudeMissingMessage)
	}
	if len(runner.commands) != 0 {
		t.Fatalf("runner.commands = %#v, want zero commands issued — update must abort before touching the marketplace when claude is missing", runner.commands)
	}
}

func TestInstallCommand_CodexOnly_NonInteractiveOmitsNativeModelWithoutFlagAndDoesNotRequireClaude(t *testing.T) {
	claudeHome := t.TempDir()
	stateHome := t.TempDir()
	codexHome := filepath.Join(stateHome, "codex-home")
	runner := newTestCommandRunner(claudeHome)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()
	lookup := cliFakeBinaryLookup{resolved: map[string]string{"codex": "/usr/bin/codex"}}

	out, err := execRootWithHomesAndLookup(t, claudeHome, stateHome, t.TempDir(), codexHome, lookup, "install")
	if err != nil {
		t.Fatalf("install command error = %v, want nil for Codex-only noninteractive install without Claude, output:\n%s", err, out)
	}
	// gentle-ai owns Codex MCP registration; Click must issue no duplicate mcp get/add command.
	if len(runner.commands) != 0 {
		t.Fatalf("runner.commands = %#v, want no Click-owned Codex MCP commands", runner.commands)
	}
	if strings.Contains(out, "gpt-5.6") {
		t.Fatalf("install output = %q, want no fabricated default Codex native model in noninteractive mode", out)
	}
	config, readErr := os.ReadFile(filepath.Join(codexHome, "config.toml"))
	if readErr != nil {
		t.Fatalf("ReadFile(config.toml) error = %v, want upstream MCP registration", readErr)
	}
	for _, want := range []string{"mcp_servers", "engram", "context7", "https://mcp.context7.com/mcp"} {
		if !strings.Contains(string(config), want) {
			t.Fatalf("config.toml = %q, want upstream MCP field %q", config, want)
		}
	}
	if strings.Contains(string(config), "model =") {
		t.Fatalf("config.toml = %q, want no Click-owned native model mutation", config)
	}
	selection, found, loadErr := installer.LoadTargetSelection(installer.Config{ClickStateHome: stateHome})
	if loadErr != nil {
		t.Fatalf("LoadTargetSelection() error = %v", loadErr)
	}
	if !found || !selection.Codex || selection.Claude {
		t.Fatalf("persisted selection = (%+v, %t), want Codex-only persisted state", selection, found)
	}
}

// TestInstallCommand_CodexOnly_PreservesLegacyModelProfileOnUninstall verifies install and uninstall leave legacy model metadata untouched.
func TestInstallCommand_CodexOnly_PreservesLegacyModelProfileOnUninstall(t *testing.T) {
	claudeHome := t.TempDir()
	stateHome := t.TempDir()
	codexHome := filepath.Join(stateHome, "codex-home")
	runner := newTestCommandRunner(claudeHome)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()
	lookup := cliFakeBinaryLookup{resolved: map[string]string{"codex": "/usr/bin/codex"}}

	cfg := installer.Config{CodexHome: codexHome}
	profilePath := cfg.CodexModelProfilePath()
	profile := []byte(`{"schema_version":1,"tier":"preserve-user-tier","roles":{"apply":"vendor/user"}}`)
	if err := os.MkdirAll(filepath.Dir(profilePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(profilePath, profile, 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := execRootWithHomesAndLookup(t, claudeHome, stateHome, t.TempDir(), codexHome, lookup, "install")
	if err != nil {
		t.Fatalf("install command error = %v, want nil for Codex-only noninteractive install, output:\n%s", err, out)
	}
	raw, err := os.ReadFile(profilePath)
	if err != nil || !bytes.Equal(raw, profile) {
		t.Fatalf("model-profile.json = %q (err %v), want exact legacy bytes preserved", raw, err)
	}

	if _, err := execRootWithHomesAndLookup(t, claudeHome, stateHome, t.TempDir(), codexHome, lookup, "uninstall"); err != nil {
		t.Fatalf("uninstall command error = %v", err)
	}
	got, err := os.ReadFile(profilePath)
	if err != nil || !bytes.Equal(got, profile) {
		t.Fatalf("model-profile.json after uninstall = %q (err %v), want exact legacy bytes preserved", got, err)
	}
}

func TestInstallUpdate_RetiredModelFlagsRejectBeforeSideEffects(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "install Claude profile", args: []string{"install", "--profile="}},
		{name: "install empty Codex model", args: []string{"install", "--codex-model="}},
		{name: "install Codex model", args: []string{"install", "--codex-model=gpt-5.6"}},
		{name: "install empty OpenClaw model", args: []string{"install", "--openclaw-model="}},
		{name: "install OpenClaw model", args: []string{"install", "--openclaw-model=provider/model"}},
		{name: "install empty OpenClaw fallback", args: []string{"install", "--openclaw-fallback-model="}},
		{name: "install OpenClaw fallback", args: []string{"install", "--openclaw-fallback-model=provider/fallback"}},
		{name: "update Claude profile", args: []string{"update", "--profile="}},
		{name: "update empty Codex model", args: []string{"update", "--codex-model="}},
		{name: "update Codex model", args: []string{"update", "--codex-model=gpt-5.6"}},
		{name: "update empty OpenClaw model", args: []string{"update", "--openclaw-model="}},
		{name: "update OpenClaw model", args: []string{"update", "--openclaw-model=provider/model"}},
		{name: "update empty OpenClaw fallback", args: []string{"update", "--openclaw-fallback-model="}},
		{name: "update OpenClaw fallback", args: []string{"update", "--openclaw-fallback-model=provider/fallback"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claudeHome, stateHome := t.TempDir(), t.TempDir()
			openClawHome, codexHome := t.TempDir(), t.TempDir()
			runner := newTestCommandRunner(claudeHome)
			restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
			defer restoreRunner()

			provisionerFactoryCalls := 0
			restoreProvisioner := installer.SetGentleAIProvisionerFactoryForTests(func() installer.GentleAIProvisioner {
				provisionerFactoryCalls++
				return installer.NewGentleAIProvisioner()
			})
			defer restoreProvisioner()

			lookup := cliFakeBinaryLookup{resolved: map[string]string{
				"claude": "/usr/bin/claude", "git": "/usr/bin/git",
				"codex": "/usr/bin/codex", "openclaw": "/usr/bin/openclaw",
			}}
			out, err := execRootWithHomesAndLookup(t, claudeHome, stateHome, openClawHome, codexHome, lookup, tt.args...)
			if err == nil || !strings.Contains(err.Error(), "modelos") {
				t.Fatalf("%v error = %v, output %q; want actionable Spanish model deprecation", tt.args[0], err, out)
			}
			if len(runner.commands) != 0 || provisionerFactoryCalls != 0 {
				t.Fatalf("side effects: runner commands=%#v provisioner factory calls=%d; want none", runner.commands, provisionerFactoryCalls)
			}
			cfg := installer.Config{ClickStateHome: stateHome, CodexHome: codexHome, OpenClawHome: openClawHome}
			for _, path := range []string{cfg.TargetSelectionPath(), filepath.Join(cfg.BackupDir(), "latest", "manifest.json")} {
				if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
					t.Fatalf("%v created state at %q: %v", tt.args[0], path, statErr)
				}
			}
		})
	}
}

func TestInstallUpdate_RetiredModelFlagsAreHiddenCompatibilityFlags(t *testing.T) {
	for _, command := range []*cobra.Command{newInstallCommand(), newUpdateCommand()} {
		for _, name := range []string{profileFlag, codexModelFlag, openClawModelFlag, openClawFallbackFlag} {
			flag := command.Flags().Lookup(name)
			if flag == nil || !flag.Hidden {
				t.Errorf("%s --%s flag = %#v, want hidden compatibility flag", command.Name(), name, flag)
			}
		}
	}
}

func TestCodexOpenClawModelStatePreservedAcrossInstallUpdateUninstall(t *testing.T) {
	for _, command := range []string{"install", "update", "uninstall"} {
		t.Run(command, func(t *testing.T) {
			claudeHome, stateHome := t.TempDir(), t.TempDir()
			codexHome, openClawHome := t.TempDir(), t.TempDir()
			cfg := installer.Config{ClickStateHome: stateHome, CodexHome: codexHome, OpenClawHome: openClawHome}
			if err := os.MkdirAll(codexHome, 0o755); err != nil {
				t.Fatal(err)
			}
			codexConfig := []byte("model = \"vendor/codex-user-model\"\n\n[profiles.sdd-strong]\nmodel = \"vendor/codex-profile-model\"\n")
			if err := os.WriteFile(cfg.CodexConfigPath(), codexConfig, 0o600); err != nil {
				t.Fatal(err)
			}
			codexProfile := []byte(`{"schema_version":1,"tier":"user-tier","roles":{"apply":"user-model"}}`)
			if err := os.MkdirAll(filepath.Dir(cfg.CodexModelProfilePath()), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(cfg.CodexModelProfilePath(), codexProfile, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(openClawHome, 0o755); err != nil {
				t.Fatal(err)
			}
			openClawConfig := []byte(`{"agents":{"defaults":{"model":{"primary":"provider/openclaw-user","fallbacks":["provider/fallback-user"]}}},"mcp":{"servers":{"custom":{"command":"keep-me"}}}}`)
			if err := os.WriteFile(cfg.OpenClawMCPConfigPath(), openClawConfig, 0o600); err != nil {
				t.Fatal(err)
			}
			openClawProfile := []byte(`{"schema_version":2,"profile":"legacy-user-profile","models":{"default":"preserve-me"}}`)
			if err := os.MkdirAll(filepath.Dir(cfg.OpenClawModelProfilePath()), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(cfg.OpenClawModelProfilePath(), openClawProfile, 0o600); err != nil {
				t.Fatal(err)
			}
			if command != "install" {
				if err := installer.SaveTargetSelection(cfg, installer.TargetSelection{Configured: true, Codex: true, OpenClaw: true}); err != nil {
					t.Fatal(err)
				}
			}
			runner := newTestCommandRunner(claudeHome)
			restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
			defer restoreRunner()
			lookup := cliFakeBinaryLookup{resolved: map[string]string{"codex": "/usr/bin/codex", "openclaw": "/usr/bin/openclaw"}}
			out, err := execRootWithHomesAndLookup(t, claudeHome, stateHome, openClawHome, codexHome, lookup, command)
			if err != nil {
				t.Fatalf("%s error = %v, output:\n%s", command, err, out)
			}

			gotCodex, readErr := os.ReadFile(cfg.CodexConfigPath())
			if readErr != nil {
				t.Fatalf("read Codex config: %v", readErr)
			}
			var codexSettings map[string]any
			if err := toml.Unmarshal(gotCodex, &codexSettings); err != nil {
				t.Fatalf("parse Codex config: %v", err)
			}
			profiles := codexSettings["profiles"].(map[string]any)
			sddStrong := profiles["sdd-strong"].(map[string]any)
			if codexSettings["model"] != "vendor/codex-user-model" || sddStrong["model"] != "vendor/codex-profile-model" {
				t.Fatalf("Codex model settings = (top-level %#v, profiles.sdd-strong %#v), want exact configured values", codexSettings["model"], sddStrong["model"])
			}
			gotCodexProfile, readErr := os.ReadFile(cfg.CodexModelProfilePath())
			if readErr != nil || !bytes.Equal(gotCodexProfile, codexProfile) {
				t.Fatalf("Codex profile = %q (err %v), want exact legacy metadata preservation", gotCodexProfile, readErr)
			}
			var gotOpenClaw map[string]any
			if err := readJSONFile(cfg.OpenClawMCPConfigPath(), &gotOpenClaw); err != nil {
				t.Fatalf("read OpenClaw config: %v", err)
			}
			agents := gotOpenClaw["agents"].(map[string]any)
			defaults := agents["defaults"].(map[string]any)
			model := defaults["model"].(map[string]any)
			if model["primary"] != "provider/openclaw-user" || !reflect.DeepEqual(model["fallbacks"], []any{"provider/fallback-user"}) {
				t.Fatalf("OpenClaw native model = %#v, want user primary and fallback preserved", model)
			}
			gotOpenClawProfile, readErr := os.ReadFile(cfg.OpenClawModelProfilePath())
			if readErr != nil || !bytes.Equal(gotOpenClawProfile, openClawProfile) {
				t.Fatalf("OpenClaw profile = %q (err %v), want exact legacy metadata preservation", gotOpenClawProfile, readErr)
			}
			for _, commandLine := range runner.commands {
				if strings.Contains(commandLine, "--config") || strings.Contains(commandLine, "config set") {
					t.Fatalf("Click emitted a model mutation: %q", commandLine)
				}
			}
		})
	}
}

func TestUpdateCommand_CodexOnly_UsesPersistedSelectionWithoutClaudeAndOmitsNativeModelWithoutFlag(t *testing.T) {
	claudeHome := t.TempDir()
	stateHome := t.TempDir()
	codexHome := filepath.Join(stateHome, "codex-home")
	runner := newTestCommandRunner(claudeHome)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()
	lookup := cliFakeBinaryLookup{resolved: map[string]string{"codex": "/usr/bin/codex"}}
	if err := installer.SaveTargetSelection(installer.Config{ClaudeHome: claudeHome, ClickStateHome: stateHome}, installer.TargetSelection{Configured: true, Codex: true}); err != nil {
		t.Fatalf("SaveTargetSelection() error = %v", err)
	}

	out, err := execRootWithHomesAndLookup(t, claudeHome, stateHome, t.TempDir(), codexHome, lookup, "update")
	if err != nil {
		t.Fatalf("update command error = %v, want nil for Codex-only update without Claude, output:\n%s", err, out)
	}
	// gentle-ai owns Codex MCP registration; Click must issue no duplicate mcp get/add command.
	if len(runner.commands) != 0 {
		t.Fatalf("runner.commands = %#v, want no Click-owned Codex MCP commands", runner.commands)
	}
	if strings.Contains(out, "gpt-5.6") {
		t.Fatalf("update output = %q, want no fabricated default Codex native model in noninteractive mode", out)
	}
	config, readErr := os.ReadFile(filepath.Join(codexHome, "config.toml"))
	if readErr != nil {
		t.Fatalf("ReadFile(config.toml) error = %v, want upstream MCP registration", readErr)
	}
	for _, want := range []string{"mcp_servers", "engram", "context7", "https://mcp.context7.com/mcp"} {
		if !strings.Contains(string(config), want) {
			t.Fatalf("config.toml = %q, want upstream MCP field %q", config, want)
		}
	}
	if strings.Contains(string(config), "model =") {
		t.Fatalf("config.toml = %q, want no native model mutation without --codex-model", config)
	}
}

func TestUpdateCommand_CodexGuidanceFailure_RollsBackCodexConfig(t *testing.T) {
	claudeHome := t.TempDir()
	stateHome := t.TempDir()
	codexHome := filepath.Join(stateHome, "codex-home")
	runner := newTestCommandRunner(claudeHome)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()
	lookup := cliFakeBinaryLookup{resolved: map[string]string{"codex": "/usr/bin/codex"}}
	if err := installer.SaveTargetSelection(installer.Config{ClaudeHome: claudeHome, ClickStateHome: stateHome}, installer.TargetSelection{Configured: true, Codex: true}); err != nil {
		t.Fatalf("SaveTargetSelection() error = %v", err)
	}
	before := "model = \"gpt-5.5\"\n"
	if err := os.MkdirAll(codexHome, 0o755); err != nil {
		t.Fatalf("MkdirAll(codexHome) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(codexHome, "config.toml"), []byte(before), 0o600); err != nil {
		t.Fatalf("WriteFile(config.toml) error = %v", err)
	}
	restoreGuidance := SetSyncCodexGuidanceFuncForTests(func(installer.Config) error {
		return errors.New("codex guidance failed after upstream MCP sync")
	})
	defer restoreGuidance()

	out, err := execRootWithHomesAndLookup(t, claudeHome, stateHome, t.TempDir(), codexHome, lookup, "update")
	if err == nil {
		t.Fatalf("update command error = nil, want rollback-triggering failure after Codex MCP sync, output:\n%s", out)
	}
	after, readErr := os.ReadFile(filepath.Join(codexHome, "config.toml"))
	if readErr != nil {
		t.Fatalf("ReadFile(config.toml) error = %v", readErr)
	}
	if string(after) != before {
		t.Fatalf("config.toml after failed update = %q, want rollback-restored bytes %q", after, before)
	}
	if !strings.Contains(strings.ToLower(err.Error()), "rollback") {
		t.Fatalf("update error = %v, want rollback context after restoring the snapshot", err)
	}
}

func TestDoctorCommand_ReportsMissingCacheEntryThenUpdateRestoresHealth(t *testing.T) {
	seedResolvableEngram(t)
	seedResolvableClickBinary(t)
	home := t.TempDir()
	runner := newTestCommandRunner(home)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()

	if _, err := execRoot(t, home, "install"); err != nil {
		t.Fatalf("install command error = %v", err)
	}
	installedPluginsPath := filepath.Join(home, "plugins", "installed_plugins.json")
	raw, err := os.ReadFile(installedPluginsPath)
	if err != nil {
		t.Fatalf("ReadFile(installed_plugins.json) error = %v", err)
	}
	var registry map[string]any
	if err := json.Unmarshal(raw, &registry); err != nil {
		t.Fatalf("json.Unmarshal(installed_plugins.json) error = %v", err)
	}
	plugins := registry["plugins"].(map[string]any)
	entries := plugins[installer.ClickSDDPluginID].([]any)
	entry := entries[0].(map[string]any)
	entry["installPath"] = filepath.Join(home, "plugins", "cache", "click-ai-devkit", "click-sdd-stale", "0.1.0")
	updated, err := json.Marshal(registry)
	if err != nil {
		t.Fatalf("json.Marshal(installed_plugins.json) error = %v", err)
	}
	if err := os.WriteFile(installedPluginsPath, updated, 0o600); err != nil {
		t.Fatalf("WriteFile(installed_plugins.json) error = %v", err)
	}

	doctorOut, doctorErr := execRoot(t, home, "doctor")
	if doctorErr == nil {
		t.Fatalf("doctor error = nil for a registered cache path with missing agent files, output:\n%s", doctorOut)
	}
	if !strings.Contains(doctorOut, "caché registrada") || !strings.Contains(doctorOut, "obsoleta o incompleta") {
		t.Fatalf("doctor output = %q, want the actual missing-cache diagnostic", doctorOut)
	}

	updateOut, updateErr := execRoot(t, home, "update")
	if updateErr != nil {
		t.Fatalf("update command error = %v, output:\n%s", updateErr, updateOut)
	}
	freshDoctorOut, freshDoctorErr := execRoot(t, home, "doctor")
	if freshDoctorErr != nil {
		t.Fatalf("doctor after update error = %v, want healthy report after the refreshed session, output:\n%s", freshDoctorErr, freshDoctorOut)
	}
	if !strings.Contains(freshDoctorOut, "Engram subagent visibility") {
		t.Fatalf("doctor after update output = %q, want the refreshed Engram visibility check", freshDoctorOut)
	}
}

func TestDoctorCommand_CodexOnlySelection_DoesNotRequireClaudeAndUsesPlanChecks(t *testing.T) {
	seedResolvableClickBinary(t)
	claudeHome := t.TempDir()
	stateHome := t.TempDir()
	codexHome := t.TempDir()
	t.Setenv("CLICK_STATE_HOME", stateHome)
	t.Setenv("CODEX_HOME", codexHome)

	selectionCfg := installer.Config{ClaudeHome: claudeHome, ClickStateHome: stateHome}
	if err := installer.SaveTargetSelection(selectionCfg, installer.TargetSelection{Configured: true, Codex: true}); err != nil {
		t.Fatalf("SaveTargetSelection() error = %v", err)
	}
	if err := installer.WriteManagedBlock(installer.Config{CodexHome: codexHome}.CodexAgentsMDPath(), installer.DefaultCodexAgentsContent); err != nil {
		t.Fatalf("WriteManagedBlock(CodexAgentsMDPath) error = %v", err)
	}

	lookup := cliFakeBinaryLookup{resolved: map[string]string{"git": "/usr/bin/git", "codex": "/usr/bin/codex"}}
	out, err := execRootWithHomesAndLookup(t, claudeHome, stateHome, t.TempDir(), codexHome, lookup, "doctor")
	if err != nil {
		t.Fatalf("doctor command error = %v, want healthy Codex-only report without Claude, output:\n%s", err, out)
	}
	if strings.Contains(out, installer.ClaudeMissingMessage) || strings.Contains(out, "[FAIL] claude:") {
		t.Fatalf("doctor output = %q, want no Claude-only failure requirements in a Codex-only report", out)
	}
	if !strings.Contains(strings.ToLower(out), "codex") {
		t.Fatalf("doctor output = %q, want Codex-specific plan-backed diagnostics", out)
	}
}

// TestInstallCommand_FreshInstall_NoBackupCreated guards that a fresh install (no prior
// models.json) behaves exactly as before the migration-wiring fix: no spurious models.json.bak
// gets created when there is nothing stale to back up.
func TestInstallCommand_FreshInstall_NoBackupCreated(t *testing.T) {
	home := t.TempDir()
	runner := newTestCommandRunner(home)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()

	if _, err := execRoot(t, home, "install"); err != nil {
		t.Fatalf("install command error = %v", err)
	}

	cfg := installer.Config{ClaudeHome: home}
	if _, err := os.Stat(cfg.ModelsPath() + ".bak"); !os.IsNotExist(err) {
		t.Fatalf("Stat(models.json.bak) error = %v, want no backup on a fresh install", err)
	}
}

// TestInstallCommand_PreservesStaleModels guards that retired model artifacts are not migrated or
// regenerated during install.
func TestInstallCommand_PreservesStaleModels(t *testing.T) {
	home := t.TempDir()
	runner := newTestCommandRunner(home)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()

	cfg := installer.Config{ClaudeHome: home}
	legacyRaw, err := json.Marshal(map[string]string{
		"orchestrator":   "haiku",
		"prd_writer":     "opus",
		"architect":      "opus",
		"reviewer":       "opus",
		"memory_curator": "sonnet",
	})
	if err != nil {
		t.Fatalf("json.Marshal(legacy) error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(cfg.ModelsPath()), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(cfg.ModelsPath(), legacyRaw, 0o600); err != nil {
		t.Fatalf("WriteFile(legacy models.json) error = %v", err)
	}

	if _, err := execRoot(t, home, "install"); err != nil {
		t.Fatalf("install command error = %v", err)
	}

	got, err := os.ReadFile(cfg.ModelsPath())
	if err != nil || !bytes.Equal(got, legacyRaw) {
		t.Fatalf("models.json = %q (err %v), want original stale bytes preserved", got, err)
	}
	if _, err := os.Stat(cfg.ModelsPath() + ".bak"); !os.IsNotExist(err) {
		t.Fatalf("models.json.bak stat error = %v, want no migration backup", err)
	}
}

// TestInstallCommand_InteractiveCancel_LeavesModelsUntouched guards that cancelling the interactive
// model-selection TUI leaves any existing (even stale) models.json completely untouched: cancel must
// mean "no changes", not "migrated then abandoned". Regression test for R3-001, where MigrateIfStale
// ran unconditionally before the cancel check, so a cancelled install still backed up and regenerated
// a stale models.json.
func TestInstallCommand_InteractiveCancel_LeavesModelsUntouched(t *testing.T) {
	home := t.TempDir()
	cfg := installer.Config{ClaudeHome: home}
	legacyRaw, err := json.Marshal(map[string]string{
		"orchestrator":   "haiku",
		"prd_writer":     "opus",
		"architect":      "opus",
		"reviewer":       "opus",
		"memory_curator": "sonnet",
	})
	if err != nil {
		t.Fatalf("json.Marshal(legacy) error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(cfg.ModelsPath()), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(cfg.ModelsPath(), legacyRaw, 0o600); err != nil {
		t.Fatalf("WriteFile(legacy models.json) error = %v", err)
	}

	cmd := NewRootCommand()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	r := rendererFor(cmd, &buf)

	cancelledSelector := func(*cobra.Command, modelconfig.ProfileName) (modelconfig.ProfileName, map[modelconfig.Phase]string, bool, error) {
		return "", nil, true, nil
	}

	// nonInteractive=false forces the interactive branch regardless of the test's non-TTY buffer,
	// so the fake selector's cancel is what actually gets exercised.
	_, _, cancelled, err := resolveInstallModels(cmd, &buf, r, cfg, false, "", cancelledSelector)
	if err != nil {
		t.Fatalf("resolveInstallModels() error = %v", err)
	}
	if !cancelled {
		t.Fatal("resolveInstallModels() cancelled = false, want true")
	}

	if _, err := os.Stat(cfg.ModelsPath() + ".bak"); !os.IsNotExist(err) {
		t.Fatalf("Stat(models.json.bak) error = %v, want no backup created on interactive cancel", err)
	}
	got, err := os.ReadFile(cfg.ModelsPath())
	if err != nil {
		t.Fatalf("ReadFile(models.json) error = %v", err)
	}
	if string(got) != string(legacyRaw) {
		t.Fatalf("models.json = %q, want unchanged legacy content %q (cancel must leave disk untouched)", got, legacyRaw)
	}
}

func TestDoctorCommand_AfterInstall_Succeeds(t *testing.T) {
	seedResolvableEngram(t)
	seedResolvableClickBinary(t)
	home := t.TempDir()
	runner := newTestCommandRunner(home)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()

	if _, err := execRoot(t, home, "install"); err != nil {
		t.Fatalf("install command error = %v", err)
	}

	out, err := execRoot(t, home, "doctor")
	if err != nil {
		t.Fatalf("doctor command error = %v after a successful install, output:\n%s", err, out)
	}
}

func TestDoctorCommand_BeforeInstall_ReturnsError(t *testing.T) {
	home := t.TempDir()

	_, err := execRoot(t, home, "doctor")
	if err == nil {
		t.Fatal("doctor command on a never-installed ClaudeHome returned nil error, want a non-nil error (non-zero exit)")
	}
}

func TestUninstallCommand_ReversesInstall(t *testing.T) {
	seedResolvableEngram(t)
	seedResolvableClickBinary(t)
	home := t.TempDir()
	runner := newTestCommandRunner(home)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()

	if _, err := execRoot(t, home, "install"); err != nil {
		t.Fatalf("install command error = %v", err)
	}
	if _, err := execRoot(t, home, "doctor"); err != nil {
		t.Fatalf("doctor command error = %v right after install", err)
	}

	if _, err := execRoot(t, home, "uninstall"); err != nil {
		t.Fatalf("uninstall command error = %v", err)
	}

	_, err := execRoot(t, home, "doctor")
	if err == nil {
		t.Fatal("doctor command after uninstall returned nil error, want a non-nil error (unhealthy)")
	}
}

func TestUninstallCommand_NoopWhenAlreadyUninstalled(t *testing.T) {
	home := t.TempDir()
	runner := newTestCommandRunner(home)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()

	if _, err := execRoot(t, home, "uninstall"); err != nil {
		t.Fatalf("uninstall command on a never-installed ClaudeHome error = %v, want nil", err)
	}
}

func TestUpdateCommand_ResyncsPluginsAndWritesEngramPin(t *testing.T) {
	home := t.TempDir()
	runner := newTestCommandRunner(home)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()
	binaryPath := filepath.Join(t.TempDir(), "engram.exe")
	if err := os.WriteFile(binaryPath, []byte("binary"), 0o644); err != nil {
		t.Fatalf("WriteFile(binary) error = %v", err)
	}
	t.Setenv("CLICK_ENGRAM_BINARY_PATH", binaryPath)

	if _, err := execRoot(t, home, "install"); err != nil {
		t.Fatalf("install command error = %v", err)
	}

	out, err := execRoot(t, home, "update")
	if err != nil {
		t.Fatalf("update command error = %v", err)
	}
	if !strings.Contains(out, "click-review") {
		t.Errorf("update output = %q, want it to mention the click-review re-sync step", out)
	}
	if !strings.Contains(out, "gentle-ai") {
		t.Errorf("update output = %q, want it to mention upstream component sync", out)
	}

	wantCommand := "claude plugin install click-review@click-ai-devkit"
	if !contains(runner.commands, wantCommand) {
		t.Fatalf("update command sequence = %#v, want it to contain %q", runner.commands, wantCommand)
	}

	// Click no longer writes a separate Engram pin or MCP registration; gentle-ai owns that lifecycle.
	if _, err := os.Stat(filepath.Join(home, "click-ai-devkit", "engram.json")); !os.IsNotExist(err) {
		t.Fatalf("update created Click-owned Engram state: stat error = %v, want no state file", err)
	}
}

// TestUpdateCommand_DoesNotCreateRetiredModelsJson verifies that update does not create the
// retired Click-owned model artifact when none exists.
func TestUpdateCommand_DoesNotCreateRetiredModelsJson(t *testing.T) {
	home := t.TempDir()
	runner := newTestCommandRunner(home)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()
	binaryPath := filepath.Join(t.TempDir(), "engram.exe")
	if err := os.WriteFile(binaryPath, []byte("binary"), 0o644); err != nil {
		t.Fatalf("WriteFile(binary) error = %v", err)
	}
	t.Setenv("CLICK_ENGRAM_BINARY_PATH", binaryPath)

	// No prior install ran, so models.json is absent. Update must leave that state unchanged rather
	// than generating Click-owned model defaults.
	if _, err := execRoot(t, home, "update"); err != nil {
		t.Fatalf("update command error = %v", err)
	}

	if _, err := os.Stat(installer.Config{ClaudeHome: home}.ModelsPath()); !os.IsNotExist(err) {
		t.Fatalf("models.json stat error = %v, want no Click model artifact created", err)
	}
}

func TestInstallAndUpdate_PreserveLegacyClaudeModelArtifacts(t *testing.T) {
	for _, command := range []string{"install", "update"} {
		t.Run(command, func(t *testing.T) {
			home := t.TempDir()
			cfg := installer.Config{ClaudeHome: home}
			if err := os.MkdirAll(filepath.Dir(cfg.ModelsPath()), 0o755); err != nil {
				t.Fatal(err)
			}
			original := []byte("legacy user-owned model data: keep byte-for-byte")
			if err := os.WriteFile(cfg.ModelsPath(), original, 0o600); err != nil {
				t.Fatal(err)
			}
			settings := []byte(`{"model":"user-default"}`)
			if err := os.WriteFile(cfg.SettingsPath(), settings, 0o600); err != nil {
				t.Fatal(err)
			}
			runner := newTestCommandRunner(home)
			restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
			defer restoreRunner()

			if _, err := execRoot(t, home, command); err != nil {
				t.Fatalf("%s command error = %v", command, err)
			}
			got, err := os.ReadFile(cfg.ModelsPath())
			if err != nil {
				t.Fatalf("ReadFile(models.json) error = %v", err)
			}
			if !bytes.Equal(got, original) {
				t.Fatalf("models.json = %q, want preserved bytes %q", got, original)
			}
			if _, err := os.Stat(cfg.ModelsPath() + ".bak"); !os.IsNotExist(err) {
				t.Fatalf("models.json.bak stat error = %v, want no migration backup", err)
			}
			var gotSettings map[string]any
			if err := readJSONFile(cfg.SettingsPath(), &gotSettings); err != nil {
				t.Fatalf("read settings.json: %v", err)
			}
			if gotSettings["model"] != "user-default" {
				t.Fatalf("settings model = %#v, want preserved native model", gotSettings["model"])
			}
			for _, command := range runner.commands {
				if strings.Contains(command, "--config") {
					t.Fatalf("runner command %q contains retired model config flags", command)
				}
			}
		})
	}
}

func TestRetiredClaudeProfileFlagRejectsBeforeInstallOrUpdateWrites(t *testing.T) {
	for _, command := range []string{"install", "update"} {
		t.Run(command, func(t *testing.T) {
			home, stateHome := t.TempDir(), t.TempDir()
			runner := newTestCommandRunner(home)
			restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
			defer restoreRunner()

			out, err := execRootWithStateHome(t, home, stateHome, command, "--profile", "quality")
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), "modelos") || !strings.Contains(strings.ToLower(err.Error()), "gentle-ai") {
				t.Fatalf("%s --profile error = %v, output %q; want actionable Spanish deprecation error", command, err, out)
			}
			if len(runner.commands) != 0 {
				t.Fatalf("runner.commands = %#v, want no commands before retired flag rejection", runner.commands)
			}
			if _, found, loadErr := installer.LoadTargetSelection(installer.Config{ClickStateHome: stateHome}); loadErr != nil || found {
				t.Fatalf("target selection = (found=%t, err=%v), want no persisted state", found, loadErr)
			}
			if has, snapshotErr := installer.HasRunSnapshot(installer.Config{ClaudeHome: home, ClickStateHome: stateHome}); snapshotErr != nil || has {
				t.Fatalf("run snapshot = (present=%t, err=%v), want rejection before snapshot", has, snapshotErr)
			}
		})
	}
}

func TestRetiredClaudeProfileFlagIsHiddenFromInstallAndUpdateHelp(t *testing.T) {
	for name, command := range map[string]*cobra.Command{
		"install": newInstallCommand(),
		"update":  newUpdateCommand(),
	} {
		t.Run(name, func(t *testing.T) {
			flag := command.Flags().Lookup(profileFlag)
			if flag == nil || !flag.Hidden {
				t.Fatalf("%s --profile flag = %#v, want hidden compatibility flag", name, flag)
			}
		})
	}
}

func readJSONFile(path string, target any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}

func TestOpenClawOnlyUpdateDoesNotReadClaudeModelsOrRewriteLegacyProfile(t *testing.T) {
	claudeHome, stateHome, openClawHome := t.TempDir(), t.TempDir(), t.TempDir()
	runner := newTestCommandRunner(claudeHome)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()
	lookup := cliFakeBinaryLookup{resolved: map[string]string{"openclaw": "/usr/bin/openclaw"}}
	cfg := installer.Config{ClaudeHome: claudeHome, ClickStateHome: stateHome, OpenClawHome: openClawHome}
	if err := installer.SaveTargetSelection(cfg, installer.TargetSelection{Configured: true, OpenClaw: true}); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(cfg.ModelsPath()), 0o755); err != nil {
		t.Fatal(err)
	}
	models := []byte("not valid JSON: must not be read")
	if err := os.WriteFile(cfg.ModelsPath(), models, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(cfg.OpenClawModelProfilePath()), 0o755); err != nil {
		t.Fatal(err)
	}
	profile := []byte("legacy OpenClaw profile metadata: preserve exactly")
	if err := os.WriteFile(cfg.OpenClawModelProfilePath(), profile, 0o600); err != nil {
		t.Fatal(err)
	}

	out, err := execRootWithHomesAndLookup(t, claudeHome, stateHome, openClawHome, t.TempDir(), lookup, "update")
	if err != nil {
		t.Fatalf("OpenClaw-only update error = %v, output %q", err, out)
	}
	gotModels, readErr := os.ReadFile(cfg.ModelsPath())
	if readErr != nil || !bytes.Equal(gotModels, models) {
		t.Fatalf("models.json = %q (err %v), want exact preservation without migration or rewrite", gotModels, readErr)
	}
	gotProfile, readErr := os.ReadFile(cfg.OpenClawModelProfilePath())
	if readErr != nil || !bytes.Equal(gotProfile, profile) {
		t.Fatalf("OpenClaw model-profile.json = %q (err %v), want exact preservation", gotProfile, readErr)
	}
}

func TestRootCommand_VersionDefaultsToDev(t *testing.T) {
	home := t.TempDir()
	out, err := execRoot(t, home, "--version")
	if err != nil {
		t.Fatalf("version command error = %v", err)
	}
	if !strings.Contains(out, "dev") {
		t.Fatalf("version output = %q, want it to contain dev", out)
	}
}

func TestInstallCommand_IssuesMarketplaceCommandsInOrder(t *testing.T) {
	home := t.TempDir()
	runner := newTestCommandRunner(home)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()
	restoreSource := installer.SetMarketplaceSourceForTests("https://github.com/Angel-MercadoCLK/click-ai-devkit")
	defer restoreSource()

	if _, err := execRoot(t, home, "install"); err != nil {
		t.Fatalf("install command error = %v", err)
	}
	want := []string{
		"claude plugin marketplace add https://github.com/Angel-MercadoCLK/click-ai-devkit --sparse .claude-plugin plugins",
		"claude plugin marketplace update click-ai-devkit",
		"claude plugin install click-sdd@click-ai-devkit",
		"claude plugin install click-memory@click-ai-devkit",
		"claude plugin install click-review@click-ai-devkit",
		"claude plugin install click-skills@click-ai-devkit",
	}
	if !reflect.DeepEqual(runner.commands[:6], want) {
		t.Fatalf("runner.commands[:6] = %#v, want %#v", runner.commands[:6], want)
	}
}

// TestInstallCommand_NonTTY_ReturnsWithoutCreatingModelArtifact guards the non-TTY/CI safety
// contract: install returns without waiting for interactive input and no longer creates Click-owned
// model artifacts. execRoot uses buffers rather than a real terminal, so this test makes the
// non-blocking behavior explicit with its own elapsed-time assertion.
func TestInstallCommand_NonTTY_ReturnsWithoutCreatingModelArtifact(t *testing.T) {
	home := t.TempDir()
	runner := newTestCommandRunner(home)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()

	start := time.Now()
	if _, err := execRoot(t, home, "install"); err != nil {
		t.Fatalf("install command error = %v", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("install command took %s on a non-TTY buffer, want it to return immediately without blocking on interactive input", elapsed)
	}

	if _, err := os.Stat(installer.Config{ClaudeHome: home}.ModelsPath()); !os.IsNotExist(err) {
		t.Fatalf("models.json stat error = %v, want no Click model artifact created", err)
	}
}

// TestInstallCommand_ProfileFlag_NonInteractive_RejectsRetiredSelection verifies that install
// rejects the retired Click profile selector before invoking any command.
func TestInstallCommand_ProfileFlag_NonInteractive_RejectsRetiredSelection(t *testing.T) {
	home := t.TempDir()
	runner := newTestCommandRunner(home)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()
	restoreSource := installer.SetMarketplaceSourceForTests("https://github.com/Angel-MercadoCLK/click-ai-devkit")
	defer restoreSource()

	_, err := execRoot(t, home, "install", "--profile", "cost-saver")
	if err == nil || !strings.Contains(err.Error(), "Configure Models") {
		t.Fatalf("install --profile error = %v, want actionable deprecation", err)
	}
	if len(runner.commands) != 0 {
		t.Fatalf("runner.commands = %#v, want rejection before any command", runner.commands)
	}
}

// TestInstallCommand_ProfileFlag_UnknownValue_StillRejectsRetiredSelection verifies that an
// unknown value does not bypass deprecation rejection or invoke any command.
func TestInstallCommand_ProfileFlag_UnknownValue_StillRejectsRetiredSelection(t *testing.T) {
	home := t.TempDir()
	runner := newTestCommandRunner(home)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()

	_, err := execRoot(t, home, "install", "--profile", "not-a-real-profile")
	if err == nil || !strings.Contains(err.Error(), "Configure Models") {
		t.Fatalf("install --profile error = %v, want actionable deprecation", err)
	}
	if len(runner.commands) != 0 {
		t.Fatalf("runner.commands = %#v, want rejection before any command", runner.commands)
	}
}

// TestResolveInstallModels_Interactive_PresetUnmodified_PersistsPresetLabel and
// TestResolveInstallModels_Interactive_TweakedPreset_PersistsCustomLabel guard the label-consistency
// rule end to end through resolveInstallModels: a preset the developer left untouched keeps its own
// name, but a hand-tweaked preset downgrades to "custom" so the persisted label never claims values
// the map no longer holds.
func TestResolveInstallModels_Interactive_PresetUnmodified_PersistsPresetLabel(t *testing.T) {
	home := t.TempDir()
	cfg := installer.Config{ClaudeHome: home}
	cmd := NewRootCommand()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	r := rendererFor(cmd, &buf)

	unmodified := modelconfig.ResolveProfile(string(modelconfig.ProfileQuality)).Models
	selector := func(*cobra.Command, modelconfig.ProfileName) (modelconfig.ProfileName, map[modelconfig.Phase]string, bool, error) {
		return modelconfig.ProfileQuality, unmodified, false, nil
	}

	profile, models, cancelled, err := resolveInstallModels(cmd, &buf, r, cfg, false, "", selector)
	if err != nil {
		t.Fatalf("resolveInstallModels() error = %v", err)
	}
	if cancelled {
		t.Fatal("resolveInstallModels() cancelled = true, want false")
	}
	if profile != modelconfig.ProfileQuality {
		t.Fatalf("resolveInstallModels() profile = %q, want %q (unmodified preset keeps its own label)", profile, modelconfig.ProfileQuality)
	}
	if !reflect.DeepEqual(models, unmodified) {
		t.Fatalf("resolveInstallModels() models = %#v, want %#v", models, unmodified)
	}
}

func TestResolveInstallModels_Interactive_TweakedPreset_PersistsCustomLabel(t *testing.T) {
	home := t.TempDir()
	cfg := installer.Config{ClaudeHome: home}
	cmd := NewRootCommand()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	r := rendererFor(cmd, &buf)

	tweaked := modelconfig.ResolveProfile(string(modelconfig.ProfileQuality)).Models
	tweaked[modelconfig.PhaseExplore] = "haiku" // hand-edit away from quality's "opus"
	selector := func(*cobra.Command, modelconfig.ProfileName) (modelconfig.ProfileName, map[modelconfig.Phase]string, bool, error) {
		return modelconfig.ProfileQuality, tweaked, false, nil
	}

	profile, models, cancelled, err := resolveInstallModels(cmd, &buf, r, cfg, false, "", selector)
	if err != nil {
		t.Fatalf("resolveInstallModels() error = %v", err)
	}
	if cancelled {
		t.Fatal("resolveInstallModels() cancelled = true, want false")
	}
	if profile != modelconfig.ProfileCustom {
		t.Fatalf("resolveInstallModels() profile = %q, want %q (tweaked preset downgrades to custom)", profile, modelconfig.ProfileCustom)
	}
	if !reflect.DeepEqual(models, tweaked) {
		t.Fatalf("resolveInstallModels() models = %#v, want %#v (tweaked values must still be preserved)", models, tweaked)
	}
}

// TestResolveInstallModels_Interactive_ThreadsProfileFlagAsInitialSelection guards the C2 fix:
// `click install --profile X` on a real terminal must pass X through to the interactive selector
// as the picker's starting profile, instead of always hardcoding balanced. The fake selector here
// stands in for runInstallSelectTUI (a real bubbletea program can't be exercised headlessly), so
// this test verifies the wiring: the initial profile resolveInstallModels hands to the selector.
func TestResolveInstallModels_Interactive_ThreadsProfileFlagAsInitialSelection(t *testing.T) {
	home := t.TempDir()
	cfg := installer.Config{ClaudeHome: home}
	cmd := NewRootCommand()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	r := rendererFor(cmd, &buf)

	var gotInitial modelconfig.ProfileName
	selector := func(_ *cobra.Command, initial modelconfig.ProfileName) (modelconfig.ProfileName, map[modelconfig.Phase]string, bool, error) {
		gotInitial = initial
		return modelconfig.ProfileCostSaver, modelconfig.ResolveProfile(string(modelconfig.ProfileCostSaver)).Models, false, nil
	}

	if _, _, _, err := resolveInstallModels(cmd, &buf, r, cfg, false, "cost-saver", selector); err != nil {
		t.Fatalf("resolveInstallModels() error = %v", err)
	}
	if gotInitial != modelconfig.ProfileCostSaver {
		t.Fatalf("selector received initial profile = %q, want %q (--profile flag must be threaded into the interactive picker)", gotInitial, modelconfig.ProfileCostSaver)
	}
}

// TestInstallCommand_NonTTY_DoesNotCreateModelArtifact verifies that a plain install leaves the
// retired Click-owned models.json artifact absent.
func TestInstallCommand_NonTTY_DoesNotCreateModelArtifact(t *testing.T) {
	home := t.TempDir()
	runner := newTestCommandRunner(home)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()

	if _, err := execRoot(t, home, "install"); err != nil {
		t.Fatalf("install command error = %v", err)
	}

	if _, err := os.Stat(installer.Config{ClaudeHome: home}.ModelsPath()); !os.IsNotExist(err) {
		t.Fatalf("models.json stat error = %v, want no Click model artifact created", err)
	}
}

// TestUpdateCommand_PreservesLegacyModelsWithoutApplyingThem verifies that update neither emits
// retired model configuration arguments nor changes an existing models.json artifact.
func TestUpdateCommand_PreservesLegacyModelsWithoutApplyingThem(t *testing.T) {
	home := t.TempDir()
	runner := newTestCommandRunner(home)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()

	if _, err := execRoot(t, home, "install"); err != nil {
		t.Fatalf("install command error = %v", err)
	}

	custom := map[modelconfig.Phase]string{
		modelconfig.PhaseExplore:           "haiku",
		modelconfig.PhasePropose:           "sonnet",
		modelconfig.PhaseSpec:              "haiku",
		modelconfig.PhaseDesign:            "sonnet",
		modelconfig.PhaseTasks:             "haiku",
		modelconfig.PhaseApply:             "sonnet",
		modelconfig.PhaseVerify:            "haiku",
		modelconfig.PhaseArchive:           "sonnet",
		modelconfig.PhaseOnboard:           "sonnet",
		modelconfig.PhaseJDJudgeA:          "haiku",
		modelconfig.PhaseJDJudgeB:          "haiku",
		modelconfig.PhaseJDFixAgent:        "haiku",
		modelconfig.PhaseReviewRisk:        "opus",
		modelconfig.PhaseReviewReadability: "sonnet",
		modelconfig.PhaseReviewReliability: "haiku",
		modelconfig.PhaseReviewResilience:  "opus",
		modelconfig.PhaseReviewRefuter:     "sonnet",
		modelconfig.PhaseDefault:           "haiku",
	}
	if err := installer.SaveModelsWithProfile(installer.Config{ClaudeHome: home}, modelconfig.ProfileCustom, custom); err != nil {
		t.Fatalf("SaveModelsWithProfile() error = %v", err)
	}
	cfg := installer.Config{ClaudeHome: home}
	originalModels, err := os.ReadFile(cfg.ModelsPath())
	if err != nil {
		t.Fatalf("ReadFile(models.json) before update: %v", err)
	}

	if _, err := execRoot(t, home, "update"); err != nil {
		t.Fatalf("update command error = %v", err)
	}

	wantCommand := "claude plugin install click-sdd@click-ai-devkit" +
		" --config orchestration_profile=custom" +
		" --config explore_model=haiku" +
		" --config propose_model=sonnet" +
		" --config spec_model=haiku" +
		" --config design_model=sonnet" +
		" --config tasks_model=haiku" +
		" --config apply_model=sonnet" +
		" --config verify_model=haiku" +
		" --config archive_model=sonnet" +
		" --config onboard_model=sonnet" +
		" --config jd_judge_a_model=haiku" +
		" --config jd_judge_b_model=haiku" +
		" --config jd_fix_agent_model=haiku" +
		" --config review_risk_model=opus" +
		" --config review_readability_model=sonnet" +
		" --config review_reliability_model=haiku" +
		" --config review_resilience_model=opus" +
		" --config review_refuter_model=sonnet" +
		" --config default_model=haiku"
	if contains(runner.commands, wantCommand) {
		t.Fatalf("update command sequence = %#v, unexpectedly reapplied retired model settings %q", runner.commands, wantCommand)
	}
	for _, command := range runner.commands {
		if strings.Contains(command, "--config") {
			t.Fatalf("update emitted retired model config arguments in %q", command)
		}
	}
	gotModels, err := os.ReadFile(cfg.ModelsPath())
	if err != nil || !bytes.Equal(gotModels, originalModels) {
		t.Fatalf("models.json after update = %q (err %v), want exact preservation", gotModels, err)
	}
}

// TestUpdateCommand_PreservesStaleModelsWithoutMigration verifies that update leaves stale legacy
// model bytes untouched, does not create a migration backup, and emits no retired model arguments.
func TestUpdateCommand_PreservesStaleModelsWithoutMigration(t *testing.T) {
	home := t.TempDir()
	runner := newTestCommandRunner(home)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()
	binaryPath := filepath.Join(t.TempDir(), "engram.exe")
	if err := os.WriteFile(binaryPath, []byte("binary"), 0o644); err != nil {
		t.Fatalf("WriteFile(binary) error = %v", err)
	}
	t.Setenv("CLICK_ENGRAM_BINARY_PATH", binaryPath)

	cfg := installer.Config{ClaudeHome: home}
	legacyRaw, err := json.Marshal(map[string]string{
		"orchestrator":   "haiku",
		"prd_writer":     "opus",
		"architect":      "opus",
		"reviewer":       "opus",
		"memory_curator": "sonnet",
	})
	if err != nil {
		t.Fatalf("json.Marshal(legacy) error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(cfg.ModelsPath()), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(cfg.ModelsPath(), legacyRaw, 0o600); err != nil {
		t.Fatalf("WriteFile(legacy models.json) error = %v", err)
	}

	if _, err := execRoot(t, home, "update"); err != nil {
		t.Fatalf("update command error = %v", err)
	}

	got, err := os.ReadFile(cfg.ModelsPath())
	if err != nil || !bytes.Equal(got, legacyRaw) {
		t.Fatalf("models.json = %q (err %v), want original stale bytes preserved", got, err)
	}
	if _, err := os.Stat(cfg.ModelsPath() + ".bak"); !os.IsNotExist(err) {
		t.Fatalf("models.json.bak stat error = %v, want no migration backup", err)
	}
	for _, command := range runner.commands {
		if strings.Contains(command, "--config") {
			t.Fatalf("update emitted retired model config arguments in %q", command)
		}
	}
}

// TestUpdateCommand_PreservesLegacyProfileArtifact guards update's non-ownership of legacy model
// state while still syncing the non-model runtime assets.
func TestUpdateCommand_PreservesLegacyProfileArtifact(t *testing.T) {
	home := t.TempDir()
	runner := newTestCommandRunner(home)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()
	cfg := installer.Config{ClaudeHome: home}
	if err := installer.SaveModelsWithProfile(cfg, modelconfig.ProfileQuality, modelconfig.ResolveProfile(string(modelconfig.ProfileQuality)).Models); err != nil {
		t.Fatalf("SaveModelsWithProfile() error = %v", err)
	}
	before, err := os.ReadFile(cfg.ModelsPath())
	if err != nil {
		t.Fatalf("ReadFile(models.json) before update: %v", err)
	}

	if _, err := execRoot(t, home, "update"); err != nil {
		t.Fatalf("update command error = %v", err)
	}
	after, err := os.ReadFile(cfg.ModelsPath())
	if err != nil || !bytes.Equal(after, before) {
		t.Fatalf("models.json after update = %q (err %v), want exact preservation", after, err)
	}
	for _, command := range runner.commands {
		if strings.Contains(command, "--config") {
			t.Fatalf("update emitted retired model config arguments in %q", command)
		}
	}
}

func TestUpdateCommand_PreservesPartialLegacyModelsJson(t *testing.T) {
	seedResolvableEngram(t)
	home := t.TempDir()
	runner := newTestCommandRunner(home)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()

	cfg := installer.Config{ClaudeHome: home}
	partial := oldThirteenPhaseModelsForProfile(modelconfig.ProfileCostSaver)
	if err := installer.SaveModelsWithProfile(cfg, modelconfig.ProfileCostSaver, partial); err != nil {
		t.Fatalf("SaveModelsWithProfile(partial cost-saver) error = %v", err)
	}
	before, err := os.ReadFile(cfg.ModelsPath())
	if err != nil {
		t.Fatalf("ReadFile(models.json) before update: %v", err)
	}

	if _, err := execRoot(t, home, "update"); err != nil {
		t.Fatalf("update command error = %v", err)
	}

	after, err := os.ReadFile(cfg.ModelsPath())
	if err != nil || !bytes.Equal(after, before) {
		t.Fatalf("models.json after update = %q (err %v), want partial legacy model artifact preserved", after, err)
	}
}

func TestDoctorCommand_IgnoresMalformedRetiredModelMetadata(t *testing.T) {
	seedResolvableEngram(t)
	seedResolvableClickBinary(t)
	home := t.TempDir()
	runner := newTestCommandRunner(home)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()
	if _, err := execRoot(t, home, "install"); err != nil {
		t.Fatalf("install command error = %v", err)
	}
	cfg := installer.Config{ClaudeHome: home}
	if err := os.MkdirAll(filepath.Dir(cfg.ModelsPath()), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg.ModelsPath(), []byte("{malformed legacy model data"), 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(cfg.ModelsPath())
	if err != nil {
		t.Fatalf("ReadFile(models.json) before doctor: %v", err)
	}
	out, err := execRoot(t, home, "doctor")
	if err != nil {
		t.Fatalf("doctor must ignore retired model metadata, got %v; output:\n%s", err, out)
	}
	after, err := os.ReadFile(cfg.ModelsPath())
	if err != nil {
		t.Fatalf("ReadFile(models.json) after doctor: %v", err)
	}
	if !bytes.Equal(after, before) {
		t.Fatalf("doctor changed retired models.json bytes; want exact preservation\nbefore: %q\nafter:  %q", before, after)
	}
	for _, retired := range []string{"Modelos por fase de click-sdd", "Perfil de orquestación"} {
		if strings.Contains(out, retired) {
			t.Errorf("doctor output contains retired model readout %q:\n%s", retired, out)
		}
	}
}

func contains(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}

func oldThirteenPhaseModelsForProfile(profile modelconfig.ProfileName) map[modelconfig.Phase]string {
	models := modelconfig.ResolveForProfile(string(profile), nil)
	delete(models, modelconfig.PhaseReviewRisk)
	delete(models, modelconfig.PhaseReviewReadability)
	delete(models, modelconfig.PhaseReviewReliability)
	delete(models, modelconfig.PhaseReviewResilience)
	delete(models, modelconfig.PhaseReviewRefuter)
	return models
}

func TestRendererFor_NoColorFlagForcesPlain(t *testing.T) {
	root := NewRootCommand()
	root.SetArgs([]string{"install", "--no-color"})
	root.PersistentFlags().Parse([]string{"--no-color"})

	var buf bytes.Buffer
	r := rendererFor(root, &buf)
	if r.Color {
		t.Fatal("rendererFor() with --no-color parsed produced a color-enabled renderer")
	}
}

// manifestCheckingRunner wraps another installer.CommandRunner and records, on its FIRST Run call
// only, whether cfg's run-start snapshot manifest.json already existed on disk at that moment — the
// install-reliability-foundation ordering invariant (spec's "Single Run-Start Snapshot Before Any
// Write" requirement): SnapshotRun must complete before the first external `claude` subprocess
// invocation, in both install.go's and update.go's actual step 1.
type manifestCheckingRunner struct {
	installer.CommandRunner
	cfg             installer.Config
	checkedFirstRun bool
	manifestExisted bool
}

func (r *manifestCheckingRunner) Run(name string, args ...string) error {
	if !r.checkedFirstRun {
		r.checkedFirstRun = true
		_, statErr := os.Stat(filepath.Join(r.cfg.BackupDir(), "latest", "manifest.json"))
		r.manifestExisted = statErr == nil
	}
	return r.CommandRunner.Run(name, args...)
}

// TestInstallCommand_SnapshotTakenBeforeFirstClaudeCommand is task 2.5's RED test: it proves
// SnapshotRun completes before the very first `claude` subprocess invocation SyncMarketplacePlugins
// issues — covering settings.json's second writer (the external claude CLI), not just the two
// direct file writes install.go itself performs.
func TestInstallCommand_SnapshotTakenBeforeFirstClaudeCommand(t *testing.T) {
	home := t.TempDir()
	stateHome := t.TempDir()
	cfg := installer.Config{ClaudeHome: home, ClickStateHome: stateHome}
	base := newTestCommandRunner(home)
	runner := &manifestCheckingRunner{CommandRunner: base, cfg: cfg}
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()

	if _, err := execRootWithStateHome(t, home, stateHome, "install"); err != nil {
		t.Fatalf("install command error = %v", err)
	}
	if !runner.checkedFirstRun {
		t.Fatal("no claude command was ever issued during install, want at least one so the ordering invariant is actually exercised")
	}
	if !runner.manifestExisted {
		t.Fatal("manifest.json did not exist before the first claude command fired — SnapshotRun must complete before step 1")
	}
}

// TestUpdateCommand_SnapshotTakenBeforeFirstClaudeCommand mirrors the install-side ordering
// invariant test above for `click update`.
func TestUpdateCommand_SnapshotTakenBeforeFirstClaudeCommand(t *testing.T) {
	home := t.TempDir()
	stateHome := t.TempDir()
	cfg := installer.Config{ClaudeHome: home, ClickStateHome: stateHome}
	base := newTestCommandRunner(home)
	runner := &manifestCheckingRunner{CommandRunner: base, cfg: cfg}
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()
	binaryPath := filepath.Join(t.TempDir(), "engram.exe")
	if err := os.WriteFile(binaryPath, []byte("binary"), 0o644); err != nil {
		t.Fatalf("WriteFile(binary) error = %v", err)
	}
	t.Setenv("CLICK_ENGRAM_BINARY_PATH", binaryPath)

	if _, err := execRootWithStateHome(t, home, stateHome, "update"); err != nil {
		t.Fatalf("update command error = %v", err)
	}
	if !runner.checkedFirstRun {
		t.Fatal("no claude command was ever issued during update, want at least one so the ordering invariant is actually exercised")
	}
	if !runner.manifestExisted {
		t.Fatal("manifest.json did not exist before the first claude command fired — SnapshotRun must complete before step 1")
	}
}

// TestInstallCommand_NonTTY_SkipsPreviewPrompt is task 2.3's RED test for install: execRoot always
// wires a bytes.Buffer (never a real terminal) as out, so isNonInteractiveInstall must resolve true
// and the confirm prompt must never appear.
func TestInstallCommand_NonTTY_SkipsPreviewPrompt(t *testing.T) {
	home := t.TempDir()
	runner := newTestCommandRunner(home)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()

	out, err := execRoot(t, home, "install")
	if err != nil {
		t.Fatalf("install command error = %v", err)
	}
	if strings.Contains(out, "¿Continuar?") {
		t.Fatalf("install output = %q, want no confirm prompt on non-TTY output", out)
	}
}

// TestUpdateCommand_NonTTY_SkipsPreviewPrompt mirrors the install-side non-TTY test above for
// `click update`.
func TestUpdateCommand_NonTTY_SkipsPreviewPrompt(t *testing.T) {
	home := t.TempDir()
	runner := newTestCommandRunner(home)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()

	out, err := execRoot(t, home, "update")
	if err != nil {
		t.Fatalf("update command error = %v", err)
	}
	if strings.Contains(out, "¿Continuar?") {
		t.Fatalf("update output = %q, want no confirm prompt on non-TTY output", out)
	}
}

// TestInstallCommand_TakesRunStartSnapshotBeforeWrites guards the install-backup capability
// end-to-end through the real CLI command: after `click install`, a completed run-start snapshot
// must exist.
func TestInstallCommand_TakesRunStartSnapshotBeforeWrites(t *testing.T) {
	home := t.TempDir()
	runner := newTestCommandRunner(home)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()

	if _, err := execRoot(t, home, "install"); err != nil {
		t.Fatalf("install command error = %v", err)
	}
	has, err := installer.HasRunSnapshot(installer.Config{ClaudeHome: home, ClickStateHome: os.Getenv("CLICK_STATE_HOME")})
	if err != nil {
		t.Fatalf("HasRunSnapshot() error = %v", err)
	}
	if !has {
		t.Fatal("HasRunSnapshot() = false after `click install`, want true")
	}
}

// TestUpdateCommand_TakesRunStartSnapshotBeforeWrites mirrors the install-side snapshot test above
// for `click update`.
func TestUpdateCommand_TakesRunStartSnapshotBeforeWrites(t *testing.T) {
	home := t.TempDir()
	runner := newTestCommandRunner(home)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()

	if _, err := execRoot(t, home, "update"); err != nil {
		t.Fatalf("update command error = %v", err)
	}
	has, err := installer.HasRunSnapshot(installer.Config{ClaudeHome: home, ClickStateHome: os.Getenv("CLICK_STATE_HOME")})
	if err != nil {
		t.Fatalf("HasRunSnapshot() error = %v", err)
	}
	if !has {
		t.Fatal("HasRunSnapshot() = false after `click update`, want true")
	}
}

// TestTestCommandRunner_writeSettings_PreservesForeignKeys proves that the fake claude
// CLI double (testCommandRunner.writeSettings) preserves unrelated top-level keys when
// writing settings.json — a merge fix (teardown-rollback-hardening) replaced the
// double's original wholesale-write behavior, which used to destroy them.
func TestTestCommandRunner_writeSettings_PreservesForeignKeys(t *testing.T) {
	home := t.TempDir()
	runner := newTestCommandRunner(home)

	// Seed a pre-existing settings.json with foreign keys
	settingsPath := filepath.Join(home, "settings.json")
	initialSettings := map[string]any{
		"enabledPlugins": map[string]any{"existing-plugin": true},
		"foreignKey":     "must survive",
		"anotherForeign": map[string]any{"nested": "value"},
		"hooks": map[string]any{
			"PreToolUse": []any{
				map[string]any{"matcher": "foreign-matcher", "hooks": []any{}},
			},
		},
	}
	settingsData, err := json.MarshalIndent(initialSettings, "", "  ")
	if err != nil {
		t.Fatalf("marshal initial settings: %v", err)
	}
	if err := os.WriteFile(settingsPath, settingsData, 0o600); err != nil {
		t.Fatalf("write initial settings: %v", err)
	}

	// Register a plugin and let the fake runner write settings
	runner.plugins["test-plugin@marketplace"] = true

	// This is the writeSettings call that currently overwrites wholesale
	if err := runner.writeSettings(); err != nil {
		t.Fatalf("writeSettings() error = %v", err)
	}

	// Read back what was written
	var after map[string]any
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings after writeSettings: %v", err)
	}
	if err := json.Unmarshal(data, &after); err != nil {
		t.Fatalf("unmarshal settings after writeSettings: %v", err)
	}

	// RED: foreign keys are destroyed by the current wholesale-write implementation
	if after["foreignKey"] != "must survive" {
		t.Fatalf("foreignKey = %v after writeSettings, want \"must survive\" (current wholesale-write destroys foreign keys)", after["foreignKey"])
	}
	if after["anotherForeign"] == nil {
		t.Fatalf("anotherForeign = <nil> after writeSettings, want map{\"nested\": \"value\"} (current wholesale-write destroys foreign keys)")
	}
	if after["hooks"] == nil {
		t.Fatalf("hooks = <nil> after writeSettings, want foreign hooks to survive (current wholesale-write destroys foreign keys)")
	}
}
