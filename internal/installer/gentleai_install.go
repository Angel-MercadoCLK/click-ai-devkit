package installer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/manifest"
)

// GentleAIInstallArgs limits upstream writes to the runtimes selected for this run.
// Engram and Context7 stay Click-owned until their registrations transfer together.
func GentleAIInstallArgs(selection TargetSelection) []string {
	var agents []string
	if selection.Claude {
		agents = append(agents, "claude-code")
	}
	if selection.Codex {
		agents = append(agents, "codex")
	}
	if selection.OpenClaw {
		agents = append(agents, "openclaw")
	}
	if len(agents) == 0 {
		return nil
	}
	return []string{"install", "--agents", strings.Join(agents, ","), "--preset", "full-gentleman", "--components", "skills,permissions,persona", "--persona", "neutral", "--scope", "global"}
}

// GentleAIInstallNotice separates actionable warnings from informational pin decisions.
type GentleAIInstallNotice struct {
	Message string
	Warning bool
}

// Install skips provisioning only for a verifiable managed pin, then reapplies the
// selected upstream components. Report runs before replacement, including downgrade
// warnings. All subprocesses use the provisioner's injected runner and environment.
func (p GentleAIProvisioner) Install(ctx context.Context, pin manifest.GentleAI, selection TargetSelection, report func(GentleAIInstallNotice)) (GentleAIProvisionResult, error) {
	var result GentleAIProvisionResult
	args := GentleAIInstallArgs(selection)
	if len(args) == 0 {
		return result, nil
	}
	if report == nil {
		report = func(GentleAIInstallNotice) {}
	}
	path, err := p.ManagedPath()
	if err != nil {
		return result, err
	}
	matches := false
	info, err := p.Lstat(path)
	switch {
	case os.IsNotExist(err):
		report(GentleAIInstallNotice{Message: fmt.Sprintf("gentle-ai no está instalado en la ruta de Click; se instalará la versión fijada %s.", pin.Version)})
	case err != nil:
		return result, fmt.Errorf("No se pudo inspeccionar gentle-ai. Revise los permisos de ~/.click/bin y vuelva a ejecutar `click install` o `click update`.")
	case !info.Mode().IsRegular() || (p.GOOS != "windows" && info.Mode().Perm()&0o111 == 0):
		report(GentleAIInstallNotice{Message: fmt.Sprintf("gentle-ai: no se pudo verificar el binario gestionado; se reinstalará la versión fijada %s.", pin.Version), Warning: true})
	default:
		probe, probeErr := p.ProbeVersion(ctx, path, pin.Version)
		if probeErr != nil {
			report(GentleAIInstallNotice{Message: fmt.Sprintf("gentle-ai: no se pudo verificar la versión; se reinstalará la versión fijada %s.", pin.Version), Warning: true})
		} else if probe.MatchesPin {
			matches = true
			report(GentleAIInstallNotice{Message: fmt.Sprintf("gentle-ai %s coincide con la versión fijada; se omite la reinstalación del binario.", pin.Version)})
		} else {
			report(GentleAIInstallNotice{Message: fmt.Sprintf("gentle-ai difiere de la versión fijada (%s → %s); se reinstalará el pin. Esto puede degradar una versión más reciente elegida por el desarrollador.", probe.Version, pin.Version), Warning: true})
		}
	}
	result.Path, result.PathDir = path, filepath.Dir(path)
	if !matches {
		result, err = p.Provision(ctx, pin)
		if err != nil {
			// Raw subprocess errors/output may contain credentials; never echo them here.
			return result, fmt.Errorf("No se pudo instalar la versión fijada de gentle-ai (%s). Revise la conexión, los permisos de ~/.click/bin y, en Windows, Go >= 1.25.10; vuelva a ejecutar `click install` o `click update`.", pin.Version)
		}
	} else {
		result = p.reconcilePath(result)
	}
	if _, err := p.Run(ctx, GentleAIEnvironment(p.Environ()), result.Path, args...); err != nil {
		return result, fmt.Errorf("No se pudo configurar gentle-ai para los runtimes seleccionados. Revise los permisos de sus directorios de configuración y vuelva a ejecutar `click install` o `click update`.")
	}
	return result, nil
}

var gentleAIProvisionerFactory = NewGentleAIProvisioner

// SetGentleAIProvisionerFactoryForTests replaces all machine/execution boundaries.
func SetGentleAIProvisionerFactoryForTests(factory func() GentleAIProvisioner) func() {
	old := gentleAIProvisionerFactory
	gentleAIProvisionerFactory = factory
	return func() { gentleAIProvisionerFactory = old }
}

// SyncGentleAI composes the qualified binary and selected upstream runtime components.
func SyncGentleAI(ctx context.Context, pin manifest.GentleAI, selection TargetSelection, report func(GentleAIInstallNotice)) (GentleAIProvisionResult, error) {
	return gentleAIProvisionerFactory().Install(ctx, pin, selection, report)
}
