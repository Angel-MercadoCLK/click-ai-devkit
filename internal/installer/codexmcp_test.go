package installer

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"
)

// codexMCPTestRunner is a minimal CommandRunner fake mirroring openClawModelTestRunner
// (openclawmodel_test.go): it records every Run/Output invocation and lets a test script a
// canned Output error (simulating `codex mcp get` success/failure) and a canned Run error
// (simulating `codex mcp add` success/failure).
type codexMCPTestRunner struct {
	commands [][]string
	outputs  [][]string
	getErr   error
	runErr   error
}

func (r *codexMCPTestRunner) Run(name string, args ...string) error {
	r.commands = append(r.commands, append([]string{name}, args...))
	return r.runErr
}

func (r *codexMCPTestRunner) Output(name string, args ...string) ([]byte, error) {
	r.outputs = append(r.outputs, append([]string{name}, args...))
	return nil, r.getErr
}

// codexMCPTestLookup mirrors openClawModelTestLookup (openclawmodel_test.go) for CodexPath's own
// binaryLookupFactory seam.
type codexMCPTestLookup struct {
	available bool
}

func (l codexMCPTestLookup) LookPath(name string) (string, error) {
	if l.available && name == "codex" {
		return "/fake/codex", nil
	}
	return "", errors.New("not found")
}

// TestSyncCodexMCP_NoOpWhenCodexHomeEmpty is the no-op guard shared by every Sync* function in this
// package: an empty cfg.CodexHome must issue zero commands.
func TestSyncCodexMCP_NoOpWhenCodexHomeEmpty(t *testing.T) {
	runner := &codexMCPTestRunner{}
	restoreRunner := SetCommandRunnerFactoryForTests(func() CommandRunner { return runner })
	defer restoreRunner()
	restoreLookup := SetBinaryLookupFactoryForTests(func() BinaryLookup { return codexMCPTestLookup{available: true} })
	defer restoreLookup()

	if err := SyncCodexMCP(Config{}); err != nil {
		t.Fatalf("SyncCodexMCP() error = %v, want nil", err)
	}
	if len(runner.commands) != 0 || len(runner.outputs) != 0 {
		t.Fatalf("SyncCodexMCP() issued commands with empty CodexHome, want zero: commands=%#v outputs=%#v", runner.commands, runner.outputs)
	}
}

// TestSyncCodexMCP_AlreadyRegistered_NoAddCall is the idempotency contract: when `codex mcp get
// engram` succeeds (real Codex evidence: it errors only when absent), SyncCodexMCP must not attempt
// `mcp add` at all — zero mutation attempts.
func TestSyncCodexMCP_AlreadyRegistered_NoAddCall(t *testing.T) {
	qualifiedBinary, err := filepath.Abs("/fake/codex")
	if err != nil {
		t.Fatal(err)
	}
	runner := &codexMCPTestRunner{} // getErr is nil -> `codex mcp get` "succeeds" -> already registered
	restoreRunner := SetCommandRunnerFactoryForTests(func() CommandRunner { return runner })
	defer restoreRunner()
	restoreLookup := SetBinaryLookupFactoryForTests(func() BinaryLookup { return codexMCPTestLookup{available: true} })
	defer restoreLookup()

	if err := SyncCodexMCP(Config{CodexHome: t.TempDir()}); err != nil {
		t.Fatalf("SyncCodexMCP() error = %v", err)
	}

	wantOutputs := [][]string{{qualifiedBinary, "mcp", "get", "engram"}}
	if !reflect.DeepEqual(runner.outputs, wantOutputs) {
		t.Fatalf("outputs = %#v, want %#v", runner.outputs, wantOutputs)
	}
	if len(runner.commands) != 0 {
		t.Fatalf("commands = %#v, want zero mutation attempts when already registered", runner.commands)
	}
}

// TestSyncCodexMCP_NotRegistered_IssuesExactGetThenAdd is the core real-syntax contract: when `codex
// mcp get engram` errors (real Codex evidence: "Error: No MCP server named 'engram' found."),
// SyncCodexMCP must issue exactly `codex mcp add engram -- engram mcp --tools=agent` — the same two
// EXACT commands confirmed against the real CLI, in order (get first, then add), with no invented
// flags.
func TestSyncCodexMCP_NotRegistered_IssuesExactGetThenAdd(t *testing.T) {
	qualifiedBinary, err := filepath.Abs("/fake/codex")
	if err != nil {
		t.Fatal(err)
	}
	runner := &codexMCPTestRunner{getErr: errors.New("Error: No MCP server named 'engram' found.")}
	restoreRunner := SetCommandRunnerFactoryForTests(func() CommandRunner { return runner })
	defer restoreRunner()
	restoreLookup := SetBinaryLookupFactoryForTests(func() BinaryLookup { return codexMCPTestLookup{available: true} })
	defer restoreLookup()

	if err := SyncCodexMCP(Config{CodexHome: t.TempDir()}); err != nil {
		t.Fatalf("SyncCodexMCP() error = %v", err)
	}

	wantOutputs := [][]string{{qualifiedBinary, "mcp", "get", "engram"}}
	if !reflect.DeepEqual(runner.outputs, wantOutputs) {
		t.Fatalf("outputs = %#v, want %#v", runner.outputs, wantOutputs)
	}
	wantCommands := [][]string{{qualifiedBinary, "mcp", "add", "engram", "--", "engram", "mcp", "--tools=agent"}}
	if !reflect.DeepEqual(runner.commands, wantCommands) {
		t.Fatalf("commands = %#v, want exactly %#v", runner.commands, wantCommands)
	}
}

// TestSyncCodexMCP_AddFailure_ReturnsWrappedError is the fail-stop contract: `codex mcp add` errors
// are wrapped and returned, never swallowed.
func TestSyncCodexMCP_AddFailure_ReturnsWrappedError(t *testing.T) {
	wantErr := errors.New("codex rejected the mcp add call")
	runner := &codexMCPTestRunner{getErr: errors.New("not found"), runErr: wantErr}
	restoreRunner := SetCommandRunnerFactoryForTests(func() CommandRunner { return runner })
	defer restoreRunner()
	restoreLookup := SetBinaryLookupFactoryForTests(func() BinaryLookup { return codexMCPTestLookup{available: true} })
	defer restoreLookup()

	err := SyncCodexMCP(Config{CodexHome: t.TempDir()})
	if !errors.Is(err, wantErr) {
		t.Fatalf("SyncCodexMCP() error = %v, want wrapped %v", err, wantErr)
	}
}

// TestSyncCodexMCP_CodexUnavailable_ReturnsClearError guards the missing-binary path: no `codex` on
// PATH must return a clear, wrapped error and issue zero commands (mirrors
// TestConfigureOpenClawModels_OpenClawAbsent_DoesNotRunCommands).
func TestSyncCodexMCP_CodexUnavailable_ReturnsClearError(t *testing.T) {
	runner := &codexMCPTestRunner{}
	restoreRunner := SetCommandRunnerFactoryForTests(func() CommandRunner { return runner })
	defer restoreRunner()
	restoreLookup := SetBinaryLookupFactoryForTests(func() BinaryLookup { return codexMCPTestLookup{} })
	defer restoreLookup()

	err := SyncCodexMCP(Config{CodexHome: t.TempDir()})
	if err == nil {
		t.Fatal("SyncCodexMCP() error = nil, want an error when Codex is unavailable")
	}
	if len(runner.commands) != 0 || len(runner.outputs) != 0 {
		t.Fatalf("SyncCodexMCP() issued commands with Codex unavailable, want zero: commands=%#v outputs=%#v", runner.commands, runner.outputs)
	}
}
