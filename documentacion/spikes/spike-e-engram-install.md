# Spike E — Making `click install` Actually Install Engram (v0.2 Slice 3, Step 0)

**Date:** 2026-07-09
**Status:** Resolved. Plugin-registration path implemented. Binary-provisioning path intentionally
NOT automated (see "Binary provisioning" below) — reporting only, per the hard rule against
shipping a fragile network-fetch inside `click install`.

All verification below was run against **throwaway `CLAUDE_CONFIG_DIR`/`CLICK_CLAUDE_HOME`
directories** under the OS temp dir, never the real `~/.claude`. The real `~/.claude` was only
ever read (never written) to compare against — confirmed unchanged (`mcp/engram.json` mtime and
`settings.json` `engram@engram` entry identical before/after this spike).

## Q1 — Does `claude plugin marketplace add` + `claude plugin install engram@engram` succeed, no auth?

**Yes.** Against a fresh throwaway `CLAUDE_CONFIG_DIR`:

```
$ claude plugin marketplace add https://github.com/Gentleman-Programming/engram
Adding marketplace…Refreshing marketplace cache (timeout: 120s)…
Cloning repository (timeout: 120s): https://github.com/Gentleman-Programming/engram.git
Clone complete, validating marketplace…
✔ Successfully added marketplace: engram (declared in user settings)

$ claude plugin install engram@engram
Installing plugin "engram@engram"...✔ Successfully installed plugin: engram@engram (scope: user)
```

Both commands succeeded cleanly, no credentials/auth prompt (public repo). The marketplace name
Claude Code assigns with no explicit name argument is the **repo basename**: "engram" from
`.../Gentleman-Programming/engram`. This is depended on by `SyncEngramPlugin`'s
`engramMarketplaceName = "engram"` constant and by the fake runner's `marketplaceKeyFromSource`.

## Q2 — Does the engram plugin's bundled MCP config point at a bare `engram` or an absolute path?

**Bare `command: "engram"` — PATH-resolved.** Read directly from the installed plugin cache:

`<CLAUDE_CONFIG_DIR>/plugins/cache/engram/engram/0.1.1/.mcp.json`:
```json
{
  "mcpServers": {
    "engram": { "command": "engram", "args": ["mcp", "--tools=agent"] }
  }
}
```

So installing the plugin alone does **not** by itself make Engram runnate — it depends entirely on
whatever `engram` resolves to on PATH at the moment Claude Code launches the MCP server.

## Q3 — Does installing the plugin alone make Engram runnable, or is a separate binary required?

**A separate `engram` binary on PATH is required.** Proven both ways with `claude mcp list`
against the same throwaway home, only toggling `PATH`:

- **With `engram` on PATH** (this dev machine has two copies — see Q3b):
  ```
  plugin:engram:engram: engram mcp --tools=agent - ✔ Connected
  ```
- **With `engram` stripped from PATH** (same throwaway home, same plugin install, only `PATH`
  changed for the subprocess):
  ```
  plugin:engram:engram: engram mcp --tools=agent - ✘ Failed to connect
  ```

This is decisive: **plugin registration is necessary but not sufficient.** `click install` must
also care about binary presence, and `click doctor` must be able to detect its absence — a
developer with the plugin "installed" per Claude Code's own registry can still have a completely
non-functional Engram MCP server if the binary isn't resolvable.

### Q3b — Binary provisioning: how is it obtained, and what did we implement?

Per Spike A, three documented paths exist: `go install
github.com/Gentleman-Programming/engram/cmd/engram@<version>`, a Homebrew tap
(`gentleman-programming/tap/engram`, macOS/Linux only), or a GitHub Releases zip
(`engram_<version>_<os>_<arch>.zip`). On this dev machine the binary already exists at
`C:\Users\CLK090\AppData\Local\engram\bin\engram.exe` (version 1.16.1, not the manifest-pinned
v1.15.3 — a pre-existing drift, unrelated to this slice) **and** separately at
`C:\Users\CLK090\go\bin\engram.exe` (from a prior `go install`) — both on PATH.

**Decision: do NOT automate binary download in this slice.** Per the task's hard rule ("if the
binary-provisioning path is genuinely fragile or uncertain, implement the plugin-registration part
… and STOP on the binary part"), the binary-fetch path is exactly the fragile kind that rule warns
about: it needs OS/arch detection, checksum/signature verification, atomic install-and-PATH-wiring
on both Windows and POSIX, and safe behavior when a version conflict exists (as it already does on
this very machine — two different resolved versions). None of that is testable meaningfully with
the fake `CommandRunner` (it's real filesystem/network/OS work), and getting it wrong ships a
supply-chain risk into every developer's machine.

**What was implemented instead:** `ResolveEngramBinaryPath` / `EngramBinaryResolvable` (carried
over and adapted from the v0.1 `mcpconfig.go` code, which already had this exact
override-then-PATH-then-default-location resolution order) — used by `click doctor` to report
whether the binary that the plugin's bare `command: "engram"` needs will actually resolve, without
click ever downloading or installing that binary itself. **Recommendation for a follow-up slice:**
if binary auto-provisioning is wanted, scope it as its own reviewed slice — `go install
.../engram/cmd/engram@<pinned-version>` (shelling out to the Go toolchain, which is already a
click build dependency) is the least-fragile of the three options, since it reuses Go's own
module-proxy verification instead of hand-rolling zip/checksum handling, but it requires Go on the
*developer's* machine too, which not every click-ai-devkit user will have — worth a real
product decision, not a default baked into `click install`.

## Q4 — Which MCP config does Claude Code actually use: the plugin's, or `~/.claude/mcp/engram.json`?

**Only the plugin's.** This is the most important finding of this spike, and it directly
contradicts v0.1's design (`mcpconfig.go`'s `ConfigureEngramMCP`, which wrote a durable
`<ClaudeHome>/mcp/engram.json`).

Evidence, gathered **read-only** against the real, already-configured `~/.claude` on this machine
(which happens to have both the plugin installed AND a legacy hand-written
`~/.claude/mcp/engram.json` — i.e. exactly the "already has Engram working" scenario this task
warned about):

1. `~/.claude/mcp/engram.json` **exists** on this machine (absolute path to the pinned binary,
   `args: ["mcp", "--tools=agent"]`) — so if Claude Code read it, we'd expect a distinct MCP server
   entry from it.
2. `claude mcp list` in the real environment shows exactly one Engram entry:
   `plugin:engram:engram: engram mcp --tools=agent - ✔ Connected`. No separate bare "engram" entry
   sourced from the hand-written file.
3. `~/.claude.json`'s top-level `mcpServers` key (Claude Code's real user-scope MCP config
   location, confirmed via Claude Code's own MCP docs) contains only `figma-desktop` — no
   `engram` entry at all.
4. This session's own available MCP tools are `mcp__plugin_engram_engram__*` — the plugin-scoped
   naming, not a bare `mcp__engram__*` that a user-scope config would produce.

**Conclusion: `~/.claude/mcp/engram.json` is dead — Claude Code never reads it.** It was almost
certainly written by some other/older tooling (possibly an older version of `engram setup
claude-code`, per Spike A) targeting a path that isn't a real Claude Code config location. v0.1's
`mcpconfig.go` copied that same (incorrect) assumption. This spike's implementation **removes
`mcpconfig.go` entirely** rather than reconciling it with the plugin path — there is nothing to
reconcile, since the file it wrote was never load-bearing. `click` no longer writes an MCP config
for Engram at all; the plugin's own bundled `.mcp.json` is the only mechanism, and click's job is
limited to (a) getting the plugin registered and (b) reporting whether the binary it needs is
resolvable.

## Q5 — Does the memory-guard hook matcher still fire?

**Yes, confirmed exactly as documented in `hooksettings.go`.** `MemoryGuardToolMatcher =
"mcp__plugin_engram_engram__mem_save"` matches Claude Code's plugin-provided MCP tool naming
convention `mcp__plugin_<plugin>_<server>__<tool>` with plugin name "engram" and MCP server key
"engram" (both confirmed from the plugin's own manifest/`.mcp.json`) — and this session's actually
available tools are literally named `mcp__plugin_engram_engram__mem_save` etc. No code change
needed; this is a confirmation, not a fix.

## Design conclusions carried into the implementation

1. `click install` registers the Engram marketplace + installs `engram@engram` through the same
   `CommandRunner` abstraction used for click's own plugins (`internal/installer/engram.go`,
   `SyncEngram`/`SyncEngramPlugin`) — **idempotent and respectful**: if `engram@engram` is already
   registered and enabled (checked via `HasInstalledPluginID` against Claude Code's own registry
   files, the same mechanism `HasInstalledPlugin` already used for click's own plugins), it is left
   completely untouched — no `marketplace add`/`install` commands are issued at all.
2. `mcpconfig.go` (the v0.1 dead-path file) is deleted outright, along with `Config.EngramMCPConfigPath()`.
   `Config.EngramStatePath()` is kept — it's click's own bookkeeping (pinned version, resolved
   binary path, and **install ownership**), not an MCP config.
3. `click doctor` reports two Engram-specific checks: plugin registered+enabled
   (`checkEngramPlugin`), and binary resolvable on disk (`checkEngramBinary`, via
   `EngramBinaryResolvable`) — surfacing exactly the Q3 gap (plugin present, binary missing) that a
   plugin-only check would silently hide.
4. `click uninstall` only removes Engram when click's own state says click installed it
   (`engramState.InstalledByClick`). A real regression was caught during this spike's own
   end-to-end verification (not just unit tests): naively deriving ownership from
   "!alreadyInstalled" on every `SyncEngram` call flips a click-owned install to "pre-existing" the
   moment click's own prior install makes a later call see it as already-there. Ownership is now
   decided once (the first time `SyncEngram` ever runs against a given `ClaudeHome`, when no state
   file exists yet) and preserved on every later call — verified with a dedicated regression test
   (`TestSyncEngram_SecondRunPreservesClickOwnership`) and re-confirmed against the real `claude`
   CLI end-to-end (install twice, then uninstall — Engram is now correctly removed).

## Residual / follow-up

- Binary auto-provisioning is out of scope for this slice (see Q3b) — `click doctor` reports the
  gap, it does not close it.
- The manifest-pinned Engram version (`v1.15.3`, from `ENGRAM_VERSION`) does not match what's
  actually on this dev machine's PATH (`1.16.1`) — pre-existing drift from before this slice,
  unrelated to the install-mechanism work here, but worth flagging: `click doctor`/`click update`
  do not currently compare `engram --version` output against the manifest pin. A future slice could
  add that check now that `EngramBinaryResolvable` already knows the resolved path.
