package cli

import (
	"fmt"
	"io"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/installer"
	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/ui"
	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/version"
	"github.com/spf13/cobra"
)

// Click Dot is a supplementary integration (D45): it is installed after every existing step, a
// failure renders a Spanish warning (never the red FAIL step) and install/update still succeed, and
// rollback deliberately leaves it alone. See installer/dot.go for the download and ownership rules.

// dotVerb selects the wording for the install and update commands.
type dotVerb struct {
	running   string // "Instalando" / "Sincronizando"
	failure   string // "instalar" / "actualizar"
	operation string // "La instalación" / "La actualización"
}

var (
	dotInstallVerb = dotVerb{running: "Instalando", failure: "instalar", operation: "La instalación"}
	dotUpdateVerb  = dotVerb{running: "Sincronizando", failure: "actualizar", operation: "La actualización"}
)

// dotSkipNotice returns the Spanish reason Dot is skipped for plan, or "" when it would be installed.
func dotSkipNotice(plan installer.DotPlan) string {
	switch plan.Outcome {
	case installer.DotSkippedPlatform:
		return "Click Dot solo está disponible en Windows; se omite."
	case installer.DotSkippedDev:
		return "Click Dot se omite en builds de desarrollo (no hay una release publicada de la que descargarlo)."
	}
	return ""
}

// syncDotStep runs after the existing install/update steps. It never returns an error.
func syncDotStep(cmd *cobra.Command, out io.Writer, r *ui.Renderer, verb dotVerb) {
	plan := installer.PlanDot(version.Version)
	if notice := dotSkipNotice(plan); notice != "" {
		fmt.Fprintln(out, r.Info(notice))
		return
	}
	fmt.Fprintln(out, r.Step(fmt.Sprintf("%s Click Dot %s…", verb.running, plan.Version)))
	result, err := installer.SyncDot(cmd.Context(), version.Version)
	if err != nil {
		fmt.Fprintln(out, r.Warn(fmt.Sprintf("No se pudo %s Click Dot: %v. %s de Click continúa; reintenta más tarde con `click update`.", verb.failure, err, verb.operation)))
		return
	}
	switch result.Outcome {
	case installer.DotCurrent:
		fmt.Fprintln(out, r.Info(fmt.Sprintf("Click Dot %s ya está actualizado.", result.Version)))
	case installer.DotUpdated:
		fmt.Fprintln(out, r.Success(fmt.Sprintf("Click Dot %s actualizado", result.Version)))
	default:
		fmt.Fprintln(out, r.Success(fmt.Sprintf("Click Dot %s instalado", result.Version)))
	}
	for _, warning := range result.Warnings {
		fmt.Fprintln(out, r.Warn("Click Dot: "+warning+"."))
	}
}

// dotPreview renders what the install/update plan would do for Dot: the numbered step to append, or
// the skip notice to print. Pure: no network and no filesystem access.
func dotPreview() (step, notice string) {
	plan := installer.PlanDot(version.Version)
	if notice := dotSkipNotice(plan); notice != "" {
		return "", notice
	}
	return fmt.Sprintf("Instalando o actualizando Click Dot %s en %s (descarga verificada con SHA-256 desde la release v%s; acceso directo en el menú Inicio)…", plan.Version, plan.InstallDir, plan.Version), ""
}

// dotRollbackNotice tells Windows users that rollback does not manage Dot.
func dotRollbackNotice(out io.Writer, r *ui.Renderer) {
	if !installer.DotSupported() {
		return
	}
	fmt.Fprintln(out, r.Info("Click Dot no se modifica con `click rollback`: la aplicación se deja instalada tal como está. Usa `click uninstall` para quitarla."))
}
