package clickmemory

import "embed"

// Files holds the click-memory plugin content embedded at build time so install works offline.
//
//go:embed .claude-plugin/plugin.json skills/*/SKILL.md docs/*.md
var Files embed.FS
