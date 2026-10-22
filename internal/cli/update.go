package cli

import (
	"fmt"
	"os"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/installer"
	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/manifest"
	"github.com/spf13/cobra"
)

func newUpdateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Re-sync plugins and the Engram pin to the currently installed click binary",
		RunE:  runUpdate,
	}
	// runUpdate's confirm gate below routes through isNonInteractiveInstall (install.go), which reads
	// --yes/--non-interactive. They are declared HERE, per command, rather than promoted to root
	// persistent flags: `install` already owns its own copies (install.go), and moving those to
	// persistent would silently widen them to every other subcommand too. Same names, same false
	// defaults, same meaning as install's — the two commands' escape hatch must not drift. The help
	// text is worded for THIS command (and in Spanish, like update's other flags): install's copy
	// says "instalar", which would be wrong on an update.
	cmd.Flags().Bool("dry-run", false, "Mostrar el plan sin ejecutar comandos ni modificar archivos")
	cmd.Flags().Bool(yesFlag, false, "Omitir toda pantalla interactiva y confirmar la actualización automáticamente")
	cmd.Flags().Bool(nonInteractiveFlag, false, "Alias de --yes")
	cmd.Flags().String(profileFlag, "", "")
	_ = cmd.Flags().MarkHidden(profileFlag)
	cmd.Flags().Bool(skipOpenClawFlag, false, "Omitir la integración con OpenClaw aunque se detecte openclaw en este equipo")
	cmd.Flags().String(codexModelFlag, "", "")
	cmd.Flags().String(openClawModelFlag, "", "")
	cmd.Flags().StringSlice(openClawFallbackFlag, nil, "")
	_ = cmd.Flags().MarkHidden(codexModelFlag)
	_ = cmd.Flags().MarkHidden(openClawModelFlag)
	_ = cmd.Flags().MarkHidden(openClawFallbackFlag)
	cmd.Flags().Bool(persistEngramCloudTokenFlag, false, "Autorizar almacenamiento de ENGRAM_CLOUD_TOKEN en ~/.claude/settings.json con permisos 0600 (requiere consentimiento explícito)")
	return cmd
}

func runUpdate(cmd *cobra.Command, args []string) error {
	if retiredModelFlagsWereSet(cmd) {
		return retiredModelConfigurationError()
	}
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
	// Narrow persisted intent by availability for this run; never enable an excluded
	// target before building the upstream selection and the local action plan.
	selection.Codex = selection.Codex && cfg.CodexHome != ""
	selection.OpenClaw = selection.OpenClaw && cfg.OpenClawHome != ""
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
	cloudResolvable := installer.EngramCloudConfigured(cfg, m) || installer.EngramCloudPartiallyConfigured(cfg, m)
	if err := preflightGentleAI(cfg, selection); err != nil {
		return err
	}
	plan := installer.BuildTargetPlan(cfg, selection, installer.PlanOptions{CloudResolvable: cloudResolvable, DotHooks: dotHooksPlanOption()})

	// install-preview/install-backup (spec): show the write plan and ask for confirmation unless
	// --yes/--non-interactive/non-TTY says to skip straight through, then take the run-start
	// snapshot before the first external `claude` subprocess invocation. A decline here means zero
	// writes: nothing below this point has run yet.
	proceed, sharedReader, err := confirmAndSnapshot(cmd, out, r, cfg, plan, isNonInteractiveInstall(cmd, out), withDotHooksPreview(updateWriteSteps(m.Engram.Version, cfg, cloudResolvable), plan, plan.UpdateActionKinds()))
	if err != nil {
		return err
	}
	if !proceed {
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		if !dryRun {
			fmt.Fprintln(out, r.Info("Actualización cancelada."))
		}
		return nil
	}

	// Engram Cloud token persistence consent: if cloud server and project are resolvable
	// and a token is available in the environment, prompt the user for consent to store the
	// token in settings.json. The shared reader from confirmAndSnapshot is used so that the
	// consent prompt consumes the next input in order.
	persistFlag, _ := cmd.Flags().GetBool(persistEngramCloudTokenFlag)
	token := os.Getenv("ENGRAM_CLOUD_TOKEN")
	// persistenceMode distinguishes three states:
	// - No process token → NoOp: never prompt, never write, never delete an existing stored token
	// - Process token present, consent declined → Decline: do not write, remove any previously stored click-owned token
	// - Process token present, consent given → Persist
	persistenceMode := installer.CloudTokenPersistenceNoOp
	if installer.EngramCloudConfigured(cfg, m) && token != "" {
		// resolveCloudTokenPersistence is exhaustive (every branch returns Persist or Decline
		// explicitly), so persistenceMode is fully determined by its return value alone — no
		// intermediate default assignment is needed here.
		var readErr error
		persistenceMode, readErr = resolveCloudTokenPersistence(isNonInteractiveInstall(cmd, out), persistFlag, sharedReader, out)
		if readErr != nil {
			fmt.Fprintln(out, r.Warn(consentSkippedWarning))
		}
		if persistenceMode == installer.CloudTokenPersistenceDecline {
			fmt.Fprintln(out, r.Warn(autosyncDisabledWarning))
		}
	}

	// Arm deferred post-run snapshot recording (only when proceed=true)
	defer func() {
		if recordErr := recordSnapshotPostRunFunc(cfg); recordErr != nil {
			fmt.Fprintln(out, r.Warn(fmt.Sprintf("No se pudo registrar el estado posterior a la ejecución para rollback: %v", recordErr)))
		}
	}()

	// The mandatory upstream foundation must succeed before Click persists selection
	// or migrates models. The run-start snapshot and deferred recording are already armed.
	if len(installer.GentleAIInstallArgs(selection)) > 0 {
		if err := runGentleAIStep(cmd, out, r, m, cfg, selection); err != nil {
			return err
		}
	}

	for _, action := range plan.UpdateActionKinds() {
		switch action {
		case installer.StepActionSyncGentleAI:
			// Already completed before Click-owned state writes; never run it twice.
			continue
		case installer.StepActionSyncMarketplacePlugins:
			if err := r.RunStep("Retirando click-sdd y sincronizando click-memory, click-review y click-skills…", "Plugins de Click sincronizados en Claude Code", func() error {
				return installer.SyncMarketplacePlugins(cfg, nil)
			}); err != nil {
				return err
			}
		case installer.StepActionWriteClaudeManagedBlock:
			if err := r.RunStep("Actualizando CLAUDE.md…", "CLAUDE.md sincronizado", func() error {
				return installer.WriteManagedBlock(cfg.ClaudeMDPath(), installer.DefaultManagedContent)
			}); err != nil {
				return err
			}
		case installer.StepActionRegisterDotHooks:
			// Needs Click Dot on disk: executed right after the Click Dot sync step below.
			continue
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
		case installer.StepActionConfigureEngramCloudSessionSync:
			// Writes the click-owned env block and SessionStart hook. With a token the consent block
			// above already resolved persistenceMode; without one (CloudResolvable) the env block and
			// hook are still written so a later token export activates everything with no reinstall
			// (DD-4). Non-fatal with a Spanish warning (ECS-10.1/10.2).
			if configureErr := installer.ConfigureEngramCloudSessionSync(cfg, m, persistenceMode, token); configureErr != nil {
				fmt.Fprintln(out, r.Warn(fmt.Sprintf("No se pudo configurar Engram Cloud Session Sync: %v. La actualización local continúa.", configureErr)))
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
			// Cleanup-only (see SyncOpenClawMCPConfig's doc comment): this removes the invalid legacy
			// "mcpServers" key click used to incorrectly write into openclaw.json, pending OpenClaw's
			// confirmed native MCP registration mechanism. It never creates openclaw.json and never
			// writes that key again.
			if err := r.RunStep("Limpiando configuración inválida heredada de OpenClaw…", "Configuración heredada de OpenClaw revisada", func() error {
				return installer.SyncOpenClawMCPConfig(cfg)
			}); err != nil {
				return err
			}
		case installer.StepActionRegisterOpenClawMCP:
			// D45 "supplementary integrations are non-fatal" pattern (same as Codex MCP above):
			// registering Engram's MCP server with OpenClaw must never abort an otherwise-good update.
			// Always attempted when OpenClaw is a target, independent of --openclaw-model.
			fmt.Fprintln(out, r.Step("Registrando Engram en OpenClaw (MCP)…"))
			if mcpErr := syncOpenClawMCPFunc(cfg); mcpErr != nil {
				fmt.Fprintln(out, r.Warn(fmt.Sprintf("No se pudo registrar Engram en OpenClaw: %v. La actualización local continúa; reintenta más tarde con `click update`.", mcpErr)))
			} else {
				fmt.Fprintln(out, r.Success("Engram registrado en OpenClaw"))
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
		case installer.StepActionRetireLegacyOpenClawSkills:
			var warnings []string
			err := r.RunStep("Retirando skills heredadas de OpenClaw…", "Skills heredadas de OpenClaw revisadas", func() error {
				var retireErr error
				warnings, retireErr = installer.RetireLegacyOpenClawSkills(cfg)
				return retireErr
			})
			for _, warning := range warnings {
				fmt.Fprintln(out, r.Warn(warning))
			}
			if err != nil {
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
		case installer.StepActionSyncCodexMCP:
			// D45 "supplementary integrations are non-fatal" pattern (same as Engram Cloud above):
			// registering Engram's MCP server with Codex must never abort an otherwise-good update.
			// Always attempted when Codex is a target, independent of --codex-model.
			fmt.Fprintln(out, r.Step("Registrando Engram en Codex (MCP)…"))
			if mcpErr := syncCodexMCPFunc(cfg); mcpErr != nil {
				fmt.Fprintln(out, r.Warn(fmt.Sprintf("No se pudo registrar Engram en Codex: %v. La actualización local continúa; reintenta más tarde con `click update`.", mcpErr)))
			} else {
				fmt.Fprintln(out, r.Success("Engram registrado en Codex"))
			}
		}
	}

	syncDotStep(cmd, out, r, dotUpdateVerb)
	if planHasAction(plan.UpdateActionKinds(), installer.StepActionRegisterDotHooks) {
		registerDotHooksStep(out, r, cfg, dotUpdateVerb)
	}

	fmt.Fprintln(out, r.Info("Update completo."))
	return nil
}
