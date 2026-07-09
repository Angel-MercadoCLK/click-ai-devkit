> Status: draft v0.1. Grounded in `00-decisions-and-open-questions.md` (D1–D20).

# click-ai-devkit — Recommendations

> Audience: decision-makers and engineers closing out v0.1 planning. This document does not
> re-litigate locked decisions or re-derive scope — it reads `00-decisions-and-open-questions.md`,
> `implementation-plan.md`, `tech-spec.md`, `mvp-scope.md`, and `adoption-plan.md` as ground truth
> and gives a prioritized, actionable take on top of them.

## 1. Executive recommendation

Build v0.1 as planned. The overall shape — thin Go CLI, markdown-first SDD flow reused from
existing tooling, bundled/pinned Engram, canary-gated rollout — is sound and already reflects
deliberate trade-offs (D1–D20) rather than untested assumptions. The one component that can sink
the whole effort is **`memory-guard`**: if its pattern set has a false negative, real insurance
data (policy numbers, claims data, customer identifiers) reaches persistent memory at a regulated
insurer, and if it over-blocks, the canary group will route around the tool entirely and the pilot
metric becomes meaningless. Everything else in this project — the CLI's four subcommands, the
three plugins' content, the release pipeline — is comparatively low-risk: it is mechanical,
reversible (`click uninstall`), and failing at it costs time, not trust or compliance. Treat
`memory-guard` as the one component that earns extra review time, extra sign-off, and the earliest
possible build slot (`implementation-plan.md` already sequences it this way — see §4 below); do not
let schedule pressure compress its red-team hardening.

## 2. Decisions awaiting sign-off

These are marked `Provisional` or `Confirmed (user can veto)` in `00-decisions-and-open-questions.md`
— i.e., accepted while the user was away, not yet explicitly confirmed. This is the single most
actionable section in this document: work through it as a checklist.

| # | Decision | Why it was proposed | Risk of leaving it as-is |
|---|---|---|---|
| D8 | Engram **bundled at latest, pinned per click release**; `click update` moves the pin. | Gives reproducibility (same click release ⇒ same Engram version on every machine, NFR-003/004) without floating dependencies. | Low — this is a sound default and the veto flag reads as a formality, but it does bind Click to whatever Engram's release cadence is; confirm the team is comfortable being a downstream consumer with no fork/vendor fallback if upstream stalls or breaks compatibility (tracked as a standing risk, `requirements.md` §5). |
| D11 | **Team-wide rollout gated by a 3–5 day canary** (2–3 devs), hard 100% red-team gate before opening to the team. | Balances "ship something everyone benefits from" against "don't expose a regulated insurer's data to an unvalidated safety control." | Low-medium — the canary window (3–5 days) is short; if it's too short to surface real false negatives, the team-wide rollout inherits an unvalidated guard. The go/no-go gate can extend the canary (`adoption-plan.md` §4), but only if someone is watching for that signal rather than defaulting to "the clock ran out, ship it." |
| D12 | SDD default mode: **Interactive** (pauses for developer confirmation at each phase handoff), not automatic. | Matches an insurer's determinism/safety posture and D10's "explain itself" persona; visibility matters more than speed during the canary. | Low — reasonable default, explicit override path proposed (tech-spec §9 OI-1). Main risk is dev friction if experienced users find the pauses tedious; mitigated by planning to revisit the default post-canary. |
| D13 | **Strict-TDD on by default** for `click-sdd-code`, opt-out for spikes. | AI-generated code without test-first discipline is a known higher-defect-risk path; aligns with the Strict TDD Mode already enabled in this environment. | Low-medium — could slow first-commit time enough to frustrate early adopters if the opt-out isn't discoverable; confirm the opt-out flag is documented prominently in the "getting started" material (`adoption-plan.md` §6), not buried in a skill file. |
| D14 | Guard pattern set: **category structure only** (PII, policy numbers, claim IDs, amounts, customer identifiers) — concrete regex authored later. | Separates "what categories must the guard cover" (a product/compliance decision, safe to lock now) from "what exact regex catches them" (an engineering task requiring red-team validation). | **Medium-high** — this is a structure decision, not a false sense that the guard is "done." The real risk lives in the *unwritten* regex (tracked as OI-3 in tech-spec §9), not in this decision itself. Confirm who signs off that the five categories are actually exhaustive for Click's data before regex authoring starts. |
| D15 | `allowed-memory.md`/`forbidden-memory.md`/`SECURITY.md` will **mirror the D14 categories exactly**. | Keeps the human-facing policy layer and the enforced guard from drifting apart. | Low — sound as a structural rule, but only holds if Slice 4 (docs) genuinely waits for Slice 2 (guard) to be final, as the implementation plan requires. Confirm this dependency is respected under schedule pressure, not treated as parallelizable. |
| D16 | **Drop `.claude-plugin/marketplace.json`** for v0.1; CLI uses its own embedded `manifest.yaml`. | Two install paths (`/plugin marketplace add` vs. `click install`) risk leaving a machine in a state `click doctor` doesn't recognize, undermining NFR-003. | Low as a technical call, but currently **inconsistently reflected in the docs** (see §8 below) — `architecture.md` §4 and `mvp-scope.md` §2 still list the file. Confirm the decision, then fix the docs so a new contributor doesn't scaffold the file back in by following architecture.md literally. |
| D17 | **Scoop only for v0.1**; brew is a fast-follow via GoReleaser; PowerShell deferred indefinitely. | Scoop already covers the committed Windows fleet (D3); brew's incremental cost is near-zero once GoReleaser exists, so it's cheap to add later rather than now. | Low — reasonable sequencing. Only matters if any pilot participant is on Mac/Linux day one; confirm the canary group is Windows-only before treating this as fully low-risk. |
| D18 | (Superseded by D23 for v0.1) Originally proposed: dedicated `Angel-MercadoCLK/scoop-bucket` repo, auto-published by CI on tag. | Kept the manifest update mechanical (GoReleaser's `scoop:` block) rather than a manual step prone to drift from the actual release. | Low — standard pattern, but superseded: D23 now publishes into a `bucket/` folder inside `click-ai-devkit` itself, using the default `GITHUB_TOKEN` (no separate deploy token to scope or store). |
| D19 | Guard latency budget: **<50ms p95** added latency. | Keeps the guard imperceptible in an interactive session; dominated by process spawn cost, not regex matching. | Low — reasonable, but note `implementation-plan.md` §Slice 2 flags that the *hook invocation* path (process spawn), not just the in-process engine, must be benchmarked against this budget. Confirm the 50ms figure was chosen with spawn overhead in mind, not just algorithmic cost. |
| D20 | Guard audit log: **local JSON, content hashes only**, no telemetry. | Supports red-team verification and false-positive/negative triage without creating a second place sensitive data could leak (a log storing raw payloads would defeat the guard's own purpose). | Low — sound design. Confirm log rotation/retention is actually specified before canary (currently described but not bounded — "rotated" with no size/age policy stated). |

**Recommended action:** treat D14 as the highest-priority item to explicitly confirm — not because the
decision itself is risky, but because it's the one where a rubber-stamp "sure, fine" without
verifying category exhaustiveness would silently propagate into the regex authoring, the policy
docs (D15), and the red-team fixtures, all of which inherit whatever gap exists here.

## 3. Top risks, ranked

| Rank | Risk | Likelihood / Impact | Recommended action | Already tracked in |
|---|---|---|---|---|
| 1 | **memory-guard gap** (false negative lets real PII/claims data reach Engram) **or over-blocking** (guard is unusable, canary devs route around it) | Medium likelihood / **Critical** impact — this is the one failure mode with regulatory and trust consequences, not just rework cost | Budget real security-adjacent review time for regex authoring (not just engineering time); keep the 100% red-team gate a hard gate with no "mostly passes" exception; run the guard in observe+block mode during the canary so blocking behavior is visible before it's trusted | `implementation-plan.md` Slice 2 DoD/risks; `mvp-scope.md` §5; `adoption-plan.md` §4; tech-spec §9 OI-3 |
| 2 | **PreToolUse mutation/redact capability is unconfirmed** — if Claude Code doesn't support payload mutation, "redact" silently can't work as designed | Medium likelihood / High impact — resolved late, this reshapes `redact.go`'s design after it's already written | Resolve as the **first task** of Slice 2, before any redact code is written (already planned this way); if unsupported, redact degrades to block-with-reason everywhere — document this explicitly rather than leaving it implicit | tech-spec §4.1; `implementation-plan.md` Slice 2 first task; decisions doc §3 |
| 3 | **Go CLI maintenance ownership** — the team accepted a Go dependency (D5) for install-UX control | Medium likelihood / Medium-high impact (long-term, not launch-blocking) — degrades over months if no one owns it, not immediately | Name an explicit CLI maintainer (or rotation) now, not after the first bug report; keep CLI scope deliberately thin (install/update/doctor/uninstall only, NFR-007) so the maintenance surface stays small | `requirements.md` §5 assumptions; `mvp-scope.md` §5; D5 trade-off |
| 4 | **Pilot metric is a subjective self-report** (minutes of re-explanation avoided) | High likelihood of noisy/inconsistent data / Medium impact — doesn't block safety, but weakens the pilot's own conclusions | Keep the log under a minute to fill in (already planned); pair with the qualitative secondary signals (red-team result, `click doctor` health, guard false-positive rate) so the go/no-go decision doesn't lean on self-report alone | `vision.md` Success metric; `adoption-plan.md` §5; `prd.md` §6 secondary signals |
| 5 | **Upstream Engram drift** — bundled/pinned dependency could change packaging or break compatibility | Low-medium likelihood (mitigated by pinning) / Medium impact if it happens | Resolve Engram's actual packaging mechanism (plugin vs. standalone MCP server) definitively by Slice 6, not left as a placeholder into the release; keep the pin-bump as its own reviewable PR (`ENGRAM_VERSION` file) rather than a silent build-time query | tech-spec §5; `implementation-plan.md` Slice 6 risks; `requirements.md` §5 assumptions |

## 4. What to do first (the critical path)

```
Phase 0: repo bootstrap
   └─▶ Slice 1: tracer-bullet install (one stub plugin, full install/doctor/uninstall loop)
         └─▶ Slice 2: memory-guard safety core + red-team harness  ← built SECOND, not last
               └─▶ Slices 3–5: real plugin content (click-sdd, click-memory, click-review)
                     └─▶ Slice 6: release pipeline (GoReleaser, scoop bucket, Engram pin resolution)
```

**Why the tracer bullet comes first:** the install/uninstall/doctor mechanic (filesystem writes,
CLAUDE.md marker patching, manifest embedding) is the riskiest *mechanical* unknown. Proving it with
one throwaway stub plugin is far cheaper than discovering a design flaw after three plugins' worth
of real content already depends on it.

**Why the guard must not be last:** `memory-guard` carries the only hard compliance gate in this
project — 100% red-team pass, no exceptions, before any developer outside the canary group touches
the tool. That gate has zero tolerance for late discovery of design problems (the redact/mutation
caveat, regex false negatives). Building it right after the tracer bullet — instead of bolting it on
at the end, the way a "nice-to-have security feature" often gets treated — maximizes the runway to
iterate on false positives and false negatives *before* the canary clock starts running. A guard
built last is a guard that gets rushed straight into a compliance-critical rollout.

## 5. Team & skills needed

| Role | Responsibility | Why it matters |
|---|---|---|
| **Go maintainer(s)** | Own `cmd/click/` and its `internal/` packages long-term. | D5's accepted trade-off — Go was chosen to match gentle-ai's proven scoop-distribution pattern, but it's a real, ongoing skill requirement, not a one-time build cost. Confirm this isn't implicitly "whoever wrote it," which stalls between releases if that person moves on. |
| **Security-adjacent engineer** | Author the concrete D14 regex/pattern set; review it against real (synthetic) Click data shapes before the canary. | This is genuinely new work with no existing reference (`implementation-plan.md` Slice 2 risks call this the single highest-severity risk in the whole project). Engineering time alone is not sufficient here — this needs someone thinking adversarially about what a policy number or claim ID actually looks like in Click's systems. |
| **Canary owner** | Runs the canary end to end: coordinates 2–3 devs, collects self-report logs, runs/verifies the red-team test, presents the go/no-go recommendation. | Explicitly defined in `adoption-plan.md` §3 — this role is load-bearing for the go/no-go gate, not a coordination nicety. |
| **Supporting engineer(s)** | Help construct red-team test cases; triage `memory-guard` false positives/negatives during and after the canary. | Same source (`adoption-plan.md` §3) — the pattern set is explicitly expected to be iterative, not a one-time authoring task; someone needs to own that loop past launch. |
| **Engineering leadership** | Approves/denies the go/no-go decision; owns the call to extend the canary if the red-team test fails. | `adoption-plan.md` §3, §4 — the gate says "do not relax to mostly passes"; that only holds if leadership is prepared to actually say no or extend. |

## 6. Making the metric trustworthy

The primary metric (minutes of re-explanation avoided per session) is self-reported by design — no
telemetry in v0.1 (`vision.md` Non-goals). That's an accepted trade-off, not a flaw to fix, but a
few practical habits keep it from becoming noise:

- **Keep the log under a minute to fill in.** `adoption-plan.md` §5 already specifies this — protect
  it; any friction here means canary devs skip the log entirely, and you get zero signal instead of
  noisy signal.
- **Capture a baseline before install.** The current plan starts measurement on day one of the
  canary (post-install) — there's no documented pre-install baseline. Without one, "15 minutes
  avoided" has no anchor; consider asking canary devs to estimate a typical pre-tool re-explanation
  cost once, before their first session, purely as a comparison point.
- **Watch for gaming.** A self-report metric that determines whether a pilot "succeeds" creates a
  soft incentive to round numbers up, especially from participants who already like the tool. Treat
  self-report trends as directional, not a precise measurement, and cross-check outliers against the
  session notes field.
- **Pair it with the qualitative secondary signals** already defined in `prd.md` §6: red-team test
  result, `click doctor` health across canary participants, guard false-positive rate, and
  team-wide adoption after go/no-go. The go/no-go gate (`adoption-plan.md` §4) is explicitly *not*
  built on the primary metric alone — the canary's actual purpose is safety validation, with the
  minutes-avoided number understood to be "inconclusive at this early stage" (adoption-plan.md §4)
  by design. Don't let the primary metric silently become the de facto gate criterion in practice.

## 7. Post-v0.1 roadmap (brief)

| Fast-follow | Trigger / timing | Source |
|---|---|---|
| **Brew tap** | Once the GoReleaser pipeline exists (Slice 6) — near-zero incremental cost via the `brews:` config block; target landing around team-wide rollout, not blocking the canary. | tech-spec §6.2, §9 OI-6; `implementation-plan.md` Slice 6 |
| **Revisit PowerShell installer** | Only if a concrete need surfaces — scoop already covers the committed Windows fleet; currently deferred indefinitely, not scheduled. | D17; tech-spec §9 OI-6 |
| **Revisit a marketplace path** | Only if a second install path is explicitly wanted later (e.g., before a brew tap exists for non-Windows devs) — tech-spec's Option C for the `.claude-plugin/marketplace.json` question, explicitly out of scope for v0.1. | tech-spec §9 OI-5 (Option C); D16 |
| **Guard pattern-set expansion** | Ongoing, iterative — not a one-time authoring task. False-positive/negative triage continues past team-wide rollout via `patterns.local.yaml` overrides, folded back into subsequent releases. | `adoption-plan.md` §7; tech-spec §4.3 |
| **Automatic SDD mode** | Consider only after the canary demonstrates interactive mode is trusted and the visibility trade-off is no longer needed by default. | tech-spec §9 OI-1 |

## 8. Documentation hygiene

Propagation debt — **RESOLVED** in the post-external-review reconciliation pass:

- ✅ **D24 (reverse D16 — ship `.claude-plugin/marketplace.json` and install via the native `claude plugin` CLI)** is now the active plan.
  repo-structure diagram and from `mvp-scope.md` §2 deliverables; reframed as a v0.2-optional path
  in references/requirements/prd.
- ✅ **Stale `D1–D11` headers** updated to `D1–D20` across all planning docs (including `tech-spec.md`).
- ✅ **Scoop command** corrected from the Homebrew-style `scoop install click-seguros/tap/click` to
  the proper two-step `scoop bucket add <bucket-name> <bucket-url>` + `scoop install click` in all six docs.

These were staleness/propagation gaps, not conflicting decisions — now closed so a new contributor
scaffolding the repo from `architecture.md`/`mvp-scope.md` won't reintroduce the dropped file.

No genuine contradiction was found across the source documents during this review — the items above
are staleness/propagation gaps, already flagged in `implementation-plan.md`, not conflicting
decisions.
