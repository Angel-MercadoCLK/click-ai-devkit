package installer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The backup directory used to be rooted at ClaudeHome, which is only populated
// when Claude is among the selected runtimes. A Codex-only or OpenClaw-only run
// left it empty, so filepath.Join("", ...) produced a RELATIVE path and click
// wrote backups into whatever directory it happened to be launched from. Those
// runs do have real files to snapshot (Codex config.toml, AGENTS.md, model
// profiles), so the safety net was landing in the wrong place rather than being
// unnecessary.
//
// BackupDir is now rooted at ClickStateHome, which is resolved unconditionally.

func TestConfig_BackupDir_RootedAtClickStateHome(t *testing.T) {
	cfg := Config{
		ClaudeHome:     filepath.Join("some", "home", ".claude"),
		ClickStateHome: filepath.Join("some", "state", "click-ai-devkit"),
	}

	want := filepath.Join("some", "state", "click-ai-devkit", "click-ai-devkit", "backups")
	if got := cfg.BackupDir(); got != want {
		t.Errorf("BackupDir() = %q, want %q", got, want)
	}
}

// The regression this whole change exists to prevent.
func TestConfig_BackupDir_ClaudeLessSelectionStaysAbsolute(t *testing.T) {
	stateHome := t.TempDir() // absolute
	cfg := Config{ClickStateHome: stateHome}

	got := cfg.BackupDir()
	if !filepath.IsAbs(got) {
		t.Errorf("BackupDir() = %q, want an absolute path when only ClickStateHome is set", got)
	}
	if got == filepath.Join("click-ai-devkit", "backups") {
		t.Errorf("BackupDir() fell back to a relative path under the process cwd: %q", got)
	}
}

func TestConfig_LegacyBackupDir(t *testing.T) {
	t.Run("rooted at ClaudeHome when set", func(t *testing.T) {
		cfg := Config{ClaudeHome: filepath.Join("some", "home", ".claude")}
		want := filepath.Join("some", "home", ".claude", "click-ai-devkit", "backups")
		if got := cfg.LegacyBackupDir(); got != want {
			t.Errorf("LegacyBackupDir() = %q, want %q", got, want)
		}
	})

	t.Run("empty when ClaudeHome is unset", func(t *testing.T) {
		cfg := Config{ClickStateHome: filepath.Join("some", "state")}
		if got := cfg.LegacyBackupDir(); got != "" {
			t.Errorf("LegacyBackupDir() = %q, want \"\" so callers can skip the fallback", got)
		}
	})
}

// seedSnapshot writes a minimal but valid manifest into <backupRoot>/latest/.
func seedSnapshot(t *testing.T, backupRoot, marker string) {
	t.Helper()
	latest := filepath.Join(backupRoot, "latest")
	if err := os.MkdirAll(latest, 0o755); err != nil {
		t.Fatalf("seed snapshot dir: %v", err)
	}
	manifest := `{"entries":[{"originalPath":"` + marker +
		`","backupFile":"CLAUDE.md","existed":true,"driftPolicy":"whole-file-veto"}]}`
	if err := os.WriteFile(filepath.Join(latest, snapshotManifestName), []byte(manifest), 0o600); err != nil {
		t.Fatalf("seed manifest: %v", err)
	}
}

// An existing install upgraded to this version still has its snapshot in the old
// ClaudeHome-rooted location. Rollback must keep seeing it instead of silently
// reporting "no snapshot" and dropping the user's safety net.
func TestSnapshot_FallsBackToLegacyLocationForReads(t *testing.T) {
	claudeHome := t.TempDir()
	cfg := Config{ClaudeHome: claudeHome, ClickStateHome: t.TempDir()}

	seedSnapshot(t, cfg.LegacyBackupDir(), "legacy-marker")

	found, err := HasRunSnapshot(cfg)
	if err != nil {
		t.Fatalf("HasRunSnapshot: %v", err)
	}
	if !found {
		t.Fatal("HasRunSnapshot() = false; want true from the legacy ClaudeHome-rooted snapshot")
	}

	manifest, err := loadSnapshotManifest(cfg)
	if err != nil {
		t.Fatalf("loadSnapshotManifest: %v", err)
	}
	if len(manifest.Entries) != 1 || manifest.Entries[0].OriginalPath != "legacy-marker" {
		t.Errorf("loaded manifest = %+v, want the legacy one", manifest.Entries)
	}
}

// Once a snapshot exists at the current location it always wins; the legacy copy
// is never consulted again.
func TestSnapshot_CurrentLocationWinsOverLegacy(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir(), ClickStateHome: t.TempDir()}

	seedSnapshot(t, cfg.LegacyBackupDir(), "legacy-marker")
	seedSnapshot(t, cfg.BackupDir(), "current-marker")

	manifest, err := loadSnapshotManifest(cfg)
	if err != nil {
		t.Fatalf("loadSnapshotManifest: %v", err)
	}
	if len(manifest.Entries) != 1 || manifest.Entries[0].OriginalPath != "current-marker" {
		t.Errorf("loaded manifest = %+v, want the current one", manifest.Entries)
	}
}

// The fallback is read-only: a new snapshot must never be written back into the
// legacy directory, otherwise the migration would never actually complete.
func TestSnapshotRun_AlwaysWritesToCurrentLocation(t *testing.T) {
	claudeHome := t.TempDir()
	cfg := Config{ClaudeHome: claudeHome, ClickStateHome: t.TempDir()}

	seedSnapshot(t, cfg.LegacyBackupDir(), "legacy-marker")

	source := filepath.Join(claudeHome, "CLAUDE.md")
	if err := os.WriteFile(source, []byte("# managed\n"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}

	if err := snapshotRunWithSources(cfg, []snapshotSource{
		{originalPath: source, backupFile: "CLAUDE.md", policy: DriftPolicyWholeFileVeto},
	}); err != nil {
		t.Fatalf("snapshotRunWithSources: %v", err)
	}

	currentManifest := filepath.Join(cfg.BackupDir(), "latest", snapshotManifestName)
	if _, err := os.Stat(currentManifest); err != nil {
		t.Errorf("expected a fresh snapshot at the current location: %v", err)
	}

	// The legacy manifest must be left exactly as it was.
	legacy, err := os.ReadFile(filepath.Join(cfg.LegacyBackupDir(), "latest", snapshotManifestName))
	if err != nil {
		t.Fatalf("read legacy manifest: %v", err)
	}
	if !strings.Contains(string(legacy), "legacy-marker") {
		t.Error("the legacy snapshot was overwritten; the fallback must be read-only")
	}
}
