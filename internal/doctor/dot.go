package doctor

import (
	"fmt"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/installer"
)

// CheckDot reports Click Dot's installation state against the version click expects. Dot is a
// supplementary component (D45), so every state is Healthy: drift or absence is a Warning that
// points at `click update`, never a failed report. It only reads the status it is given.
func CheckDot(status installer.DotStatus, buildVersion string) CheckResult {
	const name = "Click Dot"
	expected := installer.DotReleaseVersion(buildVersion)
	warn := func(detail string) CheckResult {
		return CheckResult{Name: name, Healthy: true, Warning: true, Detail: detail}
	}
	switch {
	case !status.ExePresent && !status.MarkerPresent:
		return warn("no instalado; ejecute `click install` o `click update`")
	case !status.ExePresent:
		return warn(fmt.Sprintf("falta %s; ejecute `click update` para reinstalarlo", status.ExePath))
	case status.Version == "":
		return warn("instalado sin versión registrada por Click; ejecute `click update`")
	case expected == "":
		return CheckResult{Name: name, Healthy: true, Detail: fmt.Sprintf("versión %s instalada (build de desarrollo de click: no hay versión esperada)", status.Version)}
	case status.Version != expected:
		return warn(fmt.Sprintf("versión %s instalada; click espera %s; ejecute `click update`", status.Version, expected))
	}
	return CheckResult{Name: name, Healthy: true, Detail: fmt.Sprintf("versión %s instalada, coincide con click", status.Version)}
}
