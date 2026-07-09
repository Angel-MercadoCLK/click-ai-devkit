package installer

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/modelconfig"
)

// SaveModels persists the resolved per-phase click-sdd model selection (D25) to
// cfg.ModelsPath(), so `click update` can re-apply the same --config flags and `click doctor` can
// report what's configured. It overwrites any previous file.
func SaveModels(cfg Config, models map[modelconfig.Phase]string) error {
	data := make(map[string]string, len(models))
	for phase, model := range models {
		data[string(phase)] = model
	}
	if err := writeJSONFile(cfg.ModelsPath(), data); err != nil {
		return fmt.Errorf("installer: write models.json: %w", err)
	}
	return nil
}

// LoadModels reads the per-phase model selection written by SaveModels. It returns
// (nil, false, nil) when models.json doesn't exist yet (e.g. before the first `click install`),
// so callers can distinguish "never configured" from a real read/parse error.
func LoadModels(cfg Config) (map[modelconfig.Phase]string, bool, error) {
	data, err := os.ReadFile(cfg.ModelsPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("installer: read models.json: %w", err)
	}
	var raw map[string]string
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, false, fmt.Errorf("installer: parse models.json: %w", err)
	}
	models := make(map[modelconfig.Phase]string, len(raw))
	for k, v := range raw {
		models[modelconfig.Phase(k)] = v
	}
	return models, true, nil
}
