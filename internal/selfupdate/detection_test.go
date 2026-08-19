package selfupdate

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestResolveRunningExecutable_SeamFailuresYieldUnknownInputs tests that
// resolveRunningExecutable degrades gracefully when its underlying seams fail.
func TestResolveRunningExecutable_SeamFailuresYieldUnknownInputs(t *testing.T) {
	tests := []struct {
		name          string
		setupSeams    func()
		teardownSeams func()
		wantUnknown   bool // true if we expect empty string (unknown)
	}{
		{
			name: "executablePath returns error",
			setupSeams: func() {
				executablePath = func() (string, error) {
					return "", errors.New("mock error")
				}
			},
			teardownSeams: func() {
				executablePath = os.Executable
			},
			wantUnknown: true,
		},
		{
			name: "evalExecutableSymlinks returns error",
			setupSeams: func() {
				executablePath = func() (string, error) {
					return "/some/path/click.exe", nil
				}
				evalExecutableSymlinks = func(path string) (string, error) {
					return "", errors.New("mock symlink error")
				}
			},
			teardownSeams: func() {
				executablePath = os.Executable
				evalExecutableSymlinks = filepath.EvalSymlinks
			},
			wantUnknown: true,
		},
		{
			name: "statExecutable reports a directory",
			setupSeams: func() {
				// Create a temporary directory to use as the fake executable
				tmpDir := t.TempDir()
				executablePath = func() (string, error) {
					return tmpDir, nil
				}
				evalExecutableSymlinks = func(path string) (string, error) {
					return path, nil
				}
			},
			teardownSeams: func() {
				executablePath = os.Executable
				evalExecutableSymlinks = filepath.EvalSymlinks
				statExecutable = os.Stat
			},
			wantUnknown: true,
		},
		{
			name: "resolved path is not absolute",
			setupSeams: func() {
				executablePath = func() (string, error) {
					return "relative/path/click.exe", nil
				}
				evalExecutableSymlinks = func(path string) (string, error) {
					return path, nil
				}
			},
			teardownSeams: func() {
				executablePath = os.Executable
				evalExecutableSymlinks = filepath.EvalSymlinks
				statExecutable = os.Stat
			},
			wantUnknown: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original statExecutable if we're not overriding it
			originalStat := statExecutable
			if tt.name == "statExecutable reports a directory" || tt.name == "resolved path is not absolute" {
				// These tests don't override statExecutable, so we keep the real one
				// but need to restore it if they did override
			} else {
				// For the first two tests, keep the real stat
				statExecutable = os.Stat
				t.Cleanup(func() {
					statExecutable = originalStat
				})
			}

			tt.setupSeams()
			if tt.teardownSeams != nil {
				t.Cleanup(tt.teardownSeams)
			}

			got := resolveRunningExecutable()

			if tt.wantUnknown {
				if got != "" {
					t.Errorf("resolveRunningExecutable() = %q, want empty string (unknown)", got)
				}
			} else {
				if got == "" {
					t.Errorf("resolveRunningExecutable() = empty string, want non-empty path")
				}
			}
		})
	}
}

// TestResolveRunningExecutable_ValidExecutableReturnsPath tests the happy path.
func TestResolveRunningExecutable_ValidExecutableReturnsPath(t *testing.T) {
	// Create a fake executable in a temp directory
	tmpDir := t.TempDir()
	fakeExe := filepath.Join(tmpDir, "click.exe")
	if err := os.WriteFile(fakeExe, []byte("fake exe"), 0o755); err != nil {
		t.Fatalf("failed to create fake executable: %v", err)
	}

	// Save original seams
	originalExecutablePath := executablePath
	originalEvalSymlinks := evalExecutableSymlinks
	originalStatExecutable := statExecutable

	// Set up seams to return our fake executable
	executablePath = func() (string, error) {
		return fakeExe, nil
	}
	evalExecutableSymlinks = func(path string) (string, error) {
		return path, nil
	}
	statExecutable = os.Stat

	t.Cleanup(func() {
		executablePath = originalExecutablePath
		evalExecutableSymlinks = originalEvalSymlinks
		statExecutable = originalStatExecutable
	})

	got := resolveRunningExecutable()

	if got != fakeExe {
		t.Errorf("resolveRunningExecutable() = %q, want %q", got, fakeExe)
	}
}

// TestProbeScoopDirectory_ClassifiesOwnership tests ownership state detection.
func TestProbeScoopDirectory_ClassifiesOwnership(t *testing.T) {
	tests := []struct {
		name       string
		setupFiles func(dir string) error
		wantState  ownershipState
		wantBucket string // only meaningful when state == ownershipValid
	}{
		{
			name: "both manifest and install present and valid",
			setupFiles: func(dir string) error {
				if err := os.WriteFile(
					filepath.Join(dir, "manifest.json"),
					[]byte(`{"version":"1.2.3","url":"https://example.com/click.zip","hash":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}`),
					0o644,
				); err != nil {
					return err
				}
				if err := os.WriteFile(
					filepath.Join(dir, "install.json"),
					[]byte(`{"bucket":"https://github.com/ScoopInstaller/Main","architecture":"64bit"}`),
					0o644,
				); err != nil {
					return err
				}
				return nil
			},
			wantState:  ownershipValid,
			wantBucket: "https://github.com/ScoopInstaller/Main",
		},
		{
			name: "only manifest present",
			setupFiles: func(dir string) error {
				return os.WriteFile(
					filepath.Join(dir, "manifest.json"),
					[]byte(`{"version":"1.2.3","url":"https://example.com/click.zip","hash":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}`),
					0o644,
				)
			},
			wantState: ownershipPartial,
		},
		{
			name: "only install present",
			setupFiles: func(dir string) error {
				return os.WriteFile(
					filepath.Join(dir, "install.json"),
					[]byte(`{"bucket":"https://github.com/ScoopInstaller/Main","architecture":"64bit"}`),
					0o644,
				)
			},
			wantState: ownershipPartial,
		},
		{
			name: "manifest malformed",
			setupFiles: func(dir string) error {
				if err := os.WriteFile(
					filepath.Join(dir, "manifest.json"),
					[]byte(`{"version":"1.2.3","url":"https://example.com/click.zip","hash":"not64chars"}`),
					0o644,
				); err != nil {
					return err
				}
				if err := os.WriteFile(
					filepath.Join(dir, "install.json"),
					[]byte(`{"bucket":"https://github.com/ScoopInstaller/Main","architecture":"64bit"}`),
					0o644,
				); err != nil {
					return err
				}
				return nil
			},
			wantState: ownershipInvalid,
		},
		{
			name: "install malformed",
			setupFiles: func(dir string) error {
				if err := os.WriteFile(
					filepath.Join(dir, "manifest.json"),
					[]byte(`{"version":"1.2.3","url":"https://example.com/click.zip","hash":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}`),
					0o644,
				); err != nil {
					return err
				}
				if err := os.WriteFile(
					filepath.Join(dir, "install.json"),
					[]byte(`{"bucket":"","architecture":"64bit"}`),
					0o644,
				); err != nil {
					return err
				}
				return nil
			},
			wantState: ownershipInvalid,
		},
		{
			name: "neither present",
			setupFiles: func(dir string) error {
				// Don't create any files
				return nil
			},
			wantState: ownershipNone,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := tt.setupFiles(dir); err != nil {
				t.Fatalf("failed to setup test files: %v", err)
			}

			state, bucket := probeScoopDirectory(dir)

			if state != tt.wantState {
				t.Errorf("probeScoopDirectory() state = %v, want %v", state, tt.wantState)
			}
			if tt.wantState == ownershipValid && bucket != tt.wantBucket {
				t.Errorf("probeScoopDirectory() bucket = %q, want %q", bucket, tt.wantBucket)
			}
		})
	}
}

// TestDetectInstallation_ScoopDirectMetadata tests direct Scoop detection via metadata.
func TestDetectInstallation_ScoopDirectMetadata(t *testing.T) {
	dir := t.TempDir()

	// Create fake executable
	fakeExe := filepath.Join(dir, "click.exe")
	if err := os.WriteFile(fakeExe, []byte("fake exe"), 0o755); err != nil {
		t.Fatalf("failed to create fake executable: %v", err)
	}

	// Create valid Scoop metadata
	if err := os.WriteFile(
		filepath.Join(dir, "manifest.json"),
		[]byte(`{"version":"1.2.3","url":"https://example.com/click.zip","hash":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}`),
		0o644,
	); err != nil {
		t.Fatalf("failed to create manifest: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(dir, "install.json"),
		[]byte(`{"bucket":"https://github.com/ScoopInstaller/Main","architecture":"64bit"}`),
		0o644,
	); err != nil {
		t.Fatalf("failed to create install.json: %v", err)
	}

	// Save and restore seams
	originalExecutablePath := executablePath
	originalEvalSymlinks := evalExecutableSymlinks
	originalStatExecutable := statExecutable

	executablePath = func() (string, error) { return fakeExe, nil }
	evalExecutableSymlinks = func(path string) (string, error) { return path, nil }
	statExecutable = os.Stat

	t.Cleanup(func() {
		executablePath = originalExecutablePath
		evalExecutableSymlinks = originalEvalSymlinks
		statExecutable = originalStatExecutable
	})

	install := DetectInstallation()

	if install.Method != InstallScoop {
		t.Errorf("DetectInstallation() Method = %v, want InstallScoop", install.Method)
	}
	if install.Executable != fakeExe {
		t.Errorf("DetectInstallation() Executable = %q, want %q", install.Executable, fakeExe)
	}
	if install.Bucket != "https://github.com/ScoopInstaller/Main" {
		t.Errorf("DetectInstallation() Bucket = %q, want https://github.com/ScoopInstaller/Main", install.Bucket)
	}
}

// TestDetectInstallation_ScoopViaShim tests Scoop detection via shim indirection.
func TestDetectInstallation_ScoopViaShim(t *testing.T) {
	// Create a directory with Scoop metadata
	scoopDir := t.TempDir()
	scoopExe := filepath.Join(scoopDir, "click.exe")
	if err := os.WriteFile(scoopExe, []byte("real exe"), 0o755); err != nil {
		t.Fatalf("failed to create real executable: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(scoopDir, "manifest.json"),
		[]byte(`{"version":"1.2.3","url":"https://example.com/click.zip","hash":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}`),
		0o644,
	); err != nil {
		t.Fatalf("failed to create manifest: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(scoopDir, "install.json"),
		[]byte(`{"bucket":"https://github.com/ScoopInstaller/Extras","architecture":"64bit"}`),
		0o644,
	); err != nil {
		t.Fatalf("failed to create install.json: %v", err)
	}

	// Create a shim directory with a shim file pointing to the Scoop install
	shimDir := t.TempDir()
	shimPath := filepath.Join(shimDir, "click.exe.shim")
	shimContent := `path = "` + scoopExe + `"`
	if err := os.WriteFile(shimPath, []byte(shimContent), 0o644); err != nil {
		t.Fatalf("failed to create shim: %v", err)
	}

	// Save and restore seams
	originalExecutablePath := executablePath
	originalEvalSymlinks := evalExecutableSymlinks
	originalStatExecutable := statExecutable
	originalReadOwnershipFile := readOwnershipFile

	executablePath = func() (string, error) { return shimPath, nil }
	evalExecutableSymlinks = func(path string) (string, error) { return path, nil }
	statExecutable = os.Stat

	t.Cleanup(func() {
		executablePath = originalExecutablePath
		evalExecutableSymlinks = originalEvalSymlinks
		statExecutable = originalStatExecutable
		readOwnershipFile = originalReadOwnershipFile
	})

	install := DetectInstallation()

	if install.Method != InstallScoop {
		t.Errorf("DetectInstallation() Method = %v, want InstallScoop", install.Method)
	}
	if install.Executable != scoopExe {
		t.Errorf("DetectInstallation() Executable = %q, want %q", install.Executable, scoopExe)
	}
	if install.Bucket != "https://github.com/ScoopInstaller/Extras" {
		t.Errorf("DetectInstallation() Bucket = %q, want https://github.com/ScoopInstaller/Extras", install.Bucket)
	}
}

// TestDetectInstallation_StandaloneWhenNoScoopArtifacts tests standalone detection.
func TestDetectInstallation_StandaloneWhenNoScoopArtifacts(t *testing.T) {
	dir := t.TempDir()
	fakeExe := filepath.Join(dir, "click.exe")
	if err := os.WriteFile(fakeExe, []byte("fake exe"), 0o755); err != nil {
		t.Fatalf("failed to create fake executable: %v", err)
	}

	// Save and restore seams
	originalExecutablePath := executablePath
	originalEvalSymlinks := evalExecutableSymlinks
	originalStatExecutable := statExecutable

	executablePath = func() (string, error) { return fakeExe, nil }
	evalExecutableSymlinks = func(path string) (string, error) { return path, nil }
	statExecutable = os.Stat

	t.Cleanup(func() {
		executablePath = originalExecutablePath
		evalExecutableSymlinks = originalEvalSymlinks
		statExecutable = originalStatExecutable
	})

	install := DetectInstallation()

	if install.Method != InstallStandalone {
		t.Errorf("DetectInstallation() Method = %v, want InstallStandalone", install.Method)
	}
	if install.Executable != fakeExe {
		t.Errorf("DetectInstallation() Executable = %q, want %q", install.Executable, fakeExe)
	}
	if install.Bucket != "" {
		t.Errorf("DetectInstallation() Bucket = %q, want empty string", install.Bucket)
	}
}

// TestDetectInstallation_UnknownOnAmbiguity tests various ambiguous cases.
func TestDetectInstallation_UnknownOnAmbiguity(t *testing.T) {
	tests := []struct {
		name         string
		setup        func(t *testing.T) func()
		wantMethod   InstallMethod
		wantExeEmpty bool
		wantBucket   string
	}{
		{
			name: "partial metadata (only manifest)",
			setup: func(t *testing.T) func() {
				dir := t.TempDir()
				fakeExe := filepath.Join(dir, "click.exe")
				if err := os.WriteFile(fakeExe, []byte("fake exe"), 0o755); err != nil {
					t.Fatalf("failed to create fake executable: %v", err)
				}
				if err := os.WriteFile(
					filepath.Join(dir, "manifest.json"),
					[]byte(`{"version":"1.2.3","url":"https://example.com/click.zip","hash":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}`),
					0o644,
				); err != nil {
					t.Fatalf("failed to create manifest: %v", err)
				}

				originalExecutablePath := executablePath
				originalEvalSymlinks := evalExecutableSymlinks
				originalStatExecutable := statExecutable

				executablePath = func() (string, error) { return fakeExe, nil }
				evalExecutableSymlinks = func(path string) (string, error) { return path, nil }
				statExecutable = os.Stat

				cleanup := func() {
					executablePath = originalExecutablePath
					evalExecutableSymlinks = originalEvalSymlinks
					statExecutable = originalStatExecutable
				}

				return cleanup
			},
			wantMethod:   InstallUnknown,
			wantExeEmpty: true,
			wantBucket:   "",
		},
		{
			name: "partial metadata (only install)",
			setup: func(t *testing.T) func() {
				dir := t.TempDir()
				fakeExe := filepath.Join(dir, "click.exe")
				if err := os.WriteFile(fakeExe, []byte("fake exe"), 0o755); err != nil {
					t.Fatalf("failed to create fake executable: %v", err)
				}
				if err := os.WriteFile(
					filepath.Join(dir, "install.json"),
					[]byte(`{"bucket":"https://github.com/ScoopInstaller/Main","architecture":"64bit"}`),
					0o644,
				); err != nil {
					t.Fatalf("failed to create install.json: %v", err)
				}

				originalExecutablePath := executablePath
				originalEvalSymlinks := evalExecutableSymlinks
				originalStatExecutable := statExecutable

				executablePath = func() (string, error) { return fakeExe, nil }
				evalExecutableSymlinks = func(path string) (string, error) { return path, nil }
				statExecutable = os.Stat

				cleanup := func() {
					executablePath = originalExecutablePath
					evalExecutableSymlinks = originalEvalSymlinks
					statExecutable = originalStatExecutable
				}

				return cleanup
			},
			wantMethod:   InstallUnknown,
			wantExeEmpty: true,
			wantBucket:   "",
		},
		{
			name: "shim target not a regular file",
			setup: func(t *testing.T) func() {
				shimDir := t.TempDir()
				shimPath := filepath.Join(shimDir, "click.exe.shim")
				shimContent := `path = "` + shimDir + `"`
				if err := os.WriteFile(shimPath, []byte(shimContent), 0o644); err != nil {
					t.Fatalf("failed to create shim: %v", err)
				}

				originalExecutablePath := executablePath
				originalEvalSymlinks := evalExecutableSymlinks
				originalStatExecutable := statExecutable

				executablePath = func() (string, error) { return shimPath, nil }
				evalExecutableSymlinks = func(path string) (string, error) { return path, nil }
				statExecutable = os.Stat

				cleanup := func() {
					executablePath = originalExecutablePath
					evalExecutableSymlinks = originalEvalSymlinks
					statExecutable = originalStatExecutable
				}

				return cleanup
			},
			wantMethod:   InstallUnknown,
			wantExeEmpty: true,
			wantBucket:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleanup := tt.setup(t)
			defer cleanup()

			install := DetectInstallation()

			if install.Method != tt.wantMethod {
				t.Errorf("DetectInstallation() Method = %v, want %v", install.Method, tt.wantMethod)
			}
			if tt.wantExeEmpty && install.Executable != "" {
				t.Errorf("DetectInstallation() Executable = %q, want empty string", install.Executable)
			}
			if install.Bucket != tt.wantBucket {
				t.Errorf("DetectInstallation() Bucket = %q, want %q", install.Bucket, tt.wantBucket)
			}
		})
	}
}

// TestDetectInstallation_IsIndependentOfUpdateCacheAndVersion pins that classification depends
// only on the filesystem, never on the update cache or the running version. An earlier revision
// gated detection on reading the cache plus a hardcoded placeholder version, so any machine
// without an update-check.json — every fresh install — reported Unknown even when Scoop owned the
// binary. Dev builds are excluded upstream by Check, not here.
func TestDetectInstallation_IsIndependentOfUpdateCacheAndVersion(t *testing.T) {
	dir := t.TempDir()
	fakeExe := filepath.Join(dir, "click.exe")
	if err := os.WriteFile(fakeExe, []byte("fake exe"), 0o755); err != nil {
		t.Fatalf("failed to create fake executable: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(dir, "manifest.json"),
		[]byte(`{"version":"1.2.3","url":"https://example.com/click.zip","hash":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}`),
		0o644,
	); err != nil {
		t.Fatalf("failed to create manifest: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(dir, "install.json"),
		[]byte(`{"bucket":"click","architecture":"64bit"}`),
		0o644,
	); err != nil {
		t.Fatalf("failed to create install.json: %v", err)
	}

	originalExecutablePath := executablePath
	originalEvalSymlinks := evalExecutableSymlinks
	originalStatExecutable := statExecutable
	originalResolveStateHome := resolveStateHome
	t.Cleanup(func() {
		executablePath = originalExecutablePath
		evalExecutableSymlinks = originalEvalSymlinks
		statExecutable = originalStatExecutable
		resolveStateHome = originalResolveStateHome
	})

	executablePath = func() (string, error) { return fakeExe, nil }
	evalExecutableSymlinks = func(path string) (string, error) { return path, nil }
	statExecutable = os.Stat
	// No resolvable state home at all: there is no cache to read. Scoop must still be detected.
	resolveStateHome = func() (string, error) { return "", errors.New("no state home") }

	got := DetectInstallation()
	if got.Method != InstallScoop {
		t.Errorf("DetectInstallation() Method = %v, want InstallScoop despite an unreadable state home", got.Method)
	}
	if got.Bucket != "click" {
		t.Errorf("DetectInstallation() Bucket = %q, want %q", got.Bucket, "click")
	}
}

// TestDetectInstallation_IgnoresEnvironmentAndPathNames tests that detection ignores
// SCOOP environment variables and directory names.
func TestDetectInstallation_IgnoresEnvironmentAndPathNames(t *testing.T) {
	// Create a directory literally named "scoop" (not a real Scoop install)
	scoopDir := t.TempDir()
	if filepath.Base(scoopDir) != "scoop" {
		// We need to create a directory with the exact name "scoop"
		scoopDir = filepath.Join(filepath.Dir(scoopDir), "scoop")
		if err := os.MkdirAll(scoopDir, 0o755); err != nil {
			t.Fatalf("failed to create scoop dir: %v", err)
		}
	}

	fakeExe := filepath.Join(scoopDir, "click.exe")
	if err := os.WriteFile(fakeExe, []byte("fake exe"), 0o755); err != nil {
		t.Fatalf("failed to create fake executable: %v", err)
	}

	// Set environment variables that should NOT influence detection
	t.Setenv("SCOOP", "/some/scoop/path")
	t.Setenv("SCOOP_GLOBAL", "/some/global/scoop/path")

	// Save and restore seams
	originalExecutablePath := executablePath
	originalEvalSymlinks := evalExecutableSymlinks
	originalStatExecutable := statExecutable

	executablePath = func() (string, error) { return fakeExe, nil }
	evalExecutableSymlinks = func(path string) (string, error) { return path, nil }
	statExecutable = os.Stat

	t.Cleanup(func() {
		executablePath = originalExecutablePath
		evalExecutableSymlinks = originalEvalSymlinks
		statExecutable = originalStatExecutable
	})

	install := DetectInstallation()

	// Should be standalone because there are no Scoop metadata files
	if install.Method != InstallStandalone {
		t.Errorf("DetectInstallation() Method = %v, want InstallStandalone (environment variables should not affect detection)", install.Method)
	}
	if install.Executable != fakeExe {
		t.Errorf("DetectInstallation() Executable = %q, want %q", install.Executable, fakeExe)
	}
	if install.Bucket != "" {
		t.Errorf("DetectInstallation() Bucket = %q, want empty string", install.Bucket)
	}
}
