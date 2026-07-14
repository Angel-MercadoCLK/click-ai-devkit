---
name: dotnet-10-architecture
description: >
  Architecture selection and project-organization guidance for .NET 10 REST APIs: choosing
  between a Vertical Slice Monolith with Clean Architecture separation and a Modular Monolith
  with module assemblies, plus layering, domain-model richness, data-access seams, and
  orchestration style.
  Trigger: starting a new project or module; adding a layer, project, or assembly;
  where does logic go; should I use a repository, IUnitOfWork, MediatR, or aggregate root;
  Clean Architecture; Hexagonal Architecture; Vertical Slice; modular monolith structure;
  DbContext transaction; Application layer; Program.cs organization; endpoint organization.
license: Apache-2.0
metadata:
  author: Zesh-One
  version: "2.1"
allowed-tools: Read Edit Write Bash Glob Grep
---

## When to Use

- Starting a greenfield .NET 10 REST API and choosing its overall structure.
- Adding a service class and deciding where it lives.
- Evaluating whether to split an assembly, introduce a new layer or module, or add a NuGet package.
- Implementing data access and deciding how to demarcate transactions.
- Adding a new endpoint, BackgroundService, or middleware.
- Evaluating whether canonical Clean/DDD patterns (aggregates, repositories, mediator) fit the project at hand.
- Assessing a brownfield codebase: work within its architecture or restructure it.

## Critical Patterns

### Architecture selection — two primary options

Greenfield projects — and fragile brownfield projects being restructured — choose by size.

#### Option A — Vertical Slice Monolith with Clean Architecture separation (small-to-medium projects)

**Do**: build a single deployable with a single application assembly (plus test project(s)).
**Do**: organize code by feature (vertical slices): `Features/{FeatureName}/` holds the endpoint mapping, handler/service, request/response DTOs, and validation for that feature.
**Do**: inside each slice, respect clean-architecture responsibility separation: endpoint (transport) → application logic (handler/service) → domain (entities/rules) → infrastructure (persistence, external services). Shared `Domain/` and `Infrastructure/` folders hold cross-feature building blocks.
**Do**: honor the dependency rule — domain code never depends on infrastructure. In a single assembly this is enforced by discipline and code review, not the compiler; that is the accepted tradeoff.
**Do**: when a slice grows into a real bounded context, promote it to an Option B module — the slice organization makes this an extraction, not a rewrite.

#### Option B — Modular Monolith: vertical-slice modules as assemblies with Clean Architecture (large projects)

**Do**: build a single deployable with multiple assemblies: each module = one bounded context = its own assembly pair (`Modules.{Name}` + `Modules.{Name}.Contracts`).
**Do**: organize each module internally as vertical slices with clean-architecture separation (Option A applied per module).
**Do**: make the module's public surface its Contracts assembly (public interfaces, DTOs, integration events). Other modules reference ONLY Contracts, never module internals — compile-time boundary enforcement via assembly references, optionally reinforced with architecture tests (e.g., NetArchTest, ArchUnitNET).
**Do**: isolate data per module — schema-per-module (or table prefix) in the shared database; no cross-module joins; cross-module data flows through module contracts or in-process events.
**Do**: communicate via direct contract-interface calls for synchronous needs and in-process integration events for decoupled workflows. No network hops.
**Why**: microservice-style boundaries (independent development, clear ownership, extractability) WITHOUT distributed-systems costs (network failures, eventual consistency, deployment orchestration). A module can later be extracted to a standalone service by replacing its in-process contract implementation with a network client.

#### Sizing heuristics

- **Option A**: one team, one or few bounded contexts, primarily a CRUD+/REST API.
- **Option B**: multiple teams or clearly separable bounded contexts, domains with distinct ownership, or a roadmap that may require extracting services later.
- When in doubt, start with Option A; promote to Option B when a second bounded context stabilizes.

#### Brownfield rule — respect solid existing architecture

**Do**: if the codebase already has a solid, consistently applied Clean or Hexagonal architecture, work within the established structure and its conventions.
**Don't**: restructure a solid existing architecture toward Option A/B.
**Do**: restructure ONLY when the existing architecture is fragile and mixed — unclear boundaries, business logic scattered across layers, cross-layer coupling, god classes, inconsistent patterns.
**Do**: when restructuring, migrate incrementally (strangler-fig style) toward the option that matches project size AND has the shortest distance from the existing layout.
**Don't**: attempt a big-bang rewrite.

### Layer boundaries and enforcement

**Do**: in a single-assembly layout (Option A), treat `Domain/`, `Infrastructure/`, and feature folders as descriptive navigation vocabulary; enforce layer boundaries via code review or architecture tests.
**Don't**: assume folder names enforce anything at compile time, or bolt on ad-hoc analyzer rules and `Directory.Build.props` targets to simulate compile errors for layer violations.
**Why**: compile-time enforcement requires assembly boundaries — that is Option B, a deliberate architectural decision, not a lint rule.

**Do**: when extracting a dedicated Application layer out of existing code, define ONE narrow boundary abstraction toward Infrastructure and perform the extraction in a single consistent pass.
**Don't**: extract partially, leaving orchestration logic split across two homes.

### Domain model — rich vs anemic

**Do**: let domain entities carry behavior and invariants by default (standard Clean Architecture); configure persistence via `IEntityTypeConfiguration<T>` per entity. Match DDD tactical investment (aggregates, value objects, domain events) to actual domain complexity.
**Do**: place orchestration and business logic in the dedicated service/Application layer of whichever architecture the project chose (the slice handler in Option A, the module's application services in Option B).
**Don't**: run mandatory DDD pre-implementation rituals (aggregate enumeration, domain events, ubiquitous-language inventories) when the domain model is intentionally behavior-free or the domain is simple CRUD.
**Why**: DDD tactical patterns pay off proportionally to domain complexity; ritual checklists generate noise with no safety gain on behavior-free models.

### Data access seam — DbContext, repositories, unit of work

**Do**: use `DbContext` directly as the data-access seam by default — it already implements the repository and unit-of-work patterns.
**Don't**: wrap it in generic `IRepository`, `IRepository<T>`, or `IUnitOfWork` layers by reflex — these are usually redundant abstraction.
**Do**: treat Repository + UoW as a legitimate architectural choice when there is a real driver (swappable persistence, a hard testing seam), with its tradeoffs documented.

### Orchestration — direct service injection

**Do**: inject typed service interfaces (`IAuthService`, `IOrderService`, etc.) directly into Minimal API endpoint handlers via DI parameter injection.
**Don't**: introduce MediatR, `IMediator`, `IRequest`/`IRequestHandler`, or `IPipelineBehavior`.
**Why** (stack-level blockers, not preferences): (1) MediatR v13+ requires a commercial license above the $5M revenue threshold. (2) MediatR uses open-generic reflection registration — AOT/trimming-hostile, a hard blocker for .NET 10 trimming. (3) Its exception-throwing pipeline behaviors fight typed `Result` discriminated unions.
**Do**: if a team explicitly wants the mediator shape, use a source-generated, MIT-licensed, AOT-compatible implementation (e.g., `martinothamar/Mediator`) instead.

### Inter-module communication — service and contract interfaces

**Do**: give each feature/module ownership of its entities. When one concern needs data owned by another, call the owner's interface: in Option A, a DI-injected service interface enforced by code review and folder discipline; in Option B, the owning module's Contracts interface enforced at compile time.
**Don't**: use direct `DbContext` access to entities that semantically belong to another concern.
**Don't**: introduce an in-process event bus or integration events ad hoc — adopt them as an explicit design decision (decoupled cross-module workflows in Option B are the primary use case).
**Why**: crossing into another concern's entities without going through its interface bypasses the ownership contract and couples persistence details across concerns. Where there is no compile-time boundary, code review must catch services that query another concern's entities directly.

### Entity configuration

**Do**: implement `IEntityTypeConfiguration<T>` per entity, registered explicitly per entity (`builder.ApplyConfiguration(new XxxConfiguration())`) — explicit registration is AOT-safe; the reflection-based assembly scan (`ApplyConfigurationsFromAssembly`) is not. See the data-access-persistence skill for details.
**Don't**: put column/relation configuration inline in `OnModelCreating`. Don't use data annotations for EF column config (data annotations are allowed only for Options/configuration validation via `ValidateDataAnnotations` + `ValidateOnStart`).

### Object mapping — manual only

**Do**: map entity-to-DTO manually using positional record constructors at the call site or EF LINQ `.Select()` projections for read queries. Extract a static `ToDto()` helper only when the same mapping appears in three or more places.
**Don't**: introduce AutoMapper, Mapster, or any reflection-based or convention-based object mapper.
**Why**: explicit mapping is verifiable in code review and cannot silently expose sensitive fields (`PasswordHash`, `TotpSecret`, raw key material).

### Result pattern and error handling

**Do**: use sealed record discriminated unions for expected flow outcomes (e.g., a `LoginResult` with cases `Success`, `Failure`, `ChallengeRequired` — illustrative names).
**Do**: when using PostgreSQL, map database constraint violations (e.g., Npgsql SQLSTATE `23505` unique, `23514` check) to typed result variants through a dedicated exception mapper (a `PostgresExceptionMapper`-shaped class) instead of leaking raw provider exceptions.
**Don't**: throw exceptions for expected business-rule outcomes. Don't let failure cases for authentication carry a reason message (account-enumeration oracle risk).

### HTTP responses

**Do**: use `TypedResults` (not `Results`) for all endpoint return values. Declare explicit return type unions (`Results<Ok<T>, NotFound>`) so all reachable HTTP status codes appear in the OpenAPI schema.
**Do**: return RFC 7807 `ProblemDetails` for all HTTP errors including middleware short-circuit responses. Use `TypedResults.Problem()` / `TypedResults.ValidationProblem()`.

### API versioning and OpenAPI tooling

**Do**: use URL-segment versioning: `/api/v{N}/resource`. Use `Microsoft.AspNetCore.OpenApi` (built-in) for schema generation and `Scalar.AspNetCore` for the UI.
**Don't**: use query-string or header versioning. Don't add Swashbuckle.AspNetCore or NSwag.
**Why**: the built-in generator is first-party and AOT-safe; Scalar provides the interactive UI without introducing a second schema-generation stack.

### Endpoint organization

**Do**: organize endpoints in per-feature/per-resource groups from the start — `MapGroup`, `MapXxxEndpoints` extension classes, and `AddXxxModule` registration (per feature folder in Option A; per module in Option B).
**Do**: pick ONE endpoint-organization style and apply it consistently across the codebase.
**Don't**: mix inline `Program.cs` endpoints with extracted endpoint groups — a partial mix of styles is harder to navigate than either extreme.

### Infrastructure cross-cutting rules

**Do**: use `TimeProvider` (singleton DI) for all current-time reads. Use Serilog (`ILogger<T>` via DI) for structured logging with compact JSON CLEF in production.
**Don't**: call `DateTime.UtcNow` / `DateTimeOffset.UtcNow` directly in service or endpoint code. Don't use `Console.WriteLine` / `Debug.WriteLine`.

**Do**: adopt OpenTelemetry for distributed tracing and metrics when the deployment needs them — it is the standard .NET instrumentation stack and pairs well with Serilog for logging. If adopted, verify exporter AOT/trimming compatibility. Adoption is a per-project decision.

**Do**: treat security middleware (security headers, CSRF protection, CORS policy, rate limiting) as load-bearing infrastructure — never remove or bypass it during feature work. Evaluate every new endpoint accepting unauthenticated input for rate-limit coverage.
**Do**: source connection strings and cryptographic material from environment variables or a dedicated secret store (user-secrets in development, vault/managed identity in production) — never from committed config files. Never commit secrets to the repository.

## Constraints & Tradeoffs

### Choosing between the options — decision axes

| Axis | Option A (single assembly) | Option B (module assemblies) |
|------|----------------------------|------------------------------|
| Boundary enforcement | Code review + architecture tests | Compile-time via assembly references |
| Build/solution complexity | Minimal | Higher (two projects per module) |
| Team scaling | One team | Multiple teams with clear ownership |
| Path to services | Promote slice to module first | Replace contract impl with network client |

Splitting or merging assemblies is neither mandated nor banned — it is an architectural decision. Make it deliberately, document it, and apply it consistently.

### Domain-model richness

Rich domain models — entities carrying behavior and invariants — are the default (standard Clean Architecture). Match DDD tactical investment (aggregates, value objects, domain events) to actual domain complexity — no rituals for simple CRUD.

### Data-access seam

`DbContext`-direct is the default (it already is a repository + unit of work). Repository + UoW is a legitimate choice with real drivers and documented tradeoffs.

### Orchestration style

Direct typed-service DI injection is the default. If a genuine driver for a mediator exists, prefer a source-generated, MIT-licensed, AOT-compatible implementation (e.g., `martinothamar/Mediator`); MediatR v13+ is disqualified by its commercial license and reflection-based registration on AOT/trimming paths.

### Dependency minimalism

Prefer BCL primitives first. Add caching (`IMemoryCache`, `HybridCache`, distributed caches), scheduling (Hangfire, Quartz.NET), or queuing packages only with a documented design decision backed by a real requirement.

## Anti-Patterns

- Splitting or merging assemblies without a deliberate, documented architectural decision — the undocumented drift is the anti-pattern, not the split itself.
- Introducing MediatR (commercial license above the revenue threshold; open-generic reflection registration breaks AOT/trimming).
- Wrapping `DbContext` in redundant generic `IRepository<T>`/`IUnitOfWork` layers with no real driver.
- Introducing AutoMapper or Mapster in ANY mode, or any reflection-based mapper (reflection breaks AOT trimming; sensitive-field exposure risk).
- Introducing FluentValidation on authentication/TOTP/enumeration-sensitive endpoints (pipeline validation leaks account-enumeration signal).
- Adding Swashbuckle.AspNetCore or NSwag.
- Mixing endpoint-organization styles (some endpoints inline in `Program.cs`, some extracted) — pick one style and apply it everywhere.
- Measuring line-coverage targets against behavior-free persistence POCOs — it is meaningless; when the domain is anemic, weight correctness assurance toward integration tests through production seams. Coverage targets remain valid for projects with real domain behavior.
- Querying another concern's or module's entities directly instead of calling its service/contract interface (bypasses the ownership contract).

## Progressive Disclosure

1. **Critical Patterns** first — architecture selection, placement, data-access seams, mapping, and HTTP responses.
2. **Constraints & Tradeoffs** when choosing between architecture options (assembly layout, domain-model richness, data-access seam, orchestration style, dependencies).
3. **Anti-Patterns** as a pre-commit or code-review checklist.
4. **Resources** when the work touches a neighboring domain (data access detail, security, API contracts).

## Resources

- DbContext configuration, transactions, migration workflow: [`../data-access-persistence/SKILL.md`](../data-access-persistence/SKILL.md)
- Authentication, authorization, session management: [`../security/SKILL.md`](../security/SKILL.md)
- TypedResults, endpoint organization, versioning, OpenAPI: [`../api-rest-minimal-apis/SKILL.md`](../api-rest-minimal-apis/SKILL.md)
- Options pattern, DI lifetimes, async, nullable, C# 14 features: [`../dotnet-conventions/SKILL.md`](../dotnet-conventions/SKILL.md)
- Test tiers, Testcontainers integration testing, FakeTimeProvider, xUnit: [`../testing/SKILL.md`](../testing/SKILL.md)
- Result pattern, discriminated unions, exception mapping, conditional service selection: [`../design-patterns/SKILL.md`](../design-patterns/SKILL.md)
- BackgroundService lifecycle, Serilog, OpenTelemetry, health checks: [`../observability-operations/SKILL.md`](../observability-operations/SKILL.md)
- Async I/O, query projections, N+1 prevention: [`../performance/SKILL.md`](../performance/SKILL.md)

## Changelog

### v2.1 — 2026-07-10
- Removed the multi-tenant PostgreSQL RLS runner pattern, the separate-DbContexts-per-data-domain isolation section, and the anemic-because-the-database-is-the-invariant-enforcer framing; this collection no longer targets multi-tenant RLS.

### v2.0 — 2026-07-10
- Generalized for any .NET 10 project; removed project-specific mandates and decisions; architecture selection framework (Option A / Option B / brownfield rule) added.
