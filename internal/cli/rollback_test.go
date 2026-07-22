package cli

import (
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

// TestRollback_NoSnapshotEver_ReportsNothingToRestoreNoError guards the spec install-rollback "No
// snapshot exists" scenario when SnapshotRun has never even run: rollback must not error, must not
// fabricate content, and must report cleanly.
func TestRollback_NoSnapshotEver_ReportsNothingToRestoreNoError(t *testing.T) {
	home := t.TempDir()

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
	cfg := installer.Config{ClaudeHome: home}
	if err := installer.SnapshotRun(cfg); err != nil {
		t.Fatalf("installer.SnapshotRun() error = %v", err)
	}

	out, err := execRoot(t, home, "rollback")
	if err != nil {
		t.Fatalf("rollback error = %v, want nil, output:\n%s", err, out)
	}
	if !strings.Contains(out, "No hay ningún respaldo") {
		t.Fatalf("rollback output = %q, want the nothing-to-restore informational message", out)
	}
}

// TestRollback_MatchingHash_NoForce_RestoresBothFilesAndSnapshotSurvives guards the spec
// "Successful restore" scenario: no drift since the snapshot means rollback proceeds even without
// --force, restores both files, and the snapshot itself survives (not consumed).
func TestRollback_MatchingHash_NoForce_RestoresBothFilesAndSnapshotSurvives(t *testing.T) {
	home := t.TempDir()
	cfg := installer.Config{ClaudeHome: home}
	writeRollbackTestFile(t, cfg.ClaudeMDPath(), "snapshot content\n")
	writeRollbackTestFile(t, cfg.SettingsPath(), `{"snapshot":true}`)
	if err := installer.SnapshotRun(cfg); err != nil {
		t.Fatalf("installer.SnapshotRun() error = %v", err)
	}

	out, err := execRoot(t, home, "rollback")
	if err != nil {
		t.Fatalf("rollback error = %v, want nil, output:\n%s", err, out)
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
// (refuse-by-default): a hand-edit since the snapshot must produce an error, report the drifted
// path, and leave BOTH files exactly as they currently are (zero writes).
func TestRollback_DriftNoForce_RefusesAndMakesZeroWrites(t *testing.T) {
	home := t.TempDir()
	cfg := installer.Config{ClaudeHome: home}
	writeRollbackTestFile(t, cfg.ClaudeMDPath(), "original content\n")
	writeRollbackTestFile(t, cfg.SettingsPath(), `{"original":true}`)
	if err := installer.SnapshotRun(cfg); err != nil {
		t.Fatalf("installer.SnapshotRun() error = %v", err)
	}
	writeRollbackTestFile(t, cfg.ClaudeMDPath(), "hand-edited after snapshot\n")

	out, err := execRoot(t, home, "rollback")
	if err == nil {
		t.Fatalf("rollback error = nil, want a refusal error when content drifted, output:\n%s", out)
	}
	if !strings.Contains(out, cfg.ClaudeMDPath()) {
		t.Fatalf("rollback output = %q, want it to name the drifted file %s", out, cfg.ClaudeMDPath())
	}

	gotClaude, err := os.ReadFile(cfg.ClaudeMDPath())
	if err != nil {
		t.Fatalf("read CLAUDE.md after refused rollback: %v", err)
	}
	if string(gotClaude) != "hand-edited after snapshot\n" {
		t.Fatalf("CLAUDE.md after refused rollback = %q, want untouched %q", gotClaude, "hand-edited after snapshot\n")
	}
	gotSettings, err := os.ReadFile(cfg.SettingsPath())
	if err != nil {
		t.Fatalf("read settings.json after refused rollback: %v", err)
	}
	if string(gotSettings) != `{"original":true}` {
		t.Fatalf("settings.json after refused rollback = %q, want untouched %q", gotSettings, `{"original":true}`)
	}
}

// TestRollback_DriftWithForce_Proceeds guards the explicit override: the same drift as above, but
// with --force, must proceed and restore the snapshotted content.
func TestRollback_DriftWithForce_Proceeds(t *testing.T) {
	home := t.TempDir()
	cfg := installer.Config{ClaudeHome: home}
	writeRollbackTestFile(t, cfg.ClaudeMDPath(), "original content\n")
	writeRollbackTestFile(t, cfg.SettingsPath(), `{"original":true}`)
	if err := installer.SnapshotRun(cfg); err != nil {
		t.Fatalf("installer.SnapshotRun() error = %v", err)
	}
	writeRollbackTestFile(t, cfg.ClaudeMDPath(), "hand-edited after snapshot\n")

	out, err := execRoot(t, home, "rollback", "--force")
	if err != nil {
		t.Fatalf("rollback --force error = %v, want nil, output:\n%s", err, out)
	}

	gotClaude, err := os.ReadFile(cfg.ClaudeMDPath())
	if err != nil {
		t.Fatalf("read CLAUDE.md after forced rollback: %v", err)
	}
	if string(gotClaude) != "original content\n" {
		t.Fatalf("CLAUDE.md after forced rollback = %q, want the restored snapshot content %q", gotClaude, "original content\n")
	}
}

// TestRollback_HiddenFromHelp guards that `click rollback` follows manage-backups/
// configure-models' precedent: reachable directly but hidden from `click --help`.
func TestRollback_HiddenFromHelp(t *testing.T) {
	home := t.TempDir()
	cmd := newRollbackCommand()
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
