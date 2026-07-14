---
name: backend-performance
description: >
  Operational performance defaults for ASP.NET Core 8 APIs focused on concurrency, outbound resilience, and runtime limits.
  Trigger: When tuning latency/throughput, configuring Polly resilience, sizing Kestrel limits, or parallelizing independent async work.
license: Apache-2.0
metadata:
  author: Zesh-One
  version: "2.0"
allowed-tools: Read Edit Write Bash Glob Grep
---

## When to Use

- Optimizing hot endpoints with measurable latency bottlenecks.
- Defining outbound HTTP resilience and connection behavior.
- Reviewing async orchestration and parallel execution safety.
- Setting operational Kestrel limits before production rollout.

## Critical Patterns

### Core Rules (Do/Don't + Why)

- **Do**: Parallelize independent I/O with `Task.WhenAll`. **Why**: serial awaits inflate end-to-end latency.
- **Don't**: Parallelize dependent steps. **Why**: this hides sequencing bugs and raises recovery complexity.
- **Do**: Use one named resilience pipeline per external dependency. **Why**: isolates failures and avoids cross-service false trips.
- **Don't**: Use a single global circuit breaker for all outbound APIs. **Why**: one unstable dependency can throttle healthy ones.
- **Do**: Prefer ratio-based breakers with `minimumThroughput`. **Why**: prevents circuit open on low-volume noise.
- **Do**: Set `PooledConnectionLifetime` on `SocketsHttpHandler`. **Why**: refreshes DNS targets and reduces stale connections.
- **Do**: Keep retries bounded and exponential. **Why**: aggressive retries amplify incidents under pressure.
- **Don't**: Treat cache as correctness source for shared mutable state. **Why**: stale cache can create consistency bugs.
- **Do**: Validate Kestrel limits through load tests before hardening values. **Why**: static numbers are context-dependent.
- **Do**: Keep EF query optimization guidance in `dataaccess` skill and link to it. **Why**: avoids duplicated ownership.

### Minimal Concurrency Pattern

```csharp
var usersTask = repository.GetUsersAsync(ct);
var ordersTask = repository.GetOrdersAsync(ct);
await Task.WhenAll(usersTask, ordersTask);
```

## Constraints & Tradeoffs

- `Task.WhenAll` rethrows one exception by default; inspect individual task failures when diagnostics matter.
- Short connection lifetimes improve DNS freshness but may increase connection churn under high load.
- More retries improve transient recovery but can increase tail latency and downstream pressure.
- Tight Kestrel limits protect resources but may reject valid traffic spikes if undersized.
- Output caching helps read-heavy paths but risks stale responses without explicit invalidation strategy.

### Measurement Expectations

- Any performance change must include baseline and post-change numbers for the same workload.
- Use percentile latency (p50/p95/p99), error rate, and throughput together; single metrics mislead.
- Compare under realistic concurrency, not single-request local runs.

### Operational Guardrails

- Timeouts, retries, and circuit breakers are a set; configure them together.
- Keep cancellation token flow intact across repository/service/http boundaries.
- Prefer safe defaults first; increase aggressiveness only with evidence.
- Roll out risky tuning behind config flags when possible.

## Anti-Patterns

- “Parallel everything” without dependency analysis.
- Unbounded retry policies with no timeout/circuit breaker pairing.
- Global breaker + shared fallback for unrelated integrations.
- Performance changes merged without before/after metrics.
- Duplicating dataaccess query rules inside this skill.
- Ignoring cancellation tokens in outbound calls and background operations.
- Treating local machine benchmark wins as production proof.

## Progressive Disclosure

1. **Start here**: defaults for concurrency, resilience scope, and runtime guardrails.
2. **Then deepen by concern**:
   - Query/index tuning: [`../dataaccess/SKILL.md`](../dataaccess/SKILL.md)
   - Rate limiting and abuse controls: [`../security/SKILL.md`](../security/SKILL.md)
   - Service-wide conventions: [`../general/SKILL.md`](../general/SKILL.md)
3. **Only then add complexity**: advanced caching, pipeline composition, and benchmark-driven adjustments.

### Escalation Path

- If bottleneck is database-bound, stop and move to [`../dataaccess/SKILL.md`](../dataaccess/SKILL.md).
- If bottleneck is abuse/spikes, align first with [`../security/SKILL.md`](../security/SKILL.md).
- If changes affect service contracts or defaults, reconcile with [`../general/SKILL.md`](../general/SKILL.md).

### Done Criteria for This Skill

- Parallelism changes preserve correctness and explicit dependency ordering.
- Resilience policies are per dependency, bounded, and observable.
- Runtime limits are justified by measurements, not copied defaults.
- Cross-skill ownership stays clean (no duplicated query/security guidance).
- Operational rollback path is documented for risky tuning changes.

## Resources

- Polly + .NET resilience: https://learn.microsoft.com/en-us/dotnet/core/resilience/http-resilience
- `IHttpClientFactory` guidance: https://learn.microsoft.com/en-us/dotnet/core/extensions/httpclient-factory
- Kestrel server limits: https://learn.microsoft.com/en-us/aspnet/core/fundamentals/servers/kestrel/options?view=aspnetcore-8.0
- High-performance ASP.NET Core: https://learn.microsoft.com/en-us/aspnet/core/fundamentals/best-practices?view=aspnetcore-8.0

## Changelog

### v2.0 — 2026-04-21
- Reduced encyclopedic examples and kept operational rules with explicit tradeoffs.
- Added mandatory sections: `Constraints & Tradeoffs` and `Progressive Disclosure`.
- Consolidated resilience guidance to one concise pattern plus cross-skill references.
