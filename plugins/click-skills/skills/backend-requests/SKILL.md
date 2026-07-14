---
name: backend-requests
description: >
  Operational rules for HTTP request contracts in ASP.NET Core 8 APIs.
  Trigger: When designing request DTOs, binding route/query/body inputs, or defining endpoint signatures in .NET APIs.
license: Apache-2.0
metadata:
  author: Zesh-One
  version: "1.3"
allowed-tools: Read Edit Write Bash Glob Grep
---

## When to Use

- Designing or reviewing request DTOs.
- Defining action signatures for route, query, header, or body inputs.
- Auditing pagination, upload, and identifier input safety.
- Standardizing request contracts before validation/mapping layers.

## Critical Patterns

1. **Do** suffix inbound DTOs with `Request`; **don't** use entities as request bodies. **Why:** avoids persistence-coupled APIs.
2. **Do** annotate every binding source (`[FromRoute]`, `[FromQuery]`, `[FromBody]`, `[FromHeader]`); **don't** rely on inference. **Why:** inference drifts under refactors.
3. **Do** enforce `{id:guid}` for public resource IDs; **don't** expose sequential ints. **Why:** reduces BOLA enumeration risk.
4. **Do** cap page size server-side (e.g., `Math.Min(PageSize, 100)`); **don't** trust raw client values. **Why:** prevents oversized queries.
5. **Do** use non-nullable defaults for required strings; **don't** allow implicit null required inputs. **Why:** avoids null-shape ambiguity.
6. **Do** keep request DTOs transport-focused; **don't** embed domain behavior. **Why:** keeps contracts stable across domain evolution.
7. **Do** model optional filters as nullable fields; **don't** overload sentinel values (`-1`, `"all"`). **Why:** preserves semantic clarity.
8. **Do** annotate file upload actions with `[Consumes("multipart/form-data")]`; **don't** inherit JSON consumes for uploads. **Why:** prevents 415 mismatches.
9. **Do** normalize paging defaults (`Page=1`, `PageSize=20`); **don't** leave unbounded zero/negative inputs. **Why:** deterministic query behavior.
10. **Do** keep header extraction explicit (tenant, correlation); **don't** hide multi-tenant context in service internals. **Why:** request contract remains auditable.

```csharp
[HttpGet("{id:guid}")]
public async Task<IActionResult> Search(
    [FromRoute] Guid id,
    [FromQuery] SearchUsersRequest request,
    [FromHeader(Name = "X-Tenant-ID")] string? tenantId)
{
    request.PageSize = Math.Min(request.PageSize, 100);
    return Ok();
}
```

## Constraints & Tradeoffs

- Explicit binding attributes are verbose, but remove ambiguity in API reviews.
- GUID route constraints improve security posture but reduce URL readability.
- Hard page-size caps protect infrastructure but may require export endpoints for bulk use cases.
- Transport DTO isolation adds mapping work; this cost is intentional and controlled by [`../mapping/SKILL.md`](../mapping/SKILL.md).
- Upload actions require explicit consumes metadata even when controller defaults exist.

Operational review checklist:
- Verify every public action declares binding source attributes.
- Verify list endpoints cap `PageSize` server-side.
- Verify any public identifier route uses GUID constraints.
- Verify optional request filters are nullable, not sentinel-based.
- Verify upload actions declare multipart consumes explicitly.

## Anti-Patterns

1. Reusing EF/domain entities as inbound request contracts.
2. Accepting unbounded `PageSize` directly from query string.
3. Mixed implicit and explicit binding in the same controller.
4. Public routes with `{id:int}` for externally visible resources.
5. Validating file streams by reading `IFormFile` content inside validators.
6. Using magic filter values (`"*"`, `-1`) instead of nullable optionals.

## Progressive Disclosure

1. Start with naming, explicit binding, and ID constraints in new endpoints.
2. Add pagination cap and optional filter hygiene for list endpoints.
3. Introduce upload-specific consumes + validation flow via [`../validations/SKILL.md`](../validations/SKILL.md).
4. Harden cross-cutting consistency with mapping and response alignment.
5. For security-sensitive APIs, apply this skill together with [`../security/SKILL.md`](../security/SKILL.md).

Adoption sequence by maturity:
- **Foundational**: naming + explicit binding + GUID routes.
- **Stability**: paging limits + nullable filters + default request values.
- **Advanced**: upload-specific handling + multi-tenant header contracts.
- **Governance**: periodic contract reviews with security and response skills.

## Resources

- General backend baseline: [../general/SKILL.md](../general/SKILL.md)
- DTO-to-domain conversion: [../mapping/SKILL.md](../mapping/SKILL.md)
- File upload validation pattern (D-30): [../validations/SKILL.md](../validations/SKILL.md)
- Response contract alignment: [../responses/SKILL.md](../responses/SKILL.md)
- BOLA and identifier hardening: [../security/SKILL.md](../security/SKILL.md)

Related implementation notes:
- Keep request concerns separate from persistence and response envelopes.
- Validate at boundaries, then map to application-layer command/query models.
- Use this skill before introducing endpoint-level performance optimizations.

## Changelog

### v1.3 — 2026-04-21
- Standardized to operational format with required section set.
- Converted guidance to 10 atomic Do/Don't rules with rationale.
- Reduced examples to a single short snippet and preserved cross-skill links.

### v1.2 — 2026-03-28
- Trimmed encyclopedic tutorials; kept high-signal operational guidance.
