// Package doctor owns click's read-only environment/health checks: verifying that the installed
// plugins are actually registered in Claude Code, that the managed CLAUDE.md block exists, that
// the memory-guard hook is registered, and that Context7 is registered as a user-scope MCP server
// (tech-spec.md §2.1 "click doctor"). Checks in this package never mutate state — `click doctor`
// is read-only by design (NFR-012).
package doctor

import (
	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/installer"
	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/manifest"
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
		checkPlugin(cfg),
		checkMemoryPlugin(cfg),
		checkReviewPlugin(cfg),
		checkClaudeMD(cfg),
		checkMemoryGuardHook(cfg),
		checkModelsConfig(cfg),
		checkEngramPlugin(cfg),
		checkEngramBinary(cfg),
		checkContext7(cfg),
	}}
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
