# Local Alpha: Click Dot

## Objective

Create the first visible, locally runnable Click Dot alpha: an independent browser-based demo with a polished responsive interface, local chat history, simple provider settings, and an unmistakable not-connected/demo boundary until a model is explicitly configured.

## Problem and Why

`click-dot` did not exist. The user wants an independent Click Dot experience rather than extending Click AI DevKit. This first slice establishes the repository and a useful local product surface without OS tools, remote operations, credential reuse, deployment, or paid model calls.

## Scope and Constraints

- New repository: `projects/click/click-dot`; branch: `feat/local-alpha`.
- Keep Click Dot independent from `click-ai-devkit`; do not edit sibling repositories.
- Use eve as the agent framework and Next.js for its browser UI.
- Disable agent tools; do not add shell, filesystem, browser-control, or other OS capabilities.
- Demo chat and history must work locally without provider credentials. Label demo responses and connection state honestly; never imply a real model response.
- Never put provider secrets in browser code, local storage, or committed files. Document environment-based configuration only.
- No login, model call, remote repository, deploy, or paid service operation is authorized in this bootstrap. Local work-unit commits on the feature branch are authorized under ODD; remote, PR, and merge operations remain unauthorized.
- Do not inherit TDD settings from the unrelated Go sibling. TDD is not configured; use ordinary functional checks with `npm test` (`node --test`), `npm run typecheck`, and `npm run build`.
- Current authored baseline: 399 lines including this tracker, excluding `package-lock.json`, generated `AGENTS.md`, and ignored `tsconfig.tsbuildinfo`. Recount after tracker edits; do not reduce scope or omit proof to fit a forecast.
- Delivery strategy: `ask-on-risk`, resolved with the user's explicit chain choice below. No remote work is authorized.
- Chain strategy: `feature-branch-chain` (user selected). Accumulate local work units on tracker branch `feat/local-alpha`; if remote delivery is later authorized, the tracker branch is the integration target and each child slice targets its immediate parent. This records a future shape only and does not authorize creating a remote, PR, or merge.

## Authorized Scope

- Bootstrap files: `.gitignore`, `.engram/config.json`, and this task document.
- T1 may add package/framework/agent scaffold files and reproducible scripts.
- T2 may add the local demo UI, history/settings behavior, tests, and user-facing documentation.
- Do not change files outside this new repository or perform credential, remote, deployment, or OS-tool setup.

## TDD and Route

- TDD mode: not configured for this new project; do not inherit the sibling project's Go TDD configuration.
- Test runner: Node's built-in `node --test`, exposed as `npm test`; exact check commands are finalized in `package.json`.
- Implementation route: delegated direct for T1 and T2. Both involve multi-file work and reading that prepares implementation, so the writer owns preparation and implementation. Parent performs structural readback/checks.
- Delivery slicing: first make the functional app snapshot runnable; defer the full visual styling to a dependent enhancement slice. Keep each snapshot runnable and include its relevant checks/docs; count actual authored diff lines rather than inferring from file size.

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

### T2 — Deliver responsive local demo chat and settings

- [ ] Build a responsive, polished Click Dot browser UI with clear Demo / Not connected labeling.
- [ ] Implement local-only chat history and basic provider/model settings that never collect or persist secrets in the browser.
- [ ] Keep demo responses visibly distinct from real agent responses; do not send requests to an unconfigured runtime.
- [ ] Add tests for history/settings behavior and docs for demo limitations and the boundary to a later configured model.

**Acceptance:** a user can open the local app, interact with a clearly labeled demo, refresh and recover local history, inspect/change non-secret settings, and tell at all times whether a real model is connected. No OS tool, credential, external service, or deployment path is active.

**Checks:** pending implementation. Planned checks: `npm test`, `npm run typecheck`, `npm run build`, plus local dev/browser smoke where tooling permits. Record exact outcomes and any unavailable check.

**Route evidence:** delegated direct; multi-file UI and behavior work.

**Rollback boundary:** remove only the chat/settings UI, local persistence behavior, and their tests/docs; leave T1's runnable scaffold intact.

## Progress and Verification

- [x] Created a new git repository on `feat/local-alpha`.
- [x] Added `.gitignore` and `.engram/config.json` for Click Dot.
- [x] Created this tracker before any application source write.
- [ ] Engram mirror under `odd/local-alpha/tasks` — pending; no mirror write has been confirmed yet.
- [x] T1 implementation and checks completed; commit remains parent-managed and pending.
- [ ] T2 implementation and checks pending.
- [ ] No commits created yet; next step is a local T1 work-unit commit on `feat/local-alpha`. No remote is configured or authorized.

## Delivery Slices

Counts below distinguish the current file-line inventory from future authored additions plus deletions. The current 399-line total excludes `package-lock.json`, generated `AGENTS.md`, and ignored `tsconfig.tsbuildinfo`; obtain the visual-enhancement change count from the actual future diff.

1. **Runnable functional baseline — 399 authored file lines in the current snapshot.** Includes package/config/agent scaffold, `app/layout.tsx`, `app/page.tsx`, a 45-line basic `app/globals.css`, tests, README, and this tracker. It remains runnable by itself; `npm test`, `npm run typecheck`, and `npm run build` pass. This inventory is not a future diff estimate.
2. **Visual enhancement — dependent follow-up.** Restore the original 230-line stylesheet preserved at `/tmp/click-dot-full-alpha.css` (SHA-256 `8edfa901e25b45146d5a1cf6d046645096425347baf3b8e7ff203a34f52c2c89`) and add focused visual documentation/checks. Determine the authored additions plus deletions from the actual diff against the baseline before committing; do not infer count from final file size.

Dependency: runnable functional baseline → visual enhancement → T2 local chat/history/settings (future task, not started). The user selected `feature-branch-chain`; local work units stay on `feat/local-alpha`, while remote/PR/merge remain unauthorized. If the actual cohesive slice exceeds 400 authored changes, report the exact count and required exception rather than shrinking it.

## Next Step

Parent readback and local work-unit commit for the runnable functional baseline are pending; visual enhancement is the dependent next slice, then T2. Keep the Engram mirror pending (`unknown_project`); do not retry until the project is write-enabled. Baseline checks pass; no HTTP smoke or browser visual inspection was run after reducing the stylesheet.
