---
name: dotnet-10-design-patterns
description: >
  Design-pattern rules for .NET 10 backends (REST APIs): Result
  discriminated unions, Options pattern, DI lifetimes, keyed services,
  and exception mapping.
  Trigger: Result pattern, discriminated union, sealed record cases, Options
  pattern, IOptions, ValidateOnStart, DI lifetime,
  PostgresExceptionMapper, keyed services, FromKeyedServices,
  Factory pattern, BackgroundService, MediatR, IRepository, AutoMapper, FluentValidation.
license: Apache-2.0
metadata:
  author: Zesh-One
  version: "2.1"
allowed-tools: Read Edit Write Bash Glob Grep
---

## When to Use

- Add or modify DI service registrations (`Program.cs`, `AddXxx` extensions).
- Define or consume error-return types (`Result` variants, discriminated unions, `ProblemDetails`).
- Add or modify configuration-binding classes (`IOptions`, `IOptionsSnapshot`, `IOptionsMonitor`).
- Write any code that touches the project's transactional/data-access seam, if one exists.
- Add service abstraction boundaries, `BackgroundService` implementations, or keyed-service registrations.
- Review or write tests that exercise any of the above seams.

## Critical Patterns

### Rule 1 — Do: use sealed record discriminated unions for expected failures; Don't: throw exceptions for expected outcomes

**Why**: security-sensitive flows (login, TOTP, registration) use typed results so failure reasons are never
leaked to callers. Exceptions are reserved for unexpected infrastructure failures.

Canonical shape (`LoginResult` is an example type name):

```csharp
// ✅ correct
public abstract record LoginResult
{
    public sealed record Success(Guid UserId, string AccessToken) : LoginResult;
    public sealed record Failure : LoginResult;            // No reason — prevents enumeration
    public sealed record ChallengeRequired(string ChallengeId) : LoginResult;
}
```

Switch-expression mapping at the endpoint boundary:

```csharp
return result switch
{
    LoginResult.Success s   => TypedResults.Ok(new TokenResponse(s.AccessToken)),
    LoginResult.ChallengeRequired c => TypedResults.Ok(new ChallengeResponse(c.ChallengeId)),
    LoginResult.Failure     => TypedResults.Problem(/* generic 401 */),
    _                       => throw new UnreachableException()
};
```

### Rule 2 — Do: direct typed service injection in endpoint handlers; Don't: introduce a mediator bus

**Why**: avoid `IMediator`/MediatR — it carries a commercial license for v13+ above the revenue threshold,
and its open-generic reflection registration is AOT/trimming-hostile. Direct typed service injection in
Minimal API handlers is simpler and fully AOT-safe. If in-process dispatch is genuinely needed and a team
explicitly wants the mediator shape, use a source-generated, AOT-compatible mediator such as
`martinothamar/Mediator` (MIT).

### Rule 3 — Do: validate through the Result pattern, with DB-dependent checks inside the transaction scope; Don't: let pipeline validation auto-emit 400s on enumeration-sensitive endpoints

**Why**: database-dependent checks (uniqueness, scoped reads) must run inside the same transaction scope as
the operation they guard, not in pre-handler filters where they run against different transactional state.
Auto-emitted `ValidationProblem(errors)` 400 responses before the handler executes create account-enumeration
oracles on login, TOTP, password-reset, and registration flows. Built-in `AddValidation()` with
DataAnnotations is acceptable for purely structural, non-security endpoints (e.g., pagination bounds);
everywhere else, hand-roll validation and flow outcomes through the Result pattern.

### Rule 4 — Do: manual positional-record construction or EF `.Select()` projection; Don't: use AutoMapper or Mapster

**Why**: reflection-based mappers silently drop or include fields on misconfiguration; in codebases handling
sensitive fields (e.g., password hashes, TOTP secrets, JWT claims), that is unacceptable. Compile-time
exhaustiveness on positional records means the compiler flags incomplete call sites when a new required
property is added. EF `.Select()` projections push column selection to SQL and are AOT-safe.

Extract a static `ToDto()` method only when the same mapping appears in three or more places.
Use a mapper class only for mappings with five or more fields and conditional logic.

### Rule 5 — Do: in PostgreSQL projects, centralize constraint-error mapping in one exception-mapper component; Don't: catch `PostgresException` inline

**Why**: if the project uses PostgreSQL, codes `23505` (unique violation), `P0001` (custom RAISE), and
`23514` (check violation) must produce consistent typed `Result` variants. Scattered inline catches produce
divergent error shapes. Centralize `PostgresException`-to-`Result` mapping in a single component (e.g., a
`PostgresExceptionMapper` class); when introducing a new constraint, extend that mapper — never add inline
`catch (PostgresException)` in services or endpoints.

### Rule 6 — Do: `IOptions<T>` for startup-time config with `ValidateDataAnnotations().ValidateOnStart()` for security-critical or high-risk options; Don't: defer validation without justification

**Why**: deferred validation fails at first use — misconfiguration should fail fast at startup.
`IOptionsSnapshot<T>` is scoped; injecting it into a singleton is a captive-dependency bug.
Use `IOptionsMonitor<T>` only when live-reload with `OnChange()` is actually needed.
Options classes must declare `const string Section`, `required` properties, `[Required]`, and `[Range]`/`[StringLength]` where applicable.
Use `ValidateDataAnnotations().ValidateOnStart()` for security-critical or high-risk options; for low-risk options it is recommended but a documented omission is acceptable.

### Rule 7 — Do: document DI lifetime and reason in a code comment; Don't: register silently

**Why**: without a comment stating `Singleton: stateless cipher` or `Scoped: wraps DbContext`,
future agents reregister at the wrong lifetime. Never register a longer-lived service with a
shorter-lived dependency without `IServiceScopeFactory`.

### Rule 8 — Do: register conditional implementations (real vs NoOp) at startup; Don't: branch internally after construction

**Why**: environment-dependent behaviour (e.g., live email vs NoOp, real rate limiter vs kill-switch passthrough)
must be resolved at DI registration time in `Program.cs` or a dedicated `AddXxx` extension, not via
runtime `if (env.IsDevelopment())` inside the service body.

### Rule 9 — Do: use `TypedResults` in endpoint handlers; Don't: use `Results`

**Why**: `Results.Ok()` is untyped and breaks built-in OpenAPI schema generation. `TypedResults.Ok()` etc.
produce strongly-typed return metadata required for accurate endpoint documentation.

### Rule 10 — Do: use `BackgroundService` + `IServiceScopeFactory` for recurring infrastructure tasks; Don't: inject scoped services directly

**Why**: `BackgroundService.ExecuteAsync` runs in a singleton context; injecting scoped services directly
causes a captive-dependency error at resolution time. Each iteration must create its own scope and handle
its own exceptions — an unhandled exception in `ExecuteAsync` silently terminates the hosted service.

### Rule 11 — Do: document deliberate deviations at the deviation point; Don't: leave them uncommented

**Why**: when code deliberately deviates from a standard pattern, leave a comment referencing the ADR,
ticket, or design decision that motivated it, so future agents and developers do not revert the deviation
without understanding its rationale.

## Constraints & Tradeoffs

- Rich domain models per Clean/Hexagonal architecture are the default: entities may carry behavior and
  invariants, configured via an explicit `IEntityTypeConfiguration<T>` per entity. Match DDD tactical
  investment (aggregates, value objects, domain events) to actual domain complexity — no rituals for simple
  CRUD. Keep orchestration in application-layer services, not in entities.
- Assembly layout is a per-project decision: a single-assembly vertical-slice monolith with
  folder/namespace boundaries and a modular monolith with per-module assemblies (or a multi-project Clean
  Architecture split) are both valid. Pick one deliberately per the architecture selection framework in
  [`../architecture/SKILL.md`](../architecture/SKILL.md), document it, and enforce boundaries with the
  chosen mechanism (architecture tests or project references). Do not restructure assemblies mid-project
  without an explicit decision.
- Express CQRS read/write separation lightly — `AsNoTracking()` + `.Select()` projections for reads,
  explicit transactional writes. Do not add a mediator or a separate read-model layer without a concrete
  driver.
- Keyed services (`AddKeyedScoped`, `FromKeyedServices`) are for genuine multiple-implementation
  scenarios only. They are NOT a mediator dispatch table.
- `IValidateOptions<T>` is the standard mechanism for cross-property or conditional config validation when
  `DataAnnotations` alone are insufficient. Keep options classes as plain data containers.
- Behavioral/integration tests against database-specific behaviour use Testcontainers with the real
  database engine. EF Core InMemory is permitted only for DI-wiring/structural tests that never issue a
  SQL query.

## Anti-Patterns

- `MediatR`, `IMediator`, `IRequest<T>`, `IPipelineBehavior` — avoid (commercial license for v13+ above the revenue threshold; open-generic reflection registration is AOT/trimming-hostile). Prefer direct typed service injection; if a mediator shape is explicitly wanted, use `martinothamar/Mediator` (MIT, source-generated).
- `IRepository<T>`, `IUnitOfWork`, generic repository wrappers — EF Core's `DbContext` already implements repository/unit-of-work; add generic repository abstractions only with a concrete driver.
- Pre-handler pipeline validation that auto-emits 400s on enumeration-sensitive endpoints (login, TOTP, password-reset, session-refresh, registration), or pipeline validation for DB-dependent checks — banned. Hand-rolled Result-based validation is the default; built-in `AddValidation()` with DataAnnotations only for purely structural, non-security endpoints.
- AutoMapper or Mapster (any mode), or any reflection-based mapper — banned (sensitive-field exposure; reflection breaks AOT).
- Restructuring assemblies — splitting a single assembly into Domain/Application/Infrastructure/WebApi projects or merging a multi-project split — without an explicit, documented architecture decision — banned. Both layouts are valid when chosen deliberately.
- `EF Core InMemory` for behavioral or security tests — banned. InMemory is not relational: it cannot model constraints, transactions, isolation, or provider-specific behavior. Use Testcontainers with the real engine.
- `Results.Ok()` and other untyped `Results.*` helpers in endpoint handlers — banned.
- `IOptionsSnapshot<T>` injected into singleton services — captive-dependency bug.
- A failure case on enumeration-sensitive paths (login, TOTP, password-reset, session-refresh, registration) carrying a `reason`, `code`, or `message` field — banned (enumeration oracle). E.g., a `LoginResult.Failure` case MUST carry no such field.

## Progressive Disclosure

1. **First**: `Critical Patterns` — every rule has a security or correctness rationale; read them all before writing any service or endpoint.
2. **If touching data access**: cross-read `../data-access-persistence/SKILL.md` for DbContext, migration, and Testcontainers test-tier details.
3. **If touching auth or JWT**: cross-read `../security/SKILL.md` for auth, JWT signing, and enumeration-prevention rules.
4. **If adding endpoints or ProblemDetails responses**: cross-read `../api-rest-minimal-apis/SKILL.md`.
5. **If writing or reviewing tests**: cross-read `../testing/SKILL.md` for test-tier definitions and Testcontainers fixture sharing rules.
6. **If in doubt about assembly layout or module boundaries**: cross-read `../architecture/SKILL.md` for the architecture selection framework and boundary enforcement options.

## Resources

- DbContext, migrations, Testcontainers test tiers: [`../data-access-persistence/SKILL.md`](../data-access-persistence/SKILL.md)
- Auth, JWT, TOTP, account-enumeration prevention: [`../security/SKILL.md`](../security/SKILL.md)
- C# 14 features, naming, Nullable, ImplicitUsings: [`../dotnet-conventions/SKILL.md`](../dotnet-conventions/SKILL.md)
- Endpoint declaration, versioning, ProblemDetails, OpenAPI/Scalar: [`../api-rest-minimal-apis/SKILL.md`](../api-rest-minimal-apis/SKILL.md)
- Test tiers, Testcontainers, FakeTimeProvider, xUnit Assert: [`../testing/SKILL.md`](../testing/SKILL.md)
- Architecture selection framework (vertical-slice monolith vs modular monolith), boundary enforcement: [`../architecture/SKILL.md`](../architecture/SKILL.md)

## Changelog

### v2.1 — 2026-07-10
- Removed the multi-tenant PostgreSQL RLS / context-runner seam rule and its anti-patterns, and the anemic-because-the-database-is-the-invariant-enforcer framing; renumbered the remaining rules; this collection no longer targets multi-tenant RLS.

### v2.0 — 2026-07-10
- Generalized for any .NET 10 project; removed project-specific mandates and decisions; runner/RLS and Postgres exception-mapping guidance made conditional; aligned with the architecture selection framework.
