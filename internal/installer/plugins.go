package installer

import (
	"fmt"
	"os"
	"path/filepath"

	clickstub "github.com/Angel-MercadoCLK/click-ai-devkit/plugins/click-stub"
)

// CopyStubPlugin copies the embedded click-stub plugin (plugins/click-stub, tracer bullet only —
// real plugins land in later slices) into cfg.PluginDir(). Safe to call repeatedly: it always
// overwrites with the embedded content, so re-running install never leaves stale files behind.
func CopyStubPlugin(cfg Config) error {
	dir := cfg.PluginDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("installer: create plugin dir %s: %w", dir, err)
	}

	entries, err := clickstub.Files.ReadDir(".")
	if err != nil {
		return fmt.Errorf("installer: read embedded stub plugin: %w", err)
	}

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := clickstub.Files.ReadFile(e.Name())
		if err != nil {
			return fmt.Errorf("installer: read embedded file %s: %w", e.Name(), err)
		}
		if err := os.WriteFile(filepath.Join(dir, e.Name()), data, 0o644); err != nil {
			return fmt.Errorf("installer: write plugin file %s: %w", e.Name(), err)
		}
	}
	return nil
}

// RemoveStubPlugin removes cfg.PluginDir() entirely. It is idempotent: removing an already-absent
// directory is not an error (os.RemoveAll's own contract).
func RemoveStubPlugin(cfg Config) error {
	if err := os.RemoveAll(cfg.PluginDir()); err != nil {
		return fmt.Errorf("installer: remove plugin dir %s: %w", cfg.PluginDir(), err)
	}
	return nil
}
