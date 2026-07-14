---
name: backend-responses
description: >
  Operational response contract guidance for ASP.NET Core APIs with strict
  internal boundaries and safe external error mapping.
  Trigger: When defining service return contracts, mapping domain failures to HTTP,
  or enforcing consistent response behavior across backend services.
license: Apache-2.0
metadata:
  author: Zesh-One
  version: "2.0"
allowed-tools: Read Edit Write Bash Glob Grep
---

## When to Use

- Defining how service outcomes become HTTP responses.
- Refactoring endpoints that leak internal contracts to clients.
- Establishing one response style for a service boundary.
- Reviewing error handling for security and consistency.

## Critical Patterns

This skill enforces ONE internal contract per service and ONE external contract for clients.

1. **DO choose exactly one internal contract (`Result<T>` or legacy `ResponseDTO<T>`); DO NOT mix both in one service; WHY: mixed contracts force fragile controller branching.**
2. **DO use `Result<T>` for new services; DO NOT introduce `ResponseDTO<T>` in greenfield code; WHY: explicit success/failure semantics reduce hidden behavior.**
3. **DO keep repositories returning entities/null only; DO NOT return transport envelopes from repositories; WHY: repository is data boundary, not API contract boundary.**
4. **DO map internal contracts at controller boundary (`ToHttpResponse()` style); DO NOT return internal envelope objects as HTTP payload; WHY: clients must receive stable raw JSON or `ProblemDetails`.**
5. **DO use explicit expected-failure factories for business outcomes (validation/conflict/not found by design); DO NOT throw exceptions for normal business branches; WHY: expected behavior should be first-class, testable flow.**
6. **DO keep unhandled exceptions in middleware ownership; DO NOT serialize raw exception text from services; WHY: stack traces and internals are security leaks.**
7. **DO use status-accurate mapping (201/204/400/403/404/409/500); DO NOT return `200` for failures; WHY: clients, retries, and observability depend on real status semantics.**
8. **DO emit `Location` for 201 Created; DO NOT return 201 without canonical resource URI; WHY: REST clients depend on discoverability of created resources.**
9. **DO return `Result<object>.NoContent()` for body-less successful deletes/mutations; DO NOT attach decorative payloads to 204; WHY: 204 contract has no body.**
10. **DO keep external error shape standardized with `ProblemDetails`; DO NOT create custom error objects per endpoint; WHY: standard shape lowers client complexity and incidents.**

Example boundary mapping:

```csharp
public async Task<IActionResult> Create([FromBody] CreateUserRequest request)
{
    var result = await _users.CreateAsync(request); // Result<UserDto>
    return result.ToHttpResponse();
}
```

Layer contract baseline:

- Repository: entity or null.
- Service: internal contract (`Result<T>` preferred, `ResponseDTO<T>` legacy only).
- Controller: HTTP response mapping only.

Expected exception ownership:

- Domain exceptions still possible for exceptional paths: `NotFoundException`, `ForbiddenException`, `ConflictException`.
- `401 Unauthorized` is auth pipeline ownership, not domain exception ownership.

## Constraints & Tradeoffs

- `Result<T>` clarity comes with explicit factory/status mapping work in service code.
- Legacy code may need temporary `ResponseDTO<T>` retention; migration should be per-service, never mixed in one service.
- Strict external contract (`2xx` resource JSON, non-`2xx` `ProblemDetails`) may require adapters in old controllers.
- Middleware-centric unexpected-error handling improves safety but requires disciplined no-leak service behavior.
- Uniform contracts improve maintenance but can feel verbose for trivial endpoints; keep consistency anyway.
- Cross-team migration requires a clear cutline by feature/service to avoid long-lived hybrid states.
- Contract consolidation may surface hidden assumptions in clients that expected custom error payloads.
- Consistency rules should be enforced in PR review checklists to prevent regressions.
- Teams integrating with external gateways may need explicit adapter mapping while preserving internal standards.

## Anti-Patterns

- Returning `ResponseDTO<T>` directly in controller HTTP payload.
- Returning `Result<T>` object as public JSON envelope.
- Exposing `Exception`, `StackTrace`, SQL fragments, or internal identifiers to clients.
- Returning null from service for not-found/conflict flows.
- Converting all failures to generic `500` and losing domain meaning.
- Creating `UnauthorizedException` for 401 semantics.
- Combining legacy and modern response contracts inside one feature slice.
- Ignoring 201 `Location` and 204 empty-body semantics.

## Progressive Disclosure

Use this order when implementing or reviewing:

1. Choose internal contract per service (`Result<T>` preferred).
2. Confirm repository/service/controller boundary ownership.
3. Verify status mapping for happy path and expected failures.
4. Verify unexpected failures bubble to middleware and remain sanitized.
5. Check external payload shape consistency (`ProblemDetails` for non-2xx).

Practical review checklist:

- Verify one contract keyword appears in service signatures.
- Verify controller returns mapped HTTP result, not envelope object.
- Verify 201 responses include location semantics.
- Verify 204 responses send no payload.
- Verify expected business failure and unexpected failure paths are both covered.
- Verify no exception field/string from internals is serialized.

Escalate to related skills when needed:

- Middleware + correlation/logging ownership: `../logging/SKILL.md`.
- Authorization/forbidden policy details: `../security/SKILL.md`.
- Shared exception conventions: `../general/SKILL.md`.
- Input and mapping boundaries: `../requests/SKILL.md`, `../mapping/SKILL.md`, `../validations/SKILL.md`.

## Resources

- [../logging/SKILL.md](../logging/SKILL.md)
- [../security/SKILL.md](../security/SKILL.md)
- [../general/SKILL.md](../general/SKILL.md)
- [../requests/SKILL.md](../requests/SKILL.md)
- [../mapping/SKILL.md](../mapping/SKILL.md)
- [../validations/SKILL.md](../validations/SKILL.md)
- [ASP.NET ProblemDetails docs](https://learn.microsoft.com/aspnet/core/fundamentals/error-handling)

Related governance docs in this repo:

- `README.md` for project-wide defaults.
- `ADOPTING.md` for distribution and verification workflow.

## Changelog

### v2.0 — 2026-04-21
- Rewritten as concise operational guidance with mandatory sections and repository-standard wording.
- Enforced one-contract-per-service and no raw exception leakage as default policy.

### v1.7 — 2026-04-16
- Consolidated previous history into a single legacy baseline for contract-evolution context.
