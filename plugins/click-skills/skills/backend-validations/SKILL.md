---
name: backend-validations
description: >
  Boundary validation standards for ASP.NET Core APIs with FluentValidation.
  Trigger: When validating HTTP request DTOs, placing validation responsibilities, or handling validation failures.
license: Apache-2.0
metadata:
  author: Zesh-One
  version: "1.5"
allowed-tools: Read Edit Write Bash Glob Grep
---

## When to Use

- Adding or updating validation for request DTOs at HTTP boundaries.
- Deciding whether a rule belongs in FluentValidation or in a service exception.
- Standardizing 400 responses for invalid input with ProblemDetails.
- Reviewing PRs for validation scope leaks.

## Critical Patterns

1) **DO validate boundary shape in validators; DON'T validate business intent there.** Why: keeps request hygiene separate from domain policy.

2) **DO keep one validator per request DTO; DON'T share one validator across unrelated DTOs.** Why: avoids coupling and hidden regressions.

3) **DO use `MustAsync` for repository-backed checks at boundary; DON'T move those checks to controllers.** Why: keeps controllers thin and consistent.

4) **DO throw domain/application exceptions in services for business rule failures; DON'T return validation-style errors from services.** Why: preserves architectural boundaries and error semantics.

5) **DO rely on auto-validation for default 400 flow; DON'T manually call `ValidateAsync` unless you must inspect errors programmatically.** Why: removes boilerplate and keeps behavior uniform.

6) **DO return ProblemDetails for HTTP validation failures; DON'T wrap 400 errors in custom success/failure DTO envelopes.** Why: aligns API contract and tooling expectations.

7) **DO validate file metadata in FluentValidation; DON'T read file streams in validators.** Why: stream lifecycle belongs to the endpoint/service flow.

8) **DO add stream-integrity guards in endpoint/service for uploaded files; DON'T assume non-null means readable content.** Why: prevents zero-byte/corrupted upload slips.

9) **DO use clear, client-actionable validation messages; DON'T leak internal storage or implementation details.** Why: safe diagnostics without exposing internals.

10) **DO keep validation deterministic and side-effect free; DON'T mutate state or trigger external writes in validators.** Why: validators must be repeatable and predictable.

```csharp
// Program.cs (minimal registration pattern)
builder.Services.AddFluentValidationAutoValidation();
builder.Services.AddValidatorsFromAssemblyContaining<Program>();
```

## Constraints & Tradeoffs

- FluentValidation at boundary gives consistency and composability, but it is not a substitute for domain invariants.
- Async validator checks improve early feedback, but can duplicate query paths if business logic repeats checks. Keep ownership explicit.
- Auto-validation reduces ceremony, but limits custom per-endpoint branching unless manual validation is intentionally used.
- ProblemDetails standardization improves client interoperability, but may require migration from legacy custom error envelopes.
- File uploads use a hybrid rule: metadata in validator, stream checks in endpoint/service. This is deliberate separation, not duplication.

## Anti-Patterns

- Putting pricing/authorization/domain policy in validators.
- Using DataAnnotations as default style for new backend validation.
- Returning HTTP 200 with embedded validation errors.
- Reading `IFormFile.OpenReadStream()` inside validators.
- Duplicating the same rule in validator and service without an explicit reason.
- Coupling validator errors to UI wording that cannot evolve.

## Progressive Disclosure

1. Start with boundary ownership: validator vs service vs external integration.
2. Apply default registration + auto-validation flow.
3. Add async boundary checks only when needed.
4. Introduce manual validation only for conditional logic on error contents.
5. Handle file uploads with hybrid validation (metadata vs stream integrity).
6. Validate test strategy in [../testing-unit/SKILL.md](../testing-unit/SKILL.md) once rules are stable.

## Resources

- **FluentValidation docs**: https://docs.fluentvalidation.net/en/latest/aspnet.html
- **HTTP error contract**: [../responses/SKILL.md](../responses/SKILL.md)
- **Request DTO and file upload boundaries**: [../requests/SKILL.md](../requests/SKILL.md)
- **Service-level boundaries and exception flow**: [../general/SKILL.md](../general/SKILL.md)
- **Validator testing scope**: [../testing-unit/SKILL.md](../testing-unit/SKILL.md)

## Changelog

### v1.5 — 2026-04-21
- Reframed content into concise operational defaults with required structural sections.
- Kept one minimal snippet and explicit cross-references for execution context.

### v1.4 — 2026-04-09
- Consolidated previous migration notes into one baseline: package-split awareness and reduced setup noise.
