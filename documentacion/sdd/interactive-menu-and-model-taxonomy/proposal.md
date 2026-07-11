# Proposal: Interactive Standing Menu + Model-Config Taxonomy Realignment

## Intent
`click` today only runs 5 fixed subcommands (`root.go`) with no default action, and `bubbletea` is wired into a single one-shot install screen — not a navigable menu. It also exposes an invented 5-phase model taxonomy (`modelconfig.go:14-19`: orchestrator, prd_writer, architect, reviewer, memory_curator) that does NOT match the real SDD phase set used by the ecosystem and by `click-sdd` itself. This change delivers the two P0 gaps: (1) a standing interactive menu at gentle-ai parity, and (2) a model-config taxonomy realigned to the real 12-13 phase set.

## Scope

### In Scope
- Running `click` with NO subcommand launches an interactive standing menu (j/k navigation, updates indicator at top).
- Menu wires existing working commands (install, doctor, update, uninstall) as active items; presets + agent-creation appear as inert "coming soon" placeholders for structural parity.
- Realign taxonomy to: `explore, propose, spec, design, tasks, apply, verify, archive, onboard, jd-judge-a, jd-judge-b, jd-fix-agent, default` across `modelconfig.go`, `models.json` schema, and `click-sdd` plugin skill names + `<phase>_model` config keys.
- Migration/reset path for the breaking `models.json` change (`click doctor` flags stale config; `click update` regenerates defaults).

### Out of Scope
- Ecosystem install presets (Memory Only / Dev Stack / Dev Stack+Polish / Custom) — placeholder only.
- Agent-creation flow (spike-f) — placeholder only.
- Engram auto-install (`engram.go`) — do not touch.
- Backward-compat shim for old model keys (breaking change accepted).

## Capabilities

### New Capabilities
- `interactive-menu`: standing TUI launched on bare `click`, navigable, TTY-aware, dispatches to existing commands + inert placeholders.

### Modified Capabilities
- `model-config`: phase taxonomy realigned from 5 invented phases to the real 12-13 phase set; models.json schema + plugin config keys + plugin skill names updated.

## Approach
Add a menu module (new `internal/menu` or `internal/tui`, TBD in design) hosting a persistent bubbletea program that dispatches selected items to existing cobra command runners. `root.go` gains a default action: if no subcommand AND stdout is a TTY, launch the menu; otherwise print help (non-interactive CI safety). Taxonomy realignment is a data+naming change: replace the phase slice, regenerate `models.json` defaults, rename `click-sdd` skills, and update the `--config <phase>_model=<alias>` emission.

## Affected Areas
| Area | Impact | Description |
|------|--------|-------------|
| `internal/cli/root.go` | Modified | Default no-arg action + TTY gate |
| `internal/menu` (new) | New | Standing menu model/view/update |
| `internal/modelconfig/modelconfig.go` | Modified | Phase list → real taxonomy |
| `internal/installer/models.go` + `models.json` | Modified | Schema/defaults regen |
| `internal/installer/plugins.go` | Modified | `<phase>_model` config keys |
| `plugins/click-sdd/skills/*` | Modified | Skill renames to real phases |

## Risks
| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Breaking existing `models.json` | High | Doctor flags stale; update regenerates; note in release |
| Bare `click` hangs CI waiting for TTY | Med | `isatty` gate → help on non-TTY |
| STRICT TDD for interactive TUI package | Med | Test model update/dispatch logic headless; keep view thin |
| Architecture shift (one-shot screen → standing program) | Med | Design phase decides module boundary + reuse of `internal/ui` |

## Rollback Plan
Revert the menu module + `root.go` default action (subcommands keep working untouched). Taxonomy revert = restore prior `modelconfig.go` phase slice, `models.json` defaults, and plugin skill names from git.

## Dependencies
- `bubbletea`/`bubbles`/`lipgloss` (already present).

## Success Criteria
- [ ] Bare `click` on a TTY opens a navigable menu matching gentle-ai's item shape; on non-TTY prints help (no hang).
- [ ] Active items dispatch install/doctor/update/uninstall with unchanged behavior; placeholders are visibly inert.
- [ ] `models.json` + plugin config keys + skill names use the real 12-13 phase taxonomy; doctor flags stale config.
- [ ] `go test ./...` green (TDD).
