---
name: backend-dataaccess
description: >
  EF Core data access conventions for ASP.NET Core 8 APIs with operational defaults for repositories,
  DbContext lifecycle, query behavior, transactions, and raw SQL safety.
  Trigger: When implementing or reviewing repositories, DbContext registration/configuration,
  read/write queries, migrations, or stored procedure access in backend services.
license: Apache-2.0
metadata:
  author: Zesh-One
  version: "3.0"
allowed-tools: Read Edit Write Bash Glob Grep
---

## When to Use

- Create or modify EF Core repositories.
- Register/configure `DbContext` in `Program.cs`.
- Review query performance (`AsNoTracking`, includes, hot paths).
- Implement transactions, automatic auditing, and soft delete.
- Execute raw SQL or stored procedures with parameters.

## Critical Patterns

### Rule 1 — Do: `AddDbContextPool`; Don’t: `AddDbContext`

**Why**: in this codebase, `DbContext` reuse is prioritized to reduce GC pressure and latency under concurrency.

### Rule 2 — Do: separate EF pool vs ADO.NET pool

**Why**: EF `poolSize` does not control SQL connections; `SqlConnectionStringBuilder.MaxPoolSize` does. Mixing them causes incorrect tuning.

### Rule 3 — Do: `AsNoTracking()` on reads; Don’t: default tracking in read-only queries

**Why**: tracking on reads increases memory and CPU usage with no benefit when there is no `SaveChanges` in the same scope.

### Rule 4 — Do: use `AsNoTrackingWithIdentityResolution()` on complex graphs

**Why**: it avoids duplicate entities in deep includes without re-enabling global tracking.

### Rule 5 — Do: explicit `Include/ThenInclude`; Don’t: assume lazy loading

**Why**: backend convention is lazy loading disabled; navigating properties without include breaks at runtime.

### Rule 6 — Do: repository returns entities; Don’t: return DTO/Response envelope

**Why**: DTO mapping belongs to the service layer. Layer contract: repository = domain, service = DTO/result.
Cross-ref: [`../responses/SKILL.md`](../responses/SKILL.md), [`../mapping/SKILL.md`](../mapping/SKILL.md).

### Rule 7 — Do: one `SaveChangesAsync` per atomic unit; Don’t: multiple saves without a transaction

**Why**: two saves without a transaction can leave partial state after an intermediate failure.

### Rule 8 — Do: wrap manual transactions with `CreateExecutionStrategy()` when retries are enabled

**Why**: with `EnableRetryOnFailure`, a manual transaction outside the strategy can throw `InvalidOperationException`.

### Rule 9 — Do: parameterized SQL (`SqlParameter` / safe interpolation); Don’t: concatenate strings

**Why**: concatenation in `FromSqlRaw` opens SQL Injection attack surface.

```csharp
// ✅ safe
var rows = await _context.Database.SqlQueryRaw<UserSummaryResult>(
    "EXEC sp_GetSummary @Id",
    new SqlParameter("@Id", id)
).ToListAsync(ct);

// ❌ unsafe
var rows2 = await _context.Users.FromSqlRaw($"EXEC sp_GetSummary '{id}'").ToListAsync(ct);
```

### Rule 10 — Do: automatic auditing with an interceptor; Don’t: set `CreatedAt/UpdatedAt` manually in every handler

**Why**: it reduces human error and keeps temporal consistency.

### Rule 11 — Do: soft delete with `HasQueryFilter`; Don’t: mix physical/logical deletion without policy

**Why**: without a single policy, repositories diverge and inconsistent reads appear.

### Rule 12 — Do: descriptive migration names; Don’t: `AutoMigration_*`

**Why**: readable history speeds up rollback, auditing, and deployment debugging.

## Constraints & Tradeoffs

- `IEntityTypeConfiguration<T>` is the default for new entities; in legacy contexts, do not do mass rewrites just for style.
- Compiled queries (`EF.CompileAsyncQuery`) only on measured hot paths; do not use them as a premature global rule.
- Atomic transactions across multiple `DbContext` instances are NOT guaranteed without a distributed coordinator; use Outbox for cross-context consistency.
- Soft delete adds complexity to indexes and administrative queries (`IgnoreQueryFilters`); use it only when the domain requires it.

## Anti-Patterns

- `AddDbContext` without pooling.
- Read queries without `AsNoTracking`.
- Repositories returning DTO or `ResponseDTO<T>`.
- Multiple `SaveChangesAsync` calls in a flow without an explicit transaction.
- Dynamic SQL via unsafe interpolation/concatenation.
- Migrations with opaque or automatic names.
- Assuming lazy loading for entity navigation.

## Progressive Disclosure

Read only what is necessary for the problem:

1. **First**: `Critical Patterns` (operational decisions).
2. **If there are architecture doubts**: `Constraints & Tradeoffs`.
3. **If you are reviewing legacy code**: `Anti-Patterns` for a quick checklist.
4. **If neighboring layers are impacted**: review related skills in `Resources`.

## Resources

- Layer response/contract: [../responses/SKILL.md](../responses/SKILL.md)
- Endpoint security and ownership: [../security/SKILL.md](../security/SKILL.md)
- Backend performance (resilience/concurrency): [../performance/SKILL.md](../performance/SKILL.md)
- Mapping between domain and DTO: [../mapping/SKILL.md](../mapping/SKILL.md)

## Changelog

### v3.0 — 2026-04-21
- Complete rewrite as an operational skill: less theory, atomic Do/Don’t + Why rules, structure aligned to the rubric.
