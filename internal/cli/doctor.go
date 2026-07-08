package cli

import (
	"errors"
	"fmt"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/doctor"
	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/installer"
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

	if !report.Healthy() {
		fmt.Fprintln(out, r.Fail("click-ai-devkit no está instalado correctamente"))
		return errUnhealthy
	}

	fmt.Fprintln(out, r.Success("click-ai-devkit está instalado correctamente"))
	return nil
}
