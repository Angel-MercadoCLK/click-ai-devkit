package cli

import (
	"fmt"
	"os"
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
	// runUpdate's confirm gate below routes through isNonInteractiveInstall (install.go), which reads
	// --yes/--non-interactive. They are declared HERE, per command, rather than promoted to root
	// persistent flags: `install` already owns its own copies (install.go), and moving those to
	// persistent would silently widen them to every other subcommand too. Same names, same false
	// defaults, same meaning as install's — the two commands' escape hatch must not drift. The help
	// text is worded for THIS command (and in Spanish, like update's other flags): install's copy
	// says "instalar", which would be wrong on an update.
	cmd.Flags().Bool(yesFlag, false, "Omitir toda pantalla interactiva y confirmar la actualización automáticamente")
	cmd.Flags().Bool(nonInteractiveFlag, false, "Alias de --yes")
	cmd.Flags().Bool(skipOpenClawFlag, false, "Omitir la integración con OpenClaw aunque se detecte openclaw en este equipo")
	cmd.Flags().String(codexModelFlag, "", "Referencia de modelo nativa de Codex, por ejemplo gpt-5.6")
	cmd.Flags().Bool(persistEngramCloudTokenFlag, false, "Autorizar almacenamiento de ENGRAM_CLOUD_TOKEN en ~/.claude/settings.json con permisos 0600 (requiere consentimiento explícito)")
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

	// Native-model config mirrors `click install`: no prompt, and a native mutation only when the
	// developer explicitly passes --codex-model. Without it, Codex keeps running the portable SDD
	// against the model it already has configured, and OpenClaw keeps the provider/model already
	// connected — so the native-model write is neither listed nor run. Print a Spanish info line for
	// each detected target making the resulting behavior explicit.
	codexModelValue, _ := cmd.Flags().GetString(codexModelFlag)
	codexModel := strings.TrimSpace(codexModelValue)
	codexNativeModel := selection.Codex && codexModel != ""
	if selection.Codex {
		if codexNativeModel {
			fmt.Fprintln(out, r.Info("Codex: modelo nativo configurado explícitamente: "+codexModel+"."))
		} else {
			fmt.Fprintln(out, r.Info("Codex usa su modelo nativo ya configurado; el SDD portable corre con ese default."))
		}
	}
	if selection.OpenClaw {
		fmt.Fprintln(out, r.Info("OpenClaw usa el proveedor/modelo que ya conectaste; el SDD portable corre con ese default."))
	}

	m, err := manifest.Load()
	if err != nil {
		return err
	}
	cloudResolvable := installer.EngramCloudConfigured(cfg, m) || installer.EngramCloudPartiallyConfigured(cfg, m)
	plan := installer.BuildTargetPlan(cfg, selection, installer.PlanOptions{CloudResolvable: cloudResolvable, CodexNativeModel: codexNativeModel})

	// install-preview/install-backup (spec): show the write plan and ask for confirmation unless
	// --yes/--non-interactive/non-TTY says to skip straight through, then take the run-start
	// snapshot — all BEFORE MigrateIfStale/step 1 below (the first external `claude` subprocess
	// invocation). A decline here means zero writes: nothing below this point has run yet.
	proceed, sharedReader, err := confirmAndSnapshot(cmd, out, r, cfg, plan, isNonInteractiveInstall(cmd, out), updateWriteSteps(m.Engram.Version, cfg, cloudResolvable, codexNativeModel))
	if err != nil {
		return err
	}
	if !proceed {
		fmt.Fprintln(out, r.Info("Actualización cancelada."))
		return nil
	}

	// Engram Cloud token persistence consent: if cloud server and project are resolvable
	// and a token is available in the environment, prompt the user for consent to store the
	// token in settings.json. The shared reader from confirmAndSnapshot is used so that the
	// consent prompt consumes the next input in order.
	persistFlag, _ := cmd.Flags().GetBool(persistEngramCloudTokenFlag)
	token := os.Getenv("ENGRAM_CLOUD_TOKEN")
	server := os.Getenv("CLICK_ENGRAM_CLOUD_SERVER")
	project := os.Getenv("CLICK_ENGRAM_CLOUD_PROJECT")
	// D40 enrollment only runs when the token was actually persisted: without it the session-sync
	// hook cannot authenticate on later sessions, so re-syncing the current run alone is pointless
	// (ECS-3.3). The consent block below records that decision; the StepActionSyncEngramCloud case
	// in the dispatch loop consults cloudEnrollmentAllowed.
	cloudEnrollmentAllowed := false
	if server != "" && project != "" && token != "" {
		emitConsentPrompt(out)
		persistenceMode, readErr := resolveCloudTokenPersistence(isNonInteractiveInstall(cmd, out), persistFlag, sharedReader)
		if readErr != nil {
			fmt.Fprintln(out, r.Warn(consentSkippedWarning))
		}
		if persistenceMode == installer.CloudTokenPersistenceDecline {
			fmt.Fprintln(out, r.Warn(autosyncDisabledWarning))
		}
		cloudEnrollmentAllowed = persistenceMode == installer.CloudTokenPersistencePersist
		if configureErr := installer.ConfigureEngramCloudSessionSync(cfg, m, persistenceMode, token); configureErr != nil {
			fmt.Fprintln(out, r.Warn(fmt.Sprintf("No se pudo configurar Engram Cloud Session Sync: %v. La actualización local continúa.", configureErr)))
		}
	}

	// Arm deferred post-run snapshot recording (only when proceed=true)
	defer func() {
		if recordErr := recordSnapshotPostRunFunc(cfg); recordErr != nil {
			fmt.Fprintln(out, r.Warn(fmt.Sprintf("No se pudo registrar el estado posterior a la ejecución para rollback: %v", recordErr)))
		}
	}()

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

	// Re-apply the previously chosen Codex tier, or default to "recommended" if none was saved yet.
	codexTier := "recommended"
	if selection.Codex {
		if savedTier, found, loadErr := installer.LoadCodexModelProfile(cfg); loadErr != nil {
			return loadErr
		} else if found {
			codexTier = savedTier
		}
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
			if !cloudEnrollmentAllowed {
				// Token is present but persistence was declined (or not opted into): the session-sync
				// hook has no token to authenticate future sessions, so skip the cloud re-sync. The
				// autosync-disabled warning was already printed by the consent block.
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
		case installer.StepActionSaveCodexModelProfile:
			if err := r.RunStep("Guardando perfil de modelos de Codex (referencia)…", "Perfil de modelos de Codex guardado", func() error {
				return installer.SaveCodexModelProfile(cfg, codexTier)
			}); err != nil {
				if restoreErr := installer.RestoreRun(cfg); restoreErr != nil {
					return fmt.Errorf("%w; rollback failed: %v", err, restoreErr)
				}
				return fmt.Errorf("%w; rollback restored the previous snapshot", err)
			}
		case installer.StepActionConfigureCodexNativeModel:
			// Only present in the plan when --codex-model was passed (see PlanOptions.CodexNativeModel
			// above); a plain update never reaches this native config.toml mutation.
			if err := installer.ConfigureCodexModel(cfg.CodexHome, codexModel); err != nil {
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
