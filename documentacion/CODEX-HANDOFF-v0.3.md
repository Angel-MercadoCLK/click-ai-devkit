# Codex Handoff — click-ai-devkit v0.3 roadmap

You (Codex) will implement the features below on `click-ai-devkit`, a Go 1.24.2
cobra/bubbletea CLI that installs and manages a Claude Code ecosystem (plugins,
Engram, Context7, a managed `CLAUDE.md` block, a memory-guard hook). You have
**zero memory** of the planning conversation — everything you need is in this doc
and the per-feature SDD artifacts it points to. Read this whole file first.

---

## 0. Current state (start here)

- `main` is green and released as **v0.2.1** (installable via `scoop install click`
  from `https://github.com/Angel-MercadoCLK/click-ai-devkit` — live today, not
  pending a tag push).
- The standing interactive menu (`internal/menu/menu.go`), the 18-phase model
  taxonomy (`internal/modelconfig` — the 9 flow phases + Judgment Day's 3 roles +
  the 5 review-lens roles, up from 13 pre-PR #12), orchestration profiles
  (balanced/cost-saver/quality/custom), and profile-aware install/update/doctor
  all already shipped.
- A post-v0.2.0 full-4R audit + a Phase-1 fix round already merged (PR #9): the 3
  CRITICALs are fixed and the menu was trimmed. **Do not re-open those.** **A —
  review-role models is DONE** (merged via PR #12, `feature/review-role-models`;
  see `documentacion/sdd/review-role-models/{design,tasks}.md`). **F —
  agent-builder is far along but not yet merged**: implemented across
  `feat/agent-builder-domain` → `feat/agent-builder-flow` →
  `feat/agent-builder-wizard` → `feat/agent-builder-cli`, currently in a
  pre-merge fix round against a full adversarial audit (3 confirmed CRITICALs;
  see Engram topic `review/agent-builder-v0.3/ledger`) — **do not treat it as
  shipped until that branch merges to `main`.** After Phase 1 and pending F's
  merge, the inert `(próximamente)` menu items remaining are: **Presets de
  instalación · Sincronizar configuración · Gestionar backups** (agent-builder's
  "Crear tu propio agente" moves out of this list once its branch merges). The
  two "OpenCode …" items and "Actualizar + Sincronizar" were intentionally
  removed (click is Claude Code only).

## 1. Non-negotiable execution rules

1. **STRICT TDD** is mandatory (repo policy, root `CLAUDE.md` / decision D13):
   write a failing test first, then the minimal implementation, for every unit.
2. **One feature = one branch off `main`**, delivered as the chained PRs its
   `tasks.md` prescribes (respect the ~400-line review budget; use `size:exception`
   only for vendored content). Never a single mega-PR.
3. **Before every PR**: `go build ./...`, `go vet ./...`, `go test ./... -count=1`
   all green, and `gofmt -l` clean on **every file you touch** (there is
   pre-existing repo-wide CRLF/gofmt debt on files you don't touch — leave it, or
   fix it in a dedicated `.gitattributes` cleanup PR, but never let your touched
   files be dirty).
4. **Self-review before opening a PR**: run an adversarial pass over your own diff
   across correctness/security/resilience/readability; fix any BLOCKER/CRITICAL and
   re-verify before requesting merge. (This is how every prior PR shipped.)
5. **TUI features have a manual gate**: bubbletea/PTY rendering cannot be
   automated. Keep `Model`/`Update`/`View` pure and headless-testable (drive
   `Update` with synthetic `tea.KeyMsg`; never spawn a real program in tests). For
   each TUI feature, add a documented manual smoke-test step to the PR.
6. **Sandbox for real-run testing** (never touch a real `~/.claude`): set
   `CLICK_CLAUDE_HOME=<tempdir>` — it redirects both click's own files and the
   spawned `claude` subprocess (`CLAUDE_CONFIG_DIR`). Verified: a full sandboxed
   `click install` leaves the real `~/.claude` byte-identical.
7. **Language (locked decision D10)**: docs, README, code, comments, commit
   messages, PR descriptions → **English**. Dev-facing CLI/TUI **string literals**
   → **Spanish** (voseo, matching `internal/ui/modelselect.go` /
   `profileselect.go`). See `AGENTS.md` (already reconciled with D10 in Phase 1).

## 2. Confirmed product decisions (do NOT re-litigate)

- **zesh-one-skills → `click-skills` plugin, ALL skills, vendored.** The company
  skills at `https://github.com/zeshone/zesh-one-skills` are Claude Code SKILL.md
  files. Bundle a **pinned vendored snapshot at upstream tag `v2.7.0`** into
  `plugins/click-skills/`. The repo has **no root LICENSE** (GitHub detects
  `license: None`); individual skills declare Apache-2.0 in frontmatter. **The
  repo owner has authorized vendoring now**, on the basis of preserving the
  per-skill Apache-2.0 attribution — so you MUST ship a `NOTICE`/`CREDITS`
  recording the upstream source, tag `v2.7.0`, and the per-skill Apache-2.0
  attribution, and preserve each skill's frontmatter license field. Record the
  pinned tag in `plugins/click-skills/UPSTREAM_VERSION` and (per the design) the
  manifest. Skills live under upstream `skills/{apps,backend,dotnet-10,frontend,
  shared}` — flatten to unique `plugins/click-skills/skills/<domain>-<name>/` dirs
  (upstream has colliding leaf names across domains).
- **click-skills installs silently as part of `click install`** (like
  click-memory/click-review), no new menu item, no install-time confirmation.
- **review-role models (DONE, PR #12)**: the model taxonomy was extended with 5
  `modelconfig.Phase` entries (13→18): `review-risk`, `review-readability`,
  `review-reliability`, `review-resilience`, `review-refuter`. They were added to
  the lockstep test's `phasesWithoutDedicatedSkill` exemption (they are review
  roles, not SDD skills). Do not re-implement this — it's merged and live.

## 3. Features to implement (each has full design + tasks on disk + in Engram)

| # | Feature | Artifacts | Status | Depends on |
|---|---------|-----------|--------|-----------|
| A | **review-role models** (gentle-ai improvement: model per 4R lens) | `documentacion/sdd/review-role-models/{design,tasks}.md` | **DONE — merged (PR #12)** | — |
| B | **Sincronizar configuración** (`click sync`: re-apply config, no version bump) | `documentacion/sdd/sync-config/{design,tasks}.md` | not started, ~300 LOC, **1 PR** | — |
| C | **click-skills** (vendor zesh-one-skills v2.7.0) | `documentacion/sdd/click-skills/{design,tasks}.md` | not started, 24 tasks, **2 PRs** (vendored snapshot `size:exception` → Go wiring) | — |
| D | **Presets de instalación** | `documentacion/sdd/install-presets/{design,tasks}.md` | not started, ~510 LOC, **2 PRs** | A (both touch `profiles.go`/`modelselect.go`) — A done, D unblocked |
| E | **Gestionar backups** (dedup + keep-5 + pin, gentle-ai improvement) | `documentacion/sdd/manage-backups/{design,tasks}.md` | not started, ~520 LOC, **4 slices**, high risk | — |
| F | **Crear tu propio agente** (agent-builder) | `documentacion/sdd/agent-builder-flow/{proposal,spec,design,tasks}.md` + `documentacion/CODEX-HANDOFF.md` | **in progress, near merge**: implemented across `feat/agent-builder-{domain,flow,wizard,cli}`, currently in a pre-merge fix round (see `review/agent-builder-v0.3/ledger`) | R1-001 (already fixed in Phase 1 → was unblocked) |

Each `tasks.md` contains its own ordered RED→GREEN checklist, Review Workload
Forecast, chain strategy, and acceptance criteria. Follow it.

## 4. Recommended sequence + conflict avoidance

Suggested order (fastest wins first, conflict-aware):

1. **A — review-role models** (small, config-only, no decision needed).
2. **D — Presets** *after A* (both edit `internal/modelconfig/profiles.go` and
   `internal/ui/modelselect.go`; sequencing them avoids merge conflicts).
3. **B — Sincronizar configuración** (isolated; touches `installer/plugins.go`
   sync split + a new `click sync` command).
4. **C — click-skills** (isolated; `managedPlugins` + marketplace + manifest +
   doctor + vendored tree).
5. **F — agent-builder** (isolated; consumes the now-hardened
   `internal/installer/profile_artifacts.go`).
6. **E — Gestionar backups** (largest; 4 slices; touches `claudemd.go`,
   `config.go`, `install.go`, `update.go`).

A/D and C/E/B/F are otherwise independent and could interleave, but land A before
D, and avoid running two features that both edit `modelconfig`/`profiles.go`/
`plugin.json`/`modelselect.go` at the same time.

## 5. Cross-cutting gotchas (from the audit + planning)

- **Backups restore MUST go through `installer.WriteManagedBlock`** (a new
  `ReadManagedBlockContent` reader is required) — never whole-file overwrite, or
  you corrupt the delimited managed block and clobber the user's own CLAUDE.md.
- **review-role lockstep**: `TestClickSDDSkills_LockstepWithModelconfigPhases`
  requires a `skills/<phase>/` dir per phase; the 5 review lenses have none →
  add them to `phasesWithoutDedicatedSkill` or the whole suite fails.
- **Sync boundary**: the "no version bump" line is skipping the `addMarketplace`
  refresh — factor `SyncMarketplacePluginConfigs` out of `SyncMarketplacePlugins`.
  Validate against the real `claude` CLI that skipping it truly avoids a
  plugin-version bump (flagged open item).
- **Profile label consistency** (from the audit): any new writer of `models.json`
  MUST use `SaveModelsWithProfile` + `EffectiveProfileName`, never bare
  `SaveModels` (that was CRITICAL C1 — don't reintroduce it in presets/sync).
- **Menu wiring**: inert items are `Active:false` with no `Action`. To activate
  one, give it an `Action`, add the `ActionArgs` case, and wire a hidden cobra
  command (pattern: `internal/cli/configuremodels.go` = the `click configure-models`
  precedent). Menu tests index off `len(Items)` — keep them dynamic.

## 6. Release (when ready to ship v0.3)

The release pipeline is robust now: bump `internal/manifest/manifest.yaml`
`click_version` to the new version, commit, then `git tag v0.3.0 && git push
--tags`. GoReleaser (with `--skip=validate`, already wired) builds all 5
platforms, publishes the GitHub release, and commits the updated `bucket/click.json`
scoop manifest to `main`. `scoop update click` then delivers it.

---

*Handoff generated 2026-07-13. Planning artifacts also in Engram under topics
`sdd/click-skills/*`, `sdd/manage-backups/*`, `sdd/review-role-models/*`,
`sdd/install-presets/*`, `sdd/sync-config/*`, `sdd/agent-builder-flow/*`, and the
audit at `review/main-post-merge/ledger`.*
