---
name: dotnet-10-testing
description: >
  Testing conventions, tier strategy, library choices, and real-database (Testcontainers)
  integration rules for .NET 10 Minimal API projects. Trigger: writing a test, adding a test,
  WebApplicationFactory, Testcontainers, real database integration test, database integration
  tier, Testcontainers PostgreSQL, xUnit, integration test, unit test, mocking, assertion
  library, FluentAssertions, InMemory database for tests, FakeTimeProvider, SkippableFact,
  test fixture, IAsyncLifetime in tests, test naming, test coverage.
license: Apache-2.0
metadata:
  author: Zesh-One
  version: "2.1"
allowed-tools: Read Edit Write Bash Glob Grep
---

## When to Use

- Writing or reviewing any file under the test project (`*.Tests.csproj`, `**/Tests/**/*.cs`, `**/*Tests.cs`, `**/*Fixture.cs`, `**/*Spy.cs`).
- Adding assertions, fixtures, spies, or test helpers.
- Configuring WebApplicationFactory, Testcontainers, or xUnit collection fixtures.
- Reviewing test coverage decisions or CI test-run behavior.

## Critical Patterns

### Rule 1 — Do: xUnit 2.9.x with native `Assert` only; Don't: add FluentAssertions or Moq

**Why**: FluentAssertions v8+ carries the proprietary Xceed Community License — verify license compatibility before adding it to any codebase. v7 (last Apache 2.0 release) is unmaintained and creates a silent upgrade trap. Moq 4.20.x contained a SponsorLink telemetry/supply-chain incident and is banned regardless of version. The three-tier strategy makes a mocking library architecturally redundant. If mock-generation ever becomes genuinely necessary, prefer NSubstitute (MIT); never Moq. Teams that accept the license-ceiling burden may pin FluentAssertions v7 as a documented exception, but do not use `.Should()` syntax alongside native `Assert` — it creates assertion-style inconsistency and introduces a dependency that must be managed for license compliance.

### Rule 2 — Do: real database (Testcontainers) for any test touching SQL or infrastructure seams; Don't: use EF InMemory as a behavioral substitute

**Why**: InMemory cannot model provider-specific behavior — SQL semantics, constraints, transactions, or session state. A passing InMemory integration test proves nothing about real database behavior and produces false CI green while defects silently rot.
**Permitted InMemory uses (WAF tier only, narrow exception):**
- Assert that DI resolves your DbContext(s) by checking the provider name — never call the DB.
- Assert DI lifetime/type for infrastructure seam interfaces without invoking them.
- Pure EF `IEntityTypeConfiguration` unit tests with zero HTTP or service involvement.

Never issue SQL from these tests; when in doubt, use the real-database tier.

### Rule 3 — Do: never mock a transaction seam; Don't: replace it with stubs in behavioral tests

**Why**: if the project has a transaction or data-access seam, it IS the transaction perimeter. A mocked seam never exercises the real transactional behavior, making the test structurally blind to a critical class of regressions. Test it only against the real-database tier.

### Rule 4 — Do: three-tier strategy; Don't: collapse tiers or substitute a lower tier for a higher one

| Tier | Tool | Use for |
|---|---|---|
| Unit | xUnit, no DB | Pure C# logic, mappers, validators with no external dependency |
| WAF | `WebApplicationFactory<Program>` | HTTP contract, middleware, routing, structural DI; real DI + targeted test-double overrides for external services |
| Real DB | Testcontainers with the production database engine (match the production version) | Any test touching SQL, database-specific behavior, transactions, or database-enforced security where present |

The real-database tier name is per-project (e.g., `RealPg` is a common choice for Postgres projects); what matters is that the tier runs against the real engine.

### Rule 5 — Do: `MethodName_Condition_ExpectedResult` naming; Don't: prose names as the default

**Why**: this is the house naming convention of this skill — it keeps test intent scannable and greppable. Use `[Fact]` for single scenarios, `[Theory]` + `[InlineData]`/`[MemberData]` for parameterized paths. `[Fact(DisplayName = "...")]` is an exception for genuinely prose-friendly scenarios only.

**Exception (requirement traceability):** if the project maps tests to audit or acceptance requirement IDs, the traceability scheme takes precedence over pure naming purity. Preferred form keeps `MethodName_Condition_ExpectedResult` and moves the ID to a `[Trait("audit","AC-12")]` or category — never drop the link to the requirement.

### Rule 6 — Do: AAA structure with blank-line separators; Don't: interleave setup and assertion

**Why**: shared setup goes in the constructor (xUnit creates a new instance per test) or `IAsyncLifetime.InitializeAsync`. Never use `[SetUp]`/`[TearDown]` — those are MSTest/NUnit attributes.

### Rule 7 — Do: FakeTimeProvider for all time-dependent logic; Don't: use `DateTime.UtcNow` or `DateTimeOffset.UtcNow` in production code or tests

**Why**: production code must inject `TimeProvider` as a singleton so the test host can substitute `FakeTimeProvider` (package: `Microsoft.Extensions.TimeProvider.Testing 10.x`) via the DI override in `WithWebHostBuilder`. This applies to token expiry, JWT key rotation, TOTP windows, and rate-limit boundaries.

### Rule 8 — Do: serial execution when integration tests share a container; Don't: enable parallelism against shared state

**Why**: when integration tests share a single Testcontainers instance per xUnit Collection fixture, parallel execution against the same container causes non-deterministic state collisions — do not add `xunit.runner.json` parallelism settings in that setup. If you instead provision per-test or per-class isolated databases, or use Respawn-style resets, parallelism may be re-enabled as an explicit, documented decision paired with the isolation strategy that makes it safe.

### Rule 9 — Do: hard-fail in CI when Docker is unavailable; Don't: skip real-database tests silently

**Why**: a silently skipped integration test is an invisible regression — especially when the real-database tier is what verifies security invariants. In CI, throw an explicit exception (red build). Locally, use `Xunit.SkippableFact` and `throw new SkipException("Docker not available")` only when Docker is verified absent at runtime.

### Rule 10 — Do: `Assert.IsType<T>` for Result/discriminated union assertions; Don't: cast or assert on protected Failure reason messages

**Why**: assert sealed-record Result union outcomes by type — `Assert.IsType<OrderResult.Failure>(result)` — rather than casting. When a Failure type intentionally carries no reason message as an anti-enumeration measure (typical in auth/login/signup flows), tests must not assert on reason strings — asserting the reason would force the code to violate the invariant.

### Rule 11 — Do: manual hand-rolled spies in `Spies/` or `Fakes/`; Don't: generate spies from a mocking library

**Why**: a boolean was-called flag and a captured-args list are sufficient for most observation needs. Keep spy classes simple. Mocking libraries are redundant given the three-tier strategy.

## Constraints & Tradeoffs

- Recommended baseline test `.csproj` packages: `xunit 2.9.x`, `xunit.runner.visualstudio 3.x` (use current 3.x), `Microsoft.NET.Test.Sdk 17.x`, `Microsoft.AspNetCore.Mvc.Testing 10.0.x`, `Testcontainers.PostgreSql 4.4.x` for Postgres — or the Testcontainers module matching your production engine — `Microsoft.Extensions.TimeProvider.Testing 10.x`, `Xunit.SkippableFact`, `coverlet.collector`. Add `Microsoft.EntityFrameworkCore.InMemory` only if the narrow WAF-tier DI-wiring exception (Rule 2) genuinely requires it — adding the package reference creates drift risk even with limited intent.
- Coverage targets are integration-weighted, not line-level. Every Minimal API endpoint needs at least one WAF test for HTTP contract. Every security-critical path (auth, TOTP, session revocation) needs at least one real-database test at the real security seam. If the domain layer consists of behavior-free persistence POCOs, do not enforce line-coverage targets (e.g., the generic 85%) on it — that produces coverage noise, not confidence. If domain entities carry behavior, domain coverage targets apply normally.
- Serial execution is the default WHEN integration tests share one container (Rule 8). Any switch to parallelism must be documented together with the per-test isolation strategy that makes it safe.
- Applicability: the no-mocking-library and no-EF-InMemory rules are strongest when the database enforces security-relevant invariants (constraints, checks, triggers) — there, real-database testing is non-negotiable. In projects without database-enforced security, these defaults still reflect this skill's style but may be relaxed via explicit architectural review.
- Connection strings are injected via `ConfigureAppConfiguration` from the Testcontainers-provided string. Never hard-code database credentials in test code.
- If the project uses a transaction/unit-of-work seam that forbids nested transactions, never nest its calls in tests — each test action enters its own context, or the fixture provides one.

## Anti-Patterns

- `UseInMemoryDatabase` as an integration/behavioral test substitute.
- Mocking the transaction/security-context seam in behavioral tests.
- Adding Moq (banned — SponsorLink incident, supply-chain risk).
- Adding FluentAssertions v8+ (proprietary Xceed license) or even v7 (unmaintained, license-ceiling trap) without a documented license review.
- Using `.Should()` assertion syntax in the test project.
- Silently skipping real-database (Testcontainers) tests in CI when Docker is unavailable.
- Enabling test parallelism while integration tests share one container.
- Asserting on `Failure` reason strings where the no-reason invariant is an intentional anti-enumeration measure (auth flows).
- Enforcing line-coverage targets on a behavior-free persistence-POCO domain layer — it produces coverage noise, not confidence.
- Hard-coding connection strings or database credentials.
- Nesting calls into a transaction seam that forbids nested transactions inside a test already running in that seam's context.

## Progressive Disclosure

1. **First**: read `Critical Patterns` — operational defaults.
2. **On package or library decisions**: check `Constraints & Tradeoffs` for the recommended baseline versions.
3. **On real-database fixture setup**: R8 (fixture lifecycle and parallelism) and R9 (Docker/CI) together cover the container setup contract.
4. **On coverage questions**: the coverage rule is in `Constraints & Tradeoffs` — lead with integration weight, not a blanket line-coverage target.
5. **If neighboring domains are touched**: follow `Resources`.

## Resources

- Persistence patterns, transaction seams: [`../data-access-persistence/SKILL.md`](../data-access-persistence/SKILL.md)
- AuthN/AuthZ policies, JWT, anti-enumeration: [`../security/SKILL.md`](../security/SKILL.md)
- xUnit, packages, C# 14 patterns: [`../dotnet-conventions/SKILL.md`](../dotnet-conventions/SKILL.md)
- Architecture selection framework (vertical-slice monolith / modular monolith) and domain model guidance: [`../architecture/SKILL.md`](../architecture/SKILL.md)
- Result discriminated-union assertion conventions: [`../design-patterns/SKILL.md`](../design-patterns/SKILL.md)
- Endpoint contracts, TypedResults, ProblemDetails (WAF-tier assertions): [`../api-rest-minimal-apis/SKILL.md`](../api-rest-minimal-apis/SKILL.md)

## Changelog

### v2.1 — 2026-07-10
- Removed the multi-tenant RLS isolation-testing references from the real-database, seam-mocking, three-tier, coverage, and applicability rules; the general RealPg/Testcontainers-over-InMemory and no-seam-mocking guidance is retained; this collection no longer targets multi-tenant RLS.

### v2.0 — 2026-07-10
- Generalized for any .NET 10 project; removed project-specific mandates and decisions; multi-tenant/RLS testing guidance made conditional and the fixture migration-order rule removed, in alignment with the architecture selection framework.
