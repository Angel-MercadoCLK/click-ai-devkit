package installer

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	clickmemory "github.com/Angel-MercadoCLK/click-ai-devkit/plugins/click-memory"
	clickreview "github.com/Angel-MercadoCLK/click-ai-devkit/plugins/click-review"
	clicksdd "github.com/Angel-MercadoCLK/click-ai-devkit/plugins/click-sdd"
)

// CopyClickSDDPlugin copies the embedded click-sdd plugin into cfg.ClickSDDPluginDir(). Safe to
// call repeatedly: it rewrites the embedded content, so re-running install never leaves stale
// files behind for the shipped tree.
func CopyClickSDDPlugin(cfg Config) error {
	return copyEmbeddedTree(clicksdd.Files, cfg.ClickSDDPluginDir())
}

// CopyClickMemoryPlugin copies the embedded click-memory plugin into cfg.ClickMemoryPluginDir().
func CopyClickMemoryPlugin(cfg Config) error {
	return copyEmbeddedTree(clickmemory.Files, cfg.ClickMemoryPluginDir())
}

// CopyClickReviewPlugin copies the embedded click-review plugin into cfg.ClickReviewPluginDir().
func CopyClickReviewPlugin(cfg Config) error {
	return copyEmbeddedTree(clickreview.Files, cfg.ClickReviewPluginDir())
}

// RemoveClickSDDPlugin removes cfg.ClickSDDPluginDir() entirely. It is idempotent.
func RemoveClickSDDPlugin(cfg Config) error {
	if err := os.RemoveAll(cfg.ClickSDDPluginDir()); err != nil {
		return fmt.Errorf("installer: remove plugin dir %s: %w", cfg.ClickSDDPluginDir(), err)
	}
	return nil
}

// RemoveClickMemoryPlugin removes cfg.ClickMemoryPluginDir() entirely. It is idempotent.
func RemoveClickMemoryPlugin(cfg Config) error {
	if err := os.RemoveAll(cfg.ClickMemoryPluginDir()); err != nil {
		return fmt.Errorf("installer: remove plugin dir %s: %w", cfg.ClickMemoryPluginDir(), err)
	}
	return nil
}

// RemoveClickReviewPlugin removes cfg.ClickReviewPluginDir() entirely. It is idempotent.
func RemoveClickReviewPlugin(cfg Config) error {
	if err := os.RemoveAll(cfg.ClickReviewPluginDir()); err != nil {
		return fmt.Errorf("installer: remove plugin dir %s: %w", cfg.ClickReviewPluginDir(), err)
	}
	return nil
}

func copyEmbeddedTree(source fs.FS, destinationRoot string) error {
	if err := os.MkdirAll(destinationRoot, 0o755); err != nil {
		return fmt.Errorf("installer: create plugin dir %s: %w", destinationRoot, err)
	}

	return fs.WalkDir(source, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("installer: walk embedded tree %s: %w", path, err)
		}
		if path == "." {
			return nil
		}

		target := filepath.Join(destinationRoot, filepath.FromSlash(path))
		if d.IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return fmt.Errorf("installer: create embedded dir %s: %w", target, err)
			}
			return nil
		}

		data, readErr := fs.ReadFile(source, path)
		if readErr != nil {
			return fmt.Errorf("installer: read embedded file %s: %w", path, readErr)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fmt.Errorf("installer: create parent dir %s: %w", target, err)
		}
		if err := os.WriteFile(target, data, 0o644); err != nil {
			return fmt.Errorf("installer: write embedded file %s: %w", target, err)
		}
		return nil
	})
}
