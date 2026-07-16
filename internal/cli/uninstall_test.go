package cli

import (
	"errors"
	"strings"
	"testing"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/installer"
)

// TestUninstallCommand_SurfacesEngramPathWarning_WarningOnlyNoError is the pre-existing-shaped half
// of the T4-1 follow-up regression coverage: a warning-only RemoveEngramPlugin result (err == nil)
// must surface via surfacePathWarning and `click uninstall` must still succeed. This mirrors the
// success path runUninstall already exercised before the fix — kept here as an explicit CLI-level
// case alongside the double-failure test below, so both branches of the same `if err != nil` split
// are covered from the same seam.
func TestUninstallCommand_SurfacesEngramPathWarning_WarningOnlyNoError(t *testing.T) {
	home := t.TempDir()
	runner := newTestCommandRunner(home)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()

	wantWarning := "no se pudo quitar C:\\fake\\gobin del PATH persistente: acceso denegado"
	restoreRemove := SetRemoveEngramPluginFuncForTests(func(cfg installer.Config) (string, error) {
		return wantWarning, nil
	})
	defer restoreRemove()

	out, err := execRoot(t, home, "uninstall")
	if err != nil {
		t.Fatalf("uninstall command error = %v, want nil when RemoveEngramPlugin only returns a pathWarning", err)
	}
	if !strings.Contains(out, wantWarning) {
		t.Fatalf("uninstall output = %q, want it to contain the PATH warning %q", out, wantWarning)
	}
}

// TestUninstallCommand_SurfacesEngramPathWarning_EvenOnFatalError is the actual regression this
// fix closes: RemoveEngramPlugin returning BOTH a non-empty pathWarning AND a fatal error in the
// same call (e.g. one PATH entry failed to be removed AND a later, unrelated step such as
// uninstalling the plugin itself also failed) used to silently drop the pathWarning — runUninstall
// captured it into engramPathWarning but returned early on `err != nil`, before ever reaching
// surfacePathWarning. The fatal error itself still surfaced (via cobra), but the PATH-specific
// detail vanished. The fix surfaces engramPathWarning on BOTH the error path and the success path.
func TestUninstallCommand_SurfacesEngramPathWarning_EvenOnFatalError(t *testing.T) {
	home := t.TempDir()
	runner := newTestCommandRunner(home)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()

	wantWarning := "no se pudo quitar C:\\fake\\gobin del PATH persistente: acceso denegado"
	wantErr := errors.New("no se pudo desinstalar el plugin engram@engram")
	restoreRemove := SetRemoveEngramPluginFuncForTests(func(cfg installer.Config) (string, error) {
		return wantWarning, wantErr
	})
	defer restoreRemove()

	out, err := execRoot(t, home, "uninstall")
	if err == nil {
		t.Fatalf("uninstall command error = nil, want a non-nil error when RemoveEngramPlugin also fails, output:\n%s", out)
	}
	if !strings.Contains(out, wantWarning) {
		t.Fatalf("uninstall output = %q, want it to contain the PATH warning %q even though RemoveEngramPlugin also returned a fatal error", out, wantWarning)
	}
}
