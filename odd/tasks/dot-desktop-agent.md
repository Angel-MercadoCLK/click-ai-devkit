# Dot Desktop Agent

Objective: turn Click Dot into an always-running desktop agent with its own chat that can see the user's screen (with permission), be called on demand, and be installed, updated and removed by click-ai-devkit.
Branches: `click-dot` tracker `feat/local-alpha` (feature-branch-chain); `click-ai-devkit` new feature branch for the installer task. Delivery strategy: ask-on-risk. Test runners: `npm test` (click-dot), `go test ./...` (click-ai-devkit).

## Specs

- S1 — Installed with the devkit. User: "Que cuando instalemos el click ai dev kit, instalemos esto que traemos que es el repositorio que altera claude, y esas cosas, que traemos pero tambien click dot" and "quiero que se instale todo con el click ai dev kit". `click install`/`update`/`uninstall`/`doctor` manage Dot like the rest of Click-managed content.
- S2 — Always running and callable. User: "click dot quiero que viva y vea todo, puedas llamarlo, etc".
- S3 — Own chat, sees what the user does with permission. User: "quiero que sea como un pequeño agentito que tiene su propio chat, que ve todo lo que haces, si se lo permites, etc". Reference: https://www.copilotkit.ai/blog/introducing-opendots
- S4 — Screen observation from the first version, not only developer-tool activity. User: "dale necesito que desde el inicio".
- S5 — The model receives the screen images, not only locally extracted text. User: "que vea".
- S6 — Before an image leaves the machine, Dot blurs regions containing personal data, found locally with OCR and the memory-guard patterns; the model still sees the screen. User answered "si" to: "propongo que Dot difumine en la máquina las zonas con datos personales antes de enviar cada imagen. Usaría OCR para encontrar el texto y los patrones del `memory-guard` para decidir qué difuminar. El modelo sigue viendo la pantalla, pero no los nombres, documentos o pólizas. ¿Incluimos ese difuminado en la tarea 3?"

Assumptions (not user requirements; confirm or correct):
- A1 — Windows first, since the devkit is distributed through Scoop. macOS/Linux later.
- A2 — Desktop shell is Electron: it can host the existing Next.js/Node server and provides screen capture, tray, autostart and global shortcuts natively. Tauri would need a Node sidecar for the existing code.
- A3 — Model access keeps the existing per-user Sign in with ChatGPT flow; image input through `POST /v1/responses` must be verified before T3.
- A4 — "si se lo permites": capture is off until the user turns it on, shows a visible indicator while active, and can be paused at any time.

Known constraint: current storage fails closed on native Windows until private ACLs can be verified (README). T1 must solve protected storage on Windows.

Open risk: S5 sends screenshots that may contain insured-party personal data to OpenAI under each user's personal ChatGPT account. S6 reduces it; Click Seguros legal/compliance approval is still pending before team rollout.

## Tasks

| ID | Specs | Task | Route | Status | Commit |
|----|-------|------|-------|--------|--------|
| T1a | S2, S3 | Electron shell on Windows: tray, autostart, global shortcut opening Dot's chat window, in-process Next server on a fixed loopback port, storage protector injection | delegated (writer trigger a: parallel with T1b, disjoint surfaces) | done (interactive Windows checks pending T7) | 6f57bcf (deps), a4465d1 |
| T1b | S2, S3 | Windows-protected credential/history storage (DPAPI through the injected protector); unchanged POSIX behavior; native Windows without protector keeps failing closed | delegated (writer trigger a: parallel with T1a) | done (real Windows checks pending T7) | fd779d9 |
| T1c | S2, S3 | Shell hardening from native review advisories: subframe navigation guard, startup failure dialog, second launch before server ready, tray preference save failure, unreadable preferences leave login item alone, visible encryption-unavailable warning | inline (small, ~85 lines) | done | see L10 |
| T2 | S3, S4 | Opt-in screen capture: permission, visible indicator, pause, bounded local retention | TBD | pending | |
| T3 | S5, S6 | Send captured screen images to the model in chat with local personal-data blurring; verify SIWC image input | TBD | pending | |
| T4 | S2 | Call Dot from Claude Code/Codex (local MCP server exposed by Dot) | TBD | pending | |
| T5 | S1 | Click Dot Windows release pipeline: packaged artifact, checksums, GitHub release | TBD | pending | |
| T6 | S1 | click-ai-devkit: pinned Dot version, install/update/uninstall/doctor/rollback of Dot | TBD | pending | |
| T7 | S1–S5 | Disposable Windows qualification of devkit install plus Dot end to end | TBD | pending | |

## Log

- L1 (2026-10-21) — Original request: "a ver, cual es mi idea? Que cuando instalemos el click ai dev kit, instalemos estoq ue traemos que es el repositorio que altera claude, y esas cosas, que traemos pero tambien click dot, y click dot quiero que viva y vea todo, puedas llamarlo, etc, mira, por ahi vi una idea https://www.copilotkit.ai/blog/introducing-opendots, quiero que sea como un pequeño agentito que tiene su propio chat, que ve todo lo que haces, si se lo permites, etc"
- L2 (2026-10-21) — Verified: click-ai-devkit `main` has no Click Dot references; install/update do not provision Dot today.
- L3 (2026-10-21) — OpenDots reference (per article): open-source, self-hosted Dots with own role and chat, CopilotKit/AG-UI, computer access off by default. It does not describe screen capture.
- L4 (2026-10-21) — User chose screen observation from the start and full install through the devkit: "dale necesito que desde el inicio, y quiero que se instale todo con el click ai dev kit".
- L5 (2026-10-21) — Offered local-only text extraction (no raw screenshots leave the machine). User chose image input: "que vea".
- L6 (2026-10-21) — Pending decision: whether to blur personal-data regions locally (OCR + memory-guard patterns) before sending images. The model would still see the screen.
- L7 (2026-10-21) — User: "si". L6 resolved into S6 (linked to T3). Next: T1.
- L8 (2026-10-21) — T1 split into T1a (shell) and T1b (storage), built in parallel on branch `feat/dot-desktop` from `feat/local-alpha`. Contract between them: `lib/storage-protector.ts` reads `globalThis[Symbol.for("click-dot.storage-protector")]` (`{ encrypt(text): Buffer, decrypt(data): string }`), set by Electron main from `safeStorage` before Next starts in the same process. Electron pinned to 44.9.3 (Node 24.22.0). Risk: item 2 (credential storage) and item 5 (packaging/dependencies).
- L9 (2026-10-21) — T1 evidence. Writers: T1b RED 8/9 then GREEN; fixed drive-letter split in `ensurePrivateDirectory`. T1a RED then GREEN; Next config served via `__NEXT_PRIVATE_STANDALONE_CONFIG` (avoids runtime swc download), `process.chdir(appDir)`. Packaged `release/win-unpacked` (546 MB) served 200 with Click Dot markers on Windows via WSL interop; tray, shortcut, hide-on-close and port-busy dialog unverified (WSL interop runs in session 0). Independent Opus verifier: POSIX storage identical to baseline on 37 probes; Windows branch fails closed on every abuse case; no navigation bypass. Blocker found: sign-in cookie set in the Electron session but callback lands in the system browser. Correction: `systemBrowserTarget` sends sign-in to Dot's page in the system browser so the whole flow runs there (RED then GREEN); recheck confirmed resolved. Checks: `npm test` 72/72, `npm run typecheck` clean, `npm run desktop:compile` ok, `git diff --check` clean. Follow-ups (advisories, not blockers): unreadable preferences file re-enables start at login; subframe navigation unguarded; Electron fuses/asar hardening (T5); trim ~300 MB unused production deps (T5); per-user install location because Next writes route cache in the app dir (T5/T6); DPAPI does not protect against same-user processes (document).
- L10 (2026-10-21) — Review. The T1 commit exceeded the native lens context budget (package-lock), so it was split locally (unpushed) into `6f57bcf` deps (medium, under_budget, no review due), `fd779d9` storage and `a4465d1` desktop; final tree identical. User granted both reviews. `fd779d9`: lineage `review-b15aa7557eb829df`, 3 lenses, approved with 6 advisories, acknowledged and burned. `a4465d1` (escalated, item 2): lineage `review-29ddf89df9e21b70`, approved with 11 advisories, acknowledged and burned. Reviewed boundary: `a4465d1`. T1c addresses the shell advisories (RED 2 then GREEN; `npm test` 73/73, typecheck and desktop compile clean). Remaining follow-ups: Windows storage directory ownership/writability not verified (integrity, not confidentiality); undecryptable store has no recovery path; Windows rename has no retry; auth write/read size bound asymmetry; asar off and Electron fuses not hardened (T5); untested `startDotServer` env/chdir/500 path.

