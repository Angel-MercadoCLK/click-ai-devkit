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
//
// profileFlag lets a non-interactive/scripted install pick a built-in orchestration profile
// (design D4) without going through the interactive profile-select TUI; an empty or unrecognized
// value falls back to "balanced" (modelconfig.ResolveProfile's own fallback rule).
//
// skipOpenClawFlag is openclaw-target-support's escape hatch: even when `openclaw` resolves on
// PATH, --skip-openclaw forces a Claude-only install/update, matching the spec's "no per-target
// wizard" decision with a single explicit opt-out instead.
const (
	yesFlag            = "yes"
	nonInteractiveFlag = "non-interactive"
	profileFlag        = "profile"
	skipOpenClawFlag   = "skip-openclaw"
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
	cmd.Flags().String(profileFlag, "", "Perfil de orquestación a usar (balanced/cost-saver/quality); default balanced. En instalación no interactiva selecciona el preset directamente; en la interactiva sólo precarga el editor por fase inicial.")
	cmd.Flags().Bool(skipOpenClawFlag, false, "Omitir la integración con OpenClaw aunque se detecte openclaw en este equipo")
	return cmd
}

func runInstall(cmd *cobra.Command) error {
	out := cmd.OutOrStdout()
	r := rendererFor(cmd, out)

	fmt.Fprintln(out, r.Banner())

	// PreflightClaude must run before PreflightGit and before anything else, including the
	// interactive model-selection TUI below. claude is the more fundamental dependency here: click
	// registers every plugin via the claude CLI itself (plugins.go's pluginCLIBinary), so a machine
	// missing Claude Code entirely should fail on that actionable message first, not on git's.
	if err := installer.PreflightClaude(); err != nil {
		return err
	}

	// PreflightGit must run before anything else — including the interactive model-selection TUI
	// below. `click install` registers the plugin marketplace via `claude plugin marketplace add`,
	// which shells out to `git clone` under the hood; on a machine with no git on PATH that clone
	// used to fail deep inside plugin registration with a cryptic error, well after the developer
	// had already gone through the TUI (reproduced live on a fresh Windows VM). Failing fast here
	// means a developer on a fresh machine finds out instantly, not after an interactive detour.
	if err := installer.PreflightGit(); err != nil {
		return err
	}

	claudeHome, err := installer.ResolveClaudeHome()
	if err != nil {
		return err
	}
	cfg := installer.Config{ClaudeHome: claudeHome}

	// openclaw-target-support: detect+confirm, no per-target wizard. OpenClawHome stays empty
	// (zero value) whenever --skip-openclaw is set OR openclaw doesn't resolve on PATH — every
	// OpenClaw-aware helper below (installWriteSteps, SyncOpenClawWorkspace, SyncOpenClawMCPConfig,
	// snapshotSources) treats an empty OpenClawHome as "absent, skip silently", so this single
	// assignment is the ONLY place that decides whether OpenClaw is in scope for this run.
	skipOpenClaw, err := cmd.Flags().GetBool(skipOpenClawFlag)
	if err != nil {
		return err
	}
	if !skipOpenClaw && installer.OpenClawAvailable() {
		openClawHome, err := installer.ResolveOpenClawHome()
		if err != nil {
			return err
		}
		cfg.OpenClawHome = openClawHome
	}

	nonInteractive := isNonInteractiveInstall(cmd, out)
	profileFlagValue, _ := cmd.Flags().GetString(profileFlag)
	profile, models, cancelled, err := resolveInstallModels(cmd, out, r, cfg, nonInteractive, profileFlagValue, runInstallSelectTUI)
	if err != nil {
		return err
	}
	if cancelled {
		return nil
	}

	m, err := manifest.Load()
	if err != nil {
		return err
	}
	cloudConfigured := installer.EngramCloudConfigured(cfg, m)

	// install-preview/install-backup (spec): show the write plan and ask for confirmation unless
	// --yes/--non-interactive/non-TTY says to skip straight through, then take the run-start
	// snapshot — all BEFORE step 1 below (the first external `claude` subprocess invocation). A
	// decline here means zero writes: nothing below this point has run yet.
	proceed, err := confirmAndSnapshot(cmd, out, r, cfg, nonInteractive, installWriteSteps(cfg, cloudConfigured))
	if err != nil {
		return err
	}
	if !proceed {
		fmt.Fprintln(out, r.Info("Instalación cancelada."))
		return nil
	}

	if err := r.RunStep("Registrando plugins click-sdd, click-memory, click-review y click-skills…", "Plugins registrados en Claude Code", func() error {
		return installer.SyncMarketplacePlugins(models, profile)
	}); err != nil {
		return err
	}

	engramAlreadyInstalled := false
	engramPathWarning := ""
	if err := r.RunStep("Instalando Engram (memoria persistente)…", "Engram sincronizado", func() error {
		var syncErr error
		engramAlreadyInstalled, engramPathWarning, syncErr = installer.SyncEngram(cfg, m)
		return syncErr
	}); err != nil {
		return err
	}
	if engramAlreadyInstalled {
		fmt.Fprintln(out, r.Info("Engram ya estaba instalado — se dejó como está, sin reinstalar."))
	}
	surfacePathWarning(out, r, engramPathWarning)

	if installer.EngramCloudPartiallyConfigured(cfg, m) {
		reportSkippedCloudEnrollment(out, r)
	} else if cloudConfigured {
		if err := r.RunStep("Enrolando Engram Cloud…", "Engram Cloud enrolado", func() error {
			return syncEngramCloudFunc(cfg, m)
		}); err != nil {
			return err
		}
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
		return installer.SaveModelsWithProfile(cfg, profile, models)
	}); err != nil {
		return err
	}

	// openclaw-target-support: appended LAST, matching openClawWriteSteps' position at the end of
	// installWriteSteps(cfg) — this whole block (3 RunStep calls: AGENTS.md/SOUL.md, MCP
	// registration, and the memory-guard plugin) is gated on the same condition openClawWriteSteps
	// used to decide whether to list them in the preview at all.
	if cfg.OpenClawHome != "" {
		if err := r.RunStep("Actualizando AGENTS.md y SOUL.md de OpenClaw…", "AGENTS.md y SOUL.md de OpenClaw actualizados", func() error {
			return installer.SyncOpenClawWorkspace(cfg)
		}); err != nil {
			return err
		}
		if err := r.RunStep("Registrando Engram en OpenClaw (mcpServers)…", "Engram registrado en OpenClaw", func() error {
			return installer.SyncOpenClawMCPConfig(cfg)
		}); err != nil {
			return err
		}
		// PR-C (design #1666's memory-guard-parity piece, OCG-1..6): installs the click-memory-guard
		// OpenClaw plugin last, after both OpenClaw writes above, matching openClawWriteSteps'
		// position for it in the preview plan.
		if err := r.RunStep("Instalando plugin de memory-guard para OpenClaw…", "Plugin de memory-guard instalado en OpenClaw", func() error {
			return installer.SyncOpenClawPlugin(cfg)
		}); err != nil {
			return err
		}
		// PR4: synchronize the click-owned OpenClaw skill manifests (clickhola, clickdev) after the
		// plugin install, matching openClawWriteSteps' position for the skill sync step.
		if err := r.RunStep("Sincronizando skills clickhola y clickdev en OpenClaw…", "Skills clickhola y clickdev sincronizados en OpenClaw", func() error {
			return installer.SyncOpenClawSkills(cfg)
		}); err != nil {
			return err
		}
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

// installSelector drives the two-step interactive flow (profile-select, then the per-phase editor
// seeded from that profile) and matches runInstallSelectTUI's signature so resolveInstallModels can
// be driven by a fake selector in tests (a real bubbletea program can't be exercised headlessly).
// initialProfile (C2 fix) is the profile the picker's cursor should start on — resolveInstallModels
// derives it from --profile so the interactive picker actually honors the flag's own help text
// instead of always hardcoding balanced.
type installSelector func(cmd *cobra.Command, initialProfile modelconfig.ProfileName) (profile modelconfig.ProfileName, models map[modelconfig.Phase]string, cancelled bool, err error)

// resolveInstallModels decides the active orchestration profile AND the per-phase model set for
// `click install`, and performs the D8 stale-migration safety net at the correct point in the flow.
//
// Cancel must mean "no changes": if the developer cancels either interactive step, models.json must
// be left byte-for-byte untouched, so MigrateIfStale only runs once we know the install is actually
// proceeding — non-interactive installs always proceed, and interactive installs only proceed past
// the cancel check. Both proceeding paths (interactive-confirmed and non-interactive) still migrate
// before the fresh models get written, preserving the existing "never clobber without a backup"
// behavior.
//
// CRITICAL non-TTY/CI safety (carried over unchanged from before profiles existed): the
// non-interactive branch below NEVER calls selector — it resolves synchronously from profileFlagValue
// (default/unknown -> "balanced") with no prompt, so a non-TTY `click install` can never hang.
//
// The label actually returned is resolved via modelconfig.EffectiveProfileName for the interactive
// path: a preset the developer left untouched keeps its own name, but a hand-tweaked preset (or an
// explicit "custom" pick) downgrades to "custom" — the persisted `profile` field must never claim a
// preset name the final per-phase map no longer matches. The non-interactive path never needs this
// downgrade: modelconfig.ResolveProfile(profileFlagValue).Models is always emitted verbatim, with no
// per-phase editor step to diverge from it.
//
// C2 fix: the interactive path also passes profileFlagValue through to selector as the picker's
// initial profile, so `click install --profile X` on a real terminal preloads the profile-select
// screen's cursor on X (matching the flag's own help text) instead of always hardcoding balanced. An
// empty or unrecognized value falls back to balanced, same as the non-interactive path.
func resolveInstallModels(cmd *cobra.Command, out io.Writer, r *ui.Renderer, cfg installer.Config, nonInteractive bool, profileFlagValue string, selector installSelector) (profile modelconfig.ProfileName, models map[modelconfig.Phase]string, cancelled bool, err error) {
	if nonInteractive {
		if _, err := installer.MigrateIfStale(cfg); err != nil {
			return "", nil, false, err
		}
		resolved := modelconfig.ResolveProfile(profileFlagValue)
		return resolved.Name, resolved.Models, false, nil
	}

	chosenProfile, selection, cancelled, err := selector(cmd, modelconfig.ProfileName(profileFlagValue))
	if err != nil {
		return "", nil, false, err
	}
	if cancelled {
		fmt.Fprintln(out, r.Info("Instalación cancelada."))
		return "", nil, true, nil
	}

	if _, err := installer.MigrateIfStale(cfg); err != nil {
		return "", nil, false, err
	}
	return modelconfig.EffectiveProfileName(chosenProfile, selection), selection, false, nil
}

// runModelSelectTUI drives ui.ModelSelectModel (with no profile seeding) through a real bubbletea
// program attached to cmd's in/out, and returns the developer's final per-phase selection. Kept
// standalone (not folded into runInstallSelectTUI) because configuremodels.go's "Configure models"
// menu entry reuses exactly this single-step screen — it deliberately has no profile-select step.
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

// runInstallSelectTUI drives ui.ProfileSelectModel (seeded on initialProfile — C2 fix) then
// ui.ModelSelectModel (seeded from the chosen profile, or from Defaults() when "custom" was picked)
// through real bubbletea programs attached to cmd's in/out, and returns the developer's final
// profile + per-phase selection. Only reached when isNonInteractiveInstall has already confirmed
// out is a real terminal.
func runInstallSelectTUI(cmd *cobra.Command, initialProfile modelconfig.ProfileName) (modelconfig.ProfileName, map[modelconfig.Phase]string, bool, error) {
	profileProgram := tea.NewProgram(ui.NewProfileSelectModelForProfile(initialProfile),
		tea.WithInput(cmd.InOrStdin()),
		tea.WithOutput(cmd.OutOrStdout()),
	)
	finalProfile, err := profileProgram.Run()
	if err != nil {
		return "", nil, false, fmt.Errorf("cli: run profile selection TUI: %w", err)
	}
	profileResult := finalProfile.(ui.ProfileSelectModel)
	if profileResult.Cancelled {
		return "", nil, true, nil
	}

	seed := ui.NewModelSelectModel()
	if profileResult.Selected != modelconfig.ProfileCustom {
		seed = ui.NewModelSelectModelForProfile(profileResult.Selected)
	}
	modelProgram := tea.NewProgram(seed,
		tea.WithInput(cmd.InOrStdin()),
		tea.WithOutput(cmd.OutOrStdout()),
	)
	finalModel, err := modelProgram.Run()
	if err != nil {
		return "", nil, false, fmt.Errorf("cli: run model selection TUI: %w", err)
	}
	modelResult := finalModel.(ui.ModelSelectModel)
	if modelResult.Cancelled {
		return "", nil, true, nil
	}
	return profileResult.Selected, modelResult.Selection, false, nil
}
