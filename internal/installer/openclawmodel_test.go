package installer

import (
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type openClawModelTestRunner struct {
	commands    [][]string
	outputs     [][]string
	runErr      error
	outputErr   error
	outputBytes []byte
}

func (r *openClawModelTestRunner) Run(name string, args ...string) error {
	r.commands = append(r.commands, append([]string{name}, args...))
	return r.runErr
}

func (r *openClawModelTestRunner) Output(name string, args ...string) ([]byte, error) {
	r.outputs = append(r.outputs, append([]string{name}, args...))
	return r.outputBytes, r.outputErr
}

type openClawModelTestLookup struct {
	available bool
}

func (l openClawModelTestLookup) LookPath(name string) (string, error) {
	if l.available && name == "openclaw" {
		return "/fake/openclaw", nil
	}
	return "", errors.New("not found")
}

func TestConfigureOpenClawModels_PrimaryOnly_UsesOfficialConfigCommand(t *testing.T) {
	qualifiedBinary, err := filepath.Abs("/fake/openclaw")
	if err != nil {
		t.Fatal(err)
	}
	runner := &openClawModelTestRunner{outputBytes: []byte("config set agents.defaults.model.primary agents.defaults.model.fallbacks --strict-json")}
	restoreRunner := SetCommandRunnerFactoryForTests(func() CommandRunner { return runner })
	defer restoreRunner()
	restoreLookup := SetBinaryLookupFactoryForTests(func() BinaryLookup { return openClawModelTestLookup{available: true} })
	defer restoreLookup()

	if err := ConfigureOpenClawModels("openai/gpt-5.6-sol", nil); err != nil {
		t.Fatalf("ConfigureOpenClawModels() error = %v", err)
	}

	wantProbe := [][]string{{qualifiedBinary, "config", "set", "--help"}}
	if !reflect.DeepEqual(runner.outputs, wantProbe) {
		t.Fatalf("probe = %#v, want %#v", runner.outputs, wantProbe)
	}

	want := [][]string{{qualifiedBinary, "config", "set", "agents.defaults.model.primary", "openai/gpt-5.6-sol"}}
	if !reflect.DeepEqual(runner.commands, want) {
		t.Fatalf("commands = %#v, want %#v", runner.commands, want)
	}
}

func TestConfigureOpenClawModels_PrimaryAndFallbacks_UsesStrictJSON(t *testing.T) {
	qualifiedBinary, err := filepath.Abs("/fake/openclaw")
	if err != nil {
		t.Fatal(err)
	}
	runner := &openClawModelTestRunner{outputBytes: []byte("config set agents.defaults.model.primary agents.defaults.model.fallbacks --strict-json")}
	restoreRunner := SetCommandRunnerFactoryForTests(func() CommandRunner { return runner })
	defer restoreRunner()
	restoreLookup := SetBinaryLookupFactoryForTests(func() BinaryLookup { return openClawModelTestLookup{available: true} })
	defer restoreLookup()

	if err := ConfigureOpenClawModels("anthropic/claude-sonnet-4-6", []string{"openai/gpt-5.6-sol", "google/gemini-2.5-pro"}); err != nil {
		t.Fatalf("ConfigureOpenClawModels() error = %v", err)
	}

	want := [][]string{
		{qualifiedBinary, "config", "set", "agents.defaults.model.primary", "anthropic/claude-sonnet-4-6"},
		{qualifiedBinary, "config", "set", "agents.defaults.model.fallbacks", `["openai/gpt-5.6-sol","google/gemini-2.5-pro"]`, "--strict-json"},
	}
	if !reflect.DeepEqual(runner.commands, want) {
		t.Fatalf("commands = %#v, want %#v", runner.commands, want)
	}
}

func TestConfigureOpenClawModels_OpenClawAbsent_DoesNotRunCommands(t *testing.T) {
	runner := &openClawModelTestRunner{}
	restoreRunner := SetCommandRunnerFactoryForTests(func() CommandRunner { return runner })
	defer restoreRunner()
	restoreLookup := SetBinaryLookupFactoryForTests(func() BinaryLookup { return openClawModelTestLookup{} })
	defer restoreLookup()

	err := ConfigureOpenClawModels("openai/gpt-5.6-sol", nil)
	if err == nil || !strings.Contains(err.Error(), "OpenClaw") || !strings.Contains(err.Error(), "openclaw") {
		t.Fatalf("error = %v, want actionable missing OpenClaw message", err)
	}
	if len(runner.commands) != 0 {
		t.Fatalf("commands = %#v, want no command when OpenClaw is absent", runner.commands)
	}
}

func TestConfigureOpenClawModels_QualificationFailureDoesNotRunWrites(t *testing.T) {
	runner := &openClawModelTestRunner{outputErr: errors.New("help failed")}
	restoreRunner := SetCommandRunnerFactoryForTests(func() CommandRunner { return runner })
	defer restoreRunner()
	restoreLookup := SetBinaryLookupFactoryForTests(func() BinaryLookup { return openClawModelTestLookup{available: true} })
	defer restoreLookup()

	err := ConfigureOpenClawModels("openai/gpt-5.6-sol", nil)
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "qualif") {
		t.Fatalf("error = %v, want qualification failure", err)
	}
	if len(runner.commands) != 0 {
		t.Fatalf("commands = %#v, want no writes after failed qualification", runner.commands)
	}
}

func TestConfigureOpenClawModels_UnexpectedProbeOutputDoesNotRunWrites(t *testing.T) {
	runner := &openClawModelTestRunner{outputBytes: []byte("usage: openclaw help")}
	restoreRunner := SetCommandRunnerFactoryForTests(func() CommandRunner { return runner })
	defer restoreRunner()
	restoreLookup := SetBinaryLookupFactoryForTests(func() BinaryLookup { return openClawModelTestLookup{available: true} })
	defer restoreLookup()

	err := ConfigureOpenClawModels("openai/gpt-5.6-sol", nil)
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "contract") {
		t.Fatalf("error = %v, want unexpected-output contract failure", err)
	}
	if len(runner.commands) != 0 {
		t.Fatalf("commands = %#v, want no writes after unexpected probe output", runner.commands)
	}
}

func TestConfigureOpenClawModels_ProbeTimeoutDoesNotRunWrites(t *testing.T) {
	runner := &openClawModelTestRunner{outputErr: errors.New("installer: command \"/fake/openclaw\" timed out after 30s")}
	restoreRunner := SetCommandRunnerFactoryForTests(func() CommandRunner { return runner })
	defer restoreRunner()
	restoreLookup := SetBinaryLookupFactoryForTests(func() BinaryLookup { return openClawModelTestLookup{available: true} })
	defer restoreLookup()

	err := ConfigureOpenClawModels("openai/gpt-5.6-sol", nil)
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "timed out") {
		t.Fatalf("error = %v, want timeout qualification failure", err)
	}
	if len(runner.commands) != 0 {
		t.Fatalf("commands = %#v, want no writes after timeout", runner.commands)
	}
}

func TestConfigureOpenClawModels_CommandFailureIsWrappedAfterQualification(t *testing.T) {
	wantErr := errors.New("schema rejected model")
	runner := &openClawModelTestRunner{
		runErr:      wantErr,
		outputBytes: []byte("config set agents.defaults.model.primary agents.defaults.model.fallbacks --strict-json"),
	}
	restoreRunner := SetCommandRunnerFactoryForTests(func() CommandRunner { return runner })
	defer restoreRunner()
	restoreLookup := SetBinaryLookupFactoryForTests(func() BinaryLookup { return openClawModelTestLookup{available: true} })
	defer restoreLookup()

	err := ConfigureOpenClawModels("openai/gpt-5.6-sol", []string{"openai/gpt-5.5"})
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want wrapped %v", err, wantErr)
	}
	if len(runner.commands) != 1 {
		t.Fatalf("commands = %#v, want fallback command skipped after primary failure", runner.commands)
	}
}
