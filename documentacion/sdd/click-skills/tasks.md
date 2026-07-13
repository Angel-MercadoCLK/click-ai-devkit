# Tasks: click-skills plugin (vendored zesh-one-skills)

> STRICT TDD. Every unit: write the failing test FIRST (RED), run `go test ./...`
> to see it fail, then implement to GREEN. Codex handoff — no prior memory assumed.
> Repo: `C:\Proyectos\click-ai-devkit`, Go 1.24.2. Test command: `go test ./...`.
> Peer references: `plugins/click-memory` (no-config plugin shape),
> `internal/doctor/checks.go` `checkReviewPlugin` (doctor pattern),
> `internal/installer/plugins_test.go` `assertPluginStructure` (structural helper).

## Phase 0 — Pre-flight (manual / blocking, no code)

- [ ] **T0.1** WebFetch `https://github.com/zeshone/zesh-one-skills`; confirm the latest release tag (exploration recorded ~v2.7.0) and record the exact tag + commit SHA.
- [ ] **T0.2** Confirm redistribution basis: check for a **root LICENSE** file; if absent, confirm each SKILL.md frontmatter declares `license: Apache-2.0`. Record the redistribution basis for NOTICE.
- [ ] **T0.3** Enumerate the full upstream skill set and freeze the **canonical inventory** (domain-prefixed dir names, e.g. `backend-security`, `frontend-react-19`, `apps-capacitor`, `shared-github-pr`). This list drives T2.1's inventory guard.

## Phase 1 — Vendor the snapshot (content)

- [ ] **T1.1** Create `plugins/click-skills/.claude-plugin/plugin.json` — copy `plugins/click-memory/.claude-plugin/plugin.json` shape; name `click-skills`, version `0.1.0`, description mentioning "Zesh-One company skills (vendored, Apache-2.0)", author `Click Seguros`. NO `userConfig`.
- [ ] **T1.2** Vendor each upstream skill into `plugins/click-skills/skills/<domain>-<name>/SKILL.md`. Prefix BOTH the directory AND the frontmatter `name` with the domain (`backend-`/`frontend-`/`apps-`/`shared-`) to guarantee uniqueness. Preserve upstream frontmatter (`license`, `metadata.author: Zesh-One`, `inspired-by`).
- [ ] **T1.3** Add `plugins/click-skills/LICENSE` (upstream Apache-2.0 text), `plugins/click-skills/NOTICE` (attribution: Zesh-One, source URL, pinned tag from T0.1, inspired-by gentleman-programming, redistribution basis from T0.2), `plugins/click-skills/UPSTREAM_VERSION` (tag + SHA + URL).

## Phase 2 — Content guard tests (RED → GREEN)

- [ ] **T2.1 (RED)** Create `internal/installer/click_skills_test.go`:
  - `TestClickSkills_LicenseAndAttributionPresent` — `LICENSE`, `NOTICE`, `UPSTREAM_VERSION` exist, non-empty, and `NOTICE`/`LICENSE` contain `Apache-2.0`; `NOTICE` contains `Zesh-One`.
  - `TestClickSkills_SkillInventory` — the set of dirs under `plugins/click-skills/skills/` equals the canonical inventory (T0.3); each has a non-empty `SKILL.md`. Use the `mustReadRepoFile` / `os.ReadDir` patterns already in `plugins_test.go` / `plugins_lockstep_test.go`.
  Run `go test ./internal/installer/...` → FAIL (files/inventory not matching yet).
- [ ] **T2.2 (GREEN)** Ensure T1.1–T1.3 content satisfies the guards. `go test ./internal/installer/...` → PASS.

## Phase 3 — Structural + manifest wiring (RED → GREEN per unit)

- [ ] **T3.1 (RED)** `internal/installer/plugins_test.go`: change `TestMarketplaceManifest_Parses` `want 3` → `want 4` (lines 28-29); add `TestClickSkillsPlugin_ManifestAndFilesAreStructurallyValid` via `assertPluginStructure(t, filepath.Join("plugins","click-skills"), "click-skills", []string{ a few sample skills[...] })`. FAIL.
- [ ] **T3.2 (GREEN)** Add the 4th plugin object to `.claude-plugin/marketplace.json` (`name: click-skills`, `source: "./plugins/click-skills"`, category `productivity`, homepage, description). PASS.
- [ ] **T3.3 (RED)** `internal/manifest/manifest_test.go` line 30: add `"click-skills"` to `wantPlugins`. FAIL.
- [ ] **T3.4 (GREEN)** Add `click-skills` block under `plugins:` in `internal/manifest/manifest.yaml` (`version: "0.0.0-placeholder"`, `path: "plugins/click-skills"`). PASS.

## Phase 4 — Install/uninstall command wiring (RED → GREEN)

- [ ] **T4.1 (RED)** `internal/installer/plugins_config_test.go`: extend the expected command slice (line ~71) to include `{Name:"claude", Args:[]string{"plugin","install","click-skills@click-ai-devkit"}}` AFTER click-review, with NO `--config`. FAIL.
- [ ] **T4.2 (RED)** `internal/installer/installer_test.go`: add `"claude plugin install click-skills@click-ai-devkit"` to the expected install sequence (after line ~86) and the uninstall sequence (line ~266 region). FAIL.
- [ ] **T4.3 (GREEN)** `internal/installer/plugins.go` line 20: append `"click-skills"` to `managedPlugins`. The `if plugin == "click-sdd"` guard already ensures no `--config` for it; `RemoveMarketplacePlugins` loops the same slice. `go test ./internal/installer/...` → PASS.

## Phase 5 — Doctor check (RED → GREEN)

- [ ] **T5.1 (RED)** `internal/doctor/checks_test.go`: bump `wantChecks` 9→10 (line ~89) and its message; add click-skills to the seeded installed-plugins registry + `enabledPlugins` (lines ~270/288 region); add a healthy case and a "no registrado" unhealthy case mirroring the click-review check tests. FAIL.
- [ ] **T5.2 (GREEN)** `internal/doctor/checks.go`: add `checkSkillsPlugin(cfg)` (copy `checkReviewPlugin`, swap to `"click-skills"` / `"plugin click-skills"`) and register it in `Run()`'s slice after `checkReviewPlugin(cfg)`. PASS.

## Phase 6 — CLI step labels (RED → GREEN)

- [ ] **T6.1 (RED)** `internal/cli/commands_test.go`: update the full install command-sequence expectation (line ~492) and uninstall sequence to include click-skills; the install-output substring check (line ~178) stays green if the new label still contains `click-memory`. Update any exact sequence asserts. FAIL.
- [ ] **T6.2 (GREEN)** `internal/cli/install.go` (step label line ~66) and `internal/cli/uninstall.go` (line ~30): reword the Spanish labels to name click-skills (e.g. "Registrando plugins click-sdd, click-memory, click-review y click-skills…"). PASS.

## Phase 7 — Full green + manual verify

- [ ] **T7.1** `go test ./...` → ALL PASS.
- [ ] **T7.2 (manual-verify)** In a real Claude Code session after `click install`: run `/skills` (or open the skills list) and confirm the vendored click-skills entries appear and are invocable. Cannot be asserted in Go tests — mark manual.

## Acceptance Criteria

1. `plugins/click-skills/` exists with `plugin.json` (no userConfig), the full vendored skill set (domain-prefixed, unique names), `LICENSE`, `NOTICE`, `UPSTREAM_VERSION`.
2. `managedPlugins` includes `click-skills`; `click install` invokes `claude plugin install click-skills@click-ai-devkit` with NO `--config`, and `click uninstall` removes it — both proven by fake-runner tests.
3. `.claude-plugin/marketplace.json` and `internal/manifest/manifest.yaml` both list click-skills; structural + manifest tests green (marketplace `want 4`).
4. `click doctor` reports `plugin click-skills` health; total checks = 10; healthy/unhealthy paths tested.
5. LICENSE/NOTICE/UPSTREAM_VERSION + inventory guard tests green (Apache-2.0 attribution honest, no silent vendored drift).
6. `go test ./...` fully green.
7. **Manual**: vendored skills visible/invocable via `/skills` in a real Claude Code session (T7.2).

## Review Workload Forecast

- **Authored/reviewable diff** (Go + JSON + YAML + tests + LICENSE/NOTICE): ~300–380 changed lines. plugins.go (1), marketplace.json (~12), manifest.yaml (~4), checks.go (~15), install/uninstall labels (~2), plugin.json/NOTICE/UPSTREAM_VERSION (~30), tests across 6 files (~250–320).
- **Vendored content diff** (~21 SKILL.md + LICENSE): potentially 1000+ lines, but third-party vendored docs — reviewed for attribution/inventory only, not line-by-line logic.

**Decision needed before apply: Yes** (confirm chained-PR split; confirm latest upstream tag T0.1; confirm root-LICENSE redistribution basis T0.2).
**Chained PRs recommended: Yes.**
**400-line budget risk: Medium** (authored diff alone ~300–380, borderline; vendored content pushes total far over budget but is `size:exception` vendored material).

Recommended slices:
- **PR1 — vendored snapshot + content guards** (Phases 1–2): the `plugins/click-skills/` tree, LICENSE/NOTICE/UPSTREAM_VERSION, and the inventory/attribution tests. Bulk is vendored → tag PR `size:exception`, review = attribution + inventory correctness. Self-contained: adds an inert plugin dir not yet wired in.
- **PR2 — Go wiring + tests** (Phases 3–7): managedPlugins, marketplace.json, manifest.yaml, doctor, CLI labels, and all Go tests. ~300–380 authored lines, under/at budget, standard review. Depends on PR1's tree existing.
