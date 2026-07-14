---
name: frontend-zod-4
description: >
  Operational Zod 4 rules for schema-first validation and type-safe form flows.
  Trigger: When creating, updating, or integrating Zod schemas in frontend forms and data boundaries.
license: Apache-2.0
allowed-tools: Read Write Edit Bash
metadata:
  author: Zesh-One
  version: "1.2"
  inspired-by: gentleman-programming/zod-4
---

## When to Use

Load this skill when defining runtime validation contracts, deriving static types from schemas, or wiring schema validation into form/data entry flows.

## Critical Patterns

1. **Do** use Zod 4 top-level validators (`z.email()`, `z.uuid()`, `z.url()`); **don't** rely on deprecated chained string validators. **Why:** aligns with v4 API.
2. **Do** use `{ error: ... }` for custom messages; **don't** use `{ message: ... }`. **Why:** v4 error customization contract.
3. **Do** prefer `safeParse` in user-facing flows; **don't** let `parse` throw into UI control flow. **Why:** predictable error handling.
4. **Do** derive types with `z.infer<typeof Schema>`; **don't** duplicate interface definitions. **Why:** schema and type stay synchronized.
5. **Do** encode required strings with `.min(1)`; **don't** use removed `.nonempty()`. **Why:** v4-compatible required semantics.
6. **Do** use `superRefine` for cross-field rules; **don't** split dependent validation across disconnected checks. **Why:** one coherent error surface.
7. **Do** centralize reusable schema fragments; **don't** copy-paste the same validators across features. **Why:** avoids drift.
8. **Do** validate external input at boundaries (forms, query params, API payloads); **don't** trust TypeScript-only typing. **Why:** runtime safety.
9. **Do** map `safeParse` issues to UX-friendly field errors; **don't** expose raw internal issue objects. **Why:** stable UI contracts.
10. **Do** follow this skill's current Zod 4 patterns; **don't** use legacy/deprecated APIs from older Zod versions; **do** verify exact-version syntax/examples/definitions in Context7 before version-sensitive schema changes. **Why:** prevents migration regressions.

```typescript
const SignInSchema = z.object({
  email: z.email({ error: "Invalid email" }),
  password: z.string().min(8, { error: "Minimum 8 characters" }),
});

type SignInData = z.infer<typeof SignInSchema>;
```

## Constraints & Tradeoffs

- Schema-first design improves consistency but adds upfront modeling effort.
- `safeParse` avoids exceptions, but requires explicit branching for success/error states.
- Shared schema fragments reduce duplication, but over-abstraction can hide local intent.
- Cross-field validation in `superRefine` centralizes logic, yet can be harder to unit-isolate.
- Version drift (v3 vs v4) is a real risk; lock package major versions intentionally.

Version and contract checklist:
- Confirm installed Zod major before applying syntax conventions.
- Keep schema definitions close to boundary adapters/forms.
- Ensure every schema mutation updates inferred type consumers.
- Keep custom error messages stable for UX and localization.
- Validate unknown external data before any business logic transform.

Schema maintenance notes:
- Prefer small composable schema fragments over one massive object schema.
- Keep field error copy centralized when localization is required.
- Tie schema module ownership to feature boundaries to prevent hidden coupling.

## Anti-Patterns

1. Mixing v3 and v4 APIs (`message`, `.nonempty()`, chained email validators).
2. Creating separate TypeScript interfaces not derived from the schema.
3. Throwing `parse` errors directly into UI event handlers.
4. Duplicating schema literals per form instead of extracting shared parts.
5. Emitting raw Zod issue internals as final user-facing messages.
6. Encoding cross-field business rules as scattered single-field validators.
7. Mutating parsed objects ad hoc instead of validating target shape first.

## Progressive Disclosure

1. Confirm installed Zod version and check Context7 exact-version docs before API-sensitive schema edits.
2. Start with field-level validators and type inference.
3. Move to `safeParse`-driven error handling in all form submits.
4. Add `superRefine` for dependent field rules.
5. Extract schema modules shared by forms, actions, and adapters.
6. Integrate consistently with app form architecture (see [../react-19/SKILL.md](../react-19/SKILL.md)).

Adoption by capability:
- **Level 1**: basic field schemas + inferred types.
- **Level 2**: safe parsing and user-facing error mapping.
- **Level 3**: cross-field refinement and reusable schema composition.
- **Level 4**: shared contracts across UI and server adapters.

Execution checkpoints:
- Add schema tests for critical business validations.
- Reuse parser helpers where multiple entry points share the same payload shape.
- Review cross-field refinements with domain owners before rollout.

## Resources

- Zod Docs — https://zod.dev/
- Zod API Reference — https://zod.dev/api
- Zod Ecosystem — https://zod.dev/ecosystem
- React Hook Form Resolvers — https://github.com/react-hook-form/resolvers
- React 19 form/action conventions — [../react-19/SKILL.md](../react-19/SKILL.md)

Related operational pairings:
- Use with [../typescript/SKILL.md](../typescript/SKILL.md) for strict inferred types.
- Use with [../security/SKILL.md](../security/SKILL.md) for input boundary hardening.
- Use with [../tanstack-query/SKILL.md](../tanstack-query/SKILL.md) for validated query/mutation payloads.
- Keep schema files versioned with API contract changes to prevent drift.

## Changelog

### v1.2 — 2026-04-22
- Aligned versioning metadata and changelog with today's frontend skill updates.

### v1.1 — 2026-04-21
- Standardized to operational format with exact required section set.
- Converted guidance into 10 atomic Do/Don't rules with concise rationale.
- Reduced snippets to one compact example and added progression path.
- Added version/contract checklist to reduce v3-v4 migration mistakes.
