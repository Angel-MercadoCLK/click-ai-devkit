# Design: click-skills plugin (vendored zesh-one-skills)

> Codex handoff — self-contained. Codex implements later with ZERO memory of this
> session. Every path, symbol, and test below is exact. Do NOT re-litigate the
> confirmed user decisions (new managed plugin `click-skills`, bundle ALL upstream
> skills, install alongside existing plugins, Apache-2.0 permissive → redistribute
> preserving attribution).

## Technical Approach

`click-skills` becomes the 4th click-managed Claude Code plugin, living at
`plugins/click-skills/` with the same shape as the existing three
(`.claude-plugin/plugin.json` + `skills/<name>/SKILL.md`). It carries a **vendored,
pinned snapshot** of every skill from `github.com/zeshone/zesh-one-skills` (latest
tag ~v2.7.0), plus `LICENSE` + `NOTICE` + `UPSTREAM_VERSION` to keep Apache-2.0
attribution honest. It has **no userConfig schema** (like click-memory/click-review),
so it installs silently through the existing `SyncMarketplacePlugins` loop with no
`--config` flags, and uninstalls through `RemoveMarketplacePlugins` automatically.
The only Go change beyond wiring is one new doctor check.

Source of truth for the install set is `managedPlugins` in
`internal/installer/plugins.go:20` plus the marketplace manifest and the embedded
release manifest — all three must gain a `click-skills` entry in lockstep.

## Architecture Decisions

### D1: Plugin tree layout & skill organization

**Choice**: `plugins/click-skills/` with:
- `.claude-plugin/plugin.json` — `{name: "click-skills", version: "0.1.0", description, author:{name:"Click Seguros"}}`, NO `userConfig` (mirrors `plugins/click-memory/.claude-plugin/plugin.json` exactly).
- `skills/<domain>-<name>/SKILL.md` — one directory per vendored skill, **flat**, directory + frontmatter `name` prefixed by upstream domain (`backend-`, `frontend-`, `apps-`, `shared-`).
- `LICENSE` (Apache-2.0 text), `NOTICE` (attribution), `UPSTREAM_VERSION` (pinned tag + commit SHA + source URL).

**Alternatives considered**: (a) nested `skills/backend/<name>/SKILL.md` domain folders; (b) keep raw upstream leaf names (`security`, not `backend-security`).
**Rationale**: click's established pattern is flat `skills/<name>/SKILL.md` (click-sdd, click-memory). Upstream has **colliding leaf names** across domains (backend `security` AND frontend `security`; both have `testing`/`validations`-style overlaps), and Claude Code requires unique skill names within a plugin — so the domain prefix is mandatory for uniqueness, applied to BOTH the directory and the frontmatter `name`. Nested folders are not click's convention and complicate the inventory guard.

### D2: Version pinning & Apache-2.0 attribution

**Choice**: **Vendor a pinned snapshot** at a fixed upstream tag. Record the tag/SHA/URL in `plugins/click-skills/UPSTREAM_VERSION` and `NOTICE`; ship the upstream Apache-2.0 `LICENSE`. Add `click-skills` to `internal/manifest/manifest.yaml` `plugins:` map (own click plugin `version` + `path`, same as peers). A test asserts LICENSE+NOTICE+UPSTREAM_VERSION exist and are non-empty and mention `Apache-2.0`.
**Alternatives considered**: (a) fetch skills from GitHub at install time; (b) add an `upstream_version` field to `manifest.Plugin` struct.
**Rationale**: Vendoring is offline-safe, reviewable, reproducible, and matches how click already ships click-sdd's own skills; it honors D16 ("manifest/​repo is the source of truth, no network at install"). Fetch-at-install breaks D16 and reproducibility. Extending the `manifest.Plugin` struct is avoided to minimize schema churn — the upstream tag is content metadata and lives in `UPSTREAM_VERSION`/`NOTICE`; the manifest entry only needs the click-side `version`+`path` that `manifest_test.go` enforces.

### D3: Wiring edits (exact)

**Choice**: Add `click-skills` to `managedPlugins` slice; it then flows through both `SyncMarketplacePlugins` (no `extraArgs` — the `if plugin == "click-sdd"` guard skips it) and `RemoveMarketplacePlugins` with zero new logic. Add a marketplace entry, a manifest entry, a doctor check, and update the two Spanish step labels.
**Alternatives considered**: a bespoke install path for skills.
**Rationale**: The managed-plugins loop already does exactly the right thing for a no-config plugin; reusing it is the smallest, safest change.

### D4: No new menu item; silent install

**Choice**: `click-skills` is purely part of `click install` (the managedPlugins loop). NO new `internal/menu/menu.go` entry, NO install-TUI confirmation. Only the Spanish step label text in `install.go`/`uninstall.go` is updated to name it.
**Rationale**: User said "en la instalación". Matches click-memory/click-review, which install silently with no prompt. Adding a menu item or confirmation is inconsistent friction. Reject.

### D5: Test strategy (STRICT TDD, headless)

All tests use the existing `fakeCommandRunner` + `CLICK_CLAUDE_HOME` sandbox — no new infra. Assert `claude plugin install click-skills@click-ai-devkit` is invoked with NO `--config`, positioned LAST (append order), and that uninstall/doctor/manifest all know it. Add a vendored-skill **inventory guard** test (canonical expected skill-dir set) plus a **license/attribution** test, analogous to the click-sdd lockstep tests — because the vendored skills are the plugin's entire payload and must not silently drift.

## Data Flow

    click install
        └─> SyncMarketplacePlugins(models, profile)
              addMarketplace(click-ai-devkit)
              for plugin in managedPlugins:            # now 4
                 click-sdd     -> install --config ...
                 click-memory  -> install (no config)
                 click-review  -> install (no config)
                 click-skills  -> install (no config)  # NEW, last
    click uninstall
        └─> RemoveMarketplacePlugins()  # loops managedPlugins -> uninstall each
    click doctor
        └─> doctor.Run(cfg) -> checkSkillsPlugin(cfg)  # NEW check

## File Changes

| File | Action | Description |
|------|--------|-------------|
| `plugins/click-skills/.claude-plugin/plugin.json` | Create | Manifest, no userConfig (copy click-memory shape) |
| `plugins/click-skills/skills/<domain>-<name>/SKILL.md` (~21) | Create | Vendored upstream skills, domain-prefixed |
| `plugins/click-skills/LICENSE` | Create | Upstream Apache-2.0 text |
| `plugins/click-skills/NOTICE` | Create | Attribution: Zesh-One, source URL, pinned tag, inspired-by gentleman-programming |
| `plugins/click-skills/UPSTREAM_VERSION` | Create | Pinned tag + commit SHA + source URL |
| `internal/installer/plugins.go` | Modify | Line 20: add `"click-skills"` to `managedPlugins` |
| `.claude-plugin/marketplace.json` | Modify | Add 4th plugin entry (`source: "./plugins/click-skills"`) |
| `internal/manifest/manifest.yaml` | Modify | Add `click-skills` under `plugins:` (version+path) |
| `internal/doctor/checks.go` | Modify | Add `checkSkillsPlugin(cfg)` + register in `Run()` |
| `internal/cli/install.go` | Modify | Step label (line ~66) names click-skills |
| `internal/cli/uninstall.go` | Modify | Step label (line ~30) names click-skills |
| `internal/installer/plugins_test.go` | Modify | marketplace plugins `want 4`; new `TestClickSkillsPlugin_ManifestAndFilesAreStructurallyValid` |
| `internal/installer/plugins_config_test.go` | Modify | Add click-skills to no-config assertion (line ~71) |
| `internal/installer/installer_test.go` | Modify | Command-order + uninstall now include click-skills (lines ~85, ~266) |
| `internal/manifest/manifest_test.go` | Modify | `wantPlugins` adds `click-skills` (line 30) |
| `internal/doctor/checks_test.go` | Modify | `wantChecks` 9→10 (line ~89); seed click-skills registry+enabled; healthy/unhealthy cases |
| `internal/cli/commands_test.go` | Modify | Install-step string + full install/uninstall command sequences add click-skills (lines ~178, ~492) |
| `internal/installer/click_skills_test.go` | Create | Inventory guard + LICENSE/NOTICE/UPSTREAM_VERSION attribution test |

## Interfaces / Contracts

New doctor check (mirrors `checkReviewPlugin` in `internal/doctor/checks.go`):

```go
func checkSkillsPlugin(cfg installer.Config) CheckResult {
    const name = "plugin click-skills"
    ok, err := installer.HasInstalledPlugin(cfg, "click-skills")
    // err -> unhealthy; !ok -> "no registrado en Claude Code"; else "registrado y habilitado"
}
```

`plugin.json` contract (byte-for-byte peer of click-memory):

```json
{ "name": "click-skills", "version": "0.1.0",
  "description": "Click Seguros bundle of Zesh-One company skills (vendored, Apache-2.0) for Claude Code.",
  "author": { "name": "Click Seguros" } }
```

## Testing Strategy

| Layer | What to Test | Approach |
|-------|-------------|----------|
| Unit | marketplace has 4 plugins; plugin.json valid; sample SKILL.md non-empty | `plugins_test.go` structural assert |
| Unit | install emits `install click-skills@click-ai-devkit` with NO `--config`, last | `plugins_config_test.go`, `installer_test.go` fake runner |
| Unit | uninstall removes click-skills | `installer_test.go` |
| Unit | doctor `checkSkillsPlugin` healthy/unhealthy; total checks = 10 | `checks_test.go` seeded registry |
| Unit | manifest lists click-skills | `manifest_test.go` |
| Unit | LICENSE+NOTICE+UPSTREAM_VERSION present, non-empty, mention Apache-2.0; vendored skill inventory matches canonical set | new `click_skills_test.go` |
| Manual | after `click install`, `/skills` (or skill list) shows vendored skills in a real Claude Code session | manual-verify |

## Migration / Rollout

No migration. Additive plugin. Existing installs pick up click-skills on next
`click install`/`click update`. Rollback = revert the wiring commits; `click
uninstall` already removes it via the managedPlugins loop.

## Open Questions

- [ ] Confirm latest upstream tag (WebFetch was unavailable this session — exploration recorded ~v2.7.0). Pin the exact tag/SHA at vendor time.
- [ ] Confirm a **root LICENSE** exists in zesh-one-skills for redistribution (exploration noted Apache-2.0 is declared per-skill in frontmatter but root LICENSE was unconfirmed). If only per-skill, NOTICE must cite the per-skill frontmatter license as the redistribution basis.
- [ ] Confirm whether any upstream skills should be excluded — user decision is "bundle ALL"; honor it, but flag the .NET/Next/Ionic stack mismatch with click's Go codebase as informational only (they are inert until a session needs them).
