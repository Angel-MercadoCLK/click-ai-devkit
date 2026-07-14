---
name: backend-logging
description: >
  Operational logging standards for ASP.NET Core APIs using Serilog structured events,
  request correlation, and canonical exception telemetry.
  Trigger: When implementing or reviewing API logging, middleware instrumentation,
  incident diagnostics, or log safety controls.
license: Apache-2.0
metadata:
  author: Zesh-One
  version: "2.9"
allowed-tools: Read Edit Write Bash Glob Grep
---

## When to Use

- Adding or refactoring API logging middleware.
- Defining severity rules for domain and HTTP outcomes.
- Investigating incidents using correlation IDs.
- Reviewing logs for security, privacy, and compliance.
- Aligning logging behavior with `responses`, `security`, and `general` skills.

## Critical Patterns

1) **Do** use Serilog structured fields; **Don’t** emit unstructured free-text logs.
Why: queryable fields are required for fast incident reconstruction.

2) **Do** ensure every request has a `CorrelationId`; **Don’t** depend on client header quality.
Why: caller-provided IDs may be missing or malicious.

3) **Do** sanitize incoming `X-Correlation-ID`; **Don’t** trust control characters or long payloads.
Why: prevents log injection and parser breakage.

4) **Do** include minimum traceability fields on warnings/errors: `CorrelationId`, `Application`,
`Environment`, `UserId`, `RequestPath`, `RequestMethod`, `StatusCode`, `ExceptionType`, `Duration`.
**Don’t** omit context in high-severity events.
Why: these are the minimum forensic chain.

5) **Do** return `ProblemDetails` with `Extensions["correlationId"]`; **Don’t** hide correlation from callers.
Why: support needs the same identifier users receive.

6) **Do** keep canonical exception handling in middleware (`ExceptionHandlingMiddleware`);
**Don’t** duplicate try/catch policy across controllers.
Why: centralized behavior prevents drift in status and log level mapping.

7) **Do** map exceptions consistently: `ValidationException`→400 (Info),
`ForbiddenException`/`NotFoundException`/`ConflictException`→Warn, unexpected exceptions→Error.
**Don’t** over-escalate expected failures.
Why: noisy error logs hide true incidents.

8) **Do** enrich request logs once (`UseSerilogRequestLogging`); **Don’t** duplicate `StatusCode` and `Duration`.
Why: duplicates increase volume without signal.

9) **Do** keep user enrichment after authentication when needed; **Don’t** assume identity exists at pipeline start.
Why: `UserId` before auth is unreliable.

10) **Do** redact sensitive values before logging; **Don’t** log secrets, tokens, credentials, full PII, or connection secrets.
Why: logs are long-lived and broadly accessible.

11) **Do** keep logs English-only; **Don’t** mix languages in message templates.
Why: consistent operations language is mandatory.

12) **Do** prefer one stable message template per event type; **Don’t** create template variants for the same event.
Why: stable templates improve analytics and alerting quality.

```csharp
// Canonical API error response enrichment
problem.Extensions["correlationId"] = context.Items["CorrelationId"] as string;
```

## Constraints & Tradeoffs

- Serilog is mandatory for structured logging in this project.
- Correlation middleware must run first in the pipeline (`general/SKILL.md`).
- Repositories and low-level data code must not own API exception logging policy.
- Per-user file partitioning can help investigations but hurts scalability for high-cardinality traffic.
- File sinks are acceptable for local/server diagnostics; centralized aggregators are preferred at scale.
- Verbose payload logging increases supportability but raises compliance and storage risks.
- Keep log schema stable over time; schema churn breaks dashboards and alert rules.
- Avoid dynamic property names; prefer fixed keys for index efficiency.
- Warn-level and above events must remain sparse enough to be actionable.
- Correlation IDs must be echoed to response headers for client-side traceability.

## Anti-Patterns

- Logging request/response bodies by default.
- Logging access tokens, refresh tokens, API keys, passwords, or connection strings.
- Using `Console.WriteLine` in runtime code.
- Logging expected validation failures as errors.
- Returning generic 500 responses without correlation ID.
- Generating a new correlation ID mid-request.
- Duplicating exception→HTTP mappings in controllers and middleware simultaneously.
- Using localized/non-English log templates.

## Progressive Disclosure

- **Start here (minimum):** correlation middleware + request logging + exception middleware + redaction discipline.
- **Then optimize:** tune level mapping and template consistency for alert quality.
- **Then scale:** route events to centralized sinks and dashboards.
- **Advanced optional:** per-user partitioning only for bounded-identity systems.

Operational checklist for PR reviews:
- Confirm `CorrelationId` is present end-to-end (header, log context, ProblemDetails extension).
- Confirm exception classes map to intended status/log severity.
- Confirm no forbidden fields appear in message templates or structured properties.
- Confirm middleware order remains aligned with `general` skill.

For exception contract details see [`../responses/SKILL.md`](../responses/SKILL.md).
For pipeline order see [`../general/SKILL.md`](../general/SKILL.md).
For security logging boundaries see [`../security/SKILL.md`](../security/SKILL.md).

## Resources

- Serilog overview: https://serilog.net/
- Serilog ASP.NET Core: https://github.com/serilog/serilog-aspnetcore
- Serilog enrichment (`LogContext`): https://github.com/serilog/serilog/wiki/Enrichment
- ProblemDetails in ASP.NET Core: https://learn.microsoft.com/aspnet/core/web-api/handle-errors

## Changelog

### v2.9 — 2026-04-21
- Rewritten as concise operational guidance (P1 format).
- Added mandatory section structure and atomic Do/Don’t rules with rationale.
- Enforced explicit English-only and sensitive-data boundaries.
- Preserved canonical references to responses, general, and security skills.
