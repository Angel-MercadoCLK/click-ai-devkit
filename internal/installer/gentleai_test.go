package installer

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/manifest"
)

func gentleAIArchive(t *testing.T, headers []*tar.Header) []byte {
	t.Helper()
	var b bytes.Buffer
	gz := gzip.NewWriter(&b)
	tw := tar.NewWriter(gz)
	for _, h := range headers {
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if h.Size > 0 {
			if _, err := tw.Write(bytes.Repeat([]byte("x"), int(h.Size))); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func gentleAIFixture(t *testing.T, platform string, archive []byte) (GentleAIProvisioner, manifest.GentleAI, string) {
	t.Helper()
	home := t.TempDir()
	parts := strings.Split(platform, "_")
	m, err := manifest.Load()
	if err != nil {
		t.Fatal(err)
	}
	pin := m.GentleAI
	if archive != nil {
		pin.Assets = map[string]manifest.GentleAIAsset{platform: {Name: "fixture.tar.gz", SHA256: fmt.Sprintf("%x", sha256.Sum256(archive))}}
	}
	p := NewGentleAIProvisioner()
	p.GOOS, p.GOARCH = parts[0], parts[1]
	p.HomeDir = func() (string, error) { return home, nil }
	p.Environ = func() []string { return []string{"PATH=isolated", "GENTLE_AI_NO_SELF_UPDATE=0"} }
	p.Fetch = func(_ context.Context, url string) (io.ReadCloser, error) {
		want := "https://github.com/Gentleman-Programming/gentle-ai/releases/download/v4.0.0/fixture.tar.gz"
		if url != want {
			t.Fatalf("URL = %q, want %q", url, want)
		}
		return io.NopCloser(bytes.NewReader(archive)), nil
	}
	p.Run = func(context.Context, []string, string, ...string) ([]byte, error) {
		t.Fatal("unexpected command")
		return nil, nil
	}
	p.EnsureOnPath = func(string) (bool, error) { t.Fatal("unexpected PATH write"); return false, nil }
	return p, pin, filepath.Join(home, ".click", "bin")
}

func TestGentleAIProvision_QualifiedPlatforms(t *testing.T) {
	for _, platform := range []string{"linux_amd64", "linux_arm64", "darwin_amd64", "darwin_arm64"} {
		t.Run(platform, func(t *testing.T) {
			archive := gentleAIArchive(t, []*tar.Header{{Name: "../../ignored", Mode: 0o644, Size: 3}, {Name: "gentle-ai", Mode: 0o755, Size: 6}})
			p, pin, dir := gentleAIFixture(t, platform, archive)
			calls := 0
			p.EnsureOnPath = func(got string) (bool, error) {
				calls++
				if got != dir {
					t.Fatalf("PATH dir = %s, want %s", got, dir)
				}
				if b, err := os.ReadFile(filepath.Join(dir, "gentle-ai")); err != nil || string(b) != "xxxxxx" {
					t.Fatalf("binary not installed before PATH persistence: %q, %v", b, err)
				}
				return true, nil
			}
			for i := 0; i < 2; i++ {
				result, err := p.Provision(context.Background(), pin)
				if err != nil {
					t.Fatal(err)
				}
				if result.Path != filepath.Join(dir, "gentle-ai") || !result.PathChanged || result.PathDir != dir || result.PathWarning != "" {
					t.Fatalf("result = %+v", result)
				}
			}
			entries, err := os.ReadDir(dir)
			if err != nil || len(entries) != 1 || entries[0].Name() != "gentle-ai" {
				t.Fatalf("extracted extra files: %v, %v", entries, err)
			}
			info, err := os.Stat(filepath.Join(dir, "gentle-ai"))
			if err != nil || (runtime.GOOS != "windows" && info.Mode().Perm() != 0o755) {
				t.Fatalf("binary permissions: %v, %v", info, err)
			}
			if calls != 2 {
				t.Fatalf("PATH persistence calls = %d, want 2", calls)
			}
		})
	}
}

func TestGentleAIProvision_RejectsInvalidDownloadsWithoutReplacingBinary(t *testing.T) {
	regular := &tar.Header{Name: "gentle-ai", Mode: 0o755, Size: 6}
	for _, tt := range []struct {
		name                                     string
		headers                                  []*tar.Header
		hashMismatch, fetchFailure, writeFailure bool
		want                                     string
	}{
		{name: "checksum checked before invalid gzip", hashMismatch: true, want: "SHA-256"},
		{name: "invalid gzip", want: "extraer"},
		{name: "missing binary", headers: []*tar.Header{{Name: "README", Size: 3}}, want: "gentle-ai"},
		{name: "symlink binary", headers: []*tar.Header{{Name: "gentle-ai", Typeflag: tar.TypeSymlink, Linkname: "../../outside"}}, want: "regular"},
		{name: "empty binary", headers: []*tar.Header{{Name: "gentle-ai"}}, want: "vacío"},
		{name: "duplicate binary", headers: []*tar.Header{regular, regular}, want: "duplicado"},
		{name: "fetch failed", fetchFailure: true, want: "descargar"},
		{name: "atomic write failed", headers: []*tar.Header{regular}, writeFailure: true, want: "instalar"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			archive := []byte("not gzip")
			if tt.headers != nil {
				archive = gentleAIArchive(t, tt.headers)
			}
			p, pin, dir := gentleAIFixture(t, "linux_amd64", archive)
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "gentle-ai")
			if err := os.WriteFile(path, []byte("previous"), 0o755); err != nil {
				t.Fatal(err)
			}
			if tt.hashMismatch {
				a := pin.Assets["linux_amd64"]
				a.SHA256 = strings.Repeat("0", 64)
				pin.Assets["linux_amd64"] = a
			}
			if tt.fetchFailure {
				p.Fetch = func(context.Context, string) (io.ReadCloser, error) { return nil, errors.New("offline") }
			}
			if tt.writeFailure {
				p.WriteBinary = func(string, []byte) error { return errors.New("disk full") }
			}
			if _, err := p.Provision(context.Background(), pin); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
			if b, err := os.ReadFile(path); err != nil || string(b) != "previous" {
				t.Fatalf("old binary changed: %q, %v", b, err)
			}
			entries, _ := os.ReadDir(dir)
			if len(entries) != 1 {
				t.Fatalf("partial files remain: %v", entries)
			}
		})
	}
}

func TestGentleAIProvision_UnsupportedPlatformsAndInvalidPin(t *testing.T) {
	for _, platform := range []string{"freebsd_amd64", "linux_386", "darwin_386", "windows_386"} {
		t.Run(platform, func(t *testing.T) {
			p, pin, _ := gentleAIFixture(t, platform, nil)
			p.Fetch = func(context.Context, string) (io.ReadCloser, error) {
				t.Fatal("download on unsupported platform")
				return nil, nil
			}
			if _, err := p.Provision(context.Background(), pin); err == nil || !strings.Contains(err.Error(), "no compatible") {
				t.Fatalf("error = %v", err)
			}
		})
	}
	p, pin, _ := gentleAIFixture(t, "linux_amd64", nil)
	pin.Version = "latest"
	if _, err := p.Provision(context.Background(), pin); err == nil || !strings.Contains(err.Error(), "versión") {
		t.Fatalf("error = %v", err)
	}
}

func TestGentleAIProvision_WindowsGoVersionGate(t *testing.T) {
	for _, tt := range []struct {
		output               string
		missing, wantInstall bool
	}{
		{missing: true}, {output: "go version go1.25.9 windows/amd64"}, {output: "go version go1.24.15 windows/amd64"},
		{output: "go version go1.25.10 windows/amd64", wantInstall: true},
		{output: "go version go1.26.0 windows/arm64", wantInstall: true},
		{output: "go version go1.25.10rc1 windows/amd64"}, {output: "invalid"},
	} {
		t.Run(fmt.Sprintf("%s_missing=%t", tt.output, tt.missing), func(t *testing.T) {
			p, pin, dir := gentleAIFixture(t, "windows_amd64", nil)
			installs := 0
			p.Run = func(_ context.Context, env []string, name string, args ...string) ([]byte, error) {
				if name != "go" {
					t.Fatalf("command = %q", name)
				}
				if reflect.DeepEqual(args, []string{"version"}) {
					if tt.missing {
						return nil, errors.New("not found")
					}
					return []byte(tt.output), nil
				}
				installs++
				if !reflect.DeepEqual(args, []string{"install", "github.com/gentleman-programming/gentle-ai/v4/cmd/gentle-ai@v4.0.0"}) {
					t.Fatalf("args = %v", args)
				}
				if !containsEnv(env, "GOSUMDB=sum.golang.org") || !containsEnv(env, "GOTOOLCHAIN=local") || !containsEnv(env, "GONOSUMDB=none") {
					t.Fatalf("integrity env = %v", env)
				}
				for _, e := range env {
					if strings.HasPrefix(e, "GOBIN=") {
						return nil, os.WriteFile(filepath.Join(strings.TrimPrefix(e, "GOBIN="), "gentle-ai.exe"), []byte("built"), 0o755)
					}
				}
				t.Fatal("missing isolated GOBIN")
				return nil, nil
			}
			p.EnsureOnPath = func(string) (bool, error) { return false, nil }
			result, err := p.Provision(context.Background(), pin)
			if tt.wantInstall {
				if err != nil || installs != 1 || result.Path != filepath.Join(dir, "gentle-ai.exe") {
					t.Fatalf("result = %+v, installs = %d, err = %v", result, installs, err)
				}
			} else if err == nil || installs != 0 || !strings.Contains(err.Error(), "Go >= 1.25.10") || !strings.Contains(err.Error(), pin.Module+"@"+pin.Version) {
				t.Fatalf("installs = %d, remediation = %v", installs, err)
			}
		})
	}
}

func TestGentleAIProvision_WindowsFailuresPreserveBinary(t *testing.T) {
	for _, fail := range []string{"install", "missing output", "write"} {
		t.Run(fail, func(t *testing.T) {
			p, pin, dir := gentleAIFixture(t, "windows_arm64", nil)
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "gentle-ai.exe")
			if err := os.WriteFile(path, []byte("old"), 0o755); err != nil {
				t.Fatal(err)
			}
			p.Run = func(_ context.Context, env []string, _ string, args ...string) ([]byte, error) {
				if args[0] == "version" {
					return []byte("go version go1.25.10 windows/arm64"), nil
				}
				if fail == "install" {
					return nil, errors.New("compile failed")
				}
				if fail == "write" {
					for _, e := range env {
						if strings.HasPrefix(e, "GOBIN=") {
							return nil, os.WriteFile(filepath.Join(strings.TrimPrefix(e, "GOBIN="), "gentle-ai.exe"), []byte("new"), 0o755)
						}
					}
				}
				return nil, nil
			}
			if fail == "write" {
				p.WriteBinary = func(string, []byte) error { return errors.New("disk full") }
			}
			if _, err := p.Provision(context.Background(), pin); err == nil {
				t.Fatal("expected error")
			}
			b, _ := os.ReadFile(path)
			entries, _ := os.ReadDir(dir)
			if string(b) != "old" || len(entries) != 1 {
				t.Fatalf("binary or staging changed: %q, %v", b, entries)
			}
		})
	}
}

func TestGentleAIProbeAndSuppressedEnvironment(t *testing.T) {
	for _, tt := range []struct {
		output, version string
		matches, fails  bool
	}{
		{output: "gentle-ai 4.0.0\n", version: "v4.0.0", matches: true},
		{output: "gentle-ai v4.0.1\n", version: "v4.0.1"},
		{output: "gentle-ai 4.0.0-rc.1\n", version: "v4.0.0-rc.1"},
		{output: "invalid", fails: true}, {fails: true},
	} {
		t.Run(tt.output, func(t *testing.T) {
			p, pin, dir := gentleAIFixture(t, "linux_amd64", nil)
			path := filepath.Join(dir, "gentle-ai")
			p.Run = func(_ context.Context, env []string, name string, args ...string) ([]byte, error) {
				if name != path || !reflect.DeepEqual(args, []string{"version"}) || !containsEnv(env, "GENTLE_AI_NO_SELF_UPDATE=1") {
					t.Fatalf("probe: %s %v, env %v", name, args, env)
				}
				if tt.output == "" {
					return nil, errors.New("missing binary")
				}
				return []byte(tt.output), nil
			}
			got, err := p.ProbeVersion(context.Background(), path, pin.Version)
			if tt.fails {
				if err == nil {
					t.Fatal("expected probe error")
				}
				return
			}
			if err != nil || got.Version != tt.version || got.MatchesPin != tt.matches {
				t.Fatalf("probe = %+v, %v", got, err)
			}
		})
	}
	base := []string{"A=B", "GENTLE_AI_NO_SELF_UPDATE=0", "gentle_ai_no_self_update=0"}
	original := append([]string(nil), base...)
	got := GentleAIEnvironment(base)
	if !reflect.DeepEqual(base, original) || !reflect.DeepEqual(got, []string{"A=B", "GENTLE_AI_NO_SELF_UPDATE=1"}) {
		t.Fatalf("env = %v, base = %v", got, base)
	}
}

func TestGentleAIProvision_PathWarningDoesNotUndoInstallation(t *testing.T) {
	archive := gentleAIArchive(t, []*tar.Header{{Name: "gentle-ai", Size: 1}})
	p, pin, dir := gentleAIFixture(t, "linux_amd64", archive)
	p.EnsureOnPath = func(string) (bool, error) { return true, errors.New("permission denied") }
	result, err := p.Provision(context.Background(), pin)
	if err != nil || !result.PathChanged || result.PathDir != dir || !strings.Contains(result.PathWarning, "PATH persistente") {
		t.Fatalf("result = %+v, err = %v", result, err)
	}
}

func containsEnv(env []string, value string) bool {
	for _, e := range env {
		if e == value {
			return true
		}
	}
	return false
}

func TestGentleAIPathPersistence_PreservesEngramAndIsIdempotent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell PATH persistence")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SHELL", "/bin/bash")
	dir := filepath.Join(home, ".click", "bin")
	prior := "# >>> click-ai-devkit go bin (managed) >>>\nexport PATH='/engram/go/bin':\"$PATH\"\n# <<< click-ai-devkit go bin (managed) <<<\n"
	for _, file := range []string{".bash_profile", ".bashrc"} {
		if err := os.WriteFile(filepath.Join(home, file), []byte(prior), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 2; i++ {
		changed, err := persistGentleAIPath(dir)
		if err != nil || changed != (i == 0) {
			t.Fatalf("changed = %t, err = %v", changed, err)
		}
	}
	for _, file := range []string{".bash_profile", ".bashrc"} {
		b, err := os.ReadFile(filepath.Join(home, file))
		if err != nil || !strings.HasPrefix(string(b), prior) || strings.Count(string(b), "export PATH=") != 2 || !strings.Contains(string(b), dir) {
			t.Fatalf("PATH content = %q, %v", b, err)
		}
	}
}

func TestGentleAIProvision_InvalidAssetMetadataNeverFetches(t *testing.T) {
	for _, asset := range []manifest.GentleAIAsset{
		{Name: "release.tar.gz", SHA256: "bad"},
		{Name: "../../escape.tar.gz", SHA256: strings.Repeat("0", 64)},
		{Name: "release.tar.gz?redirect=bad", SHA256: strings.Repeat("0", 64)},
	} {
		t.Run(asset.Name+asset.SHA256[:3], func(t *testing.T) {
			p, pin, _ := gentleAIFixture(t, "linux_amd64", nil)
			pin.Assets = map[string]manifest.GentleAIAsset{"linux_amd64": asset}
			p.Fetch = func(context.Context, string) (io.ReadCloser, error) {
				t.Fatal("unqualified asset fetched")
				return nil, nil
			}
			if _, err := p.Provision(context.Background(), pin); err == nil || !strings.Contains(err.Error(), "SHA-256") {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestGentleAIBinaryWrite_AtomicFailureAndSymlinkIsolation(t *testing.T) {
	for _, tt := range []string{"rename failure", "symlink isolation"} {
		t.Run(tt, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "gentle-ai")
			if tt == "rename failure" {
				if err := os.Mkdir(path, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := writeGentleAIBinary(path, []byte("new")); err == nil {
					t.Fatal("expected rename failure")
				}
				entries, err := os.ReadDir(dir)
				if err != nil || len(entries) != 1 || !entries[0].IsDir() {
					t.Fatalf("partial temp files: %v, %v", entries, err)
				}
				return
			}
			outside := filepath.Join(t.TempDir(), "developer-binary")
			if err := os.WriteFile(outside, []byte("old"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, path); err != nil {
				t.Skipf("symlinks unavailable: %v", err)
			}
			if err := writeGentleAIBinary(path, []byte("new")); err != nil {
				t.Fatal(err)
			}
			b, err := os.ReadFile(outside)
			if err != nil || string(b) != "old" {
				t.Fatalf("foreign symlink target changed: %q, %v", b, err)
			}
			b, err = os.ReadFile(path)
			if err != nil || string(b) != "new" {
				t.Fatalf("managed binary = %q, %v", b, err)
			}
		})
	}
}

func TestGentleAIPathPersistence_Shells(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell PATH persistence")
	}
	for _, shell := range []string{"zsh", "bash", "sh", "fish"} {
		t.Run(shell, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("SHELL", "/bin/"+shell)
			if shell == "bash" {
				if err := os.WriteFile(filepath.Join(home, ".bash_login"), []byte("# existing\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			changed, err := persistGentleAIPath(filepath.Join(home, "quoted' bin"))
			if shell == "fish" {
				if changed || err == nil {
					t.Fatalf("changed = %t, err = %v", changed, err)
				}
				return
			}
			if err != nil || !changed {
				t.Fatalf("changed = %t, err = %v", changed, err)
			}
			file := ".profile"
			if shell == "bash" {
				file = ".bash_login"
			} else if shell == "zsh" {
				file = ".zshrc"
			}
			b, err := os.ReadFile(filepath.Join(home, file))
			if err != nil || !strings.Contains(string(b), `quoted'"'"' bin`) {
				t.Fatalf("quoted PATH = %q, %v", b, err)
			}
			if shell == "bash" {
				info, err := os.Stat(filepath.Join(home, file))
				if err != nil || info.Mode().Perm() != 0o600 {
					t.Fatalf("existing permissions changed: %v, %v", info, err)
				}
			}
		})
	}
}

// A saved Go environment must never be trusted by the qualified build.
func gentleAIHostileGoConfig(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "go-env")
	settings := "GONOSUMDB=*\nGOPRIVATE=*\nGONOPROXY=*\nGOSUMDB=off\nGOPROXY=http://untrusted.invalid,direct\nGOINSECURE=*\nGOFLAGS=-toolexec=/bin/true\nGOTOOLCHAIN=auto\n"
	if err := os.WriteFile(path, []byte(settings), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOENV", path)
	return path
}

func gentleAICaptureGoEnvironments(t *testing.T, p GentleAIProvisioner, pin manifest.GentleAI, dir string) [][]string {
	t.Helper()
	var calls [][]string
	p.Run = func(_ context.Context, env []string, name string, args ...string) ([]byte, error) {
		if name != "go" {
			t.Fatalf("unexpected command: %s %v", name, args)
		}
		calls = append(calls, append([]string(nil), env...))
		if reflect.DeepEqual(args, []string{"version"}) {
			return []byte("go version go1.25.10 windows/amd64"), nil
		}
		if !reflect.DeepEqual(args, []string{"install", pin.Module + "@" + pin.Version}) {
			t.Fatalf("unexpected arguments: %v", args)
		}
		for _, e := range env {
			if strings.HasPrefix(e, "GOBIN=") {
				return nil, os.WriteFile(filepath.Join(strings.TrimPrefix(e, "GOBIN="), "gentle-ai.exe"), []byte("built"), 0o755)
			}
		}
		t.Fatal("missing private GOBIN")
		return nil, nil
	}
	if _, err := p.buildWindows(context.Background(), pin, dir); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 {
		t.Fatalf("Go invocations = %d, want 2", len(calls))
	}
	return calls
}

func TestGentleAIWindows_SavedGoConfigCannotBypassIntegrity(t *testing.T) {
	gentleAIHostileGoConfig(t)
	p, pin, dir := gentleAIFixture(t, "windows_amd64", nil)
	base := os.Environ()
	// Include case variants to exercise Windows's case-insensitive environment.
	base = append(base, "goenv=hostile", "gonosumdb=*", "goproxy=http://hostile.invalid", "gobin=/unowned")
	original := append([]string(nil), base...)
	p.Environ = func() []string { return base }
	calls := gentleAICaptureGoEnvironments(t, p, pin, dir)
	want := map[string]string{
		"GOENV": "off", "GOFLAGS": "", "GOSUMDB": "sum.golang.org",
		"GONOSUMDB": "none", "GOPRIVATE": "none", "GONOPROXY": "none",
		"GOINSECURE": "", "GOPROXY": "https://proxy.golang.org", "GOTOOLCHAIN": "local",
		"GOOS": "windows", "GOARCH": "amd64", "GENTLE_AI_NO_SELF_UPDATE": "1",
	}
	for key, value := range want {
		var matches []string
		for _, e := range calls[1] {
			k, _, _ := strings.Cut(e, "=")
			if strings.EqualFold(k, key) {
				matches = append(matches, e)
			}
		}
		if !reflect.DeepEqual(matches, []string{key + "=" + value}) {
			t.Errorf("install env %s = %q, want exactly %q", key, matches, key+"="+value)
		}
	}
	for _, e := range calls[1] {
		if strings.HasPrefix(e, "GOBIN=") {
			stage := strings.TrimPrefix(e, "GOBIN=")
			if filepath.Dir(stage) != dir || !strings.HasPrefix(filepath.Base(stage), ".gentle-ai-build-") {
				t.Errorf("GOBIN is not private staging: %q", stage)
			}
			if _, err := os.Stat(stage); !os.IsNotExist(err) {
				t.Errorf("staging not cleaned: %v", err)
			}
		}
	}
	if !reflect.DeepEqual(base, original) {
		t.Fatal("ambient environment mutated")
	}
}

func TestGentleAIWindows_HostileProcessGOFLAGS(t *testing.T) {
	gentleAIHostileGoConfig(t)
	t.Setenv("GOFLAGS", "-toolexec=/bin/true")
	p, pin, dir := gentleAIFixture(t, "windows_amd64", nil)
	p.Environ = os.Environ
	calls := gentleAICaptureGoEnvironments(t, p, pin, dir)
	for i, env := range calls {
		if !containsEnv(env, "GOFLAGS=") || containsEnv(env, "GOFLAGS=-toolexec=/bin/true") {
			t.Errorf("invocation %d inherited hostile GOFLAGS instead of clearing it", i)
		}
	}
}

func TestGentleAIWindows_ToolchainPolicyBeforeVersion(t *testing.T) {
	p, pin, dir := gentleAIFixture(t, "windows_amd64", nil)
	p.Environ = func() []string { return []string{"GOTOOLCHAIN=auto"} }
	calls := gentleAICaptureGoEnvironments(t, p, pin, dir)
	if !containsEnv(calls[0], "GOTOOLCHAIN=local") {
		t.Error("go version ran without GOTOOLCHAIN=local")
	}
	if !reflect.DeepEqual(calls[0], calls[1]) {
		t.Error("go version and go install received different controlled environments")
	}
}

func TestGentleAIWindows_RealGoEnvIgnoresHostileSavedConfig(t *testing.T) {
	if testing.Short() {
		t.Skip("real Go subprocess")
	}
	goPath, err := exec.LookPath("go")
	if err != nil {
		t.Fatal(err)
	}
	gentleAIHostileGoConfig(t)
	// Empty process values previously fell back to the hostile saved configuration.
	for _, key := range []string{"GOFLAGS", "GONOSUMDB", "GOPRIVATE", "GONOPROXY", "GOINSECURE"} {
		t.Setenv(key, "")
	}
	p, pin, dir := gentleAIFixture(t, "windows_amd64", nil)
	p.Environ = os.Environ
	keys := []string{"GOENV", "GOFLAGS", "GOSUMDB", "GONOSUMDB", "GOPRIVATE", "GONOPROXY", "GOINSECURE", "GOPROXY", "GOTOOLCHAIN", "GOBIN"}
	calls := 0
	p.Run = func(ctx context.Context, env []string, _ string, args ...string) ([]byte, error) {
		calls++
		cmd := exec.CommandContext(ctx, goPath, append([]string{"env"}, keys...)...)
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("real go env: %v: %s", err, out)
		}
		values := strings.Split(strings.TrimSuffix(strings.ReplaceAll(string(out), "\r\n", "\n"), "\n"), "\n")
		// go env reports an empty config path when GOENV=off disables the file.
		want := []string{"", "", "sum.golang.org", "none", "none", "none", "", "https://proxy.golang.org", "local"}
		if len(values) != len(keys) {
			t.Fatalf("go env output: %q", out)
		}
		for i, value := range want {
			if values[i] != value {
				t.Errorf("invocation %s real go env %s = %q, want %q", args[0], keys[i], values[i], value)
			}
		}
		if args[0] == "version" {
			return []byte("go version go1.25.10 windows/amd64"), nil
		}
		return nil, os.WriteFile(filepath.Join(values[len(keys)-1], "gentle-ai.exe"), []byte("built"), 0o755)
	}
	if _, err := p.buildWindows(context.Background(), pin, dir); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("real go env invocations = %d, want 2", calls)
	}
}

type gentleAIRoundTripper func(*http.Request) (*http.Response, error)

func (f gentleAIRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestGentleAIFetch_HTTPSOnly(t *testing.T) {
	for _, tt := range []struct {
		name, initial, location, want string
		wantCalls                     int
	}{
		{name: "initial HTTP", initial: "http://release.invalid/asset", want: "HTTPS", wantCalls: 0},
		{name: "redirect downgrade", initial: "https://release.invalid/asset", location: "http://cdn.invalid/asset", want: "HTTPS", wantCalls: 1},
		{name: "redirect loop", initial: "https://release.invalid/asset", location: "https://release.invalid/asset", want: "redirecciones", wantCalls: 10},
		{name: "HTTPS redirect", initial: "https://release.invalid/asset", location: "https://cdn.invalid/asset", wantCalls: 2},
	} {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			old := http.DefaultTransport
			t.Cleanup(func() { http.DefaultTransport = old })
			http.DefaultTransport = gentleAIRoundTripper(func(req *http.Request) (*http.Response, error) {
				calls++
				if calls > 12 {
					return nil, errors.New("unbounded redirects")
				}
				response := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("archive")), Request: req}
				if tt.location != "" && (calls == 1 || tt.name == "redirect loop") {
					response.StatusCode = http.StatusFound
					response.Header.Set("Location", tt.location)
				}
				return response, nil
			})
			body, err := fetchGentleAIAsset(context.Background(), tt.initial)
			if body != nil {
				defer body.Close()
			}
			if tt.want != "" {
				if err == nil || !strings.Contains(err.Error(), tt.want) || body != nil {
					t.Errorf("fetch error = %v, body = %v, want rejection containing %q", err, body, tt.want)
				}
			} else if err != nil {
				t.Errorf("HTTPS redirect failed: %v", err)
			}
			if calls != tt.wantCalls {
				t.Errorf("transport calls = %d, want %d", calls, tt.wantCalls)
			}
		})
	}
}

func TestGentleAIGoToolchainVersions(t *testing.T) {
	for _, tt := range []struct {
		name, output string
		want         bool
	}{
		{name: "too old", output: "go version go1.25.9 windows/amd64"},
		{name: "minimum", output: "go version go1.25.10 windows/amd64", want: true},
		{name: "invalid patch RC", output: "go version go1.25.10rc1 windows/amd64"},
		{name: "newer RC", output: "go version go1.26rc1 windows/amd64", want: true},
		{name: "newer beta", output: "go version go1.26beta1 windows/amd64", want: true},
		{name: "devel", output: "go version devel go1.27-abcdef windows/amd64"},
		{name: "unidentifiable", output: "unknown"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := gentleAIGoVersionSupported(tt.output); got != tt.want {
				t.Errorf("supported(%q) = %t, want %t", tt.output, got, tt.want)
			}
		})
	}
}

func TestGentleAIWindows_UnsupportedToolchainDiagnostic(t *testing.T) {
	for _, output := range []string{"go version devel go1.27-abcdef windows/amd64", "unidentifiable", "go version go1.25.10rc1 windows/amd64"} {
		t.Run(output, func(t *testing.T) {
			p, pin, dir := gentleAIFixture(t, "windows_amd64", nil)
			p.Run = func(_ context.Context, _ []string, _ string, args ...string) ([]byte, error) {
				if !reflect.DeepEqual(args, []string{"version"}) {
					t.Fatal("unsupported toolchain attempted install")
				}
				return []byte(output), nil
			}
			_, err := p.buildWindows(context.Background(), pin, dir)
			if err == nil || !strings.Contains(err.Error(), "no compatible o no identificable") || !strings.Contains(err.Error(), pin.Module+"@"+pin.Version) {
				t.Errorf("unsupported-toolchain diagnostic = %v", err)
			}
		})
	}
}

type gentleAIZeroReader struct{}

func (gentleAIZeroReader) Read(p []byte) (int, error) {
	clear(p)
	return len(p), nil
}

func TestGentleAIExtraction_DecompressedBudget(t *testing.T) {
	for _, tt := range []struct {
		name     string
		skipped  []int64
		trailing int64
	}{
		{name: "oversized skipped entry", skipped: []int64{gentleAIMaxBytes + 1}},
		{name: "multiple skipped entries share budget", skipped: []int64{gentleAIMaxBytes / 2, gentleAIMaxBytes / 2}},
		{name: "oversized trailing data", trailing: gentleAIMaxBytes + 1},
		{name: "tar and footer drain share budget", skipped: []int64{gentleAIMaxBytes / 2}, trailing: gentleAIMaxBytes / 2},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var archive bytes.Buffer
			gz := gzip.NewWriter(&archive)
			tw := tar.NewWriter(gz)
			for i, size := range tt.skipped {
				if err := tw.WriteHeader(&tar.Header{Name: fmt.Sprintf("ignored-%d", i), Size: size}); err != nil {
					t.Fatal(err)
				}
				if _, err := io.CopyN(tw, gentleAIZeroReader{}, size); err != nil {
					t.Fatal(err)
				}
			}
			if err := tw.WriteHeader(&tar.Header{Name: "gentle-ai", Size: 1}); err != nil {
				t.Fatal(err)
			}
			if _, err := tw.Write([]byte("x")); err != nil {
				t.Fatal(err)
			}
			if err := tw.Close(); err != nil {
				t.Fatal(err)
			}
			if _, err := io.CopyN(gz, gentleAIZeroReader{}, tt.trailing); err != nil {
				t.Fatal(err)
			}
			if err := gz.Close(); err != nil {
				t.Fatal(err)
			}
			binary, err := extractGentleAIBinary(archive.Bytes())
			if err == nil || !strings.Contains(err.Error(), "archivo descomprimido demasiado grande") || binary != nil {
				t.Errorf("extraction = %q, %v; want explicit decompressed-budget rejection", binary, err)
			}
		})
	}
}
