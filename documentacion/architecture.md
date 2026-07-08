# click-ai-devkit — Architecture

> Status: draft v0.1. Grounded in `00-decisions-and-open-questions.md` (D1–D20).
> Audience: engineers building or maintaining click-ai-devkit itself.

## 1. High-level architecture

`click-ai-devkit` has four moving parts. Three are installed *into* a developer's Claude Code
setup as **plugins** (markdown: agents + skills). One is a small **Go binary** that puts them
there and keeps them current. A bundled **Engram** instance provides the persistent-memory backend
all of it writes to.

```
                    ┌─────────────────────────────────────────┐
                    │              click (Go CLI)              │
                    │   install · update · doctor · uninstall  │
                    └───────────────────┬───────────────────────┘
                                        │ installs / wires
                                        ▼
        ┌─────────────────────────────────────────────────────────────┐
        │                        ~/.claude                             │
        │                                                               │
        │  CLAUDE.md rules   Engram MCP config   plugins/               │
        │                                          ├─ click-sdd/        │
        │                                          ├─ click-memory/     │
        │                                          └─ click-review/     │
        └───────────────────────────┬───────────────────────────────────┘
                                     │ used at runtime by
                                     ▼
                    ┌─────────────────────────────────┐
                    │           Claude Code             │
                    │  ClickOrchestrator + agents/skills │
                    └───────────────┬────────────────────┘
                                     │ mem_save / mem_search (MCP)
                                     ▼
                    ┌─────────────────────────────────┐
                    │     memory-guard PreToolUse hook  │
                    │   deterministic scan: allow /     │
                    │   block / redact                  │
                    └───────────────┬────────────────────┘
                                     │ only allowed content passes
                                     ▼
                    ┌─────────────────────────────────┐
                    │      Engram (bundled, pinned)     │
                    │   persistent memory across         │
                    │   sessions and compactions         │
                    └─────────────────────────────────┘
```

### Components

| Component | What it is | Decision |
|---|---|---|
| **Go CLI (`click`)** | Single static binary. Thin installer/manager — not the orchestration brain. | D3, D5 |
| **`plugins/click-sdd/`** | Orchestrator + SDD agents/skills (explore → prd → design → tasks → code → review). Rebranded from existing SDD machinery. | D9 |
| **`plugins/click-memory/`** | Memory policy docs + the memory-guard PreToolUse hook + memory-related skills. | D6, D7 |
| **`plugins/click-review/`** | PR/code review agent and skills, adapted from `guardian-angel`. | D9 |
| **Engram** | Persistent-memory MCP server. Bundled, not reimplemented, pinned per click-ai-devkit release. | D1, D8 |
| **memory-guard hook** | Deterministic Claude Code PreToolUse hook. Scans every `mem_save` call before it reaches Engram. | D7 |

**Why a CLI at all, given the SDD flow is "just markdown"?** Because *getting* that markdown (and
Engram, and the hook, and the CLAUDE.md rules) onto every developer's machine, identically, is
itself a problem worth solving with tooling — that's D3/D5. The CLI's job ends at install/update/
doctor/uninstall; it does not participate in the SDD flow at runtime.

## 2. Install flow, end to end

```
1. scoop bucket add click https://github.com/Angel-MercadoCLK/scoop-bucket && scoop install click        (D3, D5, D18 — scoop primary)
        │
        ▼
2. click install
        │
        ├─ copies plugins/click-sdd, click-memory, click-review → ~/.claude/plugins/
        ├─ writes/updates CLAUDE.md rules (Click conventions, orchestrator activation)
        ├─ configures the Engram MCP server entry (pinned version, per D8)
        └─ registers the memory-guard PreToolUse hook in Claude Code settings
        │
        ▼
3. click doctor                                        (verifies steps above landed correctly)
        │
        ▼
4. Developer opens Claude Code → ClickOrchestrator is active,
   click-sdd-* skills available, Engram memory online, memory-guard enforcing.
```

`click update` re-runs the sync step against the version pinned in the current click-ai-devkit
release (including the pinned Engram version — D8). `click uninstall` reverses step 2.

**What the CLI does *not* do:** it does not run the SDD flow, does not call Claude, does not touch
git or PRs. All of that happens inside Claude Code via the installed markdown agents/skills, per
the "thin installer" guardrail agreed in the decisions doc.

## 3. The SDD flow

```
User Request
   │
   ▼
ClickOrchestrator ───────────────────────────────────────────────────┐
   │                                                                   │ explains each
   ▼                                                                   │ step in plain
click-sdd-explore   → investigate codebase, compare approaches        │ language (D10)
   ▼                                                                   │
click-sdd-prd       → what/why, requirements                          │
   ▼                                                                   │
click-sdd-design    → technical approach, architecture decisions      │
   ▼                                                                   │
click-sdd-tasks     → ordered, actionable task breakdown              │
   ▼                                                                   │
click-sdd-code      → implementation                                  │
   ▼                                                                   │
click-sdd-review    → pre-PR / pre-merge check                        │
   ▼                                                                   │
click-memory-curator → decides what, if anything, is worth persisting │
   ▼                                                                   │
Engram (via memory-guard) ────────────────────────────────────────────┘
```

This is a **rebrand and adaptation**, not a rewrite: the phase machinery is reused from existing
SDD tooling (D9). Click's own value-add concentrates in two places: the **memory layer**
(guard + curator, so nothing sensitive is ever persisted) and **review** (Click-specific
pre-merge checks). The orchestrator persona is Click's own — professional, plain-spoken, replies
to developers in Spanish, produces artifacts in English (D10).

### Reference → Click mapping

| Reference (do not copy persona/branding) | Click component |
|---|---|
| `gentle-orchestrator` | `ClickOrchestrator` |
| `sdd-explore` / `sdd-design` / `sdd-code` / `sdd-review` | `click-sdd-explore` / `click-sdd-design` / `click-sdd-code` / `click-sdd-review` |
| *(new phase, not in reference)* | `click-sdd-prd` |
| `agent-teams` subagents | Click agents (`click-orchestrator`, `click-prd-writer`, `click-architect`, `click-reviewer`, `click-memory-curator`) |
| `Gentleman-Skills` | Click skills (`SKILL.md` structure, reused) |
| `engram` | Click memory layer — bundled dependency, not reimplemented |
| `guardian-angel` | `click-pr-review` / `click-reviewer` |
| Claude Code marketplace/CLI distribution | click-ai-devkit distribution: Go CLI (`click`) + Click scoop bucket |

## 4. Repo structure

```
click-ai-devkit/
├── README.md
├── CLAUDE.md
├── SECURITY.md
├── cmd/
│   └── click/                     # Go CLI entrypoint (install/update/doctor/uninstall)
│
├── plugins/
│   ├── click-sdd/
│   │   ├── agents/
│   │   │   ├── click-orchestrator.md
│   │   │   ├── click-prd-writer.md
│   │   │   ├── click-architect.md
│   │   │   ├── click-reviewer.md
│   │   │   └── click-memory-curator.md
│   │   └── skills/
│   │       ├── sdd-explore/SKILL.md
│   │       ├── sdd-prd/SKILL.md
│   │       ├── sdd-design/SKILL.md
│   │       ├── sdd-tasks/SKILL.md
│   │       ├── sdd-code/SKILL.md
│   │       └── sdd-review/SKILL.md
│   │
│   ├── click-memory/
│   │   ├── skills/
│   │   │   ├── memory-proposal/SKILL.md
│   │   │   └── memory-review/SKILL.md
│   │   ├── hooks/                 # memory-guard PreToolUse hook (D7)
│   │   └── docs/
│   │       ├── memory-policy.md
│   │       ├── allowed-memory.md
│   │       ├── forbidden-memory.md
│   │       └── engram-setup.md
│   │
│   └── click-review/
│       ├── agents/
│       │   └── click-pr-reviewer.md
│       └── skills/
│           ├── pr-review/SKILL.md
│           └── pre-merge-checklist/SKILL.md
│
└── docs/
    ├── vision.md
    ├── architecture.md
    ├── mvp-scope.md
    ├── sdd-workflow.md
    ├── adoption-plan.md
    └── references.md
```

> No `.claude-plugin/marketplace.json` in v0.1 — the CLI uses its embedded `manifest.yaml` (D16); a native Marketplace path is a possible v0.2 option.

`cmd/click/` is the one addition to the brief's original structure — the Go CLI needs a home, and
`cmd/<binary-name>/` is the idiomatic Go layout. The scoop bucket itself (manifest + release
artifacts) is a separate repo in the Click org, published by CI on release — it is *distribution*
plumbing, not part of click-ai-devkit's own source tree.

> Note: this document lives in `documentacion/` alongside the decisions log during the planning
> phase; the `docs/` path above is where these files land once the repo is scaffolded (per the
> brief's target structure).

## 5. Security & memory enforcement architecture

Two layers, by design (D6, D7):

```
        Developer / agent calls mem_save(content)
                        │
                        ▼
        ┌───────────────────────────────────┐
        │   Layer 1 — human-facing policy     │
        │   memory-policy.md, allowed-memory  │
        │   .md, forbidden-memory.md          │
        │   (guides *what the model attempts* │
        │    to save)                         │
        └───────────────┬─────────────────────┘
                        │
                        ▼
        ┌───────────────────────────────────┐
        │   Layer 2 — deterministic guard     │
        │   memory-guard PreToolUse hook       │
        │   pattern-matches every mem_save     │
        │   call, independent of the model     │
        │        │              │              │
        │     allow          block/redact      │
        └────────┬──────────────┬──────────────┘
                 │              │
                 ▼              ▼
             Engram        rejected / stripped,
                            never reaches Engram
```

**Posture:** deny-by-default / allowlist (D6). Only technical knowledge is persisted —
architecture/design decisions, conventions, patterns, gotchas, bugfixes. **Always forbidden:**
PII, policy numbers, claims (siniestros) data, amounts, customer identifiers.

**Why two layers and not just the markdown policy?** A markdown policy only constrains a model
that reads and follows it faithfully — it is not a control, it is a suggestion. The PreToolUse
hook runs deterministically in the harness, outside the model's discretion, and inspects every
`mem_save` payload before it can reach Engram. The markdown policy remains valuable as the
human-facing explanation of *why* — but it is not the enforcement mechanism (D7).

**Validation gate before wide rollout:** the hook must pass a 100% red-team PII test during the
hardening canary before the team-wide rollout proceeds (D11).

## 6. Rollout shape

Per D11: team-wide rollout, gated by a short hardening canary.

```
Canary (3–5 days, 2–3 devs)
   memory-guard in observe+block mode
   red-team PII test → must pass 100%
        │
        ▼ (pass)
Team-wide rollout, same sprint
        │
        ▼
Re-explanation minutes tracked via before/after self-report, from day one of the canary
```

If the red-team test does not pass 100%, rollout does not proceed to the whole team — the guard's
pattern set gets fixed and re-tested first.

## 7. Open questions / to settle

These are explicitly **not blocking v0.1** per the decisions doc, but are unresolved as of this
writing:

- **SDD flow defaults.** Interactive vs. automatic mode as the default, and whether strict-TDD
  mode is on by default for `click-sdd-code`.
- **Memory-guard pattern set.** The concrete PII/insurance regex/pattern list the hook matches
  against (policy number formats, claim ID formats, amount patterns, etc.) is not yet authored.
- **`allowed-memory.md` / `forbidden-memory.md` / `SECURITY.md`.** Referenced in the repo
  structure above; content not yet written.
- **Brew/PowerShell installers.** D5 marks brew as "optional later" — no committed timeline for
  when (or whether, for v0.1) it ships alongside the scoop bucket.
- **Scoop bucket repo ownership.** Confirmed to be a Click-hosted scoop bucket (D3), but its exact
  repo location/CI release process relative to click-ai-devkit itself is not detailed in the
  decisions doc.

See `00-decisions-and-open-questions.md` §3 for the authoritative, evolving queue.
