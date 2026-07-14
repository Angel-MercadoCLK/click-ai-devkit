---
name: frontend-tanstack-query
description: >
  ZeshOne TanStack Query defaults for server/API state management.
  Trigger: When implementing async data fetching, caching, mutations, invalidation, SSR prefetch, or hydration.
license: Apache-2.0
allowed-tools: Read Write Edit Bash
metadata:
  author: Zesh-One
  version: "2.1"
  inspired-by: frontend-nextjs/tanstack-query.md
---

## When to Use

Load this skill whenever state comes from an API, database proxy, or remote service.

Use TanStack Query as the single source of truth for server state in React/Next.js.
Combine with `nextjs-15` for Server Component prefetch flows and `zustand-5` for non-server global UI state only.

## Critical Patterns

Atomic rules (Do/Don't + Why):

1. **Do** use TanStack Query for ALL server/API state. **Don't** use React Context for fetch/cache state. **Why:** Context does not provide cache lifecycle, dedupe, or invalidation semantics.
2. **Do** centralize `QueryClient` defaults in one module. **Don't** instantiate ad-hoc clients in features. **Why:** inconsistent defaults produce unpredictable cache behavior.
3. **Do** define query key factories per feature (`all/lists/list/details/detail`). **Don't** inline array keys repeatedly. **Why:** factories prevent key drift and invalidation bugs.
4. **Do** wrap fetch definitions in `queryOptions`/`infiniteQueryOptions`. **Don't** duplicate `queryFn` + keys across hooks/prefetch calls. **Why:** one contract powers hooks, prefetch, and tests.
5. **Do** prefetch in Server Components when first paint needs data. **Don't** force client-only cold starts for critical lists/details. **Why:** hydration with warm cache removes initial loading gaps.
6. **Do** make `queryFn` consume the project's single, consistent data-access layer. **Don't** mix direct endpoint calls with alternative access paths. **Why:** one read/write contract keeps auth, error mapping, tracing, and retries coherent.
7. **Do** use `HydrationBoundary` with dehydrated state from the same query options. **Don't** hydrate a different key/options shape. **Why:** mismatch causes duplicate requests and stale UI.
8. **Do** invalidate narrowly after mutations (detail + affected lists). **Don't** invalidate everything by default. **Why:** broad invalidation increases network churn and UI flicker.
9. **Do** use optimistic updates only with rollback context. **Don't** mutate cache without recovery strategy. **Why:** failed mutations otherwise leave corrupted UI state.
10. **Do** keep query functions pure and deterministic. **Don't** hide navigation/toast side effects inside `queryFn`. **Why:** side effects belong in mutation callbacks/effects.
11. **Do** propagate domain/HTTP failures from `queryFn` with `throw`. **Don't** swallow errors or return fake success shapes. **Why:** `isError`, `retry`, and error boundaries depend on real rejected promises.
12. **Do** derive UI data from `query.data` via `select` or memoized transforms. **Don't** copy query data into local state just to "sync" it. **Why:** duplicated state drifts, causes stale bugs, and adds unnecessary effects.
13. **Do** build query keys from stable, serializable primitives. **Don't** put mutable/non-serializable values (class instances, functions, unstable objects) in keys. **Why:** unstable keys fragment cache and break invalidation targeting.
14. **Do** follow this skill's current TanStack Query patterns; **Don't** use legacy/deprecated APIs from older releases; **Do** verify exact-version syntax/examples/definitions in Context7 before version-sensitive query/hydration changes. **Why:** avoids cache/API drift.

Single reference snippet (factory + options + server prefetch):

```ts
// feature-level contract reused by hooks + prefetch
export const postKeys = { all: ["posts"] as const, detail: (id: string) => [...postKeys.all, "detail", id] as const };
export const postDetailOptions = (id: string) => queryOptions({ queryKey: postKeys.detail(id), queryFn: () => postsActions.getDetail(id) });
// Server Component: await queryClient.prefetchQuery(postDetailOptions(id)); then <HydrationBoundary state={dehydrate(queryClient)}>
```

## Constraints & Tradeoffs

- TanStack Query adds conceptual overhead; pay this to gain consistent async-state architecture.
- Aggressive `staleTime` lowers refetches but can show stale data longer.
- Frequent invalidation improves freshness but increases API cost.
- Server prefetch improves first render but can increase server latency if overused.
- Optimistic updates improve perceived speed but require careful rollback paths.
- Hydration payload size grows with over-prefetching; prefetch only what first paint needs.
- Query key inputs must be stable and serializable; unstable references create hidden cache misses.

## SSR Strategy Matrix (Prefetch Decision)

Use this quick matrix before adding server prefetch:

| Scenario | Prefetch on Server? | Why |
|---|---|---|
| Critical above-the-fold data needed at first paint | **Yes** | Reduces loading gaps and improves perceived performance. |
| Personalization that depends on request-time auth/cookies and is visible immediately | **Yes** | Ensures first render is already user-correct. |
| User-triggered/secondary panels (tabs, modals, deferred sections) | **No** | Avoids server work and hydration payload bloat for non-critical paths. |
| High-churn/short-lived data where freshness matters more than initial paint | **Usually No** | Client fetch after mount can be fresher and cheaper than stale prefetch. |
| Heavy lists unlikely to be seen on entry | **No** | Prevents over-prefetching and unnecessary TTFB impact. |

Rule of thumb: prefetch only what the user must see immediately.

Operational verification before merge:

- Confirm no API state lives in React Context providers.
- Confirm each feature exposes key factory + options helper pair.
- Confirm hooks and server prefetch both consume the same options helpers.
- Confirm mutation flows invalidate exactly the impacted detail/list keys.
- Confirm SSR pages use `dehydrate` + `HydrationBoundary` when warm-first-render is required.

## Anti-Patterns

- React Context provider storing API payloads and loading flags.
- `useEffect + fetch + useState` for endpoints already managed by Query.
- Query keys constructed inline with changing object literals in components.
- Query keys containing mutable/non-serializable values (functions, class instances, unstable object/array references).
- Query functions that bypass the project's defined data-access layer.
- Different key shapes for read and invalidation paths.
- Mutation success handlers that never invalidate dependent caches.
- One giant "global query file" instead of feature-level contracts.
- Prefetching everything "just in case" without first-render need.
- Calling `new QueryClient()` inside render paths.
- Mixing UI-only state (modals, theme toggles) into query caches.
- Manually duplicating optimistic cache writes in multiple components.
- Chaining `invalidateQueries` + manual `refetch` + `setTimeout` to force UI sync.

## Mutation Design Template

Keep mutation design explicit and repeatable:

1. `mutationFn`: call the project's data-access layer; throw on failure.
2. Invalidation plan: scope invalidation to exact detail/list keys impacted.
3. Optimistic path (only if UX requires instant feedback):
   - `onMutate`: snapshot + optimistic write
   - `onError`: rollback snapshot
   - `onSettled`: final scoped invalidation
4. Side effects (toast, navigation, analytics): keep in mutation callbacks/effects, never in `queryFn`.

## Legacy Migration Playbook (Incremental)

Adopt in this order to reduce risk:

1. Pick one feature and introduce key factory + `queryOptions` helpers.
2. Replace `useEffect + fetch + useState` reads with `useQuery` using those helpers.
3. Migrate writes to `useMutation` with scoped invalidation.
4. Remove duplicated derived local state from query results (`select`/memo instead).
5. Add SSR prefetch only for first-paint critical routes.
6. Repeat feature-by-feature; avoid big-bang rewrites.

## Progressive Disclosure

Implement only the next needed layer:

1. **Version checkpoint**
   - confirm installed TanStack Query version
   - check Context7 exact-version docs for API-sensitive changes
2. **Foundational contract**
   - feature query key factory
   - feature `queryOptions` helpers
3. **Basic consumption**
   - `useQuery`/`useInfiniteQuery` using helpers only
4. **Mutation coherence**
   - scoped invalidation
   - optimistic update + rollback when UX requires it
5. **SSR performance**
   - prefetch in Server Component
   - `dehydrate` + `HydrationBoundary` reuse same options contract

Do not jump to optimistic patterns before baseline key/options discipline is solid.

Task sizing guidance:

- Small task: add key factory/options for one endpoint.
- Medium task: migrate one feature from `useEffect` fetching to Query hooks + invalidation.
- Large task: cross-feature SSR prefetch and hydration standardization.

## Resources

- TanStack Query overview: https://tanstack.com/query/latest/docs/framework/react/overview
- TanStack Query keys: https://tanstack.com/query/latest/docs/framework/react/guides/query-keys
- TanStack Query advanced SSR: https://tanstack.com/query/latest/docs/framework/react/guides/advanced-ssr
- TanStack Query mutations: https://tanstack.com/query/latest/docs/framework/react/guides/mutations
- Cross-reference: `skills/frontend/nextjs-15/SKILL.md`
- Cross-reference: `skills/frontend/zustand-5/SKILL.md`
- Cross-reference: `skills/frontend/security/SKILL.md` (protect private pages before query prefetch)
- Cross-reference: `skills/frontend/typescript/SKILL.md`

Quick review checklist:
- Every query uses a factory-generated key.
- Every query/mutation is declared via options helper.
- Every mutation has a defined invalidation plan.
- No server state persisted in Context or UI stores.

## Changelog

### v2.1 — 2026-04-22
- Reinforced best practices with explicit error semantics (`queryFn` must throw), derived-state discipline, and query-key stability rules.
- Added anti-pattern guidance (`invalidate + refetch + setTimeout`), SSR prefetch decision matrix, mutation design template, and incremental legacy migration playbook.
- Removed repo-specific data-flow wording in favor of a project-agnostic "single data-access layer" rule.

### v2.0 — 2026-04-21
- Rewritten as concise operational guidance with repo-specific defaults and strict ownership of server/API state.
- Standardized key factories, `queryOptions` helpers, and Server prefetch/HydrationBoundary flow.

### v1.9 — 2026-04-16
- Consolidated prior baseline guidance that preceded the v2.0 operational rewrite.
