# Dot alive

Objective: the island can disappear completely, the Lente character feels alive and fluid, and the chat keeps a browsable history. This feature does not cover the agent engine (Codex, computer use, Chrome, Engram, parallel Claude Code); that goes in a separate feature after research.
Branch: `feat/dot-alive` (from `origin/main` 944bce5). Delivery strategy: ask-on-risk, resolved by the user's earlier choice "prefiero acumular en una rama": one PR. Test runner: `npm test` and `npm run typecheck` in `dot/`.

## Specs

- S1 — Hide the notch. User words: "no me gusta que todo el tiempo esté la muesca deberiamos de poder ocultarla".
  - When nothing is going on, the island is fully hidden: no visible bar. An invisible hot strip on the top edge, at the island's position, reveals it on hover.
  - `Ctrl+Shift+Espacio` toggles it.
  - The tray menu gains "Ocultar Dot" / "Mostrar Dot", which disables or re-enables the island until toggled back. The choice persists in the desktop preferences.
  - While hidden by the user, a pending approval still brings the island up without stealing focus: approvals never silently time out because the island is hidden.
- S2 — Alive character. User words: "se ve horrible la mascota, estatica, quiero que se mueva, que se vea fluida, bonita, bien, nada estatica, que parezca con vida".
  - The Lente eye follows the mouse cursor anywhere on the screen. The cursor position comes from the main process; poll it at about 30 Hz only while the island is visible, and smooth it with a spring.
  - Idle life: the eye glances around, blinks at natural random intervals (sometimes double), breathes, and the iris dilates slightly when listening or speaking.
  - Hover makes the eye widen; a click gives a squash-and-stretch bounce.
  - Each state change (idle, working, needs-you, done) changes color and shape with spring motion, never a jump. Working shows a lively orbit; needs-you adds a gentle attention bob.
  - Opening, closing and resizing the island morph smoothly: content fades and scales in, and does not pop.
  - Reduced motion (Windows "Animation effects" off) lowers the amplitude and removes the bounces, but the character is never fully static: it still blinks and follows the cursor gently.
  - Animations run on the compositor (transform and opacity) and stay smooth; no layout thrash.
- S3 — Chat history. User words: "quiero tener historial de chats".
  - The chat card has a history view listing previous conversations, newest first. Each shows a title (the first user message, trimmed) and a relative time.
  - The user can open one and continue it, start a new chat, or delete one, with confirmation.
  - It uses the existing conversation store (30 conversations per account, unchanged persisted format).
- S4 — Keep everything working: agent tools and approvals, Y/N keys, capture indicator, voice, sign-in. The packaged smoke labels stay exactly: "Compartir mi pantalla", "Voz", "Mensaje", "Conectar con ChatGPT".

## Tasks

| ID | Specs | Route | Status | Commit |
|----|-------|-------|--------|--------|
| A1 | S1, S2, S3, S4 | delegated writer (one UI unit; the engine research runs in parallel, read-only) | done | (next commit) |
| A2 | all | inline: seam check, native review, delivery | pending | |

## Log

- L1 (2026-10-22) — User message: "me gusta, solo no me gusta que todo el tiempo esté la muesca deberiamos de poder ocultarla, adicional, claude code no es que deba de funcionar no entendiste / iniciamos sesion con openai, openai usa gentle-ai, engram, y las reglas de click para programar, tiene todo acceso al shell, a usar tu maquina como agente, con el mouse y todo, puedes enviarle archivos de todo tipo, y cosas asi para que las lea, o lo que sea, en fin / con claude code yo me referia a que si yo le decia "sabes que, esta tarea la ejecutas en claude mientras tu haces x" lo haga / quiero que pueda usar mi computadora, mi chrome, todo / y se ve horrible la mascota, estatica, quiero que se mueva, que se vea fluida, bonita, bien, nada estatica, que parezca con vida / quiero tener historial de chats, quiero que pueda consultar engram, que tengamos esa interaccion correctamente y organica"
- L2 (2026-10-22) — Scope split. This feature covers hiding, an alive character and chat history. The engine items (a coding agent with gentle-ai, Engram and Click rules; full shell; files of any type; mouse and keyboard; the user's Chrome; parallel Claude Code delegation; Engram queries) wait for research on using Codex as Dot's engine.
- L3 (2026-10-22) — A1 was done by the writer.
  - What it built:
    - Hide: the collapsed island is an invisible 4 px hot strip at about 1% alpha; tray "Ocultar Dot"/"Mostrar Dot", persisted as optional `islandHidden`; approvals and shared screens still surface the island.
    - Alive Lente: one rAF loop in `lente-motion.ts` with cursor gaze (main polls about 30 Hz while visible and pushes through a constant `executeJavaScript` event), random blinks, glances, breathing, dilation, hover and squash, spring states. Reduced motion gives about 55% amplitude, never static; the old CSS keyframes are removed.
    - Morph: a clip-path unfold, and a fade before shrinking.
    - History: "Historial" with open, new and delete with confirmation, on the existing store.
  - Checks: `npm test` 740/740 with the engine work in the tree; typecheck, compile and build clean; headless CDP QA of gaze, states, history and the smoke selectors.
  - Writer's open choices, accepted by the parent:
    - The single flipping tray item replaces "Mostrar a Dot".
    - `Ctrl+Shift+Espacio` opens and closes the chat card, which works as the show/hide toggle.
  - Not verified: the real Windows window (hover on a 4 px strip, click-through, the animation-effects flag, 30 Hz cost).
