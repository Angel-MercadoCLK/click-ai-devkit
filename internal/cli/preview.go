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

// installWriteSteps builds runInstall's ordered list of write steps for cfg — reused VERBATIM
// (same Spanish labels) from its own r.RunStep calls, so the preview plan renderWritePlan shows
// can never drift from what install.go actually runs (spec's install-preview capability: "the plan
// MUST be shown ... files touched, order"). The trailing OpenClaw steps (openClawWriteSteps) are
// appended only when cfg.OpenClawHome is populated — a Claude-only host (cfg.OpenClawHome == "")
// gets the exact same 6-step list this returned before OpenClaw support existed. The Engram Cloud
// step is inserted right after the local Engram step only when cloudConfigured is true.
func installWriteSteps(cfg installer.Config, cloudConfigured bool) []string {
	steps := []string{
		"Registrando plugins click-sdd, click-memory, click-review y click-skills…",
		"Instalando Engram (memoria persistente)…",
	}
	if cloudConfigured {
		steps = append(steps, "Enrolando Engram Cloud…")
	}
	steps = append(steps,
		"Registrando Context7 (documentación de librerías)…",
		"Actualizando CLAUDE.md…",
		"Registrando memory-guard…",
		"Guardando modelos por fase de click-sdd…",
	)
	steps = append(steps, openClawWriteSteps(cfg)...)
	return append(steps, codexWriteSteps(cfg)...)
}

// updateWriteSteps builds runUpdate's ordered write-step list for cfg, reusing its own r.RunStep
// labels verbatim — including the Engram pin version (engramVersion), matching the exact label
// runUpdate itself prints later for that step — so the preview plan can never drift from what
// update.go actually runs. Same OpenClaw-appending contract as installWriteSteps. The Engram Cloud
// step is inserted right after the local Engram pin step only when cloudConfigured is true.
func updateWriteSteps(engramVersion string, cfg installer.Config, cloudConfigured bool) []string {
	steps := []string{
		"Re-sincronizando plugins click-sdd, click-memory, click-review y click-skills…",
		"Guardando modelos por fase de click-sdd…",
		"Actualizando CLAUDE.md…",
		"Re-registrando memory-guard…",
		fmt.Sprintf("Sincronizando Engram (pin %s)…", engramVersion),
	}
	if cloudConfigured {
		steps = append(steps, "Sincronizando Engram Cloud…")
	}
	steps = append(steps, "Sincronizando Context7 (documentación de librerías)…")
	steps = append(steps, openClawWriteSteps(cfg)...)
	return append(steps, codexWriteSteps(cfg)...)
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

func codexWriteSteps(cfg installer.Config) []string {
	if cfg.CodexHome == "" {
		return nil
	}
	return []string{"Actualizando AGENTS.md de Codex…"}
}

// renderWritePlan prints steps as a numbered plan to out, plus where the run-start snapshot backing
// them up will land (cfg.BackupDir()/latest) — the spec's install-preview "files touched, order"
// requirement — so a developer sees the full plan before confirmProceed asks them to continue.
func renderWritePlan(out io.Writer, r *ui.Renderer, cfg installer.Config, steps []string) {
	fmt.Fprintln(out, r.Info(fmt.Sprintf(
		"Se tomará un respaldo de CLAUDE.md y settings.json en %s antes de continuar.",
		filepath.Join(cfg.BackupDir(), "latest"),
	)))
	if cfg.OpenClawHome != "" {
		fmt.Fprintln(out, r.Info(fmt.Sprintf("La recomendación portable de modelos para OpenClaw se guardará en %s; no modifica la configuración nativa de OpenClaw.", cfg.OpenClawModelProfilePath())))
	}
	if cfg.CodexHome != "" {
		fmt.Fprintln(out, r.Info(fmt.Sprintf("Codex sólo recibirá orientación gestionada en %s; no se modifica config.toml ni el modelo.", cfg.CodexAgentsMDPath())))
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
func confirmAndSnapshot(cmd *cobra.Command, out io.Writer, r *ui.Renderer, cfg installer.Config, nonInteractive bool, steps []string) (bool, error) {
	if !nonInteractive {
		renderWritePlan(out, r, cfg, steps)
		proceed, err := confirmProceed(cmd.InOrStdin(), out, r)
		if err != nil {
			return false, err
		}
		if !proceed {
			return false, nil
		}
	}
	if err := installer.SnapshotRun(cfg); err != nil {
		return false, err
	}
	return true, nil
}
