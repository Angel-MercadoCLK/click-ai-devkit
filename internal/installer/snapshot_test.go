package installer

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
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
	cfg := Config{ClaudeHome: t.TempDir()}
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
	cfg := Config{ClaudeHome: t.TempDir()}

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
	cfg := Config{ClaudeHome: t.TempDir()}
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
	cfg := Config{ClaudeHome: t.TempDir()}
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
	cfg := Config{ClaudeHome: t.TempDir()}
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
	cfg := Config{ClaudeHome: t.TempDir()}

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
	cfg := Config{ClaudeHome: t.TempDir()}
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
	cfg := Config{ClaudeHome: t.TempDir()}
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
	cfg := Config{ClaudeHome: t.TempDir()}
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

	gotLF := canonicalContentHash(lf)
	gotCRLF := canonicalContentHash(crlf)
	if gotLF != gotCRLF {
		t.Fatalf("canonicalContentHash(LF) = %q, canonicalContentHash(CRLF) = %q, want equal for the same logical content", gotLF, gotCRLF)
	}
}

// TestCanonicalContentHash_DifferentContentDiffers triangulates against the trivial
// "always returns the same hash" implementation: genuinely different content must hash
// differently.
func TestCanonicalContentHash_DifferentContentDiffers(t *testing.T) {
	got1 := canonicalContentHash("content A\n")
	got2 := canonicalContentHash("content B\n")
	if got1 == got2 {
		t.Fatalf("canonicalContentHash(%q) == canonicalContentHash(%q) == %q, want different hashes for different content", "content A\n", "content B\n", got1)
	}
}
