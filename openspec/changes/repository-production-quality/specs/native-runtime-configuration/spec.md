# Native Runtime Configuration Specification

## Purpose

Safely configure only qualified native contracts for Codex and OpenClaw.

## Requirements

### Requirement: Explicit and safe Codex model mutation

The system MUST mutate Codex native configuration only for an explicitly selected model. Noninteractive execution, including `--yes`, MUST reject native mutation without the explicit Codex model flag. A successful mutation MUST be TOML table-aware, atomic, and recoverable from the plan snapshot; it MUST NOT change a same-named key in another table.

#### Scenario: Noninteractive model omission
- GIVEN Codex is selected with `--yes` and no Codex model flag
- WHEN native configuration is reached
- THEN no native file MUST change and actionable flag guidance MUST be returned

#### Scenario: Scoped Codex update
- GIVEN `config.toml` has root and table-scoped `model` keys
- WHEN an explicit Codex model is confirmed
- THEN only the supported native key MUST change and rollback MUST restore the original file

### Requirement: Qualified target-native ownership

The system MUST perform OpenClaw native actions only after its installed CLI command, keys, and result contract are qualified. Until qualification, it MUST make no OpenClaw mutation or release claim. Codex MUST receive only its supported native configuration and managed guidance; OpenClaw MCP, skills, or guard work MUST NOT be represented as Claude work.

#### Scenario: Unqualified OpenClaw contract
- GIVEN OpenClaw CLI qualification is absent or fails
- WHEN OpenClaw is selected
- THEN the plan MUST block its native action with recovery guidance and preserve all configuration
