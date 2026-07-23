# Codex CLI Target

Click can manage Codex CLI as an optional installation target alongside Claude Code and OpenClaw.
Claude Code remains mandatory for the native Click plugin workflow; Codex never blocks a Claude-only
installation.

## Detection and selection

`click targets` resolves the `codex` binary through the same injectable PATH lookup used by the other
targets. `click configure-targets` can explicitly enable Codex, and the selection is persisted with
the other Click target settings. An unconfigured legacy selection does not auto-enable Codex, because
Codex did not exist when those artifacts were created.

## Current boundary

This slice manages guidance only. When Codex is selected, Click resolves `CODEX_HOME` (or the user
home `.codex` directory) and writes a Click-owned block to `AGENTS.md`. The block points Codex at
the portable Click SDD workflow and explicitly avoids Claude-specific Agent, Skill, plugin, and
registry instructions.

Click does not modify Codex `config.toml`, the configured model (`model = "gpt-5.6"` or otherwise),
credentials, providers, or a native Codex skill/plugin installation location. Native Codex
skill/plugin packaging is deferred until this repository verifies the supported location and
interface.

`click update` refreshes the managed block, `click uninstall` removes only that block, and the
run-start snapshot includes the Codex `AGENTS.md` file when the target is selected.
