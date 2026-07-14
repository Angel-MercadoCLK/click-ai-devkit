---
name: frontend-shadcn-ui
description: >
  ZeshOne shadcn/ui conventions for composed UI primitives.
  Trigger: When building components/forms with shadcn/ui and customizing behavior via composition or theme tokens.
license: Apache-2.0
allowed-tools: Read Write Edit Bash
metadata:
  author: Zesh-One
  version: "2.1"
  inspired-by: frontend-nextjs/components.md
---

## Applicability / Opt-in Gate

This skill is **optional** and applies **only when the developer explicitly requests shadcn/ui**.

Use it when all of the following are true:
- The work includes shadcn/ui primitives or forms.
- The developer explicitly chose shadcn/ui for this task.
- The goal is to accelerate UI delivery with composable primitives.

Do **not** apply it when:
- The project uses a different UI system/design system.
- The developer did not request shadcn/ui.
- The change is backend/non-UI and shadcn/ui adds no value.

This guide is focused on **shadcn/ui best practices**: stable generation, composition-first customization, accessible forms, and token-driven theming.

## When to Use

Use this skill only after the **Applicability / Opt-in Gate** passes.

Load it when you are implementing or reviewing:
- shadcn/ui primitive composition,
- shadcn-based forms and accessibility semantics,
- theme/token customization for shadcn components,
- version-sensitive shadcn component/pattern updates.

Cross-reference:
- `frontend/tailwind-4`: utility-first styling and token discipline.
- `frontend/zod-4`: schema conventions for form validation.

## Critical Patterns

- DO treat this skill as a non-default accelerator, not a mandatory baseline — Why: UI stack selection remains a developer decision.
- DO add/update primitives using the `shadcn` CLI — Why: keeps generator output and registry metadata aligned.
- DO treat `components/ui/*` as vendor-like generated base — Why: direct edits are brittle on regeneration.
- DO customize behavior via wrappers/composition in feature components — Why: preserves upgrade path.
- DO customize look through shared tokens and Tailwind utilities — Why: consistent theming across primitives.
- DO keep form contracts explicit (`React Hook Form` + Zod schema + typed defaults) — Why: avoids runtime mismatch bugs.
- DO keep dark mode class-based through a shared provider — Why: compatible with Tailwind dark variants.
- DO review Context7 and prefer the latest documented shadcn/ui component/pattern versions before any version-sensitive change — Why: reduces drift and prevents applying outdated APIs.
- DON'T fork generated primitive internals unless unavoidable — Why: difficult merges and drift from upstream.
- DON'T place business logic inside generic primitives — Why: primitives should remain reusable and low coupling.
- DON'T import heavy animation libraries in server-rendered paths by default — Why: unnecessary JS and hydration cost.
- DON'T assume legacy template snippets are valid without checking current docs/context — Why: old patterns frequently break on newer releases.

```tsx
<Card className="p-4">
  <CardHeader><CardTitle>Plan</CardTitle></CardHeader>
  <CardContent><Button asChild><a href="/billing">Manage billing</a></Button></CardContent>
</Card>
```

The snippet shows the default strategy: compose primitives into feature-level UI instead of editing primitive source.

## Constraints & Tradeoffs

- Generated primitives are fast to adopt but opinionated; prefer wrappers when behavior diverges.
- Composition improves maintainability but may add extra small components; this is acceptable for long-term safety.
- Theme/token customization scales globally but can be slower for one-off visual experiments.
- Dynamic import for heavy animated widgets reduces SSR cost but can delay first interactive paint for that widget.
- Forms built with typed schema contracts are more verbose, but prevent invalid state drift.

Operational constraints:
- Keep primitive APIs predictable; wrapper components should expose focused domain props, not every underlying prop by default.
- Avoid deep prop-drilling through primitive wrappers; co-locate composition near the feature boundary.
- If a primitive needs repeated visual overrides, prefer a variant/composition layer before touching generated source.
- For form fields, keep label/help/error semantics intact even when custom layouts are introduced.
- Dark mode changes should be token/class driven, not conditional hardcoded class forks per component.

## Anti-Patterns

- Editing `components/ui/button.tsx` for app-specific behavior.
- Embedding API calls directly inside shared primitive files.
- Duplicating primitive variants ad-hoc across pages instead of centralizing composition.
- Mixing multiple theming strategies (inline hardcoded colors + token classes) in the same component.
- Importing animation-heavy components globally when only one route uses them.
- Coupling feature state management directly into generic primitive wrappers.
- Creating inaccessible custom controls by omitting keyboard/focus/aria semantics from shadcn patterns.
- Building forms without schema parity between UI and submit handler contracts.

Quick decision checks:
- If the change is domain-specific, do it in composition layer, not primitive source.
- If the change is visual-system wide, do it in tokens/theme.
- If the change is behavior-core to the primitive and reusable, document why direct primitive edit is unavoidable.

## Progressive Disclosure

1. Confirm installed shadcn/ui setup versions, review Context7, and prefer the most recent documented component/pattern approach before API-sensitive primitive changes.
2. Start with generated primitives as-is.
3. Compose primitives into feature components with domain labels/actions.
4. Add validation and typed form state where input flows exist.
5. Add token-level theming adjustments before touching primitive internals.
6. Add dynamic imports only for genuinely heavy client-only visuals.
7. Consider primitive internals changes only as a last resort and document the reason.

If a styling rule conflicts with primitive composition, follow `frontend/tailwind-4` semantic utility rules first.

Review checklist before merge:
- Generated primitive files remain untouched unless explicitly justified.
- Domain behavior lives in feature composition, not in `components/ui/*`.
- Form semantics (`Label`, error messaging, focus behavior) are preserved.
- Theme behavior remains token/class-based and consistent with dark mode strategy.
- Heavy client-only visual dependencies are isolated behind dynamic boundaries when needed.

Definition of done for component updates:
- Primitive changes are composed at feature level by default.
- Form flows keep schema, defaults, and UI field contracts aligned.
- Accessibility semantics remain intact after customization.
- Theme changes stay consistent with token-driven styling.

## Resources

- shadcn/ui Docs — https://ui.shadcn.com/docs
- shadcn/ui Components — https://ui.shadcn.com/docs/components
- shadcn/ui Forms — https://ui.shadcn.com/docs/components/form
- shadcn/ui Dark Mode — https://ui.shadcn.com/docs/dark-mode/next
- Cross-skill: `skills/frontend/tailwind-4/SKILL.md`

## Changelog

### v2.1 — 2026-04-22
- Added mandatory **Applicability / Opt-in Gate** clarifying shadcn/ui is used only on explicit developer request.
- Clarified this skill is optional (not default mandatory) and focused on shadcn/ui best-practice guidance.
- Strengthened version-governance rule: review Context7 and prefer latest documented component/pattern versions before version-sensitive changes.

### v2.0 — 2026-04-21
- Reduced encyclopedic setup material and converted to concise operational rules with constraints, anti-patterns, and progressive disclosure.
