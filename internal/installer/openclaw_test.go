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

// --- SyncOpenClawMCPConfig (cleanup-only rewrite, urgent fix) ---
//
// SyncOpenClawMCPConfig used to WRITE a top-level "mcpServers" key into openclaw.json to register
// Engram. Real evidence from a live OpenClaw instance's `validate config` proved that key does not
// exist in OpenClaw's real schema ("Unrecognized key: \"mcpServers\""), and every write corrupted
// the user's real config. The function is now cleanup-only: it NEVER writes that key, and only
// removes it when a previous (broken) click run already wrote it.

// TestSyncOpenClawMCPConfig_MissingFile_NoOp proves the cleanup-only contract's most important
// guardrail: when openclaw.json does not exist yet, SyncOpenClawMCPConfig must never create it —
// there is nothing to clean, and this step is cleanup-only, never creation.
func TestSyncOpenClawMCPConfig_MissingFile_NoOp(t *testing.T) {
	cfg := Config{OpenClawHome: t.TempDir()}
	if _, err := os.Stat(cfg.OpenClawMCPConfigPath()); !os.IsNotExist(err) {
		t.Fatalf("Stat(openclaw.json) error = %v, want the file to not exist yet", err)
	}

	if err := SyncOpenClawMCPConfig(cfg); err != nil {
		t.Fatalf("SyncOpenClawMCPConfig() error = %v", err)
	}

	if _, err := os.Stat(cfg.OpenClawMCPConfigPath()); !os.IsNotExist(err) {
		t.Fatalf("Stat(openclaw.json) error = %v, want the file to remain absent — cleanup-only must never create it", err)
	}
}

// TestSyncOpenClawMCPConfig_NoMCPServersKey_NoWriteAtAll proves the already-clean case is a true
// no-op: an openclaw.json with no top-level "mcpServers" key is left completely untouched (verified
// by mtime, not just content) — there is nothing to clean.
func TestSyncOpenClawMCPConfig_NoMCPServersKey_NoWriteAtAll(t *testing.T) {
	cfg := Config{OpenClawHome: t.TempDir()}
	original := `{"someOtherKey": {"nested": true}}`
	writeTestFile(t, cfg.OpenClawMCPConfigPath(), original)
	before, err := os.Stat(cfg.OpenClawMCPConfigPath())
	if err != nil {
		t.Fatalf("Stat(openclaw.json) before error = %v", err)
	}

	if err := SyncOpenClawMCPConfig(cfg); err != nil {
		t.Fatalf("SyncOpenClawMCPConfig() error = %v", err)
	}

	after, err := os.Stat(cfg.OpenClawMCPConfigPath())
	if err != nil {
		t.Fatalf("Stat(openclaw.json) after error = %v", err)
	}
	if !after.ModTime().Equal(before.ModTime()) {
		t.Fatalf("openclaw.json mtime changed from %v to %v, want untouched (no \"mcpServers\" key to clean, so no write at all)", before.ModTime(), after.ModTime())
	}
	got, err := os.ReadFile(cfg.OpenClawMCPConfigPath())
	if err != nil {
		t.Fatalf("ReadFile(openclaw.json) error = %v", err)
	}
	if string(got) != original {
		t.Fatalf("openclaw.json content = %q, want unchanged %q", got, original)
	}
}

// TestSyncOpenClawMCPConfig_RemovesLegacyMCPServersKey is the core healing contract: a pre-existing
// top-level "mcpServers" key — matching what click used to write, complete with an "engram" entry —
// is removed entirely, and every other top-level key survives with its value semantically
// unchanged. Compared via unmarshal-and-compare, not raw bytes: key ORDER changing is fine
// (encoding/json sorts map keys), only VALUES must be identical.
func TestSyncOpenClawMCPConfig_RemovesLegacyMCPServersKey(t *testing.T) {
	cfg := Config{OpenClawHome: t.TempDir()}
	writeTestFile(t, cfg.OpenClawMCPConfigPath(), `{
		"someOtherKey": {"nested": true},
		"mcpServers": {"engram": {"command": "engram", "args": ["mcp", "--tools=agent"], "transport": "stdio"}, "otherTool": {"command": "othertool"}}
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
	if _, ok := parsed["mcpServers"]; ok {
		t.Fatalf("openclaw.json top level = %#v, want \"mcpServers\" removed entirely", parsed)
	}
	var otherKey map[string]any
	if err := json.Unmarshal(parsed["someOtherKey"], &otherKey); err != nil {
		t.Fatalf("json.Unmarshal(someOtherKey) error = %v", err)
	}
	if nested, ok := otherKey["nested"].(bool); !ok || !nested {
		t.Fatalf("someOtherKey = %#v, want {\"nested\": true} preserved semantically", otherKey)
	}
}

// TestSyncOpenClawMCPConfig_ArbitraryMCPServersValue_StillRemoved proves the healing does not
// depend on the key matching what click specifically used to write: real `validate config` evidence
// proves OpenClaw recognizes no legitimate top-level "mcpServers" key at all, so ANY value under
// that key is click's own past mistake and safe to remove unconditionally.
func TestSyncOpenClawMCPConfig_ArbitraryMCPServersValue_StillRemoved(t *testing.T) {
	cfg := Config{OpenClawHome: t.TempDir()}
	writeTestFile(t, cfg.OpenClawMCPConfigPath(), `{"mcpServers": "not even an object", "keep": 42}`)

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
	if _, ok := parsed["mcpServers"]; ok {
		t.Fatal("openclaw.json still has a \"mcpServers\" key, want it removed regardless of its value shape")
	}
	var keep float64
	if err := json.Unmarshal(parsed["keep"], &keep); err != nil || keep != 42 {
		t.Fatalf("keep = %v, err = %v, want 42 preserved", keep, err)
	}
}

// TestSyncOpenClawMCPConfig_Rerun_Idempotent is the idempotency contract: a second run against an
// already-clean file (no "mcpServers" key, because the first run just removed it) is a true no-op —
// byte-identical output, never re-adds anything.
func TestSyncOpenClawMCPConfig_Rerun_Idempotent(t *testing.T) {
	cfg := Config{OpenClawHome: t.TempDir()}
	writeTestFile(t, cfg.OpenClawMCPConfigPath(), `{"mcpServers": {"otherTool": {"command": "othertool"}}, "keep": true}`)

	if err := SyncOpenClawMCPConfig(cfg); err != nil {
		t.Fatalf("SyncOpenClawMCPConfig() (first run) error = %v", err)
	}
	first, err := os.ReadFile(cfg.OpenClawMCPConfigPath())
	if err != nil {
		t.Fatalf("ReadFile() after first run error = %v", err)
	}
	if strings.Contains(string(first), "mcpServers") {
		t.Fatalf("openclaw.json after first run = %s, want \"mcpServers\" removed", first)
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

// TestSyncOpenClawMCPConfig_Absent_NoOp mirrors TestSyncOpenClawWorkspace_Absent_NoOp for the MCP
// config cleanup — defense in depth alongside the CLI-level skip.
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
