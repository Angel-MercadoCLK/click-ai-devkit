---
name: dotnet-10-security
description: >
  Security rules for .NET 10 Minimal API backends covering Argon2id, ES256 JWT with KEK-wrapped keys,
  CSRF, CORS, rate limiting, and no-leakage error design.
  Trigger: security, authentication, authorization, password hashing, JWT, TOTP, 2FA,
  CORS, CSRF, rate limit, security headers, session revocation, key management, KEK, Argon2, ES256, secrets,
  input validation, account enumeration, ProblemDetails error, token storage.
license: Apache-2.0
metadata:
  author: Zesh-One
  version: "2.1"
allowed-tools: Read Edit Write Bash Glob Grep
---

## When to Use

- Implementing or reviewing authentication, authorization, or session management.
- Writing or modifying security infrastructure code (`Infrastructure/Security/**` or equivalent).
- Touching `Domain/` entities with sensitive fields (`PasswordHash`, `TotpSecret`, `SessionId`, `KeyMaterial`).
- Configuring middleware pipeline, CORS, CSRF, rate limiting, or security headers in `Program.cs`.
- Writing integration tests against real Postgres (e.g., Testcontainers) that verify security-critical data paths.

## Critical Patterns

### Rule 1 — Password hashing: Do Argon2id; Don't BCrypt

**Config**: memory >= 64 MB, iterations >= 3, parallelism >= 1. Enforce 128-char hard cap **before** any KDF call. Reject against the embedded SecLists 10k common-password list before hashing. No mandatory composition rules (NIST SP 800-63B).

**Exception:** `memory >= 64 MB / iterations >= 3` is a high-cost target, not a hard floor. The OWASP-recommended low-memory Argon2id profile — `m = 19456 KiB (19 MiB), t = 2, p = 1` — is an acceptable minimum. Treat 64 MB / t=3 as a stronger optional target, not a violation threshold. Do not raise cost parameters (slowing login) without a measured reason.

**Why**: BCrypt silently truncates input at 72 bytes. A password in the 73–128 char range hashes identically to its 72-byte prefix, making the 128-char cap cryptographically hollow. Argon2id is stronger against GPU/ASIC offline attacks.

### Rule 2 — JWT signing: Do ES256 with KEK-wrapped private key; Don't HS256 with a config-string secret

**Config**: sign with ECDSA P-256 asymmetric keys. Private key material stored AES-256-GCM encrypted at rest under the JWT KEK (purpose-separated from the TOTP KEK). Verifying parties receive only the public key. Rotate keys via a key-rotation mechanism (e.g., a `BackgroundService`). Resolve `IssuerSigningKey` from the application's key service / in-memory public key cache — never from config strings.

**Why**: HS256 requires every verifier to hold the signing key; one leaked key forges any identity. Violates RFC 8725 §3.4 for multi-party verification.

### Rule 3 — Authentication result failure cases carry no reason field

Authentication result types (login, password-reset, TOTP verification) use sealed discriminated-union record cases. The failure case must carry no reason, no error code distinguishing account-not-found from wrong-password, and no field that enables account enumeration — all failure modes must be indistinguishable to the client. The same shape applies to any endpoint where distinguishing failure modes creates an enumeration oracle.

### Rule 4 — Session validity: per-request check when immediate revocation is required

When immediate revocation is a requirement, validate the `sid` JWT claim against the session store on every authenticated request — revocation then takes effect on the next request. Any session-validity cache introduces a revocation delay window; if caching is accepted, bound the window explicitly and document it.

### Rule 5 — TOTP: AES-256-GCM encrypted secret under TOTP KEK; backup codes SHA-256 hashed

Library: `Otp.NET` (RFC 6238). TOTP KEK is distinct from the JWT KEK. Backup codes must be hashed before storage. TOTP endpoints must have a dedicated rate-limit policy and lockout after a configurable number of consecutive failures.

### Rule 6 — Token persistence: SHA-256 hash only; raw token returned once, never stored

All tokens (refresh, password-reset, email-verification, backup codes in raw form) are persisted only as their SHA-256 hash. Generate with cryptographically secure randomness; return to the client once.

### Rule 7 — JWT validation: all four checks required; non-zero ClockSkew

`ValidateIssuer: true`, `ValidateAudience: true`, `ValidateLifetime: true`, and a minimal but non-zero `ClockSkew`. Disabling any is prohibited. In tests, substitute `FakeTimeProvider`; never use `DateTime.UtcNow` or `DateTimeOffset.UtcNow` directly for security-related time comparisons.

### Rule 8 — CORS: regex-anchored allow-list; AllowAnyOrigin prohibited with credentials

Specify `WithMethods` and `WithHeaders` explicitly. `AllowAnyMethod` and `AllowAnyHeader` are prohibited in production. Development-only patterns (e.g., `lvh.me`) must be excluded via an environment guard.

### Rule 9 — CSRF: double-submit cookie on refresh and logout endpoints

CSRF token appears both in the cookie and in a request header; server validates they match. Applies to any endpoint that accepts cookies and performs session creation/revocation.

### Rule 10 — Rate limiting: sliding-window policies; 429 must return ProblemDetails

Define named sliding-window policies per flow class — e.g., login/registration, TOTP verification, password reset, general API, authenticated API. Register a kill-switch pass-through (no-op) implementation behind a feature flag for operational use. Rate-limit rejections return a 429 ProblemDetails response, never a raw 429 with no body.

### Rule 11 — Security headers via middleware (not per-endpoint)

Required: `Strict-Transport-Security` (max-age >= 31536000; includeSubDomains), `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`, `Referrer-Policy: no-referrer`, `Content-Security-Policy` appropriate to an API (no inline scripts, restrict sources).

**Exception:** For a JSON-only API with no server-rendered HTML deployed behind a reverse proxy, `Content-Security-Policy` may be owned at the edge instead of the API middleware. Document which layer owns each header; optionally add `default-src 'none'` in-app as defense-in-depth.

### Rule 12 — Secrets from environment variables or secrets manager only

Connection strings, KEK paths, TOTP key references, JWT key storage paths — never in `appsettings.json` or any committed file. Failing to resolve a required secret at startup must cause a fast-fail, not a degraded start.

### Rule 13 — Security-critical Options: ValidateOnStart required

JWT options, Argon2 options, TOTP options, key management options. Deliberate omission is allowed only when the default is safe and that choice is documented inline with a comment citing the reasoning. For the full Options class shape, see the design-patterns skill.

### Rule 14 — Middleware pipeline order is fixed

The canonical middleware pipeline order is owned by the api-rest-minimal-apis skill — that skill is authoritative. Reordering security middleware (CORS, rate limiter, auth) can silently bypass protections — follow the canonical order exactly. If TLS terminates at a reverse proxy, `UseHttpsRedirection` may be omitted from the app pipeline — document the decision; otherwise include it.

### Rule 15 — Validation on auth/enumeration-sensitive endpoints: handler-internal only

`AddValidation()` pipeline filters and FluentValidation validators are prohibited on authentication endpoints — they emit `ValidationProblem(errors)` 400 responses before the handler runs and expose field-level failures that leak account existence. Use inline guard checks returning the appropriate typed `Result` variant. `DataAnnotations` + `ValidateDataAnnotations()` + `ValidateOnStart()` remain acceptable exclusively for Options configuration.

### Rule 16 — Entity-to-DTO mapping for sensitive types: explicit only

Types with `PasswordHash`, `TotpSecret`, `SessionId`, `KeyMaterial`, `BackupCodeHashes` must use positional record constructors or EF Core `.Select()` projections. `AutoMapper`, `Mapster`, and all convention-based mappers are prohibited — sensitive fields must be absent from response record types and that absence must be verifiable at compile time.

### Rule 17 — ProblemDetails for all error responses; no exception detail in production

Rate-limit rejections, auth failures, authorization failures, validation failures, and unhandled errors all use RFC 7807 ProblemDetails. Exception messages, stack traces, type names, and internal diagnostics must not appear in production responses. Serilog captures the full detail server-side.

### Rule 18 — Security-critical data-access tests run against real Postgres; InMemory is prohibited for them

`EF Core InMemory` is not relational — it cannot model constraints, transactions, or provider-specific behavior, so a passing InMemory test for a security-critical data path is a false green that conceals a potential defect. Run these tests against real Postgres (e.g., Testcontainers) and hard-fail the pipeline when Docker is unavailable — skipping silently is a silent security regression.

### Rule 19 — JWT signing-key rotation: a failed rotation must never leave an expired key active

JWT rotation failure must not silently leave an expired key as the active signing key. Validate the new key before swapping it in, and fail loudly (not silently fall back) when rotation cannot complete.

### Rule 20 — Scalar UI and OpenAPI endpoint disabled in production

Both gated behind an environment check. A committed `openapi.json` build artifact is for developer use — it must not be served live.

### Rule 21 — Authorization policies: register with `AddAuthorizationBuilder`; never rely on role claims alone

Declare policies using `AddAuthorizationBuilder()` with requirement-based policies. For example, a system might define:

- `AdminOnly` — requires the admin role claim **and** validates session validity.
- `ReportsReader` — requires a role claim **and** a requirement scoping access to the caller's authorized data.

Apply policies via `RequireAuthorization` on endpoint groups or individual endpoints in `Program.cs`. Role claims alone are insufficient — always validate session validity per Rule 4 (`sid` claim against the session store).

```csharp
// Illustrative example — define policies matching your own authorization model.
builder.Services.AddAuthorizationBuilder()
    .AddPolicy("AdminOnly", p => p.RequireRole("admin").AddRequirements(new SessionValidRequirement()))
    .AddPolicy("ReportsReader", p => p.RequireRole("reports-reader").AddRequirements(new DataScopeRequirement()));
```

**Why**: A valid role claim in a revoked session still satisfies a role-only check. Session validation is the revocation enforcement point; skipping it creates a window where a revoked token retains elevated access.

## Constraints & Tradeoffs

- Centralize Postgres error-code-to-`Result` translation in a single mapper component (e.g., a `PostgresExceptionMapper` class) rather than handling it inline per endpoint. Error codes `23505` (unique), `P0001` (custom raise), `23514` (check violation) must go through the mapper.
- Security logs (login success/failure, TOTP failure, session revocation, key rotation, suspicious patterns) go through Serilog with structured properties (`user_id`, event type). Never log raw token values, password material, or key material.

## Anti-Patterns

- BCrypt for password hashing (silently truncates at 72 bytes).
- HS256 JWT with a config-string secret (symmetric key, violates RFC 8725 for multi-party verification).
- Storing raw JWT signing key MATERIAL or TOTP secret BYTES in plaintext (appsettings, env vars, or database) — the wrapped/encrypted key material and its path/reference are different; references may live in env vars, raw key bytes must not.
- Auth failure results carrying a reason or error-code field (account-enumeration oracle).
- Tokens persisted in plaintext (only SHA-256 hash is stored).
- Session-validity caching without a documented, bounded revocation-delay window.
- `AllowAnyOrigin` with `AllowCredentials`.
- Missing CSRF protection on refresh/logout endpoints.
- FluentValidation or `AddValidation()` pipeline on authentication or enumeration-sensitive endpoints.
- Convention-based mappers (AutoMapper, Mapster) on types with sensitive fields.
- EF Core InMemory tests standing in for security-critical data-path tests — false greens.
- Missing `ValidateOnStart` on security-critical Options.
- `ClockSkew = TimeSpan.Zero` in production JWT validation.
- Scalar UI or raw OpenAPI endpoint served in production.
- Exception messages or stack traces in ProblemDetails production responses.
- Authorizing by role claim alone without session validity check (revoked tokens pass role assertions).

## Progressive Disclosure

1. **First**: `Critical Patterns` — all security decisions are here; treat each rule as a hard constraint.
2. **On architecture questions**: `Constraints & Tradeoffs` — Postgres error mapping and security logging.
3. **On code review**: `Anti-Patterns` as a checklist.
4. **On neighboring layers**: see `Resources` below.

## Resources

- Data access seams, transaction boundaries: [`../data-access-persistence/SKILL.md`](../data-access-persistence/SKILL.md)
- Result pattern, Options, DI lifetimes: [`../dotnet-conventions/SKILL.md`](../dotnet-conventions/SKILL.md)
- Architecture selection (vertical slice monolith, modular monolith) and dependency rules: [`../architecture/SKILL.md`](../architecture/SKILL.md)
- Test tiers, FakeTimeProvider, integration tests against real Postgres: [`../testing/SKILL.md`](../testing/SKILL.md)
- Middleware order, ProblemDetails, endpoint grouping: [`../api-rest-minimal-apis/SKILL.md`](../api-rest-minimal-apis/SKILL.md)
- Result discriminated union, Postgres error mapping, Options class shape: [`../design-patterns/SKILL.md`](../design-patterns/SKILL.md)

## Changelog

### v2.1 — 2026-07-10
- Removed all multi-tenant PostgreSQL RLS content — the RLS-authoritative-perimeter, RLS session-variable context-setting seam, privileged/`SET ROLE` seam, nested-context-scope, `NOSUPERUSER NOBYPASSRLS NOINHERIT` connection-role, and separate-DbContexts-per-tenant rules and their anti-patterns — neutralized the multi-tenant authorization-policy examples, and generalized the security data-test rule; renumbered the remaining rules. The auth stack (Argon2id, ES256 JWT, TOTP, CSRF, CORS, rate limiting, security headers, enumeration prevention) is unchanged. This collection no longer targets multi-tenant RLS.
  - Corrected Rule 19's title to match its JWT signing-key-rotation body (it previously carried a mismatched "background services / scoped DbContext" heading).

### v2.0 — 2026-07-10
- Generalized for any .NET 10 project; removed project-specific mandates and decisions; multi-tenant RLS and runner guidance made conditional on project architecture.
