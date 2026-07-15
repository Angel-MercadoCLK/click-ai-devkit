// Package doctor owns click's read-only environment/health checks: verifying that the installed
// plugins are actually registered in Claude Code, that the managed CLAUDE.md block exists, that
// the memory-guard hook is registered, and that Context7 is registered as a user-scope MCP server
// (tech-spec.md §2.1 "click doctor"). Checks in this package never mutate state — `click doctor`
// is read-only by design (NFR-012).
package doctor

import (
	"sort"
	"strconv"
	"strings"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/installer"
	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/manifest"
	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/modelconfig"
)

// EngramChecksCount is the number of doctor checks contributed by Engram (plugin + binary), kept
// as an exported constant so other packages/tests documenting Run()'s total check count don't have
// to hardcode a magic number that silently drifts if a check is added or removed here.
const EngramChecksCount = 2

// Context7ChecksCount is the number of doctor checks contributed by Context7, kept as an exported
// constant for the same reason as EngramChecksCount.
const Context7ChecksCount = 1

// CheckResult is the outcome of a single doctor check.
type CheckResult struct {
	Name    string
	Healthy bool
	Detail  string
}

// Report is the full set of check results from one Run.
type Report struct {
	Checks []CheckResult
}

// Healthy reports whether every check in the report passed.
func (r Report) Healthy() bool {
	for _, c := range r.Checks {
		if !c.Healthy {
			return false
		}
	}
	return true
}

// Run executes every current health check against cfg.ClaudeHome. It never mutates the
// filesystem.
func Run(cfg installer.Config) Report {
	return Report{Checks: []CheckResult{
		checkGit(cfg),
		checkPlugin(cfg),
		checkMemoryPlugin(cfg),
		checkReviewPlugin(cfg),
		checkSkillsPlugin(cfg),
		checkClaudeMD(cfg),
		checkMemoryGuardHook(cfg),
		checkModelsConfig(cfg),
		checkAppliedPluginConfig(cfg),
		checkEngramPlugin(cfg),
		checkEngramBinary(cfg),
		checkContext7(cfg),
	}}
}

// checkGit reports whether git is resolvable on PATH. It is foundational and read first (NFR-012:
// read-only — it only resolves PATH, never installs anything): click's plugin marketplace
// registration (`claude plugin marketplace add <source>`, run by both `click install` and `click
// update`) shells out to `git clone` under the hood, so a missing git breaks install/update deep
// inside plugin registration with a cryptic error — reproduced live on a fresh Windows VM with no
// git installed. When missing, Detail carries the exact same actionable message
// installer.GitMissingMessage that `click install`/`click update`'s own PreflightGit uses, so
// doctor and install/update never give a developer conflicting instructions (mirroring
// checkEngramBinary's shared-message contract with EngramBinaryRemediationMessage).
func checkGit(cfg installer.Config) CheckResult {
	const name = "git"

	path, ok := installer.GitPath()
	if ok {
		return CheckResult{Name: name, Healthy: true, Detail: "resuelto en " + path}
	}
	return CheckResult{Name: name, Healthy: false, Detail: installer.GitMissingMessage}
}

func checkPlugin(cfg installer.Config) CheckResult {
	const name = "plugin click-sdd"

	ok, err := installer.HasInstalledPlugin(cfg, "click-sdd")
	if err != nil {
		return CheckResult{Name: name, Healthy: false, Detail: err.Error()}
	}
	if !ok {
		return CheckResult{Name: name, Healthy: false, Detail: "no registrado en Claude Code"}
	}
	return CheckResult{Name: name, Healthy: true, Detail: "registrado y habilitado"}
}

func checkMemoryPlugin(cfg installer.Config) CheckResult {
	const name = "plugin click-memory"

	ok, err := installer.HasInstalledPlugin(cfg, "click-memory")
	if err != nil {
		return CheckResult{Name: name, Healthy: false, Detail: err.Error()}
	}
	if !ok {
		return CheckResult{Name: name, Healthy: false, Detail: "no registrado en Claude Code"}
	}
	return CheckResult{Name: name, Healthy: true, Detail: "registrado y habilitado"}
}

func checkReviewPlugin(cfg installer.Config) CheckResult {
	const name = "plugin click-review"

	ok, err := installer.HasInstalledPlugin(cfg, "click-review")
	if err != nil {
		return CheckResult{Name: name, Healthy: false, Detail: err.Error()}
	}
	if !ok {
		return CheckResult{Name: name, Healthy: false, Detail: "no registrado en Claude Code"}
	}
	return CheckResult{Name: name, Healthy: true, Detail: "registrado y habilitado"}
}

func checkSkillsPlugin(cfg installer.Config) CheckResult {
	const name = "plugin click-skills"

	ok, err := installer.HasInstalledPlugin(cfg, "click-skills")
	if err != nil {
		return CheckResult{Name: name, Healthy: false, Detail: err.Error()}
	}
	if !ok {
		return CheckResult{Name: name, Healthy: false, Detail: "no registrado en Claude Code"}
	}
	return CheckResult{Name: name, Healthy: true, Detail: "registrado y habilitado"}
}

func checkClaudeMD(cfg installer.Config) CheckResult {
	const name = "CLAUDE.md managed block"

	ok, err := installer.HasManagedBlock(cfg.ClaudeMDPath())
	if err != nil {
		return CheckResult{Name: name, Healthy: false, Detail: err.Error()}
	}
	if !ok {
		return CheckResult{Name: name, Healthy: false, Detail: "bloque gestionado ausente en " + cfg.ClaudeMDPath()}
	}
	return CheckResult{Name: name, Healthy: true, Detail: "bloque gestionado presente"}
}

// checkEngramPlugin reports whether the engram@engram plugin is registered and enabled — the
// mechanism confirmed in Step 0 (spike-e-engram-install.md) that actually wires Engram's tools
// into a Claude Code session. It does not care whether click or the developer installed it.
func checkEngramPlugin(cfg installer.Config) CheckResult {
	const name = "plugin engram"

	ok, err := installer.HasInstalledPluginID(cfg, installer.EngramPluginID)
	if err != nil {
		return CheckResult{Name: name, Healthy: false, Detail: err.Error()}
	}
	if !ok {
		return CheckResult{Name: name, Healthy: false, Detail: "no registrado en Claude Code"}
	}
	return CheckResult{Name: name, Healthy: true, Detail: "registrado y habilitado"}
}

// checkEngramBinary reports whether the Engram binary the plugin's bundled MCP server needs
// (bare, PATH-resolved `command: "engram"` — confirmed in Step 0) actually resolves to a file on
// disk. The plugin can be registered and enabled yet still fail to connect if this binary is
// missing, so this is a separate check from checkEngramPlugin. This check is read-only (NFR-012:
// `click doctor` never mutates state) — it never attempts to provision the binary itself, unlike
// `click install`'s EnsureEngramBinary. When missing, the Detail includes the exact same
// remediation `go install` command that `click install`'s own non-fatal provisioning fallback shows
// (installer.EngramBinaryRemediationMessage), so doctor and install never give a developer
// conflicting instructions.
func checkEngramBinary(cfg installer.Config) CheckResult {
	const name = "engram binary"

	path, ok, err := installer.EngramBinaryResolvable(cfg)
	if err != nil {
		return CheckResult{Name: name, Healthy: false, Detail: err.Error()}
	}
	if ok {
		return CheckResult{Name: name, Healthy: true, Detail: "resuelto en " + path}
	}

	version := ""
	if m, mErr := manifest.Load(); mErr == nil {
		version = m.Engram.Version
	}
	return CheckResult{
		Name:    name,
		Healthy: false,
		Detail:  "no encontrado en " + path + " (el MCP de engram no podrá conectar). " + installer.EngramBinaryRemediationMessage(version),
	}
}

// checkContext7 reports whether Context7 is registered as a user-scope MCP server, read directly
// from Claude Code's own user config file (installer.HasContext7) — matching checkEngramPlugin's
// pure-file-read approach so `click doctor` never shells out to the real `claude` CLI (NFR-012:
// read-only by design).
func checkContext7(cfg installer.Config) CheckResult {
	const name = "context7 MCP"

	ok, err := installer.HasContext7(cfg)
	if err != nil {
		return CheckResult{Name: name, Healthy: false, Detail: err.Error()}
	}
	if !ok {
		return CheckResult{Name: name, Healthy: false, Detail: "no registrado como servidor MCP de usuario"}
	}
	return CheckResult{Name: name, Healthy: true, Detail: "registrado (scope user, https://mcp.context7.com/mcp)"}
}

// checkModelsConfig reports whether cfg.ModelsPath() holds a stale (pre-taxonomy-realignment or
// otherwise outdated schema_version) models.json. It only READS the file via installer.IsStale —
// never installer.MigrateIfStale — so `click doctor` stays read-only (NFR-012). An absent file is
// healthy: it just means defaults will be generated on the next `click install`/`click update`.
func checkModelsConfig(cfg installer.Config) CheckResult {
	const name = "models.json schema"

	stale, err := installer.IsStale(cfg)
	if err != nil {
		return CheckResult{Name: name, Healthy: false, Detail: err.Error()}
	}
	if stale {
		return CheckResult{
			Name:    name,
			Healthy: false,
			Detail:  "models.json usa una taxonomía o schema desactualizado — se regenerará (con backup en models.json.bak) en el próximo `click update`",
		}
	}
	return CheckResult{Name: name, Healthy: true, Detail: "actualizado (o aún no generado)"}
}

// checkAppliedPluginConfig reports whether every click-sdd `--config` key click computed it SHOULD
// configure (expectedClickSDDConfigKeys — modelconfig.ProfileConfigKey plus one ConfigKey() per
// modelconfig.Phases entry) is ACTUALLY present in Claude Code's own settings.json, under
// pluginConfigs[installer.ClickSDDPluginID].options — i.e. what Claude Code itself accepted and
// applied, not merely what click intended to send.
//
// This closes a real blind spot checkModelsConfig cannot see: checkModelsConfig only validates
// click's own models.json (what SHOULD be configured, computed from modelconfig). During a live
// incident, click-sdd's plugin.json grew from 13 to 18 model-config phases (adding the 5
// review_*_model userConfig keys), but because the plugin version never bumped, Claude Code cached
// a stale schema and silently DROPPED those 5 --config keys during `claude plugin install` — so
// the developer's real settings.json ended up with only 14 of the 19 expected keys, while `click
// doctor` kept reporting healthy the whole time, because checkModelsConfig never looks at the
// applied side at all. checkAppliedPluginConfig is a SEPARATE check from checkModelsConfig on
// purpose: the two answer different questions (should-be-configured vs. actually-applied) and
// either can go unhealthy independently of the other.
//
// It is intentionally read-only (NFR-012): it only reads settings.json via
// installer.AppliedClickSDDPluginConfig, never writes it.
func checkAppliedPluginConfig(cfg installer.Config) CheckResult {
	const name = "click-sdd applied plugin config"

	expected := expectedClickSDDConfigKeys()

	applied, found, err := installer.AppliedClickSDDPluginConfig(cfg)
	if err != nil {
		return CheckResult{Name: name, Healthy: false, Detail: err.Error()}
	}
	if !found {
		return CheckResult{
			Name:    name,
			Healthy: false,
			Detail:  "pluginConfigs[\"" + installer.ClickSDDPluginID + "\"] ausente en " + cfg.SettingsPath() + " — ejecuta `click update` para aplicarla",
		}
	}

	var missing []string
	for _, key := range expected {
		if _, ok := applied[key]; !ok {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return CheckResult{
			Name:    name,
			Healthy: false,
			Detail: "Claude Code no aplicó " + strconv.Itoa(len(missing)) + " de " + strconv.Itoa(len(expected)) +
				" claves esperadas (posible schema en caché desactualizado): " + strings.Join(missing, ", ") +
				" — ejecuta `click update` para forzar un refresh del marketplace y reaplicarlas",
		}
	}

	return CheckResult{
		Name:    name,
		Healthy: true,
		Detail:  strconv.Itoa(len(expected)) + "/" + strconv.Itoa(len(expected)) + " claves aplicadas correctamente en " + cfg.SettingsPath(),
	}
}

// expectedClickSDDConfigKeys returns every plugin.json userConfig key click-sdd's --config flags
// SHOULD apply: modelconfig.ProfileConfigKey plus one Phase.ConfigKey() per modelconfig.Phases
// entry. It deliberately derives this from modelconfig — never a hardcoded literal count or key
// list — so checkAppliedPluginConfig auto-tracks any future taxonomy change (a phase added or
// removed) without a second manual edit here.
func expectedClickSDDConfigKeys() []string {
	keys := make([]string, 0, len(modelconfig.Phases)+1)
	keys = append(keys, modelconfig.ProfileConfigKey)
	for _, phase := range modelconfig.Phases {
		keys = append(keys, phase.ConfigKey())
	}
	return keys
}

func checkMemoryGuardHook(cfg installer.Config) CheckResult {
	const name = "memory-guard hook"

	ok, err := installer.HasMemoryGuardHook(cfg)
	if err != nil {
		return CheckResult{Name: name, Healthy: false, Detail: err.Error()}
	}
	if !ok {
		return CheckResult{Name: name, Healthy: false, Detail: "hook PreToolUse ausente en " + cfg.SettingsPath()}
	}
	return CheckResult{Name: name, Healthy: true, Detail: "hook PreToolUse registrado"}
}
