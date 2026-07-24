package installer

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// claudeHomeEnvOverride lets tests (and power users) point click at a directory other than the
// real ~/.claude, without touching the developer's actual Claude Code install. Every test that
// exercises install/doctor/uninstall MUST use this override with a t.TempDir() — never the real
// home directory (implementation brief's hard safety rule).
const claudeHomeEnvOverride = "CLICK_CLAUDE_HOME"

// openClawHomeEnvOverride mirrors claudeHomeEnvOverride for OpenClaw: it lets tests (and power
// users) point click at a directory other than the real ~/.openclaw. Every test that exercises
// OpenClaw install/update writes MUST use this override with a t.TempDir() — never the real home
// directory, same hard safety rule as claudeHomeEnvOverride.
const openClawHomeEnvOverride = "CLICK_OPENCLAW_HOME"

// clickStateHomeEnvOverride lets tests and power users inject a Click-owned neutral state root
// independent of any specific runtime home.
const clickStateHomeEnvOverride = "CLICK_STATE_HOME"

var userConfigDir = os.UserConfigDir

// Config carries every path click's installer needs. It is deliberately just ClaudeHome today —
// Slice 1 only touches the plugin dir and CLAUDE.md, both derived from it.
type Config struct {
	// ClaudeHome is the root of the target Claude Code installation, normally ~/.claude.
	ClaudeHome string

	// OpenClawHome is the root of a detected OpenClaw installation, normally ~/.openclaw. It stays
	// the zero value (empty string) when OpenClaw is not detected on this machine (or
	// --skip-openclaw was passed) — a valid, silent state (openclaw-target-support spec's
	// install-config capability, "OpenClaw absent" scenario): every derived OpenClawXxxPath() below
	// still resolves (as empty-rooted paths), and every Claude-only path (ClaudeMDPath,
	// SettingsPath, ...) is completely unaffected. Populated by install.go/update.go via
	// ResolveOpenClawHome(), gated on OpenClawAvailable() — see runInstall's cfg construction for
	// the detect+confirm wiring.
	OpenClawHome string

	// CodexHome is the Codex user home, normally ~/.codex. This slice manages only AGENTS.md there.
	CodexHome string

	// ClickStateHome is Click's neutral, Click-owned state root. When populated, target selection and
	// other runtime-neutral lifecycle artifacts live here instead of under a runtime-specific home.
	ClickStateHome string
}

// ResolveClickStateHome resolves Click's neutral state root: the CLICK_STATE_HOME override when set,
// otherwise the OS user config directory joined with click-ai-devkit.
func ResolveClickStateHome() (string, error) {
	if v := os.Getenv(clickStateHomeEnvOverride); v != "" {
		return v, nil
	}
	root, err := userConfigDir()
	if err != nil {
		return "", fmt.Errorf("installer: resolve click state home: %w", err)
	}
	return filepath.Join(root, "click-ai-devkit"), nil
}

// ResolveClaudeHome resolves the Claude Code home directory click should install into: the
// CLICK_CLAUDE_HOME env override if set (used by tests and advanced overrides), otherwise
// <user home>/.claude.
func ResolveClaudeHome() (string, error) {
	if v := os.Getenv(claudeHomeEnvOverride); v != "" {
		return v, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("installer: resolve claude home: %w", err)
	}
	return filepath.Join(home, ".claude"), nil
}

// ResolveOpenClawHome resolves the OpenClaw home directory click should write into when OpenClaw
// is detected (and not skipped via --skip-openclaw): the CLICK_OPENCLAW_HOME env override if set
// (used by tests and advanced overrides), otherwise <user home>/.openclaw. Mirrors
// ResolveClaudeHome's exact pattern. Callers are responsible for deciding WHETHER to call this at
// all (openclaw-target-support spec's skip-on-absent semantics) — this function only resolves the
// path, it never probes whether OpenClaw is actually installed.
func ResolveOpenClawHome() (string, error) {
	if v := os.Getenv(openClawHomeEnvOverride); v != "" {
		return v, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("installer: resolve openclaw home: %w", err)
	}
	return filepath.Join(home, ".openclaw"), nil
}

// ResolveCodexHome resolves CODEX_HOME when set, otherwise <user home>/.codex.
func ResolveCodexHome() (string, error) {
	if v := os.Getenv("CODEX_HOME"); v != "" {
		return v, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("installer: resolve codex home: %w", err)
	}
	return filepath.Join(home, ".codex"), nil
}

// ClickSDDPluginDir is where the click-sdd plugin is installed under this Config's ClaudeHome.
func (c Config) ClickSDDPluginDir() string {
	return filepath.Join(c.ClaudeHome, "plugins", "click-sdd")
}

// ClickMemoryPluginDir is where the click-memory plugin is installed under this Config's ClaudeHome.
func (c Config) ClickMemoryPluginDir() string {
	return filepath.Join(c.ClaudeHome, "plugins", "click-memory")
}

// ClickReviewPluginDir is where the click-review plugin is installed under this Config's ClaudeHome.
func (c Config) ClickReviewPluginDir() string {
	return filepath.Join(c.ClaudeHome, "plugins", "click-review")
}

// KnownMarketplacesPath is Claude Code's plugin marketplace registry.
func (c Config) KnownMarketplacesPath() string {
	return filepath.Join(c.ClaudeHome, "plugins", "known_marketplaces.json")
}

// InstalledPluginsPath is Claude Code's installed plugins registry.
func (c Config) InstalledPluginsPath() string {
	return filepath.Join(c.ClaudeHome, "plugins", "installed_plugins.json")
}

// ClickPluginStagingDir is a Click-owned, local drop location for future plugin content. Listing it
// never creates or reads arbitrary files from it; native target registration remains separate.
func (c Config) ClickPluginStagingDir() string {
	return filepath.Join(c.ClaudeHome, "click-ai-devkit", "plugins")
}

// DefaultEngramBinaryPath is where Click-managed Engram binaries are expected to live locally.
func (c Config) DefaultEngramBinaryPath() string {
	name := "engram"
	if runtime.GOOS == "windows" {
		name = "engram.exe"
	}
	return filepath.Join(c.ClaudeHome, "bin", name)
}

// ClaudeMDPath is the managed CLAUDE.md file's path under this Config's ClaudeHome.
func (c Config) ClaudeMDPath() string {
	return filepath.Join(c.ClaudeHome, "CLAUDE.md")
}

// SettingsPath is Claude Code's settings.json under ClaudeHome.
func (c Config) SettingsPath() string {
	return filepath.Join(c.ClaudeHome, "settings.json")
}

// EngramStatePath stores the Click-managed pinned Engram metadata.
func (c Config) EngramStatePath() string {
	return filepath.Join(c.ClaudeHome, "click-ai-devkit", "engram.json")
}

// EngramCloudStatePath stores the Engram Cloud enrollment state ({enrolled, server, project}).
// It mirrors EngramStatePath's location under click-ai-devkit/ but returns empty when ClaudeHome
// is unset, letting callers no-op safely on an unconfigured installer.
func (c Config) EngramCloudStatePath() string {
	if c.ClaudeHome == "" {
		return ""
	}
	return filepath.Join(c.ClaudeHome, "click-ai-devkit", "engram-cloud.json")
}

// ModelsPath stores the per-phase click-sdd model selection (D25) so `click update` can re-apply
// the same choices and `click doctor` can report them.
func (c Config) ModelsPath() string {
	return filepath.Join(c.ClaudeHome, "click-ai-devkit", "models.json")
}

// BackupDir is where snapshot.go stores the single run-start snapshot of CLAUDE.md + settings.json
// (install-reliability-foundation change): one run writes BackupDir()/latest/{CLAUDE.md,
// settings.json,manifest.json}, overwritten on the next successful run (single-latest retention —
// see design's "Retention" decision). Kept under click-ai-devkit/ rather than as flat siblings next
// to the snapshotted files themselves, matching where engram.json/models.json already live.
func (c Config) BackupDir() string {
	return filepath.Join(c.ClaudeHome, "click-ai-devkit", "backups")
}

// ProfileArtifactPath is where a named orchestration profile's generic RuntimeProfile JSON
// artifact lives (design D2, salvaged by PR2b's profile_artifacts.go as substrate for the
// separate agent-builder-flow change). It is a DIFFERENT file from ModelsPath(): ModelsPath()
// holds the single active profile + resolved map this change's install/update/doctor flow reads
// and writes; ProfileArtifactPath(name) holds one JSON artifact per profile, keyed by name, and is
// not itself the active-profile store.
func (c Config) ProfileArtifactPath(name string) string {
	return filepath.Join(c.ClaudeHome, "click-ai-devkit", "profiles", name+".json")
}

// ProfileAgentsDir is where PR2b's profile_artifacts.go writes generated markdown agent files for
// a named profile (RenderMarkdownAgent/SaveMarkdownAgent substrate). Unwired to any UI in this
// change — see the orchestration-profiles-reconciled spec's explicit out-of-scope agent-creation
// boundary.
func (c Config) ProfileAgentsDir(name string) string {
	return filepath.Join(c.ClaudeHome, "click-ai-devkit", "profiles", name, "agents")
}

// Context7ConfigPath is Claude Code's own user-scope config file — the same file our runner's
// `claude mcp add --scope user ...` writes to, whose top-level `mcpServers` key holds user-scope
// MCP entries. It MUST mirror exactly where the claude subprocess writes (see execCommandRunner):
//   - with CLICK_CLAUDE_HOME override set → <override>/.claude.json (CLAUDE_CONFIG_DIR is forced there)
//   - real run (no override)             → <OS home>/.claude.json (home ROOT, NOT <ClaudeHome>/.claude.json)
//
// Getting this wrong caused a real bug: `<ClaudeHome>/.claude.json` (= ~/.claude/.claude.json) is a
// file a normal Claude Code session never reads, so HasContext7 always reported "missing" on a real
// machine. Reading the file directly keeps HasContext7 a pure filesystem read (no `claude mcp get`).
func (c Config) Context7ConfigPath() string {
	// In production ClaudeHome is <home>/.claude, but claude stores user-scope MCP config at
	// <home>/.claude.json (home ROOT) — a real run does NOT force CLAUDE_CONFIG_DIR (see
	// execCommandRunner.commandEnv), so `claude mcp add` lands there. Under a CLICK_CLAUDE_HOME
	// override (tests/power-users) ClaudeHome is an arbitrary dir and claude, with
	// CLAUDE_CONFIG_DIR pointed at it, writes <dir>/.claude.json INSIDE it. Distinguish the two by
	// whether ClaudeHome is the default "~/.claude" (basename ".claude") vs an override dir. This
	// keeps HasContext7 a hermetic, env-free filesystem read that mirrors where the runner writes.
	if filepath.Base(c.ClaudeHome) == ".claude" {
		return filepath.Join(filepath.Dir(c.ClaudeHome), ".claude.json")
	}
	return filepath.Join(c.ClaudeHome, ".claude.json")
}

// Context7StatePath stores click's own bookkeeping about the Context7 MCP install — specifically
// install ownership — mirroring EngramStatePath's shape and purpose.
func (c Config) Context7StatePath() string {
	return filepath.Join(c.ClaudeHome, "click-ai-devkit", "context7.json")
}

// OpenClawWorkspaceDir is where OpenClaw's per-workspace managed files (AGENTS.md, SOUL.md) live,
// mirroring how ClaudeMDPath derives from ClaudeHome (openclaw-target-support spec's
// openclaw-managed-files capability).
func (c Config) OpenClawWorkspaceDir() string {
	return filepath.Join(c.OpenClawHome, "workspace")
}

// OpenClawAgentsMDPath is the managed AGENTS.md file's path under this Config's
// OpenClawWorkspaceDir — OpenClaw's counterpart to ClaudeMDPath.
func (c Config) OpenClawAgentsMDPath() string {
	return filepath.Join(c.OpenClawWorkspaceDir(), "AGENTS.md")
}

// OpenClawSoulMDPath is the managed SOUL.md file's path under this Config's OpenClawWorkspaceDir.
func (c Config) OpenClawSoulMDPath() string {
	return filepath.Join(c.OpenClawWorkspaceDir(), "SOUL.md")
}

// OpenClawMCPConfigPath is OpenClaw's own MCP server registry file, directly under OpenClawHome
// (confirmed shape: ~/.openclaw/openclaw.json, top-level mcpServers object — design #1666's
// resolved risk) — OpenClaw's counterpart to SettingsPath.
func (c Config) OpenClawMCPConfigPath() string {
	return filepath.Join(c.OpenClawHome, "openclaw.json")
}

// OpenClawPluginDir is where the click-memory-guard OpenClaw plugin (design #1666's "ADDED PIECE:
// OpenClaw memory-guard parity", decisions OCG-1..6) is installed under this Config's OpenClawHome —
// OpenClaw's plugins/ directory, mirroring ClickSDDPluginDir's shape under ClaudeHome's own
// plugins/ directory.
func (c Config) OpenClawPluginDir() string {
	return filepath.Join(c.OpenClawHome, "plugins", "click-memory-guard")
}

// OpenClawSkillsDir is where click-owned OpenClaw skill manifests are synchronized under this
// Config's OpenClawHome — OpenClaw's native skills/ directory. It returns empty
// when OpenClawHome is empty, so SyncOpenClawSkills/RemoveOpenClawSkills can no-op safely.
func (c Config) OpenClawSkillsDir() string {
	if c.OpenClawHome == "" {
		return ""
	}
	return filepath.Join(c.OpenClawHome, "skills")
}

// OpenClawModelProfilePath stores Click's portable model/profile recommendation for OpenClaw.
// It is deliberately outside OpenClaw's native configuration files: OpenClaw model routing is not
// established by this repository, so this artifact is reference data only, not active configuration.
func (c Config) OpenClawModelProfilePath() string {
	if c.OpenClawHome == "" {
		return ""
	}
	return filepath.Join(c.OpenClawHome, "click-ai-devkit", "model-profile.json")
}

// CodexAgentsMDPath is Codex's user-scope global instruction file.
func (c Config) CodexAgentsMDPath() string {
	return filepath.Join(c.CodexHome, "AGENTS.md")
}

// CodexConfigPath is Codex's native user-scope config file.
func (c Config) CodexConfigPath() string {
	return filepath.Join(c.CodexHome, "config.toml")
}
