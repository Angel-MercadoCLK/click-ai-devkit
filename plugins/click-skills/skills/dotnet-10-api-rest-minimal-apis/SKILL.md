---
name: dotnet-10-api-rest-minimal-apis
description: >
  REST API design and Minimal API surface rules for .NET 10 / C# 14 REST APIs,
  covering TypedResults, Results<T1,T2> unions, ProblemDetails (RFC 7807), URL/versioning
  conventions, built-in OpenAPI with Scalar, middleware ordering, and endpoint organization.
  Trigger: adding or modifying endpoints in Program.cs, a MapGroup extension, or endpoint
  extension classes; creating DTO records; configuring middleware pipeline ordering; working
  on OpenAPI/Scalar; mapping entities to DTOs in handlers; or designing URL structure and
  versioning segments.
license: Apache-2.0
metadata:
  author: Zesh-One
  version: "2.1"
allowed-tools: Read Edit Write Bash Glob Grep
---

## When to Use

- Adding or modifying any Minimal API endpoint in `Program.cs`, a `MapGroup` extension, or an endpoint extension class.
- Creating or reviewing DTO record types for request or response.
- Configuring or reviewing middleware pipeline order.
- Working on OpenAPI document configuration, Scalar setup, or build-time doc generation.
- Designing URL routes, versioning segments, or authorization/rate-limit policies on endpoints.
- Mapping entities to response DTOs inside handlers or service methods.

## Critical Patterns

### Rule 1 — Do: `TypedResults`; Don't: `Results`

```csharp
// ✅
static async Task<Results<Ok<UserResponse>, NotFound>> GetUser(
    Guid userId, IUserService svc) =>
    await svc.FindAsync(userId) is { } user
        ? TypedResults.Ok(new UserResponse(user.Id, user.Email))
        : TypedResults.NotFound();

// ❌ — Results.Ok() emits no OpenAPI response schema in .NET 10
return Results.Ok(user);
```

**Why**: `TypedResults` variants are the mechanism .NET 10 uses to infer response schema for the built-in OpenAPI provider. `Results.*` produces an opaque `IResult` — the generated doc omits the type.

### Rule 2 — Do: declare `Results<T1, T2, ...>` union return type; Don't: use `IResult`

**Why**: without the explicit union, OpenAPI cannot enumerate all possible HTTP responses. Use `IResult` only for genuinely open-ended surfaces and document the exception.

### Rule 3 — Do: URL-segment versioning via Asp.Versioning; Don't: hardcode `"v1"` literals

Pattern: `/api/v{version}/resource` — version is the second segment, always present on public endpoints.

**Why**: hardcoded literals drift from the Asp.Versioning.Http configuration and break version negotiation.

### Rule 4 — Do: plural kebab-case nouns in paths; Don't: verbs or singular nouns

- Collections: `/api/v1/users`, `/api/v1/purchase-orders`
- Identifiers follow collection: `/api/v1/users/{userId}`
- RPC-style verbs only when no REST-idiomatic alternative exists: `/api/v1/auth/refresh`

### Rule 5 — Do: nest at most two resource levels; Don't: add a third

`/api/v1/teams/{teamId}/members` — acceptable.
`/api/v1/teams/{teamId}/members/{memberId}/permissions` — flatten to a top-level resource.

**Why**: depth beyond two couples routing to domain hierarchy without navigational benefit.

**Exception:** RPC-style verbs are acceptable for explicit state transitions that carry governance semantics — e.g. `POST /api/v1/entries/{entryId}/deactivate` and `/reactivate` — where a generic `PATCH {active:false}` would hide hierarchy/governance rules (active children, immutable fields). `/users/me/...` sub-resources are also acceptable: `me` denotes the authenticated subject, not an arbitrary third nesting level. Prefer <=2 levels and REST verbs for new resources.

### Rule 6 — Do: ProblemDetails (RFC 7807) everywhere; Don't: plain strings or custom error envelopes

Applies to: endpoint handlers, middleware short-circuits (rate-limit 429, 401, 403), `UseStatusCodePages`, and `UseExceptionHandler`.

Minimum fields: `status`, `title`, and a `traceId` extension from `HttpContext.TraceIdentifier`.

**Why**: every error surface must be contract-consistent. One envelope — `ProblemDetails` — no exceptions.

### Rule 7 — Do: generic 401 on auth failure; Don't: leak distinguishing details

**Why (security critical)**: authentication failure results must NOT carry a `reason`, `code`, or `message` field that distinguishes causes. A 401 that distinguishes bad password / unknown user / locked account is an account-enumeration oracle.

### Rule 8 — Do: built-in `Microsoft.AspNetCore.OpenApi` + Scalar; Don't: Swashbuckle or NSwag

- OpenAPI doc generated at build time via `Microsoft.Extensions.ApiDescription.Server`; output committed to `docs/openapi.json`.
- Scalar mounted at `/scalar/v1` and gated behind a non-production environment check.
- Do not expose Scalar or `/openapi/{documentName}.json` in production.

### Rule 9 — Do: MapGroup for shared metadata; Don't: repeat policy/prefix per endpoint

```csharp
app.MapGroup("/api/v{version}/users")
   .WithTags("Users")
   .RequireAuthorization("AdminOnly")
   .RequireRateLimiting("api-default")
   .MapUsersEndpoints();
```

Register groups as static extension methods on `IEndpointRouteBuilder` (`app.MapUsersGroup()`). This organization works in any assembly layout — single-assembly vertical slices and modular-monolith modules alike; assembly topology is an architecture-level decision (see the architecture skill).

### Rule 10 — Do: every endpoint gets `WithName`, `WithTags`, and the `Results<>` return union; Don't: `WithOpenApi()` alone

`WithName`: kebab-case, globally unique operationId. `WithTags`: one or two tags matching the resource. Add `WithSummary`/`WithDescription` when the operationId is not self-explanatory.

### Rule 11 — Do: thin handlers via typed service injection; Don't: business logic or DB calls in handlers

Handlers receive typed service interfaces via DI parameter injection, call the service, pattern-match the returned `Result` discriminated union, and return `TypedResults`. No `DbContext`, no transactions, no validation logic beyond a null-guard.

### Rule 12 — Do: manual validation through the Result pattern; Don't: `AddValidation()` or FluentValidation on auth/security endpoints

**Why (security critical)**:
- `AddValidation()` / FluentValidation auto-emit `ValidationProblem(errors)` 400 responses before the handler runs — this creates account-enumeration oracles on login, TOTP, password-reset, and session-refresh endpoints. Never let pipeline validation fire on those surfaces.
- If validation needs database reads under a transactional seam (uniqueness checks, domain invariants), it belongs in the service layer inside that seam — pipeline filters fire too early to read consistently.
- If database constraint violations are the authoritative uniqueness/business-rule signal (e.g. Postgres `23505`, `P0001`, `23514`), translate their error codes in one central mapper instead of pre-validating in the pipeline.

Conditional exception: purely structural, non-security endpoints (e.g. pagination query-param range checks) MAY opt into `AddValidation()` with DataAnnotations after confirming no enumeration exposure. Default remains hand-rolled validation through the `Result` union.

### Rule 13 — Do: direct typed service injection; Don't: MediatR or any mediator bus

**Why**:
- MediatR v13+ requires a commercial license above a revenue threshold (compliance risk).
- MediatR's open-generic reflection-based registration is AOT/trimming-hostile — a hard blocker for net10.0 AOT/trimming publication.
- The typed `Result` discriminated unions are incompatible with MediatR exception-throwing pipeline behaviors.

Default is direct typed-service DI injection. If a team explicitly wants the mediator shape and documents a real decoupled-dispatch use case, use a source-generated, MIT-licensed mediator (e.g. `martinothamar/Mediator` — AOT-compatible). Document the decision.

### Rule 14 — Do: manual positional record constructors for DTO mapping; Don't: AutoMapper, Mapster, or any convention-based mapper

```csharp
// ✅ — sensitive fields are visibly absent, compile-time exhaustiveness
var dto = new UserResponse(entity.Id, entity.Email, entity.DisplayName);
```

**Why**: explicit mapping makes the absence of sensitive fields (`PasswordHash`, `TotpSecret`, raw key material, session identifiers) auditable at the call site and verifiable in code review. Convention mappers can silently include new properties. EF Core LINQ `Select` projections are also acceptable (push column selection to SQL, AOT-safe).

### Rule 15 — Do: enforce middleware pipeline order; Don't: deviate without documenting why

Required order:
1. `UseExceptionHandler` + `UseStatusCodePages`
2. `UseHttpsRedirection`
3. `UseCors`
4. `UseRateLimiter`
5. `UseAuthentication`
6. `UseAuthorization`
7. `UseSerilogRequestLogging` (or equivalent)
8. `app.Map*` endpoint registrations

Critical: `UseAuthentication` MUST precede `UseAuthorization`. Do NOT call `app.UseRouting()` explicitly — routing is implicit in .NET 10.

**Exception (proxy TLS termination):** if TLS terminates at a reverse proxy, `UseHttpsRedirection` may be omitted from the app pipeline — the in-app redirect is redundant. Document any pipeline-order deviation and its reason.

> **Cross-ref — request logging placement**: the exact position of the request-logging middleware (`UseSerilogRequestLogging` or equivalent) in the pipeline is owned by the **observability-operations** skill — see there for placement details.

## Constraints & Tradeoffs

- **Security headers (HSTS, X-Content-Type-Options, X-Frame-Options, Referrer-Policy, CSP)** are owned by the **security** skill. Note: for JSON-only APIs, CSP is often handled at the reverse proxy — check the security skill before adding CSP middleware.
- **Caching vs. session revocation**: never cache authenticated responses when the project validates session revocation on every request — any response cache reintroduces a revocation window. Response/output caching on anonymous, non-sensitive endpoints is acceptable with an explicit design decision.
- **Centralize database error translation**: if the project treats database constraint violations as authoritative business-rule signals (e.g. Postgres `23505`/`P0001`/`23514`), keep the error-code-to-`Result`/`ProblemDetails` translation in a single mapper component. Never duplicate the mapping inline in endpoints or services; extend the mapper when adding new constraints.
- **CORS**: never `AllowAnyOrigin` outside development. Configure explicit allowed origins and apply globally via `app.UseCors()`. A regex-anchored origin pattern is one option — useful when a controlled set of dynamic subdomains must be allowed — a static list is fine for fixed origins.
- **Rate limiting**: use the built-in `AddRateLimiter` with named policies applied at the group level. The `OnRejected` handler must emit a `ProblemDetails` 429 response, not a plain string.
- **Authorization**: define named authorization policies (e.g. `AdminOnly`) and apply them via `RequireAuthorization` at group or endpoint level. Do not duplicate policy logic inline; document the rationale for any new policy at registration.
- **Health checks are outside the versioned namespace**: `/health/live` (no DB), `/health/ready` (with `DbContext` check). Detailed response requires an admin-restricted authorization policy.
- **Connection strings from environment variables or a secret store** — never from committed configuration files. `ValidateOnStart` must be applied for always-required configuration; omissions must be commented with justification.
- **Endpoint organization**: organize endpoint modules via static `IEndpointRouteBuilder` extension methods. This works in single-assembly and multi-project layouts alike — assembly topology is an architecture-level decision (see the architecture skill).

## Anti-Patterns

- `Results.Ok()` / `Results.NotFound()` instead of `TypedResults.*`.
- `IResult` return type where a `Results<T1,T2>` union is possible.
- Business logic, DB calls, or transactions inside endpoint handler lambdas.
- `AddValidation()` or FluentValidation on any auth/TOTP/password-reset/session-refresh endpoint.
- Non-ProblemDetails error responses from any handler or middleware.
- 401 response body that distinguishes failure cause (bad password vs. unknown user vs. locked account).
- Route nesting beyond two resource levels.
- Hardcoded `"v1"` literals instead of `Asp.Versioning` route templates.
- Swashbuckle or NSwag (use built-in `Microsoft.AspNetCore.OpenApi`).
- Scalar or OpenAPI endpoint exposed in production.
- `UseAuthentication` placed after `UseAuthorization` in the pipeline.
- `app.UseRouting()` called explicitly in a .NET 10 Minimal API pipeline.
- MediatR (`IMediator`, `IRequest`, `IPipelineBehavior`) for endpoint dispatch.
- AutoMapper, Mapster, or any convention-based mapper in handler or service code.
- `AllowAnyOrigin` CORS in any non-development environment.

## Progressive Disclosure

1. **Start here**: `Critical Patterns` — the decisions agents most commonly get wrong.
2. **If reviewing a security-sensitive flow**: rules 7 and 12 (enumeration-oracle prevention, validation gate timing).
3. **If reviewing middleware or pipeline changes**: rule 15 and `Constraints & Tradeoffs` (CORS, rate limiting, caching).
4. **If a neighboring layer is involved**: consult `Resources` for data-access/transaction boundaries, auth configuration, or testing guidance.

## Resources

- Architecture selection and assembly layout: [`../architecture/SKILL.md`](../architecture/SKILL.md)
- Transactional boundaries and data access: [`../data-access-persistence/SKILL.md`](../data-access-persistence/SKILL.md)
- Authentication, sessions, and CORS: [`../security/SKILL.md`](../security/SKILL.md)
- Options pattern, TypedResults, C# 14 records, nullable: [`../dotnet-conventions/SKILL.md`](../dotnet-conventions/SKILL.md)
- Endpoint and integration testing tiers: [`../testing/SKILL.md`](../testing/SKILL.md)
- Result discriminated unions, error mapping, manual mapping: [`../design-patterns/SKILL.md`](../design-patterns/SKILL.md)
- Microsoft Learn — Minimal APIs overview: https://learn.microsoft.com/en-us/aspnet/core/fundamentals/minimal-apis/overview
- Microsoft Learn — OpenAPI in ASP.NET Core (.NET 9+): https://learn.microsoft.com/en-us/aspnet/core/fundamentals/openapi/overview

## Changelog

### v2.1 — 2026-07-10
- Removed multi-tenant references — the tenancy-seam clauses in the validation (Rule 12) and MediatR (Rule 13) rules, the wildcard-tenant-subdomain CORS example, and the multi-tenant URL path examples; this collection no longer targets multi-tenant RLS.

### v2.0 — 2026-07-10
- Generalized for any .NET 10 project; removed project-specific mandates and decisions; endpoint organization aligned with the architecture selection framework.
