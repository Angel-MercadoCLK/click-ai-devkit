# Click Dot local alpha

Click Dot is an independent local browser-app scaffold. T1 provides an honest demo boundary; it does not yet include chat, history, provider settings, or a live model connection.

## Quick start

Requirements: Node.js 24 or newer.

```sh
npm install
npm run dev
```

Open `http://127.0.0.1:3000`. The page shows **Demo · Not connected**. The Next.js app runs on loopback, disables telemetry, and makes no model requests.

## Checks

```sh
npm test
npm run typecheck
npm run build
```

`npm start` serves a production build on loopback. The development and production scripts set `NEXT_TELEMETRY_DISABLED=1`.

## Runtime boundary

- The browser page is not connected to the eve agent runtime. No `/eve/v1` routes or provider settings are exposed in this task.
- The eve definition has no enabled default or parent-callable tools, and this scaffold declares no tools, connections, channels, subagents, or skills.
- No provider credential is needed or collected. Do not add credentials to browser code or local storage.
- This is a preview scaffold, not a production deployment or authenticated service.

The next alpha task can add a local-only demo chat and non-secret settings without weakening this boundary.
