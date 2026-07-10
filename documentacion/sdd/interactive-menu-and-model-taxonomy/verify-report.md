# Verify Report: interactive-menu-and-model-taxonomy — PR1 / Work Unit 1

> Persistence note: Engram MCP tools were unavailable in this verify run's toolset.
> Spec/tasks artifacts have no on-disk file fallback (only proposal.md + apply-progress.md exist);
> WU1 requirements were reconstructed from proposal.md and apply-progress.md citations, then
> verified independently against source. Reconcile into Engram topic
> `sdd/interactive-menu-and-model-taxonomy/verify-report` when MCP is restored.

## Scope
PR1 / Work Unit 1 ONLY (stacked-to-main chain). Work Unit 2 (interactive menu, internal/menu,
root.go default action) and Work Unit 3 (skill/agent content, taxonomy-lockstep test) are
by-design out of scope and NOT flagged as missing.

## Verdict: PASS WITH WARNINGS — ready to commit/PR as PR1

## Build / Test Evidence (re-run independently, not trusted from apply-progress)
- `go build ./...` — clean (exit 0).
- `go vet ./...` — clean (exit 0).
- `go test ./... -count=1` — all packages ok: audit, cli, doctor, guard, installer, manifest,
  modelconfig, ui. (exit 0)

## Spec Compliance (WU1 capabilities)
| Requirement | Evidence | Status |
|---|---|---|
| Phase taxonomy (13 real phases) | modelconfig.go:21-42 Phases slice; Defaults() opus=propose/design/verify, haiku=archive/onboard, sonnet=rest | PASS |
| ConfigKey (hyphen→underscore+_model) | modelconfig.go:93-104; jd-judge-a→jd_judge_a_model | PASS |
| Resolve ignores unknown/old phases | modelconfig.go:76-88; test old-key-drop asserts want=Defaults() | PASS |
| Plugin config emission realigned | plugin.json 13 <phase>_model keys match Defaults(); plugins.go generic over Phases | PASS |
| models.json schema_version=2 + IsStale + MigrateIfStale | models.go:15,85-134; backup-then-regen, no override merge | PASS |
| Doctor detects stale (absent=healthy, read-only) | checks.go:180-195 uses IsStale not MigrateIfStale; test asserts no mutation + no .bak | PASS |
| Update regenerates stale config | update.go:38 MigrateIfStale before re-apply | PASS |
| Install regenerates stale config (confirmed follow-up) | install.go:56 MigrateIfStale; test verifies verbatim .bak + post-migration non-stale | PASS |

## Strict TDD Compliance
Real RED→GREEN pairs confirmed. Tests assert genuine behavior, not superficial:
- IsStale: legacy-flat / lower-schema / current / absent cases.
- MigrateIfStale: verbatim .bak backup, full regen, override NOT carried forward, no-op on
  fresh/current.
- Doctor stale check: read-only invariant asserted (raw content unchanged + no .bak created).
- Resolve: old 5-phase keys dropped (want=Defaults()), no-mutation/no-leak across calls.
- install/update migration wired at cobra-command level with real fixtures.

## Regression Check
No weakened/silently-passing old-taxonomy tests. All old-key references are either (a) intentional
staleness-detection keys (models.go oldTaxonomyPhaseKeys) or (b) legitimate negative fixtures
asserting old keys are dropped/migrated. plugins_test.go agents/click-*.md paths are agent-file
presence checks (WU3 concern), not model-phase taxonomy — untouched, not a regression.

## Issues
### CRITICAL
- None.

### WARNING
- W1: `gofmt -l .` is non-empty. 6 files THIS PR edits in place still carry CRLF and appear in
  gofmt -l: internal/cli/install.go, internal/cli/update.go, internal/doctor/checks.go,
  internal/doctor/checks_test.go, internal/cli/commands_test.go, internal/installer/installer_test.go.
  A gofmt/CI gate could reject the PR. VERIFIED as pre-existing repo-wide CRLF (untouched files
  config.go, uninstall.go, plugins.go, hooksettings.go also appear), and all touched files are
  content-clean after CRLF strip (zero real format defects). Newly-rewritten files (modelconfig.go,
  models.go, modelselect.go + their tests) are LF-clean and absent from gofmt -l. Recommend
  normalizing line endings on the 6 touched files (or add .gitattributes `*.go text eol=lf`)
  before merge so the PR does not trip a gofmt gate.

### SUGGESTION
- S1: Separate repo-wide CRLF normalization change + `.gitattributes` to end the recurring gofmt
  noise permanently (deliberately out of scope here to avoid an all-files diff inside a stacked PR).

## Next Recommended
sdd-archive (WU1 clean) OR proceed to Work Unit 2 apply (interactive menu). No CRITICAL blockers.
