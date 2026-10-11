package cli

import (
	"bufio"
	"fmt"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/installer"
	"github.com/spf13/cobra"
)

// rollbackForceFlag lets a developer explicitly override the refuse-by-default hand-edit guard
// (spec install-rollback Decision 3) and restore anyway. It bypasses VETOES only — a pending
// warning for an eligible Click-owned legacy artifact still requires --yes or an interactive "y".
const rollbackForceFlag = "force"

// newRollbackCommand backs `click rollback`: restores the last run-start snapshot
// (installer.PrepareRestore + installer.ApplyPreparedRestore). Deliberately a SEPARATE command
// from `manage-backups --restore` — see design's "Rollback surface" decision: the run snapshot is
// a distinct, multi-file, manifest-backed artifact from models.json.bak, and reusing --restore
// would silently change manage-backups' already-tested semantics (managebackups_test.go). Hidden
// like manage-backups/configure-models: reached mainly through the standing menu, but still
// directly runnable for scripts/developers — matching newManageBackupsCommand's exact pattern.
//
// --yes/--non-interactive are rollback-local (bound HERE, not root-persistent), mirroring how
// install.go/update.go declare their own. NOTE the semantics differ from install's: for rollback,
// --non-interactive is NOT an alias for --yes — any eligible legacy-artifact warning requires
// --yes or an interactive answer, never from merely being non-interactive.
func newRollbackCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "rollback",
		Short:  "Restaurar los archivos gestionados por click desde el último respaldo de instalación/actualización",
		Long:   "Rollback restaura contenido Click cuya propiedad puede verificarse. Conserva las configuraciones nativas compartidas y las rutas históricas cuya propiedad no puede confirmarse.",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRollback(cmd)
		},
	}
	cmd.Flags().Bool(rollbackForceFlag, false, "Sobrescribir aunque los archivos se hayan editado manualmente desde el respaldo")
	cmd.Flags().Bool(yesFlag, false, "Confirmar sin preguntar cualquier advertencia de restauración")
	cmd.Flags().Bool(nonInteractiveFlag, false, "No preguntar: rechazar si la restauración requiere confirmación (para scripts/CI; combinar con --yes para confirmar)")
	return cmd
}

// runRollback implements the spec's install-rollback capability, ownership-scoped:
//  1. no restorable snapshot -> report cleanly, no error, no fabricated content.
//  2. click-owned content drifted since the snapshot (veto) and no --force -> refuse, name the
//     drifted files, zero writes (spec Decision 3: refuse-by-default).
//  3. shared native configs and unknown historical roots are reported and skipped regardless of
//     --force/--yes; they never enter the write plan.
//  4. otherwise -> ApplyPreparedRestore applies only the prepared exact-owned transformations and
//     leaves the snapshot intact for a future rollback.
func runRollback(cmd *cobra.Command) error {
	out := cmd.OutOrStdout()
	r := rendererFor(cmd, out)

	force, err := cmd.Flags().GetBool(rollbackForceFlag)
	if err != nil {
		return err
	}
	yes, err := cmd.Flags().GetBool(yesFlag)
	if err != nil {
		return err
	}

	claudeHome, err := installer.ResolveClaudeHome()
	if err != nil {
		return err
	}
	// ClickStateHome roots BackupDir(); ClaudeHome is still needed so the read-through to a
	// pre-migration snapshot location (LegacyBackupDir) can find one.
	clickStateHome, err := installer.ResolveClickStateHome()
	if err != nil {
		return err
	}
	codexHome, err := installer.ResolveCodexHome()
	if err != nil {
		return err
	}
	openClawHome, err := installer.ResolveOpenClawHome()
	if err != nil {
		return err
	}
	cfg := installer.Config{ClaudeHome: claudeHome, CodexHome: codexHome, OpenClawHome: openClawHome, ClickStateHome: clickStateHome}

	restorable, err := installer.HasRestorableSnapshot(cfg)
	if err != nil {
		return err
	}
	if !restorable {
		fmt.Fprintln(out, r.Info("No hay ningún respaldo de instalación/actualización para restaurar."))
		return nil
	}

	prepared, err := installer.PrepareRestore(cfg)
	if err != nil {
		return err
	}
	report := prepared.Drift
	if len(report.Skipped) > 0 {
		fmt.Fprintln(out, r.Info("Se conservaron las configuraciones compartidas o rutas históricas sin propiedad Click verificable:"))
		for _, path := range report.Skipped {
			fmt.Fprintf(out, "  - %s\n", path)
		}
	}

	if !force && len(report.Vetoes) > 0 {
		fmt.Fprintln(out, r.Warn("Los siguientes archivos se editaron manualmente desde el respaldo; rollback rechazado:"))
		for _, path := range report.Vetoes {
			fmt.Fprintf(out, "  - %s\n", path)
		}
		fmt.Fprintln(out, r.Info("Ejecute `click rollback --force` para sobrescribirlos de todas formas."))
		return fmt.Errorf("cli: rollback rechazado: %d archivo(s) editado(s) desde el respaldo", len(report.Vetoes))
	}

	if len(report.WarnableNonVeto) > 0 {
		fmt.Fprintln(out, r.Warn("Advertencia: estos archivos gestionados cambiaron desde el respaldo y pueden sobrescribirse:"))
		for _, path := range report.WarnableNonVeto {
			fmt.Fprintf(out, "  - %s\n", path)
		}
		fmt.Fprintln(out, r.Info("Estos archivos pueden contener estado ajeno a Click. Revíselos después de la restauración."))
		if !yes {
			if isNonInteractiveInstall(cmd, out) {
				fmt.Fprintln(out, r.Info("Modo no interactivo: ejecute `click rollback --yes` para confirmar la restauración."))
				return fmt.Errorf("cli: rollback rechazado: %d archivo(s) compartido(s) requieren confirmación con --yes", len(report.WarnableNonVeto))
			}
			reader := bufio.NewReader(cmd.InOrStdin())
			proceed, err := confirmProceed(reader, out, r)
			if err != nil {
				return err
			}
			if !proceed {
				fmt.Fprintln(out, r.Info("Rollback cancelado."))
				return nil
			}
		}
	}
	if err := installer.ApplyPreparedRestore(prepared); err != nil {
		return err
	}
	fmt.Fprintln(out, r.Success("Archivos gestionados por click restaurados desde el último respaldo."))

	return nil
}
