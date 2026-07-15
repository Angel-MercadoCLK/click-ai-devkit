package installer

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestGoBinDir_PrefersGOBIN covers the common case: `go env GOBIN` is set (either via the process
// env or a persisted `go env -w GOBIN=...`), so GoBinDir must use it directly without ever
// consulting GOPATH.
func TestGoBinDir_PrefersGOBIN(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir()}
	runner := newFakeCommandRunner(cfg)
	runner.lookup["go env GOBIN"] = []byte("C:\\Users\\dev\\customgobin\n")
	runner.lookup["go env GOPATH"] = []byte("C:\\Users\\dev\\go\n")
	restore := SetCommandRunnerFactoryForTests(func() CommandRunner { return runner })
	defer restore()

	got, err := GoBinDir(cfg)
	if err != nil {
		t.Fatalf("GoBinDir() error = %v", err)
	}
	want := "C:\\Users\\dev\\customgobin"
	if got != want {
		t.Fatalf("GoBinDir() = %q, want %q (GOBIN must win over GOPATH/bin)", got, want)
	}
	for _, inv := range runner.commands {
		if inv.Name == "go" && len(inv.Args) == 2 && inv.Args[1] == "GOPATH" {
			t.Fatalf("GoBinDir() queried go env GOPATH even though GOBIN was set: %#v", runner.commands)
		}
	}
}

// TestGoBinDir_FallsBackToGOPATHBin covers the documented fallback: an empty `go env GOBIN` means
// GoBinDir must resolve to GOPATH/bin instead.
func TestGoBinDir_FallsBackToGOPATHBin(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir()}
	runner := newFakeCommandRunner(cfg)
	// gopath is a fixture value, not a real filesystem path — it deliberately does not use
	// filepath.Join for its own construction so this test stays independent of the host OS's
	// separator. GoBinDir() itself joins it with "bin" via filepath.Join, so `want` must be
	// derived the same way rather than hardcoded, or this test only passes on the OS it was
	// authored on (it previously hardcoded a `\`-joined `want`, which fails on any non-Windows
	// CI leg since filepath.Join uses `/` there).
	gopath := "C:\\Users\\dev\\go"
	runner.lookup["go env GOBIN"] = []byte("\n")
	runner.lookup["go env GOPATH"] = []byte(gopath + "\n")
	restore := SetCommandRunnerFactoryForTests(func() CommandRunner { return runner })
	defer restore()

	got, err := GoBinDir(cfg)
	if err != nil {
		t.Fatalf("GoBinDir() error = %v", err)
	}
	want := filepath.Join(gopath, "bin")
	if got != want {
		t.Fatalf("GoBinDir() = %q, want %q", got, want)
	}
}

// TestGoBinDir_ErrorsWhenNeitherResolves covers the failure path: neither GOBIN nor GOPATH is
// resolvable — GoBinDir must surface an error rather than silently return an empty/invalid path.
func TestGoBinDir_ErrorsWhenNeitherResolves(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir()}
	runner := newFakeCommandRunner(cfg)
	// lookup left empty: fakeCommandRunner.Output returns []byte{}, nil for unrecognized keys.
	restore := SetCommandRunnerFactoryForTests(func() CommandRunner { return runner })
	defer restore()

	_, err := GoBinDir(cfg)
	if err == nil {
		t.Fatal("GoBinDir() error = nil, want an error when neither GOBIN nor GOPATH resolves")
	}
	if !strings.Contains(err.Error(), "GOPATH") {
		t.Fatalf("GoBinDir() error = %q, want it to mention GOPATH", err.Error())
	}
}

// TestAtomicWriteFile_WritesContentAndLeavesNoLeftovers covers the happy path: a new file is
// created with the given content and mode, and no ".click-*" temp file lingers in the directory
// afterward (the rename must have consumed it).
func TestAtomicWriteFile_WritesContentAndLeavesNoLeftovers(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "target.txt")
	content := []byte("hello atomic world\n")

	if err := atomicWriteFile(path, content, 0o644); err != nil {
		t.Fatalf("atomicWriteFile() error = %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(got) != string(content) {
		t.Fatalf("file content = %q, want %q", got, content)
	}

	leftovers, globErr := filepath.Glob(filepath.Join(dir, ".click-*"))
	if globErr != nil {
		t.Fatalf("Glob() error = %v", globErr)
	}
	if len(leftovers) != 0 {
		t.Fatalf("leftover temp files after a successful write: %v", leftovers)
	}
}

// TestAtomicWriteFile_ReplacesExistingContent triangulates against an existing (not new) target
// file, proving the rename actually replaces prior content rather than only working for brand-new
// files.
func TestAtomicWriteFile_ReplacesExistingContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "target.txt")
	if err := os.WriteFile(path, []byte("old content"), 0o644); err != nil {
		t.Fatalf("WriteFile(old) error = %v", err)
	}

	if err := atomicWriteFile(path, []byte("new content"), 0o644); err != nil {
		t.Fatalf("atomicWriteFile() error = %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(got) != "new content" {
		t.Fatalf("file content = %q, want %q", got, "new content")
	}
}

// fakeFailingTempFile is a tempFileWriter double whose Write always fails with a fixed error,
// letting TestAtomicWriteFile_InjectedWriteErrorLeavesOriginalIntact exercise atomicWriteFile's
// error path deterministically — without relying on flaky OS-level failure injection (full disk,
// permission races, etc.).
type fakeFailingTempFile struct {
	name     string
	writeErr error
}

func (f *fakeFailingTempFile) Write(p []byte) (int, error) { return 0, f.writeErr }
func (f *fakeFailingTempFile) Sync() error                 { return nil }
func (f *fakeFailingTempFile) Close() error                { return nil }
func (f *fakeFailingTempFile) Name() string                { return f.name }

// TestAtomicWriteFile_InjectedWriteErrorLeavesOriginalIntact is the strict-TDD-required case for
// atomicWriteFile: when the write step fails, the original file at path must be left
// byte-for-byte intact and the error must propagate to the caller.
func TestAtomicWriteFile_InjectedWriteErrorLeavesOriginalIntact(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "target.txt")
	original := []byte("original content that must survive\n")
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatalf("WriteFile(original) error = %v", err)
	}

	injectedErr := errors.New("injected write failure")
	old := createTempFile
	createTempFile = func(dir, pattern string) (tempFileWriter, error) {
		return &fakeFailingTempFile{name: filepath.Join(dir, ".click-injected-fake"), writeErr: injectedErr}, nil
	}
	defer func() { createTempFile = old }()

	err := atomicWriteFile(path, []byte("new content that must never land"), 0o644)
	if err == nil {
		t.Fatal("atomicWriteFile() error = nil, want the injected write error to propagate")
	}
	if !errors.Is(err, injectedErr) {
		t.Fatalf("atomicWriteFile() error = %v, want it to wrap %v", err, injectedErr)
	}

	got, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("ReadFile(path) error = %v", readErr)
	}
	if string(got) != string(original) {
		t.Fatalf("target file = %q after a failed write, want untouched original %q", got, original)
	}
}

// fakePathStore is a trivial pathStore double used only to prove SetPathStoreFactoryForTests
// actually overrides (and later restores) pathStoreFactory.
type fakePathStore struct{}

func (fakePathStore) PersistedPathContains(dir string) (bool, error) { return false, nil }
func (fakePathStore) EnsureOnPath(dir string) (bool, error)          { return false, nil }

// TestSetPathStoreFactoryForTests_OverridesAndRestores proves the injectable-factory seam PR2/PR3
// rely on actually works: overriding returns the fake, and calling the restore func puts
// pathStoreFactory back to whatever it was before the override — WITHOUT this generic,
// cross-platform test file asserting a specific pre-override value or type. PR1 (this file) leaves
// pathStoreFactory nil, but PR2's windows-tagged pathenv_windows.go init() (and PR3's POSIX
// equivalent) assign a platform default via a build-tagged init(); this file has no build tag and
// must keep compiling on every CI platform, so it deliberately never references a concrete
// platform type like osPathStore. It only proves restore() puts back exactly the pre-override
// func value (nil-ness and, when non-nil, "no longer the injected fake").
func TestSetPathStoreFactoryForTests_OverridesAndRestores(t *testing.T) {
	before := pathStoreFactory

	restore := SetPathStoreFactoryForTests(func() pathStore { return fakePathStore{} })
	defer restore()

	got := pathStoreFactory()
	if _, ok := got.(fakePathStore); !ok {
		t.Fatalf("pathStoreFactory() = %#v, want the injected fakePathStore", got)
	}

	restore()
	after := pathStoreFactory
	if (before == nil) != (after == nil) {
		t.Fatalf("pathStoreFactory nil-ness after restore() = %v, want it back to its pre-override nil-ness %v", after == nil, before == nil)
	}
	if after != nil {
		if _, ok := after().(fakePathStore); ok {
			t.Fatal("pathStoreFactory() after restore() is still the injected fakePathStore, want it reverted to the pre-override value")
		}
	}
}

// TestComputeNewPath covers the pure REG_EXPAND_SZ-safe PATH-value mutation: idempotency (exact
// match, case-insensitive match, trailing "\" normalized match) and the empty-PATH bootstrap case.
func TestComputeNewPath(t *testing.T) {
	tests := []struct {
		name        string
		current     string
		dir         string
		wantValue   string
		wantChanged bool
	}{
		{
			name:        "empty current bootstraps to dir alone",
			current:     "",
			dir:         `C:\Users\dev\go\bin`,
			wantValue:   `C:\Users\dev\go\bin`,
			wantChanged: true,
		},
		{
			name:        "dir already present exact match is a no-op",
			current:     `C:\Windows\system32;C:\Users\dev\go\bin`,
			dir:         `C:\Users\dev\go\bin`,
			wantValue:   `C:\Windows\system32;C:\Users\dev\go\bin`,
			wantChanged: false,
		},
		{
			name:        "dir present with different case is a no-op",
			current:     `C:\Windows\system32;C:\USERS\DEV\GO\BIN`,
			dir:         `C:\Users\dev\go\bin`,
			wantValue:   `C:\Windows\system32;C:\USERS\DEV\GO\BIN`,
			wantChanged: false,
		},
		{
			name:        "dir present with trailing backslash is a no-op",
			current:     `C:\Windows\system32;C:\Users\dev\go\bin\`,
			dir:         `C:\Users\dev\go\bin`,
			wantValue:   `C:\Windows\system32;C:\Users\dev\go\bin\`,
			wantChanged: false,
		},
		{
			name:        "dir missing is appended",
			current:     `C:\Windows\system32`,
			dir:         `C:\Users\dev\go\bin`,
			wantValue:   `C:\Windows\system32;C:\Users\dev\go\bin`,
			wantChanged: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotValue, gotChanged := computeNewPath(tt.current, tt.dir)
			if gotValue != tt.wantValue || gotChanged != tt.wantChanged {
				t.Fatalf("computeNewPath(%q, %q) = (%q, %v), want (%q, %v)",
					tt.current, tt.dir, gotValue, gotChanged, tt.wantValue, tt.wantChanged)
			}
		})
	}
}
