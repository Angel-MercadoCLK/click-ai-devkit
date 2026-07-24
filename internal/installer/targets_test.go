package installer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestTargetSelection_DefaultsToClaudeAndAutoDetectsOpenClaw(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir()}
	selection, found, err := LoadTargetSelection(cfg)
	if err != nil {
		t.Fatalf("LoadTargetSelection() error = %v", err)
	}
	if found {
		t.Fatal("LoadTargetSelection() found = true for a missing artifact")
	}
	if !selection.Claude || !selection.OpenClaw || selection.Configured {
		t.Fatalf("default selection = %+v, want Claude selected, OpenClaw auto-detect, unconfigured", selection)
	}
}

func TestTargetSelection_RoundTripsVersionedArtifact(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir()}
	want := TargetSelection{SchemaVersion: targetSelectionSchemaVersion, Configured: true, Claude: true, OpenClaw: false}
	if err := SaveTargetSelection(cfg, want); err != nil {
		t.Fatalf("SaveTargetSelection() error = %v", err)
	}

	got, found, err := LoadTargetSelection(cfg)
	if err != nil {
		t.Fatalf("LoadTargetSelection() error = %v", err)
	}
	if !found || got != want {
		t.Fatalf("LoadTargetSelection() = (%+v, %t), want (%+v, true)", got, found, want)
	}
	if _, err := os.Stat(filepath.Join(cfg.ClaudeHome, "click-ai-devkit", "targets.json")); err != nil {
		t.Fatalf("target artifact missing: %v", err)
	}
}

func TestTargetSelection_AllowsClaudeFreeTargetSelection(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir()}
	if err := SaveTargetSelection(cfg, TargetSelection{Configured: true, Claude: false, OpenClaw: true, Codex: true}); err != nil {
		t.Fatalf("SaveTargetSelection() error = %v, want Claude-free selection to persist", err)
	}
}

func TestResolveOpenClawTarget_ExplicitSelectionOverridesDetection(t *testing.T) {
	configured := TargetSelection{SchemaVersion: targetSelectionSchemaVersion, Configured: true, Claude: true, OpenClaw: false}
	if got := ResolveOpenClawTarget(configured, true); got {
		t.Fatal("ResolveOpenClawTarget() = true, want explicit deselection to override detection")
	}
	if got := ResolveOpenClawTarget(TargetSelection{Claude: true}, true); !got {
		t.Fatal("ResolveOpenClawTarget() = false, want legacy auto-detection when unconfigured")
	}
}

func TestCodexTarget_IsExplicitOnlyAndDetectable(t *testing.T) {
	configured := TargetSelection{SchemaVersion: targetSelectionSchemaVersion, Configured: true, Claude: true, Codex: true}
	if !ResolveCodexTarget(configured, true) {
		t.Fatal("ResolveCodexTarget() = false, want explicitly selected Codex")
	}
	if ResolveCodexTarget(configured, false) {
		t.Fatal("ResolveCodexTarget() = true, want absent Codex to be skipped")
	}
	if ResolveCodexTarget(TargetSelection{Claude: true}, true) {
		t.Fatal("ResolveCodexTarget() = true, want new Codex target disabled until explicitly selected")
	}
}

func TestCodexPath_UsesBinaryLookupSeam(t *testing.T) {
	restore := SetBinaryLookupFactoryForTests(func() BinaryLookup {
		return &fakeBinaryLookup{resolved: map[string]string{"codex": "/usr/bin/codex"}}
	})
	defer restore()

	path, ok := CodexPath()
	if !ok || path != "/usr/bin/codex" {
		t.Fatalf("CodexPath() = (%q, %t), want (/usr/bin/codex, true)", path, ok)
	}
}

func TestCodexGuidance_IsIdempotentRemovableAndSnapshotRestorable(t *testing.T) {
	codexHome := t.TempDir()
	cfg := Config{ClaudeHome: t.TempDir(), CodexHome: codexHome}
	path := cfg.CodexAgentsMDPath()
	if err := os.WriteFile(path, []byte("user guidance\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := SyncCodexGuidance(cfg); err != nil {
		t.Fatalf("SyncCodexGuidance() error = %v", err)
	}
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := SyncCodexGuidance(cfg); err != nil {
		t.Fatalf("second SyncCodexGuidance() error = %v", err)
	}
	second, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) || string(first) == "user guidance\n" {
		t.Fatalf("Codex guidance is not idempotent: first=%q second=%q", first, second)
	}
	if err := SnapshotRun(cfg); err != nil {
		t.Fatal(err)
	}
	if err := SyncCodexGuidance(Config{ClaudeHome: cfg.ClaudeHome, CodexHome: codexHome}); err != nil {
		t.Fatal(err)
	}
	if err := StripCodexGuidance(cfg); err != nil {
		t.Fatal(err)
	}
	if err := RestoreRun(cfg); err != nil {
		t.Fatal(err)
	}
	restored, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(restored) != string(first) {
		t.Fatalf("restored Codex guidance = %q, want %q", restored, first)
	}
}

func TestTargetSelection_IsIncludedInSnapshotAndRestore(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir()}
	want := TargetSelection{Configured: true, Claude: true, OpenClaw: false}
	if err := SaveTargetSelection(cfg, want); err != nil {
		t.Fatalf("SaveTargetSelection() error = %v", err)
	}
	if err := SnapshotRun(cfg); err != nil {
		t.Fatalf("SnapshotRun() error = %v", err)
	}
	if err := SaveTargetSelection(cfg, TargetSelection{Configured: true, Claude: true, OpenClaw: true}); err != nil {
		t.Fatalf("SaveTargetSelection(updated) error = %v", err)
	}
	if err := RestoreRun(cfg); err != nil {
		t.Fatalf("RestoreRun() error = %v", err)
	}
	got, found, err := LoadTargetSelection(cfg)
	wantRestored := TargetSelection{SchemaVersion: targetSelectionSchemaVersion, Configured: true, Claude: true, OpenClaw: false}
	if err != nil || !found || got != wantRestored {
		t.Fatalf("restored selection = (%+v, %t), err = %v", got, found, err)
	}
	manifestData, err := os.ReadFile(filepath.Join(cfg.BackupDir(), "latest", snapshotManifestName))
	if err != nil {
		t.Fatalf("ReadFile(manifest) error = %v", err)
	}
	var manifest runManifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	if len(manifest.Entries) != 3 || manifest.Entries[2].BackupFile != "targets.json" {
		t.Fatalf("snapshot entries = %+v, want target artifact as third entry", manifest.Entries)
	}
}
