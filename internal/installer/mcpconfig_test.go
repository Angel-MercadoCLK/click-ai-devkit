package installer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/manifest"
)

func TestConfigureEngramMCP_WritesAbsoluteBinaryPathAndState(t *testing.T) {
	claudeHome := t.TempDir()
	cfg := Config{ClaudeHome: claudeHome}
	binaryPath := filepath.Join(t.TempDir(), "engram.exe")
	if err := os.WriteFile(binaryPath, []byte("binary"), 0o644); err != nil {
		t.Fatalf("WriteFile(binary) error = %v", err)
	}
	t.Setenv("CLICK_ENGRAM_BINARY_PATH", binaryPath)

	m, err := manifest.Load()
	if err != nil {
		t.Fatalf("manifest.Load() error = %v", err)
	}

	if err := ConfigureEngramMCP(cfg, m); err != nil {
		t.Fatalf("ConfigureEngramMCP() error = %v", err)
	}

	mcpData, err := os.ReadFile(cfg.EngramMCPConfigPath())
	if err != nil {
		t.Fatalf("ReadFile(EngramMCPConfigPath) error = %v", err)
	}
	if !filepath.IsAbs(binaryPath) {
		t.Fatal("test binary path is not absolute")
	}
	if string(mcpData) == "" {
		t.Fatal("mcp config is empty")
	}

	type serverConfig struct {
		Command string   `json:"command"`
		Args    []string `json:"args"`
	}
	type mcpConfig struct {
		MCPServers map[string]serverConfig `json:"mcpServers"`
	}
	var got mcpConfig
	if err := json.Unmarshal(mcpData, &got); err != nil {
		t.Fatalf("json.Unmarshal(mcp config) error = %v", err)
	}
	engram, ok := got.MCPServers["engram"]
	if !ok {
		t.Fatal("mcpServers.engram missing")
	}
	if engram.Command != binaryPath {
		t.Fatalf("engram command = %q, want %q", engram.Command, binaryPath)
	}

	stateData, err := os.ReadFile(cfg.EngramStatePath())
	if err != nil {
		t.Fatalf("ReadFile(EngramStatePath) error = %v", err)
	}
	type state struct {
		Version    string `json:"version"`
		BinaryPath string `json:"binary_path"`
	}
	var gotState state
	if err := json.Unmarshal(stateData, &gotState); err != nil {
		t.Fatalf("json.Unmarshal(state) error = %v", err)
	}
	if gotState.Version != m.Engram.Version {
		t.Fatalf("state version = %q, want %q", gotState.Version, m.Engram.Version)
	}
	if gotState.BinaryPath != binaryPath {
		t.Fatalf("state binary path = %q, want %q", gotState.BinaryPath, binaryPath)
	}
}

// TestRemoveEngramMCP_RemovesConfigAndState covers the asymmetry `click update` introduces:
// ConfigureEngramMCP (only ever called by `click update`, never by `click install`) writes the MCP
// config and the click-managed state file, but nothing previously removed them again. Uninstall
// must fully reverse anything install *or* update wrote, so RemoveEngramMCP must clean up both
// files, and must be a no-op (not an error) when they were never written.
func TestRemoveEngramMCP_RemovesConfigAndState(t *testing.T) {
	claudeHome := t.TempDir()
	cfg := Config{ClaudeHome: claudeHome}
	binaryPath := filepath.Join(t.TempDir(), "engram.exe")
	if err := os.WriteFile(binaryPath, []byte("binary"), 0o644); err != nil {
		t.Fatalf("WriteFile(binary) error = %v", err)
	}
	t.Setenv("CLICK_ENGRAM_BINARY_PATH", binaryPath)

	m, err := manifest.Load()
	if err != nil {
		t.Fatalf("manifest.Load() error = %v", err)
	}
	if err := ConfigureEngramMCP(cfg, m); err != nil {
		t.Fatalf("ConfigureEngramMCP() error = %v", err)
	}

	if err := RemoveEngramMCP(cfg); err != nil {
		t.Fatalf("RemoveEngramMCP() error = %v", err)
	}

	if _, err := os.Stat(cfg.EngramMCPConfigPath()); !os.IsNotExist(err) {
		t.Errorf("RemoveEngramMCP() left the MCP config behind at %s (err = %v)", cfg.EngramMCPConfigPath(), err)
	}
	if _, err := os.Stat(cfg.EngramStatePath()); !os.IsNotExist(err) {
		t.Errorf("RemoveEngramMCP() left the state file behind at %s (err = %v)", cfg.EngramStatePath(), err)
	}
}

func TestRemoveEngramMCP_NoopWhenNeverConfigured(t *testing.T) {
	claudeHome := t.TempDir()
	cfg := Config{ClaudeHome: claudeHome}

	if err := RemoveEngramMCP(cfg); err != nil {
		t.Fatalf("RemoveEngramMCP() on a never-configured home error = %v, want nil", err)
	}
}
