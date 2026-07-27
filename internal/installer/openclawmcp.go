package installer

import (
	"fmt"
	"path/filepath"
	"strings"
)

// engramOpenClawMCPName is the MCP server name Engram is registered under in OpenClaw's own state
// (`openclaw mcp add/show/unset <name>`), matching the "engram" identifier this package already uses
// elsewhere (Claude's engram@engram plugin, Codex's engram MCP server).
const engramOpenClawMCPName = "engram"

var openClawMCPQualificationTokens = []string{"add", "--command", "--arg"}

// SyncOpenClawMCP registers Engram as an MCP server in OpenClaw's own CLI state, using the exact
// confirmed real syntax from the OpenClaw CLI:
//
//	openclaw mcp add <NAME> --command <EXE> --arg <ARG> --arg=--tools=agent
//
// This function makes ZERO file writes — it is 100% CLI delegation via the injected CommandRunner. It
// does not read or rewrite openclaw.json or any other OpenClaw file (mirrors ConfigureOpenClawModels'
// doc style: "does not read or rewrite openclaw.json").
//
// Fail-closed qualification: before mutating, it runs `openclaw mcp add --help` and requires the
// output to contain the tokens "add", "--command", and "--arg". If the probe errors or any token is
// missing, it returns a wrapped error and issues no mutation — guarding against an OpenClaw CLI whose
// contract differs from the assumed one.
//
// Idempotency is checked via `openclaw mcp show engram`: when already registered, this function
// returns nil immediately without re-adding. When show errors, registration proceeds.
//
// Fail-stop: if `add` errors, that error is wrapped and returned, never swallowed — the caller
// (install.go/update.go) decides whether this is fatal; for click's own D45 "supplementary
// integrations are non-fatal" pattern, callers surface a warning and continue instead of aborting.
//
// It is a no-op (nil) when cfg.OpenClawHome is empty, mirroring every other Sync* no-op guard in this
// package.
func SyncOpenClawMCP(cfg Config) error {
	if cfg.OpenClawHome == "" {
		return nil
	}

	path, ok := OpenClawPath()
	if !ok {
		return fmt.Errorf("installer: OpenClaw CLI is not available; install OpenClaw and re-run `click install`/`click update` to register Engram's MCP server")
	}
	if !filepath.IsAbs(path) {
		abs, err := filepath.Abs(path)
		if err != nil {
			return fmt.Errorf("installer: resolve absolute OpenClaw binary path: %w", err)
		}
		path = abs
	}

	runner := commandRunnerFactory()

	out, err := runner.Output(path, "mcp", "add", "--help")
	if err != nil {
		return fmt.Errorf("installer: OpenClaw MCP registration contract qualification failed: %w", err)
	}
	evidence := strings.TrimSpace(string(out))
	for _, token := range openClawMCPQualificationTokens {
		if !strings.Contains(evidence, token) {
			return fmt.Errorf("installer: OpenClaw MCP registration contract probe returned unexpected output; missing %q in evidence %q", token, evidence)
		}
	}

	if _, err := runner.Output(path, "mcp", "show", engramOpenClawMCPName); err == nil {
		// Already registered — `openclaw mcp show <NAME>` succeeds once registered, so there is nothing
		// to add.
		return nil
	}

	if err := runner.Run(path, "mcp", "add", engramOpenClawMCPName, "--command", "engram", "--arg", "mcp", "--arg=--tools=agent"); err != nil {
		return fmt.Errorf("installer: register Engram MCP server with OpenClaw: %w", err)
	}
	return nil
}

// RemoveOpenClawMCP deregisters Engram's MCP server from OpenClaw's own CLI state —
// SyncOpenClawMCP's reversal — using the confirmed real syntax `openclaw mcp unset <NAME>`. Like
// SyncOpenClawMCP it makes ZERO file writes: it is 100% CLI delegation via the injected CommandRunner
// and never reads or rewrites openclaw.json.
//
// Idempotency reuses SyncOpenClawMCP's exact membership probe: `openclaw mcp show engram` ERRORS when
// the server is not registered and SUCCEEDS when it is. When show errors, there is nothing to remove
// and this returns nil without issuing `mcp unset`. When show succeeds, it runs
// `openclaw mcp unset engram`.
//
// Fail-stop: an `unset` error is wrapped and returned, never swallowed — the caller (uninstall.go)
// decides whether it is fatal; for click's own D45 "supplementary integrations are non-fatal"
// pattern, uninstall is resilient-continue and simply records the failure in its summary.
//
// It is a no-op (nil) when cfg.OpenClawHome is empty, mirroring SyncOpenClawMCP's own guard.
func RemoveOpenClawMCP(cfg Config) error {
	if cfg.OpenClawHome == "" {
		return nil
	}

	path, ok := OpenClawPath()
	if !ok {
		return fmt.Errorf("installer: OpenClaw CLI is not available; install OpenClaw and re-run `click uninstall` to deregister Engram's MCP server")
	}
	if !filepath.IsAbs(path) {
		abs, err := filepath.Abs(path)
		if err != nil {
			return fmt.Errorf("installer: resolve absolute OpenClaw binary path: %w", err)
		}
		path = abs
	}

	runner := commandRunnerFactory()
	if _, err := runner.Output(path, "mcp", "show", engramOpenClawMCPName); err != nil {
		// Not registered — `openclaw mcp show <NAME>` errors when absent, so there is nothing to remove.
		return nil
	}

	if err := runner.Run(path, "mcp", "unset", engramOpenClawMCPName); err != nil {
		return fmt.Errorf("installer: deregister Engram MCP server from OpenClaw: %w", err)
	}
	return nil
}
