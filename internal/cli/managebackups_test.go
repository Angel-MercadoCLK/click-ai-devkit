package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/installer"
)

// seedModelsBackup writes cfg.ModelsPath()+".bak" with the given content under home, creating
// its parent directory as needed, and returns the backup path.
func seedModelsBackup(t *testing.T, home, content string) string {
	t.Helper()
	cfg := installer.Config{ClaudeHome: home}
	backupPath := cfg.ModelsPath() + ".bak"
	if err := os.MkdirAll(filepath.Dir(backupPath), 0o755); err != nil {
		t.Fatalf("seed backup dir: %v", err)
	}
	if err := os.WriteFile(backupPath, []byte(content), 0o600); err != nil {
		t.Fatalf("seed backup file: %v", err)
	}
	return backupPath
}

// TestManageBackups_NoBackupFile_ReportsCleanlyNoError guards the "nothing to manage" case: no
// .bak file exists, the command must report that cleanly and exit success (no error), never a
// confusing failure.
func TestManageBackups_NoBackupFile_ReportsCleanlyNoError(t *testing.T) {
	home := t.TempDir()

	out, err := execRoot(t, home, "manage-backups")
	if err != nil {
		t.Fatalf("manage-backups error = %v, want nil when no backup exists", err)
	}
	if !strings.Contains(out, "No hay copias de seguridad disponibles.") {
		t.Fatalf("manage-backups output = %q, want the no-backups informational message", out)
	}
}

// TestManageBackups_NoFlags_ReportsExistenceAndAvailableActions guards the interactive/
// informational case: a backup exists, no flags passed — report its existence and the two
// available actions, and never mutate anything.
func TestManageBackups_NoFlags_ReportsExistenceAndAvailableActions(t *testing.T) {
	home := t.TempDir()
	backupPath := seedModelsBackup(t, home, `{"schema_version":2,"models":{}}`)
	before, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("read seeded backup: %v", err)
	}

	out, err := execRoot(t, home, "manage-backups")
	if err != nil {
		t.Fatalf("manage-backups error = %v, want nil", err)
	}
	if !strings.Contains(out, "--restore") || !strings.Contains(out, "--delete") {
		t.Fatalf("manage-backups output = %q, want it to mention both --restore and --delete", out)
	}

	after, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("read backup after no-flags run: %v", err)
	}
	if string(before) != string(after) {
		t.Fatal("manage-backups with no flags mutated the backup file, want untouched")
	}
	cfg := installer.Config{ClaudeHome: home}
	if _, err := os.Stat(cfg.ModelsPath()); !os.IsNotExist(err) {
		t.Fatalf("manage-backups with no flags created/touched models.json, want it to stay absent (err = %v)", err)
	}
}

// TestManageBackups_Restore_CopiesBackupContentAndKeepsBackupFile guards --restore: models.json
// ends up with the backup's exact content, and the .bak file itself is NOT consumed/deleted by
// restore (read+write, never rename/move).
func TestManageBackups_Restore_CopiesBackupContentAndKeepsBackupFile(t *testing.T) {
	home := t.TempDir()
	wantContent := `{"schema_version":2,"profile":"cost-saver","models":{"explore":"haiku"}}`
	backupPath := seedModelsBackup(t, home, wantContent)

	out, err := execRoot(t, home, "manage-backups", "--restore")
	if err != nil {
		t.Fatalf("manage-backups --restore error = %v, want nil, output:\n%s", err, out)
	}

	cfg := installer.Config{ClaudeHome: home}
	got, err := os.ReadFile(cfg.ModelsPath())
	if err != nil {
		t.Fatalf("read restored models.json: %v", err)
	}
	if string(got) != wantContent {
		t.Fatalf("restored models.json content = %q, want %q", string(got), wantContent)
	}

	if _, err := os.Stat(backupPath); err != nil {
		t.Fatalf("backup file missing after --restore, want it to still exist: %v", err)
	}
}

// TestManageBackups_Delete_RemovesBackupLeavesModelsUntouched guards --delete: the .bak file no
// longer exists afterward, and the real models.json (if any) is untouched.
func TestManageBackups_Delete_RemovesBackupLeavesModelsUntouched(t *testing.T) {
	home := t.TempDir()
	backupPath := seedModelsBackup(t, home, `{"schema_version":2,"models":{}}`)

	cfg := installer.Config{ClaudeHome: home}
	liveContent := `{"schema_version":2,"models":{"explore":"opus"}}`
	if err := os.WriteFile(cfg.ModelsPath(), []byte(liveContent), 0o600); err != nil {
		t.Fatalf("seed live models.json: %v", err)
	}

	out, err := execRoot(t, home, "manage-backups", "--delete")
	if err != nil {
		t.Fatalf("manage-backups --delete error = %v, want nil, output:\n%s", err, out)
	}

	if _, err := os.Stat(backupPath); !os.IsNotExist(err) {
		t.Fatalf("backup file still exists after --delete (err = %v), want it removed", err)
	}

	got, err := os.ReadFile(cfg.ModelsPath())
	if err != nil {
		t.Fatalf("read live models.json after --delete: %v", err)
	}
	if string(got) != liveContent {
		t.Fatalf("live models.json content = %q after --delete, want untouched %q", string(got), liveContent)
	}
}

// TestManageBackups_RestoreAndDeleteTogether_ReturnsErrorNoMutation guards the mutually exclusive
// flag combination: passing both --restore and --delete must return a clear error and must not
// mutate either file.
func TestManageBackups_RestoreAndDeleteTogether_ReturnsErrorNoMutation(t *testing.T) {
	home := t.TempDir()
	backupContent := `{"schema_version":2,"models":{}}`
	backupPath := seedModelsBackup(t, home, backupContent)

	out, err := execRoot(t, home, "manage-backups", "--restore", "--delete")
	if err == nil {
		t.Fatalf("manage-backups --restore --delete error = nil, want a clear error, output:\n%s", out)
	}

	cfg := installer.Config{ClaudeHome: home}
	if _, statErr := os.Stat(cfg.ModelsPath()); !os.IsNotExist(statErr) {
		t.Fatalf("manage-backups --restore --delete mutated models.json, want it to stay absent (err = %v)", statErr)
	}
	got, readErr := os.ReadFile(backupPath)
	if readErr != nil {
		t.Fatalf("backup file missing after failed --restore --delete: %v", readErr)
	}
	if string(got) != backupContent {
		t.Fatalf("backup content = %q after failed --restore --delete, want untouched %q", string(got), backupContent)
	}
}
