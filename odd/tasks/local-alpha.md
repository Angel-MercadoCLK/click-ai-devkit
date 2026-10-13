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

- [x] T3 implementation forecast: approximately 800–1,100 authored changed lines across the server-only SIWC transport/catalog, protected account-scoped history and guarded local routes, streaming chat UI, offline tests, and setup/security documentation. Actual authored diff is 1,355 lines excluding the pre-existing `next-env.d.ts` dev-path change; the cohesive offline coverage and security behavior were kept intact rather than reduced to meet the estimate.
- [x] User reports that browser sign-in works for their account. This is user-reported manual evidence only; the implementation agent did not perform or independently observe the sign-in, and no real inference/provider request has been authorized for development verification.
- [x] Keep the transport on the explicitly selected SIWC OAuth path (no API-key onboarding, API key inspection, or ambient credentials); use the current stored OAuth access token only when direct plan usage is granted and the token is unexpired. This slice deliberately requires explicit reauthorization after expiry; refresh-token rotation is out of scope.
- [x] Route: one bounded single-writer delegated-direct implementation (the assigned worker owns source preparation and implementation); no additional writer or reviewer delegation from the worker. Functional checks use offline fixtures and temporary protected storage only; all model/provider calls are user-triggered in the local app, never from rendering, build, or tests.
- [x] Functional verification: `npm test` PASS (3 test files, 0 failures; chat coverage includes split/multiline SSE and size/event limits), `npm run typecheck` PASS, `NEXT_BUILD_DIST_DIR=.next-chat-build npm run build` PASS, and `git diff --check` PASS. Offline coverage exercises local-origin/auth guards, bounded inputs and allowed model validation, account isolation, atomic history updates/clear races, SSE terminal success/failure/EOF, provider disconnect, local response-reader cancellation and incoming request abort (both abort upstream and save no turn), and expired/no-scope authorization. Real account or inference proof remains explicitly out of scope.
- [x] Rollback remains limited to T3 chat UI, SIWC model/response transport, chat/history routes and protected history files, tests, and documentation; preserve the T1 scaffold and T2 account-connection behavior.
- [x] Record source basis in README: [SIWC models and inference](https://developers.openai.com/siwc/token-sharing-open-source/models-and-inference) and [SIWC accounts and sessions](https://developers.openai.com/siwc/token-sharing-open-source/profiles-and-sessions). The documented flow lists the signed-in account’s models from `/v1/models`, filters `visibility: "list"`, preserves order, and streams `/v1/responses` with `store: false`; inference is successful only after `response.completed`.

- [x] Connect the UI only to an authenticated, scope-authorized local ChatGPT session; retain the no-tools boundary.
- [x] Render genuine incremental assistant output and explicit submitted/streaming/complete/error states; never present test fixtures or demo text as provider success.
- [x] Persist and restore local conversation history without exposing credentials to browser storage; let the user start/clear conversations.
- [x] Add offline tests for stream projection, persistence, recovery, and error paths; document how to sign in and run locally.

**Acceptance:** a locally signed-in user can send a message, see genuine streamed output and status, then reload and recover history. Unauthenticated, scope-denied, and failed requests show honest errors and never fabricate an assistant response. No OS tools, remote hosting, or credential reuse are enabled.

**Checks:** `npm test` PASS (3 files, 0 failures); `npm run typecheck` PASS after restoring the pre-existing `.next/dev/types` imports; `NEXT_BUILD_DIST_DIR=.next-chat-build npm run build` PASS (the isolated `.next-chat-build` output was removed and generated edits to `tsconfig.json`/`next-env.d.ts` were reverted to their pre-build baseline); `git diff --check` PASS. Tests use offline fakes and temporary protected storage. No real account or inference request was made.

**Route evidence:** delegated direct; multi-file UI, stream, and persistence behavior. One bounded writer owns source preparation and implementation; offline tests/build are isolated from the running preview and use fixtures/temp storage only.

**Rollback boundary:** remove only the authenticated chat UI, stream handling, local conversation storage, and associated tests/docs; preserve T1 and T2 account connection.

### T4 — Raise the bounded model-catalog response limit

- [x] Fix catalog parsing for valid account model-list responses larger than the old 512,000-byte cap. The user-provided upstream response measured 614,466 bytes; offline reproduction used valid JSON at 626,192 bytes.
- [x] Raise only the model-catalog body bound to 2 MiB; preserve the 12-second timeout, model filtering/order/projection, 500-model output bound, and credential boundary.
- [x] Add offline regressions proving valid oversized-for-old-limit catalog JSON is accepted, UTF-8 byte length (not character count) controls the limit, and bodies over 2 MiB are rejected with the reader canceled. Add a focused route test for the previously blocked/no-model outcome where practical.
- [x] Do not use real credentials, make provider/catalog calls, mutate auth or conversation history, or expand UI/auth behavior. Parent may run a separately authorized one-shot catalog check after source review; this work itself does not claim live-fix verification.
- [x] TDD is not configured; use ordinary offline functional tests. Route: delegated direct, single writer, since the bounded fix touches non-trivial transport and tests. Forecast: under 150 authored additions plus deletions. Required checks: `npm test`, `npm run typecheck`, and `git diff --check`; production build may be skipped because the transport response shape is unchanged.
- [ ] Engram mirror `odd/local-alpha/tasks` remains pending until `click-dot` is recognized; preserve this local tracker as the source of progress meanwhile.

**Acceptance:** valid UTF-8 JSON catalogs up to 2 MiB can be decoded and retain existing model filtering/order/output bounds; a response exceeding 2 MiB fails closed and its body reader is canceled.

**Checks:** `npm test` PASS (3 files, 0 failures; focused catalog tests cover a chunked 626,192-byte valid JSON catalog, listed-model order/filtering, multibyte UTF-8 byte-bound rejection, reader cancellation above 2 MiB, and the local models route returning 200); `npm run typecheck` PASS; `git diff --check` PASS. Actual authored diff is 79 lines excluding the pre-existing `next-env.d.ts` dev-path change. Production build intentionally skipped because only the transport response-size bound and offline tests changed. No real credentials or provider/catalog call were used; live fix verification remains pending parent’s separately authorized one-shot check.

**Rollback boundary:** revert only the model catalog response-size constant, its regression tests, and this T4 tracker record; preserve the streamed chat/auth behavior and existing history.

## Progress and Verification

- Parent and independent verifier confirmed the two initial-attempt account race and cross-process storage regressions are fixed. `npm test` and `npm run typecheck` pass; the writer's isolated production build passes. Real OpenAI sign-in is untested. Authentication work-unit commit `253279ec1c7bfd44eeb0b1873a83472ca9834561` contains 1,181 authored changed lines excluding the lockfile and pre-existing generated `next-env.d.ts` change; keep its security behavior and regression tests together rather than removing proof to meet a planning heuristic. Native review is approved and acknowledged; no push, PR, or merge was performed.

- [x] Created a new git repository on `feat/local-alpha`.
- [x] Added `.gitignore` and `.engram/config.json` for Click Dot.
- [x] Created this tracker before any application source write.
- [ ] Engram mirror under `odd/local-alpha/tasks` — pending; no mirror write has been confirmed yet.
- [x] T1 implementation, checks, and local commit completed: `038944955adfebaa619d572c3a89ac0e35a32cce`.
- [x] T2 implementation, offline checks, work-unit commit, and native review are complete. Local disconnect removes local credentials; remote revocation remains available through ChatGPT account settings. Engram mirror remains pending (`unknown_project`).
- [x] User reports successful browser sign-in; the implementation agent did not independently observe it. No real inference request was run.
- [x] T3 source, offline checks, parent readback, and work-unit commit completed: `cfcab71df5f016e04fe3d733dfb1c7452da76a36` (1,355 authored changed lines). Native review decision and user-run live inference remain pending. No push, PR, merge, or real provider request was performed.
- [x] Independent cancellation reproduction now observes `abortAtCancel: true` and `historyCount: 0` (previously `false` and `1`). Parent reran `npm test`: all 3 test files passed. Independent verifier also reran typecheck successfully.
- [x] T4 offline catalog-size fix raises the response-body bound from 512,000 bytes to 2 MiB, preserving the existing timeout/filter/order/projection behavior. Valid chunked 626,192-byte JSON now reaches the local models route; UTF-8 payloads over 2 MiB are rejected and canceled. `npm test`, `npm run typecheck`, and `git diff --check` passed; no production build or live catalog request was run.
- [x] Baseline committed on `feat/local-alpha`; no remote is configured or authorized.

### Baseline review evidence

- Native review: medium risk, one reliability lens, explicit candidate consent granted.
- Approved and acknowledged under `review-fb09125bb7034e05`; authority burned. Committed tree `897a5bed6997cbe06b44a5d1c2b071c1186a7bb2` matches the reviewed tree.
- Non-blocking advisory R3-001: launcher tests inspect source text. Follow-up subprocess tests should verify arguments, telemetry environment, signals, and exit propagation; these behaviors are not yet proven by runtime tests.
- Review did not establish dependency-lock resolution correctness or run commands. Functional test/build evidence remains the separate writer/parent checks above.

## Delivery Slices

### Chat review evidence

- The user explicitly granted four-lens review of the committed chat/history slice through `e125da6`. Final capture approved without blocking findings or a correction. Exact acknowledgement returned `authority: burned` for lineage `review-6392fdfb351d9fb6`, target `sha256:e289640a295b5f78b05b459626dda3bcb4f61c2f4f2bd4631913e6721faef527`, and candidate tree `374881d0b7d8d08b04a6ef35ceba51f0b5a40459`. This review is terminal.
- Four distinct non-blocking follow-ups remain, not fixed by this review: isolate mutable test history fixtures (`R2-001`); recover the busy UI when clearing history fails during streaming (`R3-clear-failure-stuck-busy`); refresh stale history after a revision conflict across tabs (`R3-stale-history-recovery`, `R4-stale-history-recovery`); check known history capacity before consuming inference (`R4-capacity-check-after-inference`). They do not reopen this candidate or authorize an automatic correction.
- Reviewers inspected immutable patches, not live services. Previously recorded functional checks remain separate evidence. No account credentials were inspected and no real inference was requested. Next: user-run model loading, a short message, and reload/history verification in the local preview; report failures honestly. Prefer a single browser tab for this alpha, and reload if a history revision conflict occurs.

### Authentication review evidence

- Explicitly granted four-lens native review approved the committed authentication slice through `f0edac9`; exact acknowledgement returned `authority: burned` for lineage `review-7e2000f520561103` and target `sha256:d7e3ee5ddaba4e4516b0d5fff58742f2756e100229202db2ab6e2039e3219fb4`. Reviewed tree: `f1d6e81206a9316a2043638a1e4519eb6613c307`.
- No blocking finding or correction was opened. Three distinct non-blocking follow-up topics remain: isolate test environment/storage fixtures (`R2-001`); handle a status read racing an atomic credential replacement (`R3-001`, `R4-status-read-race`); recover from a retained initial registration after failed token exchange (`R3-002`). These advisories do not reopen this review and have not been implemented.
- Reviewers inspected immutable patches; they did not run tests, perform dependency scanning, or verify a live sign-in. Existing functional checks remain separate evidence.

Counts below distinguish historical inventory from future authored additions plus deletions. The historical 399-line total excludes `package-lock.json`, generated `AGENTS.md`, and ignored `tsconfig.tsbuildinfo`; it does not forecast the auth/chat work.

1. **Runnable functional baseline — 399 authored file lines in the current snapshot.** Includes package/config/agent scaffold, `app/layout.tsx`, `app/page.tsx`, a 45-line basic `app/globals.css`, tests, README, and this tracker. It remains runnable by itself; `npm test`, `npm run typecheck`, and `npm run build` pass. This inventory is not a future diff estimate.
2. **Visual enhancement — deferred follow-up.** The earlier styling-only slice is postponed until authenticated chat works. Its existing saved stylesheet reference is historical context only; do not restore it ahead of the user’s newly selected functional priority.

Dependency: T1 runnable baseline → T2 trusted local ChatGPT connection → T3 real streaming chat/history → optional visual enhancement. The user selected `feature-branch-chain`; local work units stay on `feat/local-alpha`, while remote/PR/merge remain unauthorized. About 400 authored changed lines is a planning heuristic, not a cap; report actual counts and preserve the cohesive behavior/tests/docs.

## Next Step

T4 closure: committed as `eb911ff6fac8b1a98ea9ce81dff5bc99e8b046ff` (79 authored changed lines). Parent reran `npm test` successfully. An authorized live GET of the running preview's `/api/chat/models` returned HTTP 200, valid JSON, and 7 models in 1,143 ms; no inference was performed. Native assessment against `54be098` returned medium risk, `review_due: false`, `under_budget`; this small follow-up remains pending in the next review slice, not independently reviewed. The original T3 review remains terminal and approved. Next: user refreshes the page, loads models, selects one, and sends a short message. No further sign-in is needed unless the session expires again. The production build was intentionally skipped for this constant-and-tests fix; tests, typecheck, and diff checks passed. The Engram mirror remains pending.

T2 implementation and offline verification are committed, reviewed, and acknowledged. The user reports successful browser sign-in; the implementation agent did not independently observe it. T3 real chat/history is committed with parent and independent offline verification; native review decision and user-run live inference remain. The advisory topics above are separate follow-up work, not an automatic correction or repeated review. If a stale lock remains after a crash, stop all Click Dot processes using that file and manually `rmdir` the empty `.lock` directory; the app never reclaims one by PID or age. Keep the Engram mirror pending (`unknown_project`); no mirror write is confirmed. Preserve the pre-existing generated `next-env.d.ts` dev-path change. No real inference/provider request was run.
