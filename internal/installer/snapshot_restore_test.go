package installer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestPrepareRestore_ProducesReportWithZeroWrites guards the prepare half of the preview/apply
// seam: PrepareRestore must classify drift (veto vs warnable) exactly like SnapshotDrift while
// writing NOTHING — every original file keeps its current content, so an interactive confirmation
// can safely sit between prepare and apply.
func TestPrepareRestore_ProducesReportWithZeroWrites(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir()}
	sharedPath := filepath.Join(cfg.ClaudeHome, "shared.txt")
	writeTestFile(t, cfg.ClaudeMDPath(), managedBlockMD("managed v1"))
	writeTestFile(t, cfg.SettingsPath(), `{"original":true}`)
	writeTestFile(t, sharedPath, "shared original\n")
	if err := SnapshotRun(cfg); err != nil {
		t.Fatalf("SnapshotRun() error = %v", err)
	}
	manifest, err := loadSnapshotManifest(cfg)
	if err != nil {
		t.Fatalf("loadSnapshotManifest() error = %v", err)
	}
	manifest.Entries = append(manifest.Entries, manifestEntry{
		OriginalPath: sharedPath,
		BackupFile:   "shared.txt",
		Existed:      true,
		DriftPolicy:  DriftPolicyNonVeto,
	})
	writeManifestDirect(t, cfg, manifest)
	writeTestFile(t, filepath.Join(snapshotLatestDir(cfg), "shared.txt"), "shared original\n")

	// Drift: a veto (managed block edit) and a warnable non-veto edit.
	writeTestFile(t, cfg.ClaudeMDPath(), managedBlockMD("hand-edited managed block"))
	writeTestFile(t, sharedPath, "shared edited by someone else\n")

	prepared, err := PrepareRestore(cfg)
	if err != nil {
		t.Fatalf("PrepareRestore() error = %v, want nil", err)
	}
	if len(prepared.Drift.Vetoes) != 1 || prepared.Drift.Vetoes[0] != cfg.ClaudeMDPath() {
		t.Fatalf("PrepareRestore().Drift.Vetoes = %v, want exactly [%s]", prepared.Drift.Vetoes, cfg.ClaudeMDPath())
	}
	if len(prepared.Drift.WarnableNonVeto) != 1 || prepared.Drift.WarnableNonVeto[0] != sharedPath {
		t.Fatalf("PrepareRestore().Drift.WarnableNonVeto = %v, want exactly [%s]", prepared.Drift.WarnableNonVeto, sharedPath)
	}

	// Zero writes: every original file must still hold its current (drifted) content.
	for path, want := range map[string]string{
		cfg.ClaudeMDPath(): managedBlockMD("hand-edited managed block"),
		cfg.SettingsPath(): `{"original":true}`,
		sharedPath:         "shared edited by someone else\n",
	} {
		got, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatalf("ReadFile(%s) after PrepareRestore error = %v", path, readErr)
		}
		if string(got) != want {
			t.Fatalf("%s after PrepareRestore = %q, want untouched %q (prepare must write nothing)", path, got, want)
		}
	}
}

// TestApplyPreparedRestore_RestoresEveryEntryIncludingNonVeto guards restore COVERAGE: once the
// caller has decided to proceed, apply restores EVERY manifest entry from its pre-run snapshot —
// including warnable non-veto entries (only veto scope is ownership-scoped, never coverage).
func TestApplyPreparedRestore_RestoresEveryEntryIncludingNonVeto(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir()}
	sharedPath := filepath.Join(cfg.ClaudeHome, "shared.txt")
	writeTestFile(t, cfg.ClaudeMDPath(), managedBlockMD("managed v1"))
	writeTestFile(t, cfg.SettingsPath(), `{"original":true}`)
	writeTestFile(t, sharedPath, "shared original\n")
	if err := SnapshotRun(cfg); err != nil {
		t.Fatalf("SnapshotRun() error = %v", err)
	}
	manifest, err := loadSnapshotManifest(cfg)
	if err != nil {
		t.Fatalf("loadSnapshotManifest() error = %v", err)
	}
	manifest.Entries = append(manifest.Entries, manifestEntry{
		OriginalPath: sharedPath,
		BackupFile:   "shared.txt",
		Existed:      true,
		DriftPolicy:  DriftPolicyNonVeto,
	})
	writeManifestDirect(t, cfg, manifest)
	writeTestFile(t, filepath.Join(snapshotLatestDir(cfg), "shared.txt"), "shared original\n")

	writeTestFile(t, cfg.ClaudeMDPath(), managedBlockMD("hand-edited managed block"))
	writeTestFile(t, sharedPath, "shared edited by someone else\n")

	prepared, err := PrepareRestore(cfg)
	if err != nil {
		t.Fatalf("PrepareRestore() error = %v", err)
	}
	if err := ApplyPreparedRestore(prepared); err != nil {
		t.Fatalf("ApplyPreparedRestore() error = %v, want nil", err)
	}

	for path, want := range map[string]string{
		cfg.ClaudeMDPath(): managedBlockMD("managed v1"),
		cfg.SettingsPath(): `{"original":true}`,
		sharedPath:         "shared original\n",
	} {
		got, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatalf("ReadFile(%s) after apply error = %v", path, readErr)
		}
		if string(got) != want {
			t.Fatalf("%s after apply = %q, want restored pre-run content %q", path, got, want)
		}
	}
}

// TestApplyPreparedRestore_RefusesWhenFingerprintChanged guards the TOCTOU guard: if any file's
// on-disk state changed between PrepareRestore and ApplyPreparedRestore (e.g. while an interactive
// confirmation prompt sat in between), apply must refuse BEFORE any write — leaving every file
// untouched, not just the changed one.
func TestApplyPreparedRestore_RefusesWhenFingerprintChanged(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir()}
	writeTestFile(t, cfg.ClaudeMDPath(), managedBlockMD("managed v1"))
	writeTestFile(t, cfg.SettingsPath(), `{"original":true}`)
	if err := SnapshotRun(cfg); err != nil {
		t.Fatalf("SnapshotRun() error = %v", err)
	}
	writeTestFile(t, cfg.ClaudeMDPath(), managedBlockMD("hand-edited managed block"))

	prepared, err := PrepareRestore(cfg)
	if err != nil {
		t.Fatalf("PrepareRestore() error = %v", err)
	}

	// Something edits a file after prepare (the prompt window).
	writeTestFile(t, cfg.SettingsPath(), `{"original":true,"edited-during-prompt":true}`)

	err = ApplyPreparedRestore(prepared)
	if err == nil {
		t.Fatal("ApplyPreparedRestore() error = nil, want a refusal when a fingerprint changed since prepare")
	}
	if !strings.Contains(err.Error(), cfg.SettingsPath()) {
		t.Fatalf("ApplyPreparedRestore() error = %v, want it to name the changed file %s", err, cfg.SettingsPath())
	}

	// Zero writes: the veto-drifted CLAUDE.md must NOT have been restored either.
	got, readErr := os.ReadFile(cfg.ClaudeMDPath())
	if readErr != nil {
		t.Fatalf("ReadFile(CLAUDE.md) after refused apply error = %v", readErr)
	}
	if string(got) != managedBlockMD("hand-edited managed block") {
		t.Fatalf("CLAUDE.md after refused apply = %q, want untouched (refusal must precede ANY write)", got)
	}
}

// TestRestoreRun_ComposesPrepareAndApply guards the thin-wrapper contract: RestoreRun restores the
// full manifest (including non-veto entries) without any prompting or drift refusal of its own,
// exactly as install/update's automatic failure-recovery paths require.
func TestRestoreRun_ComposesPrepareAndApply(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir()}
	writeTestFile(t, cfg.ClaudeMDPath(), managedBlockMD("managed v1"))
	writeTestFile(t, cfg.SettingsPath(), `{"original":true}`)
	if err := SnapshotRun(cfg); err != nil {
		t.Fatalf("SnapshotRun() error = %v", err)
	}
	writeTestFile(t, cfg.ClaudeMDPath(), managedBlockMD("drifted, would veto an interactive rollback"))
	writeTestFile(t, cfg.SettingsPath(), `{"drifted":true}`)

	if err := RestoreRun(cfg); err != nil {
		t.Fatalf("RestoreRun() error = %v, want nil (automatic recovery never refuses on drift)", err)
	}
	got, err := os.ReadFile(cfg.ClaudeMDPath())
	if err != nil {
		t.Fatalf("ReadFile(CLAUDE.md) error = %v", err)
	}
	if string(got) != managedBlockMD("managed v1") {
		t.Fatalf("CLAUDE.md after RestoreRun = %q, want restored pre-run content", got)
	}
}
