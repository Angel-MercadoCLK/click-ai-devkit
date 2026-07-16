# click-ai-devkit — Hardening Roadmap

Consolidated, prioritized backlog of every real weakness surfaced and **verified against the actual
code** during the deep end-to-end stress-test session (the `engram-mcp-resolution` cycle + the
click-sdd agent audit + real fresh-machine install testing). Nothing here is invented or
speculative — each item cites where the evidence came from. Items explicitly marked as *not a bug*
or *unverified* are labeled so you don't waste effort.

## The one principle behind almost all of this

**Determinism over prose. Enforce, don't hope.** The core quality gap versus a mature toolkit is
that critical rules are written as prose in `.md` files and rely on the model to comply, instead of
being enforced by a mechanism. This repo *already knows how to do this right* — the `memory-guard`
PreToolUse hook and the `PreflightGit` check are exactly the pattern: deterministic, fires
regardless of model intent. The work below is largely about generalizing that discipline from
safety into orchestration.

Second principle: **right-size the ceremony.** A two-line frontmatter fix does not need a 5-PR
chain; a cross-platform PATH mutation does. Match effort to risk.

Third principle: **validate end-to-end, repeatedly.** Most robustness is just plumbing gaps closed
by actually running the flow, finding the break, fixing it, repeating.

---

## Tier 1 — highest leverage, low effort, verified, safe. Do these first.

### T1-1 · Grant Engram MCP tools to click-sdd agents
- **Severity**: High. **Effort**: tiny (frontmatter edits).
- **Evidence**: All five `plugins/click-sdd/agents/*.md` have zero `mcp__plugin_engram_engram__*`
  tools, yet `click-orchestrator.md` (lines 57–60) documents a memory-persistence flow. The flow is
  structurally impossible today. Full detail in `documentacion/click-sdd-agent-fixes-for-codex.md`
  (CONFIRMED-1).
- **Fix**: add the Engram tools to the agent that owns persistence (orchestrator) and read-only
  Engram tools to `click-memory-curator`. Mirror the proven working pattern from the generic
  `sdd-apply`/`sdd-verify` agents.
- **Determinism lens**: this IS the principle — wire the capability into the mechanism instead of
  describing it in prose.

### T1-2 · Add `PreflightClaude` (mirror `PreflightGit`)
- **Severity**: High (UX). **Effort**: small, exact in-repo precedent. **Discipline**: strict TDD.
- **Evidence**: On a machine without `claude` on PATH, `click install` ran the whole interactive
  TUI and then died with a raw Go dump: `exec: "claude": executable file not found in %PATH%`
  (`internal/installer/plugins.go:286`). `PreflightGit` (`internal/installer/gitpreflight.go`,
  wired in `install.go:57`/`update.go:27`) already solves exactly this class for git — there is no
  equivalent for the most fundamental dependency, `claude`.
- **Fix**: a `PreflightClaude()` that `LookPath("claude")` and fails fast with a friendly,
  actionable message before the TUI. Called alongside `PreflightGit`.
- **Note**: this improves the *message/timing*; it does not remove the requirement (you still need
  Claude Code installed — it's a Claude Code devkit).

### T1-3 · Absolute path for the memory-policy reference in the managed CLAUDE.md block
- **Severity**: Low. **Effort**: tiny.
- **Evidence**: the managed CLAUDE.md block says "review `plugins/click-memory/docs/memory-policy.md`"
  — a repo-relative path. The installed file actually lives at
  `~/.claude/plugins/cache/click-ai-devkit/click-memory/<ver>/docs/memory-policy.md`. Ambiguous when
  working outside the repo.
- **Fix**: have `click install`/`click update` resolve and inject the absolute installed path (or
  drop the path and rely on the deterministic guard, since the guard is the real enforcement).

---

## Tier 2 — real value, medium effort. Do after Tier 1.

### T2-1 · Instruct the orchestrator to hand off skill paths into delegations
- **Severity**: Medium. **Effort**: prose addition + ideally a mechanism.
- **Evidence**: `click-orchestrator.md` names which `SKILL.md` backs each phase but never instructs
  itself to pass that path into the `Agent` prompt so the specialist actually `Read`s it. Result:
  phases done "inline" from memory. Full detail in `click-sdd-agent-fixes-for-codex.md` (CONFIRMED-2).
- **Determinism lens**: strongest form is a skill-registry + resolver (how the mature toolkit does
  it), so the path is looked up mechanically, not remembered.

### T2-2 · Enforce per-phase model routing (don't just document it)
- **Severity**: Medium. **Effort**: medium (this is where "prose → mechanism" gets real).
- **Evidence**: `click-orchestrator.md` (lines 68–111) correctly and thoroughly documents reading
  `pluginConfigs` and passing the resolved `model` on each `Agent` call. A full cycle ran everything
  on the session default anyway (opus phases ran sonnet, haiku phases ran sonnet). The instruction
  is fine; compliance failed. Detail in `click-sdd-agent-fixes-for-codex.md` (OBSERVED-3).
- **Fix**: a hard pre-flight gate in the orchestrator prompt is the cheap step; the durable fix is a
  harness-side resolver (`internal/cli` / `internal/modelconfig`) that hands the orchestrator a
  ready phase→model map instead of relying on it to parse `settings.json` itself each session.
- **Before investing deeply**: reproduce 2–3× — don't engineer against a single anecdote.

### T2-3 · Tune the v0.1 memory-guard regexes to cut false positives
- **Severity**: Medium (friction). **Effort**: medium (must not weaken safety).
- **Evidence**: `internal/guard/patterns.yaml` placeholder patterns (claim/policy/customer-id/PII)
  match ordinary English review/security vocabulary and repeatedly blocked legitimate `mem_save`
  calls this session with "blocked: detected claim identifier" — triggered by normal word adjacency,
  not real sensitive data.
- **Fix**: require stronger context cues (labels, delimiters, real formats) before matching; add a
  regression test corpus of "should NOT match" review-vocabulary strings so tuning can't silently
  regress safety.
- **Determinism lens**: keep the guard deterministic and fail-closed — only reduce the false-positive
  surface, never the true-positive coverage.

---

## Tier 3 — preventive / infrastructure. Lower urgency, high compounding value.

### T3-1 · CI guard against Windows drive-letter literals in cross-platform test fixtures
- **Evidence**: this exact bug class hit **three times** in one change (GoBinDir tests ×2 via
  separator mismatch, LivePathContains test via `filepath.SplitList` colon-collision). Invisible on
  the Windows dev host, only caught by Linux CI.
- **Fix**: a lint/CI check flagging `"C:` / backslash path literals in `*_test.go` for non-`windows`
  build-tagged files, or a documented convention to build fixtures via `filepath.Join(...)`.

### T3-2 · Timeout on doctor's `go env` subprocess
- **Evidence**: `checkEngramPath` → `GoBinDir` → `go env` shells out with no `context.WithTimeout`,
  the first doctor check to spawn a subprocess, against the repo's own NFR-012 "doctor never hangs"
  convention (review-resilience WARNING on PR #29).
- **Fix**: bound the `go env` call with a timeout in the shared command runner.

### T3-3 · Document the scoop `git` prerequisite for updates
- **Evidence**: `scoop update click` silently reported the stale version because scoop needs `git`
  to refresh buckets; without it, it reads the stale local manifest and reports "latest" — misleading.
- **Fix**: a short note in install docs / a doctor hint: "if `scoop update` won't see new versions,
  run `scoop install git` first."

---

## Tier 4 — larger / deferred. Deliberate future work, not now.

- **T4-1 · D-9 PATH-ownership tracking + `click uninstall` PATH reversal.** Consciously descoped
  during `engram-mcp-resolution` design review (unimplementable under the committed signal-wiring
  signature). Needs a properly wired `changed`/`targets` signal. Future change.
- **T4-2 · Nested-agent Bash availability.** `click-architect.md` declares `Bash` but a session
  reported it unavailable in a nested `Agent`-in-`Agent` delegation. **Unverified — needs an
  isolated live repro, not a file patch.** Detail in `click-sdd-agent-fixes-for-codex.md`
  (UNVERIFIED-5).
- **T4-3 · Engram binary version bump (1.15.3 → newer).** *Not a bug* — the pin is intentional
  (`ENGRAM_VERSION` + D16, for reproducibility). This is a maintenance decision: bump
  `ENGRAM_VERSION` and cut a release when you want it.

---

## Explicitly NOT to do

- **Do not add a `Skill` tool** to any agent frontmatter. A prior report diagnosed a "missing Skill
  tool"; verified wrong — skills here are plain `Read`-able markdown, no agent in the system uses a
  `Skill` grant. Detail in `click-sdd-agent-fixes-for-codex.md` (CORRECTED-4).

---

## Suggested execution order

1. T1-1, T1-2, T1-3 (a single small, high-leverage batch — verified, low risk).
2. T2-1 + T2-2 together (both are the orchestrator becoming deterministic).
3. T2-3 (guard tuning, with a safety regression corpus).
4. T3-1 → T3-2 → T3-3 (preventive infra, whenever).
5. Revisit Tier 4 deliberately, one at a time, only when each is actually needed.

Attack top-down. Ship and end-to-end-validate each tier before starting the next. That cadence —
not scope — is what turns "far behind" into "hardened."
