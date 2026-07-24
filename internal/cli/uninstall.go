package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/installer"
	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/ui"
	"github.com/spf13/cobra"
)

// removeEngramPluginFunc is the injectable seam behind runUninstall's installer.RemoveEngramPlugin
// call, mirroring this codebase's existing installer.SetCommandRunnerFactoryForTests /
// SetBinaryLookupFactoryForTests factory-injection pattern. It exists purely so a CLI-level test can
// simulate RemoveEngramPlugin returning BOTH a non-empty pathWarning AND a non-nil error at once — a
// real combination it can legitimately produce (e.g. removing one PATH entry fails while a LATER,
// unrelated step such as uninstalling the plugin itself also fails) but one that cannot be driven
// hermetically from this package: installer.pathStoreFactory is unexported and, in a normal build
// (test builds included), is wired to the REAL OS PATH store (Windows registry / POSIX rc files) by
// pathenv_windows.go/pathenv_unix.go's own init(), so this package deliberately never lets a real
// PATH mutation happen in its own tests (see seedResolvableEngram's doc comment).
var removeEngramPluginFunc = installer.RemoveEngramPlugin

// SetRemoveEngramPluginFuncForTests overrides removeEngramPluginFunc for tests and returns a
// restore function.
func SetRemoveEngramPluginFuncForTests(fn func(installer.Config) (string, error)) func() {
	old := removeEngramPluginFunc
	removeEngramPluginFunc = fn
	return func() { removeEngramPluginFunc = old }
}

// removeOpenClawSkillsFunc is the injectable seam behind runUninstall's
// installer.RemoveOpenClawSkills call, mirroring removeEngramPluginFunc above. It lets CLI-level
// tests prove PR4's resilience contract: a failure removing the click-owned OpenClaw skill
// directories must be recorded and reported without aborting the rest of the teardown.
var removeOpenClawSkillsFunc = installer.RemoveOpenClawSkills

// SetRemoveOpenClawSkillsFuncForTests overrides removeOpenClawSkillsFunc for tests and returns a
// restore function.
func SetRemoveOpenClawSkillsFuncForTests(fn func(installer.Config) error) func() {
	old := removeOpenClawSkillsFunc
	removeOpenClawSkillsFunc = fn
	return func() { removeOpenClawSkillsFunc = old }
}

var stripCodexGuidanceFunc = installer.StripCodexGuidance

func SetStripCodexGuidanceFuncForTests(fn func(installer.Config) error) func() {
	old := stripCodexGuidanceFunc
	stripCodexGuidanceFunc = fn
	return func() { stripCodexGuidanceFunc = old }
}

func newUninstallCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "uninstall",
		Short: "Reverse everything click install and click update wrote",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUninstall(cmd)
		},
	}
}

// uninstallStepOutcome records one teardown step's Spanish label (used in the final summary) and
// the error it produced, nil for a step that succeeded.
type uninstallStepOutcome struct {
	label string
	err   error
}

// runUninstall reverses everything `click install`/`click update` can have written, following a
// RESILIENT teardown design (Finding 2, review-resilience WARNING): every cleanup step below is
// attempted regardless of whether an earlier one failed, and the command's final result is a
// complete summary of what succeeded and what didn't — never a silent partial teardown.
//
// Before this fix, runUninstall returned immediately on the FIRST step's error (`return err`).
// Since step 1 (RemoveMarketplacePlugins) shells out to `claude`, a completely realistic uninstall
// scenario — the developer already removed Claude Code as part of tearing down their setup — made
// EVERY later step (CLAUDE.md, the memory-guard hook, Engram/Context7 bookkeeping) silently never
// run, directly contradicting installer.go's own doc comment that Uninstall "reverses everything
// Install can have written". Every error collected below is still guaranteed to surface in the final
// summary (reportUninstallOutcome) — resilience here means "keep going", never "swallow the error".
//
// PreflightClaude/PreflightGit (mirroring install.go/update.go's own preflight calls) run up front
// too, but — unlike install/update — they never abort here: a missing claude/git IS the "tearing
// down my setup" scenario this fix targets, so aborting on it would defeat the whole point. Their
// result is used two ways instead: (1) printed immediately as an upfront advisory, so the developer
// understands WHY several steps below are about to fail before they even run; and (2) any
// claude-dependent step's raw exec error is wrapped with the SAME actionable ClaudeMissingMessage
// text install/update already show, instead of a bare, unwrapped `exec: "claude": executable file
// not found` error propagating into the final summary untouched.
func runUninstall(cmd *cobra.Command) error {
	out := cmd.OutOrStdout()
	r := rendererFor(cmd, out)
	clickStateHome, err := installer.ResolveClickStateHome()
	if err != nil {
		return err
	}
	cfg := installer.Config{ClickStateHome: clickStateHome}
	selection, configured, err := installer.LoadTargetSelection(cfg)
	if err != nil {
		return err
	}
	if !configured {
		selection = installer.TargetSelection{Configured: true, Claude: installer.ClaudeAvailable(), OpenClaw: installer.OpenClawAvailable(), Codex: installer.CodexAvailable()}
	}
	var claudeErr error
	if selection.Claude {
		claudeHome, resolveErr := installer.ResolveClaudeHome()
		if resolveErr != nil {
			return resolveErr
		}
		cfg.ClaudeHome = claudeHome
		claudeErr = installer.PreflightClaude()
		if claudeErr != nil {
			fmt.Fprintln(out, r.Warn(claudeErr.Error()))
		}
		if gitErr := installer.PreflightGit(); gitErr != nil {
			fmt.Fprintln(out, r.Warn(gitErr.Error()))
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
	plan := installer.BuildTargetPlan(cfg, selection, installer.PlanOptions{})
	if err := installer.SnapshotTargetPlan(cfg, plan); err != nil {
		return err
	}

	var outcomes []uninstallStepOutcome
	runStep := func(label, running, done string, needsClaude bool, fn func() error) error {
		stepErr := r.RunStep(running, done, fn)
		if stepErr != nil && needsClaude && claudeErr != nil {
			stepErr = fmt.Errorf("%s (%w)", installer.ClaudeMissingMessage, stepErr)
		}
		outcomes = append(outcomes, uninstallStepOutcome{label: label, err: stepErr})
		return stepErr
	}
	rollbackAndReport := func() error {
		if restoreErr := installer.RestoreRun(cfg); restoreErr != nil {
			outcomes = append(outcomes, uninstallStepOutcome{label: "rollback", err: restoreErr})
		}
		return reportUninstallOutcome(out, r, outcomes)
	}

	for _, action := range plan.UninstallActionKinds() {
		switch action {
		case installer.StepActionRemoveMarketplacePlugins:
			if err := runStep("Plugins de Claude Code", "Quitando plugins click-sdd, click-memory, click-review y click-skills…", "Plugins eliminados de Claude Code", true, func() error {
				return installer.RemoveMarketplacePlugins()
			}); err != nil {
				return rollbackAndReport()
			}
		case installer.StepActionRemoveOpenClawPlugin:
			if err := runStep("plugin de OpenClaw", "Quitando plugin de memory-guard para OpenClaw…", "Plugin de memory-guard para OpenClaw eliminado", false, func() error {
				return installer.RemoveOpenClawPlugin(cfg)
			}); err != nil {
				return rollbackAndReport()
			}
		case installer.StepActionRemoveOpenClawSkills:
			if err := runStep("skills de OpenClaw", "Quitando skills de Click de OpenClaw…", "Skills de Click de OpenClaw eliminados", false, func() error {
				return removeOpenClawSkillsFunc(cfg)
			}); err != nil {
				return rollbackAndReport()
			}
		case installer.StepActionStripCodexGuidance:
			if err := runStep("AGENTS.md de Codex", "Limpiando AGENTS.md de Codex…", "Bloque de AGENTS.md de Codex eliminado", false, func() error {
				return stripCodexGuidanceFunc(cfg)
			}); err != nil {
				return rollbackAndReport()
			}
		case installer.StepActionRemoveEngram:
			engramPathWarning := ""
			if err := runStep("Engram", "Quitando Engram (si click lo instaló)…", "Engram procesado", true, func() error {
				var pathErr error
				engramPathWarning, pathErr = removeEngramPluginFunc(cfg)
				return pathErr
			}); err != nil {
				surfacePathWarning(out, r, engramPathWarning)
				return rollbackAndReport()
			}
			surfacePathWarning(out, r, engramPathWarning)
		case installer.StepActionRemoveContext7:
			context7Warning := ""
			if err := runStep("Context7", "Quitando Context7 (si click lo instaló)…", "Context7 procesado", true, func() error {
				var stepErr error
				context7Warning, stepErr = installer.RemoveContext7(cfg)
				return stepErr
			}); err != nil {
				surfacePathWarning(out, r, context7Warning)
				return rollbackAndReport()
			}
			surfacePathWarning(out, r, context7Warning)
		case installer.StepActionStripClaudeManagedBlock:
			if err := runStep("CLAUDE.md", "Limpiando CLAUDE.md…", "Bloque de CLAUDE.md eliminado", false, func() error {
				return installer.StripManagedBlock(cfg.ClaudeMDPath())
			}); err != nil {
				return rollbackAndReport()
			}
		case installer.StepActionUnregisterMemoryGuard:
			if err := runStep("memory-guard", "Quitando memory-guard…", "memory-guard eliminado", false, func() error {
				return installer.UnregisterMemoryGuardHook(cfg)
			}); err != nil {
				return rollbackAndReport()
			}
		case installer.StepActionRemoveTargetSelection:
			if err := runStep("selección de runtimes", "Quitando selección persistente de runtimes…", "Selección persistente de runtimes eliminada", false, func() error {
				return installer.RemoveTargetSelection(cfg)
			}); err != nil {
				return rollbackAndReport()
			}
		}
	}

	return reportUninstallOutcome(out, r, outcomes)
}

// reportUninstallOutcome renders the final Spanish summary of every teardown step's outcome and
// decides `click uninstall`'s overall result. Every error collected in outcomes surfaces here —
// never silently dropped — satisfying Finding 2(b)'s "do not silently swallow errors" requirement,
// even though every step already ran to completion above regardless of earlier failures.
func reportUninstallOutcome(out io.Writer, r *ui.Renderer, outcomes []uninstallStepOutcome) error {
	var failed []uninstallStepOutcome
	for _, o := range outcomes {
		if o.err != nil {
			failed = append(failed, o)
		}
	}
	if len(failed) == 0 {
		fmt.Fprintln(out, r.Info("Desinstalación completa."))
		return nil
	}

	lines := make([]string, 0, len(failed))
	for _, o := range failed {
		lines = append(lines, fmt.Sprintf("%s: %v", o.label, o.err))
	}
	fmt.Fprintln(out, r.Warn(fmt.Sprintf(
		"Desinstalación incompleta: %d de %d pasos fallaron. Los pasos que sí se completaron NO se revirtieron; revise el detalle debajo y atienda manualmente lo que falta.",
		len(failed), len(outcomes),
	)))
	for _, line := range lines {
		fmt.Fprintln(out, r.Fail(line))
	}
	return fmt.Errorf("click uninstall: %d de %d pasos fallaron:\n%s", len(failed), len(outcomes), strings.Join(lines, "\n"))
}
