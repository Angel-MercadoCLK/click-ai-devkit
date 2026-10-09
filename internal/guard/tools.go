package guard

import (
	"regexp"
	"strings"
)

// persistingEngramTools is the single source of truth for the Engram MCP tools that PERSIST
// caller-supplied content and therefore must be screened by the memory guard. Read-only tools
// (mem_search, mem_context, mem_get_observation, ...) and tools that carry no free text
// (mem_pin, mem_delete, mem_session_start, ...) are intentionally absent.
//
// Evidence (Engram v3.2.1 `engram mcp` tools/list; the pinned ENGRAM_VERSION v1.20.0 exposes a
// subset, which is harmless because an extra entry only means "scan this tool if it exists"):
//   - mem_save            title, content, topic_key, type, ...
//   - mem_update          title, content, topic_key, find, replace, ...
//   - mem_save_prompt     content
//   - mem_session_summary content
//   - mem_session_end     summary
//   - mem_capture_passive content
//   - mem_judge           reason, evidence (free text stored with the verdict)
//   - mem_compare         reasoning (free text stored with the verdict)
//
// The OpenClaw adapter (internal/installer/assets/openclaw/click-memory-guard/plugins/hooks.js)
// keeps its own copy because it cannot import Go; a test keeps both copies identical.
// Sorted, so every derived value (the hook matcher) is deterministic.
var persistingEngramTools = []string{
	"mem_capture_passive",
	"mem_compare",
	"mem_judge",
	"mem_save",
	"mem_save_prompt",
	"mem_session_end",
	"mem_session_summary",
	"mem_update",
}

// engramToolPrefixes are the MCP tool-name forms Claude Code exposes for Engram: the plugin-bundled
// server (engram@engram plugin) and a standalone `engram` MCP server registration.
var engramToolPrefixes = []string{
	"mcp__plugin_engram_engram__",
	"mcp__engram__",
}

// PersistingEngramTools returns a copy of the sorted set of Engram tool names (without any MCP
// prefix) that persist caller-supplied content.
func PersistingEngramTools() []string {
	return append([]string(nil), persistingEngramTools...)
}

// IsPersistingEngramTool reports whether toolName is a full MCP tool name (any accepted prefix
// form) of a content-persisting Engram tool. Matching is exact and case-sensitive.
func IsPersistingEngramTool(toolName string) bool {
	for _, prefix := range engramToolPrefixes {
		bare, ok := strings.CutPrefix(toolName, prefix)
		if !ok {
			continue
		}
		for _, tool := range persistingEngramTools {
			if bare == tool {
				return true
			}
		}
	}
	return false
}

// ToolMatcherPattern returns the anchored regular expression Claude Code's PreToolUse `matcher`
// uses to select exactly the tool names IsPersistingEngramTool accepts. Claude Code treats a
// matcher containing regex metacharacters as a regular expression.
func ToolMatcherPattern() string {
	prefixes := make([]string, len(engramToolPrefixes))
	for i, prefix := range engramToolPrefixes {
		prefixes[i] = regexp.QuoteMeta(prefix)
	}
	tools := make([]string, len(persistingEngramTools))
	for i, tool := range persistingEngramTools {
		tools[i] = regexp.QuoteMeta(tool)
	}
	return "^(?:" + strings.Join(prefixes, "|") + ")(?:" + strings.Join(tools, "|") + ")$"
}
