// OpenClaw skill synchronizer (clickhola-clickdev-triggers PR3).
//
// Click AI owns two small OpenClaw skill manifests — clickhola (Spanish idea-capture for non-
// technical requesters) and clickdev (Spanish developer hand-off bridge). This file synchronizes
// those manifests into OpenClaw's skills/ directory, mirroring the structure and safety patterns
// established by openclawplugin.go.
package installer

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
)

// openClawSkillsAssetsRoot is the embedded asset tree's root for OpenClaw skill manifests,
// relative to this package's own directory.
const openClawSkillsAssetsRoot = "assets/openclaw/skills"

//go:embed assets/openclaw/skills/clickhola/SKILL.md assets/openclaw/skills/clickdev/SKILL.md
var openClawSkillsAssets embed.FS

// openClawSkillRelPaths lists every skill manifest this synchronizer installs, relative to
// openClawSkillsAssetsRoot, using forward slashes (embed.FS's own path convention, GOOS-independent).
// It is the single source of truth for SyncOpenClawSkills and RemoveOpenClawSkills.
var openClawSkillRelPaths = []string{
	"clickhola/SKILL.md",
	"clickdev/SKILL.md",
}

// removeAll is the injectable seam behind RemoveOpenClawSkills's directory removal, mirroring
// openclawplugin.go's osExecutable pattern and allowing tests to inject a deterministic failure
// without relying on flaky OS-level permission races.
var removeAll = os.RemoveAll

// SyncOpenClawSkills writes/refreshes the click-owned OpenClaw skill manifests under
// cfg.OpenClawSkillsDir(): clickhola/SKILL.md and clickdev/SKILL.md, copied wholesale from the
// embedded assets/openclaw/skills tree. It is a no-op when cfg.OpenClawHome is empty.
//
// Idempotent by construction: re-running with unchanged embedded bytes produces byte-identical
// output on disk (same source bytes, same atomic write). Re-running after the on-disk files have
// drifted restores the owned bytes.
func SyncOpenClawSkills(cfg Config) error {
	if cfg.OpenClawHome == "" {
		return nil
	}
	destRoot := cfg.OpenClawSkillsDir()

	for _, rel := range openClawSkillRelPaths {
		assetPath := openClawSkillsAssetsRoot + "/" + rel
		data, readErr := openClawSkillsAssets.ReadFile(assetPath)
		if readErr != nil {
			return fmt.Errorf("installer: read embedded openclaw skill asset %s: %w", assetPath, readErr)
		}
		destPath := filepath.Join(destRoot, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
			return fmt.Errorf("installer: create dir for %s: %w", destPath, err)
		}
		if err := atomicWriteFile(destPath, data, 0o644); err != nil {
			return fmt.Errorf("installer: write openclaw skill file %s: %w", destPath, err)
		}
	}
	return nil
}

// RemoveOpenClawSkills removes the click-owned OpenClaw skill directories (clickhola, clickdev)
// under cfg.OpenClawSkillsDir(), leaving any user-created sibling skill directories untouched. It is
// idempotent: removing already-absent directories, or being called with cfg.OpenClawHome empty, is a
// no-op, never an error.
func RemoveOpenClawSkills(cfg Config) error {
	if cfg.OpenClawHome == "" {
		return nil
	}
	for _, name := range []string{"clickhola", "clickdev"} {
		dir := filepath.Join(cfg.OpenClawSkillsDir(), name)
		if err := removeAll(dir); err != nil {
			return fmt.Errorf("installer: remove openclaw skill dir %s: %w", dir, err)
		}
	}
	return nil
}
