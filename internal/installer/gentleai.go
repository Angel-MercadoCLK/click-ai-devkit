package installer

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"go/version"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/manifest"
)

const gentleAIMaxBytes = 100 << 20

var gentleAIVersionPattern = regexp.MustCompile(`^v?([0-9]+)\.([0-9]+)\.([0-9]+)(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$`)

// GentleAIProvisioner keeps machine and execution boundaries injectable. HomeDir controls
// the filesystem root; WriteBinary is the atomic filesystem write seam. Tests must replace
// Fetch, Run, and EnsureOnPath as well as HomeDir to avoid network and real-user mutations.
type GentleAIProvisioner struct {
	GOOS, GOARCH string
	HomeDir      func() (string, error)
	Environ      func() []string
	Fetch        func(context.Context, string) (io.ReadCloser, error)
	Run          func(context.Context, []string, string, ...string) ([]byte, error)
	WriteBinary  func(string, []byte) error
	EnsureOnPath func(string) (bool, error)
}

// GentleAIProvisionResult exposes PATH ownership for later install/rollback bookkeeping.
// PATH warnings are non-fatal, like Engram provisioning; callers can use Path immediately
// without waiting for a new shell or relying on the current process's PATH.
type GentleAIProvisionResult struct {
	Path, PathDir, PathWarning string
	PathChanged                bool
}

// GentleAIVersionProbe distinguishes version drift from an unavailable/invalid probe.
type GentleAIVersionProbe struct {
	Version    string
	MatchesPin bool
}

// NewGentleAIProvisioner creates production dependencies without executing any of them.
func NewGentleAIProvisioner() GentleAIProvisioner {
	return GentleAIProvisioner{
		GOOS: runtime.GOOS, GOARCH: runtime.GOARCH,
		HomeDir: os.UserHomeDir, Environ: os.Environ,
		Fetch: fetchGentleAIAsset,
		Run: func(ctx context.Context, env []string, name string, args ...string) ([]byte, error) {
			cmd := exec.CommandContext(ctx, name, args...)
			cmd.Env = env
			return cmd.Output()
		},
		WriteBinary:  writeGentleAIBinary,
		EnsureOnPath: persistGentleAIPath,
	}
}

// Provision always installs the qualified pin, including when a newer binary exists.
// The binary lives in ~/.click/bin, independent of the selected runtime's config home.
// No runtime installation, plugin registration, doctor check, or state file is wired here.
func (p GentleAIProvisioner) Provision(ctx context.Context, pin manifest.GentleAI) (GentleAIProvisionResult, error) {
	var result GentleAIProvisionResult
	if !gentleAIVersionPattern.MatchString(pin.Version) || !strings.HasPrefix(pin.Version, "v") || pin.Module == "" {
		return result, fmt.Errorf("gentle-ai: versión o módulo fijado no válido")
	}
	if (p.GOOS != "linux" && p.GOOS != "darwin" && p.GOOS != "windows") || (p.GOARCH != "amd64" && p.GOARCH != "arm64") {
		return result, fmt.Errorf("gentle-ai: plataforma no compatible: %s/%s", p.GOOS, p.GOARCH)
	}
	home, err := p.HomeDir()
	if err != nil || !filepath.IsAbs(home) {
		return result, fmt.Errorf("gentle-ai: no se pudo resolver un directorio personal absoluto: %v", err)
	}
	dir := filepath.Join(home, ".click", "bin")
	name := "gentle-ai"
	var binary []byte
	if p.GOOS == "windows" {
		name += ".exe"
		binary, err = p.buildWindows(ctx, pin, dir)
	} else {
		binary, err = p.downloadBinary(ctx, pin)
	}
	if err != nil {
		return result, err
	}
	path := filepath.Join(dir, name)
	if err := p.WriteBinary(path, binary); err != nil {
		return result, fmt.Errorf("gentle-ai: no se pudo instalar el binario en %s: %w", path, err)
	}
	result.Path, result.PathDir = path, dir
	result.PathChanged, err = p.EnsureOnPath(dir)
	if err != nil {
		result.PathWarning = fmt.Sprintf("No se pudo agregar %s al PATH persistente: %v. Use %s o abra una nueva terminal después de corregir el PATH.", dir, err, path)
	}
	return result, nil
}

func (p GentleAIProvisioner) downloadBinary(ctx context.Context, pin manifest.GentleAI) ([]byte, error) {
	asset, ok := pin.Assets[p.GOOS+"_"+p.GOARCH]
	if !ok {
		return nil, fmt.Errorf("gentle-ai: plataforma no compatible sin artefacto calificado: %s/%s", p.GOOS, p.GOARCH)
	}
	expected, err := hex.DecodeString(asset.SHA256)
	if err != nil || len(expected) != sha256.Size || asset.Name == "" || filepath.Base(asset.Name) != asset.Name || strings.ContainsAny(asset.Name, `\/?#`) {
		return nil, fmt.Errorf("gentle-ai: artefacto o SHA-256 fijado no válido")
	}
	url := "https://github.com/Gentleman-Programming/gentle-ai/releases/download/" + pin.Version + "/" + asset.Name
	body, err := p.Fetch(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("gentle-ai: no se pudo descargar %s: %w", asset.Name, err)
	}
	defer body.Close()
	archive, err := io.ReadAll(io.LimitReader(body, gentleAIMaxBytes+1))
	if err != nil || len(archive) > gentleAIMaxBytes {
		return nil, fmt.Errorf("gentle-ai: descarga incompleta o demasiado grande: %v", err)
	}
	actual := sha256.Sum256(archive)
	if !bytes.Equal(actual[:], expected) {
		return nil, fmt.Errorf("gentle-ai: SHA-256 no coincide para %s; se rechazó la descarga", asset.Name)
	}
	binary, err := extractGentleAIBinary(archive)
	if err != nil {
		return nil, fmt.Errorf("gentle-ai: no se pudo extraer el binario: %w", err)
	}
	return binary, nil
}

func extractGentleAIBinary(archive []byte) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, err
	}
	defer gz.Close()
	// Count every decompressed byte, including tar headers, skipped entries and
	// trailing gzip data. The extra byte distinguishes an exact limit from overflow.
	decompressed := &io.LimitedReader{R: gz, N: gentleAIMaxBytes + 1}
	checkBudget := func(err error) error {
		if decompressed.N == 0 {
			return fmt.Errorf("archivo descomprimido demasiado grande (máximo %d bytes)", gentleAIMaxBytes)
		}
		return err
	}
	tr := tar.NewReader(decompressed)
	var binary []byte
	for {
		h, err := tr.Next()
		err = checkBudget(err)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		// Ignore all other paths, including traversal names; never extract them to disk.
		if h.Name != "gentle-ai" && h.Name != "./gentle-ai" {
			continue
		}
		if binary != nil {
			return nil, fmt.Errorf("binario gentle-ai duplicado")
		}
		if h.Typeflag != tar.TypeReg && h.Typeflag != tar.TypeRegA {
			return nil, fmt.Errorf("gentle-ai no es un archivo regular")
		}
		if h.Size <= 0 {
			return nil, fmt.Errorf("binario gentle-ai vacío")
		}
		if h.Size > gentleAIMaxBytes {
			return nil, fmt.Errorf("binario gentle-ai demasiado grande")
		}
		binary, err = io.ReadAll(tr)
		if err := checkBudget(err); err != nil {
			return nil, err
		}
	}
	// Consume the gzip footer too, so truncated/corrupt compressed streams cannot pass.
	_, err = io.Copy(io.Discard, decompressed)
	if err := checkBudget(err); err != nil {
		return nil, err
	}
	if binary == nil {
		return nil, fmt.Errorf("el archivo no contiene el binario gentle-ai")
	}
	return binary, nil
}

func (p GentleAIProvisioner) buildWindows(ctx context.Context, pin manifest.GentleAI, dir string) ([]byte, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("gentle-ai: no se pudo crear %s: %w", dir, err)
	}
	// A private GOBIN prevents a failed build from replacing the installed binary and
	// avoids overwriting a developer-managed Go bin. Delete only this owned staging dir.
	stage, err := os.MkdirTemp(dir, ".gentle-ai-build-*")
	if err != nil {
		return nil, fmt.Errorf("gentle-ai: no se pudo preparar la compilación: %w", err)
	}
	defer os.RemoveAll(stage)
	env := gentleAIGoEnvironment(p.Environ(), stage, p.GOARCH)
	output, err := p.Run(ctx, env, "go", "version")
	if err == nil && !version.IsValid(gentleAIGoVersionToken(string(output))) {
		return nil, fmt.Errorf("gentle-ai: versión de la herramienta Go no compatible o no identificable. Use una versión oficial de Go >= 1.25.10. Comando fijado: go install %s@%s", pin.Module, pin.Version)
	}
	if err != nil || !gentleAIGoVersionSupported(string(output)) {
		return nil, fmt.Errorf("gentle-ai requiere Go >= 1.25.10. Instale o actualice Go y vuelva a ejecutar click install. Comando fijado: go install %s@%s", pin.Module, pin.Version)
	}
	if _, err := p.Run(ctx, env, "go", "install", pin.Module+"@"+pin.Version); err != nil {
		return nil, fmt.Errorf("gentle-ai: no se pudo ejecutar go install %s@%s: %w", pin.Module, pin.Version, err)
	}
	binary, err := os.ReadFile(filepath.Join(stage, "gentle-ai.exe"))
	if err != nil || len(binary) == 0 {
		return nil, fmt.Errorf("gentle-ai: go install no produjo un binario válido: %v", err)
	}
	return binary, nil
}

// Disable saved Go configuration and ambient build hooks before any Go invocation.
// Use only the trusted HTTPS proxy: a direct fallback would bypass this boundary.
func gentleAIGoEnvironment(base []string, stage, arch string) []string {
	env := GentleAIEnvironment(base)
	for _, setting := range []string{
		"GOENV=off", "GOFLAGS=", "GOSUMDB=sum.golang.org", "GONOSUMDB=none",
		"GOPRIVATE=none", "GONOPROXY=none", "GOINSECURE=",
		"GOPROXY=https://proxy.golang.org", "GOTOOLCHAIN=local", "GOBIN=" + stage,
		"GOOS=windows", "GOARCH=" + arch,
	} {
		env = replaceGentleAIEnv(env, setting)
	}
	return env
}

func gentleAIGoVersionToken(output string) string {
	fields := strings.Fields(output)
	if len(fields) != 4 || fields[0] != "go" || fields[1] != "version" {
		return ""
	}
	return fields[2]
}

func gentleAIGoVersionSupported(output string) bool {
	toolchain := gentleAIGoVersionToken(output)
	return version.IsValid(toolchain) && version.Compare(toolchain, "go1.25.10") >= 0
}

// ProbeVersion executes an explicitly resolved binary; callers choose managed-path or
// PATH discovery. A valid nonmatching version is drift, not a provisioning failure.
func (p GentleAIProvisioner) ProbeVersion(ctx context.Context, path, pin string) (GentleAIVersionProbe, error) {
	output, err := p.Run(ctx, GentleAIEnvironment(p.Environ()), path, "version")
	if err != nil {
		return GentleAIVersionProbe{}, fmt.Errorf("gentle-ai: no se pudo consultar la versión: %w", err)
	}
	fields := strings.Fields(string(output))
	if len(fields) != 2 || fields[0] != "gentle-ai" || !gentleAIVersionPattern.MatchString(fields[1]) {
		return GentleAIVersionProbe{}, fmt.Errorf("gentle-ai: salida de versión no válida")
	}
	version := "v" + strings.TrimPrefix(fields[1], "v")
	return GentleAIVersionProbe{Version: version, MatchesPin: version == "v"+strings.TrimPrefix(pin, "v")}, nil
}

// GentleAIEnvironment returns a copy with upstream self-update unconditionally disabled.
func GentleAIEnvironment(base []string) []string {
	return replaceGentleAIEnv(base, "GENTLE_AI_NO_SELF_UPDATE=1")
}

func replaceGentleAIEnv(base []string, setting string) []string {
	key, _, _ := strings.Cut(setting, "=")
	env := make([]string, 0, len(base)+1)
	for _, entry := range base {
		entryKey, _, _ := strings.Cut(entry, "=")
		if !strings.EqualFold(entryKey, key) {
			env = append(env, entry)
		}
	}
	return append(env, setting)
}

func fetchGentleAIAsset(ctx context.Context, url string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if req.URL.Scheme != "https" {
		return nil, fmt.Errorf("gentle-ai: la descarga requiere HTTPS")
	}
	client := &http.Client{
		Timeout: 2 * time.Minute,
		CheckRedirect: func(next *http.Request, via []*http.Request) error {
			if next.URL.Scheme != "https" {
				return fmt.Errorf("gentle-ai: la redirección requiere HTTPS")
			}
			if len(via) >= 10 {
				return fmt.Errorf("gentle-ai: demasiadas redirecciones (máximo 10)")
			}
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

// writeGentleAIBinary stages on the same filesystem, syncs and renames without following
// an existing binary symlink. Failure preserves the old binary and removes only our temp file.
func writeGentleAIBinary(path string, binary []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".gentle-ai-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()
	if _, err := tmp.Write(binary); err != nil {
		return err
	}
	if err := tmp.Chmod(0o755); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// persistGentleAIPath uses the existing Windows registry store, but a separate POSIX
// block: the Engram store has only one Go-bin block and would replace Engram's PATH.
// Bookkeeping/removal of this gentle-ai block belongs to later integration tasks.
func persistGentleAIPath(dir string) (bool, error) {
	if runtime.GOOS == "windows" {
		return pathStoreFactory().EnsureOnPath(dir)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return false, err
	}
	var targets []string
	switch filepath.Base(os.Getenv("SHELL")) {
	case "fish":
		return false, fmt.Errorf("agregue %s al PATH de fish manualmente", dir)
	case "zsh":
		targets = []string{filepath.Join(home, ".zshrc")}
	case "bash":
		login := filepath.Join(home, ".bash_profile")
		for _, candidate := range []string{".bash_profile", ".bash_login", ".profile"} {
			path := filepath.Join(home, candidate)
			if _, err := os.Stat(path); err == nil {
				login = path
				break
			} else if !os.IsNotExist(err) {
				return false, err
			}
		}
		targets = []string{login, filepath.Join(home, ".bashrc")}
	default:
		targets = []string{filepath.Join(home, ".profile")}
	}
	quoted := "'" + strings.ReplaceAll(dir, "'", "'\"'\"'") + "'"
	block := "# >>> click-ai-devkit gentle-ai bin (managed) >>>\nexport PATH=" + quoted + ":\"$PATH\"\n# <<< click-ai-devkit gentle-ai bin (managed) <<<\n"
	changed := false
	for _, path := range targets {
		existing, err := readFileOrEmpty(path)
		if err != nil {
			return changed, err
		}
		if strings.Contains(existing, block) {
			continue
		}
		mode := os.FileMode(0o644)
		if info, err := os.Stat(path); err == nil {
			mode = info.Mode().Perm()
		} else if !os.IsNotExist(err) {
			return changed, err
		}
		if existing != "" && !strings.HasSuffix(existing, "\n") {
			existing += "\n"
		}
		if err := atomicWriteFile(path, []byte(existing+block), mode); err != nil {
			return changed, err
		}
		changed = true
	}
	return changed, nil
}
