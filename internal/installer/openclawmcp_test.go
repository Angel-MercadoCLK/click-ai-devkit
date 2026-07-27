package installer

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"
)

// openClawMCPTestRunner is a minimal CommandRunner fake mirroring codexMCPTestRunner
// (codexmcp_test.go): it records every Run/Output invocation and lets a test script canned
// errors/bytes for the qualification probe (`openclaw mcp add --help`) and the idempotency probe
// (`openclaw mcp show engram`), plus a canned Run error for the mutating `mcp add`/`mcp unset`.
type openClawMCPTestRunner struct {
	commands    [][]string
	outputs     [][]string
	calls       [][]string // unified, global-order log of every Run AND Output (verify: proves cross-call ordering)
	outputBytes []byte
	probeErr    error
	showErr     error
	runErr      error
}

func (r *openClawMCPTestRunner) Run(name string, args ...string) error {
	call := append([]string{name}, args...)
	r.commands = append(r.commands, call)
	r.calls = append(r.calls, call)
	return r.runErr
}

func (r *openClawMCPTestRunner) Output(name string, args ...string) ([]byte, error) {
	call := append([]string{name}, args...)
	r.outputs = append(r.outputs, call)
	r.calls = append(r.calls, call)
	if len(args) >= 3 && args[0] == "mcp" && args[1] == "add" && args[2] == "--help" {
		return r.outputBytes, r.probeErr
	}
	return r.outputBytes, r.showErr
}

// openClawMCPTestLookup mirrors codexMCPTestLookup (codexmcp_test.go) for OpenClawPath's own
// binaryLookupFactory seam.
type openClawMCPTestLookup struct {
	available bool
}

func (l openClawMCPTestLookup) LookPath(name string) (string, error) {
	if l.available && name == "openclaw" {
		return "/fake/openclaw", nil
	}
	return "", errors.New("not found")
}

var qualifiedOpenClawHelpOutput = []byte("add --command --arg")

// TestSyncOpenClawMCP_NoOpWhenOpenClawHomeEmpty is the no-op guard shared by every Sync* function in
// this package: an empty cfg.OpenClawHome must issue zero commands.
func TestSyncOpenClawMCP_NoOpWhenOpenClawHomeEmpty(t *testing.T) {
	runner := &openClawMCPTestRunner{}
	restoreRunner := SetCommandRunnerFactoryForTests(func() CommandRunner { return runner })
	defer restoreRunner()
	restoreLookup := SetBinaryLookupFactoryForTests(func() BinaryLookup { return openClawMCPTestLookup{available: true} })
	defer restoreLookup()

	if err := SyncOpenClawMCP(Config{}); err != nil {
		t.Fatalf("SyncOpenClawMCP() error = %v, want nil", err)
	}
	if len(runner.commands) != 0 || len(runner.outputs) != 0 {
		t.Fatalf("SyncOpenClawMCP() issued commands with empty OpenClawHome, want zero: commands=%#v outputs=%#v", runner.commands, runner.outputs)
	}
}

// TestSyncOpenClawMCP_QualificationProbeMissingToken_DoesNotAdd is the fail-closed contract: when
// `openclaw mcp add --help` returns output missing a required token, SyncOpenClawMCP must error and
// must NOT issue `mcp add`.
func TestSyncOpenClawMCP_QualificationProbeMissingToken_DoesNotAdd(t *testing.T) {
	qualifiedBinary, err := filepath.Abs("/fake/openclaw")
	if err != nil {
		t.Fatal(err)
	}
	runner := &openClawMCPTestRunner{outputBytes: []byte("add --command")} // missing --arg
	restoreRunner := SetCommandRunnerFactoryForTests(func() CommandRunner { return runner })
	defer restoreRunner()
	restoreLookup := SetBinaryLookupFactoryForTests(func() BinaryLookup { return openClawMCPTestLookup{available: true} })
	defer restoreLookup()

	if err := SyncOpenClawMCP(Config{OpenClawHome: t.TempDir()}); err == nil {
		t.Fatal("SyncOpenClawMCP() error = nil, want an error when qualification probe misses a required token")
	}

	wantOutputs := [][]string{{qualifiedBinary, "mcp", "add", "--help"}}
	if !reflect.DeepEqual(runner.outputs, wantOutputs) {
		t.Fatalf("outputs = %#v, want %#v", runner.outputs, wantOutputs)
	}
	if len(runner.commands) != 0 {
		t.Fatalf("commands = %#v, want zero mutation attempts when qualification fails", runner.commands)
	}
}

// TestSyncOpenClawMCP_AlreadyRegistered_NoAddCall is the idempotency contract: when `openclaw mcp
// show engram` succeeds, SyncOpenClawMCP must not attempt `mcp add` at all — zero mutation attempts.
func TestSyncOpenClawMCP_AlreadyRegistered_NoAddCall(t *testing.T) {
	qualifiedBinary, err := filepath.Abs("/fake/openclaw")
	if err != nil {
		t.Fatal(err)
	}
	runner := &openClawMCPTestRunner{outputBytes: qualifiedOpenClawHelpOutput} // showErr nil -> show succeeds
	restoreRunner := SetCommandRunnerFactoryForTests(func() CommandRunner { return runner })
	defer restoreRunner()
	restoreLookup := SetBinaryLookupFactoryForTests(func() BinaryLookup { return openClawMCPTestLookup{available: true} })
	defer restoreLookup()

	if err := SyncOpenClawMCP(Config{OpenClawHome: t.TempDir()}); err != nil {
		t.Fatalf("SyncOpenClawMCP() error = %v", err)
	}

	wantOutputs := [][]string{
		{qualifiedBinary, "mcp", "add", "--help"},
		{qualifiedBinary, "mcp", "show", "engram"},
	}
	if !reflect.DeepEqual(runner.outputs, wantOutputs) {
		t.Fatalf("outputs = %#v, want %#v", runner.outputs, wantOutputs)
	}
	if len(runner.commands) != 0 {
		t.Fatalf("commands = %#v, want zero mutation attempts when already registered", runner.commands)
	}
}

// TestSyncOpenClawMCP_NotRegistered_IssuesExactProbeShowThenAdd is the core real-syntax contract:
// when `openclaw mcp show engram` errors (not registered), SyncOpenClawMCP must issue exactly the
// qualification probe, the show probe, then `openclaw mcp add engram --command engram --arg mcp
// --arg=--tools=agent` — no invented flags.
func TestSyncOpenClawMCP_NotRegistered_IssuesExactProbeShowThenAdd(t *testing.T) {
	qualifiedBinary, err := filepath.Abs("/fake/openclaw")
	if err != nil {
		t.Fatal(err)
	}
	runner := &openClawMCPTestRunner{outputBytes: qualifiedOpenClawHelpOutput, showErr: errors.New("Error: No MCP server named 'engram' found.")}
	restoreRunner := SetCommandRunnerFactoryForTests(func() CommandRunner { return runner })
	defer restoreRunner()
	restoreLookup := SetBinaryLookupFactoryForTests(func() BinaryLookup { return openClawMCPTestLookup{available: true} })
	defer restoreLookup()

	if err := SyncOpenClawMCP(Config{OpenClawHome: t.TempDir()}); err != nil {
		t.Fatalf("SyncOpenClawMCP() error = %v", err)
	}

	wantOutputs := [][]string{
		{qualifiedBinary, "mcp", "add", "--help"},
		{qualifiedBinary, "mcp", "show", "engram"},
	}
	if !reflect.DeepEqual(runner.outputs, wantOutputs) {
		t.Fatalf("outputs = %#v, want %#v", runner.outputs, wantOutputs)
	}
	wantCommands := [][]string{{qualifiedBinary, "mcp", "add", "engram", "--command", "engram", "--arg", "mcp", "--arg=--tools=agent"}}
	if !reflect.DeepEqual(runner.commands, wantCommands) {
		t.Fatalf("commands = %#v, want exactly %#v", runner.commands, wantCommands)
	}
	// Global cross-call order (verify criterion 8): probe --help, then show, then add — a reordered
	// mutation would change this even though the separate outputs/commands slices could look right.
	wantCalls := [][]string{
		{qualifiedBinary, "mcp", "add", "--help"},
		{qualifiedBinary, "mcp", "show", "engram"},
		{qualifiedBinary, "mcp", "add", "engram", "--command", "engram", "--arg", "mcp", "--arg=--tools=agent"},
	}
	if !reflect.DeepEqual(runner.calls, wantCalls) {
		t.Fatalf("calls (global order) = %#v, want %#v", runner.calls, wantCalls)
	}
}

// TestSyncOpenClawMCP_AddFailure_ReturnsWrappedError is the fail-stop contract: `openclaw mcp add`
// errors are wrapped and returned, never swallowed — and the attempted mutation is exactly the add.
func TestSyncOpenClawMCP_AddFailure_ReturnsWrappedError(t *testing.T) {
	qualifiedBinary, absErr := filepath.Abs("/fake/openclaw")
	if absErr != nil {
		t.Fatal(absErr)
	}
	wantErr := errors.New("openclaw rejected the mcp add call")
	runner := &openClawMCPTestRunner{outputBytes: qualifiedOpenClawHelpOutput, showErr: errors.New("not found"), runErr: wantErr}
	restoreRunner := SetCommandRunnerFactoryForTests(func() CommandRunner { return runner })
	defer restoreRunner()
	restoreLookup := SetBinaryLookupFactoryForTests(func() BinaryLookup { return openClawMCPTestLookup{available: true} })
	defer restoreLookup()

	err := SyncOpenClawMCP(Config{OpenClawHome: t.TempDir()})
	if !errors.Is(err, wantErr) {
		t.Fatalf("SyncOpenClawMCP() error = %v, want wrapped %v", err, wantErr)
	}
	// Verify criterion 8: the failing mutation must be exactly the add command, in global order.
	wantCalls := [][]string{
		{qualifiedBinary, "mcp", "add", "--help"},
		{qualifiedBinary, "mcp", "show", "engram"},
		{qualifiedBinary, "mcp", "add", "engram", "--command", "engram", "--arg", "mcp", "--arg=--tools=agent"},
	}
	if !reflect.DeepEqual(runner.calls, wantCalls) {
		t.Fatalf("calls = %#v, want %#v (add must be the exact attempted mutation)", runner.calls, wantCalls)
	}
}

// TestSyncOpenClawMCP_OpenClawUnavailable_ReturnsClearError guards the missing-binary path: no
// `openclaw` on PATH must return a clear, wrapped error and issue zero commands.
func TestSyncOpenClawMCP_OpenClawUnavailable_ReturnsClearError(t *testing.T) {
	runner := &openClawMCPTestRunner{}
	restoreRunner := SetCommandRunnerFactoryForTests(func() CommandRunner { return runner })
	defer restoreRunner()
	restoreLookup := SetBinaryLookupFactoryForTests(func() BinaryLookup { return openClawMCPTestLookup{} })
	defer restoreLookup()

	err := SyncOpenClawMCP(Config{OpenClawHome: t.TempDir()})
	if err == nil {
		t.Fatal("SyncOpenClawMCP() error = nil, want an error when OpenClaw is unavailable")
	}
	if len(runner.commands) != 0 || len(runner.outputs) != 0 {
		t.Fatalf("SyncOpenClawMCP() issued commands with OpenClaw unavailable, want zero: commands=%#v outputs=%#v", runner.commands, runner.outputs)
	}
}

// TestRemoveOpenClawMCP_NoOpWhenOpenClawHomeEmpty is RemoveOpenClawMCP's shared no-op guard: an empty
// cfg.OpenClawHome must issue zero commands.
func TestRemoveOpenClawMCP_NoOpWhenOpenClawHomeEmpty(t *testing.T) {
	runner := &openClawMCPTestRunner{}
	restoreRunner := SetCommandRunnerFactoryForTests(func() CommandRunner { return runner })
	defer restoreRunner()
	restoreLookup := SetBinaryLookupFactoryForTests(func() BinaryLookup { return openClawMCPTestLookup{available: true} })
	defer restoreLookup()

	if err := RemoveOpenClawMCP(Config{}); err != nil {
		t.Fatalf("RemoveOpenClawMCP() error = %v, want nil", err)
	}
	if len(runner.commands) != 0 || len(runner.outputs) != 0 {
		t.Fatalf("RemoveOpenClawMCP() issued commands with empty OpenClawHome, want zero: commands=%#v outputs=%#v", runner.commands, runner.outputs)
	}
}

// TestRemoveOpenClawMCP_NotRegistered_NoRemoveCall is the idempotency contract: when `openclaw mcp
// show engram` ERRORS (not registered), there is nothing to remove — so RemoveOpenClawMCP probes with
// `show` but must NOT attempt `mcp unset` at all.
func TestRemoveOpenClawMCP_NotRegistered_NoRemoveCall(t *testing.T) {
	qualifiedBinary, err := filepath.Abs("/fake/openclaw")
	if err != nil {
		t.Fatal(err)
	}
	runner := &openClawMCPTestRunner{showErr: errors.New("Error: No MCP server named 'engram' found.")}
	restoreRunner := SetCommandRunnerFactoryForTests(func() CommandRunner { return runner })
	defer restoreRunner()
	restoreLookup := SetBinaryLookupFactoryForTests(func() BinaryLookup { return openClawMCPTestLookup{available: true} })
	defer restoreLookup()

	if err := RemoveOpenClawMCP(Config{OpenClawHome: t.TempDir()}); err != nil {
		t.Fatalf("RemoveOpenClawMCP() error = %v", err)
	}

	wantOutputs := [][]string{{qualifiedBinary, "mcp", "show", "engram"}}
	if !reflect.DeepEqual(runner.outputs, wantOutputs) {
		t.Fatalf("outputs = %#v, want %#v", runner.outputs, wantOutputs)
	}
	if len(runner.commands) != 0 {
		t.Fatalf("commands = %#v, want zero unset attempts when not registered", runner.commands)
	}
}

// TestRemoveOpenClawMCP_Registered_IssuesExactUnset is the core real-syntax contract: when `openclaw
// mcp show engram` SUCCEEDS (registered), RemoveOpenClawMCP must issue exactly `openclaw mcp unset
// engram` after the show probe, in order, with no invented flags.
func TestRemoveOpenClawMCP_Registered_IssuesExactUnset(t *testing.T) {
	qualifiedBinary, err := filepath.Abs("/fake/openclaw")
	if err != nil {
		t.Fatal(err)
	}
	runner := &openClawMCPTestRunner{} // showErr is nil -> show succeeds -> registered
	restoreRunner := SetCommandRunnerFactoryForTests(func() CommandRunner { return runner })
	defer restoreRunner()
	restoreLookup := SetBinaryLookupFactoryForTests(func() BinaryLookup { return openClawMCPTestLookup{available: true} })
	defer restoreLookup()

	if err := RemoveOpenClawMCP(Config{OpenClawHome: t.TempDir()}); err != nil {
		t.Fatalf("RemoveOpenClawMCP() error = %v", err)
	}

	wantOutputs := [][]string{{qualifiedBinary, "mcp", "show", "engram"}}
	if !reflect.DeepEqual(runner.outputs, wantOutputs) {
		t.Fatalf("outputs = %#v, want %#v", runner.outputs, wantOutputs)
	}
	wantCommands := [][]string{{qualifiedBinary, "mcp", "unset", "engram"}}
	if !reflect.DeepEqual(runner.commands, wantCommands) {
		t.Fatalf("commands = %#v, want exactly %#v", runner.commands, wantCommands)
	}
}

// TestRemoveOpenClawMCP_UnsetFailure_ReturnsWrappedError is the fail-stop contract: `openclaw mcp
// unset` errors are wrapped and returned, never swallowed.
func TestRemoveOpenClawMCP_UnsetFailure_ReturnsWrappedError(t *testing.T) {
	qualifiedBinary, absErr := filepath.Abs("/fake/openclaw")
	if absErr != nil {
		t.Fatal(absErr)
	}
	wantErr := errors.New("openclaw rejected the mcp unset call")
	runner := &openClawMCPTestRunner{runErr: wantErr} // showErr nil -> registered -> unset attempted
	restoreRunner := SetCommandRunnerFactoryForTests(func() CommandRunner { return runner })
	defer restoreRunner()
	restoreLookup := SetBinaryLookupFactoryForTests(func() BinaryLookup { return openClawMCPTestLookup{available: true} })
	defer restoreLookup()

	err := RemoveOpenClawMCP(Config{OpenClawHome: t.TempDir()})
	if !errors.Is(err, wantErr) {
		t.Fatalf("RemoveOpenClawMCP() error = %v, want wrapped %v", err, wantErr)
	}
	// Verify criterion 8: the failing mutation must be exactly the unset command, after the show probe.
	wantCalls := [][]string{
		{qualifiedBinary, "mcp", "show", "engram"},
		{qualifiedBinary, "mcp", "unset", "engram"},
	}
	if !reflect.DeepEqual(runner.calls, wantCalls) {
		t.Fatalf("calls = %#v, want %#v (unset must be the exact attempted mutation)", runner.calls, wantCalls)
	}
}
