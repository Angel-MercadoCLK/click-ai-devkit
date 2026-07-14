---
name: frontend-nextjs-15
description: >
  Operational conventions for Next.js 15 in this repo with App Router only,
  Server Components by default, Server Actions for mutations,
  and middleware-first route protection.
  Trigger: When implementing Next.js routing, data flow, auth guards,
  cache updates, or mutation paths in frontend features.
license: Apache-2.0
allowed-tools: Read Edit Write Glob Grep
metadata:
  author: Zesh-One
  version: "2.2"
---

## When to Use

- Building or refactoring any Next.js 15 feature in this repository.
- Defining server/client boundaries in route segments.
- Implementing secure mutation flows with Server Actions.
- Reviewing route protection, caching, and rendering consistency.

## Critical Patterns

These are mandatory defaults for this repository.

1. **DO use App Router only; DO NOT create `pages/` routes; WHY: route conventions, layout boundaries, and error/loading strategy are App Router based.**
2. **DO default to Server Components; DO NOT add `"use client"` unless state/events/browser APIs are required; WHY: server-first rendering reduces JS shipped to clients.**
3. **DO route ALL app API calls (GET + mutations) through `Client -> fetch wrapper -> Server Action -> backend API`; DO NOT call backend endpoints directly from client components; WHY: one canonical boundary centralizes auth/validation/logging and avoids architecture drift.**
4. **DO enforce route protection in `middleware.ts`; DO NOT rely only on hidden buttons or client redirects; WHY: middleware blocks unauthorized requests before rendering.**
5. **DO await async request APIs (`params`, `searchParams`, `cookies()`, `headers()`); DO NOT treat them as sync values; WHY: Next.js 15 request APIs are async.**
6. **DO call `redirect()` outside `try/catch`; DO NOT swallow redirect control flow in catch blocks; WHY: redirect is implemented by throwing internally.**
7. **DO add `loading.tsx` and `error.tsx` at route-segment level where needed; DO NOT centralize all failure/loading UI globally; WHY: local boundaries preserve feature UX and isolate failures.**
8. **DO organize by vertical slices (`features/<feature>/actions|schemas|components`); DO NOT scatter feature files across generic folders; WHY: locality reduces coupling and review cost.**
9. **DO revalidate cache after successful mutations (`revalidatePath` or `revalidateTag`); DO NOT leave stale server-rendered pages after writes; WHY: cache coherence is explicit in App Router flows.**
10. **DO reserve `route.ts` handlers for webhooks/third-party callbacks only; DO NOT add alternative app-facing route handlers for internal reads/writes; WHY: app traffic must stay on the canonical fetch-wrapper + Server Action path without exceptions.**
11. **DO keep secrets in server-only env vars; DO NOT expose internal API/auth values with `NEXT_PUBLIC_`; WHY: client bundles are inspectable.**
12. **DO follow this skill's current Next.js 15 patterns; DO NOT use legacy/deprecated APIs from older Next versions; DO verify exact-version syntax/examples/definitions in Context7 before version-sensitive changes; WHY: prevents API drift and broken migrations.**

```typescript
export default async function Page({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return <UserDetails id={id} />;
}
```

Server Action baseline:

- Validate input on server before mutation.
- Check auth/permissions on server path.
- Revalidate cache after successful mutation.
- Return controlled error objects for expected validation failures.

## Constraints & Tradeoffs

- Server Actions simplify secure mutations but depend on Next runtime conventions.
- Middleware-first protection improves security posture but requires precise matcher scoping.
- Server Components reduce bundle size by default, but interactive features still need explicit client islands.
- Strict route-handler scope (webhooks/callbacks only) may require architecture changes for teams used to API-route-first designs.
- Aggressive revalidation improves consistency but can increase backend traffic if applied too broadly.
- Segment-level loading/error boundaries improve resilience but require teams to think per feature slice.
- Deep client boundaries improve performance but can make prop contracts more explicit and verbose.
- Async request API handling in Next.js 15 adds clarity but requires discipline in typing and awaiting.
- Middleware ownership centralizes access control but increases sensitivity to matcher misconfiguration.

## Anti-Patterns

- Any direct client-side backend API call (reads or writes).
- Adding new `pages/` routes or legacy API-route patterns.
- Wrapping `redirect()` in broad `try/catch` and returning fallback states.
- Marking root layouts/pages with `"use client"` without hard necessity.
- Reading secret env vars from client modules.
- Duplicating app API logic in route handlers instead of Server Actions.
- Depending on client-only guards for protected routes.
- Skipping revalidation and shipping stale post-mutation UI.

## Progressive Disclosure

Implement in this order:

1. Define route segment and default server-rendered component boundary.
2. Confirm installed Next.js version and check Context7 exact-version docs before changing routing/request/caching APIs.
3. Add middleware protection for access and security policy.
4. Implement mutation via Server Action (validate, authorize, mutate, revalidate).
5. Introduce minimal client component only where interaction requires it.
6. Add route-local `loading.tsx`/`error.tsx` for resilience.

Quick reviewer checklist:

- Confirm route exists under `app/` and not `pages/`.
- Confirm every app API call (GET + mutation) follows `Client -> fetch wrapper -> Server Action -> backend API`.
- Confirm middleware matcher protects required routes.
- Confirm async request APIs are awaited.
- Confirm revalidation happens after successful writes.
- Confirm secrets remain server-only.

Cross-skill references:

- Data cache and hydration patterns: `../tanstack-query/SKILL.md`.
- Frontend security hardening details: `../security/SKILL.md`.
- TS conventions and boundary typing: `../typescript/SKILL.md`.
- State management integration guidance: `../zustand-5/SKILL.md`.

## Resources

- [https://nextjs.org/docs/app](https://nextjs.org/docs/app)
- [https://nextjs.org/docs/app/building-your-application/data-fetching/server-actions-and-mutations](https://nextjs.org/docs/app/building-your-application/data-fetching/server-actions-and-mutations)
- [https://nextjs.org/blog/next-15](https://nextjs.org/blog/next-15)
- [../tanstack-query/SKILL.md](../tanstack-query/SKILL.md)
- [../security/SKILL.md](../security/SKILL.md)
- [../typescript/SKILL.md](../typescript/SKILL.md)

Repository-level context:

- `README.md` for project architecture defaults.
- `ADOPTING.md` for onboarding and verification flow.

## Changelog

### v2.2 — 2026-04-22
- Aligned versioning metadata and changelog with today's frontend skill updates.

### v2.1 — 2026-04-21
- Rewritten into concise operational guidance with strict section structure and repository constraints.
- Enforced App Router-only routing, Server Action-first mutations, and middleware-first protection defaults.

### v2.0 — 2026-04-16
- Consolidated prior baseline updates for async request APIs and vertical-slice/server-first patterns.
