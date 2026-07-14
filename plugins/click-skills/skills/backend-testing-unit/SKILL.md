---
name: backend-testing-unit
description: >
  Operational unit-testing defaults for backend code using xUnit, NSubstitute, and FluentAssertions, focused on services, validators, mappings, and domain behavior.
  Trigger: When writing or reviewing unit tests for business logic, validation rules, mapping logic, or authorization/ownership branches in backend services.
license: Apache-2.0
metadata:
  author: Zesh-One
  version: "2.0"
allowed-tools: Read Edit Write Bash Glob Grep
---

## When to Use

- Writing unit tests for service-layer behavior and branch logic
- Verifying validator rules without infrastructure concerns
- Testing mappings and pure transformations
- Reviewing whether tests assert behavior (not implementation details)

---

## Critical Patterns

1) **DO** use xUnit + NSubstitute + FluentAssertions as defaults; **DON'T** mix frameworks in new test files.  
Why: consistency lowers onboarding and maintenance cost.

2) **DO** prioritize service, validator, and mapping tests; **DON'T** default to controller unit tests.  
Why: controller behavior is usually better verified in integration tests.

3) **DO** structure tests with AAA and a single Act; **DON'T** combine multiple actions in one test.  
Why: failures remain diagnosable and intent stays clear.

4) **DO** name tests as `{Method}_When{Condition}_Should{Expected}`; **DON'T** use generic names like `Test1`.  
Why: test reports must be self-explanatory.

5) **DO** use test data builders for entities/value setup; **DON'T** inline large object graphs repeatedly.  
Why: reduces duplication and makes intent explicit.

6) **DO** assert observable behavior and outputs; **DON'T** assert private/internal implementation details.  
Why: resilient tests survive safe refactors.

7) **DO** verify ownership/policy branches in services (`success`, `not found`, `forbidden`); **DON'T** only test happy path.  
Why: authorization bugs typically hide in non-happy branches.

8) **DO** mock collaborators at boundaries (repositories/ports); **DON'T** mock DTOs/value objects.  
Why: excessive mocking creates fragile, low-signal tests.

9) **DO** keep one behavioral reason per test; **DON'T** create “kitchen sink” assertions for unrelated outcomes.  
Why: focused tests communicate domain rules better.

10) **DO** keep unit tests fast and deterministic; **DON'T** involve DB/network/host lifecycle in this layer.  
Why: slow/flaky tests break feedback loops.

```csharp
[Fact]
public async Task GetByIdAsync_WhenOwnerDiffers_ShouldThrowForbiddenException()
{
    var userId = Guid.NewGuid();
    var order = new OrderBuilder().WithUserId(Guid.NewGuid()).Build();
    _repo.GetByIdAsync(order.Id).Returns(order);

    await FluentActions.Awaiting(() => _sut.GetByIdAsync(order.Id, userId))
        .Should().ThrowAsync<ForbiddenException>();
}
```

---

## Constraints & Tradeoffs

- Unit tests provide strong logic confidence but do NOT validate middleware/routing/pipeline wiring.
- Avoiding controller unit tests may feel less immediate, but reduces coupling to framework internals.
- Builder patterns improve readability at cost of maintaining builder helpers.
- Strict behavior assertions increase robustness but require clearer domain expectations.

---

## Anti-Patterns

- Controller unit testing as default strategy.
- Tests that assert framework internals rather than domain behavior.
- Multiple Acts in a single test method.
- Over-mocking simple data types and objects.
- Reliance on DB/network/hosted-service behavior in unit tests.
- Naming that hides intent (`ShouldWork`, `TestCase1`, etc.).

---

## Progressive Disclosure

1. **Baseline for every new unit test:** rules 1, 3, 4, 6, 10.
2. **Service logic depth:** add rules 7 and 9 for branch completeness and focus.
3. **Maintainability at scale:** add rules 5 and 8 when fixture setup grows.
4. **Boundary check:** if test needs pipeline/routing/EF behavior, move to integration skill.

---

## Resources

- Security skill for ownership/policy scenarios: [`../security/SKILL.md`](../security/SKILL.md)
- Backend conventions and structure: [`../general/SKILL.md`](../general/SKILL.md)
- xUnit: https://xunit.net/docs/getting-started/netcore/cmdline
- NSubstitute: https://nsubstitute.github.io/
- FluentAssertions: https://fluentassertions.com/introduction

---

## Changelog

### v2.0 — 2026-04-21
- P2 trim: reduced tutorial/setup density and kept operational defaults only.
- Standardized required section structure and concise rule format.
- Added 10 atomic Do/Don't rules with short rationale.
- Kept one short snippet and preserved cross-reference to security ownership tests.
