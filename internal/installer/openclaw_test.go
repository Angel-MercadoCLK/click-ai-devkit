package installer

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/modelconfig"
)

// --- SyncOpenClawWorkspace (tasks 2.1-2.4) ---

// TestSyncOpenClawWorkspace_FirstWrite_CreatesAgentsAndSoulWithManagedBlock is the RED test for
// task 2.1: SyncOpenClawWorkspace does not exist until 2.4's GREEN change. First-ever write must
// create both files with a well-formed managed block (spec's "First-ever write" scenario).
func TestSyncOpenClawWorkspace_FirstWrite_CreatesAgentsAndSoulWithManagedBlock(t *testing.T) {
	cfg := Config{OpenClawHome: t.TempDir()}

	if err := SyncOpenClawWorkspace(cfg); err != nil {
		t.Fatalf("SyncOpenClawWorkspace() error = %v, want nil", err)
	}

	agentsHas, err := HasManagedBlock(cfg.OpenClawAgentsMDPath())
	if err != nil {
		t.Fatalf("HasManagedBlock(AGENTS.md) error = %v", err)
	}
	if !agentsHas {
		t.Fatal("HasManagedBlock(AGENTS.md) = false, want true after first-ever SyncOpenClawWorkspace")
	}
	soulHas, err := HasManagedBlock(cfg.OpenClawSoulMDPath())
	if err != nil {
		t.Fatalf("HasManagedBlock(SOUL.md) error = %v", err)
	}
	if !soulHas {
		t.Fatal("HasManagedBlock(SOUL.md) = false, want true after first-ever SyncOpenClawWorkspace")
	}
}

// TestSyncOpenClawWorkspace_Rerun_NoDuplicateBlock is task 2.2's RED test: re-running with an
// existing managed block must not create a duplicate block or a spurious diff — the second run's
// byte content must equal the first run's, exactly like WriteManagedBlock's own idempotency
// contract (claudemd_test.go).
func TestSyncOpenClawWorkspace_Rerun_NoDuplicateBlock(t *testing.T) {
	cfg := Config{OpenClawHome: t.TempDir()}

	if err := SyncOpenClawWorkspace(cfg); err != nil {
		t.Fatalf("SyncOpenClawWorkspace() (first run) error = %v", err)
	}
	firstAgents, err := os.ReadFile(cfg.OpenClawAgentsMDPath())
	if err != nil {
		t.Fatalf("ReadFile(AGENTS.md) after first run error = %v", err)
	}
	firstSoul, err := os.ReadFile(cfg.OpenClawSoulMDPath())
	if err != nil {
		t.Fatalf("ReadFile(SOUL.md) after first run error = %v", err)
	}

	if err := SyncOpenClawWorkspace(cfg); err != nil {
		t.Fatalf("SyncOpenClawWorkspace() (second run) error = %v", err)
	}
	secondAgents, err := os.ReadFile(cfg.OpenClawAgentsMDPath())
	if err != nil {
		t.Fatalf("ReadFile(AGENTS.md) after second run error = %v", err)
	}
	secondSoul, err := os.ReadFile(cfg.OpenClawSoulMDPath())
	if err != nil {
		t.Fatalf("ReadFile(SOUL.md) after second run error = %v", err)
	}

	if string(firstAgents) != string(secondAgents) {
		t.Fatalf("AGENTS.md changed on re-run:\nfirst:  %q\nsecond: %q", firstAgents, secondAgents)
	}
	if string(firstSoul) != string(secondSoul) {
		t.Fatalf("SOUL.md changed on re-run:\nfirst:  %q\nsecond: %q", firstSoul, secondSoul)
	}
	if strings.Count(string(secondAgents), managedBeginMarker) != 1 {
		t.Fatalf("AGENTS.md contains %d begin markers after re-run, want exactly 1 (no duplicate block)", strings.Count(string(secondAgents), managedBeginMarker))
	}
}

// TestSyncOpenClawWorkspace_PreservesHandWrittenContentOutsideMarkers is task 2.3's RED test: a
// developer's own content outside the managed markers must survive byte-for-byte, exactly like
// WriteManagedBlock's own contract for CLAUDE.md.
func TestSyncOpenClawWorkspace_PreservesHandWrittenContentOutsideMarkers(t *testing.T) {
	cfg := Config{OpenClawHome: t.TempDir()}
	handWritten := "# My own AGENTS.md notes\nDo not touch this line.\n"
	writeTestFile(t, cfg.OpenClawAgentsMDPath(), handWritten)

	if err := SyncOpenClawWorkspace(cfg); err != nil {
		t.Fatalf("SyncOpenClawWorkspace() error = %v", err)
	}

	got, err := os.ReadFile(cfg.OpenClawAgentsMDPath())
	if err != nil {
		t.Fatalf("ReadFile(AGENTS.md) error = %v", err)
	}
	if !strings.Contains(string(got), "Do not touch this line.") {
		t.Fatalf("AGENTS.md = %q, want the pre-existing hand-written content preserved", got)
	}
	if !strings.Contains(string(got), managedBeginMarker) {
		t.Fatalf("AGENTS.md = %q, want the managed block appended", got)
	}
}

// TestSyncOpenClawWorkspace_Absent_NoOp guards the skip-on-absent contract: an empty
// cfg.OpenClawHome must write nothing at all, defense-in-depth alongside the CLI-level skip.
func TestSyncOpenClawWorkspace_Absent_NoOp(t *testing.T) {
	cfg := Config{}

	if err := SyncOpenClawWorkspace(cfg); err != nil {
		t.Fatalf("SyncOpenClawWorkspace() error = %v, want nil when OpenClawHome is empty", err)
	}
}

// --- SyncOpenClawMCPConfig (tasks 2.5-2.8) ---

// TestSyncOpenClawMCPConfig_FirstRegistration_AddsEngramEntry is task 2.5's RED test: no existing
// entry -> the engram entry is added.
func TestSyncOpenClawMCPConfig_FirstRegistration_AddsEngramEntry(t *testing.T) {
	cfg := Config{OpenClawHome: t.TempDir()}

	if err := SyncOpenClawMCPConfig(cfg); err != nil {
		t.Fatalf("SyncOpenClawMCPConfig() error = %v", err)
	}

	data, err := os.ReadFile(cfg.OpenClawMCPConfigPath())
	if err != nil {
		t.Fatalf("ReadFile(openclaw.json) error = %v", err)
	}
	var parsed struct {
		MCPServers map[string]struct {
			Command   string   `json:"command"`
			Args      []string `json:"args"`
			Transport string   `json:"transport"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("json.Unmarshal(openclaw.json) error = %v", err)
	}
	entry, ok := parsed.MCPServers["engram"]
	if !ok {
		t.Fatalf("openclaw.json mcpServers = %#v, want an \"engram\" entry", parsed.MCPServers)
	}
	if entry.Command != "engram" {
		t.Fatalf("engram entry command = %q, want %q", entry.Command, "engram")
	}
	if entry.Transport != "stdio" {
		t.Fatalf("engram entry transport = %q, want %q", entry.Transport, "stdio")
	}
}

// TestSyncOpenClawMCPConfig_PreservesOtherMCPServersAndTopLevelKeys triangulates 2.5's happy path
// with an existing file that has UNRELATED mcpServers entries AND an unrelated top-level key —
// both must survive untouched (RawMessage passthrough, the design's core decision).
func TestSyncOpenClawMCPConfig_PreservesOtherMCPServersAndTopLevelKeys(t *testing.T) {
	cfg := Config{OpenClawHome: t.TempDir()}
	writeTestFile(t, cfg.OpenClawMCPConfigPath(), `{
		"someOtherKey": {"nested": true},
		"mcpServers": {"otherTool": {"command": "othertool", "args": ["serve"], "transport": "stdio"}}
	}`)

	if err := SyncOpenClawMCPConfig(cfg); err != nil {
		t.Fatalf("SyncOpenClawMCPConfig() error = %v", err)
	}

	data, err := os.ReadFile(cfg.OpenClawMCPConfigPath())
	if err != nil {
		t.Fatalf("ReadFile(openclaw.json) error = %v", err)
	}
	var parsed map[string]json.RawMessage
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("json.Unmarshal(openclaw.json) error = %v", err)
	}
	if _, ok := parsed["someOtherKey"]; !ok {
		t.Fatalf("openclaw.json top level = %#v, want unrelated key \"someOtherKey\" preserved", parsed)
	}
	var servers map[string]json.RawMessage
	if err := json.Unmarshal(parsed["mcpServers"], &servers); err != nil {
		t.Fatalf("json.Unmarshal(mcpServers) error = %v", err)
	}
	if _, ok := servers["otherTool"]; !ok {
		t.Fatalf("mcpServers = %#v, want unrelated entry \"otherTool\" preserved", servers)
	}
	if _, ok := servers["engram"]; !ok {
		t.Fatalf("mcpServers = %#v, want \"engram\" entry added", servers)
	}
}

// TestSyncOpenClawMCPConfig_Rerun_Idempotent is task 2.6's RED test: running twice must produce
// byte-identical output and never duplicate the engram entry.
func TestSyncOpenClawMCPConfig_Rerun_Idempotent(t *testing.T) {
	cfg := Config{OpenClawHome: t.TempDir()}
	writeTestFile(t, cfg.OpenClawMCPConfigPath(), `{"mcpServers": {"otherTool": {"command": "othertool"}}}`)

	if err := SyncOpenClawMCPConfig(cfg); err != nil {
		t.Fatalf("SyncOpenClawMCPConfig() (first run) error = %v", err)
	}
	first, err := os.ReadFile(cfg.OpenClawMCPConfigPath())
	if err != nil {
		t.Fatalf("ReadFile() after first run error = %v", err)
	}

	if err := SyncOpenClawMCPConfig(cfg); err != nil {
		t.Fatalf("SyncOpenClawMCPConfig() (second run) error = %v", err)
	}
	second, err := os.ReadFile(cfg.OpenClawMCPConfigPath())
	if err != nil {
		t.Fatalf("ReadFile() after second run error = %v", err)
	}

	if string(first) != string(second) {
		t.Fatalf("openclaw.json changed on re-run:\nfirst:  %s\nsecond: %s", first, second)
	}
}

// TestSyncOpenClawMCPConfig_MissingFile_CreatesNewFileWithOnlyEngramEntry is task 2.7's RED test:
// no openclaw.json exists yet -> a new file is created containing only the engram entry.
func TestSyncOpenClawMCPConfig_MissingFile_CreatesNewFileWithOnlyEngramEntry(t *testing.T) {
	cfg := Config{OpenClawHome: t.TempDir()}
	if _, err := os.Stat(cfg.OpenClawMCPConfigPath()); !os.IsNotExist(err) {
		t.Fatalf("Stat(openclaw.json) error = %v, want the file to not exist yet", err)
	}

	if err := SyncOpenClawMCPConfig(cfg); err != nil {
		t.Fatalf("SyncOpenClawMCPConfig() error = %v", err)
	}

	data, err := os.ReadFile(cfg.OpenClawMCPConfigPath())
	if err != nil {
		t.Fatalf("ReadFile(openclaw.json) error = %v, want the file created", err)
	}
	var parsed struct {
		MCPServers map[string]json.RawMessage `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("json.Unmarshal(openclaw.json) error = %v", err)
	}
	if len(parsed.MCPServers) != 1 {
		t.Fatalf("mcpServers = %#v, want exactly 1 entry (engram only)", parsed.MCPServers)
	}
	if _, ok := parsed.MCPServers["engram"]; !ok {
		t.Fatal("mcpServers has no \"engram\" entry")
	}
}

// TestSyncOpenClawMCPConfig_Absent_NoOp mirrors TestSyncOpenClawWorkspace_Absent_NoOp for the MCP
// config sync — defense in depth alongside the CLI-level skip.
func TestSyncOpenClawMCPConfig_Absent_NoOp(t *testing.T) {
	cfg := Config{}

	if err := SyncOpenClawMCPConfig(cfg); err != nil {
		t.Fatalf("SyncOpenClawMCPConfig() error = %v, want nil when OpenClawHome is empty", err)
	}
}

func TestSyncOpenClawModelProfile_WritesPortableRecommendation(t *testing.T) {
	cfg := Config{OpenClawHome: t.TempDir()}
	models := modelconfig.ResolveProfile(string(modelconfig.ProfileCostSaver)).Models
	if err := SyncOpenClawModelProfile(cfg, modelconfig.ProfileCostSaver, models); err != nil {
		t.Fatalf("SyncOpenClawModelProfile() error = %v", err)
	}
	profile, got, found, err := LoadModelProfile(cfg.OpenClawModelProfilePath())
	if err != nil || !found || profile != modelconfig.ProfileCostSaver {
		t.Fatalf("LoadModelProfile() = (%q, %#v, %v), err = %v", profile, got, found, err)
	}
	if len(got) != len(models) {
		t.Fatalf("profile model count = %d, want %d", len(got), len(models))
	}
}

func TestSyncOpenClawModelProfile_Absent_NoOp(t *testing.T) {
	if err := SyncOpenClawModelProfile(Config{}, modelconfig.ProfileBalanced, modelconfig.Defaults()); err != nil {
		t.Fatalf("SyncOpenClawModelProfile() error = %v, want nil", err)
	}
}
