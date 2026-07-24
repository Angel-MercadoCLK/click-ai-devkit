package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/installer"
	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/ui"
	"github.com/spf13/cobra"
)

func installWriteSteps(cfg installer.Config, cloudConfigured bool) []string {
	selection := installer.TargetSelection{Configured: true, Claude: cfg.ClaudeHome != "", OpenClaw: cfg.OpenClawHome != "", Codex: cfg.CodexHome != ""}
	plan := installer.BuildTargetPlan(cfg, selection, installer.PlanOptions{CloudConfigured: cloudConfigured})
	return actionLabels(plan.InstallActionKinds(), actionLabelOptions{})
}

func installWriteStepsForSelection(cfg installer.Config, cloudConfigured bool, selection installer.TargetSelection) []string {
	plan := installer.BuildTargetPlan(cfg, selection, installer.PlanOptions{CloudConfigured: cloudConfigured})
	return actionLabels(plan.InstallActionKinds(), actionLabelOptions{})
}

func updateWriteSteps(engramVersion string, cfg installer.Config, cloudConfigured bool) []string {
	selection := installer.TargetSelection{Configured: true, Claude: cfg.ClaudeHome != "", OpenClaw: cfg.OpenClawHome != "", Codex: cfg.CodexHome != ""}
	plan := installer.BuildTargetPlan(cfg, selection, installer.PlanOptions{CloudConfigured: cloudConfigured})
	return actionLabels(plan.UpdateActionKinds(), actionLabelOptions{engramVersion: engramVersion, updateMode: true})
}

// openClawWriteSteps returns the extra OpenClaw write-step labels to append when cfg.OpenClawHome
// is populated (OpenClaw detected and not skipped via --skip-openclaw), or nil when absent — the
// shared per-target write-step builder reused by both installWriteSteps and updateWriteSteps
// (task 2.18's REFACTOR) so the two commands' preview lists can never drift from each other for the
// OpenClaw portion either. Order matches where install.go/update.go actually run these writes:
// LAST, after every other write step, mirroring how they're appended here.
func openClawWriteSteps(cfg installer.Config) []string {
	if cfg.OpenClawHome == "" {
		return nil
	}
	return []string{
		"Actualizando AGENTS.md y SOUL.md de OpenClaw…",
		"Registrando Engram en OpenClaw (mcpServers)…",
		"Instalando plugin de memory-guard para OpenClaw…",
		"Sincronizando skills de Click en OpenClaw…",
		"Guardando recomendación de modelos para OpenClaw…",
	}
}

type actionLabelOptions struct {
	engramVersion string
	updateMode    bool
}

func actionLabels(kinds []installer.StepActionKind, options actionLabelOptions) []string {
	labels := make([]string, 0, len(kinds))
	for _, kind := range kinds {
		labels = append(labels, actionLabel(kind, options))
	}
	return labels
}

func actionLabel(kind installer.StepActionKind, options actionLabelOptions) string {
	switch kind {
	case installer.StepActionSyncMarketplacePlugins:
		if options.updateMode {
			return "Re-sincronizando plugins click-sdd, click-memory, click-review y click-skills…"
		}
		return "Registrando plugins click-sdd, click-memory, click-review y click-skills…"
	case installer.StepActionSyncEngram:
		if options.engramVersion != "" {
			return fmt.Sprintf("Sincronizando Engram (pin %s)…", options.engramVersion)
		}
		return "Instalando Engram (memoria persistente)…"
	case installer.StepActionSyncEngramCloud:
		if options.engramVersion != "" {
			return "Sincronizando Engram Cloud…"
		}
		return "Enrolando Engram Cloud…"
	case installer.StepActionSyncContext7:
		if options.engramVersion != "" {
			return "Sincronizando Context7 (documentación de librerías)…"
		}
		return "Registrando Context7 (documentación de librerías)…"
	case installer.StepActionWriteClaudeManagedBlock:
		return "Actualizando CLAUDE.md…"
	case installer.StepActionRegisterMemoryGuard:
		if options.engramVersion != "" {
			return "Re-registrando memory-guard…"
		}
		return "Registrando memory-guard…"
	case installer.StepActionSaveModels:
		return "Guardando modelos por fase de click-sdd…"
	case installer.StepActionSyncCodexGuidance:
		return "Actualizando AGENTS.md de Codex…"
	case installer.StepActionConfigureCodexNativeModel:
		return "Configurando modelo nativo de Codex si fue seleccionado explícitamente…"
	case installer.StepActionSyncOpenClawWorkspace:
		return "Actualizando AGENTS.md y SOUL.md de OpenClaw…"
	case installer.StepActionSyncOpenClawMCP:
		return "Registrando Engram en OpenClaw (mcpServers)…"
	case installer.StepActionSyncOpenClawPlugin:
		return "Instalando plugin de memory-guard para OpenClaw…"
	case installer.StepActionSyncOpenClawSkills:
		return "Sincronizando skills de Click en OpenClaw…"
	case installer.StepActionSyncOpenClawModelProfile:
		if options.engramVersion != "" {
			return "Guardando recomendación de modelos para OpenClaw…"
		}
		return "Configurando modelo nativo de OpenClaw mediante su CLI…"
	default:
		return string(kind)
	}
}

// renderWritePlan prints steps as a numbered plan to out, plus where the run-start snapshot backing
// them up will land (cfg.BackupDir()/latest) — the spec's install-preview "files touched, order"
// requirement — so a developer sees the full plan before confirmProceed asks them to continue.
func renderWritePlan(out io.Writer, r *ui.Renderer, cfg installer.Config, plan installer.TargetPlan, steps []string) {
	backupTargets := plan.SnapshotPaths()
	if len(backupTargets) == 0 {
		backupTargets = []string{"sin archivos seleccionados"}
	}
	fmt.Fprintln(out, r.Info(fmt.Sprintf(
		"Se tomará un respaldo de %s en %s antes de continuar.",
		strings.Join(backupTargets, ", "),
		filepath.Join(cfg.BackupDir(), "latest"),
	)))
	if cfg.OpenClawHome != "" {
		fmt.Fprintln(out, r.Info(fmt.Sprintf("OpenClaw recibe configuración nativa mediante su CLI; los metadatos locales se guardan en %s.", cfg.OpenClawModelProfilePath())))
	}
	if cfg.CodexHome != "" {
		fmt.Fprintln(out, r.Info(fmt.Sprintf("Codex recibe orientación en %s; config.toml sólo cambia con selección explícita.", cfg.CodexAgentsMDPath())))
	}
	if _, err := os.Stat(cfg.TargetSelectionPath()); err == nil {
		fmt.Fprintln(out, r.Info(fmt.Sprintf("La selección persistente de runtimes se conserva en %s.", cfg.TargetSelectionPath())))
	}
	fmt.Fprintln(out, r.Info("Se aplicarán los siguientes cambios, en este orden:"))
	for i, step := range steps {
		fmt.Fprintf(out, "  %d. %s\n", i+1, step)
	}
}

// confirmProceed prints a y/n prompt to out and reads a single line from in. Default-deny: only an
// explicit "y"/"yes" (case-insensitive, surrounding whitespace trimmed) proceeds — a bare newline,
// anything else, or immediate EOF (empty input) all decline, matching this codebase's conservative
// "never assume yes" posture (e.g. manageBackups' flag-gated destructive actions never default to
// proceeding either).
//
// bufio.Reader.ReadString stops at "\n" OR returns io.EOF with whatever partial line it already
// read — both are valid answers here (a real terminal always sends the trailing newline; a piped
// bytes.Buffer in tests may not), so io.EOF is deliberately NOT treated as an error, only as "no
// more input after this line".
func confirmProceed(in io.Reader, out io.Writer, r *ui.Renderer) (bool, error) {
	fmt.Fprint(out, r.Info("¿Continuar? [y/N]: "))
	reader := bufio.NewReader(in)
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return false, fmt.Errorf("cli: leer confirmación: %w", err)
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes", nil
}

// confirmAndSnapshot is the shared preview+confirm+snapshot gate both runInstall and runUpdate wire
// in, right after their own preflights/loads and before their first write step (design's "Snapshot
// timing" constraint: the run-start snapshot must be taken after confirm and before step 1 / any
// external `claude` subprocess invocation, in both commands).
//
// When nonInteractive is true (isNonInteractiveInstall — --yes/--non-interactive/non-TTY out), the
// plan and prompt are skipped entirely and the snapshot is taken immediately (spec's "--yes bypass"
// and "non-TTY / CI environment" scenarios). Otherwise the plan is shown and confirmProceed is
// asked; a decline returns (false, nil) with NO snapshot taken. installer.SnapshotRun is itself the
// very first write in the whole install/update write chain, so gating it behind proceed is
// sufficient to guarantee zero writes on decline — the caller only needs to check proceed and
// return before running any of its own write steps.
func confirmAndSnapshot(cmd *cobra.Command, out io.Writer, r *ui.Renderer, cfg installer.Config, plan installer.TargetPlan, nonInteractive bool, steps []string) (bool, error) {
	if !nonInteractive {
		renderWritePlan(out, r, cfg, plan, steps)
		proceed, err := confirmProceed(cmd.InOrStdin(), out, r)
		if err != nil {
			return false, err
		}
		if !proceed {
			return false, nil
		}
	}
	if err := installer.SnapshotTargetPlan(cfg, plan); err != nil {
		return false, err
	}
	return true, nil
}
