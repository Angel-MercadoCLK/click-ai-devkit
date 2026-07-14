---
name: frontend-security
description: >
  ZeshOne frontend security defaults for Next.js applications.
  Trigger: When adding middleware, route protection, headers, validation, cookies, or handling untrusted input.
license: Apache-2.0
allowed-tools: Read Write Edit Bash
metadata:
  author: Zesh-One
  version: "2.1"
  inspired-by: frontend-nextjs/security.md
---

## When to Use

Load this skill when you touch security-sensitive code in frontend projects:
- `middleware.ts` / request pipeline
- route access control
- Server Actions and Route Handlers
- cookie/session handling
- rendering or storing user-provided content

Use it with `nextjs-15` and `zod-4` to keep one consistent model.

## Critical Patterns

Atomic rules (Do/Don't + Why):

1. **Do** generate a CSP nonce in middleware for every request. **Don't** hardcode nonce values. **Why:** static nonces break CSP isolation.
2. **Do** apply security headers globally (`source: "/(.*)"`). **Don't** configure headers per-page unless explicitly required. **Why:** gaps create silent attack surface.
3. **Do** enforce auth/authorization in middleware before render. **Don't** rely only on component-level checks. **Why:** middleware blocks access earlier and consistently.
4. **Do** validate all external input with Zod in Server Actions/Route Handlers. **Don't** trust `FormData` or query params directly. **Why:** input is attacker-controlled by default.
5. **Do** use `safeParse` and return structured validation errors. **Don't** throw raw parser errors to UI. **Why:** avoids leaking internals and keeps UX predictable.
6. **Do** set sensitive cookies with `httpOnly + secure + sameSite`. **Don't** store tokens in JS-readable cookies/localStorage. **Why:** reduces XSS/CSRF blast radius.
7. **Do** keep secrets server-only environment variables. **Don't** place secrets in `NEXT_PUBLIC_*`. **Why:** public variables are bundled to client.
8. **Do** prefer React escaped rendering for user content. **Don't** use `dangerouslySetInnerHTML` unless sanitized. **Why:** default escaping is your safest baseline.
9. **Do** add rate limiting to auth and sensitive mutation endpoints. **Don't** assume upstream infra always protects you. **Why:** app-level controls are defense in depth.
10. **Do** treat endpoint hiding/embedding as architecture encapsulation only. **Don't** treat obfuscation as a security control by itself. **Why:** real security still requires backend authentication, authorization, and server-side validation on every request.
11. **Do** follow this skill's current security patterns and framework-version guidance; **Don't** use legacy/deprecated security APIs from older versions; **Do** verify exact-version syntax/examples/definitions in Context7 before version-sensitive hardening changes. **Why:** reduces outdated-control gaps.

Single reference snippet (nonce + guard + cookie policy context):

```ts
// middleware.ts (Next.js 15): nonce + pre-render guard
import { NextResponse, type NextRequest } from "next/server";

export function middleware(req: NextRequest) {
  const nonce = Buffer.from(crypto.randomUUID()).toString("base64");
  const h = new Headers(req.headers);
  h.set("x-nonce", nonce);

  const isProtected = req.nextUrl.pathname.startsWith("/dashboard");
  const hasSession = Boolean(req.cookies.get("session")?.value);
  if (isProtected && !hasSession) {
    return NextResponse.redirect(new URL("/login", req.url));
  }

  const res = NextResponse.next({ request: { headers: h } });
  res.headers.set("Content-Security-Policy", `script-src 'self' 'nonce-${nonce}'; object-src 'none'; base-uri 'self'`);
  return res;
}
```

## Constraints & Tradeoffs

- Strict CSP can break third-party scripts; allowlist intentionally and minimally.
- Middleware checks add per-request overhead; keep auth check lightweight.
- `secure: true` cookies require HTTPS in production; local dev may need conditional handling.
- Zod validation everywhere increases boilerplate; cost is acceptable for explicit contracts.
- Rate limiting can block legitimate bursts; tune limits per endpoint risk.
- Some legacy widgets require inline scripts; isolate and document every exception.
- Strong header baselines may conflict with embedded partner content; prefer explicit opt-in route carveouts.

Operational verification before merge:

- Confirm nonce exists on request path and is reused consistently for CSP script/style directives.
- Confirm protected routes are blocked in middleware without depending on UI logic.
- Confirm every write path has Zod schema + `safeParse` handling.
- Confirm auth cookies use `httpOnly`, `secure`, and `sameSite` together.
- Confirm no secret appears in client bundles (`NEXT_PUBLIC_*` review).

## Anti-Patterns

- Protecting routes only in page components.
- Copy-pasting different CSP/header policies across routes.
- Returning raw backend error payloads directly to client.
- Assuming hidden endpoints are "secure" without backend authz/validation enforcement.
- Using `NEXT_PUBLIC_API_KEY` or similar secret exposure.
- Storing auth token in localStorage "for convenience".
- Sanitizing HTML with ad-hoc regex instead of vetted sanitizers.
- Shipping middleware matcher exclusions without tests/review.
- "Temporary" bypasses left in middleware after debugging.
- Per-feature security logic that duplicates global middleware responsibilities.
- Treating validation as optional for internal/admin forms.

## Progressive Disclosure

Apply only what the task needs, in this order:

1. **Version checkpoint**
   - confirm installed framework/security package versions
   - check Context7 exact-version docs before API-sensitive security edits
2. **Baseline hardening**
   - global security headers
   - nonce generation and forwarding (`x-nonce`)
3. **Access control**
   - protect private/admin routes in middleware before render
4. **Input and output safety**
   - Zod `safeParse` on all mutation inputs
   - escaped rendering by default, sanitize only when HTML is required
5. **Session and abuse controls**
   - secure cookie flags
   - endpoint rate limiting

Escalate to dedicated security review when introducing OAuth callbacks, embedded iframes, or third-party inline scripts.

Task sizing guidance:

- Small task: header fixes, matcher correction, single schema validation.
- Medium task: middleware guard expansion + cookie policy normalization.
- Large task: CSP policy redesign affecting multiple third-party integrations (requires explicit review window).

## Resources

- Next.js middleware docs: https://nextjs.org/docs/app/building-your-application/routing/middleware
- Next.js CSP guide: https://nextjs.org/docs/app/guides/content-security-policy
- Cross-reference: `skills/frontend/nextjs-15/SKILL.md`
- Cross-reference: `skills/frontend/zod-4/SKILL.md`
- Cross-reference: `skills/frontend/tanstack-query/SKILL.md` (protect query-backed private pages via middleware first)
- OWASP Cheat Sheet Series: https://cheatsheetseries.owasp.org/
- OWASP Session Management: https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html

## Changelog

### v2.1 — 2026-04-22
- Aligned versioning metadata and changelog with today's frontend skill updates.

### v2.0 — 2026-04-21
- Rewritten into concise operational guidance with repository defaults and explicit constraints.
- Enforced middleware-first guards, CSP nonce handling, and secure cookie policy conventions.
