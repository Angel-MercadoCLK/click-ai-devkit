# click-ai-devkit — Decisions & Open Questions

> Living document. Tracks what we lock in while shaping the idea, before generating
> PRD / tech-spec / implementation plan. Last updated: 2026-07-09.
> Language policy: all artifacts in English (conversation stays in Spanish).

## 1. Vision summary (from initial brief)

`click-ai-devkit` is an internal, installable Claude Code system that standardizes AI-assisted
development at **Click Seguros**. It ships a custom orchestrator (`ClickOrchestrator`), an
internal SDD flow, specialized agents, internal skills, a memory policy, and Engram integration
as the persistent-memory engine.

**Pilot success metric:** minutes of context re-explanation avoided per session.

**Priorities:** clarity, repeatable install, security, gradual adoption.

### Target SDD flow
```
User Request → ClickOrchestrator → click-sdd-explore → click-sdd-prd → click-sdd-design
  → click-sdd-tasks → click-sdd-code → click-sdd-review → click-memory-curator → Engram
```

### Reference projects (reference only — do NOT copy persona/names/exact conventions)
- `Gentleman-Programming/gentle-ai` — installable ecosystem, SDD, orchestrator, per-phase profiles.
- `Gentleman-Programming/engram` — persistent memory / MCP (dependency, do NOT reimplement).
- `Gentleman-Programming/agent-teams-lite` — SDD with subagents, orchestrator vs specialists.
- `Gentleman-Programming/Gentleman-Skills` — skill structure / `SKILL.md`.
- `Gentleman-Programming/gentleman-guardian-angel` — AI code review, pre-PR checks.
- Claude Code Plugin Marketplace docs — packaging as installable plugin/marketplace.

## 2. Decisions locked

| # | Topic | Decision | Status |
|---|-------|----------|--------|
| D1 | Engram relationship | **Batteries-included**: one install brings Click plugins **and** Engram together (like gentle-ai pulls Engram). Engram is NOT reimplemented; it is referenced/bundled. | Confirmed |
| D2 | Repo hosting | Repo in **Click Seguros GitHub org** (private). | Confirmed |
| D3 | Distribution | **CLI now** (gentle-ai style), distributed via a Click **scoop bucket** (brew/PS installers optional later). The marketplace-only/hybrid option is dropped. | Confirmed |
| D4 | Artifact language | **All artifacts in English** — docs, README, `SKILL.md`, agents, memory-policy. Conversation stays in Spanish. | Confirmed |
| D5 | CLI stack | **Go** — single static binary, no runtime. Primary distribution = Click **scoop bucket**; **brew tap** for Mac/Linux for free. 100% match with gentle-ai. Trade-off: team needs Go to maintain. | Confirmed |
| D6 | Memory policy posture | **Deny-by-default / allowlist**. Persist ONLY technical knowledge (architecture/design decisions, conventions, patterns, tech gotchas/bugfixes) with no real data. FORBID always: PII, policy numbers, claims data, amounts, customer identifiers. | Confirmed |
| D7 | Memory enforcement | **Deterministic guard + policy docs**. A Claude Code **PreToolUse hook** scans every `mem_save` for PII/insurance patterns and **blocks/redacts** before it reaches Engram, independent of the model. Markdown policy = human-facing layer. New component: memory-guard hook (patterns + block/redact + tests). | Confirmed |
| D8 | Engram versioning | **Bundled at latest, pinned per click release**. Install brings Engram automatically; each click-ai-devkit release pins the Engram version latest at release time → reproducible across devs. Updating click updates Engram. ~~**Mechanism (Spike A):** pin the Engram **binary** (release download or `go install ...@tag`) and write click's OWN MCP entry with an **absolute path** — the engram plugin's marketplace entry has no version pin, so relying on it is not reproducible.~~ **Superseded — actual mechanism (Spike E, `spike-e-engram-install.md` Q4, and `engram-mcp-resolution` v0.4.2):** click never writes its own MCP config for Engram (`mcpconfig.go`/`~/.claude/mcp/engram.json` were dead code, confirmed unread by Claude Code, and removed). Claude Code only reads the `engram@engram` plugin's own bundled `.mcp.json`, which uses a bare, PATH-resolved `command: "engram"` — no absolute path anywhere. click's real mechanism is: (a) register the `engram@engram` plugin (`SyncEngramPlugin`), (b) provision the binary via `go install .../engram/cmd/engram@<manifest-pinned version>` when missing (`EnsureEngramBinary`), and (c) persist the resolved Go bin dir onto the user's PATH (Windows registry / POSIX shell rc) so a fresh terminal or Claude Code session can still resolve it, closing the original "works until you restart" bug — see `sdd/engram-mcp-resolution/design` (design obs #1436) and `click doctor`'s `checkEngramPath`. | Superseded (mechanism only; version-pinning intent unchanged) |
| D9 | SDD flow construction | **Reuse/rebrand existing SDD machinery** as `click-sdd-*`; adjust prompts; add Click memory guard + review on top. Not authored from scratch. Click-specific value concentrates in memory (guard + curator) and review. | Confirmed |
| D10 | Orchestrator persona | **Professional, clear, plain-spoken teacher**. Explains so the dev understands; no jargon dumps, no regional slang, no Gentleman persona. Replies to devs in **Spanish**; artifacts in English. Drives `click-orchestrator.md`. | Confirmed |
| D11 | Pilot shape | **Team-wide rollout, gated by a short hardening canary**: 3-5 day canary (2-3 devs, memory guard in observe+block, red-team PII test must pass 100%) → then open to whole team same sprint. Measure re-explanation minutes (before/after self-report) from day one. | Confirmed (signed by user) |
| D12 | SDD default mode | **Interactive** (not automatic) — see each step before continuing. | Confirmed (signed by user) |
| D13 | Strict-TDD | **On by default** for `click-sdd-code`, opt-out for spikes. Aligns with the enabled Strict TDD Mode. | Confirmed (signed by user) |
| D14 | Guard pattern set | Category **structure** (PII, policy numbers, claim IDs, amounts, customer identifiers); concrete regex authored later, validated by red-team harness. | Confirmed (signed by user) |
| D15 | Memory/security docs | `allowed-memory.md` / `forbidden-memory.md` / `SECURITY.md` mirror D14 categories; SECURITY.md outline in tech-spec §7.2. | Confirmed (signed by user) |
| D16 | marketplace.json | **Drop** `.claude-plugin/marketplace.json` for v0.1; CLI uses its own embedded `manifest.yaml` (one per release). **Superseded by D24** after Spike C verified the native Claude Code plugin registry flow. | Superseded |
| D17 | Installers | **Scoop only** for v0.1; brew fast-follow via GoReleaser; PowerShell deferred. | Confirmed (signed by user) |
| D18 | Scoop bucket repo (SUPERSEDED by D23 for v0.1 — no separate repo created; kept here as the original rationale/fallback if this ever needs to split out) | Dedicated `Angel-MercadoCLK/scoop-bucket` repo, auto-published by CI on tag. | Confirmed (signed by user) |
| D19 | Guard latency | Budget **<50ms p95** (sets NFR-006). | Confirmed (signed by user) |
| D20 | Guard audit log | Local JSON, **content hashes only** (never raw payloads), no telemetry (sets NFR-008 mechanism). | Confirmed (signed by user) |
| D21 | memory-guard v0.1 | **Block-only in v0.1; redaction in v0.2.** Spike B CONFIRMED PreToolUse can mutate input (`updatedInput`), so redaction is possible — block-only is now a deliberate policy (max-safe, keeps regex off the v0.1 critical path). v0.2 = redact-when-certain / block-when-uncertain. | Confirmed (signed by user) |
| D22 | Pre-build spikes | **DONE.** Spike A → `spikes/spike-a-engram-packaging.md` (Engram = Go MCP binary + CC plugin; pin the binary). Spike B → `spikes/spike-b-pretooluse-contract.md` (hook CAN redact; fail-closed needs `exit 2`; matcher = plugin-scoped, verify at runtime). | Done |
| D23 | Scoop bucket location | **Same repo, not separate.** The scoop manifest publishes into a `bucket/` folder inside `click-ai-devkit` itself (not a dedicated `scoop-bucket` repo). Fewer repos/secrets for a small pilot. Homebrew still needs its own `homebrew-click` repo later (Homebrew's tap-shortname constraint, not a choice) — D17 unchanged. | Confirmed (signed by user) |
| D24 | Plugin activation path | **Reverse D16** — ship `.claude-plugin/marketplace.json` and have `click install` use the native `claude plugin` CLI to register/install `click-sdd`, `click-memory`, and `click-review`. Loose-folder copying never loaded in Claude Code (Spike C). | Confirmed |
| D25 | Per-phase model routing | **Store + apply.** The user picks a model per SDD phase at install; stored in `click-sdd`'s `userConfig` → `settings.json` `pluginConfigs["click-sdd@click-ai-devkit"].options` (e.g. `orchestrator_model=opus`). The `click-orchestrator` reads that map once per session and passes the resolved alias as the **per-invocation `model`** on every `Agent` delegation (which overrides the agent's frontmatter, per sub-agents.md). Agent frontmatter stays plain — NOT `${user_config}` interpolation (verified fragile, Spike D). Defaults: orchestrator/prd-writer/architect/reviewer = opus, memory-curator = sonnet. | Confirmed (signed by user) |
| D26 | Context7 auto-install | **`click install` also provisions Context7 as a user-scope HTTP MCP via `claude mcp add`, idempotent + respectful, mirroring Engram.** Registered with `claude mcp add --transport http --scope user context7 https://mcp.context7.com/mcp` right after the Engram sync step; presence is probed by reading Claude Code's own user-scope config file directly (`<ClaudeHome>/.claude.json`'s `mcpServers` key) rather than shelling out, so `click doctor` stays subprocess-free; install ownership is decided once and preserved on every later `click install`/`click update` run, exactly mirroring the ownership guard `SyncEngram` uses (D8/Spike E); `click uninstall` only removes it when click's own state says click registered it. Verified end-to-end against the real `claude` CLI (spike-g-context7.md), including click's own compiled binary driving the full install → doctor → uninstall lifecycle against a throwaway `CLAUDE_CONFIG_DIR`. | Confirmed |
| D27 | Install reliability — backup scope | **Backup CLAUDE.md + settings.json only, not PATH.** PATH mutations are already reversible via uninstall ownership tracking (D8/Spike E); duplicate backup would add complexity without new safety value (PATH is not a "working file" edited between runs like CLAUDE.md/settings.json are). Implemented in sdd/install-reliability-foundation. | Confirmed |
| D28 | Install reliability — settings.json snapshot timing | **Single run-start snapshot before ANY step**, including before the external `claude plugin ...` subprocess that also mutates settings.json. Coarse restore point: "last-known-good before this run" intentionally does NOT distinguish click's writes from external CLI writes — accepted, documented limitation (avoids complex filtering logic). Implemented in sdd/install-reliability-foundation. | Confirmed |
| D29 | Install reliability — preview default behavior | **Preview/plan shown by default** for install and update; skipped only with `--yes` or `--non-interactive`. Non-TTY environments (CI) treated as non-interactive. User must actively acknowledge the planned mutations before writes proceed. Implemented in sdd/install-reliability-foundation. | Confirmed |
| D30 | Install reliability — doctor mutation policy | **Doctor stays strictly read-only; never repairs/mutates.** Drift detection is report-only via hash comparison (managed-block body vs. compile-time DefaultManagedContent constant). Repair path = re-running `click install`/`click update`, surfaced as actionable message. Extends existing NFR-012 (accurate reporting) convention. Implemented in sdd/install-reliability-foundation. | Confirmed |
| D31 | Install reliability — rollback on drift response | **Rollback refuses by default when hash-drift detected** (current file content hash != snapshot hash). Requires explicit `--force` confirmation to overwrite since the snapshot is ambiguous: could be hand-edits, external tool mutations, or stale block from older click version. Safe default protects against accidental overwrites. Implemented in sdd/install-reliability-foundation. | Confirmed |
| D32 | OpenClaw install tier | **OpenClaw gets a lighter-touch "Solo-agent" tier install**, not full Claude-Code parity. Detects binary, writes minimal managed files (AGENTS.md/SOUL.md/MCP JSON), includes memory-guard plugin for parity on that single critical path. No per-target opt-out UI, no multi-target wizard, no pluggable registry abstraction (2 targets only, YAGNI). Implemented in sdd/openclaw-target-support. | Confirmed |
| D33 | OpenClaw config shape | **Config widened with flat `OpenClawHome` field + derived-path methods**, not a pluggable target-registry abstraction. Matches existing 2-target reality and current flat-pattern code style. Rationale: YAGNI (no third target planned), matches `ClaudeHome` precedent exactly. Implemented in sdd/openclaw-target-support. | Confirmed |
| D34 | OpenClaw file write safety | **OpenClaw's managed files reuse existing snapshot/preview/rollback protection** (backup-aware, idempotent managed-block for AGENTS.md/SOUL.md, read-merge-write for MCP JSON, per-target file list generalization). Extends install-reliability-foundation's safety class to a second target without new abstractions. Implemented in sdd/openclaw-target-support. | Confirmed |
| D35 | OpenClaw memory-guard parity | **OpenClaw `mem_save` calls guarded by a before_tool_call plugin that shells out to existing `click memory-guard` binary** (never reimplements `internal/guard`'s scanning logic in JS). Fail-closed contract identical to PreToolUse (missing binary, non-zero exit, timeout, invalid JSON all deny). Reuses same audit log automatically; JS adapter handles target-specific tool-name rewrite to canonical matcher. Implemented in sdd/openclaw-target-support. | Confirmed |
| D36 | clickhola/clickdev delivery model | **Content-first + thin aliases, NO new executor logic.** `clickhola`/`clickdev` are OpenClaw `SKILL.md` personas + Claude-Code thin discoverability aliases reusing existing `click-elicitor`/orchestrator Paso 1. Zero new SDD sub-agent or elicitation logic authored. Implemented in sdd/click-hola-click-dev-triggers. | Confirmed |
| D37 | OpenClaw skill bundle location | **go:embed bundled, installed into `~/.openclaw/skills/`** via `internal/installer/assets/openclaw/skills/{clickhola,clickdev}/SKILL.md`. NOT separate dir outside the binary (`plugins/click-openclaw/`); go:embed bundling ensures source-of-truth co-location with the installer, paralleling memory-guard plugin precedent. Refines D35's implicit path. Implemented in sdd/click-hola-click-dev-triggers. | Confirmed |
| D38 | Cross-tool handoff slot | **Deterministic brief token = `sdd/{change-name}/elicitation` (Engram topic key).** Reuses existing slot already read by orchestrator G1; clickhola mints kebab-case change-name (confirmed with Javier), persists brief (B) to this key, clickdev looks it up identically. Zero new convention. Implemented in sdd/click-hola-click-dev-triggers. | Confirmed |
| D39 | OpenClaw skill uninstall policy | **Remove ONLY click-owned SKILL.md from `~/.openclaw/skills/{clickhola,clickdev}/`; preserve sibling user-created files in those dirs.** Least-surprise teardown — if a user adds their own skill variants/extensions to a subdirectory, uninstall does not wipe them. Snapshot/rollback tracks only the click-provided SKILL.md files. Implemented in sdd/click-hola-click-dev-triggers. | Confirmed |
| D40 | Opt-in Engram Cloud enrollment via presence gating | **Engram Cloud enrollment runs only when all three are present: server URL, project name, and non-empty `ENGRAM_CLOUD_TOKEN`.** Absence ⇒ pure local-only Engram (backward compatible no-op; server+project but no token ⇒ Spanish "skipped, token absent" report). Implemented in sdd/engram-cloud-wiring. | Confirmed |
| D41 | Token presence-only safety, never persisted | **Click reads only `ENGRAM_CLOUD_TOKEN` presence (`!= ""`); the value is never stored, never on argv, never written to disk, never logged, never in click-managed state.** It reaches the `engram` subprocess solely via inherited process env. Source + test (`TestSyncEngramCloud_TokenNotInArgvOrState`) verify the exclusions. Implemented in sdd/engram-cloud-wiring. | Confirmed |
| D42 | First-time upgrade sequence mandatory before cloud sync | **Enrolling an existing local Engram into the cloud runs `engram cloud config → enroll → upgrade doctor → upgrade repair → upgrade bootstrap → sync --cloud`, in order, fail-stop (no state written on any failure).** Subsequent already-enrolled runs use the idempotent short path (config + sync only, no re-bootstrap). Implemented in sdd/engram-cloud-wiring. | Confirmed |
| D43 | Doctor 4th check is read-only, network-free | **Cloud enrollment health check (`EngramChecksCount` 3→4) reads only the local `engram-cloud.json` state file and token env presence; it never shells out or touches the network, matching the pure-file-read pattern of checkEngramPlugin/checkContext7.** Implemented in sdd/engram-cloud-wiring. | Confirmed |
| D44 | Offline, non-destructive uninstall reversal | **`RemoveEngramCloudState` on uninstall deletes ONLY click's local `engram-cloud.json` bookkeeping file; it never shells out to un-enroll the shared cloud project (no token use, no shared-hive mutation).** Idempotent and offline. Implemented in sdd/engram-cloud-wiring (commit 4bdc15d). | Confirmed |
| D45 | Cloud enrollment failure is non-fatal to install/update | **A failed Engram Cloud enrollment/sync (unreachable or flaky server) surfaces a Spanish warning and the local `click install`/`update` still completes.** Cloud is opt-in and supplementary — it must never abort the purely-local steps (CLAUDE.md, memory-guard, models.json, OpenClaw). Rendered as a `⚠` warning, deliberately NOT via `RunStep`, so no red `✗/[FAIL]` line appears on an otherwise-successful run. Re-entrancy is safe: a partial failure leaves `Enrolled=false` and the next run re-runs the full `upgrade doctor→repair→bootstrap→sync` sequence (doctor+repair reconcile partial state). Resolves adversarial resilience review W1/W2. Implemented in sdd/engram-cloud-wiring (commit b811696). | Confirmed |

> **Owner note:** v0.1 repos live under the personal work account `Angel-MercadoCLK`
> (click-ai-devkit — which also hosts the scoop bucket per D23 — and, later, homebrew-click);
> migration to a Click Seguros org remains possible later without changing the design.

### CLI scope guardrail (agreed)
The CLI is a **thin installer/manager**, not the orchestration brain. Responsibilities:
- register/install the Click plugins through the native `claude plugin` CLI
- configure the Engram MCP
- install Click's `CLAUDE.md` rules
- commands: `install`, `update`, `doctor`, `uninstall`

The **SDD flow itself stays in markdown** (skills/agents) executed by Claude Code. This keeps v0.1
small even though a CLI now exists.

### Technical finding behind D3
Verified (gentle-ai README): gentle-ai's scoop path installs a **compiled Go CLI binary**, not
Claude Code plugins. Choosing "CLI now" means building + maintaining a binary, a scoop bucket, and
installers — a real cost, accepted deliberately for install-UX control.

## 3. Open questions queue (one at a time, suggested order)

1. ~~**CLI implementation stack.**~~ RESOLVED → D5: **Go**.
2. ~~**Memory policy posture + enforcement.**~~ RESOLVED → D6 (deny-by-default allowlist) + D7
   (deterministic PreToolUse guard). Still to author: `allowed-memory.md`, `forbidden-memory.md`,
   `SECURITY.md`, and the concrete PII/insurance pattern set.
3. ~~**Engram pinning.**~~ RESOLVED → D8: bundled at latest, pinned per click release.
4. ~~**ClickOrchestrator persona/identity.**~~ RESOLVED → D10.
5. ~~**SDD flow construction.**~~ RESOLVED → D9: reuse/rebrand existing SDD machinery.
   Still to decide within the flow: interactive vs automatic default, strict-TDD default.
6. ~~**Pilot scope.**~~ RESOLVED → D11: team-wide gated by a hardening canary.

### Minor items — mostly resolved via D12–D20 (tech-spec sign-off)
Resolved: SDD defaults (D12 interactive, D13 strict-TDD on), guard pattern structure (D14),
plugin activation path (D24 supersedes D16), installers (D17), scoop bucket (D18), guard latency (D19), audit log (D20).

Still genuinely open (authoring/build-time, not blocking planning):
- Author the concrete PII/insurance **regex** for each D14 category (validated by the red-team harness).
- Write the actual `allowed-memory.md`, `forbidden-memory.md`, `SECURITY.md` files (structure decided in D15).
- Build-time caveat (tech-spec §4): confirm whether Claude Code PreToolUse supports payload **mutation**
  for "redact"; if not, redact degrades to **block** (fail-closed) — this is now decided by D21 (v0.1 block-only).
- Update docs that still describe D16 as current; Spike C and D24 make `.claude-plugin/marketplace.json` part of the real install path.

## 4. Expected deliverables in this folder
- `vision.md`, `architecture.md`, `mvp-scope.md`, `sdd-workflow.md`, `adoption-plan.md`,
  `references.md` (per brief structure).
- PRD(s), tech-spec(s), implementation plan, requirements, recommendations.

> Next step: answer question #1 (CLI implementation stack), then #2 (memory policy).
