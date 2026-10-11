# Claude Code contributor entry point

Use [`AGENTS.md`](AGENTS.md) as the current source of truth for repository architecture, ownership,
testing, language, safety, and contributor conventions. In particular, Gentle AI v4.0.0 owns the
workflow, permissions, model selection, and shared Engram/Context7 lifecycle; Click supplies its
business and memory policy, memory guard, retained plugins, and Engram Cloud enrollment.

Claude Code is the native plugin target for Click's three retained plugins (`click-memory`,
`click-review`, and `click-skills`). The retired `click-sdd` plugin and Click model selectors must
not be treated as current interfaces. See [`documentacion/README.md`](documentacion/README.md) for
current target contracts and for clearly labeled historical planning documents.
