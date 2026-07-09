# Spike D — Per-SDD-phase model selection: RESOLVED

> Status: RESOLVED. Combines Codex's empirical tests (throwaway CLAUDE_CONFIG_DIR) with
> authoritative Claude Code docs (sub-agents.md, plugins-reference.md, settings.md, tools-reference.md).

## Verdict: PROCEED — build the TUI using orchestration-layer routing (not frontmatter interpolation)

Codex correctly refused to ship the fragile path. The missing piece was a fourth option it didn't
evaluate — the orchestrator passing a per-invocation `model` when it delegates — which is fully
documented and is exactly how this environment already routes models.

| Option | Verdict |
|--------|---------|
| A — `${user_config.KEY}` inside an agent's `model:` frontmatter | **Rejected (fragile / unconfirmed).** Codex verified userConfig values persist, but the cached agent's `model: ${user_config.orchestrator_model}` was NOT materialized to `model: opus` on disk, and docs don't confirm interpolation applies to the `model` frontmatter field. A bad value can silently break agent loading. |
| B — per-agent model override in settings.json | **Does NOT exist.** Only `model` (main session) and `CLAUDE_CODE_SUBAGENT_MODEL` (uniform across ALL subagents). No `agents.<name>.model`. |
| C-old (Codex) — rewrite cached agent markdown | **Rejected.** `claude plugin update`/reinstall overwrites the versioned cache. Not durable. |
| **C-new — orchestrator passes per-invocation `model` on the `Agent` tool** | **CONFIRMED — the answer.** sub-agents.md resolution order: `CLAUDE_CODE_SUBAGENT_MODEL` → **per-invocation `model` param** → agent frontmatter → main conversation. The per-invocation param beats the frontmatter. Same pattern this env's own CLAUDE.md mandates ("every Agent call MUST include model"). |

`model:` accepted values (sub-agents.md): `sonnet`, `opus`, `haiku`, `fable`, full model id, or `inherit`.

## Confirmed empirically by Codex
- Throwaway `CLAUDE_CONFIG_DIR` isolates cleanly (no real ~/.claude mutation).
- `claude plugin install --config <k>=<v>` persists values to `settings.json` →
  `pluginConfigs["click-sdd@click-ai-devkit"].options` (plain, readable JSON). ✅ This is our STORAGE.
- The current click-sdd plugin declares no `userConfig` yet, so `--config` is rejected until we add the schema.

## DECISION (D25): model routing = userConfig storage + orchestrator delegation

1. **Store** the choice at install: declare a `userConfig` field per phase in `plugins/click-sdd/.claude-plugin/plugin.json`
   (type `string`). The TUI runs `claude plugin install click-sdd@click-ai-devkit --config <phase>_model=<alias> ...`.
   Value lands in `settings.json → pluginConfigs["click-sdd@click-ai-devkit"].options` (Codex-verified).
2. **Apply** at runtime: the agent frontmatter `model:` stays plain (`inherit` or a sane default — do
   NOT use `${user_config...}` there). The `click-orchestrator` agent's instructions read the
   `pluginConfigs[...].options` map once per session, build `phase → model`, and pass the resolved
   alias as the `model` parameter on every `Agent` delegation to a phase subagent.

This uses only documented+verified mechanisms (userConfig persistence + per-invocation model param)
and avoids the fragile frontmatter interpolation entirely.

## Build notes for the TUI slice
- Add `userConfig` string fields to click-sdd's plugin.json (one per phase/agent). Defaults:
  orchestrator/prd-writer/architect/reviewer = `opus`, memory-curator = `sonnet` (override in TUI).
- TUI collects choices → `claude plugin install ... --config <phase>_model=<alias>`; non-interactive
  fallback uses defaults.
- `click-orchestrator.md` gains a "Model routing" section: read `pluginConfigs["click-sdd@click-ai-devkit"].options`
  at session start, map phase→model, pass `model` on every Agent delegation.
- Optionally mirror the choices into a click-managed `models.json` for `doctor` to display.

## Residual to confirm at build (throwaway dir only)
- Exact userConfig schema fields + the resulting `pluginConfigs` option keys after a real install.
- That the orchestrator reliably reads that settings.json location at session start.
