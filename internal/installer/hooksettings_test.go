package installer

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- Symlink / atomic-write regression coverage (Finding 2) ---
//
// writeSettingsFile and writeJSONFile used to write via plain os.WriteFile, which is non-atomic
// (a crash mid-write leaves a truncated/corrupted file) and inconsistent with this package's own
// existing solution (atomicWriteFile/resolveWriteTarget in pathenv.go), added specifically to avoid
// de-symlinking dotfiles-managed rc files. A developer managing ~/.claude/ with a dotfiles repo
// (chezmoi/GNU-stow/dotbot/yadm) commonly has settings.json symlinked into their dotfiles checkout;
// the tests below reuse the SAME requireSymlinkSupport/fakeFailingTempFile fixtures pathenv_test.go
// already established for atomicWriteFile itself, applied here to prove the two JSON-writing
// helpers are now wired through it instead of a second parallel implementation.

// TestWriteSettingsFile_WritesThroughSymlinkPreservingIt proves a symlinked settings.json is
// written through to its real target, leaving the symlink itself undisturbed.
func TestWriteSettingsFile_WritesThroughSymlinkPreservingIt(t *testing.T) {
	requireSymlinkSupport(t)

	root := t.TempDir()
	realDir := filepath.Join(root, "real")
	linkDir := filepath.Join(root, "linked")
	if err := os.MkdirAll(realDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(realDir) error = %v", err)
	}
	if err := os.MkdirAll(linkDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(linkDir) error = %v", err)
	}

	realTarget := filepath.Join(realDir, "settings.json")
	if err := os.WriteFile(realTarget, []byte(`{"existing":true}`+"\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(realTarget) error = %v", err)
	}

	symlinkPath := filepath.Join(linkDir, "settings.json")
	if err := os.Symlink(realTarget, symlinkPath); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}

	if err := writeSettingsFile(symlinkPath, map[string]any{"hooks": "configured"}); err != nil {
		t.Fatalf("writeSettingsFile() error = %v", err)
	}

	info, err := os.Lstat(symlinkPath)
	if err != nil {
		t.Fatalf("Lstat(symlinkPath) error = %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("symlinkPath is no longer a symlink after writeSettingsFile() (mode = %v) — it was destructively de-symlinked", info.Mode())
	}

	data, err := os.ReadFile(realTarget)
	if err != nil {
		t.Fatalf("ReadFile(realTarget) error = %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("json.Unmarshal(realTarget content) error = %v, content = %s", err, data)
	}
	if got["hooks"] != "configured" {
		t.Fatalf("realTarget content = %s, want the new settings written through the symlink", data)
	}
}

// TestWriteSettingsFile_InjectedWriteErrorLeavesOriginalIntact is the strict-TDD RED/GREEN proof
// that writeSettingsFile now goes through atomicWriteFile's temp-file+rename path: injecting a
// failing createTempFile must surface an error AND leave the original file byte-for-byte untouched.
// Against the old direct os.WriteFile implementation this injection is a no-op (createTempFile is
// never consulted), so the write silently succeeds and this test fails.
func TestWriteSettingsFile_InjectedWriteErrorLeavesOriginalIntact(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	original := []byte(`{"original":true}` + "\n")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatalf("seed WriteFile() error = %v", err)
	}

	injectedErr := errors.New("injected write failure")
	old := createTempFile
	createTempFile = func(dir, pattern string) (tempFileWriter, error) {
		return &fakeFailingTempFile{name: filepath.Join(dir, ".click-injected-fake"), writeErr: injectedErr}, nil
	}
	defer func() { createTempFile = old }()

	err := writeSettingsFile(path, map[string]any{"new": true})
	if err == nil {
		t.Fatal("writeSettingsFile() error = nil, want the injected write error to propagate")
	}

	got, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("ReadFile(path) error = %v", readErr)
	}
	if string(got) != string(original) {
		t.Fatalf("file content = %q after a failed write, want untouched original %q", got, original)
	}
}

// TestWriteJSONFile_WritesThroughSymlinkPreservingIt mirrors
// TestWriteSettingsFile_WritesThroughSymlinkPreservingIt for writeJSONFile, the shared helper this
// package's engram.go/plugins.go/models.go/context7.go/profile_artifacts.go callers all use — fixing
// it here fixes all of them without touching those files.
func TestWriteJSONFile_WritesThroughSymlinkPreservingIt(t *testing.T) {
	requireSymlinkSupport(t)

	root := t.TempDir()
	realDir := filepath.Join(root, "real")
	linkDir := filepath.Join(root, "linked")
	if err := os.MkdirAll(realDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(realDir) error = %v", err)
	}
	if err := os.MkdirAll(linkDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(linkDir) error = %v", err)
	}

	realTarget := filepath.Join(realDir, "models.json")
	if err := os.WriteFile(realTarget, []byte(`{"existing":true}`+"\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(realTarget) error = %v", err)
	}

	symlinkPath := filepath.Join(linkDir, "models.json")
	if err := os.Symlink(realTarget, symlinkPath); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}

	if err := writeJSONFile(symlinkPath, map[string]any{"profile": "default"}); err != nil {
		t.Fatalf("writeJSONFile() error = %v", err)
	}

	info, err := os.Lstat(symlinkPath)
	if err != nil {
		t.Fatalf("Lstat(symlinkPath) error = %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("symlinkPath is no longer a symlink after writeJSONFile() (mode = %v) — it was destructively de-symlinked", info.Mode())
	}

	data, err := os.ReadFile(realTarget)
	if err != nil {
		t.Fatalf("ReadFile(realTarget) error = %v", err)
	}
	if !strings.Contains(string(data), `"profile": "default"`) {
		t.Fatalf("realTarget content = %s, want the new JSON written through the symlink", data)
	}
}

// TestWriteJSONFile_InjectedWriteErrorLeavesOriginalIntact mirrors
// TestWriteSettingsFile_InjectedWriteErrorLeavesOriginalIntact for writeJSONFile.
func TestWriteJSONFile_InjectedWriteErrorLeavesOriginalIntact(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "models.json")
	original := []byte(`{"original":true}` + "\n")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatalf("seed WriteFile() error = %v", err)
	}

	injectedErr := errors.New("injected write failure")
	old := createTempFile
	createTempFile = func(dir, pattern string) (tempFileWriter, error) {
		return &fakeFailingTempFile{name: filepath.Join(dir, ".click-injected-fake"), writeErr: injectedErr}, nil
	}
	defer func() { createTempFile = old }()

	err := writeJSONFile(path, map[string]any{"new": true})
	if err == nil {
		t.Fatal("writeJSONFile() error = nil, want the injected write error to propagate")
	}

	got, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("ReadFile(path) error = %v", readErr)
	}
	if string(got) != string(original) {
		t.Fatalf("file content = %q after a failed write, want untouched original %q", got, original)
	}
}

// TestRegisterMemoryGuardHook_WritesThroughSymlinkedSettings is an end-to-end regression test for
// the exact real-world scenario the finding describes: cfg.SettingsPath() resolves to a symlink
// into a dotfiles checkout, and the public RegisterMemoryGuardHook API (not just the unexported
// helper) must write through it correctly.
func TestRegisterMemoryGuardHook_WritesThroughSymlinkedSettings(t *testing.T) {
	requireSymlinkSupport(t)

	root := t.TempDir()
	dotfilesDir := filepath.Join(root, "dotfiles")
	claudeHome := filepath.Join(root, "claude-home")
	if err := os.MkdirAll(dotfilesDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(dotfilesDir) error = %v", err)
	}
	if err := os.MkdirAll(claudeHome, 0o755); err != nil {
		t.Fatalf("MkdirAll(claudeHome) error = %v", err)
	}

	realSettings := filepath.Join(dotfilesDir, "settings.json")
	if err := os.WriteFile(realSettings, []byte(`{}`+"\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(realSettings) error = %v", err)
	}

	cfg := Config{ClaudeHome: claudeHome}
	if err := os.Symlink(realSettings, cfg.SettingsPath()); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}

	if err := RegisterMemoryGuardHook(cfg); err != nil {
		t.Fatalf("RegisterMemoryGuardHook() error = %v", err)
	}

	info, err := os.Lstat(cfg.SettingsPath())
	if err != nil {
		t.Fatalf("Lstat(cfg.SettingsPath()) error = %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("cfg.SettingsPath() is no longer a symlink after RegisterMemoryGuardHook() — it was destructively de-symlinked")
	}

	registered, err := HasMemoryGuardHook(cfg)
	if err != nil {
		t.Fatalf("HasMemoryGuardHook() error = %v", err)
	}
	if !registered {
		t.Fatal("HasMemoryGuardHook() after RegisterMemoryGuardHook() through a symlink = false, want true")
	}
}

// --- PR C: PruneEmptyClickSettingsKeys tests ---

// TestPruneEmptyClickSettingsKeys_DeletesOnlyEmptyClickKeys proves C.1(a)-(c): deletes the three
// Click-specific keys ONLY when each is an empty map, preserving them for non-map or non-empty values,
// and never touching any non-allowlisted key.
func TestPruneEmptyClickSettingsKeys_DeletesOnlyEmptyClickKeys(t *testing.T) {
	tests := []struct {
		name     string
		initial  map[string]any
		wantPost map[string]any
	}{
		{
			name: "all three empty maps are deleted",
			initial: map[string]any{
				"enabledPlugins":         map[string]any{},
				"extraKnownMarketplaces": map[string]any{},
				"pluginConfigs":          map[string]any{},
				"foreignKey":             "keep me",
				"hooks":                  map[string]any{"PreToolUse": []any{}},
			},
			wantPost: map[string]any{
				"foreignKey": "keep me",
				"hooks":      map[string]any{"PreToolUse": []any{}},
			},
		},
		{
			name: "non-empty map values are preserved",
			initial: map[string]any{
				"enabledPlugins":         map[string]any{"click-sdd": true},
				"extraKnownMarketplaces": map[string]any{},
				"pluginConfigs":          map[string]any{},
				"foreignKey":             "keep me",
			},
			wantPost: map[string]any{
				"enabledPlugins": map[string]any{"click-sdd": true},
				"foreignKey":     "keep me",
			},
		},
		{
			name: "null values are preserved",
			initial: map[string]any{
				"enabledPlugins":         nil,
				"extraKnownMarketplaces": nil,
				"pluginConfigs":          nil,
				"foreignKey":             "keep me",
			},
			wantPost: map[string]any{
				"enabledPlugins":         nil,
				"extraKnownMarketplaces": nil,
				"pluginConfigs":          nil,
				"foreignKey":             "keep me",
			},
		},
		{
			name: "array values are preserved",
			initial: map[string]any{
				"enabledPlugins":         []any{"click-sdd"},
				"extraKnownMarketplaces": map[string]any{},
				"pluginConfigs":          map[string]any{},
				"foreignKey":             "keep me",
			},
			wantPost: map[string]any{
				"enabledPlugins": []any{"click-sdd"},
				"foreignKey":     "keep me",
			},
		},
		{
			name: "string values are preserved",
			initial: map[string]any{
				"enabledPlugins":         "not a map",
				"extraKnownMarketplaces": map[string]any{},
				"pluginConfigs":          map[string]any{},
				"foreignKey":             "keep me",
			},
			wantPost: map[string]any{
				"enabledPlugins": "not a map",
				"foreignKey":     "keep me",
			},
		},
		{
			name: "number values are preserved",
			initial: map[string]any{
				"enabledPlugins":         42,
				"extraKnownMarketplaces": map[string]any{},
				"pluginConfigs":          map[string]any{},
				"foreignKey":             "keep me",
			},
			wantPost: map[string]any{
				"enabledPlugins": 42,
				"foreignKey":     "keep me",
			},
		},
		{
			name: "boolean values are preserved",
			initial: map[string]any{
				"enabledPlugins":         true,
				"extraKnownMarketplaces": map[string]any{},
				"pluginConfigs":          map[string]any{},
				"foreignKey":             "keep me",
			},
			wantPost: map[string]any{
				"enabledPlugins": true,
				"foreignKey":     "keep me",
			},
		},
		{
			name: "foreign keys are never touched",
			initial: map[string]any{
				"enabledPlugins":         map[string]any{},
				"extraKnownMarketplaces": map[string]any{},
				"pluginConfigs":          map[string]any{},
				"foreignKey1":            "keep1",
				"foreignKey2":            map[string]any{},
				"hooks":                  map[string]any{"PreToolUse": []any{}},
			},
			wantPost: map[string]any{
				"foreignKey1": "keep1",
				"foreignKey2": map[string]any{},
				"hooks":       map[string]any{"PreToolUse": []any{}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "settings.json")

			// Seed initial settings
			if err := os.WriteFile(path, []byte(`{}`+"\n"), 0o600); err != nil {
				t.Fatalf("seed WriteFile() error = %v", err)
			}
			if err := writeSettingsFile(path, tt.initial); err != nil {
				t.Fatalf("writeSettingsFile() initial seed error = %v", err)
			}

			cfg := Config{ClaudeHome: dir}
			if err := PruneEmptyClickSettingsKeys(cfg); err != nil {
				t.Fatalf("PruneEmptyClickSettingsKeys() error = %v", err)
			}

			got, err := readSettingsFile(path)
			if err != nil {
				t.Fatalf("readSettingsFile() error = %v", err)
			}

			if !mapsEqual(got, tt.wantPost) {
				t.Fatalf("PruneEmptyClickSettingsKeys() result =\n%#v\nwant\n%#v", got, tt.wantPost)
			}
		})
	}
}

// TestPruneEmptyClickSettingsKeys_NoWriteWhenNothingChanged proves C.1(d): performs no write at all
// when nothing changed. This is verified by overriding createTempFile to return an error; if a write
// were attempted, the test would fail.
func TestPruneEmptyClickSettingsKeys_NoWriteWhenNothingChanged(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	// Seed with content that has nothing to prune (no empty Click keys)
	initial := map[string]any{
		"enabledPlugins":         map[string]any{"click-sdd": true},
		"extraKnownMarketplaces": map[string]any{"official": true},
		"pluginConfigs":          map[string]any{"click-sdd": map[string]any{"option": "value"}},
		"foreignKey":             "keep me",
	}
	if err := os.WriteFile(path, []byte(`{}`+"\n"), 0o600); err != nil {
		t.Fatalf("seed WriteFile() error = %v", err)
	}
	if err := writeSettingsFile(path, initial); err != nil {
		t.Fatalf("writeSettingsFile() initial seed error = %v", err)
	}

	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() original error = %v", err)
	}

	// Inject a failing createTempFile to prove no write happens
	injectedErr := errors.New("injected write failure - should never be called")
	old := createTempFile
	createTempFile = func(dir, pattern string) (tempFileWriter, error) {
		return &fakeFailingTempFile{name: filepath.Join(dir, ".click-injected-fake"), writeErr: injectedErr}, nil
	}
	defer func() { createTempFile = old }()

	cfg := Config{ClaudeHome: dir}
	pruneErr := PruneEmptyClickSettingsKeys(cfg)

	// If a write was attempted, createTempFile would have returned the failing writer
	// and pruneErr would be non-nil (wrapping injectedErr). Success proves no write.
	if pruneErr != nil {
		t.Fatalf("PruneEmptyClickSettingsKeys() error = %v, want nil when nothing changed (proves no write was attempted)", pruneErr)
	}

	// File should be byte-identical (no write happened)
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() after error = %v", err)
	}
	if string(got) != string(original) {
		t.Fatalf("file content = %q after prune with nothing to change, want original %q (proves no write was attempted)", got, original)
	}
}

// TestPruneEmptyClickSettingsKeys_EmptyDocumentBecomesEmptyObject proves C.1(e): pruning that empties
// the document writes {} and never deletes settings.json.
func TestPruneEmptyClickSettingsKeys_EmptyDocumentBecomesEmptyObject(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	// Seed with ONLY the three empty Click keys (nothing else)
	initial := map[string]any{
		"enabledPlugins":         map[string]any{},
		"extraKnownMarketplaces": map[string]any{},
		"pluginConfigs":          map[string]any{},
	}
	if err := os.WriteFile(path, []byte(`{}`+"\n"), 0o600); err != nil {
		t.Fatalf("seed WriteFile() error = %v", err)
	}
	if err := writeSettingsFile(path, initial); err != nil {
		t.Fatalf("writeSettingsFile() initial seed error = %v", err)
	}

	cfg := Config{ClaudeHome: dir}
	if err := PruneEmptyClickSettingsKeys(cfg); err != nil {
		t.Fatalf("PruneEmptyClickSettingsKeys() error = %v", err)
	}

	// File must still exist (never deleted)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("Stat() after prune that empties document error = %v, want file to exist", err)
	}

	// Content must be exactly {} with trailing newline (writeSettingsFile format), not null
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	got := string(data)
	want := "{}\n" // json.MarshalIndent produces {} for empty object, plus newline
	if got != want {
		t.Fatalf("file content = %q after prune that empties document, want exactly %q (writeSettingsFile format, never delete, never null)", got, want)
	}

	// Verify the content is actually an empty object, not empty maps or other structures
	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("json.Unmarshal(file content) error = %v", err)
	}
	if len(parsed) != 0 {
		t.Fatalf("parsed content has %d keys, want 0 (empty object), parsed = %#v", len(parsed), parsed)
	}
}

// TestPruneEmptyClickSettingsKeys_InjectedWriteErrorLeavesOriginalIntact proves C.1(f): an injected
// write failure leaves the original file byte-identical, following TestWriteSettingsFile_InjectedWriteErrorLeavesOriginalIntact's
// established pattern.
func TestPruneEmptyClickSettingsKeys_InjectedWriteErrorLeavesOriginalIntact(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	// Seed with content that WILL be pruned (three empty maps)
	initial := map[string]any{
		"enabledPlugins":         map[string]any{},
		"extraKnownMarketplaces": map[string]any{},
		"pluginConfigs":          map[string]any{},
		"foreignKey":             "keep me",
	}
	if err := os.WriteFile(path, []byte(`{}`+"\n"), 0o600); err != nil {
		t.Fatalf("seed WriteFile() error = %v", err)
	}
	if err := writeSettingsFile(path, initial); err != nil {
		t.Fatalf("writeSettingsFile() initial seed error = %v", err)
	}

	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() original error = %v", err)
	}

	// Inject a failing createTempFile to test error handling
	injectedErr := errors.New("injected write failure")
	old := createTempFile
	createTempFile = func(dir, pattern string) (tempFileWriter, error) {
		return &fakeFailingTempFile{name: filepath.Join(dir, ".click-injected-fake"), writeErr: injectedErr}, nil
	}
	defer func() { createTempFile = old }()

	cfg := Config{ClaudeHome: dir}
	pruneErr := PruneEmptyClickSettingsKeys(cfg)

	// Error must propagate
	if pruneErr == nil {
		t.Fatal("PruneEmptyClickSettingsKeys() error = nil, want the injected write error to propagate")
	}
	if !errors.Is(pruneErr, injectedErr) {
		t.Fatalf("PruneEmptyClickSettingsKeys() error = %v, want it to wrap %v", pruneErr, injectedErr)
	}

	// File must be byte-identical to original (no corruption on write failure)
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() after error = %v", err)
	}
	if string(got) != string(original) {
		t.Fatalf("file content = %q after a failed prune write, want untouched original %q", got, original)
	}
}

// mapsEqual is a test helper for deep map comparison.
func mapsEqual(a, b map[string]any) bool {
	if len(a) != len(b) {
		return false
	}
	for k, av := range a {
		bv, ok := b[k]
		if !ok {
			return false
		}
		// Type-aware comparison for this test's use cases
		switch av := av.(type) {
		case map[string]any:
			bvMap, ok := bv.(map[string]any)
			if !ok || !mapsEqual(av, bvMap) {
				return false
			}
		case []any:
			bvSlice, ok := bv.([]any)
			if !ok || !slicesEqual(av, bvSlice) {
				return false
			}
		default:
			// Handle JSON number normalization (int becomes float64)
			if !valuesEqual(av, bv) {
				return false
			}
		}
	}
	return true
}

// valuesEqual compares two values, handling JSON number normalization and type differences.
func valuesEqual(a, b any) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}

	// Handle float64 comparison (JSON numbers unmarshal to float64)
	af, aOk := a.(float64)
	bf, bOk := b.(float64)
	if aOk && bOk {
		return af == bf
	}

	// Handle int -> float64 comparison
	ai, aOk := a.(int)
	if aOk {
		if bf, bOk := b.(float64); bOk {
			return float64(ai) == bf
		}
	}
	bi, bOk := b.(int)
	if bOk {
		if af, aOk := a.(float64); aOk {
			return float64(bi) == af
		}
	}

	return a == b
}

// slicesEqual is a test helper for deep slice comparison.
func slicesEqual(a, b []any) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		switch av := a[i].(type) {
		case map[string]any:
			bvMap, ok := b[i].(map[string]any)
			if !ok || !mapsEqual(av, bvMap) {
				return false
			}
		case []any:
			bvSlice, ok := b[i].([]any)
			if !ok || !slicesEqual(av, bvSlice) {
				return false
			}
		default:
			if av != b[i] {
				return false
			}
		}
	}
	return true
}
