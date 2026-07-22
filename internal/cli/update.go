package cli

import (
	"fmt"

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
	return cmd
}

func runUpdate(cmd *cobra.Command, args []string) error {
	out := cmd.OutOrStdout()
	r := rendererFor(cmd, out)

	// PreflightClaude must run before PreflightGit and before anything else. claude is the more
	// fundamental dependency: `click update` re-syncs every plugin via the claude CLI itself
	// (plugins.go's pluginCLIBinary), so a machine missing Claude Code entirely should fail on
	// that actionable message first, not on git's.
	if err := installer.PreflightClaude(); err != nil {
		return err
	}

	// PreflightGit must run before anything else: `click update` re-syncs the plugin marketplace
	// via SyncMarketplacePlugins, exactly like `click install` does, and that shells out to
	// `git clone` under the hood — see runInstall's PreflightGit call for the full rationale.
	if err := installer.PreflightGit(); err != nil {
		return err
	}

	claudeHome, err := installer.ResolveClaudeHome()
	if err != nil {
		return err
	}
	cfg := installer.Config{ClaudeHome: claudeHome}

	// openclaw-target-support: same detect+confirm rule as runInstall — see install.go's cfg
	// construction for the full rationale. Kept duplicated (not extracted into a shared helper)
	// because the two commands' surrounding flag/error plumbing differs enough that a shared helper
	// would need its own indirection for no real gain at this size.
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

	m, err := manifest.Load()
	if err != nil {
		return err
	}
	cloudConfigured := installer.EngramCloudConfigured(cfg, m)

	// install-preview/install-backup (spec): show the write plan and ask for confirmation unless
	// --yes/--non-interactive/non-TTY says to skip straight through, then take the run-start
	// snapshot — all BEFORE MigrateIfStale/step 1 below (the first external `claude` subprocess
	// invocation). A decline here means zero writes: nothing below this point has run yet.
	proceed, err := confirmAndSnapshot(cmd, out, r, cfg, isNonInteractiveInstall(cmd, out), updateWriteSteps(m.Engram.Version, cfg, cloudConfigured))
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
	profile, models, found, err := installer.LoadModelsWithProfile(cfg)
	if err != nil {
		return err
	}
	if !found {
		profile = modelconfig.ProfileBalanced
	}
	models = modelconfig.ResolveForProfile(string(profile), models)

	if err := r.RunStep("Re-sincronizando plugins click-sdd, click-memory, click-review y click-skills…", "Plugins sincronizados en Claude Code", func() error {
		return installer.SyncMarketplacePlugins(models, profile)
	}); err != nil {
		return err
	}
	// Symmetric with `click install`: re-applying the per-phase model routing config without also
	// re-persisting it to disk left models.json stale (or entirely absent, on a home where it was
	// never written) after `click update` — a real asymmetry bug between the two commands.
	if err := r.RunStep("Guardando modelos por fase de click-sdd…", "Modelos por fase guardados", func() error {
		return installer.SaveModelsWithProfile(cfg, profile, models)
	}); err != nil {
		return err
	}
	if err := r.RunStep("Actualizando CLAUDE.md…", "CLAUDE.md sincronizado", func() error {
		return installer.WriteManagedBlock(cfg.ClaudeMDPath(), installer.DefaultManagedContent)
	}); err != nil {
		return err
	}
	if err := r.RunStep("Re-registrando memory-guard…", "memory-guard sincronizado", func() error {
		return installer.RegisterMemoryGuardHook(cfg)
	}); err != nil {
		return err
	}
	engramPathWarning := ""
	if err := r.RunStep(fmt.Sprintf("Sincronizando Engram (pin %s)…", m.Engram.Version), "Engram sincronizado", func() error {
		var syncErr error
		_, engramPathWarning, syncErr = installer.SyncEngram(cfg, m)
		return syncErr
	}); err != nil {
		return err
	}
	surfacePathWarning(out, r, engramPathWarning)

	if installer.EngramCloudPartiallyConfigured(cfg, m) {
		reportSkippedCloudEnrollment(out, r)
	} else if cloudConfigured {
		// resilience W1: Engram Cloud re-sync is opt-in and supplementary — a flaky or unreachable
		// cloud server must never abort an otherwise-valid local update. On failure we surface a
		// Spanish warning and CONTINUE with the remaining purely-local steps (Context7, OpenClaw sync)
		// instead of returning the error. Deliberately NOT r.RunStep: a non-fatal step must not render
		// a red ✗/[FAIL] line — the outcome is either success or a warning, never a failure marker.
		fmt.Fprintln(out, r.Step("Sincronizando Engram Cloud…"))
		if cloudErr := syncEngramCloudFunc(cfg, m); cloudErr != nil {
			fmt.Fprintln(out, r.Warn(fmt.Sprintf("No se pudo sincronizar Engram Cloud: %v. La actualización local continúa; reintenta más tarde con `click update`.", cloudErr)))
		} else {
			fmt.Fprintln(out, r.Success("Engram Cloud sincronizado"))
		}
	}

	// Same non-fatal binary-provisioning report as `click install` (Slice 3b): SyncEngram already
	// attempted `go install` internally when needed; this just surfaces the resulting state.
	if _, resolvable, err := installer.EngramBinaryResolvable(cfg); err != nil {
		return err
	} else if !resolvable {
		fmt.Fprintln(out, r.Info(installer.EngramBinaryRemediationMessage(m.Engram.Version)))
	}

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

	// openclaw-target-support: appended LAST, matching openClawWriteSteps' position at the end of
	// updateWriteSteps(..., cfg) — see runInstall's mirrored block for the full rationale.
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
		// PR-C (design #1666's memory-guard-parity piece, OCG-1..6): re-syncs the click-memory-guard
		// OpenClaw plugin last — re-templating CLICK_BIN against whatever os.Executable() resolves to
		// on THIS run, so a click binary that moved since install gets picked up automatically.
		if err := r.RunStep("Instalando plugin de memory-guard para OpenClaw…", "Plugin de memory-guard sincronizado en OpenClaw", func() error {
			return installer.SyncOpenClawPlugin(cfg)
		}); err != nil {
			return err
		}
		// PR4: re-synchronize the click-owned OpenClaw skill manifests after the plugin re-sync,
		// matching openClawWriteSteps' position for the skill sync step.
		if err := r.RunStep("Sincronizando skills clickhola y clickdev en OpenClaw…", "Skills clickhola y clickdev sincronizados en OpenClaw", func() error {
			return installer.SyncOpenClawSkills(cfg)
		}); err != nil {
			return err
		}
	}

	fmt.Fprintln(out, r.Info("Update completo."))
	return nil
}
