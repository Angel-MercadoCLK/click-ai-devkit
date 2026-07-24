package cli

import (
	"fmt"
	"strings"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/installer"
	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/manifest"
	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/modelconfig"
	"github.com/spf13/cobra"
)

func newUpdateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Re-sync plugins and the Engram pin to the currently installed click binary",
		RunE:  runUpdate,
	}
	cmd.Flags().Bool(skipOpenClawFlag, false, "Omitir la integración con OpenClaw aunque se detecte openclaw en este equipo")
	cmd.Flags().String(codexModelFlag, "", "Referencia de modelo nativa de Codex, por ejemplo gpt-5.6")
	return cmd
}

func runUpdate(cmd *cobra.Command, args []string) error {
	out := cmd.OutOrStdout()
	r := rendererFor(cmd, out)

	clickStateHome, err := installer.ResolveClickStateHome()
	if err != nil {
		return err
	}
	cfg := installer.Config{ClickStateHome: clickStateHome}

	// openclaw-target-support: same detect+confirm rule as runInstall — see install.go's cfg
	// construction for the full rationale. Kept duplicated (not extracted into a shared helper)
	// because the two commands' surrounding flag/error plumbing differs enough that a shared helper
	// would need its own indirection for no real gain at this size.
	skipOpenClaw, err := cmd.Flags().GetBool(skipOpenClawFlag)
	if err != nil {
		return err
	}
	selection, cfg, err := resolveTargetConfig(cfg, skipOpenClaw, out, r)
	if err != nil {
		return err
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

	m, err := manifest.Load()
	if err != nil {
		return err
	}
	cloudConfigured := installer.EngramCloudConfigured(cfg, m)
	plan := installer.BuildTargetPlan(cfg, selection, installer.PlanOptions{CloudConfigured: cloudConfigured || installer.EngramCloudPartiallyConfigured(cfg, m)})

	// install-preview/install-backup (spec): show the write plan and ask for confirmation unless
	// --yes/--non-interactive/non-TTY says to skip straight through, then take the run-start
	// snapshot — all BEFORE MigrateIfStale/step 1 below (the first external `claude` subprocess
	// invocation). A decline here means zero writes: nothing below this point has run yet.
	proceed, err := confirmAndSnapshot(cmd, out, r, cfg, plan, isNonInteractiveInstall(cmd, out), updateWriteSteps(m.Engram.Version, cfg, cloudConfigured))
	if err != nil {
		return err
	}
	if !proceed {
		fmt.Fprintln(out, r.Info("Actualización cancelada."))
		return nil
	}

	// Confirmed migration behavior for the real-SDD-taxonomy realignment: a stale (pre-realignment
	// or otherwise outdated schema_version) models.json is backed up to models.json.bak FIRST,
	// then fully regenerated with new-taxonomy defaults — old per-phase overrides are never
	// preserved/merged (D8). A missing or already-current file is a no-op here.
	if _, err := installer.MigrateIfStale(cfg); err != nil {
		return err
	}

	// Re-apply whatever per-phase models AND active orchestration profile `click install` saved
	// (D25 / design D4), so `click update` never silently resets a developer's choice back to
	// defaults, and never silently drops the profile label back to "balanced" either. A
	// models.json-less home (installed before this feature existed, or never installed) falls back
	// to balanced + Defaults(). No interactive prompt here — update always re-applies, it never asks.
	profile := modelconfig.ProfileName("")
	var models map[modelconfig.Phase]string
	if selection.Claude || selection.OpenClaw {
		found := false
		profile, models, found, err = installer.LoadModelsWithProfile(cfg)
		if err != nil {
			return err
		}
		if !found {
			profile = modelconfig.ProfileBalanced
		}
		models = modelconfig.ResolveForProfile(string(profile), models)
	}

	for _, action := range plan.UpdateActionKinds() {
		switch action {
		case installer.StepActionSyncMarketplacePlugins:
			if err := r.RunStep("Re-sincronizando plugins click-sdd, click-memory, click-review y click-skills…", "Plugins sincronizados en Claude Code", func() error {
				return installer.SyncMarketplacePlugins(models, profile)
			}); err != nil {
				return err
			}
		case installer.StepActionSaveModels:
			if err := r.RunStep("Guardando modelos por fase de click-sdd…", "Modelos por fase guardados", func() error {
				return installer.SaveModelsWithProfile(cfg, profile, models)
			}); err != nil {
				return err
			}
		case installer.StepActionWriteClaudeManagedBlock:
			if err := r.RunStep("Actualizando CLAUDE.md…", "CLAUDE.md sincronizado", func() error {
				return installer.WriteManagedBlock(cfg.ClaudeMDPath(), installer.DefaultManagedContent)
			}); err != nil {
				return err
			}
		case installer.StepActionRegisterMemoryGuard:
			if err := r.RunStep("Re-registrando memory-guard…", "memory-guard sincronizado", func() error {
				return installer.RegisterMemoryGuardHook(cfg)
			}); err != nil {
				return err
			}
		case installer.StepActionSyncEngram:
			engramPathWarning := ""
			if err := r.RunStep(fmt.Sprintf("Sincronizando Engram (pin %s)…", m.Engram.Version), "Engram sincronizado", func() error {
				var syncErr error
				_, engramPathWarning, syncErr = installer.SyncEngram(cfg, m)
				return syncErr
			}); err != nil {
				return err
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
			fmt.Fprintln(out, r.Step("Sincronizando Engram Cloud…"))
			if cloudErr := syncEngramCloudFunc(cfg, m); cloudErr != nil {
				fmt.Fprintln(out, r.Warn(fmt.Sprintf("No se pudo sincronizar Engram Cloud: %v. La actualización local continúa; reintenta más tarde con `click update`.", cloudErr)))
			} else {
				fmt.Fprintln(out, r.Success("Engram Cloud sincronizado"))
			}
		case installer.StepActionSyncContext7:
			context7AlreadyPresent := false
			if err := r.RunStep("Sincronizando Context7 (documentación de librerías)…", "Context7 sincronizado", func() error {
				var syncErr error
				context7AlreadyPresent, syncErr = installer.SyncContext7(cfg)
				return syncErr
			}); err != nil {
				return err
			}
			if context7AlreadyPresent {
				fmt.Fprintln(out, r.Info("Context7 ya estaba configurado — se dejó como está, sin reinstalar."))
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
			if err := r.RunStep("Instalando plugin de memory-guard para OpenClaw…", "Plugin de memory-guard sincronizado en OpenClaw", func() error {
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
			if err := r.RunStep("Guardando recomendación de modelos para OpenClaw…", "Recomendación de modelos para OpenClaw guardada", func() error {
				return installer.SyncOpenClawModelProfile(cfg, profile, models)
			}); err != nil {
				return err
			}
		case installer.StepActionSyncCodexGuidance:
			if err := r.RunStep("Actualizando AGENTS.md de Codex…", "AGENTS.md de Codex actualizado", func() error {
				return syncCodexGuidanceFunc(cfg)
			}); err != nil {
				if restoreErr := installer.RestoreRun(cfg); restoreErr != nil {
					return fmt.Errorf("%w; rollback failed: %v", err, restoreErr)
				}
				return fmt.Errorf("%w; rollback restored the previous snapshot", err)
			}
		case installer.StepActionConfigureCodexNativeModel:
			model, _ := cmd.Flags().GetString(codexModelFlag)
			if strings.TrimSpace(model) == "" {
				fmt.Fprintln(out, r.Info("Codex: se omite la configuración nativa porque el modelo no fue seleccionado explícitamente (--codex-model <model>)."))
				continue
			}
			if err := installer.ConfigureCodexModel(cfg.CodexHome, model); err != nil {
				if restoreErr := installer.RestoreRun(cfg); restoreErr != nil {
					return fmt.Errorf("%w; rollback failed: %v", err, restoreErr)
				}
				return fmt.Errorf("%w; rollback restored the previous snapshot", err)
			}
		}
	}

	fmt.Fprintln(out, r.Info("Update completo."))
	return nil
}
