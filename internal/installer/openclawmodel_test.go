package installer

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

type openClawModelTestRunner struct {
	commands [][]string
	err      error
}

func (r *openClawModelTestRunner) Run(name string, args ...string) error {
	r.commands = append(r.commands, append([]string{name}, args...))
	return r.err
}

func (r *openClawModelTestRunner) Output(name string, args ...string) ([]byte, error) {
	return nil, r.err
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
	runner := &openClawModelTestRunner{}
	restoreRunner := SetCommandRunnerFactoryForTests(func() CommandRunner { return runner })
	defer restoreRunner()
	restoreLookup := SetBinaryLookupFactoryForTests(func() BinaryLookup { return openClawModelTestLookup{available: true} })
	defer restoreLookup()

	if err := ConfigureOpenClawModels("openai/gpt-5.6-sol", nil); err != nil {
		t.Fatalf("ConfigureOpenClawModels() error = %v", err)
	}

	want := [][]string{{"openclaw", "config", "set", "agents.defaults.model.primary", "openai/gpt-5.6-sol"}}
	if !reflect.DeepEqual(runner.commands, want) {
		t.Fatalf("commands = %#v, want %#v", runner.commands, want)
	}
}

func TestConfigureOpenClawModels_PrimaryAndFallbacks_UsesStrictJSON(t *testing.T) {
	runner := &openClawModelTestRunner{}
	restoreRunner := SetCommandRunnerFactoryForTests(func() CommandRunner { return runner })
	defer restoreRunner()
	restoreLookup := SetBinaryLookupFactoryForTests(func() BinaryLookup { return openClawModelTestLookup{available: true} })
	defer restoreLookup()

	if err := ConfigureOpenClawModels("anthropic/claude-sonnet-4-6", []string{"openai/gpt-5.6-sol", "google/gemini-2.5-pro"}); err != nil {
		t.Fatalf("ConfigureOpenClawModels() error = %v", err)
	}

	want := [][]string{
		{"openclaw", "config", "set", "agents.defaults.model.primary", "anthropic/claude-sonnet-4-6"},
		{"openclaw", "config", "set", "agents.defaults.model.fallbacks", `["openai/gpt-5.6-sol","google/gemini-2.5-pro"]`, "--strict-json"},
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

func TestConfigureOpenClawModels_CommandFailure_IsWrapped(t *testing.T) {
	wantErr := errors.New("schema rejected model")
	runner := &openClawModelTestRunner{err: wantErr}
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
