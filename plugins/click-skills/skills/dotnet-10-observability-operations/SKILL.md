---
name: dotnet-10-observability-operations
description: >
  Rules for Serilog structured logging (bootstrap + CLEF), health checks, background services,
  Options validation, HTTP resilience, and telemetry decisions in a .NET 10 Minimal API project.
  Trigger: add logging, configure Serilog, CLEF, log enrichment, request logging, health check,
  liveness probe, readiness probe, BackgroundService, IHostedService, ExecuteAsync, hosted service,
  Options pattern, ValidateOnStart, IValidateOptions, resilience, AddStandardResilienceHandler,
  telemetry, observability, OpenTelemetry, Scalar, docs/openapi.json, TimeProvider.
license: Apache-2.0
metadata:
  author: Zesh-One
  version: "2.1"
allowed-tools: Read Edit Write Bash Glob Grep
---

## When to Use

- Touching `Program.cs` (Serilog bootstrap/pipeline, health check registration, background service registration).
- Writing or reviewing any `*BackgroundService.cs`, `*HostedService.cs`, or `IHostedService` implementation.
- Defining or registering `Options/**/*Options.cs` / `*Validator.cs` configuration classes.
- Writing log statements inside endpoint handlers, services, or middleware.
- Adding typed `HttpClient` registrations with resilience handlers.
- Adding or changing health check endpoints, OpenAPI/Scalar setup, or `TimeProvider` usage.

## Critical Patterns

### Rule 1 — Do: bootstrap logger before `WebApplication.CreateBuilder`; Don't: start without one

**Why**: if `ValidateOnStart()` or host build fails, no structured output exists without the bootstrap logger.
The bootstrap must use `CompactJsonFormatter` (`Serilog.Formatting.Compact`) at minimum level Warning.

### Rule 2 — Do: call `builder.Host.UseSerilog()` before any `builder.Services.Add*`; Don't: register Serilog after other services

**Why**: DI-resolved services that use `ILogger<T>` receive the Serilog provider from the first resolution.
Serilog is the ONLY logging provider — do not add Microsoft console/debug providers alongside it.

### Rule 3 — Do: `CompactJsonFormatter` (CLEF) for all sinks; Don't: plain text or unstructured console

**Why**: CLEF is the machine-parseable structured log contract required for log aggregation and querying.
Configure log levels via `LoggerConfiguration`, not `appsettings.json`'s `Logging` section (it is not honored).

### Rule 4 — Do: suppress ASP.NET Core and EF Core noise via `MinimumLevel.Override`; Don't: leave default Info level

**Why**: without `Override("Microsoft.AspNetCore", Warning)` and `Override("Microsoft.EntityFrameworkCore.Database.Command", Warning)`, every request and every SQL query emits an Info event. Revert debug-level overrides before commit.

### Rule 5 — Do: `UseSerilogRequestLogging()` before endpoint routing, enriched with user subject + remote IP; Don't: rely on ASP.NET Core default request logging

**Why**: the default pipeline emits two non-CLEF events per request and breaks structured correlation. Place the middleware before routing so timing covers full handler execution. Never enrich with `PasswordHash`, `TotpSecret`, raw key material, session tokens, or any sensitive field.

**Exception:** placement may be moved before the rate-limiting middleware if you need to log rate-limit (429) outcomes — document the deviation with a comment at the call site. The canonical pipeline order is owned by the api-rest-minimal-apis skill.

### Rule 6 — Do: inject `ILogger<T>` via DI; Don't: use static `Log.Logger` directly

**Why**: static logger bypasses DI lifetime control and makes testing harder. Use `[LoggerMessage]` source generation on high-frequency hot paths; standard `logger.LogInformation(...)` is acceptable for low-frequency calls.

### Rule 7 — Do: named message template placeholders; Don't: string interpolation in log calls

**Why**: `logger.LogInformation("User {UserId} authenticated", userId)` captures a queryable structured field.
`$"User {userId} authenticated"` destroys structured capture and cannot be queried by field.

### Rule 8 — Do: NEVER log sensitive fields; no exceptions

**Why**: `PasswordHash`, `TotpSecret`, raw JWT private key, KEK values, SHA-256 token hashes, and session identifiers must be explicitly excluded from log scopes, enrichers, and diagnostic contexts. Security audit must cover log output, not just API responses.

### Rule 9 — Do: treat OpenTelemetry adoption as a deliberate, documented decision; Don't: bolt on telemetry packages ad hoc

**Why**: Serilog is the logging backbone of this stack. OpenTelemetry (traces/metrics) is the standard .NET observability option and is the recommended choice when distributed tracing or metrics are needed — adopt it as a deliberate decision, keep the instrumentation AOT/trimming-safe, and document the telemetry posture either way (what is collected, what is deliberately excluded) in the architecture notes.

### Rule 10 — Do: separate `/health/live` (tag `"live"`) and `/health/ready` (tag `"ready"`); Don't: mix liveness and readiness in one endpoint

**Why**: liveness is a self-check; readiness must verify every registered `DbContext` and each critical external dependency the project actually has — add Redis, queue, or external URL checks only when those dependencies exist. Detailed JSON health responses (check names, durations) must be gated behind an admin-level authorization policy; unauthenticated routes return the status string only.

### Rule 11 — Do: `IOptions<T>` as the default; escalate only when required

**Why**: `IOptions<T>` covers all configuration that does not change at runtime (JWT, Argon2, TOTP, connection metadata). Use `IOptionsSnapshot<T>` only for per-request reload; `IOptionsMonitor<T>` only for live `OnChange` notifications.

### Rule 12 — Do: `const string Section` property on every Options class; Don't: magic strings at registration

**Why**: self-documents `BindConfiguration` call sites and prevents binding path drift between registration and configuration files. For the full Options class shape (`const string Section`, `required init` props, `[Required]`/`[Range]`/`[StringLength]`), see the design-patterns skill.

### Rule 13 — Do: `ValidateDataAnnotations().ValidateOnStart()` on every load-bearing option; Don't: omit it without a documented reason

**Why**: options that are load-bearing at startup (JWT key paths, KEK configuration, connection strings, Argon2 parameters) must fail at startup, not on first use. Default to `ValidateOnStart()`; deliberate omission is the documented exception — justify why the defaults are safe with a comment at the registration site, never omit silently.
DataAnnotations (`[Required]`, `[Range]`, `[StringLength]`) are reserved exclusively for Options validation; request validation is hand-rolled through the Result pattern (see `../design-patterns/SKILL.md` and `../dotnet-conventions/SKILL.md`) — FluentValidation is not part of this stack.
Use `IValidateOptions<T>` (registered as singleton) for cross-property or environment-conditional logic that DataAnnotations cannot express.

### Rule 14 — Do: connection strings from environment variables only; Don't: put them in `appsettings*.json`

**Why**: connection strings and cryptographic key material in `appsettings.json` or `appsettings.Production.json` is a security violation regardless of git configuration.

### Rule 15 — Do: `AddHostedService<T>()` for background services; Don't: `AddSingleton<IHostedService, T>()`

**Why**: `AddHostedService` ensures proper `IHostedService` registration and shutdown ordering.

### Rule 16 — Do: inject `IServiceScopeFactory` and call `factory.CreateAsyncScope()` per unit of work; Don't: inject scoped services or `IServiceProvider` into the BackgroundService constructor

**Why**: `BackgroundService` is singleton-lifetime. Injecting a `DbContext` or any other scoped service via constructor creates a captive dependency. Create a scope per unit of work and dispose it when the work completes. See the performance skill for the DI-lifetime rule.

### Rule 17 — Do: propagate `stoppingToken` through every `await` inside `ExecuteAsync`; Don't: ignore or swallow cancellation

**Why**: loops that do not pass `CancellationToken` prevent clean shutdown. Handle `OperationCanceledException` by logging at Debug/Information level and returning — never swallow it.

### Rule 18 — Do: log unhandled exceptions at Error level and continue the loop; Don't: let exceptions crash the service or swallow them silently

**Why**: a crashed background service cannot recover without a process restart. Log with the exception object and relevant context identifiers (correlation ID, item ID), then continue.

### Rule 19 — Do: `AddStandardResilienceHandler()` for all typed HttpClient registrations; Don't: manual `AddPolicyHandler` chains

**Why**: the standard handler composes rate limiter, total timeout, retry with exponential backoff, circuit breaker, and per-attempt timeout in one call. Manual chains of 30+ lines are error-prone and miss the composite strategy. Use `AddStandardHedgingHandler()` only for latency-critical idempotent calls; document the idempotency guarantee with a comment.

Customize via the callback overload only when the defaults are demonstrably insufficient for a specific dependency (e.g., disabling auto-retry for non-idempotent calls) — and document why at the registration site.

### Rule 20 — Do: prefer BCL primitives for periodic work and in-process state; Don't: add caching, scheduling, or queuing infrastructure without a documented decision

**Why**: default to `BackgroundService` with `Task.Delay`/`PeriodicTimer` loops for simple periodic work; adopt Quartz.NET or Hangfire only when persistence, cron expressions, or distributed coordination are genuinely required. Caching (`IMemoryCache`/`HybridCache`/`IDistributedCache`) is permitted when justified by a real requirement — every such infrastructure dependency is a deliberate, documented design decision, never a drive-by addition. In-process deduplication uses `ConcurrentDictionary` or a bounded `Channel<T>`.

### Rule 21 — Do: `Channel<T>` (bounded, `FullMode.Wait`) for in-process producer/consumer flows; register as singleton

**Why**: producers (endpoint handlers) write to `ChannelWriter<T>`; consumers (BackgroundService) read via `ReadAllAsync(stoppingToken)`. Catch per-item exceptions inside the consumer loop and log at Error level before continuing.

### Rule 22 — Do: `TimeProvider` singleton for all time-dependent operations; Don't: call `DateTime.UtcNow` directly

**Why**: `FakeTimeProvider` is substituted in tests via conditional registration. Direct `DateTime.UtcNow` calls make time-dependent background service logic untestable.

### Rule 23 — Do: Scalar UI in non-production only; emit `docs/openapi.json` at build time via `Microsoft.Extensions.ApiDescription.Server`; Don't: use Swashbuckle or NSwag

**Why**: Scalar is the recommended OpenAPI UI (replacing Swashbuckle/NSwag). The committed `docs/openapi.json` is the artifact for external consumers; CI must fail if the committed document diverges from the generated one.

## Constraints & Tradeoffs

- Serilog is the SOLE logging provider. No other `ILoggerProvider` implementations alongside it.
- The `Logging` section of `appsettings.json` is inert — Serilog does not honor it; all level overrides go in `LoggerConfiguration`.
- `ValidateOnStart()` defaults to ON for load-bearing options: the startup-latency cost is accepted in exchange for failing fast. Deliberate omissions are documented exceptions at the registration site, never a silent posture.
- `IValidateOptions<T>` adds a singleton registration per Options type; use only when DataAnnotations are insufficient.
- `Channel<T>` with `FullMode.Wait` applies backpressure to producers; size the channel capacity to match expected throughput before production load testing.
- Tracing, caching, queues, and schedulers are deliberate architectural decisions, not defaults — adopt each only with a documented justification, and record deliberate exclusions in the project's architecture notes.

## Anti-Patterns

- Non-CLEF (plain text) log output in production sinks.
- Starting the host without a bootstrap logger (startup exceptions silently lost).
- Enabling both ASP.NET Core default request logging and `UseSerilogRequestLogging` (duplicate events per request).
- Logging `PasswordHash`, `TotpSecret`, JWT private key, KEK values, token hashes, or session identifiers.
- Silently omitting `ValidateOnStart()` on a load-bearing option without a documented reason at the registration site.
- Injecting scoped services into a `BackgroundService` constructor (captive dependency).
- Ignoring or swallowing `OperationCanceledException` inside `ExecuteAsync`.
- Writing manual Polly `AddPolicyHandler` chains instead of `AddStandardResilienceHandler()`.
- Connection strings or key material in any `appsettings*.json` file.
- Adding caching, scheduling, or queuing infrastructure without a documented architectural decision — these are deliberate adoptions, never drive-by dependencies.
- Calling `DateTime.UtcNow` directly instead of injecting `TimeProvider`.
- Using Swashbuckle or NSwag instead of `Microsoft.AspNetCore.OpenApi` + Scalar.

## Progressive Disclosure

1. **First**: `Critical Patterns` — operational rules, scan top to bottom for the relevant rule numbers.
2. **Architecture doubts**: `Constraints & Tradeoffs` — confirms deliberate constraints and tradeoffs.
3. **Code review**: `Anti-Patterns` — fast checklist for reviewers.
4. **Neighboring layers**: `Resources` below.

## Resources

- DI lifetimes, nullable, C# 14 patterns: [../dotnet-conventions/SKILL.md](../dotnet-conventions/SKILL.md)
- Endpoint registration, TypedResults, ProblemDetails: [../api-rest-minimal-apis/SKILL.md](../api-rest-minimal-apis/SKILL.md)
- JWT options validation, key-encryption (KEK) options, sensitive field exclusion: [../security/SKILL.md](../security/SKILL.md)
- Scoped DbContext lifecycle in background services: [../data-access-persistence/SKILL.md](../data-access-persistence/SKILL.md)
- Architecture selection (vertical-slice monolith vs modular monolith), infrastructure-dependency decisions: [../architecture/SKILL.md](../architecture/SKILL.md)
- FakeTimeProvider, Testcontainers-backed database tests, BackgroundService test patterns: [../testing/SKILL.md](../testing/SKILL.md)

## Changelog

### v2.1 — 2026-07-10
- Removed the multi-tenant tenant-isolation-seam background-service rule and the tenant-scoped cache-key requirement; renumbered the remaining rules; this collection no longer targets multi-tenant RLS.

### v2.0 — 2026-07-10
- Generalized for any .NET 10 project; removed project-specific mandates and decisions; telemetry, caching, and scheduling guidance made decision-oriented; multi-tenant RLS guidance made conditional; aligned with the architecture selection framework.
