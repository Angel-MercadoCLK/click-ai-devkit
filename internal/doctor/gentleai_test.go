package doctor

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/installer"
	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/manifest"
)

func TestRun_ChecksOnlyManagedGentleAIAgainstQualifiedPin(t *testing.T) {
	pin, err := manifest.Load()
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name            string
		file            func(t *testing.T, path string)
		output          []byte
		runErr          error
		lstatErr        error
		waitForDeadline bool
		wantHealth      bool
		wantDetail      string
		wantProbe       int
	}{
		{
			name:       "matching managed pin",
			file:       executableFile,
			output:     []byte("gentle-ai " + pin.GentleAI.Version),
			wantHealth: true,
			wantDetail: "coincide con el pin calificado",
			wantProbe:  1,
		},
		{
			name:       "missing managed binary",
			wantDetail: "no está instalado",
		},
		{
			name:       "older version",
			file:       executableFile,
			output:     []byte("gentle-ai v3.9.0"),
			wantDetail: "más antigua",
			wantProbe:  1,
		},
		{
			name:       "newer version",
			file:       executableFile,
			output:     []byte("gentle-ai v4.1.0"),
			wantDetail: "más reciente",
			wantProbe:  1,
		},
		{
			name:       "prerelease mismatch",
			file:       executableFile,
			output:     []byte("gentle-ai v4.0.0-rc.1"),
			wantDetail: "versión preliminar",
			wantProbe:  1,
		},
		{
			name:       "invalid version output",
			file:       executableFile,
			output:     []byte("token-secret invalid output"),
			wantDetail: "salida de versión no válida",
			wantProbe:  1,
		},
		{
			name:       "probe error is sanitized",
			file:       executableFile,
			runErr:     errors.New("credential-secret subprocess failure"),
			wantDetail: "no se pudo consultar la versión",
			wantProbe:  1,
		},
		{
			name:       "probe timeout is actionable",
			file:       executableFile,
			runErr:     context.DeadlineExceeded,
			wantDetail: "superó el tiempo límite",
			wantProbe:  1,
		},
		{
			name:            "killed probe reports elapsed deadline",
			file:            executableFile,
			waitForDeadline: true,
			wantDetail:      "superó el tiempo límite",
			wantProbe:       1,
		},
		{
			name:       "managed path inspection error is sanitized",
			lstatErr:   errors.New("credential-secret stat failure"),
			wantDetail: "no se pudo inspeccionar el directorio administrado",
		},
		{
			name: "non-regular managed path",
			file: func(t *testing.T, path string) {
				t.Helper()
				if err := os.Mkdir(path, 0o755); err != nil {
					t.Fatal(err)
				}
			},
			wantDetail: "no es un archivo regular ejecutable",
		},
		{
			name: "managed symlink is rejected",
			file: func(t *testing.T, path string) {
				t.Helper()
				other := filepath.Join(filepath.Dir(path), "outside-gentle-ai")
				if err := os.WriteFile(other, []byte("not executed"), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(other, path); err != nil {
					t.Skipf("symlinks unavailable: %v", err)
				}
			},
			wantDetail: "no es un archivo regular ejecutable",
		},
		{
			name: "click directory symlink is rejected",
			file: func(t *testing.T, path string) {
				t.Helper()
				clickDir := filepath.Dir(filepath.Dir(path))
				redirect := filepath.Join(filepath.Dir(clickDir), "redirected-click")
				if err := os.MkdirAll(filepath.Join(redirect, "bin"), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(redirect, "bin", filepath.Base(path)), []byte("not executed"), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.RemoveAll(clickDir); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(redirect, clickDir); err != nil {
					t.Skipf("symlinks unavailable: %v", err)
				}
			},
			wantDetail: "ruta administrada de gentle-ai contiene un directorio inseguro",
		},
		{
			name: "bin directory symlink is rejected",
			file: func(t *testing.T, path string) {
				t.Helper()
				binDir := filepath.Dir(path)
				redirect := filepath.Join(filepath.Dir(filepath.Dir(binDir)), "redirected-bin")
				if err := os.MkdirAll(redirect, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(redirect, filepath.Base(path)), []byte("not executed"), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.RemoveAll(binDir); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(redirect, binDir); err != nil {
					t.Skipf("symlinks unavailable: %v", err)
				}
			},
			wantDetail: "ruta administrada de gentle-ai contiene un directorio inseguro",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			p := installer.NewGentleAIProvisioner()
			p.HomeDir = func() (string, error) { return home, nil }
			path, err := p.ManagedPath()
			if err != nil {
				t.Fatal(err)
			}
			if tt.file != nil {
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				tt.file(t, path)
			}
			calls := 0
			if tt.lstatErr != nil {
				p.Lstat = func(string) (os.FileInfo, error) { return nil, tt.lstatErr }
			}
			p.Environ = func() []string { return []string{"PATH=/ambient/must-not-be-used", "GENTLE_AI_NO_SELF_UPDATE=0"} }
			p.Run = func(ctx context.Context, env []string, name string, args ...string) ([]byte, error) {
				calls++
				if _, ok := ctx.Deadline(); !ok {
					t.Error("version probe context has no deadline")
				}
				if name != path || len(args) != 1 || args[0] != "version" {
					t.Fatalf("probe command = %q %q, want managed absolute path and version", name, args)
				}
				if !containsEnv(env, "GENTLE_AI_NO_SELF_UPDATE=1") {
					t.Fatalf("probe environment lacks NO_SELF_UPDATE=1: %v", env)
				}
				if containsEnv(env, "GENTLE_AI_NO_SELF_UPDATE=0") {
					t.Fatalf("probe environment retained caller override: %v", env)
				}
				if tt.waitForDeadline {
					<-ctx.Done()
					return nil, errors.New("signal: killed")
				}
				return tt.output, tt.runErr
			}
			p.Fetch = func(context.Context, string) (io.ReadCloser, error) {
				t.Fatal("doctor must not fetch")
				return nil, nil
			}
			p.WriteBinary = func(string, []byte) error { t.Fatal("doctor must not write a binary"); return nil }
			p.EnsureOnPath = func(string) (bool, error) { t.Fatal("doctor must not alter PATH"); return false, nil }
			restore := SetManagedGentleAIProvisionerFactoryForTests(func() installer.GentleAIProvisioner { return p })
			t.Cleanup(restore)

			report := Run(installer.Config{ClickStateHome: t.TempDir()})
			check := findCheck(t, report, "gentle-ai administrado")
			if check.Healthy != tt.wantHealth {
				t.Errorf("Healthy = %v, want %v; detail: %s", check.Healthy, tt.wantHealth, check.Detail)
			}
			if !strings.Contains(check.Detail, tt.wantDetail) {
				t.Errorf("detail = %q, want substring %q", check.Detail, tt.wantDetail)
			}
			if strings.Contains(check.Detail, "secret") {
				t.Errorf("detail leaked untrusted subprocess data: %q", check.Detail)
			}
			if calls != tt.wantProbe {
				t.Errorf("probe calls = %d, want %d", calls, tt.wantProbe)
			}
		})
	}
}

func TestRun_ManagedGentleAICheckIsUniqueAcrossTargetSelections(t *testing.T) {
	for _, targets := range []struct {
		name string
		cfg  func() installer.Config
	}{
		{name: "no targets", cfg: func() installer.Config { return installer.Config{ClickStateHome: t.TempDir()} }},
		{name: "Claude", cfg: func() installer.Config { return installer.Config{ClaudeHome: t.TempDir(), ClickStateHome: t.TempDir()} }},
		{name: "Codex", cfg: func() installer.Config { return installer.Config{CodexHome: t.TempDir(), ClickStateHome: t.TempDir()} }},
		{name: "OpenClaw", cfg: func() installer.Config {
			return installer.Config{OpenClawHome: t.TempDir(), ClickStateHome: t.TempDir()}
		}},
		{name: "all targets", cfg: func() installer.Config {
			return installer.Config{ClaudeHome: t.TempDir(), CodexHome: t.TempDir(), OpenClawHome: t.TempDir(), ClickStateHome: t.TempDir()}
		}},
	} {
		t.Run(targets.name, func(t *testing.T) {
			cfg := targets.cfg()
			report := Run(cfg)
			if count := countChecks(report, "gentle-ai administrado"); count != 1 {
				t.Errorf("managed gentle-ai checks = %d, want 1", count)
			}
			if count := countChecks(report, "openclaw"); count != 1 {
				t.Errorf("OpenClaw checks = %d, want one diagnostic with no duplicate selected-target report", count)
			}
		})
	}
}

func executableFile(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("not executed"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func findCheck(t *testing.T, report Report, name string) CheckResult {
	t.Helper()
	for _, check := range report.Checks {
		if check.Name == name {
			return check
		}
	}
	t.Fatalf("missing check %q in %+v", name, report.Checks)
	return CheckResult{}
}

func countChecks(report Report, name string) int {
	count := 0
	for _, check := range report.Checks {
		if check.Name == name {
			count++
		}
	}
	return count
}

func containsEnv(env []string, want string) bool {
	for _, item := range env {
		if item == want {
			return true
		}
	}
	return false
}
