---
name: dotnet-10-performance
description: >
  Performance and efficiency rules for .NET 10 / EF Core / Minimal API backends,
  covering async correctness, EF query patterns, allocation management, HTTP resilience, channels,
  and connection-pool safety.
  Trigger: performance, efficiency, allocation, AsNoTracking, N+1 query, projection, Select DTO,
  HttpClient, IHttpClientFactory, AddStandardResilienceHandler, Channel, backpressure, BoundedChannel,
  StringBuilder, Span, async void, blocking async, GetAwaiter, .Result, .Wait, CancellationToken,
  stoppingToken, Interlocked, ConcurrentDictionary, caching, IMemoryCache, connection pool, AddDbContextPool.
license: Apache-2.0
metadata:
  author: Zesh-One
  version: "2.1"
allowed-tools: Read Edit Write Bash Glob Grep
---

## When to Use

- Adding or reviewing any async I/O path (database, HTTP, channels, background services).
- Writing or reviewing EF Core queries — read-only lookups, projections, includes.
- Registering or consuming HTTP clients.
- Implementing producer-consumer pipelines with `System.Threading.Channels`.
- Reviewing singleton services for thread-safety or captive-dependency issues.
- Any code that touches allocation-heavy paths: string building, binary/token processing, shared counters.

## Critical Patterns

### Rule 1 — Do: async all the way down; Don't: block with `.Result`, `.GetAwaiter().GetResult()`, `.Wait()`

**Why**: blocking in ASP.NET Core deadlocks the synchronization context and holds thread-pool threads during I/O waits.
All endpoint handlers and service methods that perform I/O MUST return `Task` or `ValueTask`.

### Rule 2 — Do: use explicit, well-scoped transactions for multi-statement work; Don't: run multi-statement work without an explicit transaction

**Why**: multi-statement work without an explicit transaction leaves the transaction lifecycle unmanaged and can
leave partial writes on failure. Batch the statements of a unit of work into one explicit, well-scoped transaction.

### Rule 3 — Do: share the ambient transaction context; Don't: nest transaction scopes

**Why**: nesting transaction scopes — an inner COMMIT/ROLLBACK inside an outer transaction — can commit or roll
back partial work and corrupt the outer unit of work. Services that need DB access must share the caller's context
instead of opening a new one.

### Rule 4 — Do: `AsNoTracking()` + `Select()` projection for read-only queries; Don't: load full entities and map after

**Why**: change tracking for entities that will not be mutated wastes memory and CPU. The projection pushes
column selection to SQL, reduces allocations, and is AOT-safe. Canonical pattern:

```csharp
db.Users.AsNoTracking().Select(u => new UserDto(u.Id, u.Email)).ToListAsync(ct)
```

### Rule 5 — Do: manual mapping with positional record constructors or EF `Select()`; Don't: use AutoMapper or Mapster

**Why**: AutoMapper and Mapster are banned. Reflection-based mappers break AOT trimming, can silently include or
drop sensitive fields (`PasswordHash`, `TotpSecret`, session identifiers), and are not auditable. Positional record
constructors give compile-time exhaustiveness; EF `Select()` pushes column selection to SQL. Extract a static
`ToDto()` helper only when the same mapping appears in three or more places. Never expose sensitive fields in
response types.

Note: explicit EF configuration registration (vs `ApplyConfigurationsFromAssembly`, which uses reflection) is the
AOT-safe choice and is the default in this skill set — see the `data-access-persistence` skill.

### Rule 6 — Do: use `Include()`/`ThenInclude()` or a single projection join; Don't: loop over a collection and issue per-item queries

**Why**: N+1 generates one SQL round-trip per item. Each round-trip holds a pooled connection for the duration of
the transaction and degrades throughput under concurrent load.

### Rule 7 — Do: `AddHttpClient` / typed client registration; Don't: `new HttpClient()` in a service or constructor

**Why**: directly instantiated `HttpClient` exhausts socket descriptors under concurrent load due to TIME_WAIT.

### Rule 8 — Do: attach `AddStandardResilienceHandler()` to every external HTTP client; Don't: stack Polly policies by hand

**Why**: `AddStandardResilienceHandler()` bundles rate-limiter, total timeout, exponential-backoff retry (429/5xx),
circuit breaker, and per-attempt timeout in one call. Manual Polly stacking produces 30+ lines of error-prone
wiring. Override individual strategy parameters (e.g., `options.Retry.MaxRetryAttempts`) only when the default is
demonstrably wrong for the integration.

**Partial adoption for non-idempotent operations:** `AddStandardResilienceHandler()` is the default for typed
HttpClients, but for non-idempotent operations (e.g., transactional email sends) enable timeout and circuit-breaker
WITHOUT automatic retry unless idempotency is guaranteed — retries can duplicate side effects, such as delivering
an activation or reset email twice. Apply resilience only where needed and where it does not compromise delivery
semantics.

### Rule 9 — Do: `Channel.CreateBounded<T>(BoundedChannelFullMode.Wait)`; Don't: use unbounded channels by default

**Why**: an unbounded channel consumes unbounded memory when producers outpace consumers. Register the channel as a
singleton; expose `ChannelWriter<T>` to producers and `ChannelReader<T>` to the `BackgroundService` consumer.

### Rule 10 — Do: `StringBuilder.Append()` or `string.Join()` for loop concatenation; Don't: `+` in loops

**Why**: `+` in a loop produces O(n²) heap allocations. Short known-count concatenations (2–3 operands outside a
loop) may use `$` string interpolation — the C# 14 compiler generates an efficient interpolated string handler.

### Rule 11 — Do: `Span<T>` / `ReadOnlySpan<T>` for binary and token processing; Don't: allocate intermediate byte arrays

**Why**: Span-based APIs avoid heap allocations for slices. Especially relevant in JWT parsing, TOTP
shared-secret handling, and key derivation paths that execute on every authenticated request.

### Rule 12 — Do: `Interlocked.*` for singleton counters; `ConcurrentDictionary` for singleton collections; Don't: plain `++`/`--` on shared fields

**Why**: plain increment is not atomic under concurrent requests. Applies to singleton-scoped mutable state in
general — e.g., rotation or metrics counters.

### Rule 13 — Do: inject `IServiceScopeFactory` in singletons; Don't: inject scoped services directly into singletons

**Why**: captive dependency — the scoped service outlives its intended lifetime, causing stale data and memory leaks.
Use `await using var scope = factory.CreateAsyncScope()` per unit of work. Applies to all `BackgroundService`
implementations.

### Rule 14 — Do: pass `CancellationToken` through every async signature; Don't: omit it or pass `CancellationToken.None` in production code

**Why**: omitting `ct` makes requests non-cancellable, holds resources after client disconnects, and prevents
graceful shutdown of `BackgroundService`. In `BackgroundService.ExecuteAsync`, propagate `stoppingToken` to all
inner calls, including `reader.ReadAllAsync(stoppingToken)`.

### Rule 15 — Do: `timeProvider.GetUtcNow()`; Don't: `DateTime.Now`, `DateTime.UtcNow`, or `DateTimeOffset.UtcNow` directly

**Why**: an injected `TimeProvider` (registered as a singleton) enables deterministic time-sensitive tests via
`FakeTimeProvider`; direct clock reads bypass it and make tests non-deterministic.

### Rule 16 — Do: `async Task` / `async ValueTask`; Don't: `async void`

**Why**: exceptions from `async void` cannot be caught by the caller and can crash the process silently. The sole
exception is an ASP.NET Core event-handler delegate with a framework-imposed `void` signature — in that case the
body MUST be wrapped in `try-catch` that logs via Serilog.

### Rule 17 — Do: `using var` / `await using var` for disposables; Don't: call `.Dispose()` manually at end of method

**Why**: manual disposal on the success path does not execute when an exception is thrown before it.

## Constraints & Tradeoffs

- **Caching (measured need only)**: prefer BCL primitives first; add `IMemoryCache`, `HybridCache`, or a
  distributed cache only with a measured need and a documented design decision. Never cache session-validation
  results when immediate revocation requires a live check.
- **AOT/trimming posture (first-class)**: this skill set treats AOT/trimming compatibility as a first-class
  constraint. Any API relying on runtime reflection — `ApplyConfigurationsFromAssembly`, reflection-based mappers
  (AutoMapper/Mapster), open-generic reflection DI registration, convention-based discovery — is avoided because
  trimming cannot statically analyze reflection targets. This is the underlying reason for the mapper rule and the
  explicit EF-config-registration rule. It is also why MediatR is not recommended: its open-generic reflection
  registration is AOT/trimming-hostile, and v13+ carries a commercial license above a revenue threshold. Prefer
  direct typed-service DI injection; if a team explicitly wants the mediator shape, use a source-generated
  MIT-licensed mediator (e.g., `martinothamar/Mediator`).
- **Npgsql connection pool**: pooling is managed by the Npgsql provider — do not disable it or reduce pool size
  without profiling evidence. If connection interceptors mutate session state, ensure that
  state is reset when connections return to the pool, and verify any interceptor change preserves this invariant.
  Never call `NpgsqlConnection.ClearAllPools()` outside Testcontainers test teardown.
- **`AddDbContextPool` (measure first)**: evaluate `AddDbContextPool` over `AddDbContext` only when throughput
  profiling shows benefit. Pooled `DbContext` instances are reused across requests, so no interceptor or callback
  may store per-request state on the context — audit any scoped interceptors first, as they break silently under
  pooling. See the `data-access-persistence` skill for EF configuration conventions.
- **Compiled queries**: `EF.CompileAsyncQuery` is an escape hatch for measured hot paths only — not a blanket rule.
- **CPU-intensive password hashing**: if the project performs CPU-intensive password hashing (e.g., Argon2id with
  recommended parameters such as 64 MB memory and 3 iterations), it is CPU-bound by design — it must be properly
  awaited or offloaded and must never block a thread-pool thread that could serve requests.

## Anti-Patterns

- Blocking on async: `.Result`, `.GetAwaiter().GetResult()`, `.Wait()`.
- `async void` outside mandatory framework event-handler signatures.
- Multi-statement work executed without an explicit transaction where consistency is required.
- Nesting transaction scopes — inner COMMIT/ROLLBACK can commit or roll back partial work in the outer unit.
- Full entity load for read-only endpoints (missing `AsNoTracking()` + `Select()`).
- N+1: per-item child queries inside a loop instead of `Include()` or a join projection.
- Reflection-based mappers: AutoMapper or Mapster (any mode) — reflection-based mappers banned; break AOT, hide sensitive fields.
- `new HttpClient()` in service constructors or per-request — socket exhaustion.
- Manual Polly policy stacking instead of `AddStandardResilienceHandler()`.
- `Channel.CreateUnbounded<T>()` without documented justification — unbounded memory risk.
- String `+` concatenation in loops — O(n²) allocations.
- Plain `++`/`--` on singleton shared fields — race condition under concurrent requests.
- Scoped service injected directly into a singleton (captive dependency).
- `DateTime.Now` / `DateTime.UtcNow` directly — bypasses `TimeProvider`/`FakeTimeProvider`.
- Caching session-validation results that must support immediate revocation.
- `NpgsqlConnection.ClearAllPools()` outside test teardown.
- Omitting `CancellationToken` from async signatures; using `CancellationToken.None` in `BackgroundService`.
- Manual `.Dispose()` call at end of method without `using` declaration.

## Progressive Disclosure

1. **First**: `Critical Patterns` — one rule per concern, each with a Why.
2. **If design constraints are unclear**: `Constraints & Tradeoffs` for design constraints and tradeoffs.
3. **If reviewing existing code**: `Anti-Patterns` as a fast checklist.
4. **If adjacent layers are affected**: follow cross-links in `Resources`.

## Resources

- EF Core query conventions, DbContext configuration, and transaction patterns: [../data-access-persistence/SKILL.md](../data-access-persistence/SKILL.md)
- Security boundaries: [../security/SKILL.md](../security/SKILL.md)
- Async patterns and DI lifetimes: [../dotnet-conventions/SKILL.md](../dotnet-conventions/SKILL.md)
- Architecture selection framework and BackgroundService patterns: [../architecture/SKILL.md](../architecture/SKILL.md)
- Endpoint handler async signatures and CancellationToken threading: [../api-rest-minimal-apis/SKILL.md](../api-rest-minimal-apis/SKILL.md)
- Deterministic time control (`FakeTimeProvider`) and test-tier strategy: [../testing/SKILL.md](../testing/SKILL.md)
- Result pattern, exception-mapping strategy, and manual-mapping rationale: [../design-patterns/SKILL.md](../design-patterns/SKILL.md)

## Changelog

### v2.1 — 2026-07-10
- Removed the multi-tenant RLS transaction/tenant-context-seam framing from the transaction-scope rules, the
  tenant-scoped cache-key requirement, and the `SET ROLE` interceptor example; the general transaction-scope and
  connection-pool guidance is retained; this collection no longer targets multi-tenant RLS.

### v2.0 — 2026-07-10
- Generalized for any .NET 10 project; removed project-specific mandates and decisions; multi-tenant runner/RLS
  guidance made conditional and the caching prohibition replaced with dependency-minimalism guardrails.
