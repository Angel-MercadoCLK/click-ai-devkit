---
name: frontend-react-19
description: >
  Operational React 19 rules for component boundaries, forms, and modern APIs.
  Trigger: When building or refactoring React 19 components, hooks, refs, or form actions.
license: Apache-2.0
allowed-tools: Read Edit Write Glob Grep
metadata:
  author: Zesh-One
  version: "1.3"
  inspired-by: gentleman-programming/react-19
---

## When to Use

Load this skill when creating React 19 UI units that require component boundary decisions (`server` vs `client`), form actions, async reads, or ref forwarding replacements.

## Critical Patterns

1. **Do** default to Server Components; **don't** add `"use client"` by habit. **Why:** keeps bundles smaller.
2. **Do** add `"use client"` only for state, events, refs, or browser APIs; **don't** use it for static render-only views. **Why:** client boundaries are expensive.
3. **Do** verify React Compiler activation before removing memoization; **don't** strip `useMemo`/`useCallback` blindly. **Why:** non-compiled apps can regress.
4. **Do** use named React imports; **don't** use default `React` import. **Why:** aligns with modern JSX runtime and linting.
5. **Do** use `useActionState` for form action status; **don't** use removed `useFormState`. **Why:** React 19 API compatibility.
6. **Do** treat `ref` as a prop in new components; **don't** introduce `forwardRef` for React 19-first code. **Why:** simpler component signatures.
7. **Do** use `use()` for promise reads within Suspense; **don't** re-implement loading orchestration manually. **Why:** native async read semantics.
8. **Do** keep `use()` reads close to render usage; **don't** pass unresolved promises across deep trees without boundaries. **Why:** improves failure locality.
9. **Do** model form actions as idempotent server mutations; **don't** rely only on client-side optimistic assumptions. **Why:** prevents divergence.
10. **Do** follow this skill's current React 19 patterns; **don't** use legacy/deprecated React APIs from older majors; **do** verify exact-version syntax/examples/definitions in Context7 before version-sensitive work. **Why:** avoids stale guidance and runtime mismatches.

```typescript
import { useActionState } from "react";

export function ProfileForm() {
  const [state, action, isPending] = useActionState(saveProfileAction, null);
  return <form action={action}><button disabled={isPending}>{isPending ? "Saving..." : "Save"}</button></form>;
}
```

## Constraints & Tradeoffs

- Server-first composition improves performance but can constrain direct browser API access.
- React Compiler reduces manual memo work, but only when explicitly enabled in tooling.
- `use()` improves async ergonomics yet requires clear Suspense/error boundary strategy.
- `ref` as prop simplifies new code, but migration may coexist with legacy `forwardRef` wrappers.
- Form action flows centralize mutation handling, but can feel indirect versus inline handlers.

Boundary decision checklist:
- Mark as server by default, then justify each client boundary.
- Confirm compiler status before deleting memoization primitives.
- Ensure every `use()` path has surrounding Suspense and error boundaries.
- Keep form pending and result handling colocated with action usage.
- Avoid mixed legacy/new ref patterns unless migration demands coexistence.

Compiler activation hints:
- Next.js: verify `experimental.reactCompiler` before cleanup refactors.
- Non-Next bundlers: verify compiler plugin wiring in Babel/toolchain.
- Treat compiler status as environment-specific, not repository-wide by assumption.

## Anti-Patterns

1. Client-marking entire trees because one child uses events.
2. Replacing `useActionState` with ad-hoc pending booleans per form.
3. Removing memoization without confirming compiler enablement.
4. Using `use()` outside controlled Suspense/error boundaries.
5. Mixing legacy and modern ref patterns in the same component without intent.
6. Importing broad client-only libraries into server component entry points.
7. Converting server components to client components only to run simple formatting logic.

## Progressive Disclosure

1. Confirm installed React version and check Context7 exact-version docs before API-sensitive refactors.
2. Adopt server-vs-client boundary discipline first.
3. Move forms to `useActionState` and explicit pending UX.
4. Introduce `use()` for async reads where Suspense already exists.
5. Enable and validate React Compiler before memo simplification passes.
6. Cross-check app-level integration with [../nextjs-15/SKILL.md](../nextjs-15/SKILL.md).

Team rollout pattern:
- **Phase 1**: boundary rules + import hygiene.
- **Phase 2**: forms migrated to action-based flow.
- **Phase 3**: selective `use()` adoption in high-impact async reads.
- **Phase 4**: compiler-enabled optimization cleanup.

Execution guardrails:
- Keep one concern per component boundary refactor PR.
- Validate behavior snapshots before and after client/server boundary changes.
- Prefer incremental migration over whole-tree rewrites.

## Resources

- [React 19 Blog Announcement](https://react.dev/blog/2024/12/05/react-19)
- [React `useActionState`](https://react.dev/reference/react/useActionState)
- [React `use`](https://react.dev/reference/react/use)
- [React Server Components Reference](https://react.dev/reference/rsc/server-components)
- [Next.js 15 baseline conventions](../nextjs-15/SKILL.md)

Related operational pairings:
- Use with [../typescript/SKILL.md](../typescript/SKILL.md) for typed component contracts.
- Use with [../security/SKILL.md](../security/SKILL.md) when forms mutate sensitive data.
- Use with [../tanstack-query/SKILL.md](../tanstack-query/SKILL.md) for client cache orchestration.
- Keep boundary and data-fetching concerns separated during refactors.

## Changelog

### v1.3 — 2026-04-22
- Aligned versioning metadata and changelog with today's frontend skill updates.

### v1.2 — 2026-04-21
- Standardized to operational format with exact required section set.
- Replaced long explanatory blocks with 10 atomic Do/Don't rules.
- Reduced snippets to one compact reference while preserving key React 19 guidance.
- Added boundary checklist and rollout notes to keep guidance practical.

### v1.1 — 2026-04-09
- Reworded memoization rule to make compiler prerequisite explicit.
