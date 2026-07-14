---
name: backend-mapping
description: >
  Operational mapping policy for ASP.NET Core APIs: service-layer ownership,
  repository entity boundaries, and explicit transformations by default.
  Trigger: When implementing Request↔Entity↔DTO mappings, reviewing AutoMapper usage,
  or enforcing clean layer contracts in backend features.
license: Apache-2.0
metadata:
  author: Zesh-One
  version: "1.6"
allowed-tools: Read Edit Write Bash Glob Grep
---

## When to Use

- Designing or refactoring feature mapping flows.
- Reviewing whether AutoMapper usage is justified.
- Enforcing service ownership of transformations.
- Preventing DTO leakage into repositories.
- Standardizing update semantics for tracked entities.
- Auditing feature pull requests for mapping boundary violations.

## Critical Patterns

1) **Do** keep mapping ownership in the Service layer; **Don’t** map in Controller or Repository.
Why: preserves clean boundaries and keeps orchestration explicit.

2) **Do** return entities from repositories; **Don’t** return DTOs from data access.
Why: repositories persist/query domain state, not API contracts.

3) **Do** use explicit extension methods by default; **Don’t** default to AutoMapper.
Why: explicit transforms are auditable, debuggable, and testable as pure logic.

4) **Do** reserve AutoMapper for strict 1:1 `Entity -> DTO` maps with zero `ForMember`;
**Don’t** use it for `Request -> Entity`.
Why: server-side defaults and invariants must stay visible.

5) **Do** place mapping files in `Features/{Feature}/Mappings/`; **Don’t** scatter mapping logic across layers.
Why: locality reduces accidental divergence.

6) **Do** map `Request -> Entity` in service flow; **Don’t** let controllers construct entities directly.
Why: services own business invariants and lifecycle defaults.

7) **Do** handle update by manual field merge on tracked entities; **Don’t** replace tracked objects with fresh mapped instances.
Why: replacing instances can break EF tracking and audit behavior.

8) **Do** keep `.ToDto()` non-null by contract; **Don’t** encode not-found semantics inside mapping.
Why: absence handling belongs to service control flow.

9) **Do** keep mapping deterministic and side-effect free; **Don’t** inject I/O into mapping methods.
Why: pure mapping is easier to test and reason about.

10) **Do** normalize data deliberately where policy requires it (for example email casing);
**Don’t** hide normalization in unrelated layers.
Why: normalization is part of domain write rules.

11) **Do** keep naming explicit (`ToEntity`, `ToDto`, `ToDtoList`); **Don’t** create ambiguous generic names.
Why: predictable naming improves discoverability and reviews.

12) **Do** keep guidance English-only; **Don’t** mix languages in docs or naming examples.
Why: this repository enforces English-only operations.

```csharp
public static UserDto ToDto(this User entity) => new() { Id = entity.Id, Email = entity.Email };
```

## Constraints & Tradeoffs

- Default explicit mapping increases boilerplate but improves transparency.
- AutoMapper can reduce repetitive code for trivial read models but must stay tightly constrained.
- Service-owned mapping centralizes logic but can grow service classes if feature boundaries are weak.
- Repository purity (entities only) simplifies architecture, but requires disciplined DTO construction upstream.
- High-throughput endpoints may need projection optimizations; still preserve ownership rules.
- Mapping conventions should be stable across features to reduce review friction.
- Prefer feature-local mapping files over a global mega-mapper to preserve cohesion.
- If a mapping requires external service calls, architecture is likely misplaced.
- Added convenience abstractions must not hide write-path invariants.

## Anti-Patterns

- Mapping in repositories to “save service code”.
- Controllers calling `.ToEntity()` or `.ToDto()` directly.
- AutoMapper `ForMember` hiding ID generation or business defaults.
- Replacing tracked entities with mapped instances during update.
- Generic bidirectional mappers that allow `Dto -> Entity` indiscriminately.
- Null-returning `.ToDto()` methods.
- Embedding API response shaping inside data-access queries.
- Mixing localized/non-English naming in mapping contracts.

## Progressive Disclosure

- **Start here (minimum):** service owns mapping, repositories return entities, explicit extensions.
- **Then refine:** extract cohesive mapping files per feature and add focused unit tests.
- **Then optimize:** permit AutoMapper only for proven trivial `Entity -> DTO` reads.
- **Advanced optional:** generic mapper abstractions only when they do not weaken ownership boundaries.

Operational checklist for PR reviews:
- Confirm repositories expose entities, not DTO projections.
- Confirm write-path defaults (`Id`, status, normalization) are explicit in service-owned mapping flow.
- Confirm update paths mutate tracked entities instead of replacing instances.
- Confirm AutoMapper usage (if any) is strictly trivial and one-directional.

For repository boundary contract see [`../dataaccess/SKILL.md`](../dataaccess/SKILL.md).
For request/response contracts see [`../requests/SKILL.md`](../requests/SKILL.md) and [`../responses/SKILL.md`](../responses/SKILL.md).
For validation-before-mapping flow see [`../validations/SKILL.md`](../validations/SKILL.md).

## Resources

- AutoMapper docs: https://docs.automapper.org/en/stable/
- ASP.NET Core architecture guidance: https://learn.microsoft.com/aspnet/core/architecture/
- C# extension members: https://learn.microsoft.com/dotnet/csharp/programming-guide/classes-and-structs/extension-methods

## Changelog

### v1.6 — 2026-04-21
- Rewritten into concise operational guidance (P1 format).
- Added mandatory section structure and atomic Do/Don’t rules with rationale.
- Reinforced service ownership and entity-only repository returns.
- Preserved targeted cross-references to dataaccess, requests, responses, and validations.
