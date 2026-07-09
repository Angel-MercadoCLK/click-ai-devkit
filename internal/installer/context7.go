// Context7 install support.
//
// Context7 (https://context7.com) is a third-party HTTP MCP server providing up-to-date
// library/framework documentation lookup. Unlike Engram (a Claude Code PLUGIN registered through
// `claude plugin ...`), Context7 has no plugin to install — it is registered directly as an MCP
// server through the native `claude mcp` CLI. Verified against the real CLI in this slice's Step 0
// (documentacion/spikes/spike-g-context7.md):
//
//	claude mcp add --transport http --scope user context7 https://mcp.context7.com/mcp
//
// `--scope user` matters: it is NOT the default scope (the default, with no --scope flag, is
// local/project-scoped and tied to the current working directory). `--scope user` registers a
// durable, cross-project entry — the same reach Engram's plugin registration has — written to
// Claude Code's own user config file at `<CLAUDE_CONFIG_DIR>/.claude.json`'s top-level
// `mcpServers` key (confirmed by reading that file directly before/after the real command ran).
//
// Reading that same file back (HasContext7) is how presence is probed — a pure filesystem read,
// deliberately mirroring HasInstalledPluginID's approach (installer/plugins.go) rather than
// shelling out to `claude mcp get` for the check itself. This keeps `click doctor` free of any
// subprocess execution, matching every other check in the doctor package, and keeps presence
// checks deterministic in unit tests without needing every caller to inject a fake CommandRunner.
package installer

import (
	"encoding/json"
	"fmt"
	"os"
)

const context7ServerURL = "https://mcp.context7.com/mcp"

// context7State is click's own bookkeeping about the Context7 MCP install: whether click itself
// registered it, as opposed to a developer having already run `claude mcp add ... context7`
// independently. RemoveContext7 reads InstalledByClick to decide whether it's safe to remove —
// click only ever reverses what it added. Mirrors engramState's exact ownership-tracking shape
// (internal/installer/engram.go).
type context7State struct {
	InstalledByClick bool `json:"installed_by_click"`
}

// SyncContext7 registers Context7 as a user-scope HTTP MCP server via the native `claude mcp add`
// CLI — unless it is already present (per HasContext7), in which case it is left completely
// untouched. It always (re)writes click's own state bookkeeping file so `click doctor` and `click
// uninstall` can report on, and respect, Context7's install ownership. Returns
// alreadyPresent=true when Context7 was already registered before this call.
//
// Ownership (InstalledByClick) is decided exactly once — the first time SyncContext7 ever runs
// against a given ClaudeHome — and preserved on every later call. This mirrors the exact guard
// SyncEngram uses against the same bug class (see engram.go's SyncEngram doc comment and
// TestSyncEngram_SecondRunPreservesClickOwnership): naively deriving ownership from
// "!alreadyPresent" on every call would flip a click-owned install to "pre-existing" the moment
// click's OWN prior install makes it look already-there, and RemoveContext7 would then wrongly
// refuse to remove something click actually added.
func SyncContext7(cfg Config) (alreadyPresent bool, err error) {
	alreadyPresent, err = HasContext7(cfg)
	if err != nil {
		return false, err
	}

	installedByClick := !alreadyPresent
	existing, found, err := loadContext7State(cfg)
	if err != nil {
		return alreadyPresent, err
	}
	if found {
		installedByClick = existing.InstalledByClick
	}

	if !alreadyPresent {
		runner := commandRunnerFactory()
		if err := addContext7(runner); err != nil {
			return alreadyPresent, err
		}
	}

	state := context7State{InstalledByClick: installedByClick}
	if err := writeJSONFile(cfg.Context7StatePath(), state); err != nil {
		return alreadyPresent, fmt.Errorf("installer: write context7 state: %w", err)
	}
	return alreadyPresent, nil
}

// RemoveContext7 reverses SyncContext7, but ONLY when click's own state says click registered
// Context7 in the first place. If a developer already had Context7 configured before running
// `click install`, click uninstall leaves it running untouched — click only ever removes what it
// added. It is idempotent: safe to call when Context7 was never touched by click, or has already
// been removed.
func RemoveContext7(cfg Config) error {
	state, found, err := loadContext7State(cfg)
	if err != nil {
		return err
	}
	if !found {
		// click's SyncContext7 never ran against this home — nothing click-managed to reverse.
		return nil
	}
	if !state.InstalledByClick {
		// click never owned this registration; leave Context7 alone, just drop click's own
		// bookkeeping.
		return removeContext7State(cfg)
	}
	present, err := HasContext7(cfg)
	if err != nil {
		return err
	}
	if present {
		runner := commandRunnerFactory()
		if err := removeContext7(runner); err != nil {
			return err
		}
	}
	return removeContext7State(cfg)
}

// HasContext7 reports whether Context7 is currently registered as a user-scope MCP server, by
// reading Claude Code's own user config file directly (cfg.Context7ConfigPath) rather than
// shelling out to `claude mcp get context7`. See this file's package doc comment for why: it keeps
// the check a pure filesystem read, safe to call from click doctor's read-only checks without ever
// invoking the real `claude` CLI in a unit test.
func HasContext7(cfg Config) (bool, error) {
	data, err := os.ReadFile(cfg.Context7ConfigPath())
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("installer: read claude user config: %w", err)
	}
	var parsed struct {
		MCPServers map[string]json.RawMessage `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return false, fmt.Errorf("installer: parse claude user config: %w", err)
	}
	_, ok := parsed.MCPServers["context7"]
	return ok, nil
}

func addContext7(runner CommandRunner) error {
	if err := runner.Run(pluginCLIBinary, "mcp", "add", "--transport", "http", "--scope", "user", "context7", context7ServerURL); err != nil {
		return fmt.Errorf("installer: add context7 mcp server: %w", err)
	}
	return nil
}

func removeContext7(runner CommandRunner) error {
	if err := runner.Run(pluginCLIBinary, "mcp", "remove", "context7"); err != nil {
		return fmt.Errorf("installer: remove context7 mcp server: %w", err)
	}
	return nil
}

func loadContext7State(cfg Config) (context7State, bool, error) {
	data, err := os.ReadFile(cfg.Context7StatePath())
	if err != nil {
		if os.IsNotExist(err) {
			return context7State{}, false, nil
		}
		return context7State{}, false, fmt.Errorf("installer: read context7 state: %w", err)
	}
	var state context7State
	if err := json.Unmarshal(data, &state); err != nil {
		return context7State{}, false, fmt.Errorf("installer: parse context7 state: %w", err)
	}
	return state, true, nil
}

func removeContext7State(cfg Config) error {
	if err := removeIfExists(cfg.Context7StatePath()); err != nil {
		return fmt.Errorf("installer: remove context7 state: %w", err)
	}
	return nil
}
