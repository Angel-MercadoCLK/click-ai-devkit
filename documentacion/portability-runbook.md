# Runtime portability and qualification runbook

This is the current target-boundary guide for Click on Claude Code, Codex, and OpenClaw. It does
not claim that Click owns workflow orchestration, permission lists, model selectors, or shared MCP
registration. See [the documentation index](README.md), [Codex target](codex-target.md), and the
locked decisions D48–D53.

## Current ownership

| Area | Click owns | Gentle AI owns |
|---|---|---|
| Claude Code | `click-memory`, `click-review`, `click-skills`; Click-managed guidance and memory-guard; optional Engram Cloud enrollment | ODD/RDD workflow, runtime components, permissions, model selection, Engram binary/lifecycle and shared Engram/Context7 configuration |
| Codex | Click-managed `AGENTS.md` guidance and Click-specific business/memory policy | Runtime/MCP components and supported model-selection interface |
| OpenClaw | Click-managed workspace guidance and `clickhola` skill | Runtime/MCP components and supported configuration interfaces |

Click no longer ships or installs `click-sdd`; do not follow historical instructions that name its
agents, phase skills, or orchestrator. Click's model-selection commands are compatibility notices
that fail without prompting or changing configuration. Existing model profiles and native model
settings are preserved. No shared Engram/Context7 registration is removed during Click uninstall.

## Target-root constraint

The pinned gentle-ai v4.0.0 adapters use standard process-home roots and provide no per-target root
flags. For each selected runtime, install/update must use the standard `.claude`, `.codex`, or
`.openclaw` directory under that process home. Click rejects incompatible configured target roots
before snapshot, provisioning, external runner, or config writes (D51). Tests must isolate
`HOME`, `USERPROFILE`, and `XDG_CONFIG_HOME` in a temporary root and must not set target overrides
for upstream integration fixtures. Never move, inspect, or modify a real profile for a portability
check.

## Install and removal boundaries

The release manifest pins gentle-ai v4.0.0. Install/update scope it to selected runtimes with the
`full-gentleman` preset, components `engram,skills,context7,permissions,persona`, and persona
`neutral` (D49). The managed binary uses checksum-qualified official archives on Linux/macOS. The
v4.0.0 Windows path uses the exact module/tag with `go install`, requiring Go >=1.25.10. The
disposable hosted Windows package smoke has passed. External CLIs were offline fixtures; no live
upstream provisioning was performed, so this does not qualify live Gentle AI integration.

Click uninstall removes only its owned content and user-scope Click plugins discovered in Claude
Code's native plugin inventory. It retains plugin data, other scopes, marketplace declarations, shared
Engram/Context7 state, native config, foreign hooks, and user text. Rollback similarly restores only
verifiably Click-owned portions; it preserves the current Engram Cloud token and skips shared or
unqualified paths, including with force/yes. Historical snapshots remain readable.

## Qualification checklist

Run Go tests against mocks and temporary homes; no live install is required for this checklist.

1. Select Claude only, Codex only, OpenClaw only, and combinations; verify the upstream runner gets
   only the selected agent names and pinned component/preset arguments.
2. Verify install/update invoke Gentle AI before marketplace refresh, and a failed preflight or
   provisioning action makes no target-state writes. A late failure is captured in the run snapshot
   and remains eligible for owned-content rollback.
3. Verify Click's three Claude plugins and OpenClaw/Click-specific content are retained while
   upstream-owned Engram/Context7 registration and native settings survive update/uninstall.
4. Verify user/project/local/managed/synced plugins outside the exact Click user-scope inventory
   are not removed; removal uses `--scope user --keep-data` and never marketplace removal.
5. Verify rollback preserves current Cloud token, foreign settings/hooks/Markdown, shared MCP
   configs, and unknown paths; restore only exact owned blocks or qualified historical artifacts.
6. Verify standard-root fixtures are isolated and selected nonstandard roots fail before any
   provisioning, runner, snapshot, or configuration write.

The Go integration suite uses mocks and proves only the tested Click contract; it does not prove
external Gentle AI/Claude/OpenClaw side effects. The packaged smoke requires the explicit
`-DisposableRunner` acknowledgement, Windows, and GitHub-hosted runner environment markers before
it accesses the distribution or changes process state. This is an operational guard, not cryptographic runner detection.
The actual safety boundary is the workflow's disposable
`windows-latest` job; do not run it against a normal developer profile because provisioning may
persist the managed binary directory in that Windows account's user PATH.

`pwsh` and `powershell` are unavailable in the current Linux environment. Windows package smoke
qualification therefore runs only in the disposable hosted Windows workflow described above.
