# Target-First Install Orchestration Specification

## Purpose

Define a truthful, recoverable lifecycle for the supported Claude Code, Codex, and OpenClaw targets.

## Requirements

### Requirement: Target-first neutral lifecycle

The system MUST select targets before target-specific configuration and persist that selection in Click-owned neutral state. Install, update, configure-targets, uninstall, and rollback MUST use that state; Claude home/preflight MUST be required only when Claude is selected. Claude marketplace, plugins, Context7, and memory guard MUST remain Claude-owned.

#### Scenario: Claude-free selection survives lifecycle
- GIVEN Codex is selected and Claude is not selected
- WHEN install, update, configure-targets, or uninstall runs
- THEN it MUST complete without reading or requiring a Claude home

#### Scenario: Claude selection enables Claude-owned work
- GIVEN Claude is selected with a valid Claude environment
- WHEN the plan is confirmed
- THEN only Claude-owned integrations MAY be included for Claude

### Requirement: Single source install plan and recovery

The system MUST derive preview, final confirmation, progress, mutation order, snapshots, rollback scope, and doctor expectations from one selected plan. It MUST perform no planned mutation before confirmation and MUST restore only snapshots captured by that plan after a failure.

#### Scenario: Confirmed plan executes truthfully
- GIVEN a multi-target plan is previewed and confirmed
- WHEN installation executes
- THEN reported steps, writes, snapshots, and doctor checks MUST match the plan order

#### Scenario: Planned write fails
- GIVEN a confirmed plan has captured its applicable snapshots
- WHEN a later planned mutation fails
- THEN rollback MUST restore captured content and report the failed step and recovery result
