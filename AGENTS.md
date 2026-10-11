# click-ai-devkit — contributor guide

`click-ai-devkit` is a Go installer/manager for Click Seguros guidance and tooling. It layers
Click-specific business and memory policy, a Claude memory guard, and Engram Cloud enrollment on
the pinned `gentle-ai` v4.0.0 distribution. Gentle AI owns ODD/RDD workflow, runtime adapters,
permissions, model selection, Engram lifecycle, and shared Engram/Context7 configuration. Click
ships three Claude plugins: `click-memory`, `click-review`, and `click-skills`; it no longer ships
or manages a Click SDD plugin. See `documentacion/README.md` for the current documentation map.

## Build and test

```sh
go build ./...
go test ./...
```

**Strict TDD is mandatory for any Go change.** Add a focused failing test first, observe RED,
implement the minimum change, observe GREEN, then refactor. Decision D13 is in
`documentacion/00-decisions-and-open-questions.md`.

Tests must never touch real `~/.claude`, `~/.codex`, or `~/.openclaw` state. Tests that invoke the
pinned Gentle AI installer isolate `HOME`, `USERPROFILE`, and `XDG_CONFIG_HOME` in `t.TempDir()` and
use its standard `.claude`, `.codex`, and `.openclaw` subdirectories. Do not set target-root overrides
in these integration fixtures; the pinned v4.0.0 adapters resolve destinations from process home.
An isolated regression may assert that a nonstandard configured target is rejected before snapshot,
provisioning, runner, or configuration writes.
Other Click-only tests may use target-root overrides where useful.

## Language and decisions

Technical artifacts, code, comments, and documentation default to English. Preserve the existing
Spanish README convention. Developer-facing CLI/TUI string literals are Spanish, per D10. Read the
locked decisions before changing behavior; D48–D53 define current distribution and ownership. The
current contracts and runbooks are linked from
`documentacion/README.md`.

## Ownership boundaries

- Gentle AI v4.0.0 is pinned and checksum-qualified. The selected target roots must be the standard
  process-home directories; reject incompatible overrides before mutation (D51).
- Install/update use the `full-gentleman` preset with `engram,skills,context7,permissions,persona`
  and the `neutral` persona, limited to selected Claude Code, Codex, and OpenClaw targets (D49).
- Gentle AI owns permission configuration and allow/deny/ask lists. Click retains its guard and
  insurance-data policies but does not write permission lists (D52).
- Click retires model selection and preserves existing model/profile artifacts and native settings.
  Do not infer role mappings or edit private Gentle AI state (D53).
- Click owns its business/memory guidance, Claude memory guard and three retained Claude plugins,
  Codex's managed `AGENTS.md` block, OpenClaw's managed workspace/`clickhola` content, and Click's
  Engram Cloud enrollment. Gentle AI owns shared Engram/Context7 registration and configuration.
  Uninstall must preserve shared upstream state.
- Rollback restores only verifiably Click-owned content. It preserves the current Cloud token,
  surrounding user content, shared native configuration, and unknown/unqualified paths.

## Claude plugins

The Claude marketplace manifest contains exactly `click-memory`, `click-review`, and `click-skills`.
Keep `.claude-plugin/marketplace.json`, `internal/installer/plugins.go`, the relevant doctor checks,
and their tests consistent when changing plugin lifecycle behavior. Do not add Claude plugin
registration to Codex or OpenClaw.

## Commits

Use Conventional Commits. Never add `Co-Authored-By` or other AI attribution to commits.
