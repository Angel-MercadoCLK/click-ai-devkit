// OpenClaw skill synchronizer.
//
// Click synchronizes only its retained OpenClaw business skill. Legacy workflow assets are
// migrated separately and only when their bytes match the pinned shipped templates.
package installer

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// openClawSkillsAssetsRoot is the embedded asset tree's root for OpenClaw skill manifests,
// relative to this package's own directory.
const openClawSkillsAssetsRoot = "assets/openclaw/skills"

//go:embed assets/openclaw/skills
var openClawSkillsAssets embed.FS

// openClawSkillRelPaths lists every retained skill manifest this synchronizer installs, relative to
// openClawSkillsAssetsRoot, using forward slashes (embed.FS's own path convention, GOOS-independent).
// Retired assets are deliberately not part of this active list.
var openClawSkillRelPaths = []string{
	"clickhola/SKILL.md",
}

// retiredOpenClawSkills pins exact shipped bytes independently of the embedded source tree. The
// digests are migration signatures, not proof of who installed a matching file.
var retiredOpenClawSkills = []retiredOpenClawSkill{
	{rel: "clickdev/SKILL.md", sha256: "8b880e1b56e523cd28ccee0fc6055d46a750e7c20cdc39f5fd02ddfe902321d1"},
	{rel: "click-sdd/SKILL.md", sha256: "aede4f400a28b204c4f737a046d8ff4a3ae6a70605259bb0793f7aeb1d89fdb2"},
	{rel: "click-sdd-explore/SKILL.md", sha256: "a388d5ca09843a08b90357c79c96651d9ac63e6137ef5501139195e0f0fe23ba"},
	{rel: "click-sdd-propose/SKILL.md", sha256: "224eb2970dfdcad698e359a815618c81b7a7652fa7b26bb4ee35e826a0fa8197"},
	{rel: "click-sdd-spec/SKILL.md", sha256: "7cf2c710e23286007388a91e5e10544455f89322253dbb56792efedc69f2e27e"},
	{rel: "click-sdd-design/SKILL.md", sha256: "cf4b59f19c40e64b6d3a16a7aa278e6d39b86cc36a1e12cb1cc9e26eee34a2bd"},
	{rel: "click-sdd-tasks/SKILL.md", sha256: "6bb34cb1edcc19e2e075e8b38b97ca43f8e582622c52c2b1ed26579a2105ec79"},
	{rel: "click-sdd-apply/SKILL.md", sha256: "f495f19d1d1aed2e5b21234b5b551e272b34c726c95760935e6a2eb976bdaaef"},
	{rel: "click-sdd-verify/SKILL.md", sha256: "911d7a66bceab2e82b5192ca73db06fca0aa70c716e706a78bfabaa20c864a23"},
	{rel: "click-sdd-archive/SKILL.md", sha256: "d314f4af6693480abb6cc98d4be87113f2d6e1ad06e17c0072c60b50439e069a"},
	{rel: "click-sdd-onboard/SKILL.md", sha256: "886641e3dd4577b7217da2df52cc1dbd2f43858ebb0fbd17877654d240124d69"},
}

type retiredOpenClawSkill struct {
	rel    string
	sha256 string
}

func retiredOpenClawSkillPaths() []string {
	paths := make([]string, len(retiredOpenClawSkills))
	for i, entry := range retiredOpenClawSkills {
		paths[i] = entry.rel
	}
	return paths
}

func isRetiredOpenClawSkillPath(cfg Config, path string) bool {
	if cfg.OpenClawHome == "" {
		return false
	}
	for _, rel := range retiredOpenClawSkillPaths() {
		if filepath.Clean(path) == filepath.Clean(filepath.Join(cfg.OpenClawSkillsDir(), filepath.FromSlash(rel))) {
			return true
		}
	}
	return false
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

// SyncOpenClawSkills writes/refreshes the retained Click OpenClaw skill manifests under
// cfg.OpenClawSkillsDir(), copied wholesale from the embedded assets/openclaw/skills tree. It is
// a no-op when cfg.OpenClawHome is empty.
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

// RemoveOpenClawSkills removes actively managed OpenClaw skill files under cfg.OpenClawSkillsDir(),
// leaving any user-created sibling skill directories and files untouched. It is idempotent:
// removing already-absent files, or being called with cfg.OpenClawHome empty, is a no-op.
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
	ownedDirs := map[string]struct{}{}
	for _, rel := range openClawSkillRelPaths {
		ownedDirs[filepath.Dir(filepath.FromSlash(rel))] = struct{}{}
	}
	for name := range ownedDirs {
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

// RetireLegacyOpenClawSkills removes only legacy Click skill files whose content matches a pinned
// shipped template. Modified, unknown, symlinked, and non-regular entries remain untouched and
// produce Spanish warnings. No directory tree is removed.
func RetireLegacyOpenClawSkills(cfg Config) ([]string, error) {
	if cfg.OpenClawHome == "" {
		return nil, nil
	}
	return retireLegacyOpenClawSkills(cfg, retiredOpenClawSkills)
}

func retireLegacyOpenClawSkills(cfg Config, entries []retiredOpenClawSkill) ([]string, error) {
	if cfg.OpenClawHome == "" {
		return nil, nil
	}
	warnings := make([]string, 0)
	for _, entry := range entries {
		cleanRel := filepath.Clean(filepath.FromSlash(entry.rel))
		if entry.rel == "" || filepath.IsAbs(cleanRel) || cleanRel == ".." || strings.HasPrefix(cleanRel, ".."+string(filepath.Separator)) {
			return warnings, fmt.Errorf("installer: ruta heredada de skill de OpenClaw no válida %q", entry.rel)
		}
		digest, err := hex.DecodeString(entry.sha256)
		if err != nil || len(digest) != sha256.Size {
			return warnings, fmt.Errorf("installer: digest fijado no válido para skill heredada de OpenClaw %q", entry.rel)
		}
		path := filepath.Join(cfg.OpenClawSkillsDir(), cleanRel)
		info, safe, inspectErr := inspectRetirementPath(path)
		if inspectErr != nil {
			return warnings, fmt.Errorf("installer: inspeccionar skill heredada de OpenClaw %s: %w", entry.rel, inspectErr)
		}
		if !safe {
			warnings = append(warnings, fmt.Sprintf("Se conservó la ruta heredada de OpenClaw %s porque contiene un enlace simbólico o un elemento que no es un archivo regular.", entry.rel))
			continue
		}
		if info == nil {
			continue
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return warnings, fmt.Errorf("installer: leer skill heredada de OpenClaw %s: %w", entry.rel, readErr)
		}
		sum := sha256.Sum256(data)
		if hex.EncodeToString(sum[:]) != entry.sha256 {
			warnings = append(warnings, fmt.Sprintf("Se conservó la skill heredada de OpenClaw %s porque su contenido ya no coincide con la versión distribuida por Click.", entry.rel))
			continue
		}
		if err := os.Remove(path); err != nil {
			return warnings, fmt.Errorf("installer: eliminar skill heredada de OpenClaw coincidente %s: %w", entry.rel, err)
		}
	}
	return warnings, nil
}

// inspectRetirementPath checks every path component with Lstat so cleanup and its snapshot never
// follow a symlinked OpenClaw root, skills directory, retired skill directory, or leaf.
func inspectRetirementPath(path string) (os.FileInfo, bool, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, false, err
	}
	volume := filepath.VolumeName(abs)
	remainder := strings.TrimPrefix(abs, volume)
	current := volume + string(filepath.Separator)
	parts := strings.Split(strings.TrimPrefix(remainder, string(filepath.Separator)), string(filepath.Separator))
	for i, part := range parts {
		if part == "" {
			continue
		}
		current = filepath.Join(current, part)
		info, statErr := os.Lstat(current)
		if os.IsNotExist(statErr) {
			return nil, true, nil
		}
		if statErr != nil {
			return nil, false, statErr
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, false, nil
		}
		last := i == len(parts)-1
		if last {
			if !info.Mode().IsRegular() {
				return nil, false, nil
			}
			return info, true, nil
		}
		if !info.IsDir() {
			return nil, false, nil
		}
	}
	return nil, true, nil
}
