package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/doctor"
	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/installer"
	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/modelconfig"
	"github.com/spf13/cobra"
)

// errUnhealthy is returned when any doctor check fails, so main.go's os.Exit(1) on a non-nil
// Execute() error gives `click doctor` a non-zero exit code without cobra also printing its own
// generic "Error: ..." line (SilenceErrors is set below — our own Fail lines already say why).
var errUnhealthy = errors.New("click-ai-devkit install is unhealthy")

func newDoctorCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Read-only health check of the click-ai-devkit install",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDoctor(cmd)
		},
	}
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	return cmd
}

func runDoctor(cmd *cobra.Command) error {
	out := cmd.OutOrStdout()
	r := rendererFor(cmd, out)

	claudeHome, err := installer.ResolveClaudeHome()
	if err != nil {
		return err
	}
	cfg := installer.Config{ClaudeHome: claudeHome}

	report := doctor.Run(cfg)
	for _, c := range report.Checks {
		line := fmt.Sprintf("%s: %s", c.Name, c.Detail)
		if c.Healthy {
			fmt.Fprintln(out, r.Success(line))
		} else {
			fmt.Fprintln(out, r.Fail(line))
		}
	}

	modelsLine, err := formatModelsLine(cfg)
	if err != nil {
		return err
	}
	fmt.Fprintln(out, r.Info(modelsLine))

	if !report.Healthy() {
		fmt.Fprintln(out, r.Fail("click-ai-devkit no está instalado correctamente"))
		return errUnhealthy
	}

	fmt.Fprintln(out, r.Success("click-ai-devkit está instalado correctamente"))
	return nil
}

// formatModelsLine reports the click-sdd per-phase models currently configured (D25): the
// persisted models.json selection if `click install`/`click update` ever ran, or an explicit
// "defaults" line otherwise, in modelconfig.Phases order for a stable, readable report.
func formatModelsLine(cfg installer.Config) (string, error) {
	models, found, err := installer.LoadModels(cfg)
	if err != nil {
		return "", err
	}
	if !found {
		return "Modelos por fase de click-sdd: defaults", nil
	}
	parts := make([]string, 0, len(modelconfig.Phases))
	for _, phase := range modelconfig.Phases {
		model, ok := models[phase]
		if !ok || model == "" {
			model = modelconfig.Defaults()[phase]
		}
		parts = append(parts, string(phase)+"="+model)
	}
	return "Modelos por fase de click-sdd: " + strings.Join(parts, ", "), nil
}
