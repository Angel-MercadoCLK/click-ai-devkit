package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/installer"
	"github.com/spf13/cobra"
)

func newPluginsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "plugins",
		Short: "Lista los plugins gestionados por Click y sus registros",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPlugins(cmd)
		},
	}
	cmd.SilenceUsage = true
	return cmd
}

func runPlugins(cmd *cobra.Command) error {
	out := cmd.OutOrStdout()
	r := rendererFor(cmd, out)
	claudeHome, err := installer.ResolveClaudeHome()
	if err != nil {
		return err
	}
	cfg := installer.Config{ClaudeHome: claudeHome}
	plugins, err := installer.ListClickPlugins(cfg)
	if err != nil {
		return err
	}

	fmt.Fprintln(out, r.Info("Plugins gestionados por Click"))
	fmt.Fprintln(out, r.Info("Repositorio del marketplace: "+installer.ClickMarketplaceSource()))
	fmt.Fprintln(out, r.Info("Staging local para futuros plugins: "+cfg.ClickPluginStagingDir()))
	if _, err := os.Stat(cfg.ClickPluginStagingDir()); os.IsNotExist(err) {
		fmt.Fprintln(out, r.Info("  El staging todavía no existe; este comando no lo crea ni modifica."))
	} else if err != nil {
		fmt.Fprintln(out, r.Warn("  No se pudo inspeccionar el staging: "+err.Error()))
	}
	fmt.Fprintln(out, r.Info("Fuente del repositorio: "+strings.TrimRight(installer.ClickMarketplaceSource(), "/")+"/plugins"))

	for _, plugin := range plugins {
		known := "no conocido en el registro"
		if plugin.Known {
			known = "conocido en el registro"
		}
		installed := "no instalado"
		if plugin.Installed {
			// Distinguish registered-and-enabled from registered-but-disabled so this view can never
			// contradict `click doctor`, which reports a registered-but-disabled plugin as inactive
			// (HasInstalledPlugin requires enabledPlugins[id]==true).
			if plugin.Enabled {
				installed = "instalado y habilitado"
			} else {
				installed = "registrado pero deshabilitado — ejecute `click update`"
			}
			if plugin.InstallPath != "" {
				installed += " en " + plugin.InstallPath
			}
		}
		fmt.Fprintf(out, "  %s: %s; %s\n", plugin.Name, known, installed)
	}

	fmt.Fprintln(out, r.Info("Los registros ausentes se muestran como estado vacío; no se ejecuta ningún CLI nativo."))
	fmt.Fprintln(out, r.Warn("Agregar un plugin al staging no lo instala ni lo activa. La activación requiere el flujo nativo del target y todavía no se implementa aquí."))
	return nil
}
