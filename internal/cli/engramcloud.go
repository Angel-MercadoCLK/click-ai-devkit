package cli

import (
	"fmt"
	"io"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/installer"
	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/manifest"
	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/ui"
)

// syncEngramCloudFunc is the injectable seam behind runInstall/runUpdate's
// installer.SyncEngramCloud call. It mirrors installer.SetCommandRunnerFactoryForTests and the
// removeEngramPluginFunc pattern in uninstall.go, letting CLI-level tests assert the opt-in/no-config
// behavior without shelling out to a real engram binary or network.
var syncEngramCloudFunc = installer.SyncEngramCloud

// SetSyncEngramCloudFuncForTests overrides syncEngramCloudFunc for tests and returns a restore
// function. Exported so install_test.go and update_test.go can share the same seam.
func SetSyncEngramCloudFuncForTests(fn func(installer.Config, *manifest.Manifest) error) func() {
	old := syncEngramCloudFunc
	syncEngramCloudFunc = fn
	return func() { syncEngramCloudFunc = old }
}

const skippedCloudEnrollmentMessage = "Engram Cloud (servidor y proyecto) detectado, pero falta ENGRAM_CLOUD_TOKEN. Se omite la inscripción en la nube; la operación continúa en modo local."

// reportSkippedCloudEnrollment prints the Spanish informational message used when cloud server and
// project are configured but ENGRAM_CLOUD_TOKEN is absent. It makes no subprocess calls and writes no
// state.
func reportSkippedCloudEnrollment(out io.Writer, r *ui.Renderer) {
	fmt.Fprintln(out, r.Info(skippedCloudEnrollmentMessage))
}
