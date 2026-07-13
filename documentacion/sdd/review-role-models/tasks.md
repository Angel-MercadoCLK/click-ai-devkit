# Tasks: review-role models

STRICT TDD — RED (failing test) → GREEN (minimal impl). Artifacts in English. This change is small,
mechanical, and additive; a single PR fits the review budget. Do the taxonomy const first so every
downstream lockstep test compiles against the new `Phases`.

## Ordered checklist

1. [x] RED: `modelconfig_test.go` — assert `len(Phases)==18`, correct order (5 lenses between `jd-fix-agent` and `default`), and `Defaults()` returns `"sonnet"` for each of the 5 lenses. GREEN: add 5 `Phase` consts + append to `Phases` + 5 `Defaults()` entries in `modelconfig.go`.
2. [x] RED: `modelconfig_test.go` — `PhaseReviewRisk.ConfigKey()=="review_risk_model"` (and the other 4). GREEN: no code change expected (rule already covers it) — test documents/locks the mapping.
3. [x] RED: `profiles_test.go` — `costSaverDefaults()` has `"haiku"` and `qualityDefaults()` has `"opus"` for all 5 lenses; both maps have 18 keys. GREEN: add 5 entries to each map in `profiles.go`.
4. [x] RED: `modelselect_test.go` — every `modelconfig.Phases` entry has a `phaseLabels` value (loop assertion) and the TUI renders 18 rows. GREEN: add 5 `phaseLabels` entries in `modelselect.go`.
5. [x] RED: run `TestClickSDDPluginJSON_ConfigKeysMatchModelconfigPhasesExactly` — fails (missing 5 keys). GREEN: add the 5 `review_*_model` userConfig fields to `plugin.json` (defaults: risk/readability/reliability/resilience/refuter = sonnet).
6. [x] RED: run `TestClickSDDSkills_LockstepWithModelconfigPhases` — fails (demands `skills/review-*/SKILL.md`). GREEN: add the 5 lens phases to `phasesWithoutDedicatedSkill` in `plugins_lockstep_test.go`.
7. [x] GREEN: run `TestSyncMarketplacePlugins_PassesPerPhaseConfigFlagsForClickSDD` (plugins_config_test.go) — confirm it now emits `--config review_*_model=<alias>` for the 5 lenses; extend the assertion if it enumerates expected keys.
8. [x] REFACTOR + full `go test ./...`; `go vet`. Confirm `TestClickSDDSkills_NoOrphanPhaseDirectories` still passes (no new dirs created).

## Review Workload Forecast

- Estimated changed lines: ~110 (all edits to existing files + test additions; no new files).
- Decision needed before apply: No
- Chained PRs recommended: No
- 400-line budget risk: Low
- Single PR is appropriate; standard-tier review (one lens — `review-reliability` for taxonomy/lockstep correctness).

## Acceptance Criteria

- [x] `modelconfig.Phases` has 18 entries in the documented order.
- [x] `Defaults`, `costSaverDefaults`, `qualityDefaults` each define all 18 phases (no missing lens).
- [x] `plugin.json` userConfig keys == `orchestration_profile` ∪ `ConfigKey(Phases)` exactly (both lockstep assertions green).
- [x] `configure-models`/install TUI shows 18 selectable rows including the 5 review lenses.
- [x] `SyncMarketplacePlugins` emits `--config review_*_model=<alias>` pairs.
- [x] `models.json` schema_version unchanged; existing files load without migration.
- [x] `go test ./...` fully green; no orphan skill-dir failures.
- [ ] MANUAL: eyeball the 18-row TUI on a real terminal to confirm labels/ordering read well (headless tests cannot render a live program).
