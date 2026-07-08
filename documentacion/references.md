# click-ai-devkit — References

> Status: draft v0.1. Grounded in `00-decisions-and-open-questions.md` (D1–D20).

## Reference only

All sources below are used **for reference** — architecture patterns, mechanisms, and structure. Per the brief and the decisions doc: do **not** copy persona, names, or exact conventions from any of them. Everything Click-facing is rebranded (`click-*` naming, `ClickOrchestrator` persona per D10) and adapted to Click's own decisions (D6–D11).

## Reference table

| Source | URL | What we take from it | Click component it maps to |
|---|---|---|---|
| `Gentleman-Programming/gentle-ai` | https://github.com/Gentleman-Programming/gentle-ai | Pattern for an installable ecosystem: orchestrator, SDD flow, per-phase profiles, install/sync mechanics. **Also the source of the D3/D5 technical finding** (see below). | `ClickOrchestrator`, the overall `click-sdd-*` flow shape, and the decision to build a Go CLI (D3, D5) |
| `Gentleman-Programming/engram` | https://github.com/Gentleman-Programming/engram | Persistent memory / MCP server: per-project memory, past-decision search, session continuity | Bundled Engram dependency (D1, D8) — **not reimplemented**, only referenced/consumed |
| `Gentleman-Programming/agent-teams-lite` | https://github.com/Gentleman-Programming/agent-teams-lite | SDD with subagents; separation between orchestrator and specialist agents; markdown-structured feature development | The orchestrator/specialist split in `plugins/click-sdd/agents/` (`click-orchestrator.md` vs. `click-prd-writer.md`, `click-architect.md`, etc.) |
| `Gentleman-Programming/Gentleman-Skills` | https://github.com/Gentleman-Programming/Gentleman-Skills | Skill structure: `SKILL.md` file format, reusable per-workflow skills | The `SKILL.md` structure reused across `click-sdd/`, `click-memory/`, `click-review/` skills |
| `Gentleman-Programming/gentleman-guardian-angel` | https://github.com/Gentleman-Programming/gentleman-guardian-angel | AI-driven code review, standards validation, pre-PR checks | `click-pr-review` / `click-reviewer` in `plugins/click-review/` |
| Claude Code Plugin Marketplace docs | https://code.claude.com/docs/en/plugin-marketplaces | Packaging as an installable plugin/marketplace; `.claude-plugin/marketplace.json`; distributing internal plugins with skills, agents, hooks, MCP servers | Dropped for v0.1 (D16); CLI uses embedded `manifest.yaml`. Native Marketplace = possible v0.2 path. |

## The gentle-ai distribution finding (rationale for D3/D5)

Verified directly against the `gentle-ai` README: its **scoop** installation path installs a **compiled Go CLI binary**, not a set of Claude Code plugins distributed through a marketplace. This is the concrete technical finding that shaped Click's own distribution decision:

- **D3 (Distribution):** Click chose a CLI now (gentle-ai style), distributed via a Click scoop bucket, over a marketplace-only or hybrid approach.
- **D5 (CLI stack):** Click chose Go for the CLI — a single static binary, no runtime dependency — matching gentle-ai's approach, accepting the trade-off that the team needs Go skills to maintain it.

In other words: Click is not choosing a Go CLI *instead of* what gentle-ai does — it's choosing a Go CLI *because* that's what gentle-ai's own scoop distribution actually is. The marketplace docs (last row above) remain relevant only as background for a possible native Marketplace path in v0.2 — for v0.1 this is resolved by D16 (marketplace.json dropped; CLI uses embedded `manifest.yaml`), as noted below.

## Marketplace.json resolution (D16)

D16 resolves the ambiguity: `.claude-plugin/marketplace.json` is **dropped for v0.1**. The CLI uses its own embedded `manifest.yaml` (one per release) for install/sync configuration. A native Claude Code Marketplace path becomes a v0.2-optional feature only. The reference to the Claude Code Plugin Marketplace docs (row 6 above) remains relevant as background context for how internal plugins can be distributed via Marketplace in the future (v0.2+), but it is not the v0.1 path.
