---
name: apps-capacitor
description: >
  Operational defaults for Capacitor native-layer work in app projects.
  Trigger: When implementing or modifying Capacitor plugins, platform behavior, native listeners, or mobile runtime integration.
license: Apache-2.0
allowed-tools: Read Write Edit Bash
metadata:
  author: Zesh-One
  version: "2.0"
  inspired-by: capawesome-team/ionic-capacitor
---

## When to Use

Use this skill when the task crosses from web app code into native runtime behavior.

- Installing and wiring Capacitor plugins.
- Platform checks (`web`, `ios`, `android`) and runtime branching.
- Native API calls (camera, push, filesystem, network, app lifecycle).
- Native listener lifecycle and cleanup.
- Syncing web/native layers after dependency changes.

For Ionic page architecture and lifecycle integration, pair with `../ionic-angular/SKILL.md`.

## Critical Patterns

Apply these as rules, not suggestions.

1. **Do** run `ionic cap sync` after every plugin install/change.
   **Don't** assume `npm install` updates native projects.
   **Why:** Native iOS/Android projects are not auto-synced by JS dependency install.

2. **Do** use `Capacitor.getPlatform()` and `Capacitor.isNativePlatform()`.
   **Don't** use user-agent sniffing (`navigator.userAgent`).
   **Why:** UA checks are brittle and often wrong in embedded webviews.

3. **Do** wrap every native plugin call in `try/catch`.
   **Don't** collapse cancellation and operational errors into one path.
   **Why:** User cancellation is expected behavior, not a crash signal.

4. **Do** normalize plugin error handling at service boundaries.
   **Don't** duplicate ad-hoc string matching in multiple components.
   **Why:** Centralized handling reduces drift and inconsistent UX.

5. **Do** register long-lived native listeners intentionally and store handles.
   **Don't** leak listeners across view re-entry or logout/login cycles.
   **Why:** Orphan listeners cause duplicate events, memory growth, and phantom side effects.

6. **Do** gate plugin execution by runtime capability (`native` vs `web`).
   **Don't** call native APIs unconditionally from shared code paths.
   **Why:** Browser runtime may not provide the same plugin behavior.

7. **Do** initialize push registration when user context is available.
   **Don't** register blindly before auth/session restoration.
   **Why:** Token association and deep-link routing usually depend on user identity.

8. **Do** treat filesystem persistence as data-domain design (path, retention, cleanup).
   **Don't** use Preferences for large payloads or file-like data.
   **Why:** Preferences are key-value settings storage, not document storage.

9. **Do** test plugin flows on real devices for release-critical features.
   **Don't** trust browser-only validation for native behavior.
   **Why:** Permissions, OS dialogs, and hardware quirks differ from emulators/browsers.

10. **Do** keep config changes explicit in `capacitor.config.*` and review security-impacting flags.
    **Don't** enable permissive flags (cleartext/mixed content) outside controlled dev scenarios.
    **Why:** Misconfigured transport/security flags can silently ship high-risk builds.

## Constraints & Tradeoffs

- **Constraint:** `ionic cap sync` after plugin changes is mandatory.
  **Tradeoff:** Adds iteration cost, but prevents mismatched native/JS state.

- **Constraint:** Platform detection must use Capacitor APIs.
  **Tradeoff:** Slightly more explicit branching, much higher correctness.

- **Constraint:** Native calls require `try/catch` and cancel/error separation.
  **Tradeoff:** More boilerplate; dramatically better UX and observability.

- **Constraint:** Listener cleanup is required.
  **Tradeoff:** Requires lifecycle discipline and handle management.

- **Constraint:** Skill remains concise operational guidance.
  **Tradeoff:** Deep plugin API coverage belongs in official docs, not here.

## Anti-Patterns

- Installing plugin packages without running `ionic cap sync`.
- UA sniffing for iOS/Android detection.
- Single generic catch block showing “unexpected error” for user cancellation.
- Creating listeners on every page enter without removing previous handles.
- Plugin calls from UI components without service-level guards and mapping.
- Using Preferences as a pseudo-database for large JSON blobs.
- Enabling insecure config flags and forgetting to revert for release.
- Assuming emulator success equals production readiness.
- Embedding long plugin tutorials in this skill file.

## Progressive Disclosure

Implement only the layer you need.

1. **Baseline (always):** install plugin, sync native projects, add runtime guard (`isNativePlatform`).
2. **Error layer:** add `try/catch` and explicit cancel-vs-error mapping.
3. **Lifecycle layer:** register listeners once, keep handles, remove on teardown.
4. **Security/config layer:** review `capacitor.config.*` flags for target environment.
5. **Production hardening:** verify on real devices with permission-denied/cancel/network-off scenarios.

Short non-obvious reference snippet:

```typescript
import { Capacitor } from '@capacitor/core';

const platform = Capacitor.getPlatform(); // 'ios' | 'android' | 'web'
const isNative = Capacitor.isNativePlatform();
```

## Resources

- Capacitor docs: https://capacitorjs.com/docs
- Capacitor config reference: https://capacitorjs.com/docs/config
- Capacitor plugin APIs index: https://capacitorjs.com/docs/apis
- Companion Ionic app guidance: ../ionic-angular/SKILL.md

## Changelog

### v2.0 — 2026-04-21
- Rewritten into concise operational guidance with explicit sync, platform, error, and lifecycle defaults.
