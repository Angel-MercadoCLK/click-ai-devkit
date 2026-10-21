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

Click Dot can run as a Windows tray app built with Electron. It serves the production Next.js build from the same process on `http://127.0.0.1:47321` (fixed, because the Sign in with ChatGPT callback uses this port), keeps running in the tray when the window is closed, and opens with `Ctrl+Shift+Space`.

```sh
npm run desktop:build      # next build + compile the Electron main process
npm run desktop:start      # run the desktop app locally (needs a display)
npm run desktop:pack:win   # unpacked Windows x64 app in release/win-unpacked
```

- Data lives in the per-user app data folder (`auth.json`, `conversations.json`). Credentials and history are encrypted with Electron `safeStorage` (Windows DPAPI). If OS encryption is unavailable, storage stays closed.
- If port 47321 is busy, the app shows an error and quits instead of choosing another port.
- Start with Windows is on by default for packaged builds and can be turned off from the tray menu. Pass `--no-login-item` (or set `CLICK_DOT_DISABLE_LOGIN_ITEM=1`) to never register it, for example in smoke tests.
- Choosing **Continue with ChatGPT** in the desktop window opens Click Dot in the system browser; choose **Continue with ChatGPT** there to finish signing in, because the callback must reach the same browser that started it. The desktop window refreshes when you come back to it.
- Every web permission request (camera, microphone, screen, notifications) is denied in this version.

### Screen capture

- Off by default. Turn it on from the Click Dot window ("Let Dot see your screen") or from the tray ("Let Dot see my screen"). Your choice is remembered across restarts.
- While it is on, the window shows "Dot can see your screen" and the tray icon shows a red dot with the tooltip "Click Dot — seeing your screen".
- Pause at any time from the window or the tray. Pausing is for the current session only, so a restart resumes capture if it was on.
- Dot captures your main display every 10 seconds and keeps only the last 6 frames (about one minute) in memory. Frames are never written to disk and are dropped when you pause, turn capture off, or quit.
- Capture runs inside the desktop app, and web pages still get no screen, camera or microphone permission. In a plain browser or `npm run dev`, capture is unavailable. Frames are not sent anywhere yet.

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
