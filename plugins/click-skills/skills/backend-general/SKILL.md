---
name: backend-general
description: >
  Operational defaults for ASP.NET Core 8 REST API architecture and conventions.
  Trigger: When creating, restructuring, or reviewing backend feature slices, dependency wiring, middleware order, and shared conventions.
license: Apache-2.0
metadata:
  author: Zesh-One
  version: "2.0"
allowed-tools: Read Edit Write Bash Glob Grep
---

## When to Use

- Starting a new backend feature or module in a .NET 8 API.
- Reviewing architecture consistency before PR.
- Deciding where behavior belongs (controller vs service vs repository).
- Aligning middleware order and cross-cutting concerns.
- Enforcing naming and configuration conventions.

## Critical Patterns

### Core Rules (Do/Don't + Why)

- **Do**: Organize by vertical slice (`Features/{Feature}/...`), not by global technical layers. **Why**: keeps change scope local and reduces cross-feature coupling.
- **Do**: Keep controllers thin (transport only) and move business rules to services. **Why**: improves testability and prevents HTTP concerns from leaking into domain logic.
- **Do**: Keep repository scope focused on persistence concerns only. **Why**: avoids mixing query mechanics with business policy.
- **Do**: Register stateless helpers as `Singleton`. **Why**: lower allocation churn and explicit lifecycle intent.
- **Don't**: Put secrets in `appsettings.json` or hardcode config paths. **Why**: secrets belong outside git and outside deploy artifact.
- **Do**: Load sensitive config via env-var path (`{SERVICE}_CONFIG_PATH`) to an external file. **Why**: enables secure rotation and environment separation.
- **Do**: Keep all identifiers, comments, and docs in English. **Why**: consistent language lowers review friction across teams.
- **Do**: Use plural resource names in URLs (`/users`, `/orders`). **Why**: stable REST semantics and predictable routing.
- **Do**: Keep shared exceptions canonical (`NotFoundException`, `ConflictException`) and map centrally. **Why**: consistent API error behavior.
- **Don't**: Duplicate detailed rules from specialized skills. **Why**: this skill is the index/defaults layer, not the full tutorial.

### Canonical Middleware Order

```csharp
app.UseMiddleware<CorrelationIdMiddleware>();
app.UseAuthentication();
app.UseMiddleware<UserLogContextMiddleware>(); // optional
app.UseRateLimiter();
app.UseCors();
app.UseMiddleware<ExceptionHandlingMiddleware>();
app.UseAuthorization();
app.MapControllers();
```

## Constraints & Tradeoffs

- `ExceptionHandlingMiddleware` before `UseAuthorization()` favors richer user-aware logs, but authorization handler exceptions may bypass this handler.
- `UseRateLimiter()` before `UseCors()` protects early, but can return `429` without CORS headers for preflight; consider `OPTIONS` exemptions when needed.
- Vertical slices improve ownership but may duplicate simple DTO names across features; accept local duplication over global coupling.
- External config file reload (`reloadOnChange`) improves operability but requires secure file permissions in provisioning.

### Scope Boundaries

- This skill defines baseline defaults; it does not own low-level implementation recipes.
- If a rule conflicts with a specialized skill, specialized skill wins for that domain.
- Use this file to resolve architectural placement questions first, then load domain skill.

### PR Review Checklist (Architecture)

- Is behavior in the correct layer (controller/service/repository)?
- Are external settings loaded from an external config path env var?
- Is middleware order intentionally preserved with explicit comments when changed?
- Are new shared abstractions justified by at least two real consumers?
- Are cross-skill links added when introducing non-trivial domain rules?

## Anti-Patterns

- Fat controllers with domain branching and validation orchestration.
- Shared “God services” used by unrelated features.
- Hardcoded connection strings, secrets, or static config singleton paths.
- Creating new cross-cutting conventions without linking the owning skill.
- Putting feature-specific code under `Shared/` for convenience.
- Moving exceptions and contracts ad hoc across folders during refactors.
- Merging architecture changes without updating changelog or rationale.

## Progressive Disclosure

1. **Start here (this skill)**: baseline structure, middleware order, and repo-wide defaults.
2. **Go deeper by domain**:
   - Data access: [`../dataaccess/SKILL.md`](../dataaccess/SKILL.md)
   - API contracts/errors: [`../responses/SKILL.md`](../responses/SKILL.md)
   - Auth/rate limits: [`../security/SKILL.md`](../security/SKILL.md)
   - Logging/traceability: [`../logging/SKILL.md`](../logging/SKILL.md)
3. **Implementation details**: load specialized skill only when the task enters that domain.

### Skill Routing Map

- DTO and response envelope shape: [`../responses/SKILL.md`](../responses/SKILL.md)
- Validation strategy and registration: [`../validations/SKILL.md`](../validations/SKILL.md)
- AutoMapper/extensions and mapping ownership: [`../mapping/SKILL.md`](../mapping/SKILL.md)
- Unit test patterns and fixtures: [`../testing-unit/SKILL.md`](../testing-unit/SKILL.md)
- Performance and resilience specifics: [`../performance/SKILL.md`](../performance/SKILL.md)

### Done Criteria for This Skill

- Feature structure is coherent without adding unnecessary global abstractions.
- Middleware and error handling decisions are explicit and link to logging ownership.
- Security-sensitive configuration is externalized and not committed.
- API naming and language conventions stay consistent across touched files.

## Resources

- ASP.NET Core fundamentals (.NET 8): https://learn.microsoft.com/en-us/aspnet/core/fundamentals/?view=aspnetcore-8.0
- ASP.NET Core configuration (.NET 8): https://learn.microsoft.com/en-us/aspnet/core/fundamentals/configuration/?view=aspnetcore-8.0
- Dependency injection in ASP.NET Core: https://learn.microsoft.com/en-us/aspnet/core/fundamentals/dependency-injection?view=aspnetcore-8.0
- Clean Architecture reference: https://jasontaylor.dev/clean-architecture-getting-started/

## Changelog

### v2.0 — 2026-04-21
- Reduced encyclopedic content to operational defaults and tradeoffs.
- Added mandatory sections: `Constraints & Tradeoffs`, `Anti-Patterns`, and `Progressive Disclosure`.
- Kept canonical middleware order and cross-skill references; removed duplicated deep tutorials.
