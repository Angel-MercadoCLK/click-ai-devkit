# Dot polish

Objective: fix what the user hit testing v0.12.0. Dot re-detects Codex after an update, the copy speaks for Dot rather than for Claude Code, and a new character that is aesthetic, dynamic, pretty and entertaining replaces the Lente ball.
Branch: `feat/dot-polish` (from `origin/main`). Delivery strategy: one PR (the user's accumulate-on-a-branch choice). Test runner: `npm test` and `npm run typecheck` in `dot/`.

## Specs

- S1 — Codex re-detection. User words: "lo instale cuando tenia la version 0.145.0 de codex y necesita la mas actual, actualizo codex pero como ya habia iniciado sesion ... simplemente sigue apareciendo el mensaje ... no tiene refresh".
  - When Codex is missing, unsupported or failed, Dot checks again by itself every time the chat card opens, at most every 15 s while the card is open, and when the user presses "Reintentar" in the engine note.
  - A Codex that becomes valid starts without restarting Dot.
  - The supported range accepts 0.163.0 and anything newer (`>=0.163.0`). Versions newer than the generated protocol are tried; if the handshake fails, the note says so in Spanish.
  - A Codex that is already signed in is used at once; there is no second sign-in.
- S2 — Copy. User words: "sigue diciendo de claude code que vera las sesiones abiertas, estas muy enfocado en claude code".
  - The island speaks for Dot as the user's assistant. The menu shows "Actividad": Codex threads in progress, background Claude tasks and Claude Code sessions, all as activity.
  - The empty state invites the user to ask Dot something.
  - Claude Code appears only as one of Dot's tools.
- S3 — Character. User words: "el diseño es fatal de la bola, quiero que lo hagas mas estetico, dinamico, bonito y entretenido".
  - A new character, chosen by the user from an animated prototype, replaces the Lente.
  - It must read clearly on the black island: bright, high contrast, legible at 32 px.
  - It has personality: idle behaviors, emotes for working, needs-you, done, error, listening, sleeping when idle for a long time, and reactions to hover and click.
  - Reduced motion lowers the amplitude but is never static.
- S4 — Keep everything working; the packaged smoke labels stay exactly: "Compartir mi pantalla", "Voz", "Mensaje", "Conectar con ChatGPT".

## Tasks

| ID | Specs | Route | Status | Commit |
|----|-------|-------|--------|--------|
| P1 | S1, S2, S4 | delegated writer (parallel with the design prototype) | done | 2974fd9 |
| P2 | S3 | delegated writer after the choice (Orbi), parallel with P1 on disjoint files | done | e490a32 |
| P3 | all | inline: seam check, review, delivery | pending | |

## Log

- L1 (2026-10-22) — User message: "ahora, lo instale, tiene muchos errores, lo instale cuando tenia la version 0.145.0 de codex y necesita la mas actual, actualizo codex pero como ya habia iniciado sesion y literalmente ya estaba iniciado, simplemente sigue apareciendo el mensaje de que doy necesita codex en la version x y si no se usa con su asistente integrado, no tiene refresh, sigue diciendo de claude code que vera las sesiones abiertas, estas muy enfocado en claude code, el diseño es fatal de la bola, quiero que lo hagas mas estetico, dinamico, bonito y entretenido"
- L2 (2026-10-22) — Verified causes:
  - `CODEX_SUPPORTED_RANGE = ">=0.163.0 <0.164.0"` is checked only in `start()` at launch, with no re-check.
  - The menu's empty state reads "Sin sesiones de Claude Code. Dot te avisa cuando haya una."
  - QA screenshots show the Lente as a tiny dark ball on the black island, with no contrast.
  - The latest Codex on npm is 0.163.0.
- L3 (2026-10-22) — The parent published the animated prototype "Personajes v2" (A Chispa, B Nube, C Orbi; seven states, gaze following, click boing) on the design canvas. User choice: "me gusta orbi, esta bonito". S3 character: Orbi, a little planet with a gradient ring and an orbiting moon, as in the canvas board `Personajes-v2.dc.html`.
- L4 (2026-10-22) — P1 was done by the writer: `npm test` 828/828; typecheck, compile and build clean.
  - `recheck()` coalesces, shows no "starting" flicker, and is triggered when the chat opens, every 15 s while it stays open, and by "Reintentar" through `POST /api/codex/recheck`.
  - The range is `>=0.163.0`, with a Spanish note when a newer version's handshake fails. A signed-in Codex is used at once.
  - The island shows "Actividad" (Codex turns, Claude tasks, Claude Code sessions) with "Todo tranquilo. Pídele algo a Dot." and a "Pedir algo a Dot" button. Smoke selectors are intact.
  - Open: the unused `AGENT_TEXT` strings in `lib/capture-state.ts` still say "Claude Code está trabajando".
- L5 (2026-10-22) — P2 was done by the writer. Orbi replaces Lente everywhere: a ring split into back and front halves, a moon passing behind and in front, a capture halo apart from the ring, and a simplified small-size look. It has seven states plus hover and click; sleep comes after 3 minutes of quiet, done lasts 8 s, error is wired to failed Claude tasks, and listening is its own state.
  - Checks: `npm test` 845/845; typecheck and compile clean. QA screenshots in headless Chromium, reviewed by the parent (menu done and needs, pill working).
  - Not verified: the real Windows cursor feed, display scaling, frame rate.
