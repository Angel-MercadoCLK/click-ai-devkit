package cli

import (
	"fmt"
	"io"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-isatty"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/installer"
	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/manifest"
	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/modelconfig"
	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/ui"
	"github.com/spf13/cobra"
)

// yesFlag / nonInteractiveFlag both skip click install's interactive model-selection TUI and
// install click-sdd with D25's default per-phase models. Two names are accepted (--yes is the
// short everyday form, --non-interactive is explicit for CI/scripts) but they mean the same thing.
const (
	yesFlag            = "yes"
	nonInteractiveFlag = "non-interactive"
)

func newInstallCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install click-ai-devkit's plugins, CLAUDE.md block, and memory-guard hook into Claude Code",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInstall(cmd)
		},
	}
	cmd.Flags().Bool(yesFlag, false, "Skip the interactive model-selection screen; install click-sdd with default per-phase models")
	cmd.Flags().Bool(nonInteractiveFlag, false, "Alias for --yes")
	return cmd
}

func runInstall(cmd *cobra.Command) error {
	out := cmd.OutOrStdout()
	r := rendererFor(cmd, out)

	fmt.Fprintln(out, r.Banner())

	claudeHome, err := installer.ResolveClaudeHome()
	if err != nil {
		return err
	}
	cfg := installer.Config{ClaudeHome: claudeHome}

	models, cancelled, err := resolveInstallModels(cmd, out, r, cfg, isNonInteractiveInstall(cmd, out), runModelSelectTUI)
	if err != nil {
		return err
	}
	if cancelled {
		return nil
	}

	if err := r.RunStep("Registrando plugins click-sdd, click-memory y click-review…", "Plugins registrados en Claude Code", func() error {
		return installer.SyncMarketplacePlugins(models)
	}); err != nil {
		return err
	}

	m, err := manifest.Load()
	if err != nil {
		return err
	}
	engramAlreadyInstalled := false
	if err := r.RunStep("Instalando Engram (memoria persistente)…", "Engram sincronizado", func() error {
		var syncErr error
		engramAlreadyInstalled, syncErr = installer.SyncEngram(cfg, m)
		return syncErr
	}); err != nil {
		return err
	}
	if engramAlreadyInstalled {
		fmt.Fprintln(out, r.Info("Engram ya estaba instalado — se dejó como está, sin reinstalar."))
	}

	context7AlreadyPresent := false
	if err := r.RunStep("Registrando Context7 (documentación de librerías)…", "Context7 sincronizado", func() error {
		var syncErr error
		context7AlreadyPresent, syncErr = installer.SyncContext7(cfg)
		return syncErr
	}); err != nil {
		return err
	}
	if context7AlreadyPresent {
		fmt.Fprintln(out, r.Info("Context7 ya estaba configurado — se dejó como está, sin reinstalar."))
	}
	// SyncEngram's own EnsureEngramBinary step (Slice 3b) already attempted a `go install` when the
	// binary was missing and Go was available; this just reports the resulting state to the
	// developer. It never fails the install — a missing binary/toolchain is surfaced, not fatal.
	if _, resolvable, err := installer.EngramBinaryResolvable(cfg); err != nil {
		return err
	} else if !resolvable {
		fmt.Fprintln(out, r.Info(installer.EngramBinaryRemediationMessage(m.Engram.Version)))
	}

	if err := r.RunStep("Actualizando CLAUDE.md…", "CLAUDE.md actualizado", func() error {
		return installer.WriteManagedBlock(cfg.ClaudeMDPath(), installer.DefaultManagedContent)
	}); err != nil {
		return err
	}

	if err := r.RunStep("Registrando memory-guard…", "memory-guard registrado", func() error {
		return installer.RegisterMemoryGuardHook(cfg)
	}); err != nil {
		return err
	}

	if err := r.RunStep("Guardando modelos por fase de click-sdd…", "Modelos por fase guardados", func() error {
		return installer.SaveModels(cfg, models)
	}); err != nil {
		return err
	}

	fmt.Fprintln(out, r.Info("Instalación completa."))
	return nil
}

// isNonInteractiveInstall decides whether click install should skip the TUI and go straight to
// defaults: true when --yes/--non-interactive was passed, OR when out isn't a real terminal.
// The TTY check mirrors ui.shouldUseColor's own pattern (type-assert *os.File, then isatty) so
// piped output, CI runs, and `go test`'s bytes.Buffer all fall back automatically without a flag.
func isNonInteractiveInstall(cmd *cobra.Command, out io.Writer) bool {
	if yes, _ := cmd.Flags().GetBool(yesFlag); yes {
		return true
	}
	if nonInteractive, _ := cmd.Flags().GetBool(nonInteractiveFlag); nonInteractive {
		return true
	}
	f, ok := out.(*os.File)
	if !ok {
		return true
	}
	return !(isatty.IsTerminal(f.Fd()) || isatty.IsCygwinTerminal(f.Fd()))
}

// modelSelector matches runModelSelectTUI's signature so resolveInstallModels can be driven by a
// fake selector in tests (a real bubbletea program can't be exercised headlessly).
type modelSelector func(cmd *cobra.Command) (map[modelconfig.Phase]string, bool, error)

// resolveInstallModels decides the per-phase model set for `click install` and performs the D8
// stale-migration safety net at the correct point in the flow.
//
// Cancel must mean "no changes": if the developer cancels the interactive TUI, models.json must be
// left byte-for-byte untouched, so MigrateIfStale only runs once we know the install is actually
// proceeding — non-interactive installs always proceed, and interactive installs only proceed past
// the cancel check. Both proceeding paths (interactive-confirmed and non-interactive) still migrate
// before the fresh models get written, preserving the existing "never clobber without a backup"
// behavior.
func resolveInstallModels(cmd *cobra.Command, out io.Writer, r *ui.Renderer, cfg installer.Config, nonInteractive bool, selector modelSelector) (models map[modelconfig.Phase]string, cancelled bool, err error) {
	if nonInteractive {
		if _, err := installer.MigrateIfStale(cfg); err != nil {
			return nil, false, err
		}
		return modelconfig.Defaults(), false, nil
	}

	selection, cancelled, err := selector(cmd)
	if err != nil {
		return nil, false, err
	}
	if cancelled {
		fmt.Fprintln(out, r.Info("Instalación cancelada."))
		return nil, true, nil
	}

	if _, err := installer.MigrateIfStale(cfg); err != nil {
		return nil, false, err
	}
	return selection, false, nil
}

// runModelSelectTUI drives ui.ModelSelectModel through a real bubbletea program attached to cmd's
// in/out, and returns the developer's final per-phase selection. Only reached when
// isNonInteractiveInstall has already confirmed out is a real terminal.
func runModelSelectTUI(cmd *cobra.Command) (map[modelconfig.Phase]string, bool, error) {
	program := tea.NewProgram(ui.NewModelSelectModel(),
		tea.WithInput(cmd.InOrStdin()),
		tea.WithOutput(cmd.OutOrStdout()),
	)
	finalModel, err := program.Run()
	if err != nil {
		return nil, false, fmt.Errorf("cli: run model selection TUI: %w", err)
	}
	result := finalModel.(ui.ModelSelectModel)
	if result.Cancelled {
		return nil, true, nil
	}
	return result.Selection, false, nil
}
