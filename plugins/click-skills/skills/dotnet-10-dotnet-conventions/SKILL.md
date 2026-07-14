---
name: dotnet-10-dotnet-conventions
description: >
  C# 14 and .NET 10 code-level conventions covering language style, async, nullable, XML docs,
  naming, SOLID, Result pattern, and timestamp handling for AOT-first, minimal-dependency
  .NET 10 REST APIs.
  Trigger: any .cs or .csproj file read or modified; keywords nullable, async, await,
  extension block, field keyword, null-conditional assignment, DateTime.Now, TimeProvider,
  Result pattern, discriminated union, TypedResults, ProblemDetails, IOptions, ValidateOnStart,
  primary constructor; phrases indicating mapping (AutoMapper, Mapster), mediation (MediatR,
  IMediator), or validation libraries (FluentValidation, AbstractValidator).
license: Apache-2.0
metadata:
  author: Zesh-One
  version: "2.1"
allowed-tools: Read Edit Write Bash Glob Grep
---

## When to Use

- Reading, writing, or reviewing any `.cs`, `.csproj`, or `Directory.Build.props` file.
- Evaluating a library addition that involves mapping, mediation, or validation.
- Implementing error-handling flow with `Result` / discriminated unions or `ProblemDetails`.
- Adding configuration options, timestamps, async methods, or domain entities.
- Reviewing naming, nullable annotations, or XML documentation.

## Critical Patterns

### Project Settings

**Do**: set `LangVersion = latest`, `<Nullable>enable</Nullable>`, `<ImplicitUsings>enable</ImplicitUsings>` in `Directory.Build.props`.  
**Don't**: pin to a numeric `LangVersion` or override per-project without a documented comment.  
**Why**: pinning breaks C# 14 features; per-project overrides create drift.

### C# 14 Language Features

**Do**: use extension blocks for all new extension members (supports properties, indexers, operators, static members).  
**Don't**: write traditional `this`-parameter static extension methods for new code.  
**Why**: extension blocks are a strict superset; the old syntax can't express properties or indexers.

**Do**: use the `field` keyword inside a property accessor instead of a manual private backing field.  
**Don't**: declare `private T _field;` when the only use is backing the property.  
**Why**: eliminates boilerplate and keeps the property self-contained.

**Do**: use null-conditional assignment — `obj?.Prop = value;`, `list?[0] = value;`, `obj?.Counter += 1;`.  
**Don't**: write `if (obj != null) obj.Prop = value;`.  
**Why**: it's the C# 14 canonical form; the old pattern adds noise and misleads reviewers.

### Async / Cancellation

**Do**: `async`/`await` for all I/O; pass `CancellationToken` as the last parameter to every method and forward it to every awaitable call.  
**Don't**: block with `.Result`, `.GetAwaiter().GetResult()`, or `.Wait()`; don't write `async void` outside genuine event-handler signatures.  
**Why**: blocking causes deadlock risk on ASP.NET Core; `async void` swallows exceptions silently.

**Don't**: add `ConfigureAwait(false)` to ASP.NET Core application code.  
**Why**: the ASP.NET Core synchronization context is not an issue here; it belongs in library code only.

### TypedResults (not Results)

**Do**: always use `TypedResults` (not the static `Results` class) — see the api-rest-minimal-apis skill for the full rule including `Results<T1,T2>` union return types.

### Timestamps

**Do**: inject `TimeProvider` and call `provider.GetUtcNow()` in ALL injectable service and domain code. Use `DateTime.UtcNow` / `DateTimeOffset.UtcNow` directly ONLY in non-injectable scaffolding (e.g. migration seed data) where DI is structurally impossible.  
**Don't**: use `DateTime.Now` anywhere; don't call `DateTime.UtcNow` directly in service/domain code.  
**Why**: `DateTime.Now` is timezone-dependent; `TimeProvider` + `FakeTimeProvider` keeps time deterministic in tests.

### Result / Discriminated Unions

**Do**: model expected business errors as `sealed record` discriminated unions with typed case variants. Match exhaustively with switch expressions; do not suppress incomplete-match warnings when a new case is added.  
**Don't**: throw exceptions for expected outcomes; don't suppress compiler warnings when a new `Result` variant is added.  
**Why**: exceptions are not flow control; compile-time exhaustiveness is the safety net.

**Critical security constraint**: `Failure` variants on authentication/enumeration-sensitive paths MUST carry no reason string — scope and shape defined in the design-patterns skill (Result shape).

### Error Responses

**Do**: use `ProblemDetails` (RFC 7807) for all HTTP error responses — minimum fields incl. `traceId` defined in the api-rest-minimal-apis skill. Centralize database-error-to-ProblemDetails translation in a single exception-mapper component (e.g., a `PostgresExceptionMapper` when the project uses PostgreSQL) — see the design-patterns skill.  
**Don't**: scatter ad-hoc error-to-response translation outside the mapper.

### Validation

**Do**: validate inputs inline inside service methods using imperative guards that return a typed `Result` variant — the hand-rolled default of this skill.  
**Don't**: let pipeline validators (`FluentValidation`, `AbstractValidator`, `AddValidation()`) auto-emit `ValidationProblem(errors)` responses on auth, MFA/TOTP, password-reset, or enumeration-sensitive endpoints — this is a hard rule; elsewhere pipeline validators merely contradict the hand-rolled default and need a documented reason.  
**Why**: (a) auto-emitted `ValidationProblem(errors)` 400 responses leak account-enumeration signal; (b) validation that needs database access belongs inside the transaction/data-access seam, not a filter pipeline; (c) when the database enforces uniqueness via constraints, constraint errors are the authoritative uniqueness source — map them centrally through the exception mapper.  
Exception: `DataAnnotations` + `ValidateDataAnnotations()` remain acceptable ONLY for `IOptions<T>` configuration validation at startup.

### Options Pattern

**Do**: define config sections as `sealed` classes with `const string Section`, `required init` properties, and `DataAnnotations` constraints. Register security-critical options with `.ValidateDataAnnotations().ValidateOnStart()`; for non-security options it is recommended but a deliberate omission may be documented inline — see the observability-operations skill for the selective-validation policy.  
**Don't**: omit `ValidateOnStart()` silently — document the omission with an inline comment when it is intentionally absent.  
**Do**: use `IOptions<T>` by default; `IOptionsSnapshot<T>` for per-request reload; `IOptionsMonitor<T>` only when live `OnChange()` callbacks are needed.

### Object Mapping

**Do**: map manually. Use positional record constructors at the call site for response DTOs (compile-time exhaustiveness). Use EF Core `.Select()` projection for read queries (pushes column selection to SQL, AOT-safe). Extract a static `ToDto()` helper only when the same mapping appears in three or more places.  
**Don't**: introduce `AutoMapper`, `Mapster`, or any reflection-based convention mapper.  
**Why**: AOT-incompatible; convention mapping silently exposes sensitive fields (e.g., password hashes, MFA/TOTP secrets, raw key material, session identifiers). All mapping MUST explicitly exclude sensitive fields.

### Orchestration

**Do**: inject typed service interfaces directly via DI parameter injection in endpoint handlers — the default orchestration shape.  
**Don't**: introduce `MediatR`, `IMediator`, `IRequest/IRequestHandler`, or `IPipelineBehavior`.  
**Why**: (a) MediatR v13+ requires a commercial license above a revenue threshold; (b) its open-generic reflection registration is AOT/trimming-hostile. If a team explicitly wants the mediator shape, the only acceptable form is a source-generated, MIT-licensed mediator (e.g., `martinothamar/Mediator`) — see Constraints & Tradeoffs.

### Domain Model

**Do**: let `Domain/` entities carry behavior and invariants by default (standard Clean Architecture), configured via an explicit `IEntityTypeConfiguration<T>` per entity. Match DDD tactical investment (aggregates, value objects, domain events) to actual domain complexity — no rituals for simple CRUD.  
**Don't**: run mandatory DDD rituals on simple CRUD models. Don't add `IRepository<T>`, `IUnitOfWork`, or generic repository wrappers.  
**Why**: DDD tactical patterns pay off proportionally to domain complexity. `DbContext` already implements unit-of-work; define narrow, purpose-built ports instead of generic wrappers.

### Nullable Correctness

**Do**: annotate every reference-type parameter, return type, and property. Suppress `!` only when the reference is verifiably non-null; add an inline comment on the same line explaining why.  
**Don't**: leave unannotated references or suppress warnings without justification.

### Naming & Structure

- Types: `PascalCase`; private fields: `_camelCase`; locals/parameters: `camelCase`; constants and `static readonly`: `PascalCase`; interfaces: `I`-prefixed.
- `async` methods: suffix `Async` unless the method is a Minimal API handler delegate.
- Test methods: `MethodName_Condition_ExpectedResult`. Project-defined traceability prefixes (e.g., mapping tests to a security checklist) may override the pattern — see the `testing` skill's naming section.
- File-scoped namespaces required for all new files. No block-scoped `namespace` declarations in new code.
- `sealed` on all non-abstract classes unless inheritance is explicitly planned and documented. All `Result` case records must be `sealed`.
- Primary constructor syntax for injected dependencies; list each dependency on its own line when there are multiple. **Exception:** Primary constructors are the default for injected dependencies in NEW code. A traditional constructor is accepted when the type unwraps `IOptions<T>.Value` into fields or initializes static fields in the body. Do NOT refactor existing services to primary constructors for style alone — apply only where clean and in new code.
- `required` modifier on properties that must be set at construction; combine with `init` for immutable DTOs.

### Other Code Quality

- `IReadOnlyList<T>` / `IReadOnlyCollection<T>` / `IEnumerable<T>` for public return types — never mutable concrete collections.
- `decimal` for all monetary values; document the rounding mode inline when a monetary amount is computed.
- `StringBuilder` or `string.Join` for string construction in loops — never `+=` concatenation.
- `using var` or `using` declarations for all `IDisposable` / `IAsyncDisposable` resources.
- Structured logging only: inject `ILogger<T>` via DI; use message templates with named placeholders, never string interpolation. Never log passwords, tokens, MFA/TOTP secrets, or raw key material.
- `BackgroundService.ExecuteAsync`: handle `CancellationToken` and exit gracefully; catch and log exceptions inside the background loop.
- `nameof()` for exception argument names and property names in validation; use `nameof(List<>)` (unbound generic form) in C# 14 when the type name is needed without a specific argument.
- XML documentation required on all public types and members — at minimum a `<summary>` that adds meaning, not "Gets or sets X" boilerplate. Add `<param>`, `<returns>`, `<remarks>` when the contract is non-obvious.
- DI lifetimes must be deliberate and annotated with an inline comment. Singletons must be thread-safe. Scoped services must not be injected into singletons; use `IServiceScopeFactory` when a singleton genuinely needs a scoped collaborator.

## Constraints & Tradeoffs

- **Architecture layout is per-project**: a single-assembly vertical-slice monolith (folders as layers) and a multi-project Clean/Hexagonal or modular-monolith split are both valid layouts. Pick one deliberately using the architecture skill's selection framework, keep layer boundaries explicit either way, and don't let a layout choice smuggle in rich DDD or extra dependencies.
- **If you relax a default**: if a mediator is ever justified, use an MIT-licensed, AOT-compatible, source-generated one (`martinothamar/Mediator`) — never MediatR v13+. Mappers are never relaxed — map manually (no AutoMapper/Mapster in any mode). Pipeline validation may only apply to purely structural, non-security endpoints, and only after verifying the `ValidationProblem(errors)` output matches the ProblemDetails contract.
- **AOT / trimming safety**: all patterns must remain AOT-safe. Reflection-based mappers, MediatR, and convention-based approaches are hard blockers for AOT.
- **Exception vs. Result**: infrastructure errors (DB unavailable, network failure) may propagate as exceptions caught by `ProblemDetails` middleware. Business-flow errors always use the `Result` pattern.
- **HttpClient**: if outbound HTTP is added, always use `IHttpClientFactory`; register typed clients with `AddHttpClient<TInterface, TImpl>().AddStandardResilienceHandler()`. Never instantiate `new HttpClient()` per request.

## Anti-Patterns

- `DateTime.Now` anywhere; `DateTime.UtcNow` in injectable/testable service code without `TimeProvider`.
- Traditional `this`-parameter extension methods for new code (use extension blocks).
- Manual backing field when `field` keyword applies.
- `if (obj != null) obj.Prop = value;` instead of `obj?.Prop = value;`.
- `Results.Ok()` instead of `TypedResults.Ok()`.
- `.Result` / `.GetAwaiter().GetResult()` / `.Wait()` on async code.
- `async void` outside genuine event handlers.
- Exceptions as flow control for expected business outcomes.
- Empty `catch` or catch-log-continue without rethrowing or returning an error `Result`.
- Scoped service injected into a Singleton (captive dependency).
- Mutable shared state in Singletons without thread-safety (`Interlocked`, `ConcurrentDictionary`).
- Missing nullable annotations or unexplained `!` suppressions.
- Missing XML documentation on public types and members.
- `AutoMapper`, `Mapster`, or any reflection-based mapper.
- `MediatR` / `IMediator` (commercial-license and AOT/trimming blockers).
- Pipeline validators (`FluentValidation`, `AddValidation()`) that auto-emit `ValidationProblem` on auth/enumeration-sensitive endpoints; elsewhere they contradict this skill's hand-rolled validation default.
- `IRepository<T>`, `IUnitOfWork`, or generic repository wrappers (`DbContext` is already a unit of work; use narrow purpose-built ports).
- N+1 patterns: loading a collection then querying per item in a loop.
- Mutable concrete collections in public return types.
- `string +=` concatenation in loops.
- `IDisposable` without a `using` statement.
- `ConfigureAwait(false)` in ASP.NET Core application code.
- `float` or `double` for monetary values.

## Progressive Disclosure

1. **Start here**: `Critical Patterns` — operational decisions for any `.cs` edit.
2. **Before a library decision** (mapper, mediator, validator): re-read `Constraints & Tradeoffs` for the library-decision defaults and their acceptable alternatives.
3. **In review**: `Anti-Patterns` as a fast checklist.
4. **Layer boundaries**: consult sibling skills when the task crosses into API shape, data access, security, or testing.

## Resources

- API shape, TypedResults wiring, OpenAPI metadata: [../api-rest-minimal-apis/SKILL.md](../api-rest-minimal-apis/SKILL.md)
- Transaction seams, EF Core query patterns: [../data-access-persistence/SKILL.md](../data-access-persistence/SKILL.md)
- Authentication and authorization: [../security/SKILL.md](../security/SKILL.md)
- FakeTimeProvider, xUnit, Testcontainers, test naming: [../testing/SKILL.md](../testing/SKILL.md)
- Architecture selection framework and layout options: [../architecture/SKILL.md](../architecture/SKILL.md)
- Result pattern, Options, DI lifetimes deep-dive: [../design-patterns/SKILL.md](../design-patterns/SKILL.md)

## Changelog

### v2.1 — 2026-07-10
- Removed the context-runner / tenant-scoping seam references and the anemic-because-the-database-is-the-invariant-enforcer framing from the Orchestration and Domain Model sections; this collection no longer targets multi-tenant RLS.
  - Removed the Mapster source-generator carve-out from Constraints & Tradeoffs to align with the collection-wide mapper ban.

### v2.0 — 2026-07-10
- Generalized for any .NET 10 project; removed project-specific mandates and decisions; layout and multi-tenant guidance aligned with the architecture selection framework.
