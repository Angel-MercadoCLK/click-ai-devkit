package cli

import (
	"bufio"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/installer"
	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/ui"
	"github.com/spf13/cobra"
)

// installWriteSteps is runInstall's fixed, ordered list of write steps — reused VERBATIM (same
// Spanish labels) from its own r.RunStep calls, so the preview plan renderWritePlan shows can never
// drift from what install.go actually runs (spec's install-preview capability: "the plan MUST be
// shown ... files touched, order").
var installWriteSteps = []string{
	"Registrando plugins click-sdd, click-memory, click-review y click-skills…",
	"Instalando Engram (memoria persistente)…",
	"Registrando Context7 (documentación de librerías)…",
	"Actualizando CLAUDE.md…",
	"Registrando memory-guard…",
	"Guardando modelos por fase de click-sdd…",
}

// updateWriteSteps builds runUpdate's fixed, ordered write-step list, reusing its own r.RunStep
// labels verbatim — including the Engram pin version (engramVersion), matching the exact label
// runUpdate itself prints later for that step — so the preview plan can never drift from what
// update.go actually runs.
func updateWriteSteps(engramVersion string) []string {
	return []string{
		"Re-sincronizando plugins click-sdd, click-memory, click-review y click-skills…",
		"Guardando modelos por fase de click-sdd…",
		"Actualizando CLAUDE.md…",
		"Re-registrando memory-guard…",
		fmt.Sprintf("Sincronizando Engram (pin %s)…", engramVersion),
		"Sincronizando Context7 (documentación de librerías)…",
	}
}

// renderWritePlan prints steps as a numbered plan to out, plus where the run-start snapshot backing
// them up will land (cfg.BackupDir()/latest) — the spec's install-preview "files touched, order"
// requirement — so a developer sees the full plan before confirmProceed asks them to continue.
func renderWritePlan(out io.Writer, r *ui.Renderer, cfg installer.Config, steps []string) {
	fmt.Fprintln(out, r.Info(fmt.Sprintf(
		"Se tomará un respaldo de CLAUDE.md y settings.json en %s antes de continuar.",
		filepath.Join(cfg.BackupDir(), "latest"),
	)))
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
