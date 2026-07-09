# Spike G — Context7 Auto-Install (v0.2 Slice, Step 0)

**Date:** 2026-07-09
**Status:** Resolved. Implemented as a `claude mcp add/get/remove` MCP-server registration —
no plugin to install (unlike Engram), no binary to provision (Context7 is a hosted HTTP MCP
server, not a local process).

All verification below was run against **throwaway `CLAUDE_CONFIG_DIR`/`CLICK_CLAUDE_HOME`
directories** under the OS temp dir, never the real `~/.claude`.

## Q1 — Does `claude mcp add --transport http --scope user context7 <url>` succeed, no auth?

**Yes.** Against a fresh throwaway `CLAUDE_CONFIG_DIR`:

```
$ claude mcp add --transport http --scope user context7 https://mcp.context7.com/mcp
Added HTTP MCP server context7 with URL: https://mcp.context7.com/mcp to user config
File modified: <CLAUDE_CONFIG_DIR>/.claude.json
exit=0
```

## Q2 — Where does `--scope user` actually write, and is user scope really NOT the default?

**Confirmed: `--scope user` writes to `<CLAUDE_CONFIG_DIR>/.claude.json`'s top-level `mcpServers`
key** — a durable, cross-project entry, the same reach Engram's plugin registration has:

```json
{
  "mcpServers": {
    "context7": { "type": "http", "url": "https://mcp.context7.com/mcp" }
  }
}
```

**Confirmed: the default scope (no `--scope` flag) is NOT user scope — it is local/project
scope**, tied to the current working directory, written under a nested
`projects.<cwd>.mcpServers` key instead:

```
$ claude mcp add --transport http context7-defaulttest https://mcp.context7.com/mcp
Added HTTP MCP server context7-defaulttest with URL: https://mcp.context7.com/mcp to local config
File modified: <CLAUDE_CONFIG_DIR>/.claude.json [project: C:\Users\CLK090]
```

```json
{
  "projects": {
    "C:/Users/CLK090": {
      "mcpServers": {
        "context7-defaulttest": { "type": "http", "url": "https://mcp.context7.com/mcp" }
      }
    }
  }
}
```

This matters: without `--scope user`, click would register Context7 only for whatever directory
happened to be the current working directory at install time — not the global, cross-project
availability the task requires (matching Engram's own reach). `--scope user` is deliberate, not
incidental.

## Q3 — Does `claude mcp list` show Context7 as Connected?

**Yes.**

```
$ claude mcp list
Checking MCP server health…

context7: https://mcp.context7.com/mcp (HTTP) - ✔ Connected
```

## Q4 — Does `claude mcp get context7` exit 0 with details when present, and non-zero when absent?

**Yes, both confirmed.**

Present:
```
$ claude mcp get context7
context7:
  Scope: User config (available in all your projects)
  Status: ✔ Connected
  Type: http
  URL: https://mcp.context7.com/mcp

To remove this server, run: claude mcp remove context7 -s user
exit=0
```

Absent (after `claude mcp remove context7`):
```
$ claude mcp get context7
No MCP server named "context7". Run `claude mcp add` to add one.
exit=1
```

## Q5 — Does `claude mcp remove context7` cleanly remove it?

**Yes.**

```
$ claude mcp remove context7
Removed MCP server "context7" from user config
File modified: <CLAUDE_CONFIG_DIR>/.claude.json
exit=0
```

`mcpServers` reverts to `{}` in `.claude.json`; a subsequent `claude mcp get context7` exits 1.

## Design conclusion: presence probe avoids shelling out

Given Q2's finding — user-scope MCP servers land in a plain, directly-readable JSON file
(`<ClaudeHome>/.claude.json`'s `mcpServers` key) — `HasContext7` (`internal/installer/context7.go`)
reads that file directly instead of shelling out to `claude mcp get context7` for every presence
check. This mirrors `HasInstalledPluginID`'s own pure-file-read approach (used for Engram/click's
own plugins) and, critically, keeps `click doctor`'s new "context7 MCP" check free of any
subprocess execution — consistent with every other check in the `doctor` package, and safe to run
in `go test` without a fake `CommandRunner` injected at every call site.

The actual `add`/`remove` commands still go through the `CommandRunner` interface (same
factory-injected abstraction Engram's `claude plugin ...` calls use), so `click install`/`click
update`/`click uninstall` unit tests fake them exactly like they fake `claude plugin` calls — no
real `claude mcp` command is ever invoked inside `go test`.

## Ownership guard — mirrors the exact bug class fixed for Engram

`SyncContext7` decides `InstalledByClick` exactly once — the first time it ever runs against a
given `ClaudeHome` (when no `context7.json` state file exists yet) — and preserves it on every
later call. This is the same guard `SyncEngram` uses (see `documentacion/spikes/spike-e-engram-install.md`,
`TestSyncEngram_SecondRunPreservesClickOwnership`): naively deriving ownership from
`!alreadyPresent` on every call would flip a click-owned registration to "pre-existing" the moment
click's own prior install makes a later call see it as already-there, and `RemoveContext7` would
then wrongly refuse to remove something click actually added. Covered by
`TestSyncContext7_SecondRunPreservesClickOwnership` (`internal/installer/context7_test.go`).

## Click's own compiled binary — full end-to-end verification

Beyond raw `claude` CLI calls in isolation, the actual `click.exe` binary (built via
`go build ./cmd/click`) was run against a throwaway `CLICK_CLAUDE_HOME`/`CLAUDE_CONFIG_DIR`:

1. `click install --yes` — registered click-sdd/click-memory/click-review, Engram, **and**
   Context7 (`Registrando Context7 (documentación de librerías)… ✔ Context7 sincronizado`).
2. `claude mcp list` (same throwaway home) afterward showed:
   ```
   plugin:engram:engram: engram mcp --tools=agent - ✔ Connected
   context7: https://mcp.context7.com/mcp (HTTP) - ✔ Connected
   ```
3. `claude mcp get context7` exited 0 with `Scope: User config (available in all your projects)`.
4. `click doctor` against the same home reported every check healthy, including the new
   `context7 MCP: registrado (scope user, https://mcp.context7.com/mcp)` line.
5. `click uninstall` removed click-sdd/click-memory/click-review, Engram, **and** Context7
   (`Quitando Context7 (si click lo instaló)… ✔ Context7 procesado`); a follow-up
   `claude mcp get context7` exited 1 (`No MCP server named "context7"`), confirming the removal
   actually took effect, not just that click's own bookkeeping said so.

The throwaway home was deleted after verification; the real `~/.claude` was never written to
during this spike.

## Residual / follow-up

- Context7 has no binary to provision and no plugin cache to inspect — its entire footprint is one
  JSON entry in `<ClaudeHome>/.claude.json`, which is simpler than Engram's plugin+binary duality
  and needed no equivalent to `EnsureEngramBinary`.
- Unrelated to this spike, but noticed while verifying end-to-end on this dev machine: cloning the
  Engram marketplace repo over HTTPS on Windows can hit a `Filename too long` checkout failure
  (a long path under `openspec/changes/archive/...` in that repo) unless `core.longpaths=true` is
  set for the `git` subprocess `claude plugin marketplace add` spawns. This is an environment
  quirk of the Engram repo's own history depth on Windows, not something click's install flow
  causes or can fix from the CLI side — noted here only because it was hit while re-verifying the
  full install lifecycle for this spike, not a Context7-specific finding.
