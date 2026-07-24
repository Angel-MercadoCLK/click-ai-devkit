package installer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	toml "github.com/pelletier/go-toml/v2"
)

// ConfigureCodexModel changes only Codex's documented user-level `model` key. It is called only
// after the user explicitly selected/configured a model.
//
// It uses a real TOML library (github.com/pelletier/go-toml/v2) rather than a hand-rolled
// line-by-line editor: read config.toml (empty/absent → start from an empty table), unmarshal into
// a map, set the top-level "model" key, marshal back, and atomically write it. This is semantically
// safe on ALL inputs — it can never emit invalid TOML, never corrupts a multi-line array or a
// multi-line basic string that happens to contain a `model =` line, and collapses a quoted-key
// `"model"` and a bare `model` into the single key TOML already considers them to be.
//
// ACCEPTED TRADEOFF: marshaling normalizes formatting and drops comments. That is predictable and
// recoverable (config.toml is backed up in the run snapshot — see the codex-model Step's Snapshot
// list in plan.go), and is strictly better than the previous editor's silent corruption.
func ConfigureCodexModel(codexHome, model string) error {
	model = strings.TrimSpace(model)
	if model == "" {
		return fmt.Errorf("installer: Codex native mutation requires an explicit model; re-run with --codex-model <model>")
	}
	if strings.ContainsAny(model, "\r\n") {
		return fmt.Errorf("installer: Codex model must not contain line breaks")
	}

	path := filepath.Join(codexHome, "config.toml")
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("installer: read Codex config.toml: %w", err)
	}

	config := map[string]any{}
	if len(data) > 0 {
		if err := toml.Unmarshal(data, &config); err != nil {
			return fmt.Errorf("installer: parse Codex config.toml: %w", err)
		}
	}
	config["model"] = model

	encoded, err := toml.Marshal(config)
	if err != nil {
		return fmt.Errorf("installer: encode Codex config.toml: %w", err)
	}

	if err := os.MkdirAll(codexHome, 0o755); err != nil {
		return fmt.Errorf("installer: create CODEX_HOME: %w", err)
	}
	if err := atomicWriteFile(path, encoded, 0o600); err != nil {
		return fmt.Errorf("installer: write Codex config.toml: %w", err)
	}
	return nil
}
