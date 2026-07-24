package installer

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/modelconfig"
)

// DefaultOpenClawAgentsContent is the managed AGENTS.md body written under
// cfg.OpenClawWorkspaceDir() when OpenClaw is detected — OpenClaw's counterpart to
// DefaultManagedContent (claudemd.go). Kept deliberately minimal (design's "write minimal
// idempotent managed AGENTS.md/SOUL.md" decision): it points the OpenClaw agent at click-sdd
// conventions and the click-memory policy docs, without duplicating CLAUDE.md's full content.
const DefaultOpenClawAgentsContent = `This workspace is managed by click-ai-devkit for OpenClaw. For a new change, start with the installed OpenClaw-native click-sdd skill in the skills/click-sdd directory, then follow the phase skills it names.
The OpenClaw workflow uses its native provider/model configuration. Configure it with click configure-openclaw-model provider/model; Click delegates that operation to the documented OpenClaw CLI and keeps model-profile.json as local rollback metadata. Claude-specific agents and plugin registries are not installed or required here.
Before any mem_save, review the click-memory plugin's policy docs (memory-policy.md, allowed-memory.md, forbidden-memory.md) under its installed docs/ directory. The deterministic memory-guard hook enforces this policy even if an agent attempts something unsafe.
This block is managed by click: edit via "click update" and remove via "click uninstall".`

// DefaultOpenClawSoulContent is the managed SOUL.md body written under
// cfg.OpenClawWorkspaceDir() when OpenClaw is detected. SOUL.md conventionally carries an agent's
// standing behavioral posture (as distinct from AGENTS.md's task/workflow conventions) — kept
// minimal and technical, matching this codebase's own conservative memory-safety stance.
const DefaultOpenClawSoulContent = `Be direct, technical, and conservative: verify claims before stating them, and prefer the smallest safe change over a larger rewrite.
Persist only durable technical knowledge to memory — architecture decisions, conventions, reusable patterns, and bugfixes — never business or customer data.
This block is managed by click: edit via "click update" and remove via "click uninstall".`

// SyncOpenClawWorkspace writes/refreshes AGENTS.md and SOUL.md's managed block under
// cfg.OpenClawWorkspaceDir(), reusing WriteManagedBlock (claudemd.go) verbatim — the exact same
// idempotent marker mechanism CLAUDE.md already uses, applied here for the first time to files
// other than CLAUDE.md (openclaw-target-support spec's openclaw-managed-files capability). It is a
// no-op (nil, no error, no file touched) when cfg.OpenClawHome is empty — defense in depth: the
// CLI wiring (install.go/update.go) already decides whether to call this at all based on
// detect+confirm, but this guard means the function is also safe to call unconditionally.
func SyncOpenClawWorkspace(cfg Config) error {
	if cfg.OpenClawHome == "" {
		return nil
	}
	if err := WriteManagedBlock(cfg.OpenClawAgentsMDPath(), DefaultOpenClawAgentsContent); err != nil {
		return fmt.Errorf("installer: sync OpenClaw AGENTS.md: %w", err)
	}
	if err := WriteManagedBlock(cfg.OpenClawSoulMDPath(), DefaultOpenClawSoulContent); err != nil {
		return fmt.Errorf("installer: sync OpenClaw SOUL.md: %w", err)
	}
	return nil
}

// SyncOpenClawMCPConfig is a CLEANUP-ONLY step, pending OpenClaw's confirmed native MCP
// registration mechanism (a separate future fast-follow — see SyncCodexMCP in codexmcp.go for the
// equivalent capability already wired for Codex using ITS confirmed CLI syntax).
//
// This used to write a top-level "mcpServers" key into OpenClaw's own ~/.openclaw/openclaw.json to
// register Engram. That assumption was WRONG: real evidence from a live OpenClaw instance's own
// `validate config` proves OpenClaw's schema does not recognize a top-level "mcpServers" key at all
// ("Unrecognized key: \"mcpServers\""), and writing it there corrupted every affected install,
// producing repeated "mcp loopback: request handling failed: Invalid config" errors in OpenClaw's
// own session. This function now NEVER writes that key under any circumstances.
//
// Instead it HEALS already-broken installs: if openclaw.json currently has a top-level "mcpServers"
// key, that key is removed and the file is written back — real `validate config` evidence proves
// OpenClaw recognizes no legitimate use of that key, so its presence can only be click's own past
// mistake, and removing it is always safe. Every other top-level key is preserved byte-for-byte via
// the same json.RawMessage passthrough this file already used for read-merge-write.
//
// It is a no-op — no read past the initial stat, and definitely no write — when: cfg.OpenClawHome is
// empty (mirrors SyncOpenClawWorkspace's guard); openclaw.json does not exist yet (this step is
// cleanup-only, never creation — there is nothing to clean); or the file exists but already has no
// top-level "mcpServers" key (already clean, re-running is a true no-op).
func SyncOpenClawMCPConfig(cfg Config) error {
	if cfg.OpenClawHome == "" {
		return nil
	}
	path := cfg.OpenClawMCPConfigPath()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("installer: read %s: %w", path, err)
	}
	if len(data) == 0 {
		return nil
	}

	top := map[string]json.RawMessage{}
	if err := json.Unmarshal(data, &top); err != nil {
		return fmt.Errorf("installer: parse %s: %w", path, err)
	}

	if _, ok := top["mcpServers"]; !ok {
		return nil
	}
	delete(top, "mcpServers")

	if err := writeJSONFile(path, top); err != nil {
		return fmt.Errorf("installer: write %s: %w", path, err)
	}
	return nil
}

// SyncOpenClawModelProfile writes Click's resolved profile and per-phase model map as a portable
// recommendation under OpenClaw's managed home. It does not modify openclaw.json or claim to apply
// aliases to OpenClaw, whose native model/provider API is not established here.
func SyncOpenClawModelProfile(cfg Config, profile modelconfig.ProfileName, models map[modelconfig.Phase]string) error {
	if cfg.OpenClawHome == "" {
		return nil
	}
	if err := SaveModelProfile(cfg.OpenClawModelProfilePath(), profile, models); err != nil {
		return fmt.Errorf("installer: write OpenClaw model profile: %w", err)
	}
	return nil
}
