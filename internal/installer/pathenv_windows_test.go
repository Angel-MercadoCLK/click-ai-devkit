//go:build windows

package installer

import (
	"errors"
	"testing"

	"golang.org/x/sys/windows/registry"
)

// fakeRegistryKey is a registryKey double letting pathenv_windows_test.go exercise
// osPathStore's read-modify-write and value-type-preservation logic deterministically —
// without touching the real HKCU\Environment hive.
type fakeRegistryKey struct {
	getVal  string
	getType uint32
	getErr  error

	setStringCalls []struct{ name, value string }
	setExpandCalls []struct{ name, value string }
	setErr         error

	closeCalls int
}

func (f *fakeRegistryKey) GetStringValue(name string) (string, uint32, error) {
	return f.getVal, f.getType, f.getErr
}

func (f *fakeRegistryKey) SetStringValue(name, value string) error {
	f.setStringCalls = append(f.setStringCalls, struct{ name, value string }{name, value})
	return f.setErr
}

func (f *fakeRegistryKey) SetExpandStringValue(name, value string) error {
	f.setExpandCalls = append(f.setExpandCalls, struct{ name, value string }{name, value})
	return f.setErr
}

func (f *fakeRegistryKey) Close() error {
	f.closeCalls++
	return nil
}

// withFakeRegistry overrides openEnvironmentKey to always return key (or openErr), and
// broadcastEnv to record how many times it was invoked / return broadcastErr. It returns a
// restore func and a pointer to the broadcast call counter.
func withFakeRegistry(t *testing.T, key *fakeRegistryKey, openErr error, broadcastErr error) *int {
	t.Helper()
	broadcastCalls := 0

	oldOpen := openEnvironmentKey
	openEnvironmentKey = func(access uint32) (registryKey, error) {
		if openErr != nil {
			return nil, openErr
		}
		return key, nil
	}

	oldBroadcast := broadcastEnv
	broadcastEnv = func() error {
		broadcastCalls++
		return broadcastErr
	}

	t.Cleanup(func() {
		openEnvironmentKey = oldOpen
		broadcastEnv = oldBroadcast
	})

	return &broadcastCalls
}

// TestOsPathStore_EnsureOnPath_AppendsAndPreservesExpandSz covers the common case: the Path
// value is REG_EXPAND_SZ (Windows' normal type for it), dir is missing, so EnsureOnPath must
// write the new value via SetExpandStringValue (never SetStringValue) and broadcast exactly once.
func TestOsPathStore_EnsureOnPath_AppendsAndPreservesExpandSz(t *testing.T) {
	key := &fakeRegistryKey{getVal: `C:\Windows\system32`, getType: registry.EXPAND_SZ}
	broadcastCalls := withFakeRegistry(t, key, nil, nil)

	changed, err := osPathStore{}.EnsureOnPath(`C:\Users\dev\go\bin`)
	if err != nil {
		t.Fatalf("EnsureOnPath() error = %v", err)
	}
	if !changed {
		t.Fatal("EnsureOnPath() changed = false, want true")
	}
	if len(key.setExpandCalls) != 1 {
		t.Fatalf("SetExpandStringValue calls = %d, want 1 (got SetStringValue calls = %d)", len(key.setExpandCalls), len(key.setStringCalls))
	}
	want := `C:\Windows\system32;C:\Users\dev\go\bin`
	if key.setExpandCalls[0].value != want {
		t.Fatalf("SetExpandStringValue value = %q, want %q", key.setExpandCalls[0].value, want)
	}
	if len(key.setStringCalls) != 0 {
		t.Fatalf("SetStringValue calls = %d, want 0 (REG_EXPAND_SZ must be preserved, not downgraded)", len(key.setStringCalls))
	}
	if *broadcastCalls != 1 {
		t.Fatalf("broadcastEnv calls = %d, want 1", *broadcastCalls)
	}
}

// TestOsPathStore_EnsureOnPath_PreservesRegSz triangulates the value-type-preservation
// requirement (D-8) against the opposite starting type: when the existing value is REG_SZ,
// EnsureOnPath must write back via SetStringValue, never "upgrading" it to REG_EXPAND_SZ.
func TestOsPathStore_EnsureOnPath_PreservesRegSz(t *testing.T) {
	key := &fakeRegistryKey{getVal: `C:\Windows\system32`, getType: registry.SZ}
	broadcastCalls := withFakeRegistry(t, key, nil, nil)

	changed, err := osPathStore{}.EnsureOnPath(`C:\Users\dev\go\bin`)
	if err != nil {
		t.Fatalf("EnsureOnPath() error = %v", err)
	}
	if !changed {
		t.Fatal("EnsureOnPath() changed = false, want true")
	}
	if len(key.setStringCalls) != 1 {
		t.Fatalf("SetStringValue calls = %d, want 1", len(key.setStringCalls))
	}
	if len(key.setExpandCalls) != 0 {
		t.Fatalf("SetExpandStringValue calls = %d, want 0 (REG_SZ must not be upgraded)", len(key.setExpandCalls))
	}
	if *broadcastCalls != 1 {
		t.Fatalf("broadcastEnv calls = %d, want 1", *broadcastCalls)
	}
}

// TestOsPathStore_EnsureOnPath_NoOpWhenAlreadyPresent covers idempotency: when dir is already on
// the persisted PATH, EnsureOnPath must not write anything and must not broadcast.
func TestOsPathStore_EnsureOnPath_NoOpWhenAlreadyPresent(t *testing.T) {
	key := &fakeRegistryKey{getVal: `C:\Windows\system32;C:\Users\dev\go\bin`, getType: registry.EXPAND_SZ}
	broadcastCalls := withFakeRegistry(t, key, nil, nil)

	changed, err := osPathStore{}.EnsureOnPath(`C:\Users\dev\go\bin`)
	if err != nil {
		t.Fatalf("EnsureOnPath() error = %v", err)
	}
	if changed {
		t.Fatal("EnsureOnPath() changed = true, want false (already present)")
	}
	if len(key.setStringCalls) != 0 || len(key.setExpandCalls) != 0 {
		t.Fatalf("Set*Value calls made on a no-op: string=%d expand=%d, want 0/0", len(key.setStringCalls), len(key.setExpandCalls))
	}
	if *broadcastCalls != 0 {
		t.Fatalf("broadcastEnv calls = %d, want 0 (no mutation happened)", *broadcastCalls)
	}
}

// TestOsPathStore_EnsureOnPath_MissingValueDefaultsToExpandSz covers the fresh-install edge case:
// HKCU\Environment\Path does not exist yet (ErrNotExist) — EnsureOnPath must treat current as
// empty and default the write to REG_EXPAND_SZ (Windows' own default type for this value).
func TestOsPathStore_EnsureOnPath_MissingValueDefaultsToExpandSz(t *testing.T) {
	key := &fakeRegistryKey{getErr: registry.ErrNotExist}
	broadcastCalls := withFakeRegistry(t, key, nil, nil)

	changed, err := osPathStore{}.EnsureOnPath(`C:\Users\dev\go\bin`)
	if err != nil {
		t.Fatalf("EnsureOnPath() error = %v", err)
	}
	if !changed {
		t.Fatal("EnsureOnPath() changed = false, want true")
	}
	if len(key.setExpandCalls) != 1 || key.setExpandCalls[0].value != `C:\Users\dev\go\bin` {
		t.Fatalf("SetExpandStringValue calls = %#v, want exactly one call with the bootstrap value", key.setExpandCalls)
	}
	if *broadcastCalls != 1 {
		t.Fatalf("broadcastEnv calls = %d, want 1", *broadcastCalls)
	}
}

// TestOsPathStore_EnsureOnPath_RegistryOpenFailure is the strict-TDD-required failure branch:
// when opening HKCU\Environment fails, EnsureOnPath must surface the error and never call Set*
// or broadcast.
func TestOsPathStore_EnsureOnPath_RegistryOpenFailure(t *testing.T) {
	openErr := errors.New("injected registry open failure")
	broadcastCalls := withFakeRegistry(t, &fakeRegistryKey{}, openErr, nil)

	_, err := osPathStore{}.EnsureOnPath(`C:\Users\dev\go\bin`)
	if err == nil {
		t.Fatal("EnsureOnPath() error = nil, want the injected open error to propagate")
	}
	if !errors.Is(err, openErr) {
		t.Fatalf("EnsureOnPath() error = %v, want it to wrap %v", err, openErr)
	}
	if *broadcastCalls != 0 {
		t.Fatalf("broadcastEnv calls = %d, want 0 (open failed before any mutation)", *broadcastCalls)
	}
}

// TestOsPathStore_EnsureOnPath_RegistrySetFailure is the strict-TDD-required failure branch: when
// the registry write itself fails, EnsureOnPath must surface the error and must NOT broadcast (no
// mutation actually landed).
func TestOsPathStore_EnsureOnPath_RegistrySetFailure(t *testing.T) {
	setErr := errors.New("injected registry set failure")
	key := &fakeRegistryKey{getVal: `C:\Windows\system32`, getType: registry.EXPAND_SZ, setErr: setErr}
	broadcastCalls := withFakeRegistry(t, key, nil, nil)

	_, err := osPathStore{}.EnsureOnPath(`C:\Users\dev\go\bin`)
	if err == nil {
		t.Fatal("EnsureOnPath() error = nil, want the injected set error to propagate")
	}
	if !errors.Is(err, setErr) {
		t.Fatalf("EnsureOnPath() error = %v, want it to wrap %v", err, setErr)
	}
	if *broadcastCalls != 0 {
		t.Fatalf("broadcastEnv calls = %d, want 0 (write failed, nothing to broadcast)", *broadcastCalls)
	}
}

// TestOsPathStore_EnsureOnPath_BroadcastFailure is the strict-TDD-required failure branch: the
// registry write itself succeeds but WM_SETTINGCHANGE broadcast fails. The PATH value is already
// durably written at this point, so EnsureOnPath must still report changed=true, but the broadcast
// error must be surfaced so a caller can warn the user that a new shell may be needed.
func TestOsPathStore_EnsureOnPath_BroadcastFailure(t *testing.T) {
	broadcastErr := errors.New("injected broadcast failure")
	key := &fakeRegistryKey{getVal: `C:\Windows\system32`, getType: registry.EXPAND_SZ}
	broadcastCalls := withFakeRegistry(t, key, nil, broadcastErr)

	changed, err := osPathStore{}.EnsureOnPath(`C:\Users\dev\go\bin`)
	if err == nil {
		t.Fatal("EnsureOnPath() error = nil, want the injected broadcast error to propagate")
	}
	if !errors.Is(err, broadcastErr) {
		t.Fatalf("EnsureOnPath() error = %v, want it to wrap %v", err, broadcastErr)
	}
	if !changed {
		t.Fatal("EnsureOnPath() changed = false, want true (the registry write itself succeeded)")
	}
	if len(key.setExpandCalls) != 1 {
		t.Fatalf("SetExpandStringValue calls = %d, want 1 (write must have been attempted before broadcast)", len(key.setExpandCalls))
	}
	if *broadcastCalls != 1 {
		t.Fatalf("broadcastEnv calls = %d, want 1", *broadcastCalls)
	}
}

// TestOsPathStore_PersistedPathContains_TrueWhenPresent covers the read-only query path.
func TestOsPathStore_PersistedPathContains_TrueWhenPresent(t *testing.T) {
	key := &fakeRegistryKey{getVal: `C:\Windows\system32;C:\Users\dev\go\bin`, getType: registry.EXPAND_SZ}
	withFakeRegistry(t, key, nil, nil)

	got, err := osPathStore{}.PersistedPathContains(`C:\Users\dev\go\bin`)
	if err != nil {
		t.Fatalf("PersistedPathContains() error = %v", err)
	}
	if !got {
		t.Fatal("PersistedPathContains() = false, want true")
	}
}

// TestOsPathStore_PersistedPathContains_FalseWhenAbsent triangulates against a PATH that does not
// contain dir.
func TestOsPathStore_PersistedPathContains_FalseWhenAbsent(t *testing.T) {
	key := &fakeRegistryKey{getVal: `C:\Windows\system32`, getType: registry.EXPAND_SZ}
	withFakeRegistry(t, key, nil, nil)

	got, err := osPathStore{}.PersistedPathContains(`C:\Users\dev\go\bin`)
	if err != nil {
		t.Fatalf("PersistedPathContains() error = %v", err)
	}
	if got {
		t.Fatal("PersistedPathContains() = true, want false")
	}
}

// TestOsPathStore_PersistedPathContains_RegistryOpenFailure is the strict-TDD-required failure
// branch for the read-only query path.
func TestOsPathStore_PersistedPathContains_RegistryOpenFailure(t *testing.T) {
	openErr := errors.New("injected registry open failure")
	withFakeRegistry(t, &fakeRegistryKey{}, openErr, nil)

	_, err := osPathStore{}.PersistedPathContains(`C:\Users\dev\go\bin`)
	if err == nil {
		t.Fatal("PersistedPathContains() error = nil, want the injected open error to propagate")
	}
	if !errors.Is(err, openErr) {
		t.Fatalf("PersistedPathContains() error = %v, want it to wrap %v", err, openErr)
	}
}

// TestOsPathStore_PersistedPathContains_MissingValueIsFalse covers the fresh-install edge case for
// the read-only query: an absent Path value must be treated as "not contained", not an error.
func TestOsPathStore_PersistedPathContains_MissingValueIsFalse(t *testing.T) {
	key := &fakeRegistryKey{getErr: registry.ErrNotExist}
	withFakeRegistry(t, key, nil, nil)

	got, err := osPathStore{}.PersistedPathContains(`C:\Users\dev\go\bin`)
	if err != nil {
		t.Fatalf("PersistedPathContains() error = %v", err)
	}
	if got {
		t.Fatal("PersistedPathContains() = true, want false when the Path value does not exist yet")
	}
}

// TestPathStoreFactory_DefaultsToOsPathStoreOnWindows proves the build-tagged init() in
// pathenv_windows.go actually wires pathStoreFactory, closing PR1's documented gap ("pathStoreFactory
// is nil until an OS-specific file assigns it").
func TestPathStoreFactory_DefaultsToOsPathStoreOnWindows(t *testing.T) {
	if pathStoreFactory == nil {
		t.Fatal("pathStoreFactory is nil on windows; want pathenv_windows.go's init() to have assigned it")
	}
	if _, ok := pathStoreFactory().(osPathStore); !ok {
		t.Fatalf("pathStoreFactory() = %#v, want an osPathStore", pathStoreFactory())
	}
}
