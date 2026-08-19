package selfupdate

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestParseVersionOutput_SingleTrimmedCobraLine(t *testing.T) {
	for _, tt := range []struct {
		in   string
		want string
		fail bool
	}{
		{" click version 0.7.0 \n", "0.7.0", false}, {"", "", true}, {"click version dev", "", true}, {"click version 0.7.0\nextra", "", true},
	} {
		got, err := parseVersionOutput([]byte(tt.in))
		if (err != nil) != tt.fail || got != tt.want {
			t.Errorf("%q got %q %v", tt.in, got, err)
		}
	}
}

func TestValidateExecutable_SpawnExitAndMismatchFailures(t *testing.T) {
	original := runVersionCommand
	t.Cleanup(func() { runVersionCommand = original })
	for _, tt := range []struct {
		name   string
		output []byte
		err    error
	}{
		{"spawn", nil, errors.New("spawn")}, {"nonzero", nil, errors.New("exit status 1")}, {"mismatch", []byte("click version 0.6.0\n"), nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			runVersionCommand = func(string) ([]byte, error) { return tt.output, tt.err }
			if err := validateExecutable("fake", "0.7.0"); err == nil {
				t.Fatal("expected validation failure")
			}
		})
	}
}

func TestReplaceAndValidate_BackupRenameFails_TargetUntouched(t *testing.T) {
	target, staged := replacementFiles(t)
	original := mustRead(t, target)
	withReplacementSeams(t, func(from, to string) error { return errors.New("backup denied") }, func(string) ([]byte, error) { return []byte("click version 0.6.0\n"), nil })
	if err := replaceAndValidate(target, staged, "0.6.0", "0.7.0"); err == nil {
		t.Fatal("expected error")
	}
	if got := string(mustRead(t, target)); got != string(original) {
		t.Fatalf("target changed: %q", got)
	}
	if _, err := os.Stat(staged); !os.IsNotExist(err) {
		t.Fatal("staged file not removed")
	}
}

func TestReplaceAndValidate_PlacementFails_RollbackRestoresAndValidates(t *testing.T) {
	target, staged := replacementFiles(t)
	calls := 0
	withReplacementSeams(t, func(from, to string) error {
		calls++
		if from == staged && to == target {
			return errors.New("place denied")
		}
		return os.Rename(from, to)
	}, func(path string) ([]byte, error) {
		if path != target {
			t.Fatalf("validated %q", path)
		}
		return []byte("click version 0.6.0\n"), nil
	})
	if err := replaceAndValidate(target, staged, "0.6.0", "0.7.0"); err == nil {
		t.Fatal("expected error")
	}
	if got := string(mustRead(t, target)); got != "old" {
		t.Fatalf("got %q", got)
	}
	if calls < 3 {
		t.Fatalf("rename calls=%d, rollback not attempted", calls)
	}
}

func TestReplaceAndValidate_ValidationFails_RestoresOldVersion(t *testing.T) {
	target, staged := replacementFiles(t)
	withReplacementSeams(t, os.Rename, func(string) ([]byte, error) { return []byte("click version 0.6.0\n"), nil })
	if err := replaceAndValidate(target, staged, "0.6.0", "0.7.0"); err == nil {
		t.Fatal("expected error")
	}
	if got := string(mustRead(t, target)); got != "old" {
		t.Fatalf("got %q", got)
	}
}

func TestReplaceAndValidate_PlacementAndRollbackBothFail_PreservesOldWithManualRecoveryError(t *testing.T) {
	target, staged := replacementFiles(t)
	withReplacementSeams(t, func(from, to string) error {
		if from == target || from == staged || from == target+".old" {
			return errors.New("denied")
		}
		return os.Rename(from, to)
	}, func(string) ([]byte, error) { return nil, errors.New("unused") })
	// First make backup real: reject only placement and rollback after it.
	renameExecutable = func(from, to string) error {
		if from == target && to == target+".old" {
			return os.Rename(from, to)
		}
		return errors.New("denied")
	}
	err := replaceAndValidate(target, staged, "0.6.0", "0.7.0")
	if err == nil || !strings.Contains(err.Error(), "manual recovery") {
		t.Fatalf("err=%v", err)
	}
	if _, err := os.Stat(target + ".old"); err != nil {
		t.Fatalf("backup lost: %v", err)
	}
}

func TestReplaceAndValidate_HappyPath_CommitsAndBestEffortCleansBackup(t *testing.T) {
	target, staged := replacementFiles(t)
	withReplacementSeams(t, os.Rename, func(string) ([]byte, error) { return []byte("click version 0.7.0\n"), nil })
	if err := replaceAndValidate(target, staged, "0.6.0", "0.7.0"); err != nil {
		t.Fatal(err)
	}
	if got := string(mustRead(t, target)); got != "new" {
		t.Fatalf("got %q", got)
	}
	if _, err := os.Stat(target + ".old"); !os.IsNotExist(err) {
		t.Fatal("backup not cleaned")
	}
}

func TestPlatformPlace_UnixStagesMode0755(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix mode assertion")
	}
	target, staged := replacementFiles(t)
	tx := &replacementTransaction{Target: target, Staged: staged, Backup: target + ".old"}
	if err := platformPlace(tx); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("mode=%#o", info.Mode().Perm())
	}
}

func replacementFiles(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	target, staged := filepath.Join(dir, "click"), filepath.Join(dir, "staged-new")
	if err := os.WriteFile(target, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(staged, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	return target, staged
}
func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
func withReplacementSeams(t *testing.T, rename func(string, string) error, run func(string) ([]byte, error)) {
	t.Helper()
	oldRename, oldRun := renameExecutable, runVersionCommand
	renameExecutable, runVersionCommand = rename, run
	t.Cleanup(func() { renameExecutable, runVersionCommand = oldRename, oldRun })
}
