# Local Alpha: Click Dot

## Objective

Build Click Dot into an independent, locally runnable assistant. Each installation must let its user authorize their own ChatGPT account in a browser, without an API key or ambient Codex credentials, then use the granted ChatGPT plan for real streamed responses and local conversation history.

## Problem and Why

`click-dot` is an independent product, not an extension of Click AI DevKit, Gentle Dot, or Gentle Shell. The user has explicitly selected per-install browser authorization to each installer’s own ChatGPT account, with no API key as the primary path. The prior demo-only scope is superseded. Current priority is an authentic local account connection, followed by real streaming chat and history; demo responses must never masquerade as model output.

## Scope and Constraints

- New repository: `projects/click/click-dot`; branch: `feat/local-alpha`.
- Keep Click Dot independent from `click-ai-devkit`; do not edit sibling repositories.
- Keep the Next.js app local-first. Do not use Eve 0.77.0 `chatgpt()` as a substitute for the selected auth flow: it consumes Codex/Eve local credentials rather than registering an independent SIWC client for this app.
- Disable agent tools; do not add shell, filesystem, browser-control, or other OS capabilities.
- Use OpenAI’s documented Sign in with ChatGPT local/open-source plan-usage flow. Require the explicit `chatgpt.tokens.use.direct` grant before enabling model requests; identity-only sign-in is not model authorization.
- Keep access/refresh/ID tokens and issued client IDs in protected server-side local storage only. Never expose them to browser code, browser storage, logs, analytics, or committed files. Do not discover or reuse ambient Codex credentials.
- Serialize local credential-store mutations across concurrent requests and Click Dot processes using a private atomic lock directory with bounded acquisition. Never reclaim a held/stale lock automatically; status reads do not create the account directory. Reject unsafe parent/file/lock paths, symlinks, unexpected owners, hard links, and permissive storage modes.
- The user authorized implementation of local account connection and real chat. This work does not authorize conducting a real login, using an account, or making a provider request during development; test with offline fixtures only. Remote repository, deployment, push, PR, and merge operations remain unauthorized.
- Keep this scope as a personal local alpha. Do not assume a repository license or make a future commercial/hosted product decision here.
- Do not inherit TDD settings from the unrelated Go sibling. TDD is not configured; use ordinary functional checks with `npm test` (`node --test`), `npm run typecheck`, and `npm run build`.
- Historical initial authored baseline inventory: 399 lines including this tracker, excluding `package-lock.json`, generated `AGENTS.md`, and ignored `tsconfig.tsbuildinfo`. This is not a future diff estimate; count actual changes for each work unit and do not omit security checks or tests to meet a forecast.
- Delivery strategy: `ask-on-risk`, resolved with the user's explicit chain choice below. No remote work is authorized.
- Chain strategy: `feature-branch-chain` (user selected). Accumulate local work units on tracker branch `feat/local-alpha`; if remote delivery is later authorized, the tracker branch is the integration target and each child slice targets its immediate parent. This records a future shape only and does not authorize creating a remote, PR, or merge.

## Authorized Scope

- Bootstrap files: `.gitignore`, `.engram/config.json`, and this task document.
- T1 may add package/framework/agent scaffold files and reproducible scripts.
- T2 may add the verified local SIWC account-connection/onboarding module, its server-side callback and protected storage, tests, and documentation. Final file boundaries depend on the SDK/source verification gate below.
- T3 may add real streamed chat, visible lifecycle/error state, and local conversation history after T2 is complete.
- Do not change files outside this new repository or perform credential, remote, deployment, or OS-tool setup.

## TDD and Route

- TDD mode: not configured for this new project; do not inherit the sibling project's Go TDD configuration.
- Test runner: Node's built-in `node --test`, exposed as `npm test`; exact check commands are finalized in `package.json`.
- Implementation route: delegated direct for the multi-file T2/T3 work units; the writer owns source preparation and implementation. Parent performs structural readback/checks.
- Delivery slicing: prioritize working per-install authentication and real chat over the previously planned visual-only enhancement. Keep each work-unit snapshot runnable and include relevant checks/docs; count actual authored diff lines rather than inferring from file size.
- SIWC implementation decision: public npm metadata returned E404 for the documented `@siwc/local` and `@siwc/react` names. The official OpenAI website guide demonstrates `jose` `createRemoteJWKSet`/`jwtVerify` for identity validation, so use a pinned, maintained `jose` package plus Node `crypto` for randomness/S256 only; no vendored SIWC code and no ad hoc JWT validation. Verified public npm metadata for `jose@6.2.12`: MIT license, published tarball; exact version is installed and locked. OpenAI SIWC docs remain protocol authority. This avoids ambient Eve/Codex credentials, with the tradeoff that Click Dot owns protocol/storage code and needs extensive offline tests and maintenance.
- T2 first-unit forecast before source writes: approximately 500–650 authored changed lines across protected storage, PKCE/authorization/token/identity modules, local Next start/callback/status/disconnect handlers, onboarding UI/copy, offline tests, and README. This is an estimate from required security cases, not a cap; do not drop tests or security handling to meet 400 lines. Recount actual diff after implementation. Use the canonical callback `http://127.0.0.1:${PORT-or-3000}/auth/callback`; never derive its origin from browser `Host`/forwarded headers. Local browser access through a remote-dev tunnel needs callback-port forwarding; local installs browse and run the server on the same machine. Next development logs incoming request URLs by default; exclude `/auth/callback` request logging before using real authorization codes.

## Tasks

### T1 — Create isolated Next.js/eve scaffold

- [x] Create a minimal Next.js + eve project with a browser entry point and a local development command.
- [x] Disable default and parent-callable tools; provide no OS integrations, remote channels, or credential discovery.
- [x] Keep the page disconnected from model/provider credentials and document the explicit not-connected boundary.
- [x] Add reproducible test, typecheck, and build scripts; pin direct dependency versions in `package-lock.json`.
- [x] Add focused baseline tests and document local run/test commands.

**Acceptance:** a clean install can run the local application without prompting for login or making a paid model call; agent tools are disabled; no browser auth or eve runtime routes are exposed in this scaffold; documented setup does not expose secrets.

**Checks:** Current 45-line baseline: `npm test` PASS (three named assertions; runner summary counts the test file as one), `npm run typecheck` PASS, `npm run build` PASS (static `/` and `/_not-found`). Earlier HTTP smoke returned 200 but checked the previous “No model configured” wording; no HTTP smoke or visual/browser inspection was run on this baseline. Dependency advisory lookup was not run.

**Route evidence:** delegated direct; multi-file scaffold and preparation read.

**Rollback boundary:** remove only the new framework/package/agent scaffold files and their scripts; preserve the repository bootstrap and T2 work if independently present.

### T2 — Connect each local installation to its own ChatGPT account

- [x] Pin `jose@6.2.12` and implement the documented public-client SIWC flow with Node crypto for secure randomness and S256 PKCE, and jose for JWKS-backed ID-token verification. No client secret, API key, Eve/Codex ambient login, or vendored SDK.
- [x] Keep one stable per-install `ext_agent_host_id`, a consistent actual app `agent_name_hint`, and each account’s issued `client_id`/tokens isolated. Never persist `dynamic_agent_client` as an issued ID or use ambient Codex auth.
- [x] Provide a clear “Continue with ChatGPT” onboarding and distinguish signed-in identity from granted ChatGPT plan usage. Do not enable inference unless the token response grants `chatgpt.tokens.use.direct`.
- [x] Handle state/nonce and callback errors, account mismatch, scope denial, expired/invalid grants, and missing credentials without leaking tokens. Bind each pending attempt to its callback URI, issued registration, existing verified subject, and auth generation. Store credentials only in protected server-side local storage; do not expose them to the browser.
- [x] Add offline tests with temporary storage and synthetic auth responses; add docs for local-only account connection and security boundaries. Do not perform a real sign-in/provider request as part of this work unit.

**Acceptance:** the local app has an independently registered connection for the user’s own ChatGPT account; reauthorization reuses its issued client ID and rejects account changes; callback/identity/scopes are validated with required issuer, audience, expiry, issue time, subject, nonce, and RS256 signature; denied or identity-only authorization never appears plan-authorized; secrets remain server-side and protected; cross-process mutations serialize and disconnect invalidates pending callbacks; status reads are side-effect free; stale locks fail closed for manual recovery; all agent tools remain disabled. Real provider smoke testing is not included without a separate explicit account-use authorization.

**Checks:** `npm test` PASS (synthetic RS256 tokens; missing `exp`/`iat`/`sub`, wrong issuer/audience/nonce/signature/algorithm, issued-client reuse, same-account reconnect, account-change denial, no-secret exchange, concurrent consume/create/save/clear, two independent subprocesses sharing one auth file with no lost attempts, lock timeout that leaves the holder untouched and explicit recovery, unsafe lock symlink rejection, private path rejection, read-only status, and auth routes); `npm run typecheck` PASS; `NEXT_BUILD_DIST_DIR=.next-auth-build npm run build` PASS (isolated output removed afterward); `git diff --check` PASS. The live account/provider flow is intentionally not run. Task closure remains pending parent readback/review and work-unit commit.

**Route evidence:** delegated direct; source research and multi-file auth behavior. User authorized local implementation and selected per-installer ChatGPT browser authorization.

**Rollback boundary:** remove only the SIWC connection/callback/storage/onboarding files and dependency added for them; leave T1’s runnable scaffold intact.

### T3 — Deliver real streamed chat and local history

- [ ] Connect the UI only to an authenticated, scope-authorized local ChatGPT session; retain the no-tools boundary.
- [ ] Render genuine incremental assistant output and explicit submitted/streaming/complete/error states; never present test fixtures or demo text as provider success.
- [ ] Persist and restore local conversation history without exposing credentials to browser storage; let the user start/clear conversations.
- [ ] Add offline tests for stream projection, persistence, recovery, and error paths; document how to sign in and run locally.

**Acceptance:** a locally signed-in user can send a message, see genuine streamed output and status, then reload and recover history. Unauthenticated, scope-denied, and failed requests show honest errors and never fabricate an assistant response. No OS tools, remote hosting, or credential reuse are enabled.

**Checks:** pending T3. Planned checks: `npm test`, `npm run typecheck`, `npm run build`; a real provider smoke requires a separate explicit authorization and is not implied by offline tests.

**Route evidence:** delegated direct; multi-file UI, stream, and persistence behavior.

**Rollback boundary:** remove only the authenticated chat UI, stream handling, local conversation storage, and associated tests/docs; preserve T1 and T2 account connection.

## Progress and Verification

- Parent and independent verifier confirmed the two initial-attempt account race and cross-process storage regressions are fixed. `npm test` and `npm run typecheck` pass; the writer's isolated production build passes. Real OpenAI sign-in is untested. The authentication slice is 1,179 authored changed lines excluding the lockfile and pre-existing generated `next-env.d.ts` change; keep its security behavior and regression tests together rather than removing proof to meet a planning heuristic.

- [x] Created a new git repository on `feat/local-alpha`.
- [x] Added `.gitignore` and `.engram/config.json` for Click Dot.
- [x] Created this tracker before any application source write.
- [ ] Engram mirror under `odd/local-alpha/tasks` — pending; no mirror write has been confirmed yet.
- [x] T1 implementation, checks, and local commit completed: `038944955adfebaa619d572c3a89ac0e35a32cce`.
- [ ] T2 implementation and focused checks are complete; final closure requires parent readback/native review and a recorded work-unit commit. No real login/provider call was made. Local disconnect removes local credentials; remote revocation remains available through ChatGPT account settings. Engram mirror remains pending (`unknown_project`).
- [ ] T3 real chat/history pending T2.
- [x] Baseline committed on `feat/local-alpha`; no remote is configured or authorized.

### Baseline review evidence

- Native review: medium risk, one reliability lens, explicit candidate consent granted.
- Approved and acknowledged under `review-fb09125bb7034e05`; authority burned. Committed tree `897a5bed6997cbe06b44a5d1c2b071c1186a7bb2` matches the reviewed tree.
- Non-blocking advisory R3-001: launcher tests inspect source text. Follow-up subprocess tests should verify arguments, telemetry environment, signals, and exit propagation; these behaviors are not yet proven by runtime tests.
- Review did not establish dependency-lock resolution correctness or run commands. Functional test/build evidence remains the separate writer/parent checks above.

## Delivery Slices

Counts below distinguish historical inventory from future authored additions plus deletions. The historical 399-line total excludes `package-lock.json`, generated `AGENTS.md`, and ignored `tsconfig.tsbuildinfo`; it does not forecast the auth/chat work.

1. **Runnable functional baseline — 399 authored file lines in the current snapshot.** Includes package/config/agent scaffold, `app/layout.tsx`, `app/page.tsx`, a 45-line basic `app/globals.css`, tests, README, and this tracker. It remains runnable by itself; `npm test`, `npm run typecheck`, and `npm run build` pass. This inventory is not a future diff estimate.
2. **Visual enhancement — deferred follow-up.** The earlier styling-only slice is postponed until authenticated chat works. Its existing saved stylesheet reference is historical context only; do not restore it ahead of the user’s newly selected functional priority.

Dependency: T1 runnable baseline → T2 trusted local ChatGPT connection → T3 real streaming chat/history → optional visual enhancement. The user selected `feature-branch-chain`; local work units stay on `feat/local-alpha`, while remote/PR/merge remain unauthorized. About 400 authored changed lines is a planning heuristic, not a cap; report actual counts and preserve the cohesive behavior/tests/docs.

## Next Step

T2 auth corrections (including cross-process storage locking) and offline checks are complete. Keep T2 open until parent readback/native review and a work-unit commit are recorded; do not implement T3 until parent directs it. If a stale lock remains after a crash, stop all Click Dot processes using that file and manually `rmdir` the empty `.lock` directory; the app never reclaims one by PID or age. Keep the Engram mirror pending (`unknown_project`); no mirror write is confirmed. Preserve the pre-existing generated `next-env.d.ts` dev-path change. No real login/provider request was run. The launcher-test advisory remains a separate follow-up, not a reopened review.
