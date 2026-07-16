//go:build windows

// PR2 of the engram-mcp-resolution chain: the Windows pathStore implementation. It persists the
// resolved Go bin dir onto HKCU\Environment\Path via a raw (unexpanded) read-modify-write —
// exactly the type Windows already has it as, per design D-8 — then broadcasts WM_SETTINGCHANGE
// so already-running processes (Explorer, other shells) pick up the change without a reboot.
//
// The `;`-split / case-insensitive PATH semantics this file relies on (computeNewPath,
// pathListContains, normalizePathEntry, all defined in the OS-agnostic pathenv.go) are
// Windows-only. PR3's POSIX implementation (pathenv_unix.go) MUST NOT reuse them as-is — POSIX
// PATH is `:`-split and case-sensitive.
package installer

import (
	"errors"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// environmentKeyPath is the well-known per-user environment registry key. Persisting here (not
// HKLM\...\Session Manager\Environment, the machine-wide equivalent) matches this change's scope:
// a per-user Go toolchain install should not require admin rights to fix up PATH.
const environmentKeyPath = `Environment`

// pathValueName is the registry value name holding the user's persisted PATH.
const pathValueName = "Path"

// registryKey is the minimal surface osPathStore needs from golang.org/x/sys/windows/registry.Key.
// golang.org/x/sys/windows/registry.Key already implements this interface (its methods have
// matching value-receiver signatures), so no wrapper type is needed for the real path — only
// tests substitute a fake.
type registryKey interface {
	// GetStringValue reads name's raw (unexpanded) string value and its registry type
	// (registry.SZ or registry.EXPAND_SZ). Per design D-8, callers MUST use this — never a call
	// that would expand "%SystemRoot%"-style tokens — and MUST propagate valtype back into
	// whichever Set*Value call writes the new value, so the original type is preserved.
	GetStringValue(name string) (val string, valtype uint32, err error)
	// SetStringValue writes name as REG_SZ.
	SetStringValue(name, value string) error
	// SetExpandStringValue writes name as REG_EXPAND_SZ.
	SetExpandStringValue(name, value string) error
	Close() error
}

// openEnvironmentKey is the injectable factory behind osPathStore's registry access, following
// this package's existing CommandRunner/BinaryLookup/createTempFile pattern. Tests in this same
// package (pathenv_windows_test.go) override it directly.
var openEnvironmentKey = func(access uint32) (registryKey, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, environmentKeyPath, access)
	if err != nil {
		return nil, err
	}
	return k, nil
}

// osPathStore is the Windows pathStore implementation: HKCU\Environment\Path read-modify-write +
// WM_SETTINGCHANGE broadcast. It carries no state (registry access is opened fresh per call), so
// the zero value is always ready to use.
type osPathStore struct{}

// init wires pathStoreFactory to osPathStore by default on windows builds. pathenv.go's own
// pathStoreFactory var starts nil (see its doc comment) precisely so an OS-specific file like this
// one is the sole place a default gets assigned.
func init() {
	pathStoreFactory = func() pathStore { return osPathStore{} }
}

// PersistedPathContains reports whether dir is already present in HKCU\Environment\Path. A
// missing Path value (fresh account, never customized) is treated as an empty PATH — not an
// error — since that is a legitimate, common starting state.
func (osPathStore) PersistedPathContains(dir string) (bool, error) {
	key, err := openEnvironmentKey(registry.QUERY_VALUE)
	if err != nil {
		return false, fmt.Errorf("installer: open HKCU\\Environment for read: %w", err)
	}
	defer key.Close()

	current, _, err := key.GetStringValue(pathValueName)
	if err != nil {
		if errors.Is(err, registry.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("installer: read HKCU\\Environment\\%s: %w", pathValueName, err)
	}
	return pathListContains(current, dir), nil
}

// EnsureOnPath adds dir to HKCU\Environment\Path if not already present, preserving whichever
// registry value type (REG_EXPAND_SZ or REG_SZ) the value already had — or defaulting to
// REG_EXPAND_SZ (Windows' own default for a fresh Path value) when the value does not exist yet.
// On an actual mutation it broadcasts WM_SETTINGCHANGE so already-running processes observe the
// change without a reboot; a broadcast failure is surfaced as an error (so a caller can warn the
// user a new shell may be needed) even though changed is still reported true, since the registry
// write itself already durably succeeded.
func (osPathStore) EnsureOnPath(dir string) (bool, error) {
	key, err := openEnvironmentKey(registry.QUERY_VALUE | registry.SET_VALUE)
	if err != nil {
		return false, fmt.Errorf("installer: open HKCU\\Environment for read-write: %w", err)
	}
	defer key.Close()

	current, valtype, err := key.GetStringValue(pathValueName)
	if err != nil {
		if !errors.Is(err, registry.ErrNotExist) {
			return false, fmt.Errorf("installer: read HKCU\\Environment\\%s: %w", pathValueName, err)
		}
		current = ""
		valtype = registry.EXPAND_SZ
	}

	newValue, changed := computeNewPath(current, dir)
	if !changed {
		return false, nil
	}

	if valtype == registry.EXPAND_SZ {
		err = key.SetExpandStringValue(pathValueName, newValue)
	} else {
		err = key.SetStringValue(pathValueName, newValue)
	}
	if err != nil {
		return false, fmt.Errorf("installer: write HKCU\\Environment\\%s: %w", pathValueName, err)
	}

	if err := broadcastEnv(); err != nil {
		return true, fmt.Errorf("installer: broadcast WM_SETTINGCHANGE after PATH update: %w", err)
	}
	return true, nil
}

// RemoveFromPath removes dir from HKCU\Environment\Path if present (D-9, reversal half of
// EnsureOnPath), preserving whichever registry value type (REG_EXPAND_SZ or REG_SZ) the value
// already had, and preserving every OTHER entry's exact text and relative order — only the matching
// entry/entries are dropped (via computeRemovedPath's same case-insensitive,
// trailing-separator-normalized comparison EnsureOnPath itself uses), so a sibling entry is never
// corrupted or truncated. changed=false with NO registry write at all when dir is not present (a
// missing Path value is treated as empty — nothing to remove, not an error, mirroring
// PersistedPathContains' own missing-value handling). On an actual mutation it broadcasts
// WM_SETTINGCHANGE, exactly like EnsureOnPath, so already-running processes observe the removal
// without a reboot.
func (osPathStore) RemoveFromPath(dir string) (bool, error) {
	key, err := openEnvironmentKey(registry.QUERY_VALUE | registry.SET_VALUE)
	if err != nil {
		return false, fmt.Errorf("installer: open HKCU\\Environment for read-write: %w", err)
	}
	defer key.Close()

	current, valtype, err := key.GetStringValue(pathValueName)
	if err != nil {
		if errors.Is(err, registry.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("installer: read HKCU\\Environment\\%s: %w", pathValueName, err)
	}

	newValue, changed := computeRemovedPath(current, dir)
	if !changed {
		return false, nil
	}

	if valtype == registry.EXPAND_SZ {
		err = key.SetExpandStringValue(pathValueName, newValue)
	} else {
		err = key.SetStringValue(pathValueName, newValue)
	}
	if err != nil {
		return false, fmt.Errorf("installer: write HKCU\\Environment\\%s: %w", pathValueName, err)
	}

	if err := broadcastEnv(); err != nil {
		return true, fmt.Errorf("installer: broadcast WM_SETTINGCHANGE after PATH update: %w", err)
	}
	return true, nil
}

// Windows WM_SETTINGCHANGE broadcast constants. See
// https://learn.microsoft.com/windows/win32/winmsg/wm-settingchange.
const (
	hwndBroadcast      = 0xffff
	wmSettingChange    = 0x001A
	smtoAbortIfHung    = 0x0002
	broadcastTimeoutMs = 5000
)

// broadcastEnv notifies already-running top-level windows (Explorer, other open shells) that the
// environment changed, via WM_SETTINGCHANGE with lParam "Environment" — the standard mechanism
// Windows itself uses (e.g. the System Properties "Environment Variables" dialog) so a fresh
// `go install` PATH fix-up is visible without forcing a logoff/reboot. It is a var (not a plain
// func) so tests can inject a recording/failing double.
var broadcastEnv = func() error {
	user32 := windows.NewLazySystemDLL("user32.dll")
	sendMessageTimeout := user32.NewProc("SendMessageTimeoutW")

	lparam, err := windows.UTF16PtrFromString("Environment")
	if err != nil {
		return fmt.Errorf("installer: encode WM_SETTINGCHANGE lparam: %w", err)
	}

	ret, _, callErr := sendMessageTimeout.Call(
		uintptr(hwndBroadcast),
		uintptr(wmSettingChange),
		0,
		uintptr(unsafe.Pointer(lparam)),
		uintptr(smtoAbortIfHung),
		uintptr(broadcastTimeoutMs),
		0,
	)
	if ret == 0 {
		return fmt.Errorf("installer: SendMessageTimeoutW(WM_SETTINGCHANGE) failed: %w", callErr)
	}
	return nil
}
