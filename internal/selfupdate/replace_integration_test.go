//go:build selfupdate_integration

package selfupdate

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestRunningExecutableReplacement(t *testing.T) {
	dir := t.TempDir()
	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}
	target, staged := filepath.Join(dir, "click"+ext), filepath.Join(dir, "staged"+ext)
	buildHelper(t, target, "0.6.0")
	buildHelper(t, staged, "0.7.0")
	cmd := exec.Command(target, "--sleep")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _, _ = cmd.Process.Wait() }()
	time.Sleep(100 * time.Millisecond)
	err := replaceAndValidate(target, staged, "0.6.0", "0.7.0")
	if err != nil {
		t.Fatal(err)
	}
	if err := validateExecutable(target, "0.7.0"); err != nil {
		t.Fatal(err)
	}
}

func buildHelper(t *testing.T, output, version string) {
	t.Helper()
	cmd := exec.Command("go", "build", "-ldflags", "-X main.version="+version, "-o", output, "./internal/selfupdate/testdata/selfupdatehelper")
	cmd.Dir = filepath.Join("..", "..")
	if data, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build helper: %v\n%s", err, data)
	}
	if _, err := os.Stat(output); err != nil {
		t.Fatal(err)
	}
}
