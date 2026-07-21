package cli

import (
	"fmt"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/installer"
	"github.com/spf13/cobra"
)

// rollbackForceFlag lets a developer explicitly override the refuse-by-default hand-edit guard
// (spec install-rollback Decision 3) and restore anyway.
const rollbackForceFlag = "force"

// newRollbackCommand backs `click rollback`: restores CLAUDE.md and settings.json from the last
// run-start snapshot (installer.RestoreRun). Deliberately a SEPARATE command from
// `manage-backups --restore` — see design's "Rollback surface" decision: the run snapshot is a
// distinct, multi-file, manifest-backed artifact from models.json.bak, and reusing --restore would
// silently change manage-backups' already-tested semantics (managebackups_test.go). Hidden like
// manage-backups/configure-models: reached mainly through the standing menu, but still directly
// runnable for scripts/developers — matching newManageBackupsCommand's exact pattern.
func newRollbackCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "rollback",
		Short:  "Restaurar CLAUDE.md y settings.json desde el último respaldo de instalación/actualización",
		Long:   "Rollback restaura la instantánea general y puede revertir también cambios realizados por el CLI externo `claude` desde la última ejecución de instalación o actualización, no solo cambios de Click.",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRollback(cmd)
		},
	}
	cmd.Flags().Bool(rollbackForceFlag, false, "Sobrescribir aunque los archivos se hayan editado manualmente desde el respaldo")
	return cmd
}

// runRollback implements the spec's install-rollback capability:
//  1. no restorable snapshot -> report cleanly, no error, no fabricated content.
//  2. content drifted since the snapshot (hand-edited) and no --force -> refuse, name the
//     drifted files, zero writes (spec Decision 3: refuse-by-default).
//  3. otherwise (no drift, or --force) -> installer.RestoreRun, which itself leaves the snapshot
//     intact for a possible future rollback.
func runRollback(cmd *cobra.Command) error {
	out := cmd.OutOrStdout()
	r := rendererFor(cmd, out)

	force, err := cmd.Flags().GetBool(rollbackForceFlag)
	if err != nil {
		return err
	}

	claudeHome, err := installer.ResolveClaudeHome()
	if err != nil {
		return err
	}
	cfg := installer.Config{ClaudeHome: claudeHome}

	restorable, err := installer.HasRestorableSnapshot(cfg)
	if err != nil {
		return err
	}
	if !restorable {
		fmt.Fprintln(out, r.Info("No hay ningún respaldo de instalación/actualización para restaurar."))
		return nil
	}

	if !force {
		drifted, driftErr := installer.SnapshotDrift(cfg)
		if driftErr != nil {
			return driftErr
		}
		if len(drifted) > 0 {
			fmt.Fprintln(out, r.Warn("Los siguientes archivos se editaron manualmente desde el respaldo; rollback rechazado:"))
			for _, path := range drifted {
				fmt.Fprintf(out, "  - %s\n", path)
			}
			fmt.Fprintln(out, r.Info("Ejecute `click rollback --force` para sobrescribirlos de todas formas."))
			return fmt.Errorf("cli: rollback rechazado: %d archivo(s) editado(s) desde el respaldo", len(drifted))
		}
	}

	if err := installer.RestoreRun(cfg); err != nil {
		return err
	}
	fmt.Fprintln(out, r.Success("CLAUDE.md y settings.json restaurados desde el último respaldo."))
	return nil
}
