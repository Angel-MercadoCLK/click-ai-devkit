package installer

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestGentleAIInstallArgs_SelectedTargets(t *testing.T) {
	for _, tt := range []struct {
		selection TargetSelection
		agents    string
	}{
		{TargetSelection{Claude: true}, "claude-code"},
		{TargetSelection{Codex: true}, "codex"},
		{TargetSelection{OpenClaw: true}, "openclaw"},
		{TargetSelection{Claude: true, Codex: true}, "claude-code,codex"},
		{TargetSelection{Claude: true, OpenClaw: true}, "claude-code,openclaw"},
		{TargetSelection{Codex: true, OpenClaw: true}, "codex,openclaw"},
		{TargetSelection{Claude: true, Codex: true, OpenClaw: true}, "claude-code,codex,openclaw"},
	} {
		t.Run(tt.agents, func(t *testing.T) {
			want := []string{"install", "--agents", tt.agents, "--preset", "full-gentleman", "--components", "skills,permissions,persona", "--persona", "neutral", "--scope", "global"}
			if got := GentleAIInstallArgs(tt.selection); !reflect.DeepEqual(got, want) {
				t.Fatalf("argv = %q, want %q", got, want)
			}
		})
	}
	if got := GentleAIInstallArgs(TargetSelection{}); len(got) != 0 {
		t.Fatalf("empty selection argv = %q", got)
	}
}

func TestGentleAIInstall_PinDecisionAndEnvironment(t *testing.T) {
	for _, tt := range []struct {
		name, installed, message string
		provisions               int
	}{
		{"matching", "v4.0.0", "coincide", 0},
		{"older", "v3.5.0", "v3.5.0 → v4.0.0", 1},
		{"newer", "v4.1.0", "v4.1.0 → v4.0.0", 1},
		{"missing", "", "no está instalado", 1},
		{"invalid probe", "invalid", "no se pudo verificar", 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			archive := gentleAIArchive(t, []*tar.Header{{Name: "gentle-ai", Size: 6, Mode: 0o755}})
			p, pin, dir := gentleAIFixture(t, "linux_amd64", archive)
			t.Setenv("CLICK_CLAUDE_HOME", t.TempDir())
			path := filepath.Join(dir, "gentle-ai")
			p.Lstat = func(got string) (os.FileInfo, error) {
				if got != path {
					t.Fatalf("stat path = %q, want %q", got, path)
				}
				if tt.installed == "" {
					return nil, os.ErrNotExist
				}
				return gentleAIFileInfo{mode: 0o755}, nil
			}
			downloads, writes, pathAttempts, installs, probes := 0, 0, 0, 0, 0
			fetch, write := p.Fetch, p.WriteBinary
			p.Fetch = func(ctx context.Context, url string) (io.ReadCloser, error) { downloads++; return fetch(ctx, url) }
			p.WriteBinary = func(path string, data []byte) error { writes++; return write(path, data) }
			p.EnsureOnPath = func(string) (bool, error) { pathAttempts++; return true, nil }
			p.Run = func(_ context.Context, env []string, name string, args ...string) ([]byte, error) {
				if name != path {
					t.Fatalf("executed %q, want managed %q", name, path)
				}
				if !sliceContains(env, "GENTLE_AI_NO_SELF_UPDATE=1") || sliceContains(env, "GENTLE_AI_NO_SELF_UPDATE=0") {
					t.Fatalf("self-update not suppressed: %q", env)
				}
				if args[0] == "version" {
					probes++
					return []byte("gentle-ai " + tt.installed), nil
				}
				installs++
				if !reflect.DeepEqual(args, GentleAIInstallArgs(TargetSelection{Codex: true})) {
					t.Fatalf("install argv = %q", args)
				}
				return nil, nil
			}
			var messages []string
			result, err := p.Install(context.Background(), pin, TargetSelection{Codex: true}, func(notice GentleAIInstallNotice) { messages = append(messages, notice.Message) })
			if err != nil {
				t.Fatal(err)
			}
			if result.Path != path || downloads != tt.provisions || writes != tt.provisions || pathAttempts != 1 || installs != 1 {
				t.Fatalf("result=%+v downloads=%d writes=%d PATH attempts=%d installs=%d", result, downloads, writes, pathAttempts, installs)
			}
			if tt.installed == "" && probes != 0 {
				t.Fatal("probed missing binary")
			}
			if !strings.Contains(strings.Join(messages, "\n"), tt.message) {
				t.Fatalf("messages = %q, want %q", messages, tt.message)
			}
			if (tt.name == "older" || tt.name == "newer") && !strings.Contains(strings.Join(messages, "\n"), "puede degradar") {
				t.Fatalf("missing downgrade warning: %q", messages)
			}
		})
	}
}

func TestGentleAIInstall_FailuresStopAndDoNotLeakOutput(t *testing.T) {
	for _, tt := range []struct {
		name      string
		provision bool
	}{{"provision", true}, {"install", false}} {
		t.Run(tt.name, func(t *testing.T) {
			archive := gentleAIArchive(t, []*tar.Header{{Name: "gentle-ai", Size: 6, Mode: 0o755}})
			p, pin, _ := gentleAIFixture(t, "linux_amd64", archive)
			p.EnsureOnPath = func(string) (bool, error) { return false, errors.New("path warning") }
			installs := 0
			if tt.provision {
				p.WriteBinary = func(string, []byte) error { return errors.New("secret-token") }
			}
			p.Run = func(context.Context, []string, string, ...string) ([]byte, error) {
				installs++
				return []byte("secret-token"), errors.New("secret-token")
			}
			_, err := p.Install(context.Background(), pin, TargetSelection{Claude: true}, func(GentleAIInstallNotice) {})
			if err == nil || !strings.Contains(err.Error(), "click install") || !strings.Contains(err.Error(), "click update") || strings.Contains(err.Error(), "secret-token") {
				t.Fatalf("error = %v, want safe Spanish remediation", err)
			}
			if tt.provision && installs != 0 {
				t.Fatal("ran install after provisioning failure")
			}
		})
	}
}

func TestGentleAIInstall_EmptySelectionHasNoSideEffects(t *testing.T) {
	p, pin, _ := gentleAIFixture(t, "linux_amd64", nil)
	p.HomeDir = func() (string, error) { t.Fatal("resolved home for empty selection"); return "", nil }
	if _, err := p.Install(context.Background(), pin, TargetSelection{}, nil); err != nil {
		t.Fatal(err)
	}
}

func TestGentleAIInstall_FailuresPreserveSanitizedCause(t *testing.T) {
	sentinel := errors.New("stderr: secret-token; ENGRAM_CLOUD_TOKEN=private-env-value")
	exitErr := &exec.ExitError{Stderr: []byte("secret-token private-env-value")}
	for _, stage := range []string{"provision", "install"} {
		for _, tt := range []struct {
			name    string
			cause   error
			summary string
		}{
			{"unknown", sentinel, "error no clasificado"},
			{"exit", exitErr, "código de salida -1"},
			{"permission", os.ErrPermission, "permiso denegado"},
			{"deadline", context.DeadlineExceeded, "tiempo de espera agotado"},
		} {
			t.Run(stage+"/"+tt.name, func(t *testing.T) {
				archive := gentleAIArchive(t, []*tar.Header{{Name: "gentle-ai", Size: 6, Mode: 0o755}})
				p, pin, _ := gentleAIFixture(t, "linux_amd64", archive)
				t.Setenv("CLICK_CLAUDE_HOME", t.TempDir())
				p.Environ = func() []string { return []string{"ENGRAM_CLOUD_TOKEN=private-env-value"} }
				p.EnsureOnPath = func(string) (bool, error) { return false, nil }
				injected := fmt.Errorf("stderr: secret-token; ENGRAM_CLOUD_TOKEN=private-env-value: %w", tt.cause)
				if stage == "provision" {
					p.WriteBinary = func(string, []byte) error { return injected }
				}
				p.Run = func(context.Context, []string, string, ...string) ([]byte, error) {
					return []byte("stdout/stderr: secret-token private-env-value"), injected
				}
				_, err := p.Install(context.Background(), pin, TargetSelection{Claude: true}, nil)
				if !errors.Is(err, tt.cause) {
					t.Error("underlying cause was discarded")
				}
				if tt.cause == exitErr {
					var got *exec.ExitError
					if !errors.As(err, &got) || got != exitErr {
						t.Error("underlying exit error is not reachable via errors.As")
					}
				}
				if err == nil {
					t.Fatal("expected install failure")
				}
				message := err.Error()
				for _, secret := range []string{"secret-token", "private-env-value", "ENGRAM_CLOUD_TOKEN", "stdout", "stderr"} {
					if strings.Contains(message, secret) {
						t.Error("rendered error leaked injected subprocess/environment content")
					}
				}
				if len(message) > 600 || !strings.HasPrefix(message, "No se pudo ") || !strings.Contains(message, "Revise ") || !strings.Contains(message, tt.summary) {
					t.Errorf("expected bounded Spanish remediation with sanitized summary %q", tt.summary)
				}
			})
		}
	}
}

// Simulated platforms must not inherit executable bits from the host filesystem.
type gentleAIFileInfo struct {
	os.FileInfo
	mode os.FileMode
}

func (f gentleAIFileInfo) Mode() os.FileMode { return f.mode }

func TestGentleAIInstall_SimulatedWindowsMatchingPin(t *testing.T) {
	p, pin, dir := gentleAIFixture(t, "windows_amd64", nil)
	path := filepath.Join(dir, "gentle-ai.exe")
	p.Lstat = func(got string) (os.FileInfo, error) {
		if got != path {
			t.Fatalf("stat path = %q, want .exe path %q", got, path)
		}
		return gentleAIFileInfo{mode: 0o666}, nil
	}
	p.WriteBinary = func(string, []byte) error { t.Fatal("provisioned matching pin"); return nil }
	p.EnsureOnPath = func(string) (bool, error) { return false, nil }
	probes, installs := 0, 0
	p.Run = func(_ context.Context, _ []string, name string, args ...string) ([]byte, error) {
		if name != path {
			t.Fatalf("command path = %q, want %q", name, path)
		}
		if args[0] == "version" {
			probes++
			return []byte("gentle-ai v4.0.0"), nil
		}
		installs++
		return nil, nil
	}
	result, err := p.Install(context.Background(), pin, TargetSelection{Codex: true}, nil)
	if err != nil || result.Path != path || probes != 1 || installs != 1 {
		t.Fatalf("result=%+v error=%v probes=%d installs=%d", result, err, probes, installs)
	}
}

func TestGentleAIInstall_MatchingPinReconcilesPathNonFatally(t *testing.T) {
	for _, pathFailure := range []bool{false, true} {
		t.Run(fmt.Sprintf("PATH-failure=%t", pathFailure), func(t *testing.T) {
			p, pin, dir := gentleAIFixture(t, "linux_amd64", nil)
			p.Lstat = func(string) (os.FileInfo, error) { return gentleAIFileInfo{mode: 0o755}, nil }
			downloads, writes, attempts, installs := 0, 0, 0, 0
			p.Fetch = func(context.Context, string) (io.ReadCloser, error) {
				downloads++
				return nil, errors.New("unexpected download")
			}
			p.WriteBinary = func(string, []byte) error { writes++; return errors.New("unexpected write") }
			p.EnsureOnPath = func(got string) (bool, error) {
				attempts++
				if got != dir {
					t.Fatalf("PATH dir = %q, want %q", got, dir)
				}
				if pathFailure {
					return false, errors.New("fixture PATH failure")
				}
				return true, nil
			}
			p.Run = func(_ context.Context, _ []string, _ string, args ...string) ([]byte, error) {
				if args[0] == "version" {
					return []byte("gentle-ai v4.0.0"), nil
				}
				installs++
				return nil, nil
			}
			result, err := p.Install(context.Background(), pin, TargetSelection{Codex: true}, nil)
			if err != nil || downloads != 0 || writes != 0 || attempts != 1 || installs != 1 {
				t.Fatalf("error=%v downloads=%d writes=%d PATH attempts=%d installs=%d", err, downloads, writes, attempts, installs)
			}
			if result.PathChanged != !pathFailure || (result.PathWarning != "") != pathFailure {
				t.Fatalf("PATH result = %+v", result)
			}
		})
	}
}
