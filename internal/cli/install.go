package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-isatty"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/installer"
	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/manifest"
	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/modelconfig"
	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/ui"
	"github.com/spf13/cobra"
)

// yesFlag / nonInteractiveFlag both select detected targets and skip every TUI. They use explicit
// safe defaults and preserve an existing Codex model unless the user supplies --codex-model.
//
// profileFlag lets a non-interactive/scripted install pick a built-in orchestration profile
// (design D4) without going through the interactive profile-select TUI; an empty or unrecognized
// value falls back to "balanced" (modelconfig.ResolveProfile's own fallback rule).
//
// skipOpenClawFlag is the explicit escape hatch for omitting a detected OpenClaw target.
const (
	yesFlag              = "yes"
	nonInteractiveFlag   = "non-interactive"
	profileFlag          = "profile"
	skipOpenClawFlag     = "skip-openclaw"
	codexModelFlag       = "codex-model"
	openClawModelFlag    = "openclaw-model"
	openClawFallbackFlag = "openclaw-fallback-model"
)

func newInstallCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Instala Click en los runtimes detectados y seleccionados",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInstall(cmd)
		},
	}
	cmd.Flags().Bool(yesFlag, false, "Skip the interactive model-selection screen; install click-sdd with default per-phase models")
	cmd.Flags().Bool(nonInteractiveFlag, false, "Alias for --yes")
	cmd.Flags().String(profileFlag, "", "Perfil de orquestación a usar (balanced/cost-saver/quality); default balanced. En instalación no interactiva selecciona el preset directamente; en la interactiva sólo precarga el editor por fase inicial.")
	cmd.Flags().Bool(skipOpenClawFlag, false, "Omitir la integración con OpenClaw aunque se detecte openclaw en este equipo")
	cmd.Flags().String(codexModelFlag, "", "Referencia de modelo nativa de Codex, por ejemplo gpt-5.6")
	cmd.Flags().String(openClawModelFlag, "", "Referencia provider/model nativa de OpenClaw")
	cmd.Flags().StringSlice(openClawFallbackFlag, nil, "Referencias provider/model alternativas de OpenClaw")
	return cmd
}

func runInstall(cmd *cobra.Command) error {
	out := cmd.OutOrStdout()
	r := rendererFor(cmd, out)

	fmt.Fprintln(out, r.Banner())

	clickStateHome, err := installer.ResolveClickStateHome()
	if err != nil {
		return err
	}
	cfg := installer.Config{ClickStateHome: clickStateHome}

	skipOpenClaw, err := cmd.Flags().GetBool(skipOpenClawFlag)
	if err != nil {
		return err
	}
	selection, err := resolveInstallTargetSelection(cmd, skipOpenClaw, out, r)
	if err != nil {
		return err
	}
	if !selection.Claude && !selection.Codex && !selection.OpenClaw {
		fmt.Fprintln(out, r.Warn("No se detectó ningún runtime seleccionable. Claude Code habilita plugins nativos; Codex y OpenClaw ofrecen el flujo portable. Instale o habilite un runtime y vuelva a ejecutar `click install`."))
		fmt.Fprintln(out, r.Info(installer.ClaudeMissingMessage))
		return fmt.Errorf("%s", installer.ClaudeMissingMessage)
	}
	if selection.Claude {
		claudeHome, resolveErr := installer.ResolveClaudeHome()
		if resolveErr != nil {
			return resolveErr
		}
		cfg.ClaudeHome = claudeHome
		if err := installer.PreflightClaude(); err != nil {
			return err
		}
		if err := installer.PreflightGit(); err != nil {
			return err
		}
	}
	if selection.OpenClaw {
		cfg.OpenClawHome, err = installer.ResolveOpenClawHome()
		if err != nil {
			return err
		}
	}
	if selection.Codex {
		cfg.CodexHome, err = installer.ResolveCodexHome()
		if err != nil {
			return err
		}
	}
	nonInteractive := isNonInteractiveInstall(cmd, out)
	profile := modelconfig.ProfileName("")
	var models map[modelconfig.Phase]string
	if selection.Claude {
		profileFlagValue, _ := cmd.Flags().GetString(profileFlag)
		var cancelled bool
		profile, models, cancelled, err = resolveInstallModels(cmd, out, r, cfg, nonInteractive, profileFlagValue, runInstallSelectTUI)
		if err != nil {
			return err
		}
		if cancelled {
			return nil
		}
	}
	native, cancelled, err := resolveNativeModels(cmd, selection, nonInteractive, out, r, runNativeModelConfigTUI)
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
	plan := installer.BuildTargetPlan(cfg, selection, installer.PlanOptions{CloudConfigured: cloudConfigured || installer.EngramCloudPartiallyConfigured(cfg, m)})
	fmt.Fprintln(out, r.Info("Capacidades seleccionadas: "+plan.CapabilitiesSummary()))
	fmt.Fprintln(out, r.Info("Resumen final de instalación: "+strings.Join(plan.StepLabels(), " → ")+nativeSummary(native)))

	// install-preview/install-backup (spec): show the write plan and ask for confirmation unless
	// --yes/--non-interactive/non-TTY says to skip straight through, then take the run-start
	// snapshot — all BEFORE step 1 below (the first external `claude` subprocess invocation). A
	// decline here means zero writes: nothing below this point has run yet.
	proceed, err := confirmAndSnapshot(cmd, out, r, cfg, plan, nonInteractive, installWriteStepsForSelection(cfg, cloudConfigured, selection))
	if err != nil {
		return err
	}
	if !proceed {
		fmt.Fprintln(out, r.Info("Instalación cancelada."))
		return nil
	}
	if err := installer.SaveTargetSelection(cfg, selection); err != nil {
		return err
	}
	if _, err := installer.MigrateIfStale(cfg); err != nil {
		return err
	}

	for _, action := range plan.InstallActionKinds() {
		switch action {
		case installer.StepActionSyncMarketplacePlugins:
			if err := r.RunStep("Registrando plugins click-sdd, click-memory, click-review y click-skills…", "Plugins registrados en Claude Code", func() error {
				return installer.SyncMarketplacePlugins(models, profile)
			}); err != nil {
				return err
			}
		case installer.StepActionSyncEngram:
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
			if _, resolvable, err := installer.EngramBinaryResolvable(cfg); err != nil {
				return err
			} else if !resolvable {
				fmt.Fprintln(out, r.Info(installer.EngramBinaryRemediationMessage(m.Engram.Version)))
			}
		case installer.StepActionSyncEngramCloud:
			if installer.EngramCloudPartiallyConfigured(cfg, m) {
				reportSkippedCloudEnrollment(out, r)
				continue
			}
			fmt.Fprintln(out, r.Step("Enrolando Engram Cloud…"))
			if cloudErr := syncEngramCloudFunc(cfg, m); cloudErr != nil {
				fmt.Fprintln(out, r.Warn(fmt.Sprintf("No se pudo sincronizar Engram Cloud: %v. La instalación local continúa; reintenta más tarde con `click update`.", cloudErr)))
			} else {
				fmt.Fprintln(out, r.Success("Engram Cloud enrolado"))
			}
		case installer.StepActionSyncContext7:
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
		case installer.StepActionWriteClaudeManagedBlock:
			if err := r.RunStep("Actualizando CLAUDE.md…", "CLAUDE.md actualizado", func() error {
				return installer.WriteManagedBlock(cfg.ClaudeMDPath(), installer.DefaultManagedContent)
			}); err != nil {
				return err
			}
		case installer.StepActionRegisterMemoryGuard:
			if err := r.RunStep("Registrando memory-guard…", "memory-guard registrado", func() error {
				return installer.RegisterMemoryGuardHook(cfg)
			}); err != nil {
				return err
			}
		case installer.StepActionSaveModels:
			if err := r.RunStep("Guardando modelos por fase de click-sdd…", "Modelos por fase guardados", func() error {
				return installer.SaveModelsWithProfile(cfg, profile, models)
			}); err != nil {
				return err
			}
		case installer.StepActionSyncCodexGuidance:
			if err := r.RunStep("Actualizando AGENTS.md de Codex…", "AGENTS.md de Codex actualizado", func() error {
				return syncCodexGuidanceFunc(cfg)
			}); err != nil {
				return err
			}
		case installer.StepActionConfigureCodexNativeModel:
			if selection.Codex && native.Codex.Primary != "" {
				if err := installer.ConfigureCodexModel(cfg.CodexHome, native.Codex.Primary); err != nil {
					return err
				}
			}
		case installer.StepActionSyncOpenClawWorkspace:
			if err := r.RunStep("Actualizando AGENTS.md y SOUL.md de OpenClaw…", "AGENTS.md y SOUL.md de OpenClaw actualizados", func() error {
				return installer.SyncOpenClawWorkspace(cfg)
			}); err != nil {
				return err
			}
		case installer.StepActionSyncOpenClawMCP:
			if err := r.RunStep("Registrando Engram en OpenClaw (mcpServers)…", "Engram registrado en OpenClaw", func() error {
				return installer.SyncOpenClawMCPConfig(cfg)
			}); err != nil {
				return err
			}
		case installer.StepActionSyncOpenClawPlugin:
			if err := r.RunStep("Instalando plugin de memory-guard para OpenClaw…", "Plugin de memory-guard instalado en OpenClaw", func() error {
				return installer.SyncOpenClawPlugin(cfg)
			}); err != nil {
				return err
			}
		case installer.StepActionSyncOpenClawSkills:
			if err := r.RunStep("Sincronizando skills de Click en OpenClaw…", "Skills de Click sincronizados en OpenClaw", func() error {
				return installer.SyncOpenClawSkills(cfg)
			}); err != nil {
				return err
			}
		case installer.StepActionSyncOpenClawModelProfile:
			if err := installer.ConfigureOpenClawModels(native.OpenClaw.Primary, native.OpenClaw.Fallbacks); err != nil {
				return err
			}
			if err := r.RunStep("Guardando metadatos de modelos para OpenClaw…", "Metadatos de modelos guardados", func() error {
				return installer.SyncOpenClawModelProfile(cfg, profile, models)
			}); err != nil {
				return err
			}
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

var syncCodexGuidanceFunc = installer.SyncCodexGuidance

func SetSyncCodexGuidanceFuncForTests(fn func(installer.Config) error) func() {
	old := syncCodexGuidanceFunc
	syncCodexGuidanceFunc = fn
	return func() { syncCodexGuidanceFunc = old }
}

func resolveInstallTargetSelection(cmd *cobra.Command, skipOpenClaw bool, out io.Writer, r *ui.Renderer) (installer.TargetSelection, error) {
	claudeFound := installer.ClaudeAvailable()
	openClawFound := installer.OpenClawAvailable() && !skipOpenClaw
	codexFound := installer.CodexAvailable()
	selection := installer.TargetSelection{Configured: true, Claude: claudeFound, OpenClaw: openClawFound, Codex: codexFound}
	if isNonInteractiveInstall(cmd, out) {
		fmt.Fprintln(out, r.Info("Modo no interactivo: se seleccionan únicamente los runtimes detectados; no se inicia ninguna TUI."))
		return selection, nil
	}
	model := ui.NewTargetSelectModel(claudeFound, openClawFound, selection.Claude, selection.OpenClaw, codexFound, selection.Codex)
	program := tea.NewProgram(model, tea.WithInput(cmd.InOrStdin()), tea.WithOutput(out))
	final, err := program.Run()
	if err != nil {
		return installer.TargetSelection{}, fmt.Errorf("cli: ejecutar selección de runtimes: %w", err)
	}
	result := final.(ui.TargetSelectModel)
	if result.Cancelled {
		return installer.TargetSelection{}, nil
	}
	if !result.Confirmed {
		return installer.TargetSelection{}, nil
	}
	selection = installer.TargetSelection{Configured: true, Claude: result.Claude, OpenClaw: result.OpenClaw, Codex: result.Codex}
	return selection, nil
}

type nativeTargetConfig struct {
	Codex    ui.NativeModelConfig
	OpenClaw ui.NativeModelConfig
}

type nativeModelSelector func(cmd *cobra.Command, kind, primary string, fallbacks []string) (ui.NativeModelConfig, bool, error)

func resolveNativeModels(cmd *cobra.Command, selection installer.TargetSelection, nonInteractive bool, out io.Writer, r *ui.Renderer, selector nativeModelSelector) (nativeTargetConfig, bool, error) {
	var result nativeTargetConfig
	if selection.Codex {
		model, _ := cmd.Flags().GetString(codexModelFlag)
		var cancelled bool
		if nonInteractive {
			result.Codex = ui.NativeModelConfig{Primary: model, Confirmed: true}
		} else {
			var selectorErr error
			result.Codex, cancelled, selectorErr = selector(cmd, "Codex", model, nil)
			if selectorErr != nil {
				return nativeTargetConfig{}, false, selectorErr
			}
		}
		if cancelled {
			return nativeTargetConfig{}, true, nil
		}
		if result.Codex.Primary == "" {
			fmt.Fprintln(out, r.Info("Codex: se omite la configuración nativa porque el modelo no fue seleccionado explícitamente (--codex-model <model>)."))
		} else {
			fmt.Fprintln(out, r.Info("Codex: modelo nativo seleccionado: "+result.Codex.Primary+"."))
		}
	}
	if selection.OpenClaw {
		model, _ := cmd.Flags().GetString(openClawModelFlag)
		fallbacks, _ := cmd.Flags().GetStringSlice(openClawFallbackFlag)
		if model == "" && nonInteractive {
			model = "openai/gpt-5.6-sol"
			fmt.Fprintln(out, r.Info("OpenClaw: modo no interactivo usa el modelo nativo explícito openai/gpt-5.6-sol."))
		}
		var cancelled bool
		if nonInteractive {
			result.OpenClaw = ui.NativeModelConfig{Primary: model, Fallbacks: fallbacks, Confirmed: true}
		} else {
			var selectorErr error
			result.OpenClaw, cancelled, selectorErr = selector(cmd, "OpenClaw", model, fallbacks)
			if selectorErr != nil {
				return nativeTargetConfig{}, false, selectorErr
			}
		}
		if cancelled {
			return nativeTargetConfig{}, true, nil
		}
		if result.OpenClaw.Primary == "" {
			return nativeTargetConfig{}, false, fmt.Errorf("configuración de OpenClaw cancelada o incompleta")
		}
		fmt.Fprintln(out, r.Info("OpenClaw: referencia nativa seleccionada: "+result.OpenClaw.Primary+"; fallbacks: "+strings.Join(result.OpenClaw.Fallbacks, ", ")+"."))
	}
	return result, false, nil
}

func runNativeModelConfigTUI(cmd *cobra.Command, kind, primary string, fallbacks []string) (ui.NativeModelConfig, bool, error) {
	model := ui.NewNativeModelConfigModel(kind, primary, fallbacks)
	program := tea.NewProgram(model, tea.WithInput(cmd.InOrStdin()), tea.WithOutput(cmd.OutOrStdout()))
	final, err := program.Run()
	if err != nil {
		return ui.NativeModelConfig{}, false, fmt.Errorf("cli: ejecutar configuración nativa de %s: %w", kind, err)
	}
	result := final.(ui.NativeModelConfigModel)
	return result.Result(), result.Result().Cancelled, nil
}

func nativeSummary(config nativeTargetConfig) string {
	var parts []string
	if config.Codex.Primary != "" {
		parts = append(parts, fmt.Sprintf("; Codex=%s", config.Codex.Primary))
	}
	if config.OpenClaw.Primary != "" {
		parts = append(parts, fmt.Sprintf("; OpenClaw=%s (fallbacks: %s)", config.OpenClaw.Primary, strings.Join(config.OpenClaw.Fallbacks, ", ")))
	}
	return strings.Join(parts, "")
}

// installSelector drives the two-step interactive flow (profile-select, then the per-phase editor
// seeded from that profile) and matches runInstallSelectTUI's signature so resolveInstallModels can
// be driven by a fake selector in tests (a real bubbletea program can't be exercised headlessly).
// initialProfile (C2 fix) is the profile the picker's cursor should start on — resolveInstallModels
// derives it from --profile so the interactive picker actually honors the flag's own help text
// instead of always hardcoding balanced.
type installSelector func(cmd *cobra.Command, initialProfile modelconfig.ProfileName) (profile modelconfig.ProfileName, models map[modelconfig.Phase]string, cancelled bool, err error)

// resolveInstallModels decides the active orchestration profile and per-phase model set for
// `click install`. Filesystem migration is intentionally owned by runInstall after final confirm.
//
// Cancel must mean "no changes": if the developer cancels either interactive step, models.json must
// be left byte-for-byte untouched, so MigrateIfStale only runs once we know the install is actually
// proceeding — non-interactive installs always proceed, and interactive installs only proceed past
// the cancel check.
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
