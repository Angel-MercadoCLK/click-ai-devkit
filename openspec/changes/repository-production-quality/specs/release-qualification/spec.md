# Release Qualification Specification

## Purpose

Require truthful Windows behavior, diagnostics, UX, and release evidence before publication.

## Requirements

### Requirement: Git-safe refresh and diagnostic recovery

The system MUST run marketplace Git operations from a valid repository context when launched outside a repository. Temporary Git recovery MUST be cleaned on failure. Doctor MUST distinguish stale cache or Engram diagnostics from missing source assets and guide qualified update/restart recovery without automatic repair.

#### Scenario: Windows non-repository refresh failure
- GIVEN Windows starts Click from a non-repository directory and Git recovery fails
- WHEN marketplace refresh runs
- THEN no orphan recovery repository MUST remain and the error MUST state recovery action

#### Scenario: Stale cache or Engram recovery
- GIVEN doctor reports a stale cache or Engram diagnostic
- WHEN the supported update completes and Claude restarts
- THEN doctor MUST report the refreshed state and nested-agent propagation evidence

### Requirement: Target-aware menu and release proof

The menu MUST present coherent groups, use the exact label `Plugins`, and show target availability with only supported target actions. Current documentation, CLI/package/release metadata, and update claims MUST agree; historical documentation MUST be dated or labelled historical. Publication MUST require Windows packaged-binary smoke evidence for supported target contracts, marketplace refresh, update behavior, and version/release metadata.

#### Scenario: Menu availability
- GIVEN a target lacks a qualified native action
- WHEN the grouped menu renders
- THEN its action MUST be unavailable with guidance while `Plugins` remains exactly labelled

#### Scenario: Release matrix gate
- GIVEN a candidate release is built
- WHEN Windows smoke or docs/version/update qualification fails
- THEN publication MUST be blocked with the failing matrix entry reported
