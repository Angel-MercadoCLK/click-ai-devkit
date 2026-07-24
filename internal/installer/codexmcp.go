package installer

import (
	"fmt"
	"path/filepath"
)

// engramCodexMCPName is the MCP server name Engram is registered under in Codex's own state
// (`codex mcp add/get <name>`), matching the "engram" identifier this package already uses
// elsewhere (OpenClaw's servers["engram"], Claude's engram@engram plugin).
const engramCodexMCPName = "engram"

// SyncCodexMCP registers Engram as an MCP server in Codex's own CLI state, using the exact confirmed
// real syntax from a live Codex CLI: `codex mcp add <NAME> -- <COMMAND>...`. This function makes
// ZERO file writes — it is 100% CLI delegation via the injected CommandRunner. It does not read or
// rewrite config.toml or any other Codex file (mirrors ConfigureOpenClawModels' doc style: "does not
// read or rewrite config.toml").
//
// Idempotency is checked via `codex mcp get engram`: a real Codex confirms this returns
// "Error: No MCP server named '<NAME>' found." (a non-nil error from the runner) when absent, and
// succeeds when the server is already registered. That observed behavior is used as the sole
// membership/idempotency check here, because `add`'s behavior on a duplicate name was NOT confirmed
// against the real CLI and must not be guessed. When already registered, this function returns nil
// immediately without re-adding.
//
// Fail-stop: if `add` errors, that error is wrapped and returned, never swallowed — the caller
// (install.go/update.go) decides whether this is fatal; for click's own D45 "supplementary
// integrations are non-fatal" pattern, callers surface a warning and continue instead of aborting.
//
// It is a no-op (nil) when cfg.CodexHome is empty, mirroring every other Sync* no-op guard in this
// package.
func SyncCodexMCP(cfg Config) error {
	if cfg.CodexHome == "" {
		return nil
	}

	path, ok := CodexPath()
	if !ok {
		return fmt.Errorf("installer: Codex CLI is not available; install Codex and re-run `click install`/`click update` to register Engram's MCP server")
	}
	if !filepath.IsAbs(path) {
		abs, err := filepath.Abs(path)
		if err != nil {
			return fmt.Errorf("installer: resolve absolute Codex binary path: %w", err)
		}
		path = abs
	}

	runner := commandRunnerFactory()
	if _, err := runner.Output(path, "mcp", "get", engramCodexMCPName); err == nil {
		// Already registered — real Codex evidence: `codex mcp get <NAME>` succeeds once registered,
		// so there is nothing to add.
		return nil
	}

	if err := runner.Run(path, "mcp", "add", engramCodexMCPName, "--", "engram", "mcp", "--tools=agent"); err != nil {
		return fmt.Errorf("installer: register Engram MCP server with Codex: %w", err)
	}
	return nil
}
