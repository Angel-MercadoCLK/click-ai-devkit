# Design: review-role models (per-lens model config for the 4R review roles)

## Technical Approach

Extend click's existing 13-phase model taxonomy to also cover the five review lenses gentle-ai now
lets you tune per role — `review-risk`, `review-readability`, `review-reliability`,
`review-resilience`, `review-refuter`. Add them as NEW entries in `modelconfig.Phases` so the entire
existing plumbing (defaults, presets, `ConfigKey()`, `--config` flag emission via
`SyncMarketplacePlugins`, `models.json` persistence, `configure-models` TUI, `doctor` reporting,
plugin.json lockstep) picks them up with zero new machinery. This is config-only in this change:
the orchestrator/click-review agents do not yet read the values.

## Architecture Decisions

| Decision | Choice | Rejected alternative | Rationale |
|----------|--------|----------------------|-----------|
| Model surface | Add 5 lenses as new `modelconfig.Phase` consts appended to `Phases` | Separate `review-lens` model map + parallel persistence | A separate map would duplicate `models.json` shape, TUI, flag emission, plugin.json fields, and profiles — doubling maintenance. Everything already iterates `modelconfig.Phases`; adding entries reuses `ConfigKey()`, `SyncMarketplacePlugins`, `SaveModelsWithProfile`, `doctor`, and both lockstep tests for free. This is the taxonomy-consistent path already blessed by the jd-* phases. |
| Skill-lockstep seam | Add the 5 lenses to `phasesWithoutDedicatedSkill` in `plugins_lockstep_test.go` | Create 5 `skills/review-*/SKILL.md` dirs | Review lenses are review ROLES executed by click-review/orchestrator, not SDD phase workflow skills. The exemption map is the exact seam the test author left for "config-only phases" (like `default`). WITHOUT this, `TestClickSDDSkills_LockstepWithModelconfigPhases` fails demanding skill dirs. |
| Preset assignment | Mirror the jd-* judges exactly: balanced→sonnet, cost-saver→haiku, quality→opus for all 5 lenses | Bespoke per-lens tuning | Review lenses are peer adversarial-review roles to the JD judges; matching their preset treatment keeps presets predictable and easy to reason about. |
| Config home | Keep the 5 new `<lens>_model` fields in click-sdd's `plugin.json` userConfig alongside the 13 existing keys | Move them to click-review's plugin.json | All model taxonomy lives centrally in click-sdd's userConfig; splitting it across plugins would break the single `TestClickSDDPluginJSON_ConfigKeysMatchModelconfigPhasesExactly` invariant and `SyncMarketplacePlugins`'s single flag-set. Accept the minor semantic mismatch (review roles configured via the SDD plugin). |
| Orchestrator consumption | Config-only now; no reader in `click-orchestrator.md`/click-review | Wire consumption in same change | Keeps the change small and mechanical; consumption is a separate, larger follow-up. The values still land in `settings.json` pluginConfigs, ready to be read later. |

## Data Flow

    modelconfig.Phases (13 → 18) ──► Defaults()/costSaver/quality (+5 each)
             │                                   │
             ├─► ui phaseLabels (+5) ─► configure-models & install TUI rows (13 → 18)
             ├─► ConfigKey() ─► SyncMarketplacePlugins ─► --config review_*_model=<alias>
             └─► plugin.json userConfig (+5 fields)  ◄── lockstep test asserts exact set match

## File Changes

| File | Action | Description |
|------|--------|-------------|
| `internal/modelconfig/modelconfig.go` | Modify | Add 5 `Phase` consts (`PhaseReviewRisk="review-risk"`, `-Readability`, `-Reliability`, `-Resilience`, `PhaseReviewRefuter="review-refuter"`); append to `Phases` (place after `PhaseJDFixAgent`, before `PhaseDefault`); add 5 entries to `Defaults()` → `"sonnet"`. |
| `internal/modelconfig/modelconfig_test.go` | Modify | Assert `Phases` length/order, `Defaults` covers the 5, `ConfigKey()` maps e.g. `review-risk`→`review_risk_model`. |
| `internal/modelconfig/profiles.go` | Modify | Add 5 entries to `costSaverDefaults()` → `"haiku"` and `qualityDefaults()` → `"opus"`. |
| `internal/modelconfig/profiles_test.go` | Modify | Assert preset maps stay full-taxonomy and `EffectiveProfileName` still round-trips. |
| `internal/ui/modelselect.go` | Modify | Add 5 entries to `phaseLabels` (short labels `review-risk`, etc.). |
| `internal/ui/modelselect_test.go` | Modify | Assert all `modelconfig.Phases` (now 18) have a label; row count. |
| `plugins/click-sdd/.claude-plugin/plugin.json` | Modify | Add 5 userConfig fields: `review_risk_model` (default sonnet), `review_readability_model`, `review_reliability_model`, `review_resilience_model`, `review_refuter_model`. |
| `internal/installer/plugins_lockstep_test.go` | Modify | Add the 5 lens phases to `phasesWithoutDedicatedSkill` so the skill-dir lockstep exempts them. |

## Interfaces / Contracts

```go
// internal/modelconfig/modelconfig.go — new consts
const (
    PhaseReviewRisk        Phase = "review-risk"
    PhaseReviewReadability Phase = "review-readability"
    PhaseReviewReliability Phase = "review-reliability"
    PhaseReviewResilience  Phase = "review-resilience"
    PhaseReviewRefuter     Phase = "review-refuter"
)
// Phases order: ...PhaseJDFixAgent, PhaseReviewRisk, PhaseReviewReadability,
//   PhaseReviewReliability, PhaseReviewResilience, PhaseReviewRefuter, PhaseDefault
```

`ConfigKey()` needs NO change — its `-`→`_` + `_model` rule already yields `review_risk_model` etc.
`SyncMarketplacePlugins`, `SaveModelsWithProfile`, `doctor`, and the TUIs need NO code change —
they all iterate `modelconfig.Phases` and pick the new entries up automatically.

## Testing Strategy

| Layer | What to Test | Approach |
|-------|-------------|----------|
| Unit (modelconfig) | 18 phases present/ordered; `Defaults`/costSaver/quality cover all 18; `ConfigKey` mapping for each lens | table tests |
| Unit (ui) | every phase has a label; TUI renders 18 rows | existing modelselect tests |
| Lockstep | plugin.json userConfig == `{ProfileConfigKey}` ∪ `ConfigKey(Phases)` (auto-passes once 5 fields added); skill-dir lockstep passes via exemption | existing lockstep tests (must stay green) |
| Integration | `SyncMarketplacePlugins` emits `--config review_*_model=<alias>` for the 5 lenses | existing plugins_config_test style |

## Migration / Rollout

`models.json` schema is unchanged (still a `map[phase]string`); older files simply lack the 5 new
keys and `Resolve`/`ResolveForProfile` fill them from the profile defaults — no bump of
`CurrentModelsSchemaVersion` needed. Fully additive. No orchestrator behavior change.

## Open Questions

- [ ] Confirm balanced default for lenses = `sonnet` (mirrors jd-judges) vs `opus` for `review-risk` (security lens). Proposed: sonnet for all 5 for consistency; revisit if security review demands opus by default.
- [ ] Follow-up (out of scope): have `click-orchestrator.md`/click-review actually consume `<lens>_model` from settings.json pluginConfigs.
