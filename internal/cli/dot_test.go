package cli

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/installer"
	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/version"
)

const cliDotVersion = "1.2.3"

// cliDotRig is a fake Windows machine plus fake GitHub release for the Dot CLI tests. Nothing here
// reads or writes real user state: LOCALAPPDATA/APPDATA are resolved from t.TempDir() through the
// manager's Getenv seam and every process/network boundary is a recorder.
type cliDotRig struct {
	t       *testing.T
	local   string
	roaming string
	files   map[string][]byte
	fetched []string
	runs    []string
	started []string
	fetchOK bool
}

func newCLIDotRig(t *testing.T, goos string) *cliDotRig {
	t.Helper()
	root := t.TempDir()
	rig := &cliDotRig{t: t, local: filepath.Join(root, "Local"), roaming: filepath.Join(root, "Roaming"), files: map[string][]byte{}, fetchOK: true}
	env := map[string]string{"LOCALAPPDATA": rig.local, "APPDATA": rig.roaming, "SystemRoot": filepath.Join(root, "Windows")}
	manager := installer.DotManager{
		GOOS: goos, GOARCH: "amd64",
		Getenv:  func(k string) string { return env[k] },
		BaseURL: "https://example.test/dl",
		Fetch: func(_ context.Context, url string) (io.ReadCloser, error) {
			rig.fetched = append(rig.fetched, url)
			if !rig.fetchOK {
				return nil, errors.New("sin conexión")
			}
			body, ok := rig.files[url]
			if !ok {
				return nil, errors.New("HTTP 404")
			}
			return io.NopCloser(bytes.NewReader(body)), nil
		},
		Run: func(_ context.Context, name string, args ...string) ([]byte, error) {
			rig.runs = append(rig.runs, filepath.Base(name))
			return nil, nil
		},
		Start: func(exe string) error { rig.started = append(rig.started, exe); return nil },
		Sleep: func(time.Duration) {},
	}
	t.Cleanup(installer.SetDotManagerFactoryForTests(func() installer.DotManager { return manager }))
	return rig
}

func (r *cliDotRig) publish(ver string) {
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	fw, _ := w.Create("Click Dot.exe")
	_, _ = fw.Write([]byte("exe-" + ver))
	_ = w.Close()
	sum := sha256.Sum256(buf.Bytes())
	asset := installer.DotAssetName(ver)
	base := "https://example.test/dl/v" + ver + "/"
	r.files[base+asset] = buf.Bytes()
	r.files[base+"checksums.txt"] = []byte(hex.EncodeToString(sum[:]) + "  " + asset + "\n")
}

func (r *cliDotRig) installDir() string { return filepath.Join(r.local, "Programs", "Click Dot") }

func setCLIVersion(t *testing.T, v string) {
	t.Helper()
	old := version.Version
	version.Version = v
	t.Cleanup(func() { version.Version = old })
}

func dotCLIHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	runner := newTestCommandRunner(home)
	t.Cleanup(installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner }))
	seedResolvableEngram(t)
	seedResolvableClickBinary(t)
	return home
}

func TestInstallInstallsDotOnWindowsAfterTheExistingSteps(t *testing.T) {
	setCLIVersion(t, cliDotVersion)
	rig := newCLIDotRig(t, "windows")
	rig.publish(cliDotVersion)
	home := dotCLIHome(t)

	out, err := execRoot(t, home, "install")
	if err != nil {
		t.Fatalf("install error = %v\n%s", err, out)
	}
	if !strings.Contains(out, "[..] Instalando Click Dot "+cliDotVersion) || !strings.Contains(out, "[OK] Click Dot "+cliDotVersion+" instalado") {
		t.Fatalf("output lacks the Dot step:\n%s", out)
	}
	if strings.Index(out, "Click Dot") < strings.Index(out, "CLAUDE.md actualizado") || strings.Index(out, "[OK] Click Dot") > strings.Index(out, "Instalación completa.") {
		t.Fatalf("Dot must run after the existing steps and before the final line:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(rig.installDir(), "Click Dot.exe")); err != nil {
		t.Fatalf("Dot not installed: %v", err)
	}
	if len(rig.started) != 1 {
		t.Fatalf("Dot must be started once, got %v", rig.started)
	}
}

func TestInstallDotFailureWarnsAndDoesNotFailInstall(t *testing.T) {
	setCLIVersion(t, cliDotVersion)
	rig := newCLIDotRig(t, "windows")
	rig.fetchOK = false
	home := dotCLIHome(t)

	out, err := execRoot(t, home, "install")
	if err != nil {
		t.Fatalf("a Dot failure must not fail install: %v\n%s", err, out)
	}
	if !strings.Contains(out, "[warn] No se pudo instalar Click Dot") || !strings.Contains(out, "La instalación de Click continúa") {
		t.Fatalf("missing Spanish warning:\n%s", out)
	}
	if strings.Contains(out, "[FAIL]") {
		t.Fatalf("Dot failure must not render a FAIL step:\n%s", out)
	}
	if !strings.Contains(out, "Instalación completa.") {
		t.Fatalf("install must still complete:\n%s", out)
	}
}

func TestInstallSkipsDotOffWindowsWithNotice(t *testing.T) {
	setCLIVersion(t, cliDotVersion)
	rig := newCLIDotRig(t, "linux")
	rig.publish(cliDotVersion)
	home := dotCLIHome(t)

	out, err := execRoot(t, home, "install")
	if err != nil {
		t.Fatalf("install error = %v\n%s", err, out)
	}
	if !strings.Contains(out, "[i] Click Dot solo está disponible en Windows; se omite.") {
		t.Fatalf("missing platform notice:\n%s", out)
	}
	if len(rig.fetched) != 0 || len(rig.runs) != 0 {
		t.Fatal("non-Windows must not download or run anything")
	}
}

func TestInstallSkipsDotForDevelopmentBuild(t *testing.T) {
	setCLIVersion(t, "dev")
	rig := newCLIDotRig(t, "windows")
	home := dotCLIHome(t)

	out, err := execRoot(t, home, "install")
	if err != nil {
		t.Fatalf("install error = %v\n%s", err, out)
	}
	if !strings.Contains(out, "[i] Click Dot se omite en builds de desarrollo") {
		t.Fatalf("missing development notice:\n%s", out)
	}
	if len(rig.fetched) != 0 {
		t.Fatal("a development build must not download Dot")
	}
}

func TestInstallDryRunPrintsDotPlanWithoutNetworkOrWrites(t *testing.T) {
	setCLIVersion(t, cliDotVersion)
	rig := newCLIDotRig(t, "windows")
	home := dotCLIHome(t)

	out, err := execRoot(t, home, "install", "--dry-run")
	if err != nil {
		t.Fatalf("dry-run error = %v\n%s", err, out)
	}
	if !strings.Contains(out, "Instalando o actualizando Click Dot "+cliDotVersion) || !strings.Contains(out, "SHA-256") {
		t.Fatalf("dry-run lacks the Dot step:\n%s", out)
	}
	if len(rig.fetched) != 0 || len(rig.runs) != 0 || len(rig.started) != 0 {
		t.Fatal("dry-run must not touch network, commands or processes")
	}
	if _, err := os.Stat(rig.local); !os.IsNotExist(err) {
		t.Fatal("dry-run must not write the filesystem")
	}
}

func TestInstallDryRunOffWindowsPrintsSkipNotice(t *testing.T) {
	setCLIVersion(t, cliDotVersion)
	newCLIDotRig(t, "linux")
	home := dotCLIHome(t)
	out, err := execRoot(t, home, "install", "--dry-run")
	if err != nil {
		t.Fatalf("dry-run error = %v\n%s", err, out)
	}
	if !strings.Contains(out, "Click Dot solo está disponible en Windows") {
		t.Fatalf("dry-run must explain the skip:\n%s", out)
	}
}

func TestUpdateUpdatesDotAndSkipsWhenCurrent(t *testing.T) {
	setCLIVersion(t, cliDotVersion)
	rig := newCLIDotRig(t, "windows")
	rig.publish(cliDotVersion)
	home := dotCLIHome(t)

	out, err := execRoot(t, home, "update")
	if err != nil {
		t.Fatalf("update error = %v\n%s", err, out)
	}
	if !strings.Contains(out, "[OK] Click Dot "+cliDotVersion+" instalado") || !strings.Contains(out, "Update completo.") {
		t.Fatalf("update output:\n%s", out)
	}
	rig.fetched = nil
	out, err = execRoot(t, home, "update")
	if err != nil {
		t.Fatalf("second update error = %v\n%s", err, out)
	}
	if !strings.Contains(out, "Click Dot "+cliDotVersion+" ya está actualizado") {
		t.Fatalf("second update must report current:\n%s", out)
	}
	if len(rig.fetched) != 0 {
		t.Fatalf("a current Dot must not be downloaded again: %v", rig.fetched)
	}
}

func TestUpdateDotFailureWarnsAndDoesNotFailUpdate(t *testing.T) {
	setCLIVersion(t, cliDotVersion)
	rig := newCLIDotRig(t, "windows")
	rig.fetchOK = false
	home := dotCLIHome(t)

	out, err := execRoot(t, home, "update")
	if err != nil {
		t.Fatalf("a Dot failure must not fail update: %v\n%s", err, out)
	}
	if !strings.Contains(out, "[warn] No se pudo actualizar Click Dot") || strings.Contains(out, "[FAIL]") || !strings.Contains(out, "Update completo.") {
		t.Fatalf("update output:\n%s", out)
	}
}

func TestUpdateSkipsDotOffWindowsAndDryRunPlansIt(t *testing.T) {
	setCLIVersion(t, cliDotVersion)
	newCLIDotRig(t, "linux")
	home := dotCLIHome(t)
	out, err := execRoot(t, home, "update")
	if err != nil || !strings.Contains(out, "Click Dot solo está disponible en Windows") {
		t.Fatalf("update err=%v\n%s", err, out)
	}
	rig := newCLIDotRig(t, "windows")
	out, err = execRoot(t, home, "update", "--dry-run")
	if err != nil || !strings.Contains(out, "Instalando o actualizando Click Dot "+cliDotVersion) {
		t.Fatalf("dry-run err=%v\n%s", err, out)
	}
	if len(rig.fetched) != 0 {
		t.Fatal("dry-run must not download")
	}
}

func TestUninstallRemovesDotAndPreservesUserData(t *testing.T) {
	setCLIVersion(t, cliDotVersion)
	rig := newCLIDotRig(t, "windows")
	rig.publish(cliDotVersion)
	home := dotCLIHome(t)
	if out, err := execRoot(t, home, "install"); err != nil {
		t.Fatalf("install error = %v\n%s", err, out)
	}
	userData := filepath.Join(rig.roaming, "Click Dot", "credentials.bin")
	if err := os.MkdirAll(filepath.Dir(userData), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(userData, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}

	out, err := execRoot(t, home, "uninstall")
	if err != nil {
		t.Fatalf("uninstall error = %v\n%s", err, out)
	}
	if !strings.Contains(out, "Click Dot desinstalado") || !strings.Contains(out, "se conservan") {
		t.Fatalf("uninstall output lacks the Dot line:\n%s", out)
	}
	if _, err := os.Stat(rig.installDir()); !os.IsNotExist(err) {
		t.Fatal("Dot install directory must be removed")
	}
	if got, err := os.ReadFile(userData); err != nil || string(got) != "secret" {
		t.Fatalf("Dot user data must be preserved: %v", err)
	}
}

func TestUninstallOffWindowsDoesNotMentionDot(t *testing.T) {
	setCLIVersion(t, cliDotVersion)
	rig := newCLIDotRig(t, "linux")
	home := dotCLIHome(t)
	out, err := execRoot(t, home, "uninstall")
	if err != nil {
		t.Fatalf("uninstall error = %v\n%s", err, out)
	}
	if strings.Contains(out, "Click Dot") || len(rig.runs) != 0 {
		t.Fatalf("existing uninstall output must be unchanged off Windows:\n%s", out)
	}
}

func TestDoctorReportsDotStatusAsReadOnlyLine(t *testing.T) {
	setCLIVersion(t, cliDotVersion)
	rig := newCLIDotRig(t, "windows")
	home := dotCLIHome(t)
	if _, err := execRoot(t, home, "install"); err != nil {
		t.Fatalf("install error = %v", err)
	}

	// Not installed: a warning line, healthy exit.
	out, err := execRoot(t, home, "doctor")
	if err != nil {
		t.Fatalf("doctor must stay healthy when Dot is not installed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "[warn] Click Dot: no instalado") {
		t.Fatalf("doctor output:\n%s", out)
	}

	rig.publish(cliDotVersion)
	if out, err := execRoot(t, home, "update"); err != nil {
		t.Fatalf("update error = %v\n%s", err, out)
	}
	rig.fetched, rig.runs = nil, nil
	out, err = execRoot(t, home, "doctor")
	if err != nil || !strings.Contains(out, "[OK] Click Dot: versión "+cliDotVersion+" instalada, coincide con click") {
		t.Fatalf("doctor err=%v\n%s", err, out)
	}
	if len(rig.fetched) != 0 || len(rig.runs) != 0 || len(rig.started) != 1 {
		t.Fatal("doctor must be read-only: no network, no commands, no process start beyond the install's")
	}

	setCLIVersion(t, "1.3.0")
	out, err = execRoot(t, home, "doctor")
	if err != nil || !strings.Contains(out, "[warn] Click Dot: versión "+cliDotVersion+" instalada; click espera 1.3.0") {
		t.Fatalf("version drift must warn, not fail: err=%v\n%s", err, out)
	}
}

func TestDoctorOffWindowsHasNoDotLine(t *testing.T) {
	setCLIVersion(t, cliDotVersion)
	newCLIDotRig(t, "linux")
	home := dotCLIHome(t)
	if _, err := execRoot(t, home, "install"); err != nil {
		t.Fatalf("install error = %v", err)
	}
	out, err := execRoot(t, home, "doctor")
	if err != nil || strings.Contains(out, "Click Dot") {
		t.Fatalf("doctor err=%v\n%s", err, out)
	}
}

func TestRollbackLeavesDotUntouchedAndSaysSo(t *testing.T) {
	setCLIVersion(t, cliDotVersion)
	rig := newCLIDotRig(t, "windows")
	rig.publish(cliDotVersion)
	home := dotCLIHome(t)
	if out, err := execRoot(t, home, "install"); err != nil {
		t.Fatalf("install error = %v\n%s", err, out)
	}
	rig.fetched, rig.runs, rig.started = nil, nil, nil
	exe := filepath.Join(rig.installDir(), "Click Dot.exe")

	out, err := execRoot(t, home, "rollback")
	if err != nil {
		t.Fatalf("rollback error = %v\n%s", err, out)
	}
	if !strings.Contains(out, "[i] Click Dot no se modifica con `click rollback`") {
		t.Fatalf("rollback must say Dot is left as is:\n%s", out)
	}
	if got, err := os.ReadFile(exe); err != nil || string(got) != "exe-"+cliDotVersion {
		t.Fatalf("Dot must be untouched: %v", err)
	}
	if len(rig.runs) != 0 || len(rig.fetched) != 0 {
		t.Fatal("rollback must not stop, download or restart Dot")
	}
}

func TestRollbackOffWindowsOutputUnchanged(t *testing.T) {
	newCLIDotRig(t, "linux")
	home := t.TempDir()
	out, err := execRoot(t, home, "rollback")
	if err != nil || strings.Contains(out, "Click Dot") {
		t.Fatalf("rollback err=%v\n%s", err, out)
	}
}
