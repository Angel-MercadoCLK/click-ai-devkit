# Spike A — How Engram Is Actually Packaged and Installed

**Date:** 2026-07-07
**Status:** Resolved, one residual unknown (marked below)

## Verdict

Engram is **both (c)**: a single agent-agnostic Go binary that (1) can be installed directly (Homebrew, `go install`, or a prebuilt release binary) and run as a stdio MCP server via `engram mcp`, **and** (2) is separately packaged as a **Claude Code plugin** (`Gentleman-Programming/engram`, plugin name `engram`, currently `version: 0.1.1`) whose `.mcp.json` launches that same binary (`engram mcp --tools=agent`) so it appears as a bundled MCP server. The starting signal — `mcp__plugin_engram_engram__*` tool names and plugin id `plugin_engram_engram` — is **confirmed**: Claude Code's plugin-tool-naming convention is `mcp__plugin_<plugin-name>_<server-name>__<tool-name>`, and both the plugin name and the MCP server key inside it are `engram`, which produces exactly `plugin_engram_engram`.

---

## Q1 — How is Engram distributed today?

**Both a standalone MCP server binary and a Claude Code plugin.**

- The binary (`engram`) is a single Go executable with multiple modes: CLI, MCP stdio server (`engram mcp`), HTTP API server, and TUI. Source: [Gentleman-Programming/engram README](https://github.com/Gentleman-Programming/engram) — "Persistent memory system for AI coding agents. Agent-agnostic Go binary with SQLite + FTS5, MCP server, HTTP API, CLI, and TUI."
- The Claude Code plugin (`plugin/claude-code/` in the same repo) is a thin wrapper: a `.claude-plugin/plugin.json` manifest + a `.mcp.json` that shells out to the same `engram` binary. It does not embed a separate binary — it depends on `engram` being resolvable on PATH (or via the durable user-level MCP config engram's own setup writes).
- Confirmed via raw file fetches:
  - `plugin/claude-code/.claude-plugin/plugin.json` → `{"name":"engram", "version":"0.1.1", ...}`
  - `plugin/claude-code/.mcp.json` → `{"mcpServers":{"engram":{"command":"engram","args":["mcp","--tools=agent"]}}}`
  - Root `.claude-plugin/marketplace.json` → lists plugin `engram`, `source: "./plugin/claude-code"`, `version: "0.1.1"`.

## Q2 — Exact install mechanism & command(s)

Three documented paths, all leading to the same binary + MCP wiring:

1. **Claude Code plugin marketplace (primary/documented path for Claude Code users):**
   ```
   claude plugin marketplace add Gentleman-Programming/engram
   claude plugin install engram
   ```
   Source: repo README, [docs/AGENT-SETUP.md](https://github.com/Gentleman-Programming/engram/blob/main/docs/AGENT-SETUP.md).

2. **`engram setup claude-code`** (binary-first path — install the binary yourself, then let it wire the agent):
   - Writes durable user-level MCP config to `~/.claude/mcp/engram.json` using the resolved absolute path to the `engram` binary.
   - Also writes/updates a plugin-scoped server id.
   - Prompts whether to add `engram` tool names to `~/.claude/settings.json` `permissions.allow` (avoids per-call confirmation prompts).
   - Source: [docs/AGENT-SETUP.md](https://github.com/Gentleman-Programming/engram/blob/main/docs/AGENT-SETUP.md).

3. **Bare/manual MCP entry** — for any MCP-capable client, hand-write:
   ```json
   { "mcpServers": { "engram": { "command": "engram", "args": ["mcp"] } } }
   ```

Binary install itself (prerequisite for paths 2 and 3, and implicitly needed by path 1 too, since the plugin's `.mcp.json` still shells out to `engram` on PATH):

- macOS/Linux: `brew install gentleman-programming/tap/engram`
- Windows (recommended by the project's own docs): `go install github.com/Gentleman-Programming/engram/cmd/engram@latest`
- Windows/any OS: build from source (`git clone` + `go install ./cmd/engram`)
- Windows/any OS: download a prebuilt zip from GitHub Releases (`engram_<version>_windows_amd64.zip` or arm64) and place on PATH
- Source: [docs/INSTALLATION.md](https://github.com/Gentleman-Programming/engram/blob/main/docs/INSTALLATION.md) (fetched raw).

There are also non-Claude-Code setup one-liners (`engram setup pi`, `engram setup opencode`, `engram setup gemini-cli`, `engram setup cursor`, `engram setup windsurf`, `engram setup vscode`, `engram setup codex`) — not relevant to `click`, but confirms the binary is agent-agnostic and the "plugin" concept is Claude-Code-specific packaging on top of it.

## Q3 — MCP wiring: what config, and where

- **Transport:** stdio only. The docs state explicitly: engram has no HTTP/network MCP endpoint; `engram mcp` speaks MCP over stdin/stdout.
- **Plugin-provided entry** (what you get from `claude plugin install engram`): defined in `plugin/claude-code/.mcp.json`:
  ```json
  { "mcpServers": { "engram": { "command": "engram", "args": ["mcp", "--tools=agent"] } } }
  ```
  Per Claude Code's own MCP docs ([code.claude.com/docs/en/mcp](https://code.claude.com/docs/en/mcp), section "Plugin-provided MCP servers"), plugin-bundled servers start automatically when the plugin is enabled, are managed through plugin install/enable — not `/mcp` or `claude mcp add` — and are named `plugin:<plugin-name>:<server-name>` internally, with tools exposed as `mcp__plugin_<plugin-name>_<server-name>__<tool-name>`. For `engram`/`engram` this is exactly `mcp__plugin_engram_engram__*`, matching this session's observed tool names.
- **Manual/durable entry** (what `engram setup claude-code` or a hand-written config produces): `~/.claude/mcp/engram.json` (user-level, durable) using the **absolute resolved path** to the `engram` binary — not the bare `"engram"` command Claude Code's own plugin `.mcp.json` uses. This matters because a plugin-relative `command` resolves via PATH, while the durable user config pins an absolute path.
- **Claude Code's general scope model** (from its own docs, not engram-specific): non-plugin MCP servers can live in `.mcp.json` (project scope, shared/committed) or `~/.claude.json` (local/user scope, personal). Precedence order when the same server name exists in multiple places: local > project > user > plugin-provided > claude.ai connectors. Plugin and connector servers are de-duplicated by matching `command`/`url`, not by name.

## Q4 — Versioning / pinning

Mixed picture — pinning is possible at two independent layers, and they are **not currently wired together** in what the repo documents:

1. **Binary version pinning (yes, well-supported):**
   - `go install github.com/Gentleman-Programming/engram/cmd/engram@<version>` — standard Go module semantics: replace `@latest` with a specific tag (e.g. `@v1.4.0`) or commit to pin exactly. The docs only show `@latest`, but this is baseline Go tooling behavior, not an engram-specific limitation.
   - GitHub Releases: 96 releases visible on the repo, each producing versioned platform zips (`engram_<version>_windows_amd64.zip`), so a specific release can be downloaded directly.
   - No pinning support found for the Homebrew tap formula beyond whatever `brew` itself offers (typically latest, with `brew pin` for the currently installed version — not a specific historical version without extra tap work).

2. **Claude Code plugin version pinning (documented mechanism exists, but not used by this marketplace today):**
   - Claude Code's plugin-marketplace format supports pinning a plugin's source to a **git ref (branch/tag) and/or an exact commit `sha`** in `marketplace.json`. When both are set, `sha` wins and guarantees a reproducible install. Source: [code.claude.com/docs/en/plugin-marketplaces](https://code.claude.com/docs/en/plugin-marketplaces) (confirmed via search summary of that doc; not independently refetched line-by-line in this spike — see residual unknown below).
   - **However**, Engram's own `.claude-plugin/marketplace.json` (as fetched) does **not** set a `ref` or `sha` on the plugin's `source` — it's just `"source": "./plugin/claude-code"` (a relative path within the same repo, since the marketplace and the plugin live together). Practically, `claude plugin install engram` installs whatever is at the tip of the marketplace repo's default branch at install time, unless the *marketplace add* step itself was pinned (`claude plugin marketplace add` accepts a ref per Claude Code's docs) or Claude Code separately records/updates the installed commit.
   - The plugin manifest does carry a semantic `"version": "0.1.1"` field, but that's metadata, not an install-time pin mechanism by itself.

## Q5 — Implication for `click install` / `click doctor` / `click update` (incl. D8 pinning)

**Practical conclusion for the Go CLI design:**

- **Do not assume the plugin path alone gives you a reproducible pin.** If `click install` wants a *deterministic* Engram version per `click` release (D8), the reliable lever is the **binary**, not the marketplace plugin: either (a) `click` vendors/downloads a specific `engram_<version>_<os>_<arch>` release asset directly from GitHub Releases and puts it on PATH itself, or (b) `click` shells out to `go install github.com/Gentleman-Programming/engram/cmd/engram@<pinned-version>` at a known pinned tag. Both give an exact, reproducible version independent of what the marketplace's default branch currently points to.
- **`click install` should still also register the Claude Code plugin** (`claude plugin marketplace add ...` + `claude plugin install engram`) *or* write the MCP config directly — because that's what actually wires Engram's tools into a Claude Code session (`mcp__plugin_engram_engram__*`). If `click` only installs the binary without either the plugin or an equivalent MCP entry, Engram tools won't appear in Claude Code at all.
- Given the plugin's `.mcp.json` just calls `command: "engram"` (bare, PATH-resolved) rather than an absolute path, **`click` should prefer writing its own durable MCP entry pointing at the exact binary path it installed/pinned** (mirroring what `engram setup claude-code` already does at `~/.claude/mcp/engram.json`), rather than relying solely on the plugin's relative `command`. This avoids PATH ambiguity when multiple Engram binaries/versions might exist on a dev machine.
- **`click doctor` should check, at minimum:**
  1. Is an `engram` binary resolvable, and does `engram --version` (or equivalent) match the version `click` expects/pinned for this release?
  2. Is the Engram MCP entry present and pointing at that same binary — either via the plugin (`claude plugin list` / presence of `plugin_engram_engram` in `/mcp` or equivalent) or via the durable `~/.claude/mcp/engram.json` / project `.mcp.json` entry `click` itself wrote?
  3. Is the entry using the correct args (`mcp` or `mcp --tools=agent`) — mismatched args could silently expose a different tool surface than expected.
  4. No PATH collision — flag if more than one `engram` binary is discoverable with mismatched versions.
- **`click update`** should re-pin/re-download the exact Engram version tied to the new `click` release (same mechanism as install), then re-verify the MCP entry still points at the refreshed binary path — a version bump alone doesn't help if the MCP config still references a stale path or if `command: "engram"` silently resolves to an old PATH entry.

---

## Residual Uncertainty

1. **Marketplace `ref`/`sha` pinning syntax was not independently re-verified against the live `code.claude.com/docs/en/plugin-marketplaces` page in this spike** — it comes from a web-search synthesis of that page (via WebSearch results), not a direct fetch-and-quote like the other citations here. Before `click` relies on `ref`/`sha` pinning at the *marketplace* level (as opposed to pinning the binary directly, which is the recommended approach above), fetch and quote that page directly.
2. **Whether `claude plugin install engram` records/pins the installed commit anywhere locally** (so re-running `install` later doesn't silently drift to a newer commit) is not documented in what was fetched. This affects how reproducible the plugin-based path is over time, independent of the binary-pinning question.
3. **Homebrew tap versioning** (whether the tap keeps historical formula versions, i.e., whether `brew install gentleman-programming/tap/engram@<version>` is possible) was not confirmed — the fetched docs only show the unpinned `brew install .../engram` command.

These don't change the Q5 conclusion (pin the binary directly; don't rely on marketplace-tip installs for reproducibility) but should be closed out before finalizing the D8 pinning design in detail.

---

## Sources

- [Gentleman-Programming/engram (README)](https://github.com/Gentleman-Programming/engram)
- [docs/AGENT-SETUP.md](https://github.com/Gentleman-Programming/engram/blob/main/docs/AGENT-SETUP.md)
- [docs/PLUGINS.md](https://github.com/Gentleman-Programming/engram/blob/main/docs/PLUGINS.md)
- [docs/INSTALLATION.md](https://github.com/Gentleman-Programming/engram/blob/main/docs/INSTALLATION.md) (raw)
- [plugin/claude-code/.claude-plugin/plugin.json](https://raw.githubusercontent.com/Gentleman-Programming/engram/main/plugin/claude-code/.claude-plugin/plugin.json) (raw)
- [plugin/claude-code/.mcp.json](https://raw.githubusercontent.com/Gentleman-Programming/engram/main/plugin/claude-code/.mcp.json) (raw)
- [.claude-plugin/marketplace.json](https://raw.githubusercontent.com/Gentleman-Programming/engram/main/.claude-plugin/marketplace.json) (raw)
- [Claude Code — Connect Claude Code to tools via MCP](https://code.claude.com/docs/en/mcp) (full page fetched; see "Plugin-provided MCP servers" and "MCP installation scopes" sections)
- [Claude Code — Create and distribute a plugin marketplace](https://code.claude.com/docs/en/plugin-marketplaces) (referenced via search synthesis only — see Residual Uncertainty #1)
