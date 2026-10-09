# click-ai-devkit — Vision

> **Status: Historical baseline.** This draft captures the original v0.1 product framing and is
> retained as a decision record. Its Claude Code-only boundary and other v0.1 scope statements are
> not the current runtime support contract. For current behavior, see [`README.md`](../README.md),
> [`codex-target.md`](codex-target.md), and [`portability-runbook.md`](portability-runbook.md).

## Current direction (2026-10-08)

Per [D48](00-decisions-and-open-questions.md), `click-ai-devkit` is a Click Seguros distribution
layered on qualified gentle-ai **v4.0.0 stable**, not a fork. `click` provisions the exact upstream
release artifact with pinned SHA-256 verification; Scoop installs only `click`, and upstream
self-update is suppressed. gentle-ai owns ODD, RDD, Engram lifecycle, and runtime adapters.
Click owns its rules, skills, memory guard and insurance data policy, extra permissions,
installer/doctor/rollback, and model profiles mapped to upstream roles. The migration retires
`click-sdd`; `click-review` keeps business/insurance checklists, not a separate review engine.
Only Engram Cloud retention under D40–D47 remains an open product decision.

## Historical v0.1 baseline

The conclusions below were valid for the original v0.1 scope. They are not a prohibition on the
current supported target slices: Claude Code remains the native plugin workflow, OpenClaw has a
portable SDD workflow plus documented native model CLI configuration, and Codex receives guidance
through `AGENTS.md` only. Other runtimes are not supported. Plugins are a read-only inventory and
staging surface, not arbitrary activation.

## The problem

Every developer at Click Seguros who uses Claude Code today configures it alone: their own
prompts, their own memory habits (if any), their own idea of what "good AI-assisted work" looks
like. Three consequences follow:

1. **Context re-explanation.** Every new session, every new chat, every new developer starts from
   zero. The same architecture decisions, conventions, and gotchas get re-explained to the AI over
   and over, by every person, every day.
2. **No shared discipline.** There is no common flow for how an AI-assisted change goes from idea
   to shipped code — some devs plan first, some don't, some review AI output carefully, some
   don't.
3. **No safety net for sensitive data.** Insurance work involves policy numbers, claims
   (siniestros) data, amounts, and customer identifiers. Without a control, nothing stops that
   data from being pasted into an AI memory store or a prompt log.

`click-ai-devkit` is the shared answer to all three: one installable system that gives every
developer the same orchestrator, the same planning flow, and the same guardrails — instead of each
person reinventing (or skipping) them.

## Who it's for

- **Primary user:** Click Seguros software developers using Claude Code day to day.
- **Secondary beneficiary:** engineering leadership, who need confidence that AI-assisted work is
  consistent, reviewable, and does not leak sensitive data.

This is an **internal tool for internal teams** — not a product for external customers, not a
general-purpose open-source release (see Non-goals).

## Why now

Claude Code adoption inside Click Seguros is organic and ungoverned today. Every additional week
without a shared standard means more divergent habits to unlearn later, and more exposure to the
data-leak risk described above. `click-ai-devkit` packages a known-good pattern (the `gentle-ai`
family of tools: orchestrator, SDD flow, memory via Engram) and adapts it to Click's needs —
so the team does not have to design this from scratch, and does not have to wait for a perfect
in-house solution before getting the safety and consistency benefits.

## Value proposition

For a Click Seguros developer, `click-ai-devkit` means:

- **Less re-explaining.** The orchestrator and Engram-backed memory carry architecture decisions,
  conventions, and past gotchas across sessions — so you don't restate them every time.
- **One clear way to work with AI.** A single, repeatable flow (explore → PRD → design → tasks →
  code → review) instead of ad hoc prompting.
- **A safety net you don't have to think about.** Sensitive data (PII, policy/claims data) is
  screened out of memory automatically, by a deterministic control — not by hoping the model
  remembers a policy document.
- **One command to get set up.** Install and stay current via a small CLI, not a manual checklist
  of files to copy into `~/.claude`.

## Success metric

**Primary metric:** minutes of context re-explanation avoided per session.

This is measured via **before/after self-report** from pilot participants, starting on day one of
the canary (see D11). Concretely: after each AI-assisted session, a participant estimates how many
minutes they *would have* spent re-explaining context (architecture, conventions, prior decisions)
without the tool, versus what they actually spent. This is a self-report metric, not an automated
instrumentation metric — v0.1 does not build usage analytics or a dashboard (see Non-goals).

## Principles

| Principle | What it means in practice |
|---|---|
| **Clarity** | The orchestrator explains itself in plain language — no unexplained jargon, no silent magic. A developer should always understand *why* the AI did something. |
| **Repeatable install** | One command reproduces the same setup on any developer's machine. No "works on my machine" AI tooling. |
| **Security** | Sensitive data (PII, insurance-specific data) never reaches persistent memory. Enforcement is deterministic (a hook), not just a policy document the model is expected to honor. |
| **Gradual adoption** | Roll out via a short, low-risk canary before team-wide use — validate the safety control under real usage first, expand once it's proven. |

## Non-goals (v0.1)

- **Not reimplementing Engram.** It is bundled as a dependency, pinned per release — not rebuilt.
  > **Superseded by D48:** gentle-ai owns Engram lifecycle instead of Click bundling and pinning
  > it directly; Click retains the memory guard and insurance data policy.
- **Not installing the full Gentle-AI ecosystem.** Only what Click needs is adapted and rebranded.
  > **Superseded by D48:** adaptation/rebranding is replaced by a pinned upstream distribution;
  > ODD and RDD belong to gentle-ai. See the current direction above.
- **Not a dashboard or analytics product.** Success is measured by self-report, not telemetry.
- **Not multi-tool.** Built for Claude Code only; no support for other AI coding assistants in v0.1.
- **Not production automation.** No auto-merge, no auto-deploy — a human stays in the loop.
- **Not integrating every Click repo on day one.** Starts with a pilot, expands deliberately.
- **Not uncontrolled global memory.** Memory persistence is scoped and policy-gated from the start,
  never "save everything and sort it out later."

## Open questions (not blocking v0.1, tracked for later)

- Final interactive-vs-automatic default for the SDD flow, and whether strict-TDD is on by default.
- The concrete PII/insurance pattern set the memory-guard hook will match against.
- Whether brew/PowerShell installers are built alongside the scoop bucket, or deferred.

See `00-decisions-and-open-questions.md` for the authoritative, evolving list.
