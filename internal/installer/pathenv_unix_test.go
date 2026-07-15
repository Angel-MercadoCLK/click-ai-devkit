//go:build !windows

package installer

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// resetPosixEnv clears/sets the env vars this file's shell-rc logic reads (HOME, SHELL, GOPATH,
// GOBIN) to a hermetic, deterministic state for a single test, using t.Setenv so everything is
// restored automatically at the end of the test. It NEVER touches the real user's HOME, so no
// test in this file can ever read or write the developer's actual ~/.bashrc / ~/.zshrc / ~/.profile.
func resetPosixEnv(t *testing.T, home, shell string) {
	t.Helper()
	t.Setenv("HOME", home)
	t.Setenv("SHELL", shell)
	t.Setenv("GOPATH", "")
	t.Setenv("GOBIN", "")
}

func readFileString(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	return string(data)
}

func mustNotExist(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("Stat(%s) = (_, %v), want a not-exist error (file must not have been created)", path, err)
	}
}

// --- Pure-function tests ---------------------------------------------------

func TestExpandPosixPathVars(t *testing.T) {
	t.Setenv("HOME", "/home/dev")
	t.Setenv("GOPATH", "/home/dev/gopath")
	t.Setenv("GOBIN", "/home/dev/gobin")

	tests := []struct {
		name  string
		value string
		want  string
	}{
		{"bare GOPATH", "$GOPATH/bin", "/home/dev/gopath/bin"},
		{"braced GOPATH", "${GOPATH}/bin", "/home/dev/gopath/bin"},
		{"bare HOME", "$HOME/go/bin", "/home/dev/go/bin"},
		{"bare GOBIN", "$GOBIN", "/home/dev/gobin"},
		{"unrelated var untouched", "$PATH:/x/y", "$PATH:/x/y"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := expandPosixPathVars(tt.value); got != tt.want {
				t.Fatalf("expandPosixPathVars(%q) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}

func TestPosixPathValueContains(t *testing.T) {
	tests := []struct {
		name  string
		value string
		dir   string
		want  bool
	}{
		{"exact match among entries", "/usr/bin:/home/dev/go/bin:/bin", "/home/dev/go/bin", true},
		{"trailing slash normalized", "/home/dev/go/bin/", "/home/dev/go/bin", true},
		{"dir missing", "/usr/bin:/bin", "/home/dev/go/bin", false},
		{"case-sensitive: different case is NOT a match", "/HOME/DEV/GO/BIN", "/home/dev/go/bin", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := posixPathValueContains(tt.value, tt.dir); got != tt.want {
				t.Fatalf("posixPathValueContains(%q, %q) = %v, want %v", tt.value, tt.dir, got, tt.want)
			}
		})
	}
}

func TestRcContainsDir(t *testing.T) {
	t.Setenv("HOME", "/home/dev")
	t.Setenv("GOPATH", "/home/dev/gopath")
	t.Setenv("GOBIN", "")

	tests := []struct {
		name    string
		content string
		dir     string
		want    bool
	}{
		{
			name:    "export PATH with GOPATH expansion matches",
			content: "# comment\nexport PATH=\"$PATH:$GOPATH/bin\"\n",
			dir:     "/home/dev/gopath/bin",
			want:    true,
		},
		{
			name:    "bare PATH= (no export) also matches",
			content: "PATH=$GOPATH/bin:$PATH\n",
			dir:     "/home/dev/gopath/bin",
			want:    true,
		},
		{
			name:    "commented-out line is ignored",
			content: "# export PATH=\"$PATH:$GOPATH/bin\"\n",
			dir:     "/home/dev/gopath/bin",
			want:    false,
		},
		{
			name:    "unrelated line is ignored",
			content: "alias ll='ls -la'\n",
			dir:     "/home/dev/gopath/bin",
			want:    false,
		},
		{
			name:    "no match when dir absent",
			content: "export PATH=\"$PATH:/usr/local/bin\"\n",
			dir:     "/home/dev/gopath/bin",
			want:    false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := rcContainsDir(tt.content, tt.dir); got != tt.want {
				t.Fatalf("rcContainsDir(%q, %q) = %v, want %v", tt.content, tt.dir, got, tt.want)
			}
		})
	}
}

func TestBashLoginFile(t *testing.T) {
	t.Run("no candidate files exist: defaults to .bash_profile", func(t *testing.T) {
		home := t.TempDir()
		want := filepath.Join(home, ".bash_profile")
		if got := bashLoginFile(home); got != want {
			t.Fatalf("bashLoginFile(%q) = %q, want %q", home, got, want)
		}
	})

	t.Run("only .profile exists: uses .profile, does not invent .bash_profile", func(t *testing.T) {
		home := t.TempDir()
		profile := filepath.Join(home, ".profile")
		if err := os.WriteFile(profile, []byte(""), 0o644); err != nil {
			t.Fatalf("WriteFile(.profile) error = %v", err)
		}
		if got := bashLoginFile(home); got != profile {
			t.Fatalf("bashLoginFile(%q) = %q, want %q", home, got, profile)
		}
	})

	t.Run("both .bash_profile and .profile exist: .bash_profile wins (priority order)", func(t *testing.T) {
		home := t.TempDir()
		bashProfile := filepath.Join(home, ".bash_profile")
		profile := filepath.Join(home, ".profile")
		if err := os.WriteFile(bashProfile, []byte(""), 0o644); err != nil {
			t.Fatalf("WriteFile(.bash_profile) error = %v", err)
		}
		if err := os.WriteFile(profile, []byte(""), 0o644); err != nil {
			t.Fatalf("WriteFile(.profile) error = %v", err)
		}
		if got := bashLoginFile(home); got != bashProfile {
			t.Fatalf("bashLoginFile(%q) = %q, want %q (priority order)", home, got, bashProfile)
		}
	})
}

func TestPosixShellTargets(t *testing.T) {
	t.Run("zsh targets only .zshrc", func(t *testing.T) {
		home := t.TempDir()
		resetPosixEnv(t, home, "/usr/bin/zsh")
		got, err := posixShellTargets()
		if err != nil {
			t.Fatalf("posixShellTargets() error = %v", err)
		}
		want := []string{filepath.Join(home, ".zshrc")}
		if len(got) != 1 || got[0] != want[0] {
			t.Fatalf("posixShellTargets() = %v, want %v", got, want)
		}
	})

	t.Run("bash targets login-chain file AND .bashrc", func(t *testing.T) {
		home := t.TempDir()
		resetPosixEnv(t, home, "/bin/bash")
		got, err := posixShellTargets()
		if err != nil {
			t.Fatalf("posixShellTargets() error = %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("posixShellTargets() = %v, want exactly 2 targets (login-chain file + .bashrc)", got)
		}
		if got[0] != filepath.Join(home, ".bash_profile") || got[1] != filepath.Join(home, ".bashrc") {
			t.Fatalf("posixShellTargets() = %v, want [.bash_profile, .bashrc]", got)
		}
	})

	t.Run("fish is skipped: no targets, no error", func(t *testing.T) {
		home := t.TempDir()
		resetPosixEnv(t, home, "/usr/bin/fish")
		got, err := posixShellTargets()
		if err != nil {
			t.Fatalf("posixShellTargets() error = %v, want nil", err)
		}
		if len(got) != 0 {
			t.Fatalf("posixShellTargets() = %v, want empty for fish", got)
		}
	})

	t.Run("unrecognized shell falls back to .profile", func(t *testing.T) {
		home := t.TempDir()
		resetPosixEnv(t, home, "/bin/sh")
		got, err := posixShellTargets()
		if err != nil {
			t.Fatalf("posixShellTargets() error = %v", err)
		}
		want := []string{filepath.Join(home, ".profile")}
		if len(got) != 1 || got[0] != want[0] {
			t.Fatalf("posixShellTargets() = %v, want %v", got, want)
		}
	})
}

// --- osPathStore integration tests (real t.TempDir() files, never the real home dir) -----------

func TestEnsureOnPath_FreshInstallAppendsToBashLoginChainAndBashrc(t *testing.T) {
	home := t.TempDir()
	resetPosixEnv(t, home, "/bin/bash")
	dir := "/home/dev/go/bin"

	changed, err := osPathStore{}.EnsureOnPath(dir)
	if err != nil {
		t.Fatalf("EnsureOnPath() error = %v", err)
	}
	if !changed {
		t.Fatal("EnsureOnPath() changed = false, want true on a fresh install")
	}

	for _, name := range []string{".bash_profile", ".bashrc"} {
		content := readFileString(t, filepath.Join(home, name))
		if !rcContainsDir(content, dir) {
			t.Fatalf("%s content = %q, want it to contain dir %q after EnsureOnPath", name, content, dir)
		}
	}
	// .bash_login must NOT have been created — only the winning login-chain candidate + .bashrc.
	mustNotExist(t, filepath.Join(home, ".bash_login"))
	mustNotExist(t, filepath.Join(home, ".profile"))
}

func TestEnsureOnPath_IdempotentRerunViaOwnMarkerIsNoOp(t *testing.T) {
	home := t.TempDir()
	resetPosixEnv(t, home, "/usr/bin/zsh")
	dir := "/home/dev/go/bin"

	if _, err := (osPathStore{}).EnsureOnPath(dir); err != nil {
		t.Fatalf("first EnsureOnPath() error = %v", err)
	}
	before := readFileString(t, filepath.Join(home, ".zshrc"))

	changed, err := osPathStore{}.EnsureOnPath(dir)
	if err != nil {
		t.Fatalf("second EnsureOnPath() error = %v", err)
	}
	if changed {
		t.Fatal("second EnsureOnPath() changed = true, want false (idempotent re-run via click's own marker)")
	}
	after := readFileString(t, filepath.Join(home, ".zshrc"))
	if before != after {
		t.Fatalf(".zshrc content changed on idempotent re-run:\nbefore=%q\nafter=%q", before, after)
	}
}

func TestEnsureOnPath_PreExistingManualExportLineIsNoOp(t *testing.T) {
	home := t.TempDir()
	resetPosixEnv(t, home, "/usr/bin/zsh")
	t.Setenv("GOPATH", filepath.Join(home, "gopath"))
	dir := filepath.Join(home, "gopath", "bin")

	zshrc := filepath.Join(home, ".zshrc")
	manual := "export PATH=\"$PATH:$GOPATH/bin\"\n"
	if err := os.WriteFile(zshrc, []byte(manual), 0o644); err != nil {
		t.Fatalf("WriteFile(.zshrc) error = %v", err)
	}

	contains, err := osPathStore{}.PersistedPathContains(dir)
	if err != nil {
		t.Fatalf("PersistedPathContains() error = %v", err)
	}
	if !contains {
		t.Fatal("PersistedPathContains() = false, want true: pre-existing manual export line already covers dir")
	}

	changed, err := osPathStore{}.EnsureOnPath(dir)
	if err != nil {
		t.Fatalf("EnsureOnPath() error = %v", err)
	}
	if changed {
		t.Fatal("EnsureOnPath() changed = true, want false: must not duplicate a pre-existing manual PATH line")
	}
	after := readFileString(t, zshrc)
	if after != manual {
		t.Fatalf(".zshrc content = %q after EnsureOnPath, want it byte-for-byte unchanged from %q", after, manual)
	}
}

func TestEnsureOnPath_GoBinDirChangeReplacesMarkerBlock(t *testing.T) {
	home := t.TempDir()
	resetPosixEnv(t, home, "/usr/bin/zsh")
	zshrc := filepath.Join(home, ".zshrc")
	dirOld := "/home/dev/go/bin"
	dirNew := "/home/dev/go2/bin"

	if _, err := (osPathStore{}).EnsureOnPath(dirOld); err != nil {
		t.Fatalf("first EnsureOnPath(%q) error = %v", dirOld, err)
	}
	firstContent := readFileString(t, zshrc)
	if !rcContainsDir(firstContent, dirOld) {
		t.Fatalf(".zshrc after first EnsureOnPath = %q, want it to contain %q", firstContent, dirOld)
	}

	changed, err := osPathStore{}.EnsureOnPath(dirNew)
	if err != nil {
		t.Fatalf("second EnsureOnPath(%q) error = %v", dirNew, err)
	}
	if !changed {
		t.Fatal("EnsureOnPath() with a different dir changed = false, want true (GoBinDir() changed since last run)")
	}

	secondContent := readFileString(t, zshrc)
	if rcContainsDir(secondContent, dirOld) {
		t.Fatalf(".zshrc after GoBinDir() change still contains stale dir %q: %q", dirOld, secondContent)
	}
	if !rcContainsDir(secondContent, dirNew) {
		t.Fatalf(".zshrc after GoBinDir() change = %q, want it to contain new dir %q", secondContent, dirNew)
	}
	if begin, end := findPosixMarkers(splitLines(secondContent)); begin == -1 || end == -1 {
		t.Fatalf(".zshrc after replace has no well-formed managed block: %q", secondContent)
	}
	// Exactly one begin marker must be present — a replace, not an accumulation of blocks.
	count := 0
	for _, l := range splitLines(secondContent) {
		if l == posixManagedBeginMarker {
			count++
		}
	}
	if count != 1 {
		t.Fatalf(".zshrc after replace has %d begin markers, want exactly 1 (no stale duplicate block): %q", count, secondContent)
	}
}

func TestEnsureOnPath_BashUsesExistingProfileWhenLoginFilesAbsent(t *testing.T) {
	home := t.TempDir()
	resetPosixEnv(t, home, "/bin/bash")
	profile := filepath.Join(home, ".profile")
	if err := os.WriteFile(profile, []byte("echo hello\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(.profile) error = %v", err)
	}
	dir := "/home/dev/go/bin"

	changed, err := osPathStore{}.EnsureOnPath(dir)
	if err != nil {
		t.Fatalf("EnsureOnPath() error = %v", err)
	}
	if !changed {
		t.Fatal("EnsureOnPath() changed = false, want true")
	}

	mustNotExist(t, filepath.Join(home, ".bash_profile"))
	mustNotExist(t, filepath.Join(home, ".bash_login"))

	profileContent := readFileString(t, profile)
	if !rcContainsDir(profileContent, dir) {
		t.Fatalf(".profile content = %q, want it to contain dir %q", profileContent, dir)
	}
	if !stringsContainsLine(profileContent, "echo hello") {
		t.Fatalf(".profile content = %q, want original content preserved", profileContent)
	}

	bashrcContent := readFileString(t, filepath.Join(home, ".bashrc"))
	if !rcContainsDir(bashrcContent, dir) {
		t.Fatalf(".bashrc content = %q, want it to contain dir %q", bashrcContent, dir)
	}
}

func TestEnsureOnPath_FishShellSkipsWithoutError(t *testing.T) {
	home := t.TempDir()
	resetPosixEnv(t, home, "/usr/bin/fish")
	dir := "/home/dev/go/bin"

	changed, err := osPathStore{}.EnsureOnPath(dir)
	if err != nil {
		t.Fatalf("EnsureOnPath() error = %v, want nil for fish (skip, not fail)", err)
	}
	if changed {
		t.Fatal("EnsureOnPath() changed = true for fish, want false (fish is skipped, no mutation)")
	}

	entries, err := os.ReadDir(home)
	if err != nil {
		t.Fatalf("ReadDir(home) error = %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("home dir has %d entries after EnsureOnPath() on fish, want 0 (no files created): %v", len(entries), entries)
	}
}

func TestPersistedPathContains_FreshFalseThenTrueAfterEnsure(t *testing.T) {
	home := t.TempDir()
	resetPosixEnv(t, home, "/usr/bin/zsh")
	dir := "/home/dev/go/bin"

	before, err := osPathStore{}.PersistedPathContains(dir)
	if err != nil {
		t.Fatalf("PersistedPathContains() error = %v", err)
	}
	if before {
		t.Fatal("PersistedPathContains() = true before any EnsureOnPath call, want false")
	}

	if _, err := (osPathStore{}).EnsureOnPath(dir); err != nil {
		t.Fatalf("EnsureOnPath() error = %v", err)
	}

	after, err := osPathStore{}.PersistedPathContains(dir)
	if err != nil {
		t.Fatalf("PersistedPathContains() error = %v", err)
	}
	if !after {
		t.Fatal("PersistedPathContains() = false after EnsureOnPath, want true")
	}
}

// TestEnsureOnPath_AtomicWriteFailureLeavesOriginalIntact reuses PR1's injected-write-error
// pattern (TestAtomicWriteFile_InjectedWriteErrorLeavesOriginalIntact in pathenv_test.go) at the
// osPathStore integration level: when the underlying atomic write fails mid-mutation, the rc
// file's original content must survive byte-for-byte and the error must propagate.
func TestEnsureOnPath_AtomicWriteFailureLeavesOriginalIntact(t *testing.T) {
	home := t.TempDir()
	resetPosixEnv(t, home, "/usr/bin/zsh")
	zshrc := filepath.Join(home, ".zshrc")
	original := []byte("# my existing zshrc\nalias ll='ls -la'\n")
	if err := os.WriteFile(zshrc, original, 0o644); err != nil {
		t.Fatalf("WriteFile(.zshrc) error = %v", err)
	}

	injectedErr := errors.New("injected write failure (unix rc mutation)")
	old := createTempFile
	createTempFile = func(dir, pattern string) (tempFileWriter, error) {
		return &fakeFailingTempFile{name: filepath.Join(dir, ".click-injected-fake-unix"), writeErr: injectedErr}, nil
	}
	defer func() { createTempFile = old }()

	_, err := osPathStore{}.EnsureOnPath("/home/dev/go/bin")
	if err == nil {
		t.Fatal("EnsureOnPath() error = nil, want the injected write error to propagate")
	}

	got := readFileString(t, zshrc)
	if string(got) != string(original) {
		t.Fatalf(".zshrc content = %q after a failed write, want untouched original %q", got, original)
	}
}

func stringsContainsLine(content, line string) bool {
	for _, l := range splitLines(content) {
		if l == line {
			return true
		}
	}
	return false
}
