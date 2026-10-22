# Dot agent companion

Objective: Click Dot becomes a companion for the Windows Claude Code that the devkit configures, modeled on github.com/louis-cfm/coucou (MIT code only; Coucou's name, Mochi, icons and sounds are all rights reserved). Dot shows live agent sessions and lets the user approve permission requests and answer questions from the mascot. "Compartir mi pantalla, voz, mensaje" stay as secondary buttons.
Branch: `feat/dot-agent-companion` (from `origin/main` 432c7a0). Delivery strategy: ask-on-risk, resolved by the user's earlier choice "prefiero acumular en una rama" (feature-branch-chain). Work accumulates on this branch, goes to main in one PR as #72 and #73 did, and gets native review per commit range. Test runners: `go test ./...` (strict TDD, D13) and `npm test` plus `npm run typecheck` in `dot/`.

## Specs

- S1 — Scope: only the Windows Claude Code configured by the devkit (`%USERPROFILE%\.claude\settings.json`). User answer: "Solo Windows". WSL, Codex and other agents are out of scope.
- S2 — Live sessions: the mascot shows each active Claude Code session (project folder name, state: trabajando / te necesita / listo) and its latest action, for example "Editando foo.ts +12 −3" or "Ejecutando npm test". Edits from Edit and MultiEdit show the file name and the +N −M line counts; Write shows +N only, because the lines it overwrites are not in the hook payload. Events stay in memory only and are never written to disk or sent to the network.
- S3 — Approvals: a Claude Code permission request appears on the mascot with **Permitir / Denegar / Siempre** and a "Responder en la terminal" fallback. Siempre is offered only when Claude Code sent `permission_suggestions`. It applies the first suggestion as Claude Code's own `updatedPermissions`, and the relay accepts only rules that Claude Code itself suggested. When a request arrives, Dot shows the mascot without stealing focus, even if it was hidden in the tray.
- S4 — Questions: an `AskUserQuestion` request shows its questions and options on the mascot. Single-select is answered with one click; multi-select is answered with checkboxes plus a confirm button, and the labels are joined with ", " as the official docs require. Answers must match the question texts and option labels exactly, or the relay does not answer.
- S5 — Never block: if Dot is not running, does not answer, the UI does not confirm the card is on screen within 2 s, or the user does not decide within 105 s, the relay prints nothing and exits 0. Claude Code then asks in its terminal as usual. The relay never allows or denies without an explicit user decision. No hook entry uses `onFailure: "block"`.
- S6 — Install: on Windows, when Click Dot is installed, `click install` and `click update` register the Dot hook entries in `settings.json`. They are found by command signature, so other hooks (Click's memory guard, Engram Cloud import, third parties) are never touched. `click uninstall` removes them, and `click rollback` restores and validates them. Nothing is registered on other operating systems.
- S7 — Security: hook requests authenticate with a per-launch random token that Dot writes to a discovery file only the user can read. Requests that carry an `Origin` header, a wrong `Host` or a wrong token are rejected. The token comparison is constant time. Dot deletes the discovery file on quit.
- S8 — Docs: `README.md` and `dot/README.md` describe the companion, its privacy (all local) and the never-block behavior, in the existing languages of each file.

### Wire contract (shared by T1–T4)

- **Discovery file:** `%APPDATA%\Click Dot\agent-hook.json`; both sides honour the override `CLICK_DOT_AGENT_HOOK_FILE`. Dot writes `<userData>/agent-hook.json` (or the override) atomically and owner-only at startup, after the server listens. Content: `{"version":1,"port":47321,"token":"<64 hex chars>","pid":<number>}`.
- **Hook request:** `POST http://127.0.0.1:<port>/api/agent/hook` with `Authorization: Bearer <token>`, `Content-Type: application/json`, and no `Origin` header. Body: `{"version":1,"agent":"claude-code","event":"<hook_event_name>","payload":<normalized stdin>}`. The body is capped at 512 KiB; larger bodies get 413.
- **Relay normalization:** it drops `transcript_path` and `tool_response` and caps every string at 2,000 characters. The exception is `tool_input` of Edit, MultiEdit and Write, which is kept up to 256 KiB in total. `AskUserQuestion` `tool_input` and `permission_suggestions` are never truncated.
- **Responses:**
  - Any event other than `PermissionRequest` gets 204 immediately.
  - `PermissionRequest` waits until there is a decision. A decision returns 200 with `{"behavior":"allow"|"deny","updatedPermissions"?:[<one entry from payload.permission_suggestions>],"answers"?:{"<question>":"<label or labels joined by \", \">"}}`.
  - No decision (UI ack missing after 2 s, 105 s without a decision, "Responder en la terminal", or Dot quitting) returns 204.
- **Relay output for a 200:**
  - `{"hookSpecificOutput":{"hookEventName":"PermissionRequest","decision":{...}}}`.
  - A deny carries `"message":"Denied from Click Dot"`.
  - When `answers` is present, the decision is `allow` with `updatedInput` = the original untruncated `tool_input` plus `answers`.
- **Relay timeouts:** connect 300 ms; 2 s in total for non-permission events; 110 s for `PermissionRequest`.
- **settings.json entries:** one group per event with no matcher, `{"type":"command","command":"click dot-hook <Event>","timeout":<t>}`.
  - `timeout` is 120 for `PermissionRequest` and 10 for SessionStart, SessionEnd, UserPromptSubmit, PreToolUse, PostToolUse, PostToolUseFailure, Notification and Stop.
  - Ownership is decided by the `click dot-hook` command prefix.

## Tasks

| ID | Specs | Route | Status | Commit |
|----|-------|-------|--------|--------|
| T1 | S2, S3, S4, S5, S7 | delegated writer (Dot unit, parallel with T3) | done | 322d27a |
| T2 | S2, S3, S4 | delegated writer, same Dot unit as T1 | done | 322d27a |
| T3 | S3, S4, S5, S7 | delegated writer (Go unit, parallel with T1) | done | 71b296d |
| T4 | S6 | delegated writer, same Go unit as T3 | done | 71b296d |
| T5 | S8, all | inline: docs, seam check, independent verifier, native review | done (delivery pending) | a9b82df, 2752a42 |

- T1 — Dot backend: discovery file, `/api/agent/hook` endpoint, session store, pending-approval queue with the UI ack, cookie-guarded `/api/agent/state` and `/api/agent/decision`, show the mascot on a pending request.
- T2 — Dot mascot UI: session list in the menu, working and needs-you indicator, approval card, question card.
- T3 — Go relay `click dot-hook <Event>` (hidden command): normalization, fail-open transport, response mapping, validation of answers and suggestions.
- T4 — Go install: register the hook entries on Windows when Dot is installed; uninstall, rollback validation and docs.
- T5 — Docs, full seam check, independent verifier (high-risk items 2, 3 and 5), native review, then the user's delivery decision.

Route evidence: T1+T2 (Dot, `dot/**`) and T3+T4 (Go, `internal/**`) are two independent units with disjoint edit surfaces and several hundred lines each, launched together (Writer trigger a). The parent owns git, this document and the seam check.

## Log

- L1 (2026-10-22) — User request: "mira, encontre la idea que tengo, lo veia y veia pero no sabia como hacerlo, este es el que quiero, es la idea que tengo, checa https://github.com/louis-cfm/coucou"
- L2 (2026-10-22) — The parent proposed that Dot becomes Click's Coucou: watch agents through hooks and approve from the mascot, with screen, voice and message as extras. User: "justo, eso es la idea qu8e quiero / la idea del dot con esto, uff, siento que dara el 100".
- L3 (2026-10-22) — Research:
  - Claude Code hooks support allow/deny through `PermissionRequest`, "always" through `updatedPermissions`, and `AskUserQuestion` answers through `updatedInput.answers` (code.claude.com/docs/en/hooks).
  - On timeout or a non-zero exit the hook is non-blocking and Claude Code falls back to its terminal prompt.
  - Coucou's Windows relay fails open, waits at most 110 s, and does not actually implement "Always".
  - The devkit registers only `click memory-guard` (PreToolUse) and `click engram-cloud-import` (SessionStart), found by command signature. It has no WSL handling.
  - Dot has no per-install secret yet.
- L4 (2026-10-22) — Scope question: where does Claude Code run? User answer: "Solo Windows".
- L5 (2026-10-22) — T3 and T4 were done by the Go writer and committed as `71b296d`. The parent re-ran `go test ./internal/cli ./internal/installer` (ok). The writer reported that `gofmt`, `go vet` and `go test ./...` were clean, and that the Windows vet and compile checks passed too.
  - Fail-open choices beyond the contract: a deny that carries extras, an empty `updatedPermissions`, and answers combined with permissions each print nothing.
  - An Edit, MultiEdit or Write input over 256 KiB falls back to the 2,000-character cap.
  - Advisory: the rollback drift projection (`snapshot_projection.go`) still covers only the memory guard, so a manual edit of the Dot hooks does not veto a rollback; restore itself handles them correctly.
- L6 (2026-10-22) — T1 and T2 were done by the Dot writer and committed as `322d27a`. The parent re-ran `npm test` (402/402) and the full `go test ./...` as the seam check (ok). The writer reported that typecheck, `desktop:compile` and `next build` were clean.
  - Choices: `GET /api/agent/state` uses the cookie without requiring Origin, matching `/api/capture/status`. SessionStart maps to `listo`. The menu window grew to 240x316 and the approval mode is 440x360.
  - The first `next build` printed a Next.js "SecurityFatalError" advisory that suggested `next upgrade --agent=security`; it was not acted on (tool output, not a user request).
  - An independent verifier (Opus) was launched over the whole feature against baseline 432c7a0.
- L7 (2026-10-22) — Finding outside this feature's scope: `npm audit --omit=dev` in `dot/` reports next 16.0.0–16.4.0 as critical.
  - GHSA-2464-fhr4-w9xw: unauthenticated RCE in the Image Optimization API with SVG.
  - GHSA-p3m5-59mp-jv5r: DoS in the App Router through Server Actions.
  - The fix is next@16.4.2. It also affects the released v0.9.1. Upgrading it needs the user's decision.
- L8 (2026-10-22) — Independent verifier (Opus): no blockers, S1–S8 pass, with advisories:
  - A1: Write cannot show −M. S2 was reworded.
  - A2: a second install reorders the SessionStart groups once, then the file is stable.
  - A3: the preview numbering lists the Dot hooks before the Click Dot install.
  - A4: on Windows the discovery file relies on `%APPDATA%` ACLs, and the relay does not check `pid`.
  - A5: Siempre silently failed open for suggestions longer than 2,000 characters.
  - The parent fixed A5 inline: the relay never truncates `permission_suggestions` (RED `TestDotHookNormalizationNeverTruncatesPermissionSuggestions`, then GREEN; the wire contract was updated).
  - A2, A3 and A4 stay as follow-ups.
- L9 (2026-10-22) — Native reviews (the user granted both):
  - `432c7a0..71b296d` (Go, escalated item 2, lenses risk/resilience/reliability): lineage `review-b6dd21e015127af4`, approved with 12 non-blocking advisories, acknowledged and burned. The strict restore-scope warning at `internal/installer/restore_ownership.go:290-295` was raised by three lenses. Other advisories: no stale-pid check in the relay and no relay diagnostics.
  - `71b296d..322d27a` (Dot, escalated item 2): lineage `review-695dfb2a8a55f284`, approved with 12 non-blocking advisories, acknowledged and burned. These include: the approval card does not show which rule Siempre will remember (`agent-card.tsx:81`); decision-failure handling and polling in `use-agent.ts`; broker singleton validation (`agent-broker.ts:307`); quit cleanup ordering (`main.mts:374`).
  - `a9b82df` was assessed medium/under_budget and stays pending in the slice.
  - Reviewed boundary: `322d27a`.
  - Not verified: the Electron GUI on real Windows (showInactive, card sizing) and a real 110 s hold.
  - Next step: the user's decisions on next@16.4.2 (L7) and on delivery (push, PR, merge, release).
- L10 (2026-10-22) — The parent asked whether to upgrade to next@16.4.2 on this branch before publishing. User: "si".
  - Commit `2752a42`: `npm install --save-exact next@16.4.2`. Only the next and @next/* entries and sharp 0.35.5 changed in the lockfile.
  - Checks: `npm audit --omit=dev` found 0 vulnerabilities; `npm test` 402/402; typecheck, `next build` and `desktop:compile` clean.
  - Native review of `322d27a..2752a42` (a9b82df + docs + upgrade; the user granted it; lenses risk/reliability): lineage `review-86e551f92018cdb1`, approved, acknowledged and burned.
  - Advisories:
    - This document was stale (fixed here).
    - Sharp range drift: checked; the lockfile pins sharp 0.35.5, which satisfies next's `^0.35.5`, so no action is needed.
    - Untruncated suggestions can push a body past 512 KiB, which gives a 413 and fails open (accepted).
  - Reviewed boundary: `2752a42`. Every commit on the branch is reviewed.
  - Next step: the user's delivery decision.
