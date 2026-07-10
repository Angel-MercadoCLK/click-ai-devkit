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

---

# Verify Report — Work Unit 2 (PR2 of 3): standing `internal/menu` + `root.go` default action

> Persistence note: Engram MCP tools were not available in this verify sub-agent's tool set (same
> as the WU1/WU2 apply runs). This WU2 section is appended to the file per the launch-prompt
> fallback; the WU1 section above is left intact. Reconcile into Engram topic
> `sdd/interactive-menu-and-model-taxonomy/verify-report` when MCP is restored.

## Scope verified
PR2 / Work Unit 2 ONLY — uncommitted working-tree changes on branch
`feat/model-config-taxonomy-realignment`, on top of PR1. New files: `internal/menu/menu.go`,
`internal/menu/menu_test.go`, `internal/cli/rootdefault.go`, `internal/cli/rootdefault_test.go`,
`internal/cli/configuremodels.go`. Modified: `internal/cli/root.go`, `internal/cli/commands_test.go`.
Work Unit 3 (skill/agent content authoring, taxonomy-lockstep test) is a separate future PR and its
absence is NOT treated as a failure.

## Build / static / test evidence (run in this verify, not trusted from apply report)
- `go build ./...` — clean.
- `go vet ./...` — clean.
- `go test ./... -count=1` — all 9 test packages `ok` (audit, cli, doctor, guard, installer,
  manifest, menu[new], modelconfig, ui). Verbose run confirms all 12 `internal/menu` tests and all
  new `internal/cli` rootdefault/dispatch/configure-models tests PASS.
- `gofmt -l` on the 7 WU2-touched files — empty (clean). WU2 files are LF-clean and do NOT inherit
  the repo-wide CRLF noise flagged for WU1 (W1 above).

## interactive-menu capability — spec compliance matrix
| Requirement | Status | Evidence |
|---|---|---|
| Default Action TTY Gate — non-TTY prints help, exit 0, NEVER hangs | PASS | `interactive()` returns false when out/in are not real `*os.File` terminals, `--no-interactive`, or `CI` set. `TestRootCommand_NoArgs_NonTTY_PrintsHelpAndExitsCleanly` forces the non-TTY branch via `execRoot`'s `bytes.Buffer` out + empty-buffer in (genuinely non-tautological — asserts err==nil AND "Usage:" text). `TestInteractive_FileOutButNotATerminal_ForcesFalse` uses a real `os.Pipe()` `*os.File` to prove even a file-but-not-terminal resolves false. Independent real-binary run: bare `click` with piped stdout → exit 0, prints help, no hang. |
| Menu Navigation (j/k + arrows + wrap) | PASS | `moveCursor` modular-wraps; `TestModel_Update_JKMoveCursorAndWrap` + `TestModel_Update_ArrowKeysAlsoMoveCursor` cover j/k, arrows, and wraparound both directions. |
| Active Item Dispatch (install/doctor/update/uninstall unchanged) | PASS (with UX WARNING W2) | Enter on active item sets `Chosen`+`tea.Quit`; caller maps via `ActionArgs` and runs a FRESH `NewRootCommand()` tree (`dispatch()`), so subcommand behavior is unchanged. `TestDispatch_RunsFreshCommandTreeAgainstProvidedStreams` proves streams are forwarded. See W2: it dispatches ONCE then the process exits — it does NOT loop back to the menu. |
| Placeholder Items Are Inert (no side effect, no dispatch, no quit) | PASS | `selectCurrent` on `!Active` sets transient `StatusMsg` only, returns nil cmd. `TestModel_Update_EnterOnInertItemShowsStatusAndDoesNotQuit` asserts Chosen stays empty AND cmd is nil. |
| Menu Exit (q / esc / ctrl+c → exit 0) | PASS | All three set `Chosen=ActionQuit`+`tea.Quit`; `ActionArgs(ActionQuit)`=nil → `runRootDefault` returns nil (exit 0). Covered by `TestModel_Update_QRuneQuits` + `TestModel_Update_EscAndCtrlCQuit` + `TestActionArgs_QuitAndUnknownReturnNil`. |

## Targeted claim verification
- Non-TTY no-hang: VERIFIED genuine. Tests force the false branch via `bytes.Buffer` (non-`*os.File`)
  and `os.Pipe()` (`*os.File` but not a terminal). The real `tea.NewProgram().Run()` branch is only
  reached when `interactive()` is true, which no test can hit without a PTY — acceptable and
  documented; the safety-critical false branch is exhaustively covered. Real-binary run confirms
  exit 0 + help, no hang.
- "Restored unknown-subcommand rejection": VERIFIED. `runRootDefault` guards `len(args)>0` and
  returns `unknown command %q for %q`. `TestRootCommand_UnknownSubcommand_ReturnsError` passes AND
  real-binary `click totally-bogus-typo` → `Error: unknown command "totally-bogus-typo" for "click"`,
  exit status 1. Loud failure preserved despite root now having a RunE. `TestRootCommand_ExplicitSubcommands_StillDispatch`
  confirms real subcommands (`doctor`) still route to their own RunE, not the menu.
- Updates-indicator placeholder: VERIFIED honestly labeled. `headerVersion` = static
  `"click-ai-devkit (updates: not checked)"` with an explicit doc comment stating real update
  checking is out of scope and the line only reserves structural position. Not misleadingly
  presented as real. Compliant with the disclosed simplification.
- configure-models hidden-subcommand design: REASONABLE, no public-surface leak. `Hidden: true` is
  set (absent from `click --help`) yet still directly runnable for scripts. TTY-guarded via
  `isTerminalWriter` so it can never spin a bubbletea program against non-terminal output when
  invoked directly. Reuses `runModelSelectTUI` + `installer.SaveModels` (not rebuilt). Correct.

## Issues
### CRITICAL
- None.

### WARNING
- W2 (behavior/UX vs. proposal wording): The menu dispatches exactly ONE action then the process
  exits — it does NOT return to the menu afterward (Enter on active item → `tea.Quit` → dispatch
  once → return). The proposal calls this a "standing" menu at "gentle-ai parity", where a standing
  menu typically loops back after each action. This select-and-exit behavior is documented as
  intentional in apply-progress (Update→tea.Quit→dispatch-once pattern, kept pure/testable), so it
  is a DECIDED design, not a silent defect — but it diverges from the "standing"/parity wording.
  Recommend the user explicitly confirm select-and-exit is the intended PR2 UX (vs. a follow-up that
  re-opens the menu after a dispatched action returns). Not a blocker.
- W3 (UI language consistency): The menu mixes languages. Item labels, header, and coming-soon
  message are English ("Start installation", "Upgrade tools", "coming soon — not implemented yet"),
  but the footer help line is Spanish ("j/k mover · enter seleccionar · q/esc salir"). The repo's
  established runtime-UI convention is Spanish (see install.go/update.go `r.Info(...)` strings).
  `configuremodels.go`'s Spanish runtime messages + English cobra `Short` correctly match that
  convention, but `menu.go` is internally inconsistent (English labels + Spanish footer). Pick one
  language for the menu UI for polish. Not a blocker.

### SUGGESTION
- S2: The real-TTY branch of `runRootDefault` (`tea.NewProgram(...).Run()`) has no automated
  coverage (unavoidable without a PTY). Add a manual smoke-test checklist entry (bare `click` in a
  real terminal → navigate → dispatch → observe) to the PR description so the one unexercised path
  is human-verified before merge.

## Verdict
PASS WITH WARNINGS. All safety-critical requirements (non-TTY no-hang, unknown-subcommand
rejection, inert placeholders, clean exit) pass with genuine, non-tautological tests plus
independent real-binary confirmation. Build/vet/test/gofmt all clean on WU2 files. Two non-blocking
WARNINGs (W2 select-and-exit UX confirmation, W3 menu language mix) and one SUGGESTION. Ready to
commit/PR as PR2; W2 warrants a quick user confirmation but does not block.

## Next Recommended
sdd-archive is NOT yet appropriate (WU3 still pending as a separate PR). For PR2: proceed to
commit/open PR2 after (optionally) confirming W2's select-and-exit UX intent. No CRITICAL blockers.

---

# Verify Report — Work Unit 2 follow-up (loop-back + i18n), same PR2 slice

> Persistence note: Engram MCP tools (`mem_*`) were not exposed in this verify sub-agent's tool set.
> Appended to this file per the fallback contract; all prior sections (WU1, WU2) left intact.
> Reconcile into Engram topic `sdd/interactive-menu-and-model-taxonomy/verify-report` when MCP is restored.

## Scope verified
The two WU2 verify WARNINGs (W2 loop-back UX, W3 menu language mix), fixed in the follow-up apply batch
on top of the already-verified WU2. Uncommitted working-tree changes on branch
`feat/model-config-taxonomy-realignment`. Touched: `internal/cli/rootdefault.go`,
`internal/cli/rootdefault_test.go`, `internal/menu/menu.go`, `internal/menu/menu_test.go`. Verified
independently against source, not trusted from apply-progress.

## Verdict: PASS — W2 and W3 both confirmed RESOLVED. Ready to commit/PR as PR2.

## Build / static / test evidence (re-run in this verify)
- `go build ./...` — clean (exit 0).
- `go vet ./...` — clean (exit 0).
- `go test ./... -count=1` — all 9 packages `ok` (audit, cli, doctor, guard, installer, manifest, menu,
  modelconfig, ui). exit 0.
- `gofmt -l` on the 4 follow-up files — empty (clean).
- Verbose focus run: `TestRunMenuLoop_*` (5/5), `TestRootCommand_NoArgs_NonTTY_PrintsHelpAndExitsCleanly`,
  `TestRootCommand_UnknownSubcommand_ReturnsError`, `TestModel_View_InertItemsShowComingSoonSuffix` — all PASS.

## W2 (loop-back) — RESOLVED
- `runMenuLoop` (rootdefault.go:56-70) is a genuine `for {}` loop: launch menu → if `ActionArgs(chosen)`
  is non-empty, dispatch, then re-iterate (re-launch menu); exits ONLY on launchMenu error, empty
  actionArgs (Quit/unknown → nil), or dispatch error. It does NOT return after a single dispatch.
- Wired for real in `runRootDefault` (rootdefault.go:47) via closures over `tea.NewProgram(...).Run()`
  and `dispatch()`. The WU2 fresh-command-tree dispatch pattern is unchanged; only control-flow now loops.
- Tests are genuine, not tautological: `TestRunMenuLoop_DispatchesThenLoopsBackToMenuUntilQuit` injects a
  fake `launchMenu` returning [install, doctor, quit] via an advancing index and asserts `launchCalls==3`
  with exactly 2 dispatches ([install],[doctor]) — proving the loop re-invokes the menu after each
  dispatched action. Error/quit/empty short-circuits each have their own assertion.

## W3 (Spanish i18n) — RESOLVED
- All user-visible menu text in menu.go is Spanish: item labels (Iniciar instalación / Actualizar
  herramientas / Configurar modelos / Ejecutar diagnóstico / Desinstalar / Salir), inert placeholders,
  header `headerVersion` ("click-ai-devkit (actualizaciones: sin verificar)"), View inert suffix
  "(próximamente)", `comingSoonMsg` ("próximamente — todavía no implementado"), footer
  ("j/k mover · enter seleccionar · q/esc salir"). No English fragments remain in user-visible strings.
- Code identifiers (ActionInstall, headerVersion, etc.) and doc comments stay English per repo convention.

## Regression check — NO regressions
- Non-TTY no-hang: `interactive()` gate unchanged; `TestRootCommand_NoArgs_NonTTY_PrintsHelpAndExitsCleanly`
  + all `TestInteractive_*`/`TestIsTerminal*` still pass. Bare `click` piped still exits 0 with help.
- Unknown-subcommand rejection: `TestRootCommand_UnknownSubcommand_ReturnsError` still passes;
  `runRootDefault`'s `len(args)>0` guard untouched.

## Issues
### CRITICAL — None.
### WARNING
- W1 (carried from WU1, unchanged): repo-wide pre-existing CRLF makes `gofmt -l .` non-empty on files
  this change does NOT touch. The 4 follow-up files are LF-clean and absent from gofmt -l. Not introduced
  here; recommend `.gitattributes *.go text eol=lf` before merge to avoid a CI gofmt gate trip.
### SUGGESTION
- S2 (carried): the real-TTY branch of `runRootDefault` (`tea.NewProgram(...).Run()`) and the live loop-
  back through a real bubbletea program have no automated coverage (needs a PTY). Add a manual smoke-test
  to the PR description: bare `click` in a real terminal → dispatch an action → confirm it returns to the
  menu → quit.

## Next Recommended
Proceed to commit/open PR2. `sdd-archive` is NOT yet appropriate for the whole change (Work Unit 3 —
skill/agent content + taxonomy-lockstep test — remains a separate future PR). No CRITICAL blockers.

---

## Work Unit 3 (PR3 of 3) — skill/agent content + taxonomy-lockstep test

**Verdict: PASS. Ready to commit/open PR3.** Independent verification (sdd-verify, opus). Engram MCP
unavailable this run; persisted to this file.

### Inventory (verified from filesystem, not trusted from the report)
- Skills: 13 dirs = 12 phase dirs (`explore, propose, spec, design, tasks, apply, verify, archive,
  onboard, jd-judge-a, jd-judge-b, jd-fix-agent`) + `agent-builder` (untouched meta-skill). 6 `sdd-*`
  dirs deleted, 6 renamed + 6 net-new. `default` intentionally has no skill dir.
- Agents: 5 files, all modified. Matches the apply report's corrected inventory (7 skills / 5 agents),
  NOT the stale launch assumption (5 skills / 2 agents). Self-correction confirmed accurate.

### Build / test / format evidence
- `go build ./...` clean; `go vet ./...` clean.
- `go test ./... -count=1` — all 9 packages `ok`, no regressions.
- Lockstep tests genuinely PASS (verbose-confirmed, not skipped/vacuous):
  `TestClickSDDSkills_LockstepWithModelconfigPhases` iterates `modelconfig.Phases`, skips only
  `default` via an explicit `phasesWithoutDedicatedSkill` map, and `os.ReadFile`s each `<phase>/SKILL.md`
  — renaming `apply` back to `sdd-code` would make ReadFile fail → `t.Errorf`. Inverse orphan guard
  and plugin.json key-lockstep also real (both-direction set comparison). Not tautological.
- `gofmt -l` clean on both touched Go files.

### Content quality (spot-checked)
- Frontmatter matches repo convention (`name` + `description`) across all new/rewritten files;
  voice consistent with untouched `agent-builder`. Zero TODO/lorem/placeholder (grep-confirmed).
- `jd-judge-a`/`jd-judge-b`: correct near-mirror blind-pair files; accurately describe the real
  convergence protocol (no cross-reading, converge before confirm, hand BLOCKER/CRITICAL to fix agent).
- `jd-fix-agent`: correctly scoped to converged findings only, minimal changes, 2-round budget.
- `apply`: description byte-identical to old `sdd-code` — clean rename, strict-TDD content correct.
- `sdd-prd → propose` (not spec): NO content gap. `propose` = problem/scope/requirements/high-level
  acceptance criteria + open questions; `spec` (net-new) = full verifiable acceptance-criteria
  scenarios read downstream by `tasks`/`verify`. Clean separation, explicit handoff, no overlap.

### click-orchestrator.md bug fix — VERIFIED REAL
Old `HEAD` "Model routing" referenced 5 dead keys (`orchestrator_model, prd_writer_model,
architect_model, reviewer_model, memory_curator_model`) removed by PR1. Current version lists exactly
the 13 real `<phase>_model` keys, matching `plugin.json` userConfig 1:1 and `modelconfig.Phases`.
Zero dead keys remain anywhere under `plugins/click-sdd/` (grep-confirmed). Correct handling of
`click-memory-curator` (not a phase → routes on `archive_model`).

### Findings
- CRITICAL: none.
- WARNING: none.
- SUGGESTION (S1): lockstep test guards phase↔skill-dir↔plugin.json, but agent `.md` taxonomy
  references are unguarded by any test (inherent — agents aren't phases). Future taxonomy drift in
  agent files would not be caught automatically. Non-blocking.

### Next Recommended
Commit/open PR3. Once the 3-PR chain lands, the whole change is ready for `sdd-archive`.

---

## Fix round 1 — 3 CONFIRMED findings from adversarial review, applied across the stacked chain

Applied by a single writer, TDD RED→GREEN, one fix per owning branch, each propagated forward via
merge before the next fix (PR1 → PR2 → PR3). All three merges were clean, no conflicts.

### R3-001 (CRITICAL, PR1 `feat/model-config-taxonomy-realignment`) — install migrated before cancel check
**Bug**: `internal/cli/install.go` called `installer.MigrateIfStale(cfg)` unconditionally before the
interactive model-select TUI's cancel check, so cancelling the TUI still backed up and regenerated a
stale `models.json` — cancel ≠ no changes.

**Fix**: extracted the migrate+select sequence into `resolveInstallModels(cmd, out, r, cfg,
nonInteractive, selector modelSelector)` (`internal/cli/install.go`). Migration now only runs once the
install is known to proceed (non-interactive, or interactive-confirmed); an interactive cancel returns
before `MigrateIfStale` is ever called, leaving disk untouched. `nonInteractive` is computed once by
the caller and passed in, so tests can force the interactive branch without needing a real TTY.

**Test evidence**: new `TestInstallCommand_InteractiveCancel_LeavesModelsUntouched`
(`internal/cli/commands_test.go`) — seeds a stale legacy `models.json`, injects a fake selector
returning `cancelled=true`, asserts no `.bak` created and `models.json` bytes byte-for-byte unchanged.
RED confirmed (compile error, `resolveInstallModels` undefined) before implementation, GREEN after.
Pre-existing `TestInstallCommand_MigratesStaleModelsBeforeOverwriting` and
`TestInstallCommand_FreshInstall_NoBackupCreated` still pass — non-interactive migration behavior
unchanged. Commit `d40da32`.

### R4-001 (CRITICAL, PR2 `feat/interactive-menu`) — menu-dispatched failures double-printed
**Bug**: `dispatch()` (`internal/cli/rootdefault.go`) runs a fresh `NewRootCommand().Execute()` per
menu action; on failure the inner root auto-printed "Error: ..." (+ usage, unless the subcommand
self-silenced like `doctor`), then the error propagated up through `runMenuLoop` →
`runRootDefault`'s RunE → the outer live root's own `Execute()`, which printed a second redundant
"Error: ..." line plus a dump of root's own irrelevant usage block.

**Fix**: `root.SilenceUsage = true` set globally in `NewRootCommand()` (`internal/cli/root.go`) —
usage dumps belong to genuine flag-parse errors, not runtime failures, and this applies uniformly to
both the outer live root and every fresh root `dispatch()` builds. For the double-error-print,
`dispatch()` now wraps any inner `Execute()` failure in a new sentinel `*errMenuDispatchFailed`
(`internal/cli/rootdefault.go`); `runRootDefault` calls a new `silenceIfAlreadyReported(cmd, err)`
helper that sets `cmd.SilenceErrors = true` only when the error is `errors.As`-detected as
already-reported, so the outer root's own `ExecuteC` skips its redundant print. Non-dispatch errors
(e.g. a `launchMenu`/bubbletea failure that nobody has shown yet) are left unsilenced and still get
cobra's normal single auto-print.

**Test evidence** (`internal/cli/rootdefault_test.go`, all RED→GREEN):
- `TestRootCommand_DirectSubcommandFailure_PrintsErrorExactlyOnceNoUsage` — `click install
  --this-flag-does-not-exist` direct invocation: exactly one `"Error:"` line, no `"Usage:"` dump.
- `TestDispatch_SubcommandFailure_PrintsErrorExactlyOnceNoUsageDump` — same assertion via `dispatch()`
  directly.
- `TestSilenceIfAlreadyReported_DispatchError_SilencesRoot` /
  `TestSilenceIfAlreadyReported_OtherError_LeavesRootUnsilenced` — unit coverage of the silencing
  decision in isolation.
- `TestErrMenuDispatchFailed_UnwrapsToOriginalError` — `errors.Is`/`Unwrap` contract.
- Pre-existing `TestRootCommand_UnknownSubcommand_ReturnsError` and
  `TestRootCommand_ExplicitSubcommands_StillDispatch` still pass unmodified — unknown-command
  visibility and direct-invocation exit codes are untouched. Commit `7deb235`.

### R2-001 (CRITICAL, PR3 `feat/skill-content-taxonomy`) — click-prd-writer.md self-contradictory
**Bug**: `plugins/click-sdd/agents/click-prd-writer.md`'s frontmatter description, `# Role`, and
`## Responsibilities` all said "PRD", while `## Phase mapping` said the agent owns the `propose`
phase whose skill writes "the proposal artifact" — no sentence reconciled the two terms, and "prd" is
not a phase in the 13-phase taxonomy.

**Fix**: terminology-only, behavioral instructions unchanged (output expectations already matched
`propose/SKILL.md` almost verbatim). Added one explicit reconciling statement in `# Role` ("The PRD is
this plugin's name for the proposal artifact that phase produces") and reworded `# Role`/frontmatter
description/`## Responsibilities` to reference the `propose` phase and "proposal" instead of bare
"PRD". Also fixed the related WARNING R2-002 (same one-line terminology cleanup, same PR):
`click-orchestrator.md:50` "specialist agents write the PRD, design, tasks..." → "...write the
proposal, design, tasks...".

**Test evidence**: no Go tests apply to `.md` content. Ran `go test ./internal/installer/ -run
Lockstep -v` — `TestClickSDDSkills_LockstepWithModelconfigPhases` PASS, confirming nothing broke.
Full `go build ./...`, `go vet ./...`, `go test ./... -count=1` also clean. Commit `c464b8f`.

### Verification (on final tip branch `feat/skill-content-taxonomy`, after all 3 merges + fixes)
- `go build ./...` — clean.
- `go vet ./...` — clean.
- `go test ./... -count=1` — all 9 packages `ok` (audit, cli, doctor, guard, installer, manifest,
  menu, modelconfig, ui).
- `gofmt -l` on every touched file — clean.
- All three branches pushed to `origin`: `feat/model-config-taxonomy-realignment` (`1acda9c..d40da32`),
  `feat/interactive-menu` (`4386c8f..7deb235`), `feat/skill-content-taxonomy` (`4435e4a..c464b8f`).
  No merge conflicts at any propagation step (PR1→PR2, PR2→PR3).

### Next Recommended
`sdd-verify` (re-verify the fix round), then `sdd-archive` once confirmed.

---

## Fix round 2 (FINAL) — RR1-001 (CRITICAL) — global SilenceUsage over-suppressed genuine flag-parse errors

**Bug**: round 1's fix for R4-001 set `root.SilenceUsage = true` globally in `NewRootCommand()`
(`internal/cli/root.go`). Correct effect: no usage dump on RUNTIME failures. Regression: cobra's own
post-execute usage print (`ExecuteC` in `command.go`) gates solely on `SilenceUsage`, with no
visibility into *why* `execute()` failed — so genuine flag-parse/usage errors (unknown flag, bad flag
value) also lost their `Usage:` block, a real UX regression from pre-fix cobra. The round-1 regression
test `TestRootCommand_DirectSubcommandFailure_PrintsErrorExactlyOnceNoUsage` used an unknown-flag
scenario (`install --this-flag-does-not-exist`) and asserted `!strings.Contains(out, "Usage:")`,
locking in the wrong behavior.

**Fix**: added `root.SetFlagErrorFunc(...)` in `internal/cli/root.go`. Verified against the vendored
cobra v1.10.2 source (`command.go`): `FlagErrorFunc` runs precisely when `pflag` fails to parse
(`execute()`: `err = c.ParseFlags(a); if err != nil { return c.FlagErrorFunc()(c, err) }`), before RunE
ever executes, and is inherited by every child command via `cmd.parent.FlagErrorFunc()` walk-up — so
it covers every subcommand, not just the bare root. The func explicitly writes `cmd.UsageString()`
then returns the original error unchanged, so cobra's normal single `"Error: ..."` line still prints
via its usual `ExecuteC` path (gated only by `SilenceErrors`, untouched). `root.SilenceUsage = true`
is kept for the generic post-execute path, so RunE runtime failures (and menu-dispatched failures via
`dispatch()`'s inner fresh root) still get zero usage dump, unchanged from round 1.

**Test evidence** (`internal/cli/rootdefault_test.go`, all RED→GREEN — confirmed by reverting the
`root.go` fix and re-running: all three new/updated Usage-asserting tests failed as expected before
the fix, passed after):
1. Flag-parse/usage errors → Usage shown: `TestRootCommand_UnknownFlag_PrintsErrorAndUsage` (renamed
   from the regressed `TestRootCommand_DirectSubcommandFailure_PrintsErrorExactlyOnceNoUsage`,
   scenario unchanged — `install --this-flag-does-not-exist` — assertion flipped to expect `Usage:`)
   and new `TestRootCommand_BadFlagValue_PrintsErrorAndUsage` (`--no-color=not-a-bool`, a bad bool
   flag value). Both assert exactly one `"Error:"` line AND a `Usage:` block present.
2. Runtime failures of a subcommand → no usage, reported once:
   `TestRootCommand_DirectSubcommandRuntimeFailure_PrintsErrorExactlyOnceNoUsage` (`doctor` on an
   empty home). Doctor self-silences (`cmd.SilenceErrors = true`, pre-existing, out of scope) and
   reports via its own Fail lines rather than cobra's literal `"Error:"`, so this test asserts the
   doctor unhealthy report is present exactly once and no `Usage:` leaks through — the two things this
   fix actually governs.
3. Menu-dispatched failures → still exactly one legible error, no usage dump:
   `TestDispatch_SubcommandRuntimeFailure_PrintsErrorExactlyOnceNoUsageDump` — retargeted from the
   regressed unknown-flag scenario to a genuine runtime failure (`dispatch(parent, ["doctor"])` on an
   empty home), preserving the original test's real intent (no usage dump on a runtime failure routed
   through `dispatch()`). New `TestDispatch_FlagError_PrintsUsage` locks in the fixed, symmetric
   behavior for the flag-error case via `dispatch()` (unknown flag → `Usage:` present), confirming
   `FlagErrorFunc` is inherited by the fresh root `dispatch()` builds. `errMenuDispatchFailed` +
   `silenceIfAlreadyReported` machinery untouched; `TestSilenceIfAlreadyReported_*` and
   `TestErrMenuDispatchFailed_UnwrapsToOriginalError` still pass unmodified.
4. Unknown COMMAND (`click bogus-typo`) → error visible, non-zero exit: pre-existing
   `TestRootCommand_UnknownSubcommand_ReturnsError` passes unmodified — this path never touches
   `FlagErrorFunc` (root's own RunE returns a manual error after successful flag parsing), and
   `root.SilenceUsage` behavior for it is unchanged from round 1.

**Manual binary verification** (built `cmd/click`, real process, not `go test`):
- `click install --this-flag-does-not-exist` → prints full `Usage:` block (flags list) followed by
  `Error: unknown flag: --this-flag-does-not-exist`, exit 1.
- `click doctor` on an empty `CLICK_CLAUDE_HOME` → prints only doctor's own `[FAIL]`/`[OK]` report
  lines and the final `[FAIL] click-ai-devkit no está instalado correctamente`, NO `Usage:` block,
  exit 1.

**Verification on final tip (`feat/skill-content-taxonomy`, after merge)**:
- `go build ./...` — clean.
- `go vet ./...` — clean.
- `go test ./... -count=1` — all 9 packages `ok` (audit, cli, doctor, guard, installer, manifest,
  menu, modelconfig, ui).
- `gofmt -l` on both touched files (`internal/cli/root.go`, `internal/cli/rootdefault_test.go`) —
  clean.
- Fixed on `feat/interactive-menu` (commit `0bfa31e`), merged forward into `feat/skill-content-taxonomy`
  (merge commit, clean, no conflicts). Both branches pushed to `origin`:
  `feat/interactive-menu` (`7deb235..0bfa31e`), `feat/skill-content-taxonomy` (`c464b8f..524fd7d`).

### Next Recommended
`sdd-verify` (scoped re-review of this fix diff against the review ledger), then `sdd-archive` once
confirmed — this is the final adversarial-review round per the fix-round-2 instructions.
