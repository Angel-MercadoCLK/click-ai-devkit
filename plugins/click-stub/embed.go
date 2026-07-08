// Package clickstub embeds the minimal stub plugin used by Slice 1's tracer-bullet install to
// prove the plugin-copy mechanism end to end (implementation-plan.md, Slice 1). Real plugin
// content (click-sdd, click-memory, click-review) lands in later slices and replaces this
// package's role in internal/installer.
package clickstub

import "embed"

// Files holds the stub plugin's files, embedded at build time so `click install` works offline
// (tech-spec.md §2.3 "install works offline except for whatever Engram's own runtime dependency
// resolution needs").
//
//go:embed plugin.json README.md
var Files embed.FS
