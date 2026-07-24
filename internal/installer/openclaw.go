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

// openClawEngramMCPArgs is the "args" value written for the engram mcpServers entry.
//
// VERIFY-AT-APPLY (design #1666's resolved-risk note, item 4): this mirrors the Engram Claude Code
// plugin's OWN bundled `.mcp.json` — `{"mcpServers":{"engram":{"command":"engram","args":["mcp","--tools=agent"]}}}`
// — confirmed via a raw fetch of plugin/claude-code/.mcp.json in the upstream Engram repo
// (documentacion/spikes/spike-a-engram-packaging.md, Q1/Q3). It has NOT been independently
// re-confirmed against `engram --help` in THIS apply session (no Bash tool was available). If
// `engram --help` or OpenClaw's actual MCP handshake disagrees, only this one variable needs to
// change — SyncOpenClawMCPConfig's read-merge-write logic is unaffected either way.
var openClawEngramMCPArgs = []string{"mcp", "--tools=agent"}

// openClawEngramMCPEntry is the JSON shape of the "engram" entry inside openclaw.json's
// mcpServers object — confirmed real format (design #1666's resolved risk):
// {"command":"engram","args":[...],"transport":"stdio"}. Local-only by default (spec's Engram MCP
// default decision); cloud enrollment is out of scope (sibling proposal engram-cloud-wiring).
type openClawEngramMCPEntry struct {
	Command   string   `json:"command"`
	Args      []string `json:"args"`
	Transport string   `json:"transport"`
}

// SyncOpenClawMCPConfig registers Engram as a local MCP server inside OpenClaw's own
// ~/.openclaw/openclaw.json (cfg.OpenClawMCPConfigPath), preserving every unrelated top-level key
// and every unrelated mcpServers entry via json.RawMessage passthrough (design's "MCP wiring"
// decision: read-merge-write, never regenerate the file wholesale). Idempotent by construction:
// re-running with the same inputs produces byte-identical output, because encoding/json marshals
// map keys in sorted order on every call — there is no read-mutate-append step that could grow the
// file across re-runs. It is a no-op when cfg.OpenClawHome is empty, mirroring
// SyncOpenClawWorkspace's guard.
func SyncOpenClawMCPConfig(cfg Config) error {
	if cfg.OpenClawHome == "" {
		return nil
	}
	path := cfg.OpenClawMCPConfigPath()

	top := map[string]json.RawMessage{}
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("installer: read %s: %w", path, err)
		}
	} else if len(data) > 0 {
		if err := json.Unmarshal(data, &top); err != nil {
			return fmt.Errorf("installer: parse %s: %w", path, err)
		}
	}

	servers := map[string]json.RawMessage{}
	if raw, ok := top["mcpServers"]; ok && len(raw) > 0 {
		if err := json.Unmarshal(raw, &servers); err != nil {
			return fmt.Errorf("installer: parse %s mcpServers: %w", path, err)
		}
	}

	entry, err := json.Marshal(openClawEngramMCPEntry{
		Command:   "engram",
		Args:      openClawEngramMCPArgs,
		Transport: "stdio",
	})
	if err != nil {
		return fmt.Errorf("installer: marshal engram mcp entry: %w", err)
	}
	servers["engram"] = entry

	serversRaw, err := json.Marshal(servers)
	if err != nil {
		return fmt.Errorf("installer: marshal mcpServers: %w", err)
	}
	top["mcpServers"] = serversRaw

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
