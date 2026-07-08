# Engram Setup

## Purpose

click-ai-devkit bundles Engram as the persistent-memory backend for Claude Code sessions.

## Packaging model

Engram is treated as:

- a Go MCP binary
- plus a Claude Code plugin/runtime integration layer

For reproducibility, click-ai-devkit does **not** rely on a floating marketplace install.

## Pinning rule

`click` pins the Engram **binary version** per click-ai-devkit release.

That pin is applied by:

- downloading the release binary, or
- installing a tagged version with `go install ...@tag`

The exact version is controlled by click-ai-devkit release metadata.

## Claude Code wiring

`click` writes its own MCP entry with an **absolute binary path**.

This makes the installed machine deterministic:

- every developer gets the same Engram version for the same click-ai-devkit release
- `click update` can move the pin in a controlled way
- no hidden dependency on `latest`

## Operational expectation

- install should be reproducible
- update should be idempotent
- doctor should verify the expected wiring
- uninstall should remove Click-managed configuration cleanly
