---
name: shared-github-pr
description: >
  Operational GitHub Pull Request rules for branch, title, body, and merge readiness.
  Trigger: When creating, updating, or reviewing pull requests with GitHub CLI workflows.
license: Apache-2.0
allowed-tools: Read Edit Write Glob Grep
metadata:
  author: Zesh-One
  version: "1.1"
  inspired-by: gentleman-programming/github-pr
---

## When to Use

Load this skill when preparing PRs for team review, enforcing naming conventions, or creating PRs via `gh` with a predictable structure.

## Critical Patterns

1. **Do** use Conventional Commit PR titles (`type(scope): description`); **don't** use vague titles. **Why:** improves changelog and triage quality.
2. **Do** align branch prefix with change intent (`feat/`, `fix/`, `refactor/`, `chore/`, `docs/`); **don't** use generic `update/` prefixes. **Why:** branch purpose is immediately visible.
3. **Do** target `alfa` as base unless explicitly overridden; **don't** default to `main`. **Why:** team integration flow depends on staging branch.
4. **Do** keep one logical concern per PR; **don't** mix unrelated features and fixes. **Why:** review and rollback remain tractable.
5. **Do** include `## Summary`, `## Changes`, and `## Testing`; **don't** submit body-less PRs. **Why:** reviewers need context, scope, and evidence.
6. **Do** link closing issue references (`Closes #123`); **don't** rely on manual tracker updates. **Why:** preserves automated traceability.
7. **Do** state risk and migration notes when behavior changes; **don't** hide breaking impact in code diffs. **Why:** protects release planning.
8. **Do** choose merge strategy intentionally (merge/squash/rebase); **don't** treat squash as universal default. **Why:** history needs differ by context.
9. **Do** run local verification before PR creation; **don't** open PRs with known failing checks. **Why:** avoids reviewer churn.
10. **Do** prefer heredoc bodies for non-trivial PR descriptions; **don't** compose long escaped inline strings. **Why:** fewer CLI formatting errors.

```bash
gh pr create --title "feat(users): add avatar upload" --base alfa --body "$(cat <<'EOF'
## Summary
- Add avatar upload flow to user settings.

## Changes
- Add upload action and client cropper integration.

## Testing
- [x] Added/updated tests
- [x] Manual verification completed

Closes #87
EOF
)"
```

## Constraints & Tradeoffs

- Strict title/body conventions increase upfront writing effort, but reduce review ambiguity.
- Single-concern PRs improve quality yet may increase PR count for large initiatives.
- Base-on-`alfa` flow protects `main`, but adds one more promotion step.
- Rich PR templates aid traceability, but must stay concise to avoid noise.
- Merge strategy flexibility is useful, though it requires explicit team alignment per case.

PR readiness checklist:
- Confirm branch prefix and PR title follow the same intent.
- Confirm base branch is `alfa` unless release policy says otherwise.
- Confirm required body sections include test evidence and issue closure.
- Confirm risks/migrations are documented when behavior changes.
- Confirm local checks pass before opening review.

Review quality notes:
- Keep `Summary` focused on intent and impact, not code diff narration.
- Keep `Changes` section concrete and auditable.
- Keep `Testing` aligned to repository-required checks.

## Anti-Patterns

1. PR title like `update stuff` or `fixes` with no scope.
2. Multi-feature PRs that cannot be safely reverted as one unit.
3. Missing issue closure link and missing test evidence section.
4. Opening PR against `main` by default in an `alfa`-first workflow.
5. Giant inline CLI body strings with escaped newlines that break formatting.
6. Hiding risky changes under `chore` or `refactor` labels.
7. Forcing a merge strategy that conflicts with repository history policy.

## Progressive Disclosure

1. Start with correct branch prefix + conventional PR title.
2. Add required PR body skeleton (`Summary`, `Changes`, `Testing`, issue link).
3. Add risk/migration notes when behavior is not backward compatible.
4. Standardize `gh` heredoc usage for larger PR descriptions.
5. Align release-ready checks with repository workflows before requesting review.

Adoption sequence:
- **Step 1**: naming conventions and base branch correctness.
- **Step 2**: body template completeness and issue linkage.
- **Step 3**: evidence quality (tests, manual checks, risk notes).
- **Step 4**: merge-strategy decisions aligned with release cadence.

Execution checkpoints:
- Validate issue link resolves to the intended work item.
- Validate reviewers can reproduce test evidence from provided notes.
- Validate PR scope still matches title after final changes.

## Resources

- [GitHub CLI PR Docs](https://cli.github.com/manual/gh_pr_create)
- [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/)
- [GitHub Pull Requests Guide](https://docs.github.com/en/pull-requests)
- [Repository release-readiness checks](../../../README.md)

Related operational pairings:
- Use with [../../frontend/react-19/SKILL.md](../../frontend/react-19/SKILL.md) and [../../backend/general/SKILL.md](../../backend/general/SKILL.md) when PR spans platform contracts.
- Use with [../../backend/testing-unit/SKILL.md](../../backend/testing-unit/SKILL.md) to report backend evidence consistently.
- Use with [../../frontend/tanstack-query/SKILL.md](../../frontend/tanstack-query/SKILL.md) to describe cache-impacting UI behavior changes.
- Keep PR scope aligned with one issue lifecycle for predictable release notes.

## Changelog

### v1.1 — 2026-04-21
- Standardized to operational format with required section set.
- Converted conventions into 10 atomic Do/Don't rules with rationale.
- Reduced examples to one concise `gh pr create` snippet.
- Added readiness checklist and phased adoption guidance.
