package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/installer"
	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/ui"
	"github.com/spf13/cobra"
)

// manageBackupsRestoreFlag and manageBackupsDeleteFlag name the two mutually exclusive flags
// `click manage-backups` accepts.
const (
	manageBackupsRestoreFlag = "restore"
	manageBackupsDeleteFlag  = "delete"
)

// manageBackupsTimeLayout is the human-readable timestamp format used to report the backup's
// last-modified time — a fixed, unambiguous layout (no locale dependency) consistent with this
// codebase not otherwise formatting timestamps for end users elsewhere.
const manageBackupsTimeLayout = "2006-01-02 15:04:05"

// newManageBackupsCommand backs the menu's "Gestionar backups" item. The only backup mechanism
// in this codebase today is installer.MigrateIfStale (models.go), which writes
// cfg.ModelsPath()+".bak" before regenerating a stale models.json — this command is a thin,
// flag-driven way to inspect, restore, or discard that single backup file. It deliberately does
// NOT build an interactive TUI (unlike agent-builder/configure-models): the surface is small
// enough that a flag-driven command is simpler and, unlike a bubbletea program, never hangs
// regardless of TTY state. Hidden from `click --help` since it's primarily reached through the
// standing menu, but still directly runnable (`click manage-backups`) for scripts or developers
// who want to skip the menu — matching newConfigureModelsCommand's exact pattern.
func newManageBackupsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "manage-backups",
		Short:  "Ver y gestionar la copia de seguridad de models.json",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runManageBackups(cmd)
		},
	}
	cmd.Flags().Bool(manageBackupsRestoreFlag, false, "Restaurar models.json desde la copia de seguridad")
	cmd.Flags().Bool(manageBackupsDeleteFlag, false, "Eliminar la copia de seguridad")
	return cmd
}

func runManageBackups(cmd *cobra.Command) error {
	out := cmd.OutOrStdout()
	r := rendererFor(cmd, out)

	restore, err := cmd.Flags().GetBool(manageBackupsRestoreFlag)
	if err != nil {
		return err
	}
	deleteBackup, err := cmd.Flags().GetBool(manageBackupsDeleteFlag)
	if err != nil {
		return err
	}
	// Validate the flag combination BEFORE touching the filesystem at all, so an invalid
	// invocation never mutates anything regardless of whether a backup happens to exist.
	if restore && deleteBackup {
		return fmt.Errorf("cli: --%s y --%s no pueden usarse juntos", manageBackupsRestoreFlag, manageBackupsDeleteFlag)
	}

	claudeHome, err := installer.ResolveClaudeHome()
	if err != nil {
		return err
	}
	cfg := installer.Config{ClaudeHome: claudeHome}
	backupPath := cfg.ModelsPath() + ".bak"

	info, statErr := os.Stat(backupPath)
	if statErr != nil {
		if os.IsNotExist(statErr) {
			fmt.Fprintln(out, r.Info("No hay copias de seguridad disponibles."))
			return nil
		}
		return fmt.Errorf("cli: comprobar la copia de seguridad: %w", statErr)
	}

	switch {
	case restore:
		return restoreModelsBackup(out, r, cfg.ModelsPath(), backupPath)
	case deleteBackup:
		return deleteModelsBackup(out, r, backupPath)
	default:
		return reportModelsBackup(out, r, backupPath, info.ModTime().Format(manageBackupsTimeLayout))
	}
}

// restoreModelsBackup copies the backup's exact content over the live models.json — a read+write,
// never a rename/move, so the backup file itself remains available afterward in case the
// developer wants to inspect or restore it again later. This mirrors the read+write discipline
// installer.MigrateIfStale already uses for the reverse operation (backing models.json up) — the
// only other place this codebase touches a models.json backup file.
func restoreModelsBackup(out io.Writer, r *ui.Renderer, modelsPath, backupPath string) error {
	data, err := os.ReadFile(backupPath)
	if err != nil {
		return fmt.Errorf("cli: leer la copia de seguridad: %w", err)
	}
	if err := os.WriteFile(modelsPath, data, 0o600); err != nil {
		return fmt.Errorf("cli: restaurar models.json desde la copia de seguridad: %w", err)
	}
	fmt.Fprintln(out, r.Success("models.json restaurado desde la copia de seguridad."))
	return nil
}

// deleteModelsBackup removes the backup file, leaving the live models.json untouched.
func deleteModelsBackup(out io.Writer, r *ui.Renderer, backupPath string) error {
	if err := os.Remove(backupPath); err != nil {
		return fmt.Errorf("cli: eliminar la copia de seguridad: %w", err)
	}
	fmt.Fprintln(out, r.Success("Copia de seguridad eliminada."))
	return nil
}

// reportModelsBackup is the neither-flag informational path: it reports the backup's existence
// and last-modified time, then names the two available actions, without mutating anything.
func reportModelsBackup(out io.Writer, r *ui.Renderer, backupPath, modTime string) error {
	fmt.Fprintln(out, r.Info(fmt.Sprintf("Hay una copia de seguridad disponible en %s (última modificación: %s).", backupPath, modTime)))
	fmt.Fprintln(out, r.Info("Ejecute `click manage-backups --restore` para restaurarla, o `click manage-backups --delete` para eliminarla."))
	return nil
}
