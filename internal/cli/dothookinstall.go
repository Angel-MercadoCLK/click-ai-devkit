package cli

import (
	"fmt"
	"io"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/installer"
	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/ui"
	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/version"
)

// Click Dot agent-companion hooks (S6). The hook entries only make sense once Click Dot is on the
// machine, so the register action is declared in the plan (preview, snapshot, ordering) but executed
// right after the Click Dot sync step. Like Click Dot itself it is a supplementary integration (D45):
// a failure is a Spanish warning and never fails install/update.

// dotHooksPlanOption reports whether this install/update run should plan the Click Dot hooks.
func dotHooksPlanOption() bool {
	return installer.DotHooksApplicable(version.Version)
}

func planHasAction(kinds []installer.StepActionKind, want installer.StepActionKind) bool {
	for _, kind := range kinds {
		if kind == want {
			return true
		}
	}
	return false
}

const dotHooksPreviewStep = "Registrando hooks de Click Dot en settings.json (tras instalar Click Dot; Claude Code nunca se bloquea: si Click Dot no responde, Claude pregunta en la terminal)…"

// withDotHooksPreview appends the hook step to the preview list when the plan carries it.
func withDotHooksPreview(steps []string, plan installer.TargetPlan, kinds []installer.StepActionKind) []string {
	if !planHasAction(kinds, installer.StepActionRegisterDotHooks) {
		return steps
	}
	return append(append([]string(nil), steps...), dotHooksPreviewStep)
}

// registerDotHooksStep runs after syncDotStep. It never returns an error.
func registerDotHooksStep(out io.Writer, r *ui.Renderer, cfg installer.Config, verb dotVerb) {
	if !installer.InspectDot().ExePresent {
		fmt.Fprintln(out, r.Warn("No se registraron los hooks de Click Dot porque Click Dot no está instalado; se registrarán en la próxima instalación o actualización que lo instale."))
		return
	}
	fmt.Fprintln(out, r.Step("Registrando hooks de Click Dot…"))
	if err := installer.RegisterDotHooks(cfg); err != nil {
		fmt.Fprintln(out, r.Warn(fmt.Sprintf("No se pudieron registrar los hooks de Click Dot: %v. %s de Click continúa; reintenta más tarde con `click update`.", err, verb.operation)))
		return
	}
	fmt.Fprintln(out, r.Success("Hooks de Click Dot registrados; Claude Code nunca se bloquea: si Click Dot está cerrado o no respondes, Claude pregunta en la terminal"))
}
