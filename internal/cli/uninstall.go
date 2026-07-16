package cli

import (
	"fmt"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/installer"
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

func runUninstall(cmd *cobra.Command) error {
	out := cmd.OutOrStdout()
	r := rendererFor(cmd, out)

	claudeHome, err := installer.ResolveClaudeHome()
	if err != nil {
		return err
	}
	cfg := installer.Config{ClaudeHome: claudeHome}

	if err := r.RunStep("Quitando plugins click-sdd, click-memory, click-review y click-skills…", "Plugins eliminados de Claude Code", func() error {
		return installer.RemoveMarketplacePlugins()
	}); err != nil {
		return err
	}

	if err := r.RunStep("Limpiando CLAUDE.md…", "Bloque de CLAUDE.md eliminado", func() error {
		return installer.StripManagedBlock(cfg.ClaudeMDPath())
	}); err != nil {
		return err
	}

	if err := r.RunStep("Quitando memory-guard…", "memory-guard eliminado", func() error {
		return installer.UnregisterMemoryGuardHook(cfg)
	}); err != nil {
		return err
	}

	// RemoveEngramPlugin only reverses Engram when click's own state says click installed it —
	// a pre-existing developer setup is left running untouched. It also independently reverses
	// click's own PATH mutation(s) (D-9, T4-1, T4-1 follow-up) when it recorded owning them; a
	// failure doing so is surfaced as engramPathWarning below rather than aborting the rest of the
	// uninstall. engramPathWarning and a fatal err are NOT mutually exclusive — RemoveEngramPlugin
	// can fail a LATER, unrelated step (e.g. uninstalling the plugin itself) after already computing
	// a non-empty pathWarning from an earlier PATH-removal failure — so it must be surfaced on BOTH
	// the error path and the success path below, not only after RunStep succeeds (T4-1 follow-up
	// fix: it used to be dropped on the error path).
	engramPathWarning := ""
	if err := r.RunStep("Quitando Engram (si click lo instaló)…", "Engram procesado", func() error {
		var pathErr error
		engramPathWarning, pathErr = removeEngramPluginFunc(cfg)
		return pathErr
	}); err != nil {
		surfacePathWarning(out, r, engramPathWarning)
		return err
	}
	surfacePathWarning(out, r, engramPathWarning)

	// RemoveContext7 mirrors RemoveEngramPlugin's exact respect-ownership contract: only removes
	// Context7 when click's own state says click registered it.
	if err := r.RunStep("Quitando Context7 (si click lo instaló)…", "Context7 procesado", func() error {
		return installer.RemoveContext7(cfg)
	}); err != nil {
		return err
	}

	fmt.Fprintln(out, r.Info("Desinstalación completa."))
	return nil
}
