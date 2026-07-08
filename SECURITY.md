# Security

This is a **placeholder**. The full policy is authored in Slice 4, once the memory-guard's
pattern categories (`internal/guard`) are final — see `documentacion/tech-spec.md` §7.2 and
`documentacion/00-decisions-and-open-questions.md` D15.

## Purpose (summary, to be expanded)

click-ai-devkit enforces a deny-by-default memory policy: only technical knowledge (architecture
decisions, conventions, patterns, gotchas) may be persisted to Engram. No PII, policy numbers,
claims data, amounts, or customer identifiers may ever reach persistent memory. Enforcement is a
deterministic PreToolUse hook (`memory-guard`), independent of model compliance — not just a
policy document.

The final version of this file will cover: scope, the data-safety guarantee, how enforcement
works, how to report a suspected false negative, the red-team test suite, rollback/kill-switch,
and the pattern-set changelog.
