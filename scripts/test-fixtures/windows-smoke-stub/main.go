// Command windows-smoke-stub provides hermetic executables for the Windows package smoke test.
// It intentionally uses only the standard library and refuses unknown upstream install arguments.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
)

const pinnedVersion = "v4.0.0"

func main() {
	name := strings.TrimSuffix(strings.ToLower(filepath.Base(os.Args[0])), ".exe")
	var err error
	if name == "gentle-ai" {
		err = runGentleAI(os.Args[1:])
	} else if name == "go" {
		err = runGoStub(os.Args[1:])
	} else {
		err = runCommandStub(name, os.Args[1:])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
}

func runGentleAI(args []string) error {
	if len(args) == 1 && args[0] == "version" {
		fmt.Printf("gentle-ai %s\n", pinnedVersion)
		return nil
	}
	want := []string{
		"install", "--agents", "claude-code,codex,openclaw",
		"--preset", "full-gentleman",
		"--components", "engram,skills,context7,permissions,persona",
		"--persona", "neutral", "--scope", "global",
	}
	if !reflect.DeepEqual(args, want) {
		return fmt.Errorf("unexpected gentle-ai arguments: %q", args)
	}
	if os.Getenv("GENTLE_AI_NO_SELF_UPDATE") != "1" {
		return errors.New("self-update guard was not enabled")
	}
	if err := record("gentle-ai", args); err != nil {
		return err
	}
	if os.Getenv("CLICK_SMOKE_FAIL_INSTALL") == "1" {
		if err := record("gentle-ai-failure", []string{"install"}); err != nil {
			return err
		}
		return errors.New("injected upstream failure")
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return errors.New("isolated process home is unavailable")
	}
	if err := mergeClaudeMCP(filepath.Join(home, ".claude.json")); err != nil {
		return err
	}
	if err := mergeCodexMCP(filepath.Join(home, ".codex", "config.toml")); err != nil {
		return err
	}
	return mergeOpenClawMCP(filepath.Join(home, ".openclaw", "openclaw.json"))
}

func runGoStub(args []string) error {
	if err := validateGoEnvironment(); err != nil {
		return err
	}
	if err := record("go", args); err != nil {
		return err
	}
	if len(args) == 1 && args[0] == "version" {
		fmt.Println("go version go1.25.10 windows/amd64")
		return nil
	}
	want := []string{"install", "github.com/gentleman-programming/gentle-ai/v4/cmd/gentle-ai@v4.0.0"}
	if !reflect.DeepEqual(args, want) {
		return fmt.Errorf("unexpected go install arguments: %q", args)
	}
	stage := envValue("GOBIN")
	home := envValue("USERPROFILE")
	managedBin := filepath.Join(home, ".click", "bin")
	if filepath.Dir(filepath.Clean(stage)) != managedBin || !strings.HasPrefix(filepath.Base(stage), ".gentle-ai-build-") {
		return errors.New("go install GOBIN is outside isolated Click staging")
	}
	if info, err := os.Stat(stage); err != nil || !info.IsDir() {
		return errors.New("go install staging directory is unavailable")
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	data, err := os.ReadFile(executable)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(stage, "gentle-ai.exe"), data, 0o755)
}

func validateGoEnvironment() error {
	want := map[string]string{
		"GOENV": "off", "GOFLAGS": "", "GOSUMDB": "sum.golang.org", "GONOSUMDB": "none",
		"GOPRIVATE": "none", "GONOPROXY": "none", "GOINSECURE": "",
		"GOPROXY": "https://proxy.golang.org", "GOTOOLCHAIN": "local",
		"GOOS": "windows", "GOARCH": "amd64", "GENTLE_AI_NO_SELF_UPDATE": "1",
	}
	for key, value := range want {
		if got := envValue(key); got != value {
			return fmt.Errorf("unsafe Go provision environment: %s is not controlled", key)
		}
	}
	for _, key := range []string{"GONOSUMDB", "GOPRIVATE", "GONOPROXY", "GOINSECURE"} {
		if value, ok := os.LookupEnv(key); !ok || value != want[key] {
			return fmt.Errorf("unsafe Go provision environment: %s is not controlled", key)
		}
	}
	if envValue("USERPROFILE") == "" || envValue("HOME") != envValue("USERPROFILE") {
		return errors.New("isolated Go provision home is unavailable")
	}
	return nil
}

func envValue(key string) string {
	value, _ := os.LookupEnv(key)
	return value
}

func runCommandStub(name string, args []string) error {
	if err := record(name, args); err != nil {
		return err
	}
	switch name {
	case "claude":
		return runClaudeStub(args)
	case "git":
		fmt.Println("git version 2.45.1")
	case "engram", "codex", "openclaw":
		return nil
	default:
		return fmt.Errorf("unexpected smoke command %q", name)
	}
	return nil
}

func runClaudeStub(args []string) error {
	if reflect.DeepEqual(args, []string{"plugin", "list", "--json"}) {
		statePath := os.Getenv("CLICK_SMOKE_PLUGIN_STATE")
		if statePath == "" {
			return errors.New("plugin inventory path is missing")
		}
		data, err := os.ReadFile(statePath)
		if err != nil {
			return err
		}
		_, err = os.Stdout.Write(data)
		return err
	}
	if len(args) >= 3 && args[0] == "plugin" && args[1] == "install" {
		return updatePluginState(args[2], true)
	}
	if len(args) >= 3 && args[0] == "plugin" && args[1] == "uninstall" {
		if len(args) != 6 || args[3] != "--scope" || args[4] != "user" || args[5] != "--keep-data" {
			return fmt.Errorf("unexpected plugin uninstall scope/arguments: %q", args)
		}
		return updatePluginState(args[2], false)
	}
	return nil
}

type pluginEntry struct {
	ID          string `json:"id"`
	Scope       string `json:"scope"`
	ProjectPath string `json:"projectPath,omitempty"`
	Enabled     bool   `json:"enabled"`
}

func updatePluginState(id string, add bool) error {
	statePath := os.Getenv("CLICK_SMOKE_PLUGIN_STATE")
	data, err := os.ReadFile(statePath)
	if err != nil {
		return err
	}
	var entries []pluginEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return err
	}
	filtered := make([]pluginEntry, 0, len(entries)+1)
	for _, entry := range entries {
		if entry.ID != id || entry.Scope != "user" || entry.ProjectPath != "" {
			filtered = append(filtered, entry)
		}
	}
	if add {
		filtered = append(filtered, pluginEntry{ID: id, Scope: "user", Enabled: true})
	}
	updated, err := json.MarshalIndent(filtered, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(statePath, append(updated, '\n'), 0o600)
}

func record(name string, args []string) error {
	path := os.Getenv("CLICK_SMOKE_COMMAND_LOG")
	if name == "go" {
		path = os.Getenv("CLICK_SMOKE_GO_LOG")
	}
	if path == "" {
		return nil
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprintf(f, "%s %s\n", name, strings.Join(args, " "))
	return err
}

func mergeClaudeMCP(path string) error {
	return mergeJSON(path, func(root map[string]any) {
		servers := object(root["mcpServers"])
		servers["engram"] = map[string]any{"command": "engram", "args": []any{"mcp", "--tools=agent"}}
		servers["context7"] = map[string]any{"type": "http", "url": "https://mcp.context7.com/mcp"}
		root["mcpServers"] = servers
	})
}

func mergeOpenClawMCP(path string) error {
	return mergeJSON(path, func(root map[string]any) {
		mcp := object(root["mcp"])
		servers := object(mcp["servers"])
		servers["engram"] = map[string]any{"command": "engram", "args": []any{"mcp", "--tools=agent"}}
		servers["context7"] = map[string]any{"command": "npx", "args": []any{"-y", "--package=@upstash/context7-mcp@2.2.5", "--", "context7-mcp"}}
		mcp["servers"] = servers
		root["mcp"] = mcp
	})
}

func mergeJSON(path string, update func(map[string]any)) error {
	root := map[string]any{}
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err == nil && len(strings.TrimSpace(string(data))) > 0 {
		if err := json.Unmarshal(data, &root); err != nil {
			return err
		}
	}
	update(root)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	encoded, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(encoded, '\n'), 0o600)
}

func mergeCodexMCP(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	text := string(data)
	for _, section := range []string{"[mcp_servers.engram]", "[mcp_servers.context7]"} {
		if strings.Contains(text, section) {
			continue
		}
		if text != "" && !strings.HasSuffix(text, "\n") {
			text += "\n"
		}
		text += "\n" + section + "\n"
		if strings.HasSuffix(section, "engram]") {
			text += "command = 'engram'\nargs = ['mcp', '--tools=agent']\n"
		} else {
			text += "url = 'https://mcp.context7.com/mcp'\n"
		}
	}
	return os.WriteFile(path, []byte(text), 0o600)
}

func object(value any) map[string]any {
	if current, ok := value.(map[string]any); ok && current != nil {
		return current
	}
	return map[string]any{}
}
