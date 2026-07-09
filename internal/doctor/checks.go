// Package doctor owns click's read-only environment/health checks: verifying that the installed
// plugins are actually registered in Claude Code, that the managed CLAUDE.md block exists, and
// that the memory-guard hook is registered (tech-spec.md §2.1 "click doctor"). Checks in this
// package never mutate state — `click doctor` is read-only by design (NFR-012).
package doctor

import (
	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/installer"
)

// EngramChecksCount is the number of doctor checks contributed by Engram (plugin + binary), kept
// as an exported constant so other packages/tests documenting Run()'s total check count don't have
// to hardcode a magic number that silently drifts if a check is added or removed here.
const EngramChecksCount = 2

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
		checkEngramPlugin(cfg),
		checkEngramBinary(cfg),
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
// missing, so this is a separate check from checkEngramPlugin.
func checkEngramBinary(cfg installer.Config) CheckResult {
	const name = "engram binary"

	path, ok, err := installer.EngramBinaryResolvable(cfg)
	if err != nil {
		return CheckResult{Name: name, Healthy: false, Detail: err.Error()}
	}
	if !ok {
		return CheckResult{Name: name, Healthy: false, Detail: "no encontrado en " + path + " (el MCP de engram no podrá conectar)"}
	}
	return CheckResult{Name: name, Healthy: true, Detail: "resuelto en " + path}
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
