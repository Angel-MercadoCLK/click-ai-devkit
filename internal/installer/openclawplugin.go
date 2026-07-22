// OpenClaw memory-guard parity plugin (design #1666's "ADDED PIECE: OpenClaw memory-guard parity",
// decisions OCG-1..6, PR-C / tasks 3.1-3.14).
//
// Claude Code routes every Engram mem_save call through click memory-guard via a PreToolUse hook
// (hooksettings.go's RegisterMemoryGuardHook). OpenClaw has no equivalent gate on its own, so any
// OpenClaw agent's mem_save call is unguarded. This file closes that gap by installing a small,
// static, click-owned OpenClaw plugin (plugins/hooks.js, an OpenClaw before_tool_call adapter) that
// shells out to the SAME already-tested `click memory-guard` binary — this file and hooks.js
// deliberately NEVER reimplement internal/guard's scanning logic (OCG-2).
package installer

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// openClawPluginAssetsRoot is the embedded asset tree's root, relative to this package's own
// directory.
const openClawPluginAssetsRoot = "assets/openclaw/click-memory-guard"

//go:embed assets/openclaw/click-memory-guard
var openClawPluginAssets embed.FS

// clickBinPlaceholder is the exact substring hooks.js expects to be replaced with the resolved
// absolute click binary path at install time (OCG-5). Templating is applied to every embedded file
// that CONTAINS this placeholder — currently only hooks.js — so a second templated file later would
// need no Go change, just the placeholder appearing in its own content.
const clickBinPlaceholder = "{{CLICK_BIN}}"

// openClawPluginRelPaths lists every file this plugin installs, relative to
// openClawPluginAssetsRoot, using forward slashes (embed.FS's own path convention, GOOS-independent
// — Go's embed package always uses "/" regardless of the build platform). This is the SINGLE source
// of truth both SyncOpenClawPlugin (which files to write) and snapshotSources (which files to back
// up) iterate, so the two can never drift out of sync with each other.
//
// hooks.test.js is deliberately embedded (it lives under the same directory, and go:embed without
// an "all:" prefix still includes any file not starting with "." or "_") but NOT listed here — it is
// this plugin's own test file, never meant to be installed alongside the real plugin. Its presence
// in the embed.FS costs a few KB of binary size and nothing else; it is simply never read by
// SyncOpenClawPlugin because it never appears in this list.
//
// Kept as a fixed literal rather than derived via fs.WalkDir at either compile or run time, matching
// this codebase's "no abstraction beyond what today's fixed shape needs" convention (design's
// "Config shape" decision: 2 targets only, YAGNI) — adding a third installed file later means adding
// one entry here, not touching any calling code.
var openClawPluginRelPaths = []string{
	"plugins/hooks.js",
	"plugin.json",
}

// osExecutable is the injectable seam behind SyncOpenClawPlugin's os.Executable() call (OCG-5),
// mirroring this codebase's binaryLookupFactory/commandRunnerFactory pattern, so tests can simulate
// a resolution failure or a specific resolved path without depending on the real test binary's own
// path on disk.
var osExecutable = os.Executable

// SetOSExecutableForTests overrides osExecutable for tests and returns a restore function.
func SetOSExecutableForTests(fn func() (string, error)) func() {
	old := osExecutable
	osExecutable = fn
	return func() { osExecutable = old }
}

// resolveClickBinaryPath resolves the absolute click binary path via execFunc, falling back to the
// bare "click" command name (OCG-5) when execFunc fails or returns an empty path — a stripped or
// unusual build environment where os.Executable() cannot resolve a path. In the fallback case
// hooks.js still works as long as OpenClaw's node runtime resolves "click" from its own PATH; only
// the absolute-path robustness guarantee is lost, never correctness.
func resolveClickBinaryPath(execFunc func() (string, error)) string {
	path, err := execFunc()
	if err != nil || path == "" {
		return "click"
	}
	return path
}

// escapeForJSSingleQuotedString makes s safe to embed inside hooks.js's existing single-quoted
// `'{{CLICK_BIN}}'` string literal. This matters concretely on Windows: an unescaped backslash
// before a non-special character inside a JS string literal is silently DROPPED by JS's own string
// parsing (e.g. the two characters `\U` become just `U`), which would corrupt a path like
// `C:\Users\...\click.exe` the moment Node loads hooks.js — not a hypothetical, this session's own
// platform is Windows. Backslashes are doubled first (so they survive as literal backslashes), then
// any single quote is escaped (defense in depth against a path that legitimately contains one).
func escapeForJSSingleQuotedString(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `'`, `\'`)
	return s
}

// openClawPluginBackupFileName derives snapshotSources' backupFile name for one plugin asset's
// relative path — deterministic, collision-free with the fixed "CLAUDE.md"/"settings.json"/
// "AGENTS.md"/"SOUL.md"/"openclaw.json" names snapshotSources already uses, and stable across runs
// since it is a pure function of rel.
func openClawPluginBackupFileName(rel string) string {
	return "openclaw-plugin-" + strings.ReplaceAll(rel, "/", "-")
}

// SyncOpenClawPlugin writes/refreshes the click-memory-guard OpenClaw plugin under
// cfg.OpenClawPluginDir(): every file in openClawPluginRelPaths, copied WHOLESALE from the embedded
// assets/openclaw/click-memory-guard tree (no managed-block markers — OCG-4's decision, this is
// entirely click-owned content, unlike AGENTS.md/SOUL.md), with clickBinPlaceholder templated to the
// resolved absolute click binary path (JS-string-escaped) in any file that contains it. It is a
// no-op when cfg.OpenClawHome is empty, mirroring SyncOpenClawWorkspace/SyncOpenClawMCPConfig's
// guard.
//
// Idempotent by construction (task 3.6): re-running with an unchanged resolved click binary path
// produces byte-identical output on disk (same embedded source bytes, same substitution, same
// atomic write). Re-running AFTER the click binary moves (a real, supported scenario — `click
// update` re-resolves osExecutable() every run) re-templates to the new path; that is the intended,
// correct behavior, not drift.
func SyncOpenClawPlugin(cfg Config) error {
	if cfg.OpenClawHome == "" {
		return nil
	}
	clickBin := escapeForJSSingleQuotedString(resolveClickBinaryPath(osExecutable))
	destRoot := cfg.OpenClawPluginDir()

	for _, rel := range openClawPluginRelPaths {
		assetPath := openClawPluginAssetsRoot + "/" + rel
		data, readErr := openClawPluginAssets.ReadFile(assetPath)
		if readErr != nil {
			return fmt.Errorf("installer: read embedded openclaw plugin asset %s: %w", assetPath, readErr)
		}
		if strings.Contains(string(data), clickBinPlaceholder) {
			data = []byte(strings.ReplaceAll(string(data), clickBinPlaceholder, clickBin))
		}
		destPath := filepath.Join(destRoot, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
			return fmt.Errorf("installer: create dir for %s: %w", destPath, err)
		}
		if err := atomicWriteFile(destPath, data, 0o644); err != nil {
			return fmt.Errorf("installer: write openclaw plugin file %s: %w", destPath, err)
		}
	}
	return nil
}

// RemoveOpenClawPlugin removes the entire click-memory-guard plugin directory under
// cfg.OpenClawPluginDir(), mirroring how other click-owned artifacts get torn down on `click
// uninstall` (parity with UnregisterMemoryGuardHook/StripManagedBlock — task 3.13). It is
// idempotent: removing an already-absent directory, or being called with cfg.OpenClawHome empty, is
// a no-op, never an error — `click uninstall` must be safe to run even on a machine that never had
// OpenClaw, or where OpenClaw was already removed by other means.
func RemoveOpenClawPlugin(cfg Config) error {
	if cfg.OpenClawHome == "" {
		return nil
	}
	if err := os.RemoveAll(cfg.OpenClawPluginDir()); err != nil {
		return fmt.Errorf("installer: remove openclaw plugin dir %s: %w", cfg.OpenClawPluginDir(), err)
	}
	return nil
}
