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
	"strings"
)

// openClawSkillsAssetsRoot is the embedded asset tree's root for OpenClaw skill manifests,
// relative to this package's own directory.
const openClawSkillsAssetsRoot = "assets/openclaw/skills"

//go:embed assets/openclaw/skills/clickhola/SKILL.md assets/openclaw/skills/clickdev/SKILL.md
var openClawSkillsAssets embed.FS

// openClawSkillRelPaths lists every skill manifest this synchronizer installs, relative to
// openClawSkillsAssetsRoot, using forward slashes (embed.FS's own path convention, GOOS-independent).
// It is the single source of truth for SyncOpenClawSkills, RemoveOpenClawSkills, and
// snapshotSources (which backs these files up for rollback).
var openClawSkillRelPaths = []string{
	"clickhola/SKILL.md",
	"clickdev/SKILL.md",
}

// openClawSkillBackupFileName derives snapshotSources' backupFile name for one skill asset's
// relative path — deterministic, collision-free with the fixed names snapshotSources already uses,
// and stable across runs since it is a pure function of rel.
func openClawSkillBackupFileName(rel string) string {
	return "openclaw-skill-" + strings.ReplaceAll(rel, "/", "-")
}

// removeAll is the injectable seam behind RemoveOpenClawSkills's owned-file removal, mirroring
// openclawplugin.go's osExecutable pattern and allowing tests to inject a deterministic failure
// without relying on flaky OS-level permission races.
var removeAll = os.Remove

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
	for _, rel := range openClawSkillRelPaths {
		path := filepath.Join(cfg.OpenClawSkillsDir(), filepath.FromSlash(rel))
		info, err := os.Lstat(path)
		if err != nil {
			if os.IsNotExist(err) {
				if removeErr := removeAll(path); removeErr != nil && !os.IsNotExist(removeErr) {
					return fmt.Errorf("installer: remove openclaw skill dir contents %s: %w", path, removeErr)
				}
				continue
			}
			return fmt.Errorf("installer: inspect openclaw skill dir %s: %w", path, err)
		}
		if !info.Mode().IsRegular() {
			continue
		}
		if err := removeAll(path); err != nil {
			return fmt.Errorf("installer: remove openclaw skill dir contents %s: %w", path, err)
		}
	}
	for _, name := range []string{"clickhola", "clickdev"} {
		dir := filepath.Join(cfg.OpenClawSkillsDir(), name)
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("installer: inspect openclaw skill dir %s: %w", dir, err)
		}
		if len(entries) == 0 {
			if err := os.Remove(dir); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("installer: remove openclaw skill dir %s: %w", dir, err)
			}
		}
	}
	return nil
}
