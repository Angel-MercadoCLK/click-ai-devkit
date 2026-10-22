package installer

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"

	toml "github.com/pelletier/go-toml/v2"
)

const (
	context7MCPURL     = "https://mcp.context7.com/mcp"
	context7MCPVersion = "2.2.5"
	context7MCPCommand = "npx"
)

var gentleAIEngramLookPath = exec.LookPath

// ValidateGentleAIDestinationRoots blocks a provisioning run when Click's selected
// homes differ from the pinned upstream installer's fixed user-home destinations.
// The upstream v4.0.0 installer has no per-target root flags, so continuing would
// inspect one tree and mutate another.
func ValidateGentleAIDestinationRoots(cfg Config, selection TargetSelection) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("installer: no se pudo resolver el directorio de usuario para gentle-ai: %w", err)
	}
	return validateGentleAIDestinationRoots(cfg, selection, home)
}

func validateGentleAIDestinationRoots(cfg Config, selection TargetSelection, home string) error {
	if home == "" || !filepath.IsAbs(home) {
		return fmt.Errorf("installer: gentle-ai requiere un directorio de usuario absoluto; no se ejecutó la instalación")
	}
	checks := []struct {
		selected bool
		actual   string
		expected string
		agent    string
	}{
		{selection.Claude, cfg.ClaudeHome, filepath.Join(home, ".claude"), "Claude Code"},
		{selection.Codex, cfg.CodexHome, filepath.Join(home, ".codex"), "Codex"},
		{selection.OpenClaw, cfg.OpenClawHome, filepath.Join(home, ".openclaw"), "OpenClaw"},
	}
	for _, check := range checks {
		if !check.selected {
			continue
		}
		if check.actual == "" || filepath.Clean(check.actual) != filepath.Clean(check.expected) {
			return fmt.Errorf("installer: gentle-ai v4.0.0 no admite el directorio personalizado de %s de Click; no se ejecutó nada para evitar modificar otro perfil. Use el directorio de usuario estándar o una versión de gentle-ai con opciones de destino explícitas", check.agent)
		}
	}
	return nil
}

// PreflightGentleAIRegistrations refuses to let the pinned upstream installer upsert a
// same-name server whose contents Click cannot prove are already the upstream configuration.
// It is read-only and scoped to selected targets; exact known registrations are safe no-ops.
func PreflightGentleAIRegistrations(cfg Config, selection TargetSelection) error {
	if selection.Claude {
		if cfg.ClaudeHome == "" || !filepath.IsAbs(cfg.ClaudeHome) {
			return missingGentleAIHome("Claude Code")
		}
		if err := checkClaudeMCPRegistrations(cfg); err != nil {
			return err
		}
	}
	if selection.Codex {
		if cfg.CodexHome == "" || !filepath.IsAbs(cfg.CodexHome) {
			return missingGentleAIHome("Codex")
		}
		if err := checkCodexMCPRegistrations(cfg.CodexConfigPath()); err != nil {
			return err
		}
	}
	if selection.OpenClaw {
		if cfg.OpenClawHome == "" || !filepath.IsAbs(cfg.OpenClawHome) {
			return missingGentleAIHome("OpenClaw")
		}
		if err := checkOpenClawMCPRegistrations(cfg.OpenClawMCPConfigPath()); err != nil {
			return err
		}
	}
	return nil
}

func checkClaudeMCPRegistrations(cfg Config) error {
	path := cfg.Context7ConfigPath()
	root, err := readJSONConfig(path)
	if err != nil {
		return err
	}
	servers, exists := root["mcpServers"]
	if exists {
		var ok bool
		servers, ok = servers.(map[string]any)
		if !ok {
			return malformedRegistrationConfig("Claude Code", path, "mcpServers no es un objeto")
		}
	} else {
		servers = map[string]any{}
	}
	entries := servers.(map[string]any)
	if entry, ok := entries["engram"]; ok && !matchesEngramJSONEntry(entry) {
		return registrationConflict("Claude Code", "engram", path)
	}
	if entry, ok := entries["context7"]; ok {
		if matchesLocalContext7Entry(entry) {
			return nil
		}
		if matchesLegacyContext7HTTPEntry(entry) || matchesClickMigratedContext7Entry(entry) {
			state, found, stateErr := loadContext7State(cfg)
			if stateErr != nil {
				return fmt.Errorf("installer: no se pudo verificar la propiedad del Context7 heredado en %s; no se modificó nada: %w", cfg.Context7StatePath(), stateErr)
			}
			if found && state.InstalledByClick {
				return nil
			}
		}
		return registrationConflict("Claude Code", "context7", path)
	}
	return nil
}

// HasGentleAIEngramMCP reports whether Claude's user-scope config contains the
// direct server shape written by gentle-ai. It does not infer plugin ownership.
func HasGentleAIEngramMCP(cfg Config) (bool, error) {
	if cfg.ClaudeHome == "" || !filepath.IsAbs(cfg.ClaudeHome) {
		return false, missingGentleAIHome("Claude Code")
	}
	root, err := readJSONConfig(cfg.Context7ConfigPath())
	if err != nil {
		return false, err
	}
	serversValue, exists := root["mcpServers"]
	if !exists {
		return false, nil
	}
	servers, ok := serversValue.(map[string]any)
	if !ok {
		return false, malformedRegistrationConfig("Claude Code", cfg.Context7ConfigPath(), "mcpServers no es un objeto")
	}
	entry, exists := servers["engram"]
	return exists && matchesEngramJSONEntry(entry), nil
}

func checkCodexMCPRegistrations(path string) error {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("installer: no se pudo leer la configuración MCP de Codex: %w", err)
	}
	var root map[string]any
	if err := toml.Unmarshal(data, &root); err != nil {
		return fmt.Errorf("installer: no se pudo clasificar la configuración MCP de Codex; no se modificó: %w", err)
	}
	serversValue, exists := root["mcp_servers"]
	servers := map[string]any{}
	if exists {
		var ok bool
		servers, ok = serversValue.(map[string]any)
		if !ok {
			return malformedRegistrationConfig("Codex", path, "mcp_servers no es una tabla")
		}
	}
	if entry, ok := servers["engram"]; ok && !matchesEngramTOMLEntry(entry) {
		return registrationConflict("Codex", "engram", path)
	}
	if entry, ok := servers["context7"]; ok && !matchesContext7TOMLEntry(entry) {
		return registrationConflict("Codex", "context7", path)
	}
	return nil
}

func checkOpenClawMCPRegistrations(path string) error {
	root, err := readJSONConfig(path)
	if err != nil {
		return err
	}
	legacyValue, legacyExists := root["mcpServers"]
	legacyServers := map[string]any{}
	if legacyExists {
		var ok bool
		legacyServers, ok = legacyValue.(map[string]any)
		if !ok {
			return malformedRegistrationConfig("OpenClaw", path, "mcpServers heredado no es un objeto")
		}
	}
	mcpValue, mcpExists := root["mcp"]
	mcp := map[string]any{}
	if mcpExists {
		var ok bool
		mcp, ok = mcpValue.(map[string]any)
		if !ok {
			return malformedRegistrationConfig("OpenClaw", path, "mcp no es un objeto")
		}
	}
	servers := map[string]any{}
	serversValue, serversExists := mcp["servers"]
	if serversExists {
		var ok bool
		servers, ok = serversValue.(map[string]any)
		if !ok {
			return malformedRegistrationConfig("OpenClaw", path, "mcp.servers no es un objeto")
		}
	}
	for name, legacy := range legacyServers {
		if nested, duplicated := servers[name]; duplicated && !reflect.DeepEqual(legacy, nested) {
			return registrationConflict("OpenClaw", name, path)
		}
	}
	if entry, ok := servers["engram"]; ok && !matchesEngramJSONEntry(entry) {
		return registrationConflict("OpenClaw", "engram", path)
	}
	if entry, ok := servers["context7"]; ok && !matchesLocalContext7Entry(entry) {
		return registrationConflict("OpenClaw", "context7", path)
	}
	if entry, ok := legacyServers["engram"]; ok && !matchesEngramJSONEntry(entry) {
		return registrationConflict("OpenClaw", "engram", path)
	}
	if entry, ok := legacyServers["context7"]; ok && !matchesLocalContext7Entry(entry) {
		return registrationConflict("OpenClaw", "context7", path)
	}
	return nil
}

func readJSONConfig(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return map[string]any{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("installer: no se pudo leer la configuración MCP (%s): %w", path, err)
	}
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil || root == nil {
		return nil, fmt.Errorf("installer: no se pudo clasificar la configuración MCP (%s); no se modificó: revisa el JSON y vuelve a ejecutar `click install` o `click update`", path)
	}
	return root, nil
}

// matchesEngramJSONEntry accepts the server gentle-ai writes ({command, args}) and the same server as
// Claude Code stores it after `claude mcp add`, with "type": "stdio" and an empty "env". Any other key,
// a non-empty env or another type is user-owned state.
func matchesEngramJSONEntry(value any) bool {
	entry, ok := value.(map[string]any)
	if !ok || !stringArrayEquals(entry["args"], []string{"mcp", "--tools=agent"}) {
		return false
	}
	for key, field := range entry {
		switch key {
		case "command", "args":
		case "type":
			if field != "stdio" {
				return false
			}
		case "env":
			if env, isMap := field.(map[string]any); !isMap || len(env) != 0 {
				return false
			}
		default:
			return false
		}
	}
	command, ok := entry["command"].(string)
	if !ok {
		return false
	}
	if command == "engram" {
		return true
	}
	if !filepath.IsAbs(command) {
		return false
	}
	resolved, err := gentleAIEngramLookPath("engram")
	return err == nil && filepath.Clean(command) == filepath.Clean(resolved)
}

func matchesEngramTOMLEntry(value any) bool { return matchesEngramJSONEntry(value) }

func stringArrayEquals(value any, expected []string) bool {
	actual, ok := value.([]any)
	if !ok || len(actual) != len(expected) {
		return false
	}
	for i := range expected {
		if actual[i] != expected[i] {
			return false
		}
	}
	return true
}

func matchesLocalContext7Entry(value any) bool {
	entry, ok := value.(map[string]any)
	return ok && len(entry) == 2 && entry["command"] == context7MCPCommand && stringArrayEquals(entry["args"], []string{"-y", "--package=@upstash/context7-mcp@" + context7MCPVersion, "--", "context7-mcp"})
}

func matchesLegacyContext7HTTPEntry(value any) bool {
	entry, ok := value.(map[string]any)
	return ok && len(entry) == 2 && entry["type"] == "http" && entry["url"] == context7MCPURL
}

func matchesClickMigratedContext7Entry(value any) bool {
	entry, ok := value.(map[string]any)
	return ok && len(entry) == 4 && entry["type"] == "http" && entry["url"] == context7MCPURL && matchesLocalContext7Entry(map[string]any{"command": entry["command"], "args": entry["args"]})
}

func matchesContext7TOMLEntry(value any) bool {
	entry, ok := value.(map[string]any)
	return ok && len(entry) == 1 && entry["url"] == context7MCPURL
}

func missingGentleAIHome(agent string) error {
	return fmt.Errorf("installer: falta el directorio aislado de %s; no se inspeccionó ninguna ruta relativa. Configure su directorio y vuelva a ejecutar `click install` o `click update`", agent)
}

func malformedRegistrationConfig(agent, path, reason string) error {
	return fmt.Errorf("installer: no se pudo clasificar la configuración MCP de %s en %s (%s); no se modificó nada. Corrija la estructura y vuelva a ejecutar `click install` o `click update`", agent, path, reason)
}

func registrationConflict(agent, name, path string) error {
	return fmt.Errorf("installer: %s ya tiene un servidor MCP %q distinto del bloque de gentle-ai en %s; no se modificó nada. Revísalo manualmente y vuelve a ejecutar `click install` o `click update`", agent, name, path)
}
