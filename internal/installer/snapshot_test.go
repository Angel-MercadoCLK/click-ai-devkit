package installer

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// writeTestFile is a small helper shared by this file's tests: it creates path's parent directory
// (Config's root, e.g. t.TempDir(), always exists already, but this keeps callers uniform) and
// writes content, failing the test immediately on any error.
func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", path, err)
	}
}

// TestSnapshotRun_CopiesBothFilesAndWritesManifest guards the Requirement: Single Run-Start
// Snapshot Before Any Write / "Repeated install, files exist" scenario: an existing CLAUDE.md and
// settings.json must both be copied into BackupDir()/latest/, with a manifest.json recording
// originalPath/backupFile/existed for each.
func TestSnapshotRun_CopiesBothFilesAndWritesManifest(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir(), ClickStateHome: t.TempDir()}
	writeTestFile(t, cfg.ClaudeMDPath(), "# my CLAUDE.md\n")
	writeTestFile(t, cfg.SettingsPath(), `{"hooks":{}}`)

	if err := SnapshotRun(cfg); err != nil {
		t.Fatalf("SnapshotRun() error = %v, want nil", err)
	}

	latestDir := filepath.Join(cfg.BackupDir(), "latest")
	manifestRaw, err := os.ReadFile(filepath.Join(latestDir, "manifest.json"))
	if err != nil {
		t.Fatalf("ReadFile(manifest.json) error = %v, want it written by SnapshotRun", err)
	}
	var manifest runManifest
	if err := json.Unmarshal(manifestRaw, &manifest); err != nil {
		t.Fatalf("json.Unmarshal(manifest.json) error = %v", err)
	}
	if len(manifest.Entries) != 2 {
		t.Fatalf("manifest.Entries = %#v, want exactly 2 entries (CLAUDE.md + settings.json)", manifest.Entries)
	}

	byOriginal := make(map[string]manifestEntry, len(manifest.Entries))
	for _, e := range manifest.Entries {
		byOriginal[e.OriginalPath] = e
	}

	claudeEntry, ok := byOriginal[cfg.ClaudeMDPath()]
	if !ok {
		t.Fatalf("manifest has no entry for %s", cfg.ClaudeMDPath())
	}
	if !claudeEntry.Existed {
		t.Fatal("manifest entry for CLAUDE.md: Existed = false, want true (source file existed)")
	}
	if claudeEntry.BackupFile == "" {
		t.Fatal("manifest entry for CLAUDE.md: BackupFile is empty, want a recorded backup file name")
	}
	gotClaude, err := os.ReadFile(filepath.Join(latestDir, claudeEntry.BackupFile))
	if err != nil {
		t.Fatalf("ReadFile(backup CLAUDE.md) error = %v", err)
	}
	if string(gotClaude) != "# my CLAUDE.md\n" {
		t.Fatalf("backup CLAUDE.md content = %q, want %q", gotClaude, "# my CLAUDE.md\n")
	}

	settingsEntry, ok := byOriginal[cfg.SettingsPath()]
	if !ok {
		t.Fatalf("manifest has no entry for %s", cfg.SettingsPath())
	}
	if !settingsEntry.Existed {
		t.Fatal("manifest entry for settings.json: Existed = false, want true (source file existed)")
	}
	gotSettings, err := os.ReadFile(filepath.Join(latestDir, settingsEntry.BackupFile))
	if err != nil {
		t.Fatalf("ReadFile(backup settings.json) error = %v", err)
	}
	if string(gotSettings) != `{"hooks":{}}` {
		t.Fatalf("backup settings.json content = %q, want %q", gotSettings, `{"hooks":{}}`)
	}
}

// TestSnapshotRun_MissingSource_RecordsNoPriorStateMarker guards spec Decision 1 / the "First-ever
// install, no prior file" scenario: when CLAUDE.md/settings.json don't exist yet, SnapshotRun must
// NOT error and must record an explicit no-prior-state marker (Existed=false, no backup file) —
// never an empty/missing manifest and never a fabricated backup file.
func TestSnapshotRun_MissingSource_RecordsNoPriorStateMarker(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir(), ClickStateHome: t.TempDir()}

	if err := SnapshotRun(cfg); err != nil {
		t.Fatalf("SnapshotRun() error = %v, want nil even when no source files exist yet", err)
	}

	latestDir := filepath.Join(cfg.BackupDir(), "latest")
	manifestRaw, err := os.ReadFile(filepath.Join(latestDir, "manifest.json"))
	if err != nil {
		t.Fatalf("ReadFile(manifest.json) error = %v, want a manifest recording the no-prior-state marker", err)
	}
	var manifest runManifest
	if err := json.Unmarshal(manifestRaw, &manifest); err != nil {
		t.Fatalf("json.Unmarshal(manifest.json) error = %v", err)
	}
	if len(manifest.Entries) != 2 {
		t.Fatalf("manifest.Entries = %#v, want exactly 2 entries even when both sources are absent", manifest.Entries)
	}
	for _, e := range manifest.Entries {
		if e.Existed {
			t.Fatalf("manifest entry for %s: Existed = true, want false (no-prior-state marker)", e.OriginalPath)
		}
		if e.BackupFile != "" {
			t.Fatalf("manifest entry for %s: BackupFile = %q, want empty (nothing was copied)", e.OriginalPath, e.BackupFile)
		}
	}
}

// TestRestoreRun_RestoresBothFilesByteForByte guards the "Successful restore" scenario: after
// SnapshotRun, editing both files, then RestoreRun, both files must come back byte-for-byte to
// their snapshotted content.
func TestRestoreRun_RestoresBothFilesByteForByte(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir(), ClickStateHome: t.TempDir()}
	writeTestFile(t, cfg.ClaudeMDPath(), "original CLAUDE.md\n")
	writeTestFile(t, cfg.SettingsPath(), `{"original":true}`)

	if err := SnapshotRun(cfg); err != nil {
		t.Fatalf("SnapshotRun() error = %v", err)
	}

	writeTestFile(t, cfg.ClaudeMDPath(), "edited CLAUDE.md\n")
	writeTestFile(t, cfg.SettingsPath(), `{"edited":true}`)

	if err := RestoreRun(cfg); err != nil {
		t.Fatalf("RestoreRun() error = %v, want nil", err)
	}

	gotClaude, err := os.ReadFile(cfg.ClaudeMDPath())
	if err != nil {
		t.Fatalf("ReadFile(CLAUDE.md) error = %v", err)
	}
	if string(gotClaude) != "original CLAUDE.md\n" {
		t.Fatalf("CLAUDE.md after RestoreRun() = %q, want the original snapshotted content %q", gotClaude, "original CLAUDE.md\n")
	}

	gotSettings, err := os.ReadFile(cfg.SettingsPath())
	if err != nil {
		t.Fatalf("ReadFile(settings.json) error = %v", err)
	}
	if string(gotSettings) != `{"original":true}` {
		t.Fatalf("settings.json after RestoreRun() = %q, want the original snapshotted content %q", gotSettings, `{"original":true}`)
	}
}

// TestRestoreRun_ExistedFalseRemovesOriginal guards the "No snapshot exists" / no-prior-state
// half of restore: when a file did NOT exist at snapshot time (Existed=false), RestoreRun must
// remove any file that has since appeared at that original path, rather than leaving it in place
// or fabricating content.
func TestRestoreRun_ExistedFalseRemovesOriginal(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir(), ClickStateHome: t.TempDir()}
	// Neither file exists at snapshot time -> both entries are no-prior-state markers.
	if err := SnapshotRun(cfg); err != nil {
		t.Fatalf("SnapshotRun() error = %v", err)
	}

	// Simulate a file having since been created at the original path.
	writeTestFile(t, cfg.ClaudeMDPath(), "created after snapshot, must be removed on restore\n")

	if err := RestoreRun(cfg); err != nil {
		t.Fatalf("RestoreRun() error = %v, want nil", err)
	}

	if _, err := os.Stat(cfg.ClaudeMDPath()); !os.IsNotExist(err) {
		t.Fatalf("Stat(CLAUDE.md) after RestoreRun() error = %v, want os.IsNotExist (Existed=false must remove it)", err)
	}
}

// TestRestoreRun_BackupSurvivesRestore guards that RestoreRun is a read+write (copy), never a
// consuming move: the snapshot files under backups/latest/ must still be present and unchanged
// after a restore, so rollback can be run again later.
func TestRestoreRun_BackupSurvivesRestore(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir(), ClickStateHome: t.TempDir()}
	writeTestFile(t, cfg.ClaudeMDPath(), "snapshot content\n")
	writeTestFile(t, cfg.SettingsPath(), `{"snapshot":true}`)
	if err := SnapshotRun(cfg); err != nil {
		t.Fatalf("SnapshotRun() error = %v", err)
	}

	if err := RestoreRun(cfg); err != nil {
		t.Fatalf("RestoreRun() error = %v", err)
	}

	latestDir := filepath.Join(cfg.BackupDir(), "latest")
	if _, err := os.Stat(filepath.Join(latestDir, "manifest.json")); err != nil {
		t.Fatalf("Stat(manifest.json) after RestoreRun() error = %v, want the snapshot to survive restore", err)
	}
	has, err := HasRunSnapshot(cfg)
	if err != nil {
		t.Fatalf("HasRunSnapshot() error = %v", err)
	}
	if !has {
		t.Fatal("HasRunSnapshot() = false after RestoreRun(), want true (snapshot must survive restore)")
	}
}

// TestHasRunSnapshot_FalseWhenAbsent guards the base case: a home where SnapshotRun never ran must
// report no run snapshot.
func TestHasRunSnapshot_FalseWhenAbsent(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir(), ClickStateHome: t.TempDir()}

	has, err := HasRunSnapshot(cfg)
	if err != nil {
		t.Fatalf("HasRunSnapshot() error = %v, want nil", err)
	}
	if has {
		t.Fatal("HasRunSnapshot() = true for a home where SnapshotRun never ran, want false")
	}
}

// TestHasRunSnapshot_TrueWhenManifestPresent guards the base positive case: after a real
// SnapshotRun with existing source files, HasRunSnapshot must report true.
func TestHasRunSnapshot_TrueWhenManifestPresent(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir(), ClickStateHome: t.TempDir()}
	writeTestFile(t, cfg.ClaudeMDPath(), "content\n")
	writeTestFile(t, cfg.SettingsPath(), `{}`)
	if err := SnapshotRun(cfg); err != nil {
		t.Fatalf("SnapshotRun() error = %v", err)
	}

	has, err := HasRunSnapshot(cfg)
	if err != nil {
		t.Fatalf("HasRunSnapshot() error = %v", err)
	}
	if !has {
		t.Fatal("HasRunSnapshot() = false right after SnapshotRun(), want true")
	}
}

// TestHasRunSnapshot_TrueEvenWhenAllEntriesAreNoPriorState guards the distinction between "a run
// happened" (HasRunSnapshot's own contract) and "there is content to restore" (a separate,
// per-entry concern for the future rollback command in PR3): a run whose sources were ALL absent
// at snapshot time (no-prior-state markers) still recorded a real run — HasRunSnapshot must report
// true. Callers that need "is there anything to actually restore" must inspect each manifest
// entry's Existed flag themselves (PR3's concern), not rely on HasRunSnapshot for that.
func TestHasRunSnapshot_TrueEvenWhenAllEntriesAreNoPriorState(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir(), ClickStateHome: t.TempDir()}
	// Neither CLAUDE.md nor settings.json exists -> both manifest entries are no-prior-state.
	if err := SnapshotRun(cfg); err != nil {
		t.Fatalf("SnapshotRun() error = %v", err)
	}

	has, err := HasRunSnapshot(cfg)
	if err != nil {
		t.Fatalf("HasRunSnapshot() error = %v", err)
	}
	if !has {
		t.Fatal("HasRunSnapshot() = false for an all-no-prior-state manifest, want true (a run still happened)")
	}
}

// TestSnapshotRun_InjectedTempFileFailure_LeavesPriorSnapshotAndOriginalsUntouched is the
// strict-TDD required last-known-good proof (spec Decision 2 / design's "Retention" decision): a
// SECOND SnapshotRun that fails partway through (injected createTempFile failure) must leave the
// FIRST run's completed backups/latest/ snapshot exactly as it was — never overwritten, never
// left in an ambiguous half-written state — and must never touch the original source files it only
// reads from.
func TestSnapshotRun_InjectedTempFileFailure_LeavesPriorSnapshotAndOriginalsUntouched(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir(), ClickStateHome: t.TempDir()}
	writeTestFile(t, cfg.ClaudeMDPath(), "first run content\n")
	writeTestFile(t, cfg.SettingsPath(), `{"run":1}`)

	if err := SnapshotRun(cfg); err != nil {
		t.Fatalf("SnapshotRun() (first, successful run) error = %v", err)
	}
	latestDir := filepath.Join(cfg.BackupDir(), "latest")
	firstManifest, err := os.ReadFile(filepath.Join(latestDir, "manifest.json"))
	if err != nil {
		t.Fatalf("ReadFile(manifest.json) after first SnapshotRun() error = %v", err)
	}

	// A second run starts: sources change, but the snapshot write is injected to fail.
	writeTestFile(t, cfg.ClaudeMDPath(), "second run content, must not appear in the snapshot\n")
	writeTestFile(t, cfg.SettingsPath(), `{"run":2}`)

	injectedErr := errors.New("injected temp file failure")
	old := createTempFile
	createTempFile = func(dir, pattern string) (tempFileWriter, error) {
		return nil, injectedErr
	}
	defer func() { createTempFile = old }()

	err = SnapshotRun(cfg)
	if err == nil {
		t.Fatal("SnapshotRun() (second, injected-failure run) error = nil, want the injected failure to propagate")
	}
	if !errors.Is(err, injectedErr) {
		t.Fatalf("SnapshotRun() error = %v, want it to wrap %v", err, injectedErr)
	}

	// The prior (first) snapshot must remain exactly as it was.
	secondAttemptManifest, err := os.ReadFile(filepath.Join(latestDir, "manifest.json"))
	if err != nil {
		t.Fatalf("ReadFile(manifest.json) after the failed second SnapshotRun() error = %v, want the first run's snapshot to remain", err)
	}
	if string(secondAttemptManifest) != string(firstManifest) {
		t.Fatalf("manifest.json after a failed second run = %s, want it unchanged from the first successful run %s", secondAttemptManifest, firstManifest)
	}
	backupClaude, err := os.ReadFile(filepath.Join(latestDir, "CLAUDE.md"))
	if err != nil {
		t.Fatalf("ReadFile(backup CLAUDE.md) error = %v", err)
	}
	if string(backupClaude) != "first run content\n" {
		t.Fatalf("backup CLAUDE.md after a failed second run = %q, want the first run's content %q (last-known-good must not be overwritten)", backupClaude, "first run content\n")
	}

	// The original source files (which SnapshotRun only ever reads) must be untouched by the
	// failed attempt too — still whatever the second run's setup wrote.
	gotOriginal, err := os.ReadFile(cfg.ClaudeMDPath())
	if err != nil {
		t.Fatalf("ReadFile(CLAUDE.md) error = %v", err)
	}
	if string(gotOriginal) != "second run content, must not appear in the snapshot\n" {
		t.Fatalf("original CLAUDE.md after a failed SnapshotRun() = %q, want it left exactly as the caller wrote it (SnapshotRun must never mutate its sources)", gotOriginal)
	}
}

// TestCanonicalContentHash_CRLFAndLFEqual guards the shared hash helper (extracted for PR3's
// rollback drift check and PR4's doctor drift check, design's "Drift hash" decision): a CRLF-saved
// and an LF-saved file with the same logical content must hash identically.
func TestCanonicalContentHash_CRLFAndLFEqual(t *testing.T) {
	lf := "line one\nline two\n"
	crlf := "line one\r\nline two\r\n"

	gotLF := CanonicalContentHash(lf)
	gotCRLF := CanonicalContentHash(crlf)
	if gotLF != gotCRLF {
		t.Fatalf("CanonicalContentHash(LF) = %q, CanonicalContentHash(CRLF) = %q, want equal for the same logical content", gotLF, gotCRLF)
	}
}

// TestCanonicalContentHash_DifferentContentDiffers triangulates against the trivial
// "always returns the same hash" implementation: genuinely different content must hash
// differently.
func TestCanonicalContentHash_DifferentContentDiffers(t *testing.T) {
	got1 := CanonicalContentHash("content A\n")
	got2 := CanonicalContentHash("content B\n")
	if got1 == got2 {
		t.Fatalf("CanonicalContentHash(%q) == CanonicalContentHash(%q) == %q, want different hashes for different content", "content A\n", "content B\n", got1)
	}
}

// TestHasRestorableSnapshot_FalseWhenNoSnapshotAtAll guards the base "never ran" case: no manifest
// at all means nothing to restore (PR3's `click rollback` "No snapshot exists" scenario).
func TestHasRestorableSnapshot_FalseWhenNoSnapshotAtAll(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir(), ClickStateHome: t.TempDir()}

	has, err := HasRestorableSnapshot(cfg)
	if err != nil {
		t.Fatalf("HasRestorableSnapshot() error = %v, want nil", err)
	}
	if has {
		t.Fatal("HasRestorableSnapshot() = true when no snapshot ever ran, want false")
	}
}

// TestHasRestorableSnapshot_FalseWhenAllEntriesNoPriorState guards the finer half of the same
// scenario: a real run completed (HasRunSnapshot=true) but every entry is a no-prior-state
// marker (both CLAUDE.md and settings.json were absent at snapshot time) -> still nothing to
// restore. This is exactly the distinction HasRunSnapshot's own doc comment defers to a future
// caller — this is that caller.
func TestHasRestorableSnapshot_FalseWhenAllEntriesNoPriorState(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir(), ClickStateHome: t.TempDir()}
	if err := SnapshotRun(cfg); err != nil {
		t.Fatalf("SnapshotRun() error = %v", err)
	}

	has, err := HasRestorableSnapshot(cfg)
	if err != nil {
		t.Fatalf("HasRestorableSnapshot() error = %v, want nil", err)
	}
	if has {
		t.Fatal("HasRestorableSnapshot() = true for an all-no-prior-state manifest, want false (nothing to restore)")
	}
}

// TestHasRestorableSnapshot_TrueWhenAtLeastOneEntryExisted triangulates against the trivial
// "always false" implementation: a real snapshot with actual backed-up content must report true.
func TestHasRestorableSnapshot_TrueWhenAtLeastOneEntryExisted(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir(), ClickStateHome: t.TempDir()}
	writeTestFile(t, cfg.ClaudeMDPath(), "content\n")
	if err := SnapshotRun(cfg); err != nil {
		t.Fatalf("SnapshotRun() error = %v", err)
	}

	has, err := HasRestorableSnapshot(cfg)
	if err != nil {
		t.Fatalf("HasRestorableSnapshot() error = %v, want nil", err)
	}
	if !has {
		t.Fatal("HasRestorableSnapshot() = false when CLAUDE.md existed at snapshot time, want true")
	}
}

// managedBlockMD builds a CLAUDE.md-shaped markdown file whose click-managed block contains
// managed, with editable developer content above and below the markers.
func managedBlockMD(managed string) string {
	return "# developer header\n" + managedBeginMarker + "\n" + managed + "\n" + managedEndMarker + "\ndeveloper tail\n"
}

// writeManifestDirect writes a manifest.json (and no backup files) straight into
// BackupDir()/latest/, for tests that hand-craft manifest entries with specific drift policies.
func writeManifestDirect(t *testing.T, cfg Config, manifest runManifest) {
	t.Helper()
	if err := os.MkdirAll(snapshotLatestDir(cfg), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", snapshotLatestDir(cfg), err)
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatalf("json.MarshalIndent(manifest) error = %v", err)
	}
	if err := os.WriteFile(snapshotManifestPath(cfg), append(data, '\n'), 0o600); err != nil {
		t.Fatalf("WriteFile(manifest.json) error = %v", err)
	}
}

// TestSnapshotDrift_NoEdits_ReportsNoDrift guards the "matching hash" half of spec install-rollback
// Decision 3: content unchanged since the snapshot must report zero vetoes and zero warnings.
func TestSnapshotDrift_NoEdits_ReportsNoDrift(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir(), ClickStateHome: t.TempDir()}
	writeTestFile(t, cfg.ClaudeMDPath(), "unchanged content\n")
	writeTestFile(t, cfg.SettingsPath(), `{"unchanged":true}`)
	if err := SnapshotRun(cfg); err != nil {
		t.Fatalf("SnapshotRun() error = %v", err)
	}

	report, err := SnapshotDrift(cfg)
	if err != nil {
		t.Fatalf("SnapshotDrift() error = %v, want nil", err)
	}
	if len(report.Vetoes) != 0 || len(report.WarnableNonVeto) != 0 {
		t.Fatalf("SnapshotDrift() = %+v, want empty report (no edits since snapshot)", report)
	}
}

// TestSnapshotDrift_EditedFile_ReportsDrift triangulates against the trivial "always empty"
// implementation: editing click-owned content (CLAUDE.md's managed block) after the snapshot must
// be reported as a veto for that path, while the untouched settings.json must not be reported.
func TestSnapshotDrift_EditedFile_ReportsDrift(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir(), ClickStateHome: t.TempDir()}
	writeTestFile(t, cfg.ClaudeMDPath(), managedBlockMD("managed v1"))
	writeTestFile(t, cfg.SettingsPath(), `{"original":true}`)
	if err := SnapshotRun(cfg); err != nil {
		t.Fatalf("SnapshotRun() error = %v", err)
	}

	writeTestFile(t, cfg.ClaudeMDPath(), managedBlockMD("hand-edited managed block"))

	report, err := SnapshotDrift(cfg)
	if err != nil {
		t.Fatalf("SnapshotDrift() error = %v, want nil", err)
	}
	if len(report.Vetoes) != 1 || report.Vetoes[0] != cfg.ClaudeMDPath() {
		t.Fatalf("SnapshotDrift().Vetoes = %v, want exactly [%s]", report.Vetoes, cfg.ClaudeMDPath())
	}
	if len(report.WarnableNonVeto) != 0 {
		t.Fatalf("SnapshotDrift().WarnableNonVeto = %v, want empty (a vetoing file is not merely warnable)", report.WarnableNonVeto)
	}
}

// TestSnapshotDrift_MissingCurrentFile_NotReportedAsDrift guards the deliberate exception: a file
// deleted since the snapshot is not reported as drift (RestoreRun would simply recreate the
// known-good content, which is the safe, expected outcome — not a hand-edit to warn about), for
// BOTH veto-policy and non-veto entries.
func TestSnapshotDrift_MissingCurrentFile_NotReportedAsDrift(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir(), ClickStateHome: t.TempDir()}
	writeTestFile(t, cfg.ClaudeMDPath(), "content\n")
	sharedPath := filepath.Join(cfg.ClaudeHome, "shared.txt")
	writeTestFile(t, sharedPath, "shared original\n")
	if err := SnapshotRun(cfg); err != nil {
		t.Fatalf("SnapshotRun() error = %v", err)
	}
	// Add a non-veto entry to the real snapshot's manifest, with its own backup file.
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

	if err := os.Remove(cfg.ClaudeMDPath()); err != nil {
		t.Fatalf("os.Remove(CLAUDE.md) error = %v", err)
	}
	if err := os.Remove(sharedPath); err != nil {
		t.Fatalf("os.Remove(shared.txt) error = %v", err)
	}

	report, err := SnapshotDrift(cfg)
	if err != nil {
		t.Fatalf("SnapshotDrift() error = %v, want nil", err)
	}
	if len(report.Vetoes) != 0 || len(report.WarnableNonVeto) != 0 {
		t.Fatalf("SnapshotDrift() = %+v, want empty (a missing current file is neither a veto nor a warning)", report)
	}
}

// TestSnapshotDrift_ManagedContentVeto_EditOutsideMarkers_NoVeto is the core ownership-scoping
// guarantee: a developer's own edits OUTSIDE CLAUDE.md's managed markers touch no click-owned
// content, so they must never veto a rollback (and a managed-content-veto entry is never merely
// warnable either).
func TestSnapshotDrift_ManagedContentVeto_EditOutsideMarkers_NoVeto(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir(), ClickStateHome: t.TempDir()}
	writeTestFile(t, cfg.ClaudeMDPath(), managedBlockMD("managed v1"))
	writeTestFile(t, cfg.SettingsPath(), `{"original":true}`)
	if err := SnapshotRun(cfg); err != nil {
		t.Fatalf("SnapshotRun() error = %v", err)
	}

	writeTestFile(t, cfg.ClaudeMDPath(), "# developer header, hand-edited\n"+managedBeginMarker+"\nmanaged v1\n"+managedEndMarker+"\ndeveloper tail, hand-edited\n")

	report, err := SnapshotDrift(cfg)
	if err != nil {
		t.Fatalf("SnapshotDrift() error = %v, want nil", err)
	}
	if len(report.Vetoes) != 0 || len(report.WarnableNonVeto) != 0 {
		t.Fatalf("SnapshotDrift() = %+v, want empty (edits outside the managed markers own no click content)", report)
	}
}

// TestSnapshotDrift_ManagedContentVeto_SettingsUnrelatedKey_NoVeto covers the settings.json half of
// the same guarantee: churn on unrelated settings keys (e.g. Claude Code's own writes) must never
// veto; only edits to click's owned hook entry may.
func TestSnapshotDrift_ManagedContentVeto_SettingsUnrelatedKey_NoVeto(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir(), ClickStateHome: t.TempDir()}
	owned := `{"hooks":{"PreToolUse":[{"matcher":"` + MemoryGuardToolMatcher + `","hooks":[{"type":"command","command":"` + MemoryGuardCommand + `"}]}]},"other":true}`
	writeTestFile(t, cfg.ClaudeMDPath(), managedBlockMD("managed v1"))
	writeTestFile(t, cfg.SettingsPath(), owned)
	if err := SnapshotRun(cfg); err != nil {
		t.Fatalf("SnapshotRun() error = %v", err)
	}

	// Unrelated-key churn: no veto.
	writeTestFile(t, cfg.SettingsPath(), `{"hooks":{"PreToolUse":[{"matcher":"`+MemoryGuardToolMatcher+`","hooks":[{"type":"command","command":"`+MemoryGuardCommand+`"}]}]},"other":false,"claude-code-churn":123}`)
	report, err := SnapshotDrift(cfg)
	if err != nil {
		t.Fatalf("SnapshotDrift() error = %v, want nil", err)
	}
	if len(report.Vetoes) != 0 {
		t.Fatalf("SnapshotDrift().Vetoes = %v, want empty after unrelated settings.json churn", report.Vetoes)
	}

	// Edit to the click-owned hook itself: veto.
	writeTestFile(t, cfg.SettingsPath(), `{"hooks":{"PreToolUse":[{"matcher":"`+MemoryGuardToolMatcher+`","hooks":[{"type":"command","command":"hand-edited command"}]}]},"other":true}`)
	report, err = SnapshotDrift(cfg)
	if err != nil {
		t.Fatalf("SnapshotDrift() error = %v, want nil", err)
	}
	if len(report.Vetoes) != 1 || report.Vetoes[0] != cfg.SettingsPath() {
		t.Fatalf("SnapshotDrift().Vetoes = %v, want exactly [%s] after editing the owned hook", report.Vetoes, cfg.SettingsPath())
	}
}

// TestSnapshotDrift_WholeFileVeto_AnyEdit_Vetoes guards the strict policy: a whole-file-veto entry
// vetoes on ANY content change, with no managed-slice scoping.
func TestSnapshotDrift_WholeFileVeto_AnyEdit_Vetoes(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir(), ClickStateHome: t.TempDir()}
	strictPath := filepath.Join(cfg.ClaudeHome, "strict.txt")
	writeTestFile(t, strictPath, "strict original\n")
	writeManifestDirect(t, cfg, runManifest{Entries: []manifestEntry{{
		OriginalPath: strictPath,
		BackupFile:   "strict.txt",
		Existed:      true,
		DriftPolicy:  DriftPolicyWholeFileVeto,
	}}})
	writeTestFile(t, filepath.Join(snapshotLatestDir(cfg), "strict.txt"), "strict original\n")

	writeTestFile(t, strictPath, "strict hand-edited\n")

	report, err := SnapshotDrift(cfg)
	if err != nil {
		t.Fatalf("SnapshotDrift() error = %v, want nil", err)
	}
	if len(report.Vetoes) != 1 || report.Vetoes[0] != strictPath {
		t.Fatalf("SnapshotDrift().Vetoes = %v, want exactly [%s]", report.Vetoes, strictPath)
	}
}

// TestSnapshotDrift_NonVeto_Edit_WarnsButNeverVetoes guards the non-veto policy: an edited non-veto
// file (matching neither the pre-run backup nor the post-run baseline) never vetoes, but must be
// surfaced in WarnableNonVeto so the rollback command can warn and ask for consent.
func TestSnapshotDrift_NonVeto_Edit_WarnsButNeverVetoes(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir(), ClickStateHome: t.TempDir()}
	sharedPath := filepath.Join(cfg.ClaudeHome, "shared.txt")
	writeTestFile(t, sharedPath, "shared original\n")
	writeManifestDirect(t, cfg, runManifest{Entries: []manifestEntry{{
		OriginalPath: sharedPath,
		BackupFile:   "shared.txt",
		Existed:      true,
		DriftPolicy:  DriftPolicyNonVeto,
	}}})
	writeTestFile(t, filepath.Join(snapshotLatestDir(cfg), "shared.txt"), "shared original\n")

	writeTestFile(t, sharedPath, "shared edited by someone else\n")

	report, err := SnapshotDrift(cfg)
	if err != nil {
		t.Fatalf("SnapshotDrift() error = %v, want nil", err)
	}
	if len(report.Vetoes) != 0 {
		t.Fatalf("SnapshotDrift().Vetoes = %v, want empty (non-veto entries never veto)", report.Vetoes)
	}
	if len(report.WarnableNonVeto) != 1 || report.WarnableNonVeto[0] != sharedPath {
		t.Fatalf("SnapshotDrift().WarnableNonVeto = %v, want exactly [%s]", report.WarnableNonVeto, sharedPath)
	}
}

// TestSnapshotDrift_MatchesEitherBaseline_Safe covers the full match matrix: an entry is safe when
// its current content matches the pre-run backup OR the recorded post-run hash (or both) — drift
// requires missing BOTH baselines.
func TestSnapshotDrift_MatchesEitherBaseline_Safe(t *testing.T) {
	for _, tc := range []struct {
		name    string
		pre     string
		post    string // recorded as ExpectedPostRunHash; "" = no post-run baseline
		current string
	}{
		{name: "pre-only match", pre: "same\n", post: "post-run state\n", current: "same\n"},
		{name: "post-only match", pre: "pre-run state\n", post: "same\n", current: "same\n"},
		{name: "both match", pre: "same\n", post: "same\n", current: "same\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := Config{ClaudeHome: t.TempDir(), ClickStateHome: t.TempDir()}
			path := filepath.Join(cfg.ClaudeHome, "file.txt")
			writeTestFile(t, path, tc.current)
			entry := manifestEntry{
				OriginalPath: path,
				BackupFile:   "file.txt",
				Existed:      true,
				DriftPolicy:  DriftPolicyWholeFileVeto,
			}
			if tc.post != "" {
				entry.ExpectedPostRunHash = CanonicalContentHash(tc.post)
			}
			writeManifestDirect(t, cfg, runManifest{Entries: []manifestEntry{entry}})
			writeTestFile(t, filepath.Join(snapshotLatestDir(cfg), "file.txt"), tc.pre)

			report, err := SnapshotDrift(cfg)
			if err != nil {
				t.Fatalf("SnapshotDrift() error = %v, want nil", err)
			}
			if len(report.Vetoes) != 0 || len(report.WarnableNonVeto) != 0 {
				t.Fatalf("SnapshotDrift() = %+v, want empty (current matches at least one baseline)", report)
			}
		})
	}
}

// TestSnapshotDrift_NoPostRunHash_ComparesAgainstPreRunOnly guards the fallback: an entry whose
// manifest carries no ExpectedPostRunHash compares against the pre-run backup alone, so any edit
// is drift.
func TestSnapshotDrift_NoPostRunHash_ComparesAgainstPreRunOnly(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir(), ClickStateHome: t.TempDir()}
	path := filepath.Join(cfg.ClaudeHome, "file.txt")
	writeTestFile(t, path, "edited\n")
	writeManifestDirect(t, cfg, runManifest{Entries: []manifestEntry{{
		OriginalPath: path,
		BackupFile:   "file.txt",
		Existed:      true,
		DriftPolicy:  DriftPolicyWholeFileVeto,
	}}})
	writeTestFile(t, filepath.Join(snapshotLatestDir(cfg), "file.txt"), "original\n")

	report, err := SnapshotDrift(cfg)
	if err != nil {
		t.Fatalf("SnapshotDrift() error = %v, want nil", err)
	}
	if len(report.Vetoes) != 1 || report.Vetoes[0] != path {
		t.Fatalf("SnapshotDrift().Vetoes = %v, want exactly [%s] (no post-run baseline to match)", report.Vetoes, path)
	}
}

// TestSnapshotDrift_LegacyManifest_StrictWholeFileBehavior guards backward compatibility: a
// v0.5.11-shaped manifest (no driftPolicy/expected hashes at all — normalized to whole-file-veto
// by loadSnapshotManifest) must reproduce the exact pre-hardening strict behavior: any edit to any
// present entry vetoes, and nothing is ever merely warnable.
func TestSnapshotDrift_LegacyManifest_StrictWholeFileBehavior(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir(), ClickStateHome: t.TempDir()}
	path := filepath.Join(cfg.ClaudeHome, "legacy.txt")
	writeTestFile(t, path, "edited after snapshot\n")
	if err := os.MkdirAll(snapshotLatestDir(cfg), 0o755); err != nil {
		t.Fatalf("MkdirAll(latest) error = %v", err)
	}
	legacy := `{"entries":[{"originalPath":` + strconv.Quote(path) + `,"backupFile":"legacy.txt","existed":true}]}`
	if err := os.WriteFile(snapshotManifestPath(cfg), []byte(legacy+"\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(manifest.json) error = %v", err)
	}
	writeTestFile(t, filepath.Join(snapshotLatestDir(cfg), "legacy.txt"), "original\n")

	report, err := SnapshotDrift(cfg)
	if err != nil {
		t.Fatalf("SnapshotDrift() error = %v, want nil", err)
	}
	if len(report.Vetoes) != 1 || report.Vetoes[0] != path {
		t.Fatalf("SnapshotDrift().Vetoes = %v, want exactly [%s] (legacy manifests stay strict)", report.Vetoes, path)
	}
	if len(report.WarnableNonVeto) != 0 {
		t.Fatalf("SnapshotDrift().WarnableNonVeto = %v, want empty for a legacy manifest", report.WarnableNonVeto)
	}
}

// TestSnapshotDrift_ManagedContentVeto_MatchesPostRunManagedBaseline_NoVeto guards the post-run
// managed baseline: after click's own run rewrites the managed block and RecordSnapshotPostRun
// records it, that exact state must not veto even though the managed projection now differs from
// the pre-run backup. A further hand-edit after recording must veto again.
func TestSnapshotDrift_ManagedContentVeto_MatchesPostRunManagedBaseline_NoVeto(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir(), ClickStateHome: t.TempDir()}
	writeTestFile(t, cfg.ClaudeMDPath(), managedBlockMD("managed v1"))
	writeTestFile(t, cfg.SettingsPath(), `{"original":true}`)
	if err := SnapshotRun(cfg); err != nil {
		t.Fatalf("SnapshotRun() error = %v", err)
	}

	// Click's own run rewrites its managed block, then records the post-run baseline.
	writeTestFile(t, cfg.ClaudeMDPath(), managedBlockMD("managed v2 (click's own write)"))
	if err := RecordSnapshotPostRun(cfg); err != nil {
		t.Fatalf("RecordSnapshotPostRun() error = %v", err)
	}

	report, err := SnapshotDrift(cfg)
	if err != nil {
		t.Fatalf("SnapshotDrift() error = %v, want nil", err)
	}
	if len(report.Vetoes) != 0 {
		t.Fatalf("SnapshotDrift().Vetoes = %v, want empty (current matches the recorded post-run managed baseline)", report.Vetoes)
	}

	// A hand-edit to the managed block AFTER the post-run recording matches neither baseline.
	writeTestFile(t, cfg.ClaudeMDPath(), managedBlockMD("hand-edited after post-run recording"))
	report, err = SnapshotDrift(cfg)
	if err != nil {
		t.Fatalf("SnapshotDrift() error = %v, want nil", err)
	}
	if len(report.Vetoes) != 1 || report.Vetoes[0] != cfg.ClaudeMDPath() {
		t.Fatalf("SnapshotDrift().Vetoes = %v, want exactly [%s]", report.Vetoes, cfg.ClaudeMDPath())
	}
}

// --- Per-target snapshot generalization (openclaw-target-support, tasks 2.9-2.12) ---

// TestSnapshotRun_OpenClawPresent_CapturesNineFiles is task 2.9's RED test extended by PR4: when
// cfg.OpenClawHome is populated, SnapshotRun must capture all 9 files (2 Claude + 3 OpenClaw +
// 2 click-memory-guard plugin files + 2 click-owned OpenClaw skill manifests). Count bumped from
// The test's setup deliberately does NOT write the plugin or skill files
// (SyncOpenClawPlugin/SyncOpenClawSkills are never called here), so their entries are expected as
// no-prior-state markers, exactly like a first-ever install would produce.
func TestSnapshotRun_OpenClawPresent_CapturesPortableFiles(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir(), OpenClawHome: t.TempDir(), ClickStateHome: t.TempDir()}
	writeTestFile(t, cfg.ClaudeMDPath(), "# claude\n")
	writeTestFile(t, cfg.SettingsPath(), `{}`)
	writeTestFile(t, cfg.OpenClawAgentsMDPath(), "# agents\n")
	writeTestFile(t, cfg.OpenClawSoulMDPath(), "# soul\n")
	writeTestFile(t, cfg.OpenClawMCPConfigPath(), `{}`)

	if err := SnapshotRun(cfg); err != nil {
		t.Fatalf("SnapshotRun() error = %v", err)
	}

	manifest, err := loadSnapshotManifest(cfg)
	if err != nil {
		t.Fatalf("loadSnapshotManifest() error = %v", err)
	}
	wantEntryCount := 2 + 3 + 1 + len(openClawPluginRelPaths) + len(openClawSkillRelPaths)
	if len(manifest.Entries) != wantEntryCount {
		t.Fatalf("manifest.Entries = %#v, want exactly %d entries", manifest.Entries, wantEntryCount)
	}

	wantPaths := map[string]bool{
		cfg.ClaudeMDPath():             false,
		cfg.SettingsPath():             false,
		cfg.OpenClawAgentsMDPath():     false,
		cfg.OpenClawSoulMDPath():       false,
		cfg.OpenClawMCPConfigPath():    false,
		cfg.OpenClawModelProfilePath(): false,
		filepath.Join(cfg.OpenClawPluginDir(), "plugins", "hooks.js"):   false,
		filepath.Join(cfg.OpenClawPluginDir(), "plugin.json"):           false,
		filepath.Join(cfg.OpenClawSkillsDir(), "clickhola", "SKILL.md"): false,
		filepath.Join(cfg.OpenClawSkillsDir(), "clickdev", "SKILL.md"):  false,
	}
	for _, rel := range openClawSkillRelPaths[2:] {
		wantPaths[filepath.Join(cfg.OpenClawSkillsDir(), filepath.FromSlash(rel))] = false
	}
	for _, e := range manifest.Entries {
		if _, ok := wantPaths[e.OriginalPath]; !ok {
			t.Fatalf("manifest has unexpected entry for %s", e.OriginalPath)
		}
		wantPaths[e.OriginalPath] = true
	}
	for path, found := range wantPaths {
		if !found {
			t.Fatalf("manifest has no entry for %s", path)
		}
	}
}

func TestRestoreRun_OpenClawModelProfile_RestoresPortableArtifact(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir(), OpenClawHome: t.TempDir(), ClickStateHome: t.TempDir()}
	writeTestFile(t, cfg.OpenClawModelProfilePath(), `{"schema_version":2,"profile":"balanced","models":{"explore":"sonnet"}}`)
	if err := SnapshotRun(cfg); err != nil {
		t.Fatalf("SnapshotRun() error = %v", err)
	}
	writeTestFile(t, cfg.OpenClawModelProfilePath(), `{"schema_version":2,"profile":"quality","models":{"explore":"opus"}}`)
	if err := RestoreRun(cfg); err != nil {
		t.Fatalf("RestoreRun() error = %v", err)
	}
	got, err := os.ReadFile(cfg.OpenClawModelProfilePath())
	if err != nil {
		t.Fatalf("ReadFile(model-profile.json) error = %v", err)
	}
	if string(got) != `{"schema_version":2,"profile":"balanced","models":{"explore":"sonnet"}}` {
		t.Fatalf("restored model profile = %q, want original artifact", got)
	}
}

// TestSnapshotRun_OpenClawAbsent_CapturesOnlyClaudeFiles is task 2.10's RED test, made explicit
// (rather than relying only on TestSnapshotRun_CopiesBothFilesAndWritesManifest's pre-existing
// count) so the "unchanged from pre-change behavior" guarantee has its own named regression guard.
func TestSnapshotRun_OpenClawAbsent_CapturesOnlyClaudeFiles(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir(), ClickStateHome: t.TempDir()}
	writeTestFile(t, cfg.ClaudeMDPath(), "# claude\n")
	writeTestFile(t, cfg.SettingsPath(), `{}`)

	if err := SnapshotRun(cfg); err != nil {
		t.Fatalf("SnapshotRun() error = %v", err)
	}

	manifest, err := loadSnapshotManifest(cfg)
	if err != nil {
		t.Fatalf("loadSnapshotManifest() error = %v", err)
	}
	if len(manifest.Entries) != 2 {
		t.Fatalf("manifest.Entries = %#v, want exactly 2 entries when OpenClawHome is empty", manifest.Entries)
	}
}

// TestRestoreRun_OpenClawFilesPresent_RestoresAllFiles is task 2.11's RED test: after SnapshotRun
// with OpenClaw files present, editing ALL 5 files, then RestoreRun, all 5 must come back
// byte-for-byte to their snapshotted content — proving RestoreRun needs no cfg.OpenClawHome-aware
// change of its own, since it replays whatever paths SnapshotRun recorded in the manifest.
func TestRestoreRun_OpenClawFilesPresent_RestoresAllFiles(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir(), OpenClawHome: t.TempDir(), ClickStateHome: t.TempDir()}
	writeTestFile(t, cfg.ClaudeMDPath(), "claude original\n")
	writeTestFile(t, cfg.SettingsPath(), `{"original":true}`)
	writeTestFile(t, cfg.OpenClawAgentsMDPath(), "agents original\n")
	writeTestFile(t, cfg.OpenClawSoulMDPath(), "soul original\n")
	writeTestFile(t, cfg.OpenClawMCPConfigPath(), `{"mcpServers":{}}`)

	if err := SnapshotRun(cfg); err != nil {
		t.Fatalf("SnapshotRun() error = %v", err)
	}

	writeTestFile(t, cfg.ClaudeMDPath(), "claude EDITED\n")
	writeTestFile(t, cfg.SettingsPath(), `{"edited":true}`)
	writeTestFile(t, cfg.OpenClawAgentsMDPath(), "agents EDITED\n")
	writeTestFile(t, cfg.OpenClawSoulMDPath(), "soul EDITED\n")
	writeTestFile(t, cfg.OpenClawMCPConfigPath(), `{"edited":true}`)

	if err := RestoreRun(cfg); err != nil {
		t.Fatalf("RestoreRun() error = %v", err)
	}

	checks := map[string]string{
		cfg.ClaudeMDPath():          "claude original\n",
		cfg.SettingsPath():          `{"original":true}`,
		cfg.OpenClawAgentsMDPath():  "agents original\n",
		cfg.OpenClawSoulMDPath():    "soul original\n",
		cfg.OpenClawMCPConfigPath(): `{"mcpServers":{}}`,
	}
	for path, want := range checks {
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile(%s) error = %v", path, err)
		}
		if string(got) != want {
			t.Fatalf("restored %s = %q, want %q", path, got, want)
		}
	}
}

// TestRestoreRun_OpenClawSkillsPresent_RestoresBothFiles is PR4's RED test: after SnapshotRun with
// the click-owned OpenClaw skill files present, editing both SKILL.md files, then RestoreRun, both
// must come back byte-for-byte to their snapshotted content — proving RestoreRun needs no
// cfg.OpenClawHome-aware change of its own, since it replays whatever paths SnapshotRun recorded.
func TestRestoreRun_OpenClawSkillsPresent_RestoresBothFiles(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir(), OpenClawHome: t.TempDir(), ClickStateHome: t.TempDir()}
	writeTestFile(t, cfg.ClaudeMDPath(), "claude original\n")
	writeTestFile(t, cfg.SettingsPath(), `{"original":true}`)
	writeTestFile(t, cfg.OpenClawAgentsMDPath(), "agents original\n")
	writeTestFile(t, cfg.OpenClawSoulMDPath(), "soul original\n")
	writeTestFile(t, cfg.OpenClawMCPConfigPath(), `{"mcpServers":{}}`)
	for _, rel := range openClawSkillRelPaths {
		writeTestFile(t, filepath.Join(cfg.OpenClawSkillsDir(), filepath.FromSlash(rel)), "original "+rel+"\n")
	}

	if err := SnapshotRun(cfg); err != nil {
		t.Fatalf("SnapshotRun() error = %v", err)
	}

	for _, rel := range openClawSkillRelPaths {
		writeTestFile(t, filepath.Join(cfg.OpenClawSkillsDir(), filepath.FromSlash(rel)), "edited "+rel+"\n")
	}
	if err := RemoveOpenClawSkills(cfg); err != nil {
		t.Fatalf("RemoveOpenClawSkills() error = %v", err)
	}

	if err := RestoreRun(cfg); err != nil {
		t.Fatalf("RestoreRun() error = %v", err)
	}

	for _, rel := range openClawSkillRelPaths {
		path := filepath.Join(cfg.OpenClawSkillsDir(), filepath.FromSlash(rel))
		want := "original " + rel + "\n"
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile(%s) error = %v", path, err)
		}
		if string(got) != want {
			t.Fatalf("restored %s = %q, want %q", path, got, want)
		}
	}
}

// TestSnapshotRun_OpenClawFirstInstall_RecordsNoPriorStateMarker is task 2.12's RED test: OpenClaw
// is detected (cfg.OpenClawHome set) but none of its 3 files exist yet — the snapshot must record
// an explicit no-prior-state marker for each of them, exactly like Claude's own first-ever-install
// case, never an error and never a fabricated backup file.
func TestSnapshotRun_OpenClawFirstInstall_RecordsNoPriorStateMarker(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir(), OpenClawHome: t.TempDir(), ClickStateHome: t.TempDir()}
	writeTestFile(t, cfg.ClaudeMDPath(), "# claude\n")
	writeTestFile(t, cfg.SettingsPath(), `{}`)
	// Deliberately do NOT create AGENTS.md/SOUL.md/openclaw.json — first-ever OpenClaw install.

	if err := SnapshotRun(cfg); err != nil {
		t.Fatalf("SnapshotRun() error = %v, want nil even when OpenClaw's files don't exist yet", err)
	}

	manifest, err := loadSnapshotManifest(cfg)
	if err != nil {
		t.Fatalf("loadSnapshotManifest() error = %v", err)
	}
	byOriginal := make(map[string]manifestEntry, len(manifest.Entries))
	for _, e := range manifest.Entries {
		byOriginal[e.OriginalPath] = e
	}
	for _, path := range []string{cfg.OpenClawAgentsMDPath(), cfg.OpenClawSoulMDPath(), cfg.OpenClawMCPConfigPath()} {
		entry, ok := byOriginal[path]
		if !ok {
			t.Fatalf("manifest has no entry for %s", path)
		}
		if entry.Existed {
			t.Fatalf("manifest entry for %s: Existed = true, want false (no-prior-state marker)", path)
		}
		if entry.BackupFile != "" {
			t.Fatalf("manifest entry for %s: BackupFile = %q, want empty for a no-prior-state marker", path, entry.BackupFile)
		}
	}
}

// --- Task 1: Schema round-trip tests (RED) ---

// TestManifestEntry_RoundTripNewFields tests that the three new optional fields round-trip
// through JSON marshal/unmarshal correctly when populated.
func TestManifestEntry_RoundTripNewFields(t *testing.T) {
	entry := manifestEntry{
		OriginalPath:               "/some/path",
		BackupFile:                 "backup.txt",
		Existed:                    true,
		ExpectedPostRunHash:        "abc123",
		DriftPolicy:                "whole-file-veto",
		ExpectedPostRunManagedHash: "managed456",
	}

	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("json.Marshal(entry) error = %v, want nil", err)
	}

	var decoded manifestEntry
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal(data) error = %v, want nil", err)
	}

	if decoded.ExpectedPostRunHash != entry.ExpectedPostRunHash {
		t.Fatalf("decoded.ExpectedPostRunHash = %q, want %q", decoded.ExpectedPostRunHash, entry.ExpectedPostRunHash)
	}
	if decoded.DriftPolicy != entry.DriftPolicy {
		t.Fatalf("decoded.DriftPolicy = %q, want %q", decoded.DriftPolicy, entry.DriftPolicy)
	}
	if decoded.ExpectedPostRunManagedHash != entry.ExpectedPostRunManagedHash {
		t.Fatalf("decoded.ExpectedPostRunManagedHash = %q, want %q", decoded.ExpectedPostRunManagedHash, entry.ExpectedPostRunManagedHash)
	}
}

// TestManifestEntry_AbsentFieldsOmitted tests that the new optional fields are omitted from JSON
// when empty (omitempty behavior).
func TestManifestEntry_AbsentFieldsOmitted(t *testing.T) {
	entry := manifestEntry{
		OriginalPath: "/some/path",
		BackupFile:   "backup.txt",
		Existed:      true,
		// New fields left at zero values
	}

	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("json.Marshal(entry) error = %v, want nil", err)
	}

	// Check that the new field keys are not present in the JSON
	dataStr := string(data)
	if contains(dataStr, "expectedPostRunHash") {
		t.Fatalf("JSON contains 'expectedPostRunHash' field when empty, want it omitted: %s", dataStr)
	}
	if contains(dataStr, "driftPolicy") {
		t.Fatalf("JSON contains 'driftPolicy' field when empty, want it omitted: %s", dataStr)
	}
	if contains(dataStr, "expectedPostRunManagedHash") {
		t.Fatalf("JSON contains 'expectedPostRunManagedHash' field when empty, want it omitted: %s", dataStr)
	}

	// Round-trip should preserve zero values
	var decoded manifestEntry
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal(data) error = %v, want nil", err)
	}
	if decoded.ExpectedPostRunHash != "" {
		t.Fatalf("decoded.ExpectedPostRunHash = %q, want empty string", decoded.ExpectedPostRunHash)
	}
	if decoded.DriftPolicy != "" {
		t.Fatalf("decoded.DriftPolicy = %q, want empty string", decoded.DriftPolicy)
	}
	if decoded.ExpectedPostRunManagedHash != "" {
		t.Fatalf("decoded.ExpectedPostRunManagedHash = %q, want empty string", decoded.ExpectedPostRunManagedHash)
	}
}

// TestLoadSnapshotManifest_UnknownPolicyRejected tests that an unknown policy string is rejected
// as a load error.
func TestLoadSnapshotManifest_UnknownPolicyRejected(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir(), ClickStateHome: t.TempDir()}
	manifestJSON := `{
  "entries": [
    {
      "originalPath": "/some/path",
      "backupFile": "backup.txt",
      "existed": true,
      "driftPolicy": "unknown-evil-policy"
    }
  ]
}`
	manifestPath := filepath.Join(cfg.BackupDir(), "latest", "manifest.json")
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(manifestPath, []byte(manifestJSON), 0o644); err != nil {
		t.Fatalf("WriteFile(manifest.json) error = %v", err)
	}

	_, err := loadSnapshotManifest(cfg)
	if err == nil {
		t.Fatal("loadSnapshotManifest() error = nil, want error for unknown policy")
	}
	if !contains(err.Error(), "unknown policy") {
		t.Fatalf("loadSnapshotManifest() error = %v, want error mentioning 'unknown policy'", err)
	}
}

// TestLoadSnapshotManifest_ConflictingDuplicatePathRejected tests that duplicate OriginalPath
// entries with conflicting policies are rejected as a load error.
func TestLoadSnapshotManifest_ConflictingDuplicatePathRejected(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir(), ClickStateHome: t.TempDir()}
	manifestJSON := `{
  "entries": [
    {
      "originalPath": "/some/path",
      "backupFile": "backup1.txt",
      "existed": true,
      "driftPolicy": "whole-file-veto"
    },
    {
      "originalPath": "/some/path",
      "backupFile": "backup2.txt",
      "existed": true,
      "driftPolicy": "managed-content-veto"
    }
  ]
}`
	manifestPath := filepath.Join(cfg.BackupDir(), "latest", "manifest.json")
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(manifestPath, []byte(manifestJSON), 0o644); err != nil {
		t.Fatalf("WriteFile(manifest.json) error = %v", err)
	}

	_, err := loadSnapshotManifest(cfg)
	if err == nil {
		t.Fatal("loadSnapshotManifest() error = nil, want error for conflicting duplicate paths")
	}
	if !contains(err.Error(), "conflicting policies") {
		t.Fatalf("loadSnapshotManifest() error = %v, want error mentioning 'conflicting policies'", err)
	}
}

// TestLoadSnapshotManifest_DuplicatePathWithAgreeingPolicyAccepted tests that duplicate
// OriginalPath entries with the SAME policy are accepted.
func TestLoadSnapshotManifest_DuplicatePathWithAgreeingPolicyAccepted(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir(), ClickStateHome: t.TempDir()}
	manifestJSON := `{
  "entries": [
    {
      "originalPath": "/some/path",
      "backupFile": "backup1.txt",
      "existed": true,
      "driftPolicy": "whole-file-veto"
    },
    {
      "originalPath": "/some/path",
      "backupFile": "backup2.txt",
      "existed": true,
      "driftPolicy": "whole-file-veto"
    }
  ]
}`
	manifestPath := filepath.Join(cfg.BackupDir(), "latest", "manifest.json")
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(manifestPath, []byte(manifestJSON), 0o644); err != nil {
		t.Fatalf("WriteFile(manifest.json) error = %v", err)
	}

	manifest, err := loadSnapshotManifest(cfg)
	if err != nil {
		t.Fatalf("loadSnapshotManifest() error = %v, want nil for agreeing duplicate paths", err)
	}
	if len(manifest.Entries) != 2 {
		t.Fatalf("manifest.Entries = %d, want 2 (both entries preserved)", len(manifest.Entries))
	}
}

// TestLoadSnapshotManifest_LegacyManifest_NormalizesToWholeFileVeto tests that a manifest
// without a DriftPolicy field (legacy v0.5.11 format) normalizes to DriftPolicyWholeFileVeto.
func TestLoadSnapshotManifest_LegacyManifest_NormalizesToWholeFileVeto(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir(), ClickStateHome: t.TempDir()}
	manifestJSON := `{
  "entries": [
    {
      "originalPath": "/some/path",
      "backupFile": "backup.txt",
      "existed": true
    }
  ]
}`
	manifestPath := filepath.Join(cfg.BackupDir(), "latest", "manifest.json")
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(manifestPath, []byte(manifestJSON), 0o644); err != nil {
		t.Fatalf("WriteFile(manifest.json) error = %v", err)
	}

	manifest, err := loadSnapshotManifest(cfg)
	if err != nil {
		t.Fatalf("loadSnapshotManifest() error = %v, want nil", err)
	}
	if len(manifest.Entries) != 1 {
		t.Fatalf("manifest.Entries = %d, want 1", len(manifest.Entries))
	}
	if manifest.Entries[0].DriftPolicy != DriftPolicyWholeFileVeto {
		t.Fatalf("manifest.Entries[0].DriftPolicy = %q, want %q (normalized)", manifest.Entries[0].DriftPolicy, DriftPolicyWholeFileVeto)
	}
}

// contains is a small helper to avoid importing strings package in this file.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && findSubstring(s, substr))
}

// findSubstring implements a simple substring search to avoid importing strings package.
func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// --- Task 5: Plan-matrix test (Layer B - RED) ---

// TestBuildTargetPlan_AllSnapshotDeclarationsHaveKnownPolicy tests that every
// SnapshotDecl across all plan variants has a known, non-empty policy, and that
// any path appearing multiple times has the same policy every time.
func TestBuildTargetPlan_AllSnapshotDeclarationsHaveKnownPolicy(t *testing.T) {
	knownPolicies := map[DriftPolicy]bool{
		DriftPolicyWholeFileVeto:      true,
		DriftPolicyManagedContentVeto: true,
		DriftPolicyNonVeto:            true,
	}

	testCases := []struct {
		name        string
		selection   TargetSelection
		options     PlanOptions
		description string
	}{
		{
			name:        "Claude-only",
			selection:   TargetSelection{Claude: true},
			options:     PlanOptions{},
			description: "Claude target only, no cloud config",
		},
		{
			name:        "Claude-with-cloud",
			selection:   TargetSelection{Claude: true},
			options:     PlanOptions{CloudResolvable: true},
			description: "Claude target with cloud config",
		},
		{
			name:        "Claude-with-native-model",
			selection:   TargetSelection{Claude: true},
			options:     PlanOptions{CodexNativeModel: true},
			description: "Claude target with native model option (no effect on Claude)",
		},
		{
			name:        "Codex-only",
			selection:   TargetSelection{Codex: true},
			options:     PlanOptions{},
			description: "Codex target only, no native model",
		},
		{
			name:        "Codex-with-native-model",
			selection:   TargetSelection{Codex: true},
			options:     PlanOptions{CodexNativeModel: true},
			description: "Codex target with native model mutation",
		},
		{
			name:        "OpenClaw-only",
			selection:   TargetSelection{OpenClaw: true},
			options:     PlanOptions{},
			description: "OpenClaw target only, no native model",
		},
		{
			name:        "OpenClaw-with-native-model",
			selection:   TargetSelection{OpenClaw: true},
			options:     PlanOptions{OpenClawNativeModel: true},
			description: "OpenClaw target with native model mutation",
		},
		{
			name:        "Claude-and-Codex",
			selection:   TargetSelection{Claude: true, Codex: true},
			options:     PlanOptions{},
			description: "Both Claude and Codex targets",
		},
		{
			name:        "Claude-and-OpenClaw",
			selection:   TargetSelection{Claude: true, OpenClaw: true},
			options:     PlanOptions{},
			description: "Both Claude and OpenClaw targets",
		},
		{
			name:        "All-targets",
			selection:   TargetSelection{Claude: true, Codex: true, OpenClaw: true},
			options:     PlanOptions{CloudResolvable: true},
			description: "All targets with cloud config",
		},
		{
			name:        "All-targets-all-native-models",
			selection:   TargetSelection{Claude: true, Codex: true, OpenClaw: true},
			options:     PlanOptions{CloudResolvable: true, CodexNativeModel: true, OpenClawNativeModel: true},
			description: "All targets with cloud and all native models",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := Config{
				ClaudeHome:   t.TempDir(),
				CodexHome:    t.TempDir(),
				OpenClawHome: t.TempDir(),
			}

			// Create minimal required config paths so Plan builds successfully
			writeTestFile(t, cfg.ModelsPath(), `{"schema_version":2}`)
			writeTestFile(t, cfg.KnownMarketplacesPath(), `[]`)
			writeTestFile(t, cfg.InstalledPluginsPath(), `[]`)

			plan := BuildTargetPlan(cfg, tc.selection, tc.options)

			// Track policies for duplicate detection
			pathPolicy := make(map[string]DriftPolicy)

			for i, step := range plan.Steps {
				for j, decl := range step.Snapshot {
					// Rule 1: every path must be non-empty
					if decl.Path == "" {
						t.Errorf("%s: step %d (%s), decl %d has empty Path", tc.description, i, step.ID, j)
						continue
					}

					// Rule 2: every policy must be known and non-empty
					if decl.Policy == "" {
						t.Errorf("%s: step %d (%s), decl %d (path %s) has empty Policy", tc.description, i, step.ID, j, decl.Path)
						continue
					}
					if !knownPolicies[decl.Policy] {
						t.Errorf("%s: step %d (%s), decl %d (path %s) has unknown Policy %q", tc.description, i, step.ID, j, decl.Path, decl.Policy)
						continue
					}

					// Rule 3: duplicates must have the same policy
					if priorPolicy, exists := pathPolicy[decl.Path]; exists {
						if priorPolicy != decl.Policy {
							t.Errorf("%s: path %s appears with conflicting policies %q and %q", tc.description, decl.Path, priorPolicy, decl.Policy)
						}
					} else {
						pathPolicy[decl.Path] = decl.Policy
					}
				}
			}

			// Also validate the SnapshotSpecs() method
			specs := plan.SnapshotSpecs()
			for i, spec := range specs {
				if spec.Path == "" {
					t.Errorf("%s: SnapshotSpecs() returned decl %d with empty Path", tc.description, i)
				}
				if spec.Policy == "" {
					t.Errorf("%s: SnapshotSpecs() returned decl %d (path %s) with empty Policy", tc.description, i, spec.Path)
				}
				if !knownPolicies[spec.Policy] {
					t.Errorf("%s: SnapshotSpecs() returned decl %d (path %s) with unknown Policy %q", tc.description, i, spec.Path, spec.Policy)
				}
			}
		})
	}
}

// --- Task 3: Typed snapshot declarations (RED) ---

// TestSnapshotDecl_TypeAndConstructor tests the SnapshotDecl type exists and has
// the required Path and Policy fields, and that the snapshot() constructor function
// exists and creates correct SnapshotDecl values.
func TestSnapshotDecl_TypeAndConstructor(t *testing.T) {
	// This test will compile only once SnapshotDecl and snapshot() exist
	_ = SnapshotDecl{}
	_ = snapshot

	decl := snapshot("/some/path", DriftPolicyWholeFileVeto)
	if decl.Path != "/some/path" {
		t.Fatalf("snapshot().Path = %q, want %q", decl.Path, "/some/path")
	}
	if decl.Policy != DriftPolicyWholeFileVeto {
		t.Fatalf("snapshot().Policy = %q, want %q", decl.Policy, DriftPolicyWholeFileVeto)
	}
}

// TestSnapshotDecl_RequiredArguments tests that the snapshot() constructor requires
// both path and policy arguments (this will be enforced by the compiler once we
// make the migration).
func TestSnapshotDecl_RequiredArguments(t *testing.T) {
	// This will compile once the types exist - the requirement that both args
	// are mandatory is enforced by Go's function signature
	_ = snapshot

	decl := snapshot("/path", DriftPolicyNonVeto)
	if decl.Path != "/path" {
		t.Fatalf("snapshot().Path = %q, want %q", decl.Path, "/path")
	}
	if decl.Policy != DriftPolicyNonVeto {
		t.Fatalf("snapshot().Policy = %q, want %q", decl.Policy, DriftPolicyNonVeto)
	}
}

func TestRedactEngramCloudToken_MalformedWithTokenNeverLeaksSecret(t *testing.T) {
	// Malformed JSON containing the literal token - unterminated string
	// This should fail closed (return error) rather than leaking the secret into the backup
	malformedWithToken := []byte(`{"env":{"ENGRAM_CLOUD_TOKEN":"secret-token-12345"`)

	redacted, err := redactEngramCloudToken(malformedWithToken)
	if err == nil {
		t.Fatalf("redactEngramCloudToken(malformed JSON with token) should return error, got nil")
	}
	if redacted != nil {
		t.Fatalf("redactEngramCloudToken(malformed JSON with token) should return nil output on error, got %v bytes", len(redacted))
	}

	// Ensure the secret token does not appear in any error messages
	if err != nil && strings.Contains(err.Error(), "secret-token-12345") {
		t.Fatal("Error message contains secret token")
	}
}

func TestRedactEngramCloudToken_ValidTokenDocumentPreservesUnrelatedBytes(t *testing.T) {
	// Valid JSON with unusual but legal formatting (compact, non-alphabetical key order, no trailing newline)
	compact := []byte(`{"z":1,"env":{"ENGRAM_CLOUD_TOKEN":"my-secret","ENGRAM_CLOUD_SERVER":"https://example.com"}}`)

	redacted, err := redactEngramCloudToken(compact)
	if err != nil {
		t.Fatalf("redactEngramCloudToken(valid compact JSON) error = %v", err)
	}

	// Token value should be gone
	if bytes.Contains(redacted, []byte("my-secret")) {
		t.Fatalf("Token value should not appear in redacted output")
	}

	// Every other byte should be preserved - check specific markers
	if !bytes.Contains(redacted, []byte(`"z":1`)) {
		t.Fatalf("Unrelated key 'z' should be preserved")
	}
	if !bytes.Contains(redacted, []byte(`"ENGRAM_CLOUD_SERVER":"https://example.com"`)) {
		t.Fatalf("Server key should be preserved")
	}

	// Should still be compact (no extra whitespace added)
	if bytes.Contains(redacted, []byte("  ")) {
		t.Fatalf("Should preserve compact formatting without adding spaces")
	}

	// Should not add trailing newline
	if len(redacted) > 0 && redacted[len(redacted)-1] == '\n' {
		t.Fatalf("Should not add trailing newline to compact input")
	}
}

func TestRedactEngramCloudToken_IgnoresNestedForeignEnvObjects(t *testing.T) {
	input := []byte(`{"plugin":{"env":{"ENGRAM_CLOUD_TOKEN":"foreign-value"}}}`)
	redacted, err := redactEngramCloudToken(input)
	if err != nil {
		t.Fatalf("redactEngramCloudToken() error = %v", err)
	}
	if !bytes.Equal(redacted, input) {
		t.Fatal("nested foreign env object was altered")
	}
}

func TestRedactEngramCloudToken_NoTokenReturnsInputUnchanged(t *testing.T) {
	// Input without token should return exactly the same bytes
	input := append([]byte(`{"env":{"ENGRAM_CLOUD_SERVER":"https://example.com"}}`), '\n')

	redacted, err := redactEngramCloudToken(input)
	if err != nil {
		t.Fatalf("redactEngramCloudToken(no token) error = %v", err)
	}

	if !bytes.Equal(redacted, input) {
		t.Fatalf("No-token case should return input unchanged")
	}
}

func TestRedactEngramCloudToken_EscapedKeyStillRedacted(t *testing.T) {
	secret := "escaped-key-secret"
	input := []byte(`{"env":{"ENGRAM_CLOUD\u005fTOKEN":"` + secret + `"}}`)

	redacted, err := redactEngramCloudToken(input)
	if err != nil {
		t.Fatalf("redactEngramCloudToken() error = %v", err)
	}
	if bytes.Contains(redacted, []byte(secret)) {
		t.Fatal("redacted backup contains the token")
	}
}

func TestRedactEngramCloudToken_EscapedValueStillRedacted(t *testing.T) {
	secretFragment := "escaped-value-secret"
	input := []byte(`{"env":{"ENGRAM_CLOUD_TOKEN":"escaped\\u002dvalue\\u002dsecret"}}`)

	redacted, err := redactEngramCloudToken(input)
	if err != nil {
		t.Fatalf("redactEngramCloudToken() error = %v", err)
	}
	if bytes.Contains(redacted, []byte(secretFragment)) || bytes.Contains(redacted, []byte(`\\u002d`)) {
		t.Fatal("redacted backup contains token value bytes")
	}
}
