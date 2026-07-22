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

	claudeHome, err := installer.ResolveClaudeHome()
	if err != nil {
		return err
	}
	// openclaw-target-support (PR-C, design #1666's OCG-1..6): resolved UNCONDITIONALLY, unlike
	// install/update's detect+confirm gate — removing the plugin dir is safe and idempotent even on
	// a machine where openclaw is no longer on PATH (or never was); RemoveOpenClawPlugin's
	// os.RemoveAll is a no-op when the directory doesn't exist. This mirrors StripManagedBlock's own
	// unconditional-attempt posture a few lines below, rather than RemoveEngramPlugin/RemoveContext7's
	// ownership-gated one — there is no per-install "did click own this" state for the plugin dir to
	// check, so "always attempt removal" is the simplest safe teardown.
	openClawHome, err := installer.ResolveOpenClawHome()
	if err != nil {
		return err
	}
	cfg := installer.Config{ClaudeHome: claudeHome, OpenClawHome: openClawHome}

	claudeErr := installer.PreflightClaude()
	if claudeErr != nil {
		fmt.Fprintln(out, r.Warn(claudeErr.Error()))
	}
	if gitErr := installer.PreflightGit(); gitErr != nil {
		fmt.Fprintln(out, r.Warn(gitErr.Error()))
	}

	var outcomes []uninstallStepOutcome
	// runStep wraps r.RunStep so a step's failure is RECORDED, never returned early — the core of
	// Finding 2(b)'s resilience contract. needsClaude marks steps that (transitively) shell out to
	// `claude`: when claudeErr is non-nil, their raw failure gets ClaudeMissingMessage prepended
	// (Finding 2(a)) so the developer sees the same actionable text install/update show, not a raw
	// exec dump, while the original error is still preserved via %w for diagnostics.
	runStep := func(label, running, done string, needsClaude bool, fn func() error) {
		stepErr := r.RunStep(running, done, fn)
		if stepErr != nil && needsClaude && claudeErr != nil {
			stepErr = fmt.Errorf("%s (%w)", installer.ClaudeMissingMessage, stepErr)
		}
		outcomes = append(outcomes, uninstallStepOutcome{label: label, err: stepErr})
	}

	runStep(
		"Plugins de Claude Code",
		"Quitando plugins click-sdd, click-memory, click-review y click-skills…",
		"Plugins eliminados de Claude Code",
		true,
		func() error { return installer.RemoveMarketplacePlugins() },
	)

	runStep(
		"CLAUDE.md",
		"Limpiando CLAUDE.md…",
		"Bloque de CLAUDE.md eliminado",
		false,
		func() error { return installer.StripManagedBlock(cfg.ClaudeMDPath()) },
	)

	runStep(
		"memory-guard",
		"Quitando memory-guard…",
		"memory-guard eliminado",
		false,
		func() error { return installer.UnregisterMemoryGuardHook(cfg) },
	)

	// openclaw-target-support (PR-C, task 3.13): parity with how the Claude Code memory-guard hook
	// above gets torn down — removes the click-memory-guard OpenClaw plugin directory. No-op (nil,
	// no error) when it was never installed.
	runStep(
		"plugin de OpenClaw",
		"Quitando plugin de memory-guard para OpenClaw…",
		"Plugin de memory-guard para OpenClaw eliminado",
		false,
		func() error { return installer.RemoveOpenClawPlugin(cfg) },
	)

	// RemoveEngramPlugin only reverses Engram when click's own state says click installed it —
	// a pre-existing developer setup is left running untouched. It also independently reverses
	// click's own PATH mutation(s) (D-9, T4-1, T4-1 follow-up) when it recorded owning them; a
	// failure doing so is surfaced as engramPathWarning below rather than folded into the fatal err.
	// Finding 3: a corrupted engram.json is ALSO no longer a fatal error here — RemoveEngramPlugin
	// itself now treats that as an "unknown ownership, touch nothing" case and reports it back
	// through this same pathWarning channel (see engram.go's RemoveEngramPlugin doc comment) instead
	// of the hard error that used to abort every step after it.
	engramPathWarning := ""
	runStep(
		"Engram",
		"Quitando Engram (si click lo instaló)…",
		"Engram procesado",
		true,
		func() error {
			var pathErr error
			engramPathWarning, pathErr = removeEngramPluginFunc(cfg)
			return pathErr
		},
	)
	surfacePathWarning(out, r, engramPathWarning)

	// RemoveContext7 mirrors RemoveEngramPlugin's exact respect-ownership contract: only removes
	// Context7 when click's own state says click registered it. Malformed state is unknown ownership,
	// so the installer leaves Context7 and its state file untouched and reports a warning.
	context7Warning := ""
	runStep(
		"Context7",
		"Quitando Context7 (si click lo instaló)…",
		"Context7 procesado",
		true,
		func() error {
			var stepErr error
			context7Warning, stepErr = installer.RemoveContext7(cfg)
			return stepErr
		},
	)
	surfacePathWarning(out, r, context7Warning)

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
