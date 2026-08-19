package selfupdate

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func cleanupStaleArtifacts(target string) error {
	if err := cleanupBackup(target); err != nil {
		return err
	}
	dir, base := filepath.Dir(target), filepath.Base(target)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read update directory: %w", err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, base+".download-") && !strings.HasPrefix(name, base+".stage-") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("inspect update artifact %q: %w", name, err)
		}
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			continue
		}
		path := filepath.Join(dir, name)
		if err := retryFileOperation(func() error { return removeExecutable(path) }); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove stale update artifact %q: %w", path, err)
		}
	}
	return nil
}

func cleanupBackup(target string) error {
	backup := target + ".old"
	info, err := os.Lstat(backup)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect update backup: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("update backup is not a regular file: %s", backup)
	}
	if err := retryFileOperation(func() error { return removeExecutable(backup) }); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove stale update backup: %w", err)
	}
	return nil
}
