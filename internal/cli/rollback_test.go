package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/installer"
)

// writeRollbackTestFile is rollback_test.go's own small file-seeding helper (mirrors the
// installer package's own writeTestFile, duplicated here rather than exported across packages for
// one small helper).
func writeRollbackTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", path, err)
	}
}

// rollbackManagedBlockMD builds a CLAUDE.md-shaped file whose click-managed block contains
// managed, with editable developer content around the markers. The marker literals mirror
// installer's unexported managedBeginMarker/managedEndMarker (claudemd.go).
func rollbackManagedBlockMD(managed string) string {
	return "# developer header\n" +
		"# >>> click-ai-devkit (managed) >>>\n" + managed + "\n" +
		"# <<< click-ai-devkit (managed) <<<\n" +
		"developer tail\n"
}

// rollbackManifestEntry mirrors installer's unexported manifestEntry JSON shape so tests can add
// hand-crafted entries (e.g. a non-veto shared file) to a real snapshot's manifest.
type rollbackManifestEntry struct {
	OriginalPath        string `json:"originalPath"`
	BackupFile          string `json:"backupFile"`
	Existed             bool   `json:"existed"`
	ExpectedPostRunHash string `json:"expectedPostRunHash,omitempty"`
	DriftPolicy         string `json:"driftPolicy,omitempty"`
}

// appendRollbackManifestEntry adds one entry to the real manifest.json of cfg's latest snapshot
// and writes its backup file — the honest way to give rollback's ClaudeHome-only cfg a non-veto
// entry (its real snapshotSources only ever produces managed-content-veto entries for Claude).
func appendRollbackManifestEntry(t *testing.T, cfg installer.Config, entry rollbackManifestEntry, backupContent string) {
	t.Helper()
	latestDir := filepath.Join(cfg.BackupDir(), "latest")
	manifestPath := filepath.Join(latestDir, "manifest.json")
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("ReadFile(manifest.json) error = %v", err)
	}
	var doc struct {
		Entries []rollbackManifestEntry `json:"entries"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("json.Unmarshal(manifest.json) error = %v", err)
	}
	doc.Entries = append(doc.Entries, entry)
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatalf("json.MarshalIndent(manifest) error = %v", err)
	}
	if err := os.WriteFile(manifestPath, append(data, '\n'), 0o600); err != nil {
		t.Fatalf("WriteFile(manifest.json) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(latestDir, entry.BackupFile), []byte(backupContent), 0o600); err != nil {
		t.Fatalf("WriteFile(backup %s) error = %v", entry.BackupFile, err)
	}
}

// execRollbackWithStdin runs the real `rollback` command through the real root cobra tree with an
// explicit stdin — the exact shape the standing menu's dispatch produces ([]string{"rollback"}
// with the caller's streams), unlike execRoot which always pins stdin to an empty buffer.
// stateHome must be the same one the test seeded its snapshot under: BackupDir() is rooted at
// ClickStateHome, so a throwaway one here would hide the snapshot from the command.
func execRollbackWithStdin(t *testing.T, claudeHome, stateHome, stdin string, args ...string) (string, error) {
	t.Helper()
	t.Setenv("CLICK_CLAUDE_HOME", claudeHome)
	t.Setenv("CLICK_STATE_HOME", stateHome)
	t.Setenv("CLICK_OPENCLAW_HOME", t.TempDir())

	root := NewRootCommand()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetIn(strings.NewReader(stdin))
	root.SetArgs(args)
	err := root.Execute()
	return buf.String(), err
}

// seedWarnableSharedFile gives cfg's real snapshot one edited non-veto "shared" entry: the
// scenario rollback's new warning/consent flow exists for.
func seedWarnableSharedFile(t *testing.T, cfg installer.Config) string {
	t.Helper()
	sharedPath := filepath.Join(cfg.ClaudeHome, "shared.txt")
	writeRollbackTestFile(t, cfg.ClaudeMDPath(), rollbackManagedBlockMD("managed v1"))
	writeRollbackTestFile(t, cfg.SettingsPath(), `{"original":true}`)
	writeRollbackTestFile(t, sharedPath, "shared original\n")
	if err := installer.SnapshotRun(cfg); err != nil {
		t.Fatalf("installer.SnapshotRun() error = %v", err)
	}
	appendRollbackManifestEntry(t, cfg, rollbackManifestEntry{
		OriginalPath: sharedPath,
		BackupFile:   "shared.txt",
		Existed:      true,
		DriftPolicy:  "non-veto",
	}, "shared original\n")
	writeRollbackTestFile(t, sharedPath, "shared edited by Claude Code after the run\n")
	return sharedPath
}

// TestRollback_NoSnapshotEver_ReportsNothingToRestoreNoError guards the spec install-rollback "No
// snapshot exists" scenario when SnapshotRun has never even run: rollback must not error, must not
// fabricate content, and must report cleanly.
func TestRollback_NoSnapshotEver_ReportsNothingToRestoreNoError(t *testing.T) {
	home := t.TempDir()

	// No cfg and no seeded snapshot here on purpose: this is the "SnapshotRun never ran" case, so
	// execRoot's own throwaway state home is exactly right.
	out, err := execRoot(t, home, "rollback")
	if err != nil {
		t.Fatalf("rollback error = %v, want nil when no snapshot ever ran, output:\n%s", err, out)
	}
	if !strings.Contains(out, "No hay ningún respaldo") {
		t.Fatalf("rollback output = %q, want the nothing-to-restore informational message", out)
	}
}

// TestRollback_SnapshotAllNoPriorState_ReportsNothingToRestoreNoError guards the finer half of the
// same spec scenario: a real snapshot ran, but both files were absent at snapshot time (pure
// no-prior-state markers) -> still nothing to restore.
func TestRollback_SnapshotAllNoPriorState_ReportsNothingToRestoreNoError(t *testing.T) {
	home := t.TempDir()
	stateHome := t.TempDir()
	cfg := installer.Config{ClaudeHome: home, ClickStateHome: stateHome}
	if err := installer.SnapshotRun(cfg); err != nil {
		t.Fatalf("installer.SnapshotRun() error = %v", err)
	}

	out, err := execRootWithStateHome(t, home, stateHome, "rollback")
	if err != nil {
		t.Fatalf("rollback error = %v, want nil, output:\n%s", err, out)
	}
	if !strings.Contains(out, "No hay ningún respaldo") {
		t.Fatalf("rollback output = %q, want the nothing-to-restore informational message", out)
	}
}

// TestRollback_MatchingHash_NoForce_RestoresBothFilesAndSnapshotSurvives guards the spec
// "Successful restore" scenario: no drift since the snapshot means rollback proceeds even without
// --force, restores both files, and the snapshot itself survives (not consumed). With nothing
// vetoing and nothing warnable, no consent is needed even on a non-TTY.
func TestRollback_MatchingHash_NoForce_RestoresBothFilesAndSnapshotSurvives(t *testing.T) {
	home := t.TempDir()
	stateHome := t.TempDir()
	cfg := installer.Config{ClaudeHome: home, ClickStateHome: stateHome}
	writeRollbackTestFile(t, cfg.ClaudeMDPath(), "snapshot content\n")
	writeRollbackTestFile(t, cfg.SettingsPath(), `{"snapshot":true}`)
	if err := installer.SnapshotRun(cfg); err != nil {
		t.Fatalf("installer.SnapshotRun() error = %v", err)
	}

	out, err := execRootWithStateHome(t, home, stateHome, "rollback")
	if err != nil {
		t.Fatalf("rollback error = %v, want nil, output:\n%s", err, out)
	}
	if !strings.Contains(out, "Archivos gestionados por click restaurados desde el último respaldo.") {
		t.Fatalf("rollback output = %q, want the managed-files restore success message", out)
	}

	gotClaude, err := os.ReadFile(cfg.ClaudeMDPath())
	if err != nil {
		t.Fatalf("read CLAUDE.md after rollback: %v", err)
	}
	if string(gotClaude) != "snapshot content\n" {
		t.Fatalf("CLAUDE.md after rollback = %q, want %q", gotClaude, "snapshot content\n")
	}

	has, err := installer.HasRunSnapshot(cfg)
	if err != nil {
		t.Fatalf("installer.HasRunSnapshot() error = %v", err)
	}
	if !has {
		t.Fatal("HasRunSnapshot() = false after rollback, want the snapshot to survive (not consumed)")
	}
}

// TestRollback_DriftNoForce_RefusesAndMakesZeroWrites guards spec install-rollback Decision 3
// (refuse-by-default), ownership-scoped: a hand-edit INSIDE click's managed block since the
// snapshot must produce an error, report the drifted path, and leave BOTH files exactly as they
// currently are (zero writes).
func TestRollback_DriftNoForce_RefusesAndMakesZeroWrites(t *testing.T) {
	home := t.TempDir()
	stateHome := t.TempDir()
	cfg := installer.Config{ClaudeHome: home, ClickStateHome: stateHome}
	writeRollbackTestFile(t, cfg.ClaudeMDPath(), rollbackManagedBlockMD("managed v1"))
	writeRollbackTestFile(t, cfg.SettingsPath(), `{"original":true}`)
	if err := installer.SnapshotRun(cfg); err != nil {
		t.Fatalf("installer.SnapshotRun() error = %v", err)
	}
	writeRollbackTestFile(t, cfg.ClaudeMDPath(), rollbackManagedBlockMD("hand-edited managed block"))

	out, err := execRootWithStateHome(t, home, stateHome, "rollback")
	if err == nil {
		t.Fatalf("rollback error = nil, want a refusal error when click-owned content drifted, output:\n%s", out)
	}
	if !strings.Contains(out, "Los siguientes archivos se editaron manualmente desde el respaldo; rollback rechazado:") {
		t.Fatalf("rollback output = %q, want the existing veto-refusal message shape", out)
	}
	if !strings.Contains(out, cfg.ClaudeMDPath()) {
		t.Fatalf("rollback output = %q, want it to name the drifted file %s", out, cfg.ClaudeMDPath())
	}
	if !strings.Contains(out, "click rollback --force") {
		t.Fatalf("rollback output = %q, want the --force guidance", out)
	}

	gotClaude, err := os.ReadFile(cfg.ClaudeMDPath())
	if err != nil {
		t.Fatalf("read CLAUDE.md after refused rollback: %v", err)
	}
	if string(gotClaude) != rollbackManagedBlockMD("hand-edited managed block") {
		t.Fatalf("CLAUDE.md after refused rollback = %q, want untouched hand-edited content", gotClaude)
	}
	gotSettings, err := os.ReadFile(cfg.SettingsPath())
	if err != nil {
		t.Fatalf("read settings.json after refused rollback: %v", err)
	}
	if string(gotSettings) != `{"original":true}` {
		t.Fatalf("settings.json after refused rollback = %q, want untouched %q", gotSettings, `{"original":true}`)
	}
}

// TestRollback_DriftWithForce_Proceeds guards the explicit override: the same managed-block drift
// as above, but with --force, must proceed and restore the snapshotted content (no warnable
// entries pending, so no consent is required).
func TestRollback_DriftWithForce_Proceeds(t *testing.T) {
	home := t.TempDir()
	stateHome := t.TempDir()
	cfg := installer.Config{ClaudeHome: home, ClickStateHome: stateHome}
	writeRollbackTestFile(t, cfg.ClaudeMDPath(), rollbackManagedBlockMD("managed v1"))
	writeRollbackTestFile(t, cfg.SettingsPath(), `{"original":true}`)
	if err := installer.SnapshotRun(cfg); err != nil {
		t.Fatalf("installer.SnapshotRun() error = %v", err)
	}
	writeRollbackTestFile(t, cfg.ClaudeMDPath(), rollbackManagedBlockMD("hand-edited managed block"))

	out, err := execRootWithStateHome(t, home, stateHome, "rollback", "--force")
	if err != nil {
		t.Fatalf("rollback --force error = %v, want nil, output:\n%s", err, out)
	}

	gotClaude, err := os.ReadFile(cfg.ClaudeMDPath())
	if err != nil {
		t.Fatalf("read CLAUDE.md after forced rollback: %v", err)
	}
	if string(gotClaude) != rollbackManagedBlockMD("managed v1") {
		t.Fatalf("CLAUDE.md after forced rollback = %q, want the restored snapshot content", gotClaude)
	}
}

// TestRollback_MenuDispatchShape_InteractiveYes_WarnsAndRestores proves the live menu row works:
// the exact args the standing menu dispatches ([]string{"rollback"}, no flags) with an interactive
// "y" answer must warn about the edited shared file by name, obtain consent, and then restore it
// to its pre-run backup content.
func TestRollback_MenuDispatchShape_InteractiveYes_WarnsAndRestores(t *testing.T) {
	forceTerminalDetection(t, true, true)
	home := t.TempDir()
	stateHome := t.TempDir()
	cfg := installer.Config{ClaudeHome: home, ClickStateHome: stateHome}
	sharedPath := seedWarnableSharedFile(t, cfg)

	out, err := execRollbackWithStdin(t, home, stateHome, "y\n", "rollback")
	if err != nil {
		t.Fatalf("rollback error = %v, want nil after interactive consent, output:\n%s", err, out)
	}
	if !strings.Contains(out, "Advertencia") || !strings.Contains(out, sharedPath) {
		t.Fatalf("rollback output = %q, want the shared-file warning naming %s", out, sharedPath)
	}
	if !strings.Contains(out, "Archivos gestionados por click restaurados desde el último respaldo.") {
		t.Fatalf("rollback output = %q, want the restore success message", out)
	}
	got, err := os.ReadFile(sharedPath)
	if err != nil {
		t.Fatalf("read shared.txt after rollback: %v", err)
	}
	if string(got) != "shared original\n" {
		t.Fatalf("shared.txt after consented rollback = %q, want the pre-run backup content %q", got, "shared original\n")
	}
}

// TestRollback_InteractiveDecline_ZeroWrites guards the decline path: an interactive "n" — and
// separately an immediate EOF — must cancel cleanly ("Rollback cancelado.", nil error) with ZERO
// writes, mirroring confirmAndSnapshot's own decline semantics.
func TestRollback_InteractiveDecline_ZeroWrites(t *testing.T) {
	for _, tc := range []struct {
		name  string
		stdin string
	}{
		{name: "explicit n", stdin: "n\n"},
		{name: "immediate EOF", stdin: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			forceTerminalDetection(t, true, true)
			home := t.TempDir()
			stateHome := t.TempDir()
			cfg := installer.Config{ClaudeHome: home, ClickStateHome: stateHome}
			sharedPath := seedWarnableSharedFile(t, cfg)

			out, err := execRollbackWithStdin(t, home, stateHome, tc.stdin, "rollback")
			if err != nil {
				t.Fatalf("rollback error = %v, want nil (declining is a clean outcome), output:\n%s", err, out)
			}
			if !strings.Contains(out, "Rollback cancelado.") {
				t.Fatalf("rollback output = %q, want the cancellation message", out)
			}
			got, err := os.ReadFile(sharedPath)
			if err != nil {
				t.Fatalf("read shared.txt after declined rollback: %v", err)
			}
			if string(got) != "shared edited by Claude Code after the run\n" {
				t.Fatalf("shared.txt after declined rollback = %q, want untouched (zero writes)", got)
			}
			gotClaude, err := os.ReadFile(cfg.ClaudeMDPath())
			if err != nil {
				t.Fatalf("read CLAUDE.md after declined rollback: %v", err)
			}
			if string(gotClaude) != rollbackManagedBlockMD("managed v1") {
				t.Fatalf("CLAUDE.md after declined rollback = %q, want untouched (zero writes)", gotClaude)
			}
		})
	}
}

// TestRollback_NonInteractivePendingWarnableWithoutYes_Refuses guards the CI/script safety path:
// with a pending shared-file warning and no --yes, a non-interactive rollback (a non-TTY, or an
// explicit --non-interactive) must refuse BEFORE any write and point at `click rollback --yes`.
func TestRollback_NonInteractivePendingWarnableWithoutYes_Refuses(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{name: "non-TTY streams", args: []string{"rollback"}},
		{name: "explicit --non-interactive", args: []string{"rollback", "--non-interactive"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			stateHome := t.TempDir()
			cfg := installer.Config{ClaudeHome: home, ClickStateHome: stateHome}
			sharedPath := seedWarnableSharedFile(t, cfg)

			out, err := execRootWithStateHome(t, home, stateHome, tc.args...)
			if err == nil {
				t.Fatalf("rollback error = nil, want a refusal when consent is required but cannot be asked, output:\n%s", out)
			}
			if !strings.Contains(out, "--yes") {
				t.Fatalf("rollback output = %q, want it to point at `click rollback --yes`", out)
			}
			got, err := os.ReadFile(sharedPath)
			if err != nil {
				t.Fatalf("read shared.txt after refused rollback: %v", err)
			}
			if string(got) != "shared edited by Claude Code after the run\n" {
				t.Fatalf("shared.txt after refused rollback = %q, want untouched (refusal precedes any write)", got)
			}
		})
	}
}

// TestRollback_Yes_SkipsPromptAndRestores guards the scripted-consent path: --yes skips the
// confirmation prompt entirely and restores, including the warnable shared file.
func TestRollback_Yes_SkipsPromptAndRestores(t *testing.T) {
	home := t.TempDir()
	stateHome := t.TempDir()
	cfg := installer.Config{ClaudeHome: home, ClickStateHome: stateHome}
	sharedPath := seedWarnableSharedFile(t, cfg)

	out, err := execRootWithStateHome(t, home, stateHome, "rollback", "--yes")
	if err != nil {
		t.Fatalf("rollback --yes error = %v, want nil, output:\n%s", err, out)
	}
	if strings.Contains(out, "¿Continuar?") {
		t.Fatalf("rollback --yes output = %q, want NO confirmation prompt", out)
	}
	got, err := os.ReadFile(sharedPath)
	if err != nil {
		t.Fatalf("read shared.txt after rollback --yes: %v", err)
	}
	if string(got) != "shared original\n" {
		t.Fatalf("shared.txt after rollback --yes = %q, want the restored pre-run backup content", got)
	}
}

// TestRollback_NonVetoMatchingBaseline_NotWarned guards that a non-veto file whose current content
// matches EITHER baseline (pre-run backup or recorded post-run hash) needs no warning and no
// consent: non-interactive rollback without --yes must proceed straight through.
func TestRollback_NonVetoMatchingBaseline_NotWarned(t *testing.T) {
	home := t.TempDir()
	stateHome := t.TempDir()
	cfg := installer.Config{ClaudeHome: home, ClickStateHome: stateHome}
	uneditedPath := filepath.Join(home, "unedited.txt")
	postRunPath := filepath.Join(home, "postrun.txt")
	writeRollbackTestFile(t, cfg.ClaudeMDPath(), rollbackManagedBlockMD("managed v1"))
	writeRollbackTestFile(t, cfg.SettingsPath(), `{"original":true}`)
	writeRollbackTestFile(t, uneditedPath, "same as backup\n")
	writeRollbackTestFile(t, postRunPath, "post-run state\n")
	if err := installer.SnapshotRun(cfg); err != nil {
		t.Fatalf("installer.SnapshotRun() error = %v", err)
	}
	appendRollbackManifestEntry(t, cfg, rollbackManifestEntry{
		OriginalPath: uneditedPath,
		BackupFile:   "unedited.txt",
		Existed:      true,
		DriftPolicy:  "non-veto",
	}, "same as backup\n")
	appendRollbackManifestEntry(t, cfg, rollbackManifestEntry{
		OriginalPath:        postRunPath,
		BackupFile:          "postrun.txt",
		Existed:             true,
		ExpectedPostRunHash: installer.CanonicalContentHash("post-run state\n"),
		DriftPolicy:         "non-veto",
	}, "pre-run state\n")

	out, err := execRootWithStateHome(t, home, stateHome, "rollback")
	if err != nil {
		t.Fatalf("rollback error = %v, want nil (nothing requires consent), output:\n%s", err, out)
	}
	if strings.Contains(out, "Advertencia") {
		t.Fatalf("rollback output = %q, want NO shared-file warning when both non-veto files match a baseline", out)
	}
	if !strings.Contains(out, "Archivos gestionados por click restaurados desde el último respaldo.") {
		t.Fatalf("rollback output = %q, want the restore success message", out)
	}
}

// TestRollback_Force_DoesNotImplyWarnableConsent guards the flag semantics: --force bypasses
// VETOES only — a still-pending shared-file warning requires separate consent. Non-interactive
// --force alone must refuse (naming --yes) with zero writes; --force --yes together must restore
// both the vetoing file and the shared file.
func TestRollback_Force_DoesNotImplyWarnableConsent(t *testing.T) {
	home := t.TempDir()
	stateHome := t.TempDir()
	cfg := installer.Config{ClaudeHome: home, ClickStateHome: stateHome}
	sharedPath := seedWarnableSharedFile(t, cfg)
	writeRollbackTestFile(t, cfg.ClaudeMDPath(), rollbackManagedBlockMD("hand-edited managed block"))

	out, err := execRootWithStateHome(t, home, stateHome, "rollback", "--force")
	if err == nil {
		t.Fatalf("rollback --force error = nil, want a refusal (pending shared-file consent), output:\n%s", out)
	}
	if !strings.Contains(out, "--yes") {
		t.Fatalf("rollback --force output = %q, want it to point at `click rollback --yes`", out)
	}
	got, err := os.ReadFile(sharedPath)
	if err != nil {
		t.Fatalf("read shared.txt after refused rollback: %v", err)
	}
	if string(got) != "shared edited by Claude Code after the run\n" {
		t.Fatalf("shared.txt after refused --force rollback = %q, want untouched (zero writes)", got)
	}

	out, err = execRootWithStateHome(t, home, stateHome, "rollback", "--force", "--yes")
	if err != nil {
		t.Fatalf("rollback --force --yes error = %v, want nil, output:\n%s", err, out)
	}
	got, err = os.ReadFile(sharedPath)
	if err != nil {
		t.Fatalf("read shared.txt after --force --yes rollback: %v", err)
	}
	if string(got) != "shared original\n" {
		t.Fatalf("shared.txt after --force --yes rollback = %q, want the restored pre-run backup content", got)
	}
	gotClaude, err := os.ReadFile(cfg.ClaudeMDPath())
	if err != nil {
		t.Fatalf("read CLAUDE.md after --force --yes rollback: %v", err)
	}
	if string(gotClaude) != rollbackManagedBlockMD("managed v1") {
		t.Fatalf("CLAUDE.md after --force --yes rollback = %q, want the restored snapshot content", gotClaude)
	}
}

// TestRollback_HiddenFromHelp guards that `click rollback` follows manage-backups/
// configure-models' precedent: reachable directly but hidden from `click --help`.
func TestRollback_HiddenFromHelp(t *testing.T) {
	home := t.TempDir()
	cmd := newRollbackCommand()
	if cmd.Short != "Restaurar los archivos gestionados por click desde el último respaldo de instalación/actualización" {
		t.Fatalf("rollback Short = %q, want the managed-files description", cmd.Short)
	}
	if !strings.Contains(cmd.Long, "CLI externo `claude`") || !strings.Contains(cmd.Long, "no solo cambios de Click") {
		t.Fatalf("rollback Long = %q, want coarse-snapshot warning about external claude changes", cmd.Long)
	}

	out, err := execRoot(t, home, "--help")
	if err != nil {
		t.Fatalf("--help error = %v", err)
	}
	if strings.Contains(out, "rollback") {
		t.Fatalf("--help output = %q, want `rollback` hidden from help text", out)
	}
}
