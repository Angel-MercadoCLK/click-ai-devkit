---
name: pre-merge-checklist
description: Run the Click Seguros pre-merge checklist before a PR is approved or merged.
---

## Pre-merge checklist

- [ ] Tests relevant to the change are present and green.
- [ ] The diff does not introduce secrets, tokens, passwords, certificates, or credentials.
- [ ] The diff does not include PII, policy numbers, claim identifiers, customer identifiers, or monetary business data.
- [ ] Debug code, temporary logs, and commented-out experiments have been removed.
- [ ] Error handling is appropriate for the changed behavior.
- [ ] The change follows existing project patterns and does not introduce avoidable architecture drift.
- [ ] Documentation or developer guidance was updated when behavior or workflow changed.
- [ ] Memory-related changes still respect the deny-by-default memory policy.
- [ ] Any compliance-sensitive behavior for an insurer was reviewed with extra care.
- [ ] A human reviewer still owns the final merge decision.

## How to use it

1. Walk the checklist against the final diff.
2. Mark any failed item clearly.
3. Convert failed items into review findings with priority.
4. Do not approve the change while a blocking checklist item remains open.
