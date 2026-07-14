---
name: apps-ionic-angular
description: >
  Operational defaults for Ionic + Angular application work.
  Trigger: When implementing or modifying Ionic Angular pages, routing, state, navigation, or lifecycle behavior.
license: Apache-2.0
allowed-tools: Read Write Edit Bash
metadata:
  author: Zesh-One
  version: "2.0"
  inspired-by: capawesome-team/ionic-angular, ionic-capacitor/angular.md
---

## When to Use

Use this skill for APP-level Ionic Angular work, not for generic Angular tutorials.

- Routed pages and tab flows.
- Navigation transitions and back-stack behavior.
- Page lifecycle data refresh and cached-view behavior.
- App state with Angular signals.
- Standalone Ionic architecture decisions.

If your task is mostly native plugin behavior, also load `../capacitor/SKILL.md`.

## Critical Patterns

Follow these as atomic rules. Each rule has intent (`Do/Don't`) and rationale (`Why`).

1. **Do** use standalone Ionic imports (`@ionic/angular/standalone`) for new code.
   **Don't** introduce new NgModule-only patterns.
   **Why:** Keeps architecture aligned with project standard and avoids mixed mental models.

2. **Do** wrap every routed page template with `<ion-page>`.
   **Don't** mount routed content directly under `<ion-content>`.
   **Why:** Ionic transitions, safe-area handling, and lifecycle expectations depend on page shell semantics.

3. **Do** refresh page data in `ionViewWillEnter`.
   **Don't** rely on `ngOnInit` for revisits.
   **Why:** Ionic keeps pages cached in the DOM; `ngOnInit` is not a revisit hook.

4. **Do** use `NavController` (`navigateForward`, `back`, `navigateRoot`) for user-facing transitions.
   **Don't** default to `Router.navigate` for animated app navigation.
   **Why:** NavController preserves Ionic animation and stack expectations across platforms.

5. **Do** model app/session state with Angular signals (`signal`, `computed`, readonly exposure).
   **Don't** spread mutable state across unrelated services and components.
   **Why:** Signals give explicit reactive dependencies and simpler change propagation.

6. **Do** keep presentational components dumb and `OnPush`.
   **Don't** make UI leaf components fetch or mutate domain state.
   **Why:** Improves testability, composability, and rendering predictability.

7. **Do** lazy-load routed pages with `loadComponent`.
   **Don't** eagerly import page components into route roots.
   **Why:** Reduces startup cost and keeps mobile bundle pressure under control.

8. **Do** align tab button `tab` values with child route paths exactly.
   **Don't** depend on aliases or mismatched names.
   **Why:** Tab router outlets resolve by exact key/path matching.

9. **Do** register only needed Ionicons explicitly in standalone pages/components.
   **Don't** assume global icon availability.
   **Why:** Standalone usage requires explicit registration and avoids accidental bundle growth.

10. **Do** place page-entry fetch logic behind idempotent methods (safe on every enter).
    **Don't** couple data load to constructor side effects.
    **Why:** Entry hooks may run many times; deterministic loaders avoid stale/multi-subscribe bugs.

## Constraints & Tradeoffs

- **Constraint:** Standalone-first is mandatory.
  **Tradeoff:** Some older examples/docs use `IonicModule`; adapt them instead of copying verbatim.

- **Constraint:** `ionViewWillEnter` for refresh is mandatory on routed pages.
  **Tradeoff:** Extra network calls can occur; use caching/debouncing intentionally rather than skipping refresh.

- **Constraint:** NavController is default for animated transitions.
  **Tradeoff:** Router APIs are still valid for guards/url trees; choose based on navigation intent.

- **Constraint:** Signals are preferred for app state.
  **Tradeoff:** Interop with RxJS-heavy services may require bridge points (`toSignal`, explicit effect boundaries).

- **Constraint:** Skill content must remain operational, not encyclopedic.
  **Tradeoff:** Keep only decision-driving guidance; move deep explanation to external docs.

## Anti-Patterns

- Missing `<ion-page>` in any routed page.
- Data load in constructor/`ngOnInit` expecting automatic revisit refresh.
- Programmatic navigation everywhere via `Router.navigate` causing non-Ionic transitions.
- Two-way mutable shared state without signal ownership boundaries.
- Presentational component importing HTTP services directly.
- Route eager imports that bloat initial chunk.
- Tab buttons whose `tab` key does not match route path.
- Copy-pasted NgModule setup into standalone app root.
- Shipping long onboarding prose inside this skill file.

## Progressive Disclosure

Start minimal, then expand only if the task demands it.

1. **Baseline (always):** page shell (`<ion-page>`), entry refresh (`ionViewWillEnter`), NavController for transitions.
2. **State step:** add signals for page/app state ownership and derived values.
3. **Architecture step:** split smart/presentational components and enforce OnPush at UI edges.
4. **Performance step:** audit lazy routes, icon registration scope, and repeated entry fetch cost.
5. **Advanced only when needed:** tabs edge cases, modal routing nuances, deep-link restoration.

Short non-obvious reference snippet:

```typescript
import { ViewWillEnter } from '@ionic/angular/standalone';

export class OrdersPage implements ViewWillEnter {
  ionViewWillEnter(): void {
    this.refreshOrders();
  }
}
```

## Resources

- Ionic Angular lifecycle: https://ionicframework.com/docs/angular/lifecycle
- Ionic Angular navigation: https://ionicframework.com/docs/angular/navigation
- Angular signals guide: https://angular.dev/guide/signals
- Companion native guidance: ../capacitor/SKILL.md

## Changelog

### v2.0 — 2026-04-21
- Rewritten into concise operational guidance for Ionic Angular application work.
- Enforced standalone imports, `ionViewWillEnter` refresh, NavController transitions, and signals-first state defaults.
