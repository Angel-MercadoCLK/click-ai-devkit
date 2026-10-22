# Dot island design

Objective: Dot's look and window behavior follow Coucou's design quality, with Dot's own character. A black island hangs from the top edge of the screen, hides when idle, and expands into dark nested cards. The reference is the prototype canvas https://claude.ai/artifact/5sUepL2oWVjSRnpyFnRc5G.
Branch: `feat/dot-agent-tools` (accumulates with the agent tools feature; one PR). Test runners: `npm test` and `npm run typecheck` in `dot/`; the packaged smoke runs in CI.

## Specs

- S1 — Character: "me gusta lente". Dot is the Lente character (B) from the canvas: a graphite glossy sphere with a single camera-lens eye, drawn in code (SVG or Canvas, no image assets).
  - The iris color follows the state: idle pearl, working blue, needs-you amber with a "!" badge, done green with a half-closed happy lid.
  - It breathes and blinks. While working, a dashed ring orbits the iris.
  - No Coucou assets are used: not Mochi, not its name, not its sounds.
- S2 — Island window: Dot lives at the top center of the display as a black island with rounded bottom corners. It is always on top, never steals focus, and is click-through outside the island.
  - **Oculta** (nothing running): a thin bar. Hovering the top edge peeks it out.
  - **Compacta** (an agent is working or something is new): the character, the latest action and a session count.
  - **Expandida** (on click or when Dot needs you): sessions, actions, approval, question or chat cards.
- S3 — Visual language from the canvas:
  - Island #000 with bottom radius 32.
  - Cards #141519 with a 1px rgba(255,255,255,.07) border and radius 24.
  - Text #F5F5F7 and secondary #A1A1AA. Accents: working #7FA6FF, needs-you #FFB340/#FFC864, done #7FE0A6, removed lines #FF8A7A.
  - Inter (bundled locally or the system Segoe UI Variable, no network font) and a monospace font for commands.
  - Pill buttons, the primary one white, with keyboard hints (N deny, Y allow). Stroke icons.
- S4 — Every existing function keeps working inside the island:
  - sessions with +N −M;
  - approval and question cards;
  - the agent chat with tool activity lines and Detener;
  - Compartir mi pantalla, Voz, Mensaje;
  - sign-in.
  The packaged smoke labels stay exactly: "Compartir mi pantalla", "Voz", "Mensaje", "Conectar con ChatGPT".
- S5 — Keyboard: while an approval card is shown, Y allows and N denies, and only while the island has focus.

## Tasks

| ID | Specs | Route | Status | Commit |
|----|-------|-------|--------|--------|
| D1 | S1–S5 | after the agent tools writers finish (they edit `dot/app`), one writer for the island UI and window behavior | pending | |

## Log

- L1 (2026-10-22) — User request: "bro revisa cou cou, que hermoso diseño, no la basura de diseño que hiciste donde tiene un sol, quiero algo bonito que si fucnione pero a nivel visual se vea tan estetico, de verdad,la idea es hacer como esto https://github.com/louis-cfm/coucou en diseño y todo"
- L2 (2026-10-22) — The parent studied Coucou's captures and published the canvas with three original characters (Orbe, Lente, Gota) in four states and the island boards. Coucou's Mochi, name and sounds are all rights reserved, so Dot keeps its own character.
- L3 (2026-10-22) — User choice: "me gusta lente".
