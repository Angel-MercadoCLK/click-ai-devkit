---
name: frontend-tailwind-4
description: >
  ZeshOne Tailwind CSS 4 conventions.
  Trigger: When styling UI with Tailwind utilities, semantic tokens, responsive rules, or runtime dynamic values.
license: Apache-2.0
allowed-tools: Read Write Edit Bash
metadata:
  author: Zesh-One
  version: "2.2"
  inspired-by: gentleman-programming/tailwind-4, frontend-nextjs
---

## When to Use

Load this skill when styling with Tailwind CSS 4 in this repo.

Use it to enforce utility-first classes, semantic defaults, and safe handling of truly dynamic values.

Cross-reference:
- `frontend/shadcn-ui`: composition and primitives strategy.
- `frontend/nextjs-15`: server/client boundaries that affect where styles execute.

## Critical Patterns

- DO use semantic utility classes (`bg-background`, `text-foreground`, `border-border`) — Why: keeps token mapping centralized and theme-safe.
- DO use utility-first `className` as default — Why: consistent with project styling standards and easy to review.
- DO use `cn()` only when classes are conditional or merged — Why: reduces noise in static components.
- DO keep mobile as the base and layer up with breakpoints (`md:`, `lg:`) — Why: predictable responsive behavior.
- DO use container queries for component-level adaptation (`@container`, `@sm:`) — Why: reusable components should respond to parent width, not page width.
- DO prefer `rem` and `clamp()` for typography/spacing tokens — Why: accessibility and stable zoom behavior.
- DO use `style` only for truly runtime values (e.g., width from API/user input) — Why: static values belong in classes.
- DON'T place `var(--token)` directly in `className` — Why: Tailwind cannot optimize arbitrary variable strings reliably.
- DON'T use raw hex/rgb color literals in markup — Why: breaks semantic theming and dark mode consistency.
- DO follow this skill's current Tailwind 4 patterns; DON'T use legacy/deprecated Tailwind APIs from older versions; DO verify exact-version syntax/examples/definitions in Context7 before version-sensitive styling changes — Why: prevents config/syntax drift.

```tsx
<div className={cn("rounded-md border p-4", isActive && "bg-primary text-primary-foreground")} style={{ width: `${progress}%` }} />
```

Use the snippet pattern as a hard boundary: classes for static/conditional styling, `style` only for real runtime numbers.

## Constraints & Tradeoffs

- Tailwind v4 in this repo is CSS-first; token definition lives in CSS, not in long per-component setup blocks.
- Dynamic inline style is sometimes unavoidable (charts, canvas, third-party props), but overuse removes utility consistency.
- Container queries improve component reuse but add mental overhead; use them when the component is reused in multiple layout widths.
- Fluid scales (`clamp`) improve responsiveness, but must include a `rem` term to remain zoom-accessible.
- Dark mode depends on semantic tokens; one-off hardcoded colors increase maintenance and theme bugs.

Operational constraints:
- Prefer explicit, readable utility groups over dense one-line class strings when they become difficult to review.
- Keep utility ordering stable (layout → spacing → typography → state) to reduce noisy diffs.
- If a component exceeds maintainable class complexity, extract a small wrapper component instead of moving to custom CSS too early.
- Use custom CSS only for selectors/utilities Tailwind cannot express cleanly (e.g., complex `:has()` relationships).
- Before introducing new token names, verify existing semantic tokens can represent the same intent.

## Anti-Patterns

- Writing long utility strings with duplicated classes instead of extracting reusable component patterns.
- Mixing semantic utilities and raw hardcoded colors in the same component.
- Using `style={{ padding: "16px" }}` for static spacing that should be `p-4`.
- Defining responsiveness only with page breakpoints when a component-level container rule is more correct.
- Adding custom CSS for patterns already covered by Tailwind utilities.
- Using arbitrary values as the first option (`w-[237px]`) when a scale token exists.
- Shipping components without reduced-motion handling for meaningful animation.
- Styling form controls below accessible mobile size thresholds.

Quick decision checks:
- If the value is known at build time, it should almost always be a class.
- If the value is computed from runtime/user/API state, `style` can be justified.
- If style intent is “brand/system,” use semantic tokens; if intent is “one-off visual tweak,” reconsider.

## Progressive Disclosure

1. Confirm installed Tailwind version and check Context7 exact-version docs before using syntax/config-sensitive features.
2. Start with static utility classes.
3. Add `cn()` only when conditions or variants appear.
4. Add responsive prefixes for page-level shifts.
5. Add container queries when component reuse demands local responsiveness.
6. Add inline `style` only for true runtime values.
7. Escalate to custom CSS only when utilities cannot express the requirement.

If styling decisions affect generated primitives, align with `frontend/shadcn-ui` before changing shared tokens.

Review checklist before merge:
- Utilities are semantic and token-aligned.
- No hardcoded palette literals in JSX/TSX.
- No unnecessary inline static style values.
- Responsive strategy is explicit (page breakpoint vs container query).
- Interactive targets preserve accessible sizing and motion fallbacks.

Definition of done for styling updates:
- New classes follow utility-first defaults and remain readable.
- Any runtime style usage is justified by non-static input.
- Any token changes are documented where shared theming is defined.
- Cross-skill impact on shadcn primitives has been considered.

## Resources

- Tailwind Docs — https://tailwindcss.com/docs
- Tailwind Theme Variables — https://tailwindcss.com/docs/theme
- Tailwind Container Queries — https://tailwindcss.com/docs/hover-focus-and-other-states#container-queries
- Tailwind Responsive Design — https://tailwindcss.com/docs/responsive-design
- Cross-skill: `skills/frontend/shadcn-ui/SKILL.md`

## Changelog

### v2.2 — 2026-04-22
- Aligned versioning metadata and changelog with today's frontend skill updates.

### v2.1 — 2026-04-21
- Reduced encyclopedic setup content and converted to operational rules with constraints, anti-patterns, and progressive disclosure.

### v2.0 — 2026-04-16
- Added Tailwind v4 CSS-first conventions, semantic token usage, and responsive strategy guidance.
