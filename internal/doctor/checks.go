// Package doctor owns click's read-only environment/health checks: verifying that the installed
// plugins are actually registered in Claude Code, that the managed CLAUDE.md block exists, and
// that the memory-guard hook is registered (tech-spec.md §2.1 "click doctor"). Checks in this
// package never mutate state — `click doctor` is read-only by design (NFR-012).
package doctor

import (
	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/installer"
)

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
