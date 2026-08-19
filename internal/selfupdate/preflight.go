package selfupdate

import (
	"fmt"
	"os"
	"path/filepath"
)

var (
	createTargetProbe = os.CreateTemp
	closeTargetProbe  = func(file *os.File) error { return file.Close() }
	removeTargetProbe = os.Remove
)

// probeTargetDirectory proves create, close, and removal permissions without
// inferring Windows ACL behavior from Unix mode bits.
func probeTargetDirectory(target string) error {
	f, err := createTargetProbe(filepath.Dir(target), ".click-update-probe-")
	if err != nil {
		return fmt.Errorf("create update probe: %w", err)
	}
	path := f.Name()
	if err := closeTargetProbe(f); err != nil {
		_ = removeTargetProbe(path)
		return fmt.Errorf("close update probe: %w", err)
	}
	if err := removeTargetProbe(path); err != nil {
		return fmt.Errorf("remove update probe: %w", err)
	}
	return nil
}
