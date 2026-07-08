# Allowed Memory

Only the categories below may be proposed for persistence.

## Allowed categories

### Architecture decisions

Store decisions about system structure, boundaries, or integration choices.

**Example:**
"The installer copies embedded plugin assets recursively because plugin content now includes nested `.claude-plugin`, `agents`, and `skills` directories."

### Design decisions

Store technical approach decisions that explain how a feature was implemented.

**Example:**
"The Click SDD flow keeps the curator agent in `click-sdd` while memory policy mechanics live in `click-memory`."

### Conventions

Store team conventions that help future contributors follow the established approach.

**Example:**
"All plugin markdown artifacts are written in English, while the orchestrator replies to developers in Spanish."

### Reusable patterns

Store patterns that can be applied to similar work later.

**Example:**
"Installer tests must resolve the Claude home through `CLICK_CLAUDE_HOME` and always use `t.TempDir()`."

### Technical gotchas

Store non-obvious implementation lessons.

**Example:**
"Claude Code hook stdout must contain only machine JSON or the hook parser fails."

### Bugfixes

Store bugfix summaries when they explain a durable technical lesson.

**Example:**
"Go RE2 patterns cannot use PCRE-only escapes like `\\d`, so guard regexes must use explicit numeric classes."

## Entry quality rules

- Write entries in English.
- Keep them concise and reusable.
- Prefer technical cause and effect over narrative detail.
- Remove any real business or customer data before proposing the entry.
