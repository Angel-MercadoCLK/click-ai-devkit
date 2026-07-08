package installer

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/manifest"
)

const engramBinaryPathEnvOverride = "CLICK_ENGRAM_BINARY_PATH"

type engramMCPConfig struct {
	MCPServers map[string]engramMCPServer `json:"mcpServers"`
}

type engramMCPServer struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

type engramState struct {
	Version    string `json:"version"`
	BinaryPath string `json:"binary_path"`
	Source     string `json:"source"`
}

// ConfigureEngramMCP writes a durable Claude Code MCP entry for the pinned Engram binary using an
// absolute path, plus a small Click-managed state file that records the pinned version.
func ConfigureEngramMCP(cfg Config, m *manifest.Manifest) error {
	binaryPath, err := ResolveEngramBinaryPath(cfg)
	if err != nil {
		return err
	}

	config := engramMCPConfig{MCPServers: map[string]engramMCPServer{
		"engram": {
			Command: binaryPath,
			Args:    []string{"mcp", "--tools=agent"},
		},
	}}
	if err := writeJSONFile(cfg.EngramMCPConfigPath(), config); err != nil {
		return fmt.Errorf("installer: write engram mcp config: %w", err)
	}
	state := engramState{
		Version:    m.Engram.Version,
		BinaryPath: binaryPath,
		Source:     m.Engram.Source,
	}
	if err := writeJSONFile(cfg.EngramStatePath(), state); err != nil {
		return fmt.Errorf("installer: write engram state: %w", err)
	}
	return nil
}

// ResolveEngramBinaryPath resolves the absolute path to the pinned Engram binary. v0.1 prefers a
// test/deployment override, then a PATH-resolved binary, and finally falls back to the Click-
// managed default path where a release-installed binary is expected to land. The actual binary
// download/install remains a release-pipeline TODO when network access is required.
func ResolveEngramBinaryPath(cfg Config) (string, error) {
	if override := os.Getenv(engramBinaryPathEnvOverride); override != "" {
		return filepath.Abs(override)
	}
	if path, err := exec.LookPath(engramBinaryName()); err == nil {
		return filepath.Abs(path)
	}
	return cfg.DefaultEngramBinaryPath(), nil
}

func writeJSONFile(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o600)
}

func engramBinaryName() string {
	if runtime.GOOS == "windows" {
		return "engram.exe"
	}
	return "engram"
}
