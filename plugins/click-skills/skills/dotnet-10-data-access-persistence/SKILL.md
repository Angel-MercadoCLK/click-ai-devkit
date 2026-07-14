---
name: dotnet-10-data-access-persistence
description: >
  EF Core 10 + Npgsql data-access rules for .NET 10 REST APIs: DbContext registration,
  entity configuration, query patterns, migrations, raw SQL, and testing against real
  Postgres.
  Trigger: DbContext, EF Core, IEntityTypeConfiguration, migration, Npgsql, AsNoTracking,
  .Select() projection, raw SQL, ExecuteSqlRawAsync, BackgroundService data access.
license: Apache-2.0
metadata:
  author: Zesh-One
  version: "2.1"
allowed-tools: Read Edit Write Bash Glob Grep
---

## When to Use

- Adding or modifying any code that reads or writes data: services, queries, or commands.
- Creating or configuring entities, `IEntityTypeConfiguration`, or any DbContext.
- Authoring or editing migrations for any DbContext in the solution.
- Writing integration tests against real Postgres (Testcontainers).
- Adding raw SQL — inline in internal services or behind a port, if raw-SQL ports are used.
- Any `BackgroundService` that needs data access.

## Critical Patterns

### Rule 1 — Do: register each `IEntityTypeConfiguration<T>` explicitly; Don't: rely on `ApplyConfigurationsFromAssembly`

- **Do**: register each `IEntityTypeConfiguration<T>` explicitly via
  `builder.ApplyConfiguration(new XxxConfiguration())` — this is the preferred default in this
  skill set and AOT-safe.
- **Don't**: rely on `ApplyConfigurationsFromAssembly(...)` — it is reflection-based and conflicts
  with the AOT/trimming posture.

**Why**: inline `OnModelCreating` grows unboundedly and conflates two concerns; the assembly scan
relies on reflection that AOT/trimming strips at publish time.

### Rule 2 — Do: `AsNoTracking()` + `.Select()` projection for reads; Don't: load full tracked entities unless mutating

Project to DTOs in the query (reduces wire columns, avoids allocating unmutated entities, AOT-safe).
Load tracked full entities only when the same transaction will write them back.

### Rule 3 — Do: let exactly one abstraction own the transaction boundary; Don't: open a second transaction inside an active one

One component owns transaction demarcation — never call `BeginTransactionAsync` inside an
already-active transaction scope. Design service interactions so a unit of work fits in one
transaction scope.

### Rule 4 — Do: map Postgres constraint errors to typed `Result` variants via a single exception mapper; Don't: throw exceptions for expected business violations

Convert `23505` (unique), `P0001` (custom raise), `23514` (check) to typed `Result` / sealed
record variants through one dedicated exception-mapper component that translates Postgres
SQLSTATE codes. Use the discriminated-union `Result` pattern (e.g., `CreateOrderResult.Success`,
`CreateOrderResult.DuplicateNumber`) for expected data-access outcomes. Do not surface raw
`PostgresException` to callers.

### Rule 5 — Do: use `AddDbContext` (not `AddDbContextPool`) while scoped interceptors are active; Don't: switch to pooling without auditing interceptors first

`DbContextPool` recycles instances and does NOT run the full per-request DI lifecycle. Any
scoped interceptor — e.g., a scoped connection interceptor that sets per-request session state
— will break silently under pooling.

### Rule 6 — Do: put complex raw SQL behind a port interface with an Npgsql adapter when it warrants isolated testing; Don't: treat the port as a blanket requirement

`db.Database.SqlQueryRaw<T>()` / `ExecuteSqlRawAsync()` directly inside `internal sealed`
services is acceptable otherwise — a dedicated SQL-executor port is OPTIONAL; reach for it only
when complex SQL benefits from isolated testing.

### Rule 7 — Do: map manually with positional record constructors or `.Select()` projections; Don't: use AutoMapper, Mapster, or any reflection mapper

Sensitive fields (`PasswordHash`, `TotpSecret`, raw key material) must be explicitly excluded at
the callsite. Reflection mappers silently include or drop fields; misconfiguration is
undetectable in code review. Extract a static `ToDto()` helper only when the same mapping appears
in three or more places.

### Rule 8 — Do: use `TimeProvider` singleton in interceptors and services; Don't: call `DateTime.UtcNow` directly

`FakeTimeProvider` can then control timestamps in real-Postgres (`RealPg`-tier) tests.

### Rule 9 — Do: read connection strings from environment variables or a secret store; Don't: put them in committed configuration files

Use environment variables or a secret store — user-secrets in development, a vault or managed
identity in production — never `appsettings.json` or any committed configuration file. Options
classes MUST use `ValidateDataAnnotations()` + `ValidateOnStart()` for fail-fast
misconfiguration detection.

### Rule 10 — Do: create a DI scope per work unit in `BackgroundService`; Don't: inject a scoped DbContext into the constructor

Scoped DbContext injected into a singleton `BackgroundService` is captured for the lifetime of
the process, producing stale contexts and connection leaks.

### Rule 11 — Conditional (range-partitioned tables): Do: update the partition pre-creation service in the same migration set as any schema change; Don't: rely on Postgres automatic partition creation

If a table is range-partitioned and a service (e.g., a `BackgroundService`) pre-creates future
partitions, update that service **in the same migration set** as any change to the table's
columns or partition scheme. Postgres never creates partitions automatically; a schema change
without a matching service update leaves the partition boundary logic stale.

**Why**: the partition pre-creation service and the table schema are tightly coupled — the
failure only surfaces when inserts hit the next unmapped range boundary, which makes it
invisible in review and immediate testing.

### Rule 12 — Do: cover every provider-specific feature with a real-Postgres test and a comment; Don't: use Postgres arrays, JSONB, or range types without one

When entity configurations use PostgreSQL/Npgsql provider-specific features (arrays, JSONB
columns, range types, or other Npgsql extensions), the behavior MUST be covered by a
`RealPg`-tier test (see the testing skill) — not an InMemory test, which will silently ignore
or mishandle provider-specific mappings. Document the choice with an inline comment explaining
why the feature was selected.

**Why**: InMemory silently accepts configurations it cannot enforce; a passing InMemory test for
a JSONB column proves nothing about serialization, querying, or null handling in real Postgres.

### Rule 13 — Do: apply migrations as a controlled CI/CD step; Don't: run `db.Database.MigrateAsync()` at startup

**Why**: startup migration surrenders deployment control (no review or rollback gate), races
when multiple instances start concurrently, and couples schema change to process boot. Apply
migrations as an explicit, ordered deployment step with a rollback path.

## Constraints & Tradeoffs

- **One transaction scope per unit of work.** Batch all operations of a unit of work into a
  single transaction scope. Design service interactions so a unit of work fits in one scope.
- **Choose the domain-model style deliberately.** Entity behavior and invariants in the domain
  layer are the standard Clean Architecture default; match DDD tactical investment to actual
  domain complexity — no rituals for simple CRUD. Either way, keep orchestration in
  application-layer services, not in entities.
- **`HasQueryFilter` is bypassable — know its limits.** `HasQueryFilter` is a valid
  application-level mechanism for cross-cutting filters (e.g., soft delete), but it is bypassable
  via `IgnoreQueryFilters()` — never treat it as a security boundary. Give each filter a comment
  documenting its purpose.
- **`decimal` for all monetary fields.** Never `float` or `double`.

## Anti-Patterns

- `IRepository<T>`, `IUnitOfWork`, or generic DbContext wrappers — they duplicate DbContext/DbSet
  semantics and add no seam value.
- `UseInMemoryDatabase` for any test exercising provider-specific behavior, transactions,
  constraints, or any security-enforcing data path — use Testcontainers Postgres instead.
  Wiring/routing-only tests (WAF tier) are the narrow exception — see the testing skill's
  permitted-InMemory rule.
- `DbContextPool` with scoped interceptors.
- `AutoMapper`, `Mapster`, or any reflection mapper — sensitive-field exposure risk.
- `MediatR` — commercial license for v13+ above the revenue threshold, and its open-generic
  reflection registration is hostile to AOT/trimming. Prefer direct typed-service DI injection;
  if a team explicitly wants the mediator shape, use a source-generated MIT-licensed mediator
  (e.g., `martinothamar/Mediator`).
- `db.Database.MigrateAsync()` in `Program.cs` — surrenders deployment control and races under
  multi-instance startup (Rule 13).
- Connection strings in committed configuration files or hardcoded literals.
- (Conditional, partitioned tables) changing a range-partitioned table's columns or partition
  scheme without updating the partition pre-creation service in the same migration set.
- Npgsql-specific features (arrays, JSONB, range types) in entity configurations left untested
  by a `RealPg`-tier test — InMemory silently accepts configurations it cannot enforce.

## Progressive Disclosure

1. **Start here**: `Critical Patterns` — query patterns, transaction ownership, and the most
   common pitfalls.
2. **If you are unsure about transaction scope**: re-read Rule 3 and `Constraints & Tradeoffs`.
3. **If you are writing or reviewing migrations**: Rule 13 (no startup migration).
4. **If neighboring layers are affected**: cross-link to `Resources` below.

## Resources

- Security (authentication, secret handling): [../security/SKILL.md](../security/SKILL.md)
- Architecture (architecture selection — vertical-slice monolith vs. modular monolith, dependency rules): [../architecture/SKILL.md](../architecture/SKILL.md)
- .NET 10 conventions (C# 14 records, nullable, Options pattern, TimeProvider): [../dotnet-conventions/SKILL.md](../dotnet-conventions/SKILL.md)
- Testing (RealPg tier, Testcontainers, permitted-InMemory scope): [../testing/SKILL.md](../testing/SKILL.md)
- REST endpoints and their data-access integration (ProblemDetails, TypedResults): [../api-rest-minimal-apis/SKILL.md](../api-rest-minimal-apis/SKILL.md)
- Result pattern, SQLSTATE-to-Result exception mapping, ports-and-adapters: [../design-patterns/SKILL.md](../design-patterns/SKILL.md)

## Changelog

### v2.1 — 2026-07-10
- Removed the multi-tenant PostgreSQL RLS runner rule, the separate-DbContexts isolation rule, the
  RLS session-variable / `SET ROLE` / BYPASSRLS anti-patterns, and the
  `HasQueryFilter`-for-tenant-isolation and anemic-because-the-database-is-the-invariant-enforcer
  framing; renumbered the remaining rules; this collection no longer targets multi-tenant RLS.

### v2.0 — 2026-07-10
- Generalized for any .NET 10 project; removed project-specific mandates and decisions;
  multi-tenant RLS/runner guidance made conditional and aligned with the architecture
  selection framework.
