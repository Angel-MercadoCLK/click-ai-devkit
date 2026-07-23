package installer

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ConfigureOpenClawModels delegates native model configuration to OpenClaw's documented CLI. It
// intentionally does not read or rewrite openclaw.json, preserving JSON5/comments/includes and
// letting OpenClaw enforce its strict schema and hot-reload behavior.
func ConfigureOpenClawModels(primary string, fallbacks []string) error {
	if err := validateOpenClawModelRef(primary); err != nil {
		return fmt.Errorf("installer: modelo primario de OpenClaw: %w", err)
	}
	for _, fallback := range fallbacks {
		if err := validateOpenClawModelRef(fallback); err != nil {
			return fmt.Errorf("installer: modelo alternativo de OpenClaw: %w", err)
		}
	}
	if !OpenClawAvailable() {
		return fmt.Errorf("installer: OpenClaw no está disponible en PATH; instale OpenClaw y vuelva a ejecutar `click configure-openclaw-model`")
	}

	runner := commandRunnerFactory()
	if err := runner.Run("openclaw", "config", "set", "agents.defaults.model.primary", primary); err != nil {
		return fmt.Errorf("installer: no se pudo configurar el modelo primario de OpenClaw: %w", err)
	}
	if len(fallbacks) == 0 {
		return nil
	}
	fallbackJSON, err := json.Marshal(fallbacks)
	if err != nil {
		return fmt.Errorf("installer: no se pudieron codificar los modelos alternativos de OpenClaw: %w", err)
	}
	if err := runner.Run("openclaw", "config", "set", "agents.defaults.model.fallbacks", string(fallbackJSON), "--strict-json"); err != nil {
		return fmt.Errorf("installer: no se pudieron configurar los modelos alternativos de OpenClaw: %w", err)
	}
	return nil
}

func validateOpenClawModelRef(ref string) error {
	if strings.TrimSpace(ref) != ref || strings.Count(ref, "/") != 1 {
		return fmt.Errorf("la referencia de modelo %q debe tener el formato provider/model", ref)
	}
	parts := strings.SplitN(ref, "/", 2)
	if parts[0] == "" || parts[1] == "" {
		return fmt.Errorf("la referencia de modelo %q debe tener el formato provider/model", ref)
	}
	return nil
}
