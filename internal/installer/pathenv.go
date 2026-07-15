// PR1 of the engram-mcp-resolution chain: the OS-agnostic pathStore abstraction and its pure
// helpers. Windows (`pathenv_windows.go`) and POSIX (`pathenv_unix.go`) implementations land in
// later PRs of this chain and each assign pathStoreFactory via a build-tagged init(). This file
// intentionally never references a concrete platform type, so it builds and tests standalone.
//
// Scope note: D-9 (ownership tracking + PATH-reversal on `click uninstall`) is explicitly OUT OF
// SCOPE for v0.4.2 per Judgment Day Round 2 FINAL (sdd/engram-mcp-resolution — obs #1438) — the
// pathStore interface below intentionally has no RemoveFromPath method.
package installer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// pathStore abstracts persisting a directory onto the user's *persisted* PATH — Windows registry
// (HKCU\Environment) or POSIX shell rc files, depending on build tag. It is an interface (rather
// than free functions) so the signal-wiring in EnsureEngramBinary/SyncEngram can inject a fake in
// tests, matching this package's existing CommandRunner/BinaryLookup injectable-factory pattern.
type pathStore interface {
	// PersistedPathContains reports whether dir is already present in the persisted user PATH.
	PersistedPathContains(dir string) (bool, error)
	// EnsureOnPath adds dir to the persisted user PATH if not already present. changed reports
	// whether a mutation actually happened (false on a no-op idempotent re-run).
	EnsureOnPath(dir string) (changed bool, err error)
}

// pathStoreFactory returns the platform-specific pathStore implementation. It has no default
// value here: the concrete osPathStore type — and this var's assignment — is provided by the
// OS-specific pathenv_windows.go / pathenv_unix.go files added in later PRs of this chain. Until
// one of those is compiled in, pathStoreFactory is nil; nothing in this PR calls it yet (that
// wiring lands in the Phase 4 signal-wiring PR).
var pathStoreFactory func() pathStore

// SetPathStoreFactoryForTests overrides the pathStore factory for tests and returns a restore
// function, mirroring SetCommandRunnerFactoryForTests / SetBinaryLookupFactoryForTests.
func SetPathStoreFactoryForTests(factory func() pathStore) func() {
	old := pathStoreFactory
	pathStoreFactory = factory
	return func() { pathStoreFactory = old }
}

// GoBinDir resolves the Go bin directory `go install` places provisioned binaries into: the
// toolchain's own resolved GOBIN if set, otherwise GOPATH/bin. It shells out via the same
// injectable CommandRunner ("go env ...") this package already uses for `claude plugin ...`
// (plugins.go), rather than reading the GOBIN/GOPATH process env vars directly, so it reflects
// `go env -w`-persisted values a developer may have set on their machine — not just whatever
// happens to be in this process's environment. cfg is accepted for signature symmetry with the
// rest of this package's Config-taking functions and to keep this call stable for the later
// signal-wiring PR; it is not currently consulted.
func GoBinDir(cfg Config) (string, error) {
	runner := commandRunnerFactory()
	gobin, err := goEnv(runner, "GOBIN")
	if err != nil {
		return "", err
	}
	if gobin != "" {
		return gobin, nil
	}
	gopath, err := goEnv(runner, "GOPATH")
	if err != nil {
		return "", err
	}
	if gopath == "" {
		return "", fmt.Errorf("installer: resolve go bin dir: neither go env GOBIN nor go env GOPATH resolved to a value")
	}
	return filepath.Join(gopath, "bin"), nil
}

func goEnv(runner CommandRunner, key string) (string, error) {
	out, err := runner.Output("go", "env", key)
	if err != nil {
		return "", fmt.Errorf("installer: go env %s: %w", key, err)
	}
	return strings.TrimSpace(string(out)), nil
}

// pathPersistedProbe is the injectable seam behind PathPersisted, defaulting to the real
// pathStoreFactory-backed implementation (nil-factory guard + PersistedPathContains forward).
// Exported via SetPathPersistedProbeForTests so DOWNSTREAM packages (click doctor's
// checkEngramPath, the CLI's own end-to-end test suite) can make PATH-drift diagnosis
// deterministic without this package exposing the unexported pathStore type itself — mirrors the
// commandRunnerFactory/binaryLookupFactory injectable-factory pattern already used elsewhere in
// this package, just at the probe-function level instead of a whole-store level, since callers
// outside this package only ever need the read-only PersistedPathContains question answered, never
// the full pathStore surface.
var pathPersistedProbe = func(dir string) (bool, error) {
	if pathStoreFactory == nil {
		return false, fmt.Errorf("installer: no platform pathStore wired in")
	}
	return pathStoreFactory().PersistedPathContains(dir)
}

// PathPersisted reports whether dir is present in the user's PERSISTED PATH — the platform
// pathStore's PersistedPathContains (Windows HKCU\Environment\Path, POSIX shell rc files): what a
// freshly started process/session would see. Exported so `click doctor`'s checkEngramPath (a
// different package, internal/doctor) can diagnose PERSISTED-vs-LIVE PATH drift (sdd/engram-mcp-
// resolution obs #1436, D-6) without this package exposing the unexported pathStore type itself.
func PathPersisted(dir string) (bool, error) {
	return pathPersistedProbe(dir)
}

// SetPathPersistedProbeForTests overrides PathPersisted's implementation for tests and returns a
// restore function, mirroring SetCommandRunnerFactoryForTests/SetBinaryLookupFactoryForTests.
func SetPathPersistedProbeForTests(probe func(dir string) (bool, error)) func() {
	old := pathPersistedProbe
	pathPersistedProbe = probe
	return func() { pathPersistedProbe = old }
}

// livePathContainsProbe is the injectable seam behind LivePathContains, defaulting to the real
// os.Getenv("PATH")-backed implementation. Exported via SetLivePathContainsProbeForTests for the
// same cross-package reason as pathPersistedProbe above.
var livePathContainsProbe = func(dir string) bool {
	for _, entry := range filepath.SplitList(os.Getenv("PATH")) {
		if sameDir(entry, dir) {
			return true
		}
	}
	return false
}

// LivePathContains reports whether dir is present in the CURRENT process's live PATH environment
// variable (os.Getenv("PATH")), split via the OS-native list separator (filepath.SplitList) and
// compared with sameDir's case rule (case-insensitive on Windows, case-sensitive elsewhere — the
// same rule the filesystem/PATH itself follows on each platform). This answers "would a subprocess
// launched right now, in THIS process's environment, resolve dir" — as opposed to PathPersisted,
// which answers "would a FRESH process/session resolve dir". Exported for `click doctor`'s
// checkEngramPath.
func LivePathContains(dir string) bool {
	return livePathContainsProbe(dir)
}

// SetLivePathContainsProbeForTests overrides LivePathContains's implementation for tests and
// returns a restore function, mirroring SetPathPersistedProbeForTests above.
func SetLivePathContainsProbeForTests(probe func(dir string) bool) func() {
	old := livePathContainsProbe
	livePathContainsProbe = probe
	return func() { livePathContainsProbe = old }
}

// computeNewPath is the pure core of the Windows REG_EXPAND_SZ Path value mutation: given the
// current semicolon-joined PATH value and a directory to ensure is present, it returns the new
// value to write (unchanged from current when dir is already present) and whether a write is
// actually needed. Comparison is case-insensitive and normalizes a trailing "\" so
// "C:\Users\x\go\bin" and "C:\Users\x\go\bin\" (or a different-case variant) are treated as the
// same entry — Windows PATH entries are case-insensitive and a trailing separator is cosmetic.
// Kept side-effect free (no registry I/O) so it is table-driven-tested on every CI platform, not
// just windows-tagged ones; the windows-tagged pathStore implementation (a later PR in this
// chain) is the only production caller.
func computeNewPath(current, dir string) (newValue string, changed bool) {
	if pathListContains(current, dir) {
		return current, false
	}
	if current == "" {
		return dir, true
	}
	return current + ";" + dir, true
}

// pathListContains reports whether dir is already present (per normalizePathEntry) among the
// ";"-separated entries of list.
func pathListContains(list, dir string) bool {
	target := normalizePathEntry(dir)
	for _, entry := range strings.Split(list, ";") {
		if normalizePathEntry(entry) == target {
			return true
		}
	}
	return false
}

// normalizePathEntry lower-cases entry and trims a single trailing "\" so PATH-entry comparisons
// are both case- and trailing-separator-insensitive.
func normalizePathEntry(entry string) string {
	return strings.ToLower(strings.TrimSuffix(strings.TrimSpace(entry), `\`))
}

// tempFileWriter is the minimal *os.File surface atomicWriteFile needs. Abstracting it (instead
// of calling os.CreateTemp directly) lets tests inject a writer whose Write fails deterministically
// — exercising the "leave the original byte-for-byte intact on error" guarantee without relying on
// flaky OS-level failure injection (full disk, permission races, etc.).
type tempFileWriter interface {
	Write(p []byte) (int, error)
	Sync() error
	Close() error
	Name() string
}

// createTempFile is the injectable factory behind atomicWriteFile's temp file creation, following
// this package's existing CommandRunner/BinaryLookup pattern. Tests in this same package
// (pathenv_test.go) override it directly — no exported Set...ForTests wrapper is needed since
// atomicWriteFile and its tests live in the same package.
var createTempFile = func(dir, pattern string) (tempFileWriter, error) {
	return os.CreateTemp(dir, pattern)
}

// atomicWriteFile writes content to path atomically: it creates a temp file in the SAME directory
// as path's REAL write target (so the final rename is on the same filesystem and therefore atomic
// on POSIX), writes content, fsyncs, closes, chmods to mode, then renames the temp file onto the
// real target. On any error at any step, it removes the temp file and returns the error — the
// target is never touched until the final rename, so a failure at any earlier step leaves the
// original file byte-for-byte intact.
//
// Symlink safety: if path is itself a symlink (e.g. ~/.bashrc -> ~/dotfiles/bashrc, the standard
// chezmoi/GNU-stow/dotbot/yadm dotfiles pattern), POSIX rename(2) does NOT follow a symlink at the
// destination — it atomically replaces the directory entry AT that path, which would silently
// de-symlink it (path becomes a brand-new plain file; the real tracked file the symlink pointed at
// is left stale and orphaned). To avoid that, atomicWriteFile resolves path to its real underlying
// target first (via resolveWriteTarget) and writes/renames onto THAT path instead — the symlink
// itself is left completely undisturbed, still pointing at the same place, now with updated
// content at that real location. A non-symlink path is unaffected: resolveWriteTarget returns it
// unchanged.
func atomicWriteFile(path string, content []byte, mode os.FileMode) error {
	target, err := resolveWriteTarget(path)
	if err != nil {
		return fmt.Errorf("installer: resolve write target for %s: %w", path, err)
	}

	dir := filepath.Dir(target)
	tmp, err := createTempFile(dir, ".click-*")
	if err != nil {
		return fmt.Errorf("installer: create temp file for %s: %w", target, err)
	}
	tmpName := tmp.Name()
	defer func() {
		// Best-effort cleanup: once the rename below succeeds, tmpName no longer exists under its
		// original name, so this Remove is a harmless no-op. On any earlier failure it deletes the
		// leftover temp file so it never accumulates.
		_ = os.Remove(tmpName)
	}()

	if _, err := tmp.Write(content); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("installer: write temp file for %s: %w", target, err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("installer: sync temp file for %s: %w", target, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("installer: close temp file for %s: %w", target, err)
	}
	if err := os.Chmod(tmpName, mode); err != nil {
		return fmt.Errorf("installer: chmod temp file for %s: %w", target, err)
	}
	if err := os.Rename(tmpName, target); err != nil {
		return fmt.Errorf("installer: rename temp file to %s: %w", target, err)
	}
	return nil
}

// resolveWriteTarget resolves the REAL path atomicWriteFile must write/rename onto: path itself,
// unless path is a symlink, in which case it returns the symlink's fully-resolved real target so
// the eventual rename replaces the real file's directory entry, not the symlink's. Three cases:
//   - path does not exist yet (new file, e.g. a fresh install writing a brand-new rc file):
//     returns path unchanged, nil error — there is nothing to resolve.
//   - path exists and is NOT a symlink: returns path unchanged, nil error — existing behavior.
//   - path exists and IS a symlink: returns filepath.EvalSymlinks(path) (fully resolving the whole
//     chain, including any symlinked parent directories), or a wrapped error if the symlink is
//     broken (its target does not exist) or otherwise cannot be resolved.
func resolveWriteTarget(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return path, nil
		}
		return "", err
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return path, nil
	}
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	return real, nil
}
