---
name: pr-review
description: Review a real Click Seguros diff or PR for correctness, security, test coverage, and standards compliance before merge.
---

## Workflow

1. Read the diff and identify the behavior that changed.
2. Check correctness against the intended scope.
3. Look for regressions, edge cases, and missing negative paths.
4. Review tests: confirm they exist, are relevant, and would catch the main failure modes.
5. Review security and compliance concerns:
   - no secrets or credentials
   - no PII or insurance data leakage
   - no unsafe memory persistence behavior
6. Check alignment with repo standards and existing architecture.
7. Report findings in English with clear priority.

## Finding priority

- **Blocking**: correctness bugs, security issues, compliance risks, missing critical tests, data leakage, merge-breaking design drift.
- **Non-blocking**: clarity, maintainability, smaller test improvements, or follow-up cleanup.

## Rules

- Be adversarial on correctness, but fair.
- Give evidence, not vibes.
- Prefer concrete fixes over broad criticism.
