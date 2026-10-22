# Click Dot local alpha

Click Dot is an independent local-first assistant prototype. Each installation connects its user’s own ChatGPT account through Sign in with ChatGPT; it does not use an API key or the machine’s Codex login. No chat or agent tools are enabled in this alpha.

## Quick start

Requirements: Node.js 24 or newer.

```sh
npm install
npm run dev
```

Open `http://127.0.0.1:3000` on the same machine running Click Dot. Choose **Continue with ChatGPT** to authorize this installation in your browser. The callback uses that same loopback host and port. A remote development tunnel must forward the callback port to the browser machine; this is not a hosted service.

Click Dot requests identity and direct ChatGPT plan-usage scopes. Signing in without the `chatgpt.tokens.use.direct` grant is not considered model access. Once connected, choose **Load models** to request the current account-specific model list, then select a model and send a message. Chat requests use the locally stored OAuth access token only on the server; the token never reaches browser code. Expired sessions require explicit authorization again in this alpha; automatic refresh is not implemented.

## Checks

```sh
npm test
npm run typecheck
npm run build
```

`npm start` serves a production build on loopback. The development and production scripts set `NEXT_TELEMETRY_DISABLED=1`.

## Desktop app (Windows)

Click Dot can run as a Windows tray app built with Electron. It serves the production Next.js build from the same process on `http://127.0.0.1:47321` (fixed, because the Sign in with ChatGPT callback uses this port) and keeps running in the tray. There is no app window: Dot lives on your desktop as a small mascot (see "The mascot").

```sh
npm run desktop:build      # next build + compile the Electron main process
npm run desktop:start      # run the desktop app locally (needs a display)
npm run desktop:pack:win   # unpacked Windows x64 app in release/win-unpacked
```

- Data lives in the per-user app data folder (`auth.json`, `conversations.json`). Credentials and history are encrypted with Electron `safeStorage` (Windows DPAPI). If OS encryption is unavailable, storage stays closed.
- If port 47321 is busy, the app shows an error and quits instead of choosing another port.
- Start with Windows is on by default for packaged builds and can be turned off from the tray menu. Pass `--no-login-item` (or set `CLICK_DOT_DISABLE_LOGIN_ITEM=1`) to never register it, for example in smoke tests.
- Choosing **Conectar con ChatGPT** in the mascot's chat bubble opens Click Dot's home page in the system browser; choose **Continue with ChatGPT** there to finish signing in, because the callback must reach the same browser that started it. The bubble refreshes when you come back to the mascot.
- Every web permission request (camera, microphone, screen, notifications) is denied in this version.
- The packaged app ships as a validated `app.asar` (embedded integrity check on Windows) and the exe has these Electron fuses set: `ELECTRON_RUN_AS_NODE`, `NODE_OPTIONS` and `--inspect` are disabled, cookie encryption is on, and only the asar is loaded. Next.js keeps its response cache in memory and never writes inside the install directory, which is replaced on update. Packaging edits the exe resources (icon, ProductName, version) with resedit, so it works from Linux or WSL without wine; it needs network access to fetch electron-builder's icon tools.
- Code signing is not set up yet, so Windows Smart App Control may block a freshly built unsigned `Click Dot.exe`.

### The mascot

- Dot is a small spark character, frameless and transparent, always on top and off the taskbar. It starts in the bottom-right corner of the primary display's work area, you can drag it anywhere (the position is remembered and kept on a visible display, also when monitors change), and the window cannot be resized by hand.
- Click the mascot to open a compact menu with three buttons: **Compartir mi pantalla**, **Voz** and **Mensaje**. Click it again, or press `Esc`, to close it. The menu opens toward the side of the screen with more room.
- **Mensaje** opens a compact chat bubble (about 360 x 520) beside the mascot, kept inside the screen. It is the same chat as the home page (model picker, messages, composer, redacted screen preview). When Dot is not connected, the bubble shows **Conectar con ChatGPT**, which starts the sign-in in the system browser.
- **Voz** listens through Windows' built-in speech recognition (`POST /api/voice/listen`), shows that it is listening with a way to cancel (`POST /api/voice/cancel`), then opens the bubble with the transcript in the message box for you to review. Nothing is sent until you press Enviar. If voice is unavailable the menu says so briefly.
- `Ctrl+Shift+Space` opens or closes the chat bubble, bringing the mascot back first if it was hidden. Closing the window never leaves an invisible window: the tray item **Mostrar a Dot** shows it again.
- The window is moved and resized by the desktop app. The page asks through `POST /api/window` (`{"mode":"collapsed"|"menu"|"chat"}` or `{"move":{"x":0,"y":0}}`, exact local Origin, 256-byte strict body); there is no preload script and the sandbox stays on. The mascot is dragged by hand (pointer events) because Electron drag regions swallow clicks.
- The mascot page is `/mascot`; the home page `/` stays for the system browser, where sign-in finishes. When Dot starts with Windows it waits in the tray; use the tray item **Mostrar a Dot** or `Ctrl+Shift+Space` to bring the mascot up. Opening Dot yourself shows the mascot.

### Screen capture

- Off by default. Turn it on from the mascot menu ("Compartir mi pantalla") or from the tray ("Let Dot see my screen"). Your choice is remembered across restarts.
- While it is on, the mascot shows a red ring (amber when paused) with a pause button and the label "Viendo tu pantalla" in the menu, and the tray icon shows a red dot with the tooltip "Click Dot — seeing your screen".
- Pause at any time from the mascot or the tray. Pausing is for the current session only, so a restart resumes capture if it was on.
- Dot captures your main display every 10 seconds and keeps only the last 6 frames (about one minute) in memory. Frames are never written to disk and are dropped when you pause, turn capture off, or quit.
- Capture runs inside the desktop app, and web pages still get no screen, camera or microphone permission. In a plain browser or `npm run dev`, capture is unavailable. Frames leave the computer only as described in "Screen in chat".

### Screen in chat

- When screen capture is on, each chat message includes your latest screen image by default. Before sending, the composer shows the exact image with "N regions hidden" and a checkbox to leave the screen out of that message.
- Before an image leaves your computer, Dot finds text on it with Windows' built-in OCR and pixelates every area matching the Click memory-guard patterns (emails, document numbers, phones, amounts, policy, claim and customer IDs, credentials). The model still sees the rest of the screen. Text recognition can miss text, and faces and photos are not hidden.
- Everything runs locally in memory: the frame is handed to a Windows PowerShell helper over stdin and is never written to disk. If OCR, the pattern file or redaction fails, nothing is attached and the composer says why; you can then send the message without the screen.
- The redacted image is held server-side for two minutes under a one-use id; the browser never sends image bytes. Images are not saved in history, which keeps only "[screen attached]". One image per message, up to 4 MB.
- Image input depends on the selected model. If a model refuses the image, Dot says so; choose another model.
- To check a model on demand, POST `{"model":"<slug>"}` to `/api/chat/vision-probe` with the local Origin header. It sends a synthetic 64x64 image and reports only status and error code.
- To refresh the vendored patterns: `node scripts/sync-guard-patterns.mjs <click-ai-devkit>/internal/guard/patterns.yaml`. The corpus `test/fixtures/guard-corpus.json` is meant to be shared with the devkit.

## Account and runtime boundary

- The browser page is not connected to the eve agent runtime. No `/eve/v1` routes or agent tools are exposed. Chat accepts only a user message and a model from the current account’s catalog; it does not accept tools, system roles, arbitrary provider arguments, URLs, or OS capabilities.
- The eve definition has no enabled default or parent-callable tools, and this scaffold declares no tools, connections, channels, subagents, or skills.
- Tokens and the issued local application registration are stored under `~/.config/click-dot/auth.json` with owner-only permissions. They are not sent to browser storage or exposed by the status route. Conversation history is stored separately in `~/.config/click-dot/conversations.json`, partitioned by a one-way account identifier and protected with owner-only permissions; chat history can be deleted per conversation or cleared from the UI. Disconnect removes local credentials but does not delete history. A private lock directory serializes updates across local Click Dot processes. If a process crashes while holding the lock, Click Dot fails closed rather than reclaiming it based on age or PID. Only after stopping every Click Dot process that uses a locked file, manually remove the empty lock directory with `rmdir`; do not remove a lock while a server may still hold it. On native Windows, Click Dot stores credentials and conversation history encrypted with the Windows user's DPAPI key through the desktop app's protector (Electron `safeStorage`), so POSIX file modes are not relied on. Without that protector, for example plain `npm run dev` on native Windows, storage fails closed; use the desktop app or WSL2/Linux.
- Disconnect removes Click Dot’s local credentials. To revoke the app remotely, use the connected ChatGPT account’s app settings.
- The callback binds state to a short-lived, one-use browser attempt and validates ID-token signature, issuer, audience, expiry, and nonce using OpenAI’s published signing keys. Development request logging excludes the callback URL because it contains a short-lived authorization code. Offline tests use synthetic credentials and do not contact OpenAI.
- The chat flow follows OpenAI’s [Sign in with ChatGPT model and inference guide](https://developers.openai.com/siwc/token-sharing-open-source/models-and-inference): it loads `/v1/models` on user action, displays only `visibility: "list"` models in the server’s order, and uses the selected account-visible slug with `POST /v1/responses`, `store: false`, and `stream: true`. A response is saved as complete only after the stream reports `response.completed`; failed, incomplete, canceled, or truncated output is not saved. The [accounts and sessions guide](https://developers.openai.com/siwc/token-sharing-open-source/profiles-and-sessions) informs the account-bound local session handling.
- Local chat routes reject mismatched loopback Host/Origin headers and never reflect raw upstream errors. Request, model catalog, event, response, history, and time limits are bounded. When an account is disconnected or changed during a response, the final account-generation check prevents the old stream from persisting into history. Clear/delete operations advance the history revision so a response started earlier cannot recreate deleted conversations.
- Use `127.0.0.1`, not `localhost`, so the browser origin matches the documented loopback callback. Do not expose this development server to a network or change callback host based on incoming `Host`/forwarded headers.

The next alpha task can add real streamed chat and local history without weakening this boundary.
