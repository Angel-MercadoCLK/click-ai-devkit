package installer

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExecCommandRunner_UsesDeterministicSafeCwd(t *testing.T) {
	runner := execCommandRunner{}
	got, err := runner.commandDir()
	if err != nil {
		t.Fatal(err)
	}
	if got == "" {
		t.Fatal("commandDir() is empty; Claude/Git invocation needs a safe cwd")
	}
	if _, err := os.Stat(got); err != nil {
		t.Fatalf("commandDir() = %q is not usable: %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(got, ".git")); err != nil {
		t.Fatalf("commandDir() = %q is not a Git repository: %v", got, err)
	}
}

func TestExecCommandRunner_CommandDirInitializationFailureIsClear(t *testing.T) {
	oldWorking, oldRoot, oldMkdir, oldInit := commandWorkingDir, commandTempRoot, commandMkdirAll, commandGitInit
	t.Cleanup(func() {
		commandWorkingDir, commandTempRoot, commandMkdirAll, commandGitInit = oldWorking, oldRoot, oldMkdir, oldInit
	})
	commandWorkingDir = func() (string, error) { return t.TempDir(), nil }
	commandTempRoot = func() string { return filepath.Join(t.TempDir(), "isolated") }
	commandMkdirAll = os.MkdirAll
	commandGitInit = func(string, string) error { return errors.New("synthetic git init failure") }
	restore := SetBinaryLookupFactoryForTests(func() BinaryLookup {
		return &fakeBinaryLookup{resolved: map[string]string{"git": `C:\git.exe`}}
	})
	t.Cleanup(restore)
	_, err := (execCommandRunner{}).commandDir()
	if err == nil || !strings.Contains(err.Error(), "initialize isolated command repository") || !strings.Contains(err.Error(), "synthetic git init failure") {
		t.Fatalf("commandDir() error = %v, want clear initialization failure", err)
	}
}

func TestExecCommandRunner_NonRepositoryUsesInitializedIsolatedRepo(t *testing.T) {
	oldWorking, oldRoot, oldInit := commandWorkingDir, commandTempRoot, commandGitInit
	t.Cleanup(func() { commandWorkingDir, commandTempRoot, commandGitInit = oldWorking, oldRoot, oldInit })
	commandWorkingDir = func() (string, error) { return t.TempDir(), nil }
	root := filepath.Join(t.TempDir(), "isolated")
	commandTempRoot = func() string { return root }
	commandGitInit = func(_, dir string) error { return os.Mkdir(filepath.Join(dir, ".git"), 0o755) }
	got, err := (execCommandRunner{}).commandDir()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, root+string(filepath.Separator)+"git-recovery-") {
		t.Fatalf("commandDir() = %q, want an isolated recovery repo under %q", got, root)
	}
	if _, err := os.Stat(filepath.Join(got, ".git")); err != nil {
		t.Fatalf("isolated cwd is not a Git repository: %v", err)
	}
}
