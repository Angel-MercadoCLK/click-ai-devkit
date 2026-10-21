package installer

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

// Click Dot is the Windows desktop companion that ships in the same GitHub release as `click`
// (release asset contract: odd/tasks/click-dot-distribution.md, L5). It is a supplementary
// integration: callers surface every failure as a Spanish warning and never fail install/update
// (D45 precedent). Everything below is per-user: nothing is written outside %LOCALAPPDATA%\Programs\Click Dot
// and the Start Menu shortcut, and Dot's own user data (%APPDATA%\Click Dot) is never touched.
const (
	dotReleaseBaseURL = "https://github.com/Angel-MercadoCLK/click-ai-devkit/releases/download/"
	dotInstallDirName = "Click Dot"
	dotExeName        = "Click Dot.exe"
	dotProcessName    = "Click Dot.exe"
	dotShortcutName   = "Click Dot.lnk"
	// dotVersionMarker is the Click-owned file written inside the install directory. Its presence is
	// also the ownership proof uninstall requires before deleting the directory.
	dotVersionMarker = ".click-dot-version"

	dotMaxChecksumsBytes  = 1 << 20
	dotMaxArchiveBytes    = 768 << 20
	dotMaxExtractedBytes  = 2 << 30
	dotMaxArchiveEntries  = 50000
	dotSwapAttempts       = 5
	dotSwapRetryDelay     = 300 * time.Millisecond
	dotDownloadTimeout    = 15 * time.Minute
	dotMaxRedirects       = 5
	dotPowerShellRelative = "System32/WindowsPowerShell/v1.0/powershell.exe"
	dotTaskkillRelative   = "System32/taskkill.exe"
)

var dotVersionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?$`)

// DotOutcome describes what a Sync or Remove call did.
type DotOutcome string

const (
	DotSkippedPlatform DotOutcome = "skipped-platform"
	DotSkippedDev      DotOutcome = "skipped-dev"
	DotCurrent         DotOutcome = "current"
	DotInstalled       DotOutcome = "installed"
	DotUpdated         DotOutcome = "updated"
	DotRemoved         DotOutcome = "removed"
	DotNotInstalled    DotOutcome = "not-installed"
)

// DotResult reports the outcome of SyncDot. Warnings are non-fatal degradations (shortcut or launch
// failures) that happened after the package was already installed.
type DotResult struct {
	Outcome  DotOutcome
	Version  string
	Path     string
	Warnings []string
}

// DotPlan is the side-effect-free description of what SyncDot would do; the CLI preview renders it.
type DotPlan struct {
	Outcome    DotOutcome // DotSkippedPlatform, DotSkippedDev, or DotInstalled (meaning "would install or update")
	Version    string
	InstallDir string
	Asset      string
}

// DotStatus is the read-only view `click doctor` renders.
type DotStatus struct {
	Supported     bool
	InstallDir    string
	ExePath       string
	ExePresent    bool
	MarkerPresent bool
	Version       string // installed version from the Click marker; empty when unknown
}

// DotManager keeps every machine boundary injectable. Tests replace Getenv, Fetch, Run, Start,
// LoginItems and Sleep, and point LOCALAPPDATA/APPDATA at t.TempDir() through Getenv.
type DotManager struct {
	GOOS, GOARCH string
	Getenv       func(string) string
	// BaseURL is the release download root; the tag directory v<version>/ is appended.
	BaseURL string
	Fetch   func(context.Context, string) (io.ReadCloser, error)
	Run     func(context.Context, string, ...string) ([]byte, error)
	Start   func(exe string) error
	// LoginItems edits HKCU Run on Windows; nil means there is nothing to clean.
	LoginItems DotLoginStore
	Sleep      func(time.Duration)

	// Zero values select the production limits; tests lower them to prove the guards.
	MaxArchiveBytes   int64
	MaxExtractedBytes int64
	MaxEntries        int
}

// NewDotManager creates production dependencies without executing any of them.
func NewDotManager() DotManager {
	return DotManager{
		GOOS: runtime.GOOS, GOARCH: runtime.GOARCH,
		Getenv:  os.Getenv,
		BaseURL: dotReleaseBaseURL,
		Fetch:   fetchDotAsset,
		Run: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			return exec.CommandContext(ctx, name, args...).CombinedOutput()
		},
		Start:      startDotDetached,
		LoginItems: newDotLoginStore(),
		Sleep:      time.Sleep,
	}
}

var dotManagerFactory = NewDotManager

// SetDotManagerFactoryForTests replaces all machine/network boundaries and returns a restore function.
func SetDotManagerFactoryForTests(factory func() DotManager) func() {
	old := dotManagerFactory
	dotManagerFactory = factory
	return func() { dotManagerFactory = old }
}

// SyncDot installs or updates Click Dot to the version of the running click binary.
func SyncDot(ctx context.Context, expectedVersion string) (DotResult, error) {
	return dotManagerFactory().Sync(ctx, expectedVersion)
}

// RemoveDot reverses SyncDot's Click-owned writes and preserves Dot's user data.
func RemoveDot(ctx context.Context) (DotResult, error) {
	return dotManagerFactory().Remove(ctx)
}

// PlanDot describes what SyncDot would do without touching the network or the filesystem.
func PlanDot(expectedVersion string) DotPlan {
	return dotManagerFactory().Plan(expectedVersion)
}

// InspectDot reads Dot's installation state without mutating anything.
func InspectDot() DotStatus {
	return dotManagerFactory().Inspect()
}

// DotSupported reports whether this platform manages Dot at all.
func DotSupported() bool {
	return dotManagerFactory().Supported()
}

// Supported is true only on Windows x64, the single platform the release ships a Dot package for (A1).
func (m DotManager) Supported() bool { return m.GOOS == "windows" && m.GOARCH == "amd64" }

// DotReleaseVersion normalizes a click build version into a release version, or "" for development
// builds (no release to download from).
func DotReleaseVersion(raw string) string {
	v := strings.TrimPrefix(strings.TrimSpace(raw), "v")
	if !dotVersionPattern.MatchString(v) || strings.Contains(strings.ToUpper(v), "SNAPSHOT") {
		return ""
	}
	return v
}

// DotAssetName is the release asset name for a version (contract L5).
func DotAssetName(version string) string {
	return "click-dot_" + version + "_windows_amd64.zip"
}

type dotPaths struct {
	programs, installDir, exe, marker, shortcut string
}

func (m DotManager) paths() (dotPaths, error) {
	local := m.Getenv("LOCALAPPDATA")
	appData := m.Getenv("APPDATA")
	if local == "" || !filepath.IsAbs(local) {
		return dotPaths{}, errors.New("no se pudo resolver %LOCALAPPDATA% como ruta absoluta")
	}
	if appData == "" || !filepath.IsAbs(appData) {
		return dotPaths{}, errors.New("no se pudo resolver %APPDATA% como ruta absoluta")
	}
	programs := filepath.Join(local, "Programs")
	installDir := filepath.Join(programs, dotInstallDirName)
	return dotPaths{
		programs:   programs,
		installDir: installDir,
		exe:        filepath.Join(installDir, dotExeName),
		marker:     filepath.Join(installDir, dotVersionMarker),
		shortcut:   filepath.Join(appData, "Microsoft", "Windows", "Start Menu", "Programs", dotShortcutName),
	}, nil
}

// Plan is pure: no network, no filesystem writes.
func (m DotManager) Plan(expectedVersion string) DotPlan {
	if !m.Supported() {
		return DotPlan{Outcome: DotSkippedPlatform}
	}
	version := DotReleaseVersion(expectedVersion)
	if version == "" {
		return DotPlan{Outcome: DotSkippedDev}
	}
	plan := DotPlan{Outcome: DotInstalled, Version: version, Asset: DotAssetName(version)}
	if p, err := m.paths(); err == nil {
		plan.InstallDir = p.installDir
	}
	return plan
}

// Inspect is read-only.
func (m DotManager) Inspect() DotStatus {
	status := DotStatus{Supported: m.Supported()}
	if !status.Supported {
		return status
	}
	p, err := m.paths()
	if err != nil {
		return status
	}
	status.InstallDir, status.ExePath = p.installDir, p.exe
	if info, err := os.Lstat(p.exe); err == nil && info.Mode().IsRegular() {
		status.ExePresent = true
	}
	if raw, err := os.ReadFile(p.marker); err == nil {
		status.MarkerPresent = true
		if v := strings.TrimSpace(string(raw)); dotVersionPattern.MatchString(v) {
			status.Version = v
		}
	}
	return status
}

// Sync downloads, verifies and installs the Dot package that matches expectedVersion. Failures return
// an error; nothing is left half-installed (the old directory is restored when the swap fails).
func (m DotManager) Sync(ctx context.Context, expectedVersion string) (DotResult, error) {
	plan := m.Plan(expectedVersion)
	switch plan.Outcome {
	case DotSkippedPlatform, DotSkippedDev:
		return DotResult{Outcome: plan.Outcome}, nil
	}
	p, err := m.paths()
	if err != nil {
		return DotResult{}, err
	}
	version := plan.Version
	result := DotResult{Version: version, Path: p.exe}

	before := m.Inspect()
	if before.ExePresent && before.Version == version {
		result.Outcome = DotCurrent
		if err := m.writeShortcut(ctx, p); err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("no se pudo actualizar el acceso directo del menú Inicio: %v", err))
		}
		return result, nil
	}

	if err := os.MkdirAll(p.programs, 0o755); err != nil {
		return DotResult{}, fmt.Errorf("no se pudo preparar %s: %w", p.programs, err)
	}
	stage, err := os.MkdirTemp(p.programs, ".click-dot-staging-*")
	if err != nil {
		return DotResult{}, fmt.Errorf("no se pudo crear el directorio temporal: %w", err)
	}
	defer os.RemoveAll(stage)

	base := strings.TrimRight(m.BaseURL, "/") + "/v" + version + "/"
	asset := plan.Asset
	expected, err := m.fetchExpectedChecksum(ctx, base, asset)
	if err != nil {
		return DotResult{}, err
	}
	archive := filepath.Join(stage, "package.zip")
	if err := m.downloadVerified(ctx, base+url.PathEscape(asset), archive, asset, expected); err != nil {
		return DotResult{}, err
	}
	app := filepath.Join(stage, "app")
	if err := extractDotArchive(archive, app, m.maxExtracted(), m.maxEntries()); err != nil {
		return DotResult{}, fmt.Errorf("el paquete %s no es válido: %w", asset, err)
	}
	if info, err := os.Lstat(filepath.Join(app, dotExeName)); err != nil || !info.Mode().IsRegular() {
		return DotResult{}, fmt.Errorf("el paquete %s no contiene %s en la raíz", asset, dotExeName)
	}
	if err := os.WriteFile(filepath.Join(app, dotVersionMarker), []byte(version+"\n"), 0o644); err != nil {
		return DotResult{}, fmt.Errorf("no se pudo registrar la versión instalada: %w", err)
	}

	m.stopDot(ctx)
	if err := m.swapInstallDir(p.installDir, app); err != nil {
		return DotResult{}, err
	}

	result.Outcome = DotInstalled
	if before.ExePresent || before.MarkerPresent {
		result.Outcome = DotUpdated
	}
	if err := m.writeShortcut(ctx, p); err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("no se pudo crear el acceso directo del menú Inicio: %v", err))
	}
	if m.Start != nil {
		if err := m.Start(p.exe); err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("Click Dot se instaló pero no se pudo iniciar: %v", err))
		}
	}
	return result, nil
}

// Remove stops Dot and removes the Click-owned install directory, Start Menu shortcut and start-at-login
// entry. %APPDATA%\Click Dot (credentials, history) is never touched. Every step is attempted and
// the errors are joined, matching uninstall's resilient teardown.
func (m DotManager) Remove(ctx context.Context) (DotResult, error) {
	if !m.Supported() {
		return DotResult{Outcome: DotSkippedPlatform}, nil
	}
	p, err := m.paths()
	if err != nil {
		return DotResult{}, err
	}
	result := DotResult{Outcome: DotNotInstalled, Path: p.installDir}
	var errs []error

	m.stopDot(ctx)

	if m.LoginItems != nil {
		if err := removeDotLoginItems(m.LoginItems, p.exe); err != nil {
			errs = append(errs, fmt.Errorf("entrada de inicio con Windows: %w", err))
		}
	}
	if err := os.Remove(p.shortcut); err == nil {
		result.Outcome = DotRemoved
	} else if !errors.Is(err, os.ErrNotExist) {
		errs = append(errs, fmt.Errorf("acceso directo del menú Inicio: %w", err))
	}

	info, err := os.Lstat(p.installDir)
	switch {
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		errs = append(errs, fmt.Errorf("carpeta de instalación: %w", err))
	case info.Mode()&os.ModeSymlink == 0 && !info.IsDir():
		result.Warnings = append(result.Warnings, p.installDir+" no es una carpeta; se conservó")
	default:
		if _, markerErr := os.Lstat(p.marker); markerErr != nil {
			result.Warnings = append(result.Warnings, p.installDir+" no tiene el marcador de Click; se conservó porque no se puede confirmar que sea de Click")
			break
		}
		if err := m.removeAllRetry(p.installDir); err != nil {
			errs = append(errs, fmt.Errorf("carpeta de instalación: %w", err))
		} else {
			result.Outcome = DotRemoved
		}
	}
	return result, errors.Join(errs...)
}

func (m DotManager) maxArchive() int64 {
	if m.MaxArchiveBytes > 0 {
		return m.MaxArchiveBytes
	}
	return dotMaxArchiveBytes
}

func (m DotManager) maxExtracted() int64 {
	if m.MaxExtractedBytes > 0 {
		return m.MaxExtractedBytes
	}
	return dotMaxExtractedBytes
}

func (m DotManager) maxEntries() int {
	if m.MaxEntries > 0 {
		return m.MaxEntries
	}
	return dotMaxArchiveEntries
}

func (m DotManager) sleep(d time.Duration) {
	if m.Sleep != nil {
		m.Sleep(d)
	}
}

func (m DotManager) systemTool(relative string) string {
	root := m.Getenv("SystemRoot")
	if root == "" {
		root = `C:\Windows`
	}
	return filepath.Join(root, relative)
}

// stopDot ends a running Click Dot. taskkill exits non-zero when nothing is running, and a real
// failure surfaces later as a locked-directory error from the swap, so the result is not fatal here.
func (m DotManager) stopDot(ctx context.Context) {
	if m.Run == nil {
		return
	}
	_, _ = m.Run(ctx, m.systemTool(dotTaskkillRelative), "/IM", dotProcessName, "/F")
}

func (m DotManager) fetchExpectedChecksum(ctx context.Context, base, asset string) ([]byte, error) {
	body, err := m.Fetch(ctx, base+"checksums.txt")
	if err != nil {
		return nil, fmt.Errorf("no se pudo descargar checksums.txt: %w", err)
	}
	defer body.Close()
	data, err := io.ReadAll(io.LimitReader(body, dotMaxChecksumsBytes+1))
	if err != nil {
		return nil, fmt.Errorf("no se pudo leer checksums.txt: %w", err)
	}
	if len(data) > dotMaxChecksumsBytes {
		return nil, errors.New("checksums.txt es demasiado grande")
	}
	return parseDotChecksum(data, asset)
}

// downloadVerified streams the archive to dest while hashing it, then compares against the checksum
// published in the same release. The archive is never opened before the digest matches.
func (m DotManager) downloadVerified(ctx context.Context, assetURL, dest, asset string, expected []byte) error {
	body, err := m.Fetch(ctx, assetURL)
	if err != nil {
		return fmt.Errorf("no se pudo descargar %s: %w", asset, err)
	}
	defer body.Close()
	file, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	hasher := sha256.New()
	limit := m.maxArchive()
	written, copyErr := io.Copy(io.MultiWriter(file, hasher), io.LimitReader(body, limit+1))
	closeErr := file.Close()
	switch {
	case copyErr != nil:
		return fmt.Errorf("descarga incompleta de %s: %w", asset, copyErr)
	case written > limit:
		return fmt.Errorf("%s supera el tamaño máximo permitido (%d bytes); se rechazó la descarga", asset, limit)
	case closeErr != nil:
		return closeErr
	}
	if got := hasher.Sum(nil); hex.EncodeToString(got) != hex.EncodeToString(expected) {
		return fmt.Errorf("SHA-256 no coincide para %s; se rechazó la descarga", asset)
	}
	return nil
}

// swapInstallDir replaces installDir with next. The previous directory is renamed aside first (with
// bounded retries while a dying process releases its handles) and restored if the final rename fails.
func (m DotManager) swapInstallDir(installDir, next string) error {
	var aside string
	if _, err := os.Lstat(installDir); err == nil {
		aside = installDir + ".old-" + randomSuffix()
		var renameErr error
		for attempt := 0; attempt < dotSwapAttempts; attempt++ {
			if renameErr = os.Rename(installDir, aside); renameErr == nil {
				break
			}
			m.sleep(dotSwapRetryDelay)
		}
		if renameErr != nil {
			return fmt.Errorf("no se pudo reemplazar %s (¿Click Dot sigue abierto?): %w", installDir, renameErr)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("no se pudo inspeccionar %s: %w", installDir, err)
	}
	if err := os.Rename(next, installDir); err != nil {
		if aside != "" {
			_ = os.Rename(aside, installDir)
		}
		return fmt.Errorf("no se pudo instalar Click Dot en %s: %w", installDir, err)
	}
	if aside != "" {
		_ = os.RemoveAll(aside)
	}
	return nil
}

func (m DotManager) removeAllRetry(path string) error {
	var err error
	for attempt := 0; attempt < dotSwapAttempts; attempt++ {
		if err = os.RemoveAll(path); err == nil {
			return nil
		}
		m.sleep(dotSwapRetryDelay)
	}
	return err
}

// writeShortcut creates or refreshes the Start Menu shortcut through Windows PowerShell 5.1's
// WScript.Shell, which ships with every supported Windows and needs no extra dependency.
func (m DotManager) writeShortcut(ctx context.Context, p dotPaths) error {
	if m.Run == nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(p.shortcut), 0o755); err != nil {
		return err
	}
	script := strings.Join([]string{
		"$ErrorActionPreference='Stop'",
		"$s=(New-Object -ComObject WScript.Shell).CreateShortcut(" + powerShellQuote(p.shortcut) + ")",
		"$s.TargetPath=" + powerShellQuote(p.exe),
		"$s.WorkingDirectory=" + powerShellQuote(p.installDir),
		"$s.IconLocation=" + powerShellQuote(p.exe+",0"),
		"$s.Description='Click Dot'",
		"$s.Save()",
	}, "; ")
	if _, err := m.Run(ctx, m.systemTool(dotPowerShellRelative), "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", script); err != nil {
		return err
	}
	return nil
}

func powerShellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func randomSuffix() string {
	var b [6]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

var dotAllowedDownloadHosts = []string{"github.com", "githubusercontent.com"}

func dotHostAllowed(host string) bool {
	host = strings.ToLower(host)
	for _, allowed := range dotAllowedDownloadHosts {
		if host == allowed || strings.HasSuffix(host, "."+allowed) {
			return true
		}
	}
	return false
}

// fetchDotAsset is the production download: HTTPS only, a bounded redirect chain restricted to GitHub
// hosts, no automatic decompression (the digest covers the exact bytes), and no credentials.
func fetchDotAsset(ctx context.Context, rawURL string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	if req.URL.Scheme != "https" || !dotHostAllowed(req.URL.Hostname()) {
		return nil, errors.New("la descarga requiere HTTPS desde GitHub")
	}
	client := &http.Client{
		Timeout:   dotDownloadTimeout,
		Transport: &http.Transport{Proxy: http.ProxyFromEnvironment, DisableCompression: true},
		CheckRedirect: func(next *http.Request, via []*http.Request) error {
			if len(via) > dotMaxRedirects {
				return errors.New("demasiadas redirecciones")
			}
			if next.URL.Scheme != "https" || !dotHostAllowed(next.URL.Hostname()) {
				return errors.New("la redirección sale de GitHub o no usa HTTPS")
			}
			next.Header.Del("Authorization")
			return nil
		},
	}
	response, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		return nil, fmt.Errorf("HTTP %d", response.StatusCode)
	}
	return response.Body, nil
}
