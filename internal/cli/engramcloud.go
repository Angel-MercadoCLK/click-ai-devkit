package cli

import (
	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/installer"
	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/manifest"
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
