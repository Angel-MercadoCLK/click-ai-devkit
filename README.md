# Click Dot local alpha

Click Dot is an independent local-first assistant prototype. Each installation connects its user’s own ChatGPT account through Sign in with ChatGPT; it does not use an API key or the machine’s Codex login. No chat or agent tools are enabled in this alpha.

## Quick start

Requirements: Node.js 24 or newer.

```sh
npm install
npm run dev
```

Open `http://127.0.0.1:3000` on the same machine running Click Dot. Choose **Continue with ChatGPT** to authorize this installation in your browser. The callback uses that same loopback host and port. A remote development tunnel must forward the callback port to the browser machine; this is not a hosted service.

Click Dot requests identity and direct ChatGPT plan-usage scopes. Signing in without the `chatgpt.tokens.use.direct` grant is not considered model access. This unit establishes and displays the account connection; chat is a later task.

## Checks

```sh
npm test
npm run typecheck
npm run build
```

`npm start` serves a production build on loopback. The development and production scripts set `NEXT_TELEMETRY_DISABLED=1`.

## Account and runtime boundary

- The browser page is not connected to the eve agent runtime. No `/eve/v1` routes or agent tools are exposed.
- The eve definition has no enabled default or parent-callable tools, and this scaffold declares no tools, connections, channels, subagents, or skills.
- Tokens and the issued local application registration are stored under `~/.config/click-dot/auth.json` with owner-only permissions. They are not sent to browser storage or exposed by the status route. A private lock directory serializes updates across local Click Dot processes. If a process crashes while holding the lock, Click Dot fails closed rather than reclaiming it based on age or PID. Only after stopping every Click Dot process that uses this auth file, manually remove the empty `~/.config/click-dot/auth.json.lock` directory with `rmdir`; do not remove a lock while a server may still hold it. Storage fails closed on native Windows until private ACLs can be verified; use WSL2/Linux for this alpha.
- Disconnect removes Click Dot’s local credentials. To revoke the app remotely, use the connected ChatGPT account’s app settings.
- The callback binds state to a short-lived, one-use browser attempt and validates ID-token signature, issuer, audience, expiry, and nonce using OpenAI’s published signing keys. Development request logging excludes the callback URL because it contains a short-lived authorization code. Offline tests use synthetic credentials and do not contact OpenAI.
- Use `127.0.0.1`, not `localhost`, so the browser origin matches the documented loopback callback. Do not expose this development server to a network or change callback host based on incoming `Host`/forwarded headers.

The next alpha task can add real streamed chat and local history without weakening this boundary.
