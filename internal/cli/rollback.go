package cli

import (
	"bufio"
	"fmt"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/installer"
	"github.com/spf13/cobra"
)

// rollbackForceFlag lets a developer explicitly override the refuse-by-default hand-edit guard
// (spec install-rollback Decision 3) and restore anyway. It bypasses VETOES only — a pending
// shared-file (non-veto) warning still requires separate consent via --yes or an interactive "y".
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
// --non-interactive is NOT an alias for --yes — consent for overwriting shared (non-veto) files
// must come from --yes or an interactive answer, never from merely being non-interactive.
func newRollbackCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "rollback",
		Short:  "Restaurar los archivos gestionados por click desde el último respaldo de instalación/actualización",
		Long:   "Rollback restaura la instantánea general y puede revertir también cambios realizados por el CLI externo `claude` desde la última ejecución de instalación o actualización, no solo cambios de Click.",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRollback(cmd)
		},
	}
	cmd.Flags().Bool(rollbackForceFlag, false, "Sobrescribir aunque los archivos se hayan editado manualmente desde el respaldo")
	cmd.Flags().Bool(yesFlag, false, "Confirmar la restauración sin preguntar, incluso si archivos compartidos tienen cambios posteriores al respaldo")
	cmd.Flags().Bool(nonInteractiveFlag, false, "No preguntar: rechazar si la restauración requiere confirmación (para scripts/CI; combinar con --yes para confirmar)")
	return cmd
}

// runRollback implements the spec's install-rollback capability, ownership-scoped:
//  1. no restorable snapshot -> report cleanly, no error, no fabricated content.
//  2. click-owned content drifted since the snapshot (veto) and no --force -> refuse, name the
//     drifted files, zero writes (spec Decision 3: refuse-by-default).
//  3. shared (non-veto) files drifted -> warn by name, then require consent: an interactive "y",
//     or --yes. Non-interactive without --yes refuses BEFORE any write; a declined confirmation
//     cancels cleanly (nil error, zero writes). --force alone does NOT grant this consent.
//  4. otherwise -> ApplyPreparedRestore, restoring EVERY manifest entry (restore coverage is not
//     ownership-scoped, only veto scope is), leaving the snapshot intact for a future rollback.
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
	cfg := installer.Config{ClaudeHome: claudeHome, ClickStateHome: clickStateHome}

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

	if !force && len(report.Vetoes) > 0 {
		fmt.Fprintln(out, r.Warn("Los siguientes archivos se editaron manualmente desde el respaldo; rollback rechazado:"))
		for _, path := range report.Vetoes {
			fmt.Fprintf(out, "  - %s\n", path)
		}
		fmt.Fprintln(out, r.Info("Ejecute `click rollback --force` para sobrescribirlos de todas formas."))
		return fmt.Errorf("cli: rollback rechazado: %d archivo(s) editado(s) desde el respaldo", len(report.Vetoes))
	}

	if len(report.WarnableNonVeto) > 0 {
		fmt.Fprintln(out, r.Warn("Advertencia: rollback sobrescribirá cambios posteriores en estos archivos compartidos:"))
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

	// A restored settings.json backup has ENGRAM_CLOUD_TOKEN's value permanently redacted (NFR-6):
	// left as-is, the literal placeholder would sit in the live file as a garbage token, silently
	// breaking every subsequent Engram Cloud sync. Fix it up now, non-fatally: rollback's main job
	// already succeeded above.
	repaired, repairErr := installer.RepairRedactedEngramCloudTokenAfterRestore(cfg)
	if repairErr != nil {
		fmt.Fprintln(out, r.Warn("No se pudo limpiar el token de Engram Cloud redactado tras la restauración: "+repairErr.Error()))
	} else if repaired {
		fmt.Fprintln(out, r.Warn("El respaldo restaurado no conservaba el token de Engram Cloud (no se puede recuperar tras un rollback); vuelva a ejecutar `click install`/`click update` para reconsentir el token si desea reactivar la sincronización en la nube."))
	}
	return nil
}
