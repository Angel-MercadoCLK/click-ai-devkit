package selfupdate

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCleanupStaleArtifacts_ScopedRemoval(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "click")
	if err := os.WriteFile(target, []byte("current"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"click.download-a", "click.stage-a", "other.stage-a"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(name), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "click.stage-dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := cleanupStaleArtifacts(target); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"click.download-a", "click.stage-a"} {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Errorf("%s was not removed", name)
		}
	}
	for _, name := range []string{"click", "other.stage-a", "click.stage-dir"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("%s removed: %v", name, err)
		}
	}
	if err := cleanupStaleArtifacts(target); err != nil {
		t.Fatalf("idempotence: %v", err)
	}
}

func TestCleanupStaleArtifacts_LockedOldBlocksNewTransactionBeforeNetwork(t *testing.T) {
	target, staged := replacementFiles(t)
	if err := os.WriteFile(target+".old", []byte("previous"), 0o755); err != nil {
		t.Fatal(err)
	}
	originalRemove := removeExecutable
	removeExecutable = func(path string) error {
		if path == target+".old" {
			return os.ErrPermission
		}
		return originalRemove(path)
	}
	t.Cleanup(func() { removeExecutable = originalRemove })
	if err := replaceAndValidate(target, staged, "0.6.0", "0.7.0"); err == nil {
		t.Fatal("expected stale backup to block transaction")
	}
	if _, err := os.Stat(target + ".old"); err != nil {
		t.Fatalf("backup removed: %v", err)
	}
}
