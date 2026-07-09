---
name: sdd-code
description: "Implement approved Click Seguros tasks with strict TDD by default: failing test first, implementation second, refactor third."
---

## Workflow

1. Read the approved tasks, design, and PRD.
2. Write a failing test first for the next task.
3. Implement the smallest code change that makes the test pass.
4. Refactor while keeping tests green.
5. Repeat task by task until the assigned scope is done.

## Default mode

- Strict TDD is on by default.
- The only opt-out is an explicit spike or non-production exception.
- If TDD is waived, record the reason clearly.

## Rules

- Do not skip the failing-test step by default.
- Keep changes aligned with the approved design.
- Keep the developer informed of blockers and progress.
