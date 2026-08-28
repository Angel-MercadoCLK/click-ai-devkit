package cli

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/installer"
	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/manifest"
	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/ui"
)

const skippedCloudEnrollmentMessage = "Engram Cloud (servidor y proyecto) detectado, pero falta ENGRAM_CLOUD_TOKEN. Se omite la inscripción en la nube; la operación continúa en modo local."

const consentPrompt = "¿Guardar ENGRAM_CLOUD_TOKEN en ~/.claude/settings.json con permisos 0600 para activar Engram Cloud en Claude Code? [y/N]: "

const consentSkippedWarning = "Se omitió la persistencia de ENGRAM_CLOUD_TOKEN debido a un error de lectura. El token no se ha guardado en settings.json."

const autosyncDisabledWarning = "Engram Cloud queda con autosync desactivado: ENGRAM_CLOUD_TOKEN no se persistió en settings.json. Exporta el token por otro mecanismo y vuelve a ejecutar el comando, o acepta persistirlo, para activar la sincronización."

var readCloudTokenConsentFunc = func(r *bufio.Reader) (bool, error) {
	line, err := r.ReadString('\n')
	if err != nil {
		return false, err
	}
	response := strings.TrimSpace(strings.ToLower(line))
	return response == "y" || response == "yes", nil
}

func SetReadCloudTokenConsentFuncForTests(fn func(*bufio.Reader) (bool, error)) func() {
	old := readCloudTokenConsentFunc
	readCloudTokenConsentFunc = fn
	return func() { readCloudTokenConsentFunc = old }
}

func resolveCloudTokenPersistence(isNonInteractive bool, persistFlag bool, reader *bufio.Reader, out io.Writer) (installer.CloudTokenPersistence, error) {
	if persistFlag {
		return installer.CloudTokenPersistencePersist, nil
	}
	if isNonInteractive {
		return installer.CloudTokenPersistenceDecline, nil
	}
	if reader == nil {
		return installer.CloudTokenPersistenceDecline, nil
	}
	emitConsentPrompt(out)
	affirmative, err := readCloudTokenConsentFunc(reader)
	if err != nil {
		return installer.CloudTokenPersistenceDecline, err
	}
	if affirmative {
		return installer.CloudTokenPersistencePersist, nil
	}
	return installer.CloudTokenPersistenceDecline, nil
}

func emitConsentPrompt(out io.Writer) {
	fmt.Fprint(out, consentPrompt)
}

func reportSkippedCloudEnrollment(out io.Writer, r *ui.Renderer) {
	fmt.Fprintln(out, r.Info(skippedCloudEnrollmentMessage))
}

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
