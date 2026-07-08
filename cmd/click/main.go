// Command click is the click-ai-devkit CLI entrypoint. It wires the cobra command tree defined
// in internal/cli and does nothing else — click is a thin installer/manager, not the
// orchestration brain (tech-spec.md §1 "Thin CLI, not the orchestration brain").
package main

import (
	"errors"
	"os"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/cli"
)

func main() {
	if err := cli.NewRootCommand().Execute(); err != nil {
		var exitErr interface{ ExitCode() int }
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.ExitCode())
		}
		os.Exit(1)
	}
}
