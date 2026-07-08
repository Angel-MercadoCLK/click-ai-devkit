package clicksdd

import "embed"

// Files holds the click-sdd plugin content embedded at build time so install works offline.
//
//go:embed .claude-plugin/plugin.json agents/*.md skills/*/SKILL.md
var Files embed.FS
