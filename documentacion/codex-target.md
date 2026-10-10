# Codex CLI target

Click supports Codex as an optional target. This document describes the current ownership boundary,
not a Click-owned workflow engine or model selector. See [the documentation index](README.md) and
the locked decisions D48–D53 in `00-decisions-and-open-questions.md`.

## Ownership

- Click writes and refreshes its managed block in Codex `AGENTS.md`; uninstall removes only that
  block and preserves surrounding user/upstream text.
- The pinned gentle-ai v4.0.0 install configures its selected runtime components, including Engram
  and Context7 MCP integration. Click does not write duplicate shared MCP entries.
- Click does not write Codex model/profile values, credentials, providers, or other native model
  settings. Existing native values and historical model artifacts are preserved. Click's retired
  model flags are rejected before provisioning or writes; use the supported Gentle AI interface for
  model roles it exposes.
- Click's memory guard and Click-specific business/memory policy remain separate from Gentle AI's
  permission configuration. Click does not create allow/deny/ask lists.

## Target roots and lifecycle

The pinned gentle-ai v4.0.0 adapters resolve targets from the process home and do not accept custom
per-target roots. Install/update therefore require the standard home paths (`~/.claude`, `~/.codex`,
and `~/.openclaw`) for each selected target. A configured nonstandard root is rejected before
snapshot, provisioning, runner invocation, or config writes (D51); Click does not redirect it or
inspect an alternate tree while Gentle AI mutates the standard one.

When Codex is selected, Click snapshots the managed `AGENTS.md` and native Codex config for rollback
safety. Rollback merges only provable Click-owned content; it preserves current native model values,
unrelated TOML settings, shared MCP configuration, and unknown historical paths, including when
`--force` or `--yes` is supplied.

## Qualification

Use isolated temporary process homes for integration tests; never use a developer's real profile.
The Go suite validates managed-block refresh/removal, selected-target routing, and rollback behavior.
Those mocks do not prove live Codex or Gentle AI effects. The Windows package-smoke lifecycle and
actual Windows execution remain pending; see `portability-runbook.md`.
