package installer

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/manifest"
)

// fakeEngramCloudRunner records commands and can fail on a specific call index.
type fakeEngramCloudRunner struct {
	commands []commandInvocation
	failAt   int
	calls    int
	failWith error
}

func newFakeEngramCloudRunner() *fakeEngramCloudRunner {
	return &fakeEngramCloudRunner{failAt: -1, failWith: errors.New("engram cloud command failed")}
}

func (f *fakeEngramCloudRunner) Run(name string, args ...string) error {
	f.commands = append(f.commands, commandInvocation{Name: name, Args: append([]string(nil), args...)})
	defer func() { f.calls++ }()
	if f.failAt >= 0 && f.calls == f.failAt {
		return f.failWith
	}
	return nil
}

func (f *fakeEngramCloudRunner) Output(name string, args ...string) ([]byte, error) {
	f.commands = append(f.commands, commandInvocation{Name: name, Args: append([]string(nil), args...)})
	defer func() { f.calls++ }()
	if f.failAt >= 0 && f.calls == f.failAt {
		return nil, f.failWith
	}
	return nil, nil
}

func assertCommands(t *testing.T, got, want []commandInvocation) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("commands = %d, want %d: %+v", len(got), len(want), got)
	}
	for i := range got {
		if got[i].Name != want[i].Name || len(got[i].Args) != len(want[i].Args) {
			t.Errorf("command[%d] = %+v, want %+v", i, got[i], want[i])
			continue
		}
		for j := range got[i].Args {
			if got[i].Args[j] != want[i].Args[j] {
				t.Errorf("command[%d].Args[%d] = %q, want %q", i, j, got[i].Args[j], want[i].Args[j])
			}
		}
	}
}

func assertEngramCloudState(t *testing.T, path, wantServer, wantProject string, wantEnrolled bool) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(state) error = %v", err)
	}
	var state engramCloudState
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatalf("Unmarshal(state) error = %v", err)
	}
	if state.Enrolled != wantEnrolled {
		t.Errorf("state.Enrolled = %v, want %v", state.Enrolled, wantEnrolled)
	}
	if state.Server != wantServer {
		t.Errorf("state.Server = %q, want %q", state.Server, wantServer)
	}
	if state.Project != wantProject {
		t.Errorf("state.Project = %q, want %q", state.Project, wantProject)
	}
	if wantEnrolled && state.LastSync == "" {
		t.Errorf("state.LastSync is empty, want a timestamp")
	}
}

func TestResolveEngramCloudConfig(t *testing.T) {
	tests := []struct {
		name        string
		env         map[string]string
		manifest    manifest.EngramCloud
		wantServer  string
		wantProject string
		wantToken   bool
	}{
		{"all absent", map[string]string{}, manifest.EngramCloud{}, "", "", false},
		{"manifest defaults used when token present", map[string]string{"ENGRAM_CLOUD_TOKEN": "tok"}, manifest.EngramCloud{Server: "http://127.0.0.1:18080", Project: "click-ai-devkit"}, "http://127.0.0.1:18080", "click-ai-devkit", true},
		{"env overrides manifest", map[string]string{"ENGRAM_CLOUD_TOKEN": "tok", "CLICK_ENGRAM_CLOUD_SERVER": "http://override.example", "CLICK_ENGRAM_CLOUD_PROJECT": "override-project"}, manifest.EngramCloud{Server: "http://127.0.0.1:18080", Project: "click-ai-devkit"}, "http://override.example", "override-project", true},
		{"partial config without token still resolves server/project", map[string]string{}, manifest.EngramCloud{Server: "http://127.0.0.1:18080", Project: "click-ai-devkit"}, "http://127.0.0.1:18080", "click-ai-devkit", false},
		{"token-only without server/project", map[string]string{"ENGRAM_CLOUD_TOKEN": "tok"}, manifest.EngramCloud{}, "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			server, project, tokenPresent := resolveEngramCloudConfig(Config{}, &manifest.Manifest{EngramCloud: tt.manifest})
			if server != tt.wantServer || project != tt.wantProject || tokenPresent != tt.wantToken {
				t.Errorf("resolveEngramCloudConfig() = (%q, %q, %v), want (%q, %q, %v)", server, project, tokenPresent, tt.wantServer, tt.wantProject, tt.wantToken)
			}
		})
	}
}

func TestEngramCloudPartiallyConfigured(t *testing.T) {
	tests := []struct {
		name     string
		env      map[string]string
		manifest manifest.EngramCloud
		want     bool
	}{
		{"all absent", map[string]string{}, manifest.EngramCloud{}, false},
		{"server and project with token present", map[string]string{"ENGRAM_CLOUD_TOKEN": "tok"}, manifest.EngramCloud{Server: "http://127.0.0.1:18080", Project: "click-ai-devkit"}, false},
		{"server and project without token", map[string]string{}, manifest.EngramCloud{Server: "http://127.0.0.1:18080", Project: "click-ai-devkit"}, true},
		{"token only", map[string]string{"ENGRAM_CLOUD_TOKEN": "tok"}, manifest.EngramCloud{}, false},
		{"server and token without project", map[string]string{"ENGRAM_CLOUD_TOKEN": "tok"}, manifest.EngramCloud{Server: "http://127.0.0.1:18080"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			if got := EngramCloudPartiallyConfigured(Config{}, &manifest.Manifest{EngramCloud: tt.manifest}); got != tt.want {
				t.Errorf("EngramCloudPartiallyConfigured() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSyncEngramCloud_NoOpWhenIncomplete(t *testing.T) {
	tests := []struct {
		name     string
		env      map[string]string
		manifest manifest.EngramCloud
	}{
		{"all absent", map[string]string{}, manifest.EngramCloud{}},
		{"server and project but no token", map[string]string{}, manifest.EngramCloud{Server: "http://127.0.0.1:18080", Project: "click-ai-devkit"}},
		{"token only", map[string]string{"ENGRAM_CLOUD_TOKEN": "tok"}, manifest.EngramCloud{}},
		{"server and token but no project", map[string]string{"ENGRAM_CLOUD_TOKEN": "tok"}, manifest.EngramCloud{Server: "http://127.0.0.1:18080"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			cfg := Config{ClaudeHome: t.TempDir()}
			runner := newFakeEngramCloudRunner()
			restore := SetCommandRunnerFactoryForTests(func() CommandRunner { return runner })
			defer restore()

			if err := SyncEngramCloud(cfg, &manifest.Manifest{EngramCloud: tt.manifest}); err != nil {
				t.Fatalf("SyncEngramCloud() error = %v, want nil", err)
			}
			if len(runner.commands) != 0 {
				t.Fatalf("SyncEngramCloud() issued commands, want 0: %+v", runner.commands)
			}
			if _, statErr := os.Stat(cfg.EngramCloudStatePath()); !os.IsNotExist(statErr) {
				t.Fatalf("SyncEngramCloud() wrote state file when incomplete")
			}
		})
	}
}

func TestSyncEngramCloud_FirstTimeEnrollment(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir()}
	t.Setenv("ENGRAM_CLOUD_TOKEN", "super-secret-token")

	runner := newFakeEngramCloudRunner()
	restore := SetCommandRunnerFactoryForTests(func() CommandRunner { return runner })
	defer restore()

	m := &manifest.Manifest{EngramCloud: manifest.EngramCloud{Server: "http://127.0.0.1:18080", Project: "click-ai-devkit"}}
	if err := SyncEngramCloud(cfg, m); err != nil {
		t.Fatalf("SyncEngramCloud() error = %v", err)
	}

	want := []commandInvocation{
		{Name: "engram", Args: []string{"cloud", "config", "--server", "http://127.0.0.1:18080"}},
		{Name: "engram", Args: []string{"cloud", "enroll", "click-ai-devkit"}},
		{Name: "engram", Args: []string{"cloud", "upgrade", "doctor"}},
		{Name: "engram", Args: []string{"cloud", "upgrade", "repair"}},
		{Name: "engram", Args: []string{"cloud", "upgrade", "bootstrap"}},
		{Name: "engram", Args: []string{"sync", "--cloud", "--project", "click-ai-devkit"}},
	}
	assertCommands(t, runner.commands, want)
	assertEngramCloudState(t, cfg.EngramCloudStatePath(), "http://127.0.0.1:18080", "click-ai-devkit", true)
}

func TestSyncEngramCloud_FirstTimeEnrollment_FailStop(t *testing.T) {
	steps := []string{"cloud config", "cloud enroll", "cloud upgrade doctor", "cloud upgrade repair", "cloud upgrade bootstrap", "sync"}
	for idx, step := range steps {
		t.Run("fail at "+step, func(t *testing.T) {
			cfg := Config{ClaudeHome: t.TempDir()}
			t.Setenv("ENGRAM_CLOUD_TOKEN", "super-secret-token")

			runner := newFakeEngramCloudRunner()
			runner.failAt = idx
			restore := SetCommandRunnerFactoryForTests(func() CommandRunner { return runner })
			defer restore()

			m := &manifest.Manifest{EngramCloud: manifest.EngramCloud{Server: "http://127.0.0.1:18080", Project: "click-ai-devkit"}}
			if err := SyncEngramCloud(cfg, m); err == nil {
				t.Fatalf("SyncEngramCloud() error = nil, want failure at %s", step)
			}
			if len(runner.commands) != idx+1 {
				t.Fatalf("SyncEngramCloud() issued %d commands, want %d", len(runner.commands), idx+1)
			}
			if _, statErr := os.Stat(cfg.EngramCloudStatePath()); !os.IsNotExist(statErr) {
				t.Fatalf("SyncEngramCloud() wrote state file despite failure at %s", step)
			}
		})
	}
}

func TestSyncEngramCloud_AlreadyEnrolled_ShortPath(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir()}
	t.Setenv("ENGRAM_CLOUD_TOKEN", "super-secret-token")

	if err := writeJSONFile(cfg.EngramCloudStatePath(), engramCloudState{Enrolled: true, Server: "http://127.0.0.1:18080", Project: "click-ai-devkit", LastSync: "2026-07-22T00:00:00Z"}); err != nil {
		t.Fatalf("writeJSONFile(state) error = %v", err)
	}

	runner := newFakeEngramCloudRunner()
	restore := SetCommandRunnerFactoryForTests(func() CommandRunner { return runner })
	defer restore()

	m := &manifest.Manifest{EngramCloud: manifest.EngramCloud{Server: "http://127.0.0.1:18080", Project: "click-ai-devkit"}}
	if err := SyncEngramCloud(cfg, m); err != nil {
		t.Fatalf("SyncEngramCloud() error = %v", err)
	}

	want := []commandInvocation{
		{Name: "engram", Args: []string{"cloud", "config", "--server", "http://127.0.0.1:18080"}},
		{Name: "engram", Args: []string{"sync", "--cloud", "--project", "click-ai-devkit"}},
	}
	assertCommands(t, runner.commands, want)
}

func TestSyncEngramCloud_TokenNotInArgvOrState(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir()}
	t.Setenv("ENGRAM_CLOUD_TOKEN", "do-not-leak-me")

	runner := newFakeEngramCloudRunner()
	restore := SetCommandRunnerFactoryForTests(func() CommandRunner { return runner })
	defer restore()

	m := &manifest.Manifest{EngramCloud: manifest.EngramCloud{Server: "http://127.0.0.1:18080", Project: "click-ai-devkit"}}
	if err := SyncEngramCloud(cfg, m); err != nil {
		t.Fatalf("SyncEngramCloud() error = %v", err)
	}

	for _, inv := range runner.commands {
		for _, arg := range inv.Args {
			if strings.Contains(arg, "do-not-leak-me") {
				t.Fatalf("token leaked into argv: %q", arg)
			}
		}
	}

	data, err := os.ReadFile(cfg.EngramCloudStatePath())
	if err != nil {
		t.Fatalf("ReadFile(state) error = %v", err)
	}
	if strings.Contains(string(data), "do-not-leak-me") {
		t.Fatalf("token leaked into state file: %s", string(data))
	}
}
