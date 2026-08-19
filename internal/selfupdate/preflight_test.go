package selfupdate

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestProbeTargetDirectory_CreateCloseDeleteLifecycle(t *testing.T) {
	dir := t.TempDir()
	if err := probeTargetDirectory(filepath.Join(dir, "click")); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 0 {
		t.Fatalf("entries=%v err=%v", entries, err)
	}
	if err := probeTargetDirectory(filepath.Join(dir, "missing", "click")); err == nil {
		t.Fatal("expected create failure")
	}
}

func TestProbeTargetDirectory_CloseAndDeleteFailures(t *testing.T) {
	dir := t.TempDir()
	oldClose, oldRemove := closeTargetProbe, removeTargetProbe
	t.Cleanup(func() { closeTargetProbe, removeTargetProbe = oldClose, oldRemove })
	closeTargetProbe = func(file *os.File) error { _ = file.Close(); return errors.New("close failed") }
	if err := probeTargetDirectory(filepath.Join(dir, "click")); err == nil {
		t.Fatal("expected close failure")
	}
	closeTargetProbe = oldClose
	removeTargetProbe = func(string) error { return errors.New("delete failed") }
	if err := probeTargetDirectory(filepath.Join(dir, "click")); err == nil {
		t.Fatal("expected delete failure")
	}
}
