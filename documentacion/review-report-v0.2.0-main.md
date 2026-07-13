# Review Report — main @ v0.2.0 (post-merge full audit)

Full adversarial review (4 lenses: risk, reliability, resilience, readability — model Fable) of the accumulated diff `2935e60..0d0c4a7` (~1,793 lines: the orchestration-profiles reconciliation PRs #5–#8, the release workflow `--skip=validate` change, and the manifest version bump). Three consolidated CRITICAL candidates went through independent 3-refuter adversarial verification (correctness / impact / reproducibility lenses, 2-of-3 vote); all three were CONFIRMED unanimously (9 votes of 9, zero refuted). Ledger persisted in Engram at topic `review/main-post-merge/ledger`. Status of every finding below: **open — fix required** for CRITICALs, **info — non-blocking** for the rest.

---

## Section 1: CRITICAL — changes required (3)

### C1 — `configure-models` erases the persisted profile label

- **Location:** `internal/cli/configuremodels.go:55`
- **Defect:** calls `installer.SaveModels(cfg, selection)` which equals `SaveModelsWithProfile(cfg, "", models)` — silently drops the persisted `profile` field from models.json.
- **Consequence chain** (all hops code-verified): install with cost-saver persists `"profile":"cost-saver"` → one configure-models run erases it → `click doctor` misreports "Perfil de orquestación: balanced" (doctor.go maps "" → balanced) → `click update` re-emits `--config orchestration_profile=balanced` alongside the non-balanced map and re-persists the erasure — sticky corruption across every future update.
- **Violates invariant:** the codebase's own documented invariant (profiles.go:108–110: "the persisted `profile` field must never claim a preset name the actual per-phase map no longer matches"). `EffectiveProfileName` exists exactly for this and is not called on this writer.
- **Aggravating:** `TestUpdateCommand_ReappliesPersistedModels` (commands_test.go:703–754) PINS the broken outcome as expected — the test must be corrected too or it will fight the fix.
- **Fix:** configure-models must load the current profile, apply `EffectiveProfileName(loadedProfile, selection)`, and persist via `SaveModelsWithProfile`; fix the pinning test.

### C2 — `--profile` flag silently ignored in interactive install, contradicting its own help text

- **Location:** `internal/cli/install.go:41` (help text), `install.go:171–195` (branch logic), `internal/ui/profileselect.go:48–50`.
- **Defect:** help promises "en la interactiva sólo precarga el editor por fase inicial", but `profileFlagValue` is only read on the non-interactive branch; the interactive TUI always constructs `ui.NewProfileSelectModel()` with no argument, cursor hardcoded to balanced. `click install --profile quality` on a real terminal silently ignores the flag.
- **Impact:** plausible wrong-outcome path — a dev pasting `click install --profile cost-saver` from onboarding docs, then Enter-through the picker (whose footer says "sin cambios = balanced"), silently installs balanced: real per-phase model and cost difference, not just a label.
- **Fix** (either, pick one and make help text truthful): (a) implement the promised preloading — pass the flag value into `NewProfileSelectModel(initial ProfileName)` / seed the picker cursor; or (b) change the help text to state the flag only applies to non-interactive installs. Option (a) matches the documented contract. Add a test for the interactive+flag combination (currently untested).

### C3 — AGENTS.md language rule contradicts locked decision D10 and the shipped Spanish UI

- **Location:** `AGENTS.md:21–24`.
- **Defect:** "All artifacts and code comments are in English: … and any string literal in source" contradicts (a) locked decision D10 (documentacion/00-decisions-and-open-questions.md:45 — "Replies to devs in Spanish; artifacts in English"), (b) the shipped Spanish UI strings this same release added (doctor.go "Perfil de orquestación", install.go --profile help, all of profileselect.go — which itself carries a comment citing D10), and (c) AGENTS.md's own instruction "do not contradict a locked decision" (lines 32–33).
- **Impact:** AGENTS.md is agent-consumed. The next planned contributor is Codex implementing agent-builder-flow (a TUI made largely of user-facing strings), and the agent-builder-flow SDD artifacts contain zero language guidance — AGENTS.md's wrong rule would be the only language instruction it receives, steering it to write/rewrite UI strings in English and erode the D10 convention.
- **Fix:** amend AGENTS.md's Language section to carve out dev-facing CLI/TUI string literals as Spanish per D10 (code, comments, docs, commits remain English).

---

## Section 2: WARNINGS — recommended, non-blocking (5)

- **R1-001** — `internal/installer/profile_artifacts.go:96–112`: `RenderMarkdownAgent` doesn't reject/escape newlines in Description/Model/Tools → YAML frontmatter injection (including an attacker-chosen `tools:` line = tool-permission escalation) IF ever fed free text. Unwired today (no caller); MUST be hardened before agent-builder-flow consumes it. Fix: reject or escape internal newlines in all frontmatter fields.

- **R3-002** — `internal/cli/update.go:47–66`, `doctor.go:100–112`: no label↔map self-heal at the load boundary; a hand-edited models.json keeps re-emitting a stale/garbage profile label forever. Fix: apply `EffectiveProfileName` on load in update/doctor.

- **R4-001** — `internal/cli/install.go:66–124`: persist-last ordering — if any step between plugin sync and `SaveModelsWithProfile` fails (e.g. network during Engram sync), the next `click update` hits file-absent fallback and silently downgrades the applied profile to balanced. Fix: persist models.json before (or immediately after) the plugin sync.

- **R4-003** — `.github/workflows/release.yml:24–74`: the manifest patch step has no success assertion — a silent no-op (renamed key, reformatted YAML) would ship a release with a stale embedded manifest version, and `--skip=validate` removed the only correlated signal. Fix: make the python script raise unless both substitutions occurred exactly once.

- **R2-003 + R2-004** (doc drift, group them): `plugins.go:104–115` and `models.go:34–36` comments describe a pre-PR3 caller state that no longer exists (and mask the C1 leftover); `click-orchestrator.md:61–65` still calls the orchestration-profile machinery "a later slice / forward reference only" when it shipped in this release — could mislead an agent into deriving models from the profile name instead of the 13 explicit `<phase>_model` keys. Fix: refresh both comment blocks and the .md note.

---

## Section 3: SUGGESTIONS (2)

- **R2-006** — `AGENTS.md:38–43`: garbled sentence in the Plugins section; the "four items that stay in sync by convention" cannot be reconstructed (five candidates listed). Rewrite the sentence.

- **R2-007** — `profiles.go:85–94` + `install.go:176–181`: `ResolveProfile`'s unknown→balanced fallback is silent for raw CLI input — a typo'd `--profile cost_saver` silently installs balanced. Add a one-line warning on unrecognized values.

---

## Section 4: Adversarially cleared (explicit no-finding — do not re-litigate without new evidence)

- `--skip=validate` supply-chain concerns: checkout pins the tag's exact SHA; tag-push right already equaled release right before the change; no tag-name injection path (env var, not template interpolation).
- `profile_artifacts.go` path traversal: name regex excludes traversal/absolute/drive-letter forms.
- Profile name → `--config` flag smuggling: single argv token via exec arg-slice, no shell.
- update.go on corrupt models.json: fails loudly before any sync; balanced fallback only fires when the file is absent.
- Release patch step crash: fails the job before goreleaser runs (bash -e).
- bubbletea panic/terminal restore: v1.3.10 defaults cover it; cancel paths write nothing.
- Preset tables match Defaults(); internal profile-name constants are drift-proof; Spanish voseo consistent with existing UI.

---

Review executed with Fable across 4 lenses + 3 independent refuters; report generated with Haiku; ledger: Engram `review/main-post-merge/ledger` (obs #1187). Date: 2026-07-11.
