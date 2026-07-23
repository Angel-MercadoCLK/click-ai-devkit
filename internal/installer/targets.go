package installer

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const targetSelectionSchemaVersion = 1

// TargetSelection is Click's target scope. Configured distinguishes an explicit user choice from
// the legacy default where OpenClaw follows live detection.
type TargetSelection struct {
	SchemaVersion int  `json:"schemaVersion"`
	Configured    bool `json:"configured"`
	Claude        bool `json:"claude"`
	OpenClaw      bool `json:"openclaw"`
	Codex         bool `json:"codex"`
}

func (c Config) TargetSelectionPath() string {
	return filepathJoinClickState(c.ClaudeHome, "targets.json")
}

func LoadTargetSelection(cfg Config) (TargetSelection, bool, error) {
	data, err := os.ReadFile(cfg.TargetSelectionPath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return TargetSelection{Claude: true, OpenClaw: true}, false, nil
		}
		return TargetSelection{}, false, fmt.Errorf("installer: read target selection: %w", err)
	}
	var selection TargetSelection
	if err := json.Unmarshal(data, &selection); err != nil {
		return TargetSelection{}, true, fmt.Errorf("installer: decode target selection: %w", err)
	}
	if selection.SchemaVersion != targetSelectionSchemaVersion {
		return TargetSelection{}, true, fmt.Errorf("installer: unsupported target selection schema version %d", selection.SchemaVersion)
	}
	if !selection.Configured {
		return TargetSelection{}, true, fmt.Errorf("installer: target selection artifact is not configured")
	}
	return selection, true, nil
}

func SaveTargetSelection(cfg Config, selection TargetSelection) error {
	if !selection.Configured {
		return fmt.Errorf("installer: target selection must be explicitly configured")
	}
	if !selection.Claude {
		return fmt.Errorf("installer: Claude Code must remain selected as the primary target")
	}
	selection.SchemaVersion = targetSelectionSchemaVersion
	data, err := json.MarshalIndent(selection, "", "  ")
	if err != nil {
		return fmt.Errorf("installer: encode target selection: %w", err)
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(cfg.TargetSelectionPath()), 0o755); err != nil {
		return fmt.Errorf("installer: create target selection directory: %w", err)
	}
	if err := atomicWriteFile(cfg.TargetSelectionPath(), data, 0o600); err != nil {
		return fmt.Errorf("installer: write target selection: %w", err)
	}
	return nil
}

func RemoveTargetSelection(cfg Config) error {
	if err := os.Remove(cfg.TargetSelectionPath()); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("installer: remove target selection: %w", err)
	}
	return nil
}

func ResolveOpenClawTarget(selection TargetSelection, detected bool) bool {
	if selection.Configured {
		return selection.OpenClaw && detected
	}
	return detected
}

// ResolveCodexTarget is explicit-only: no legacy selection artifact predates Codex.
func ResolveCodexTarget(selection TargetSelection, detected bool) bool {
	return selection.Configured && selection.Codex && detected
}

// filepathJoinClickState keeps Click-owned state grouped under the Claude home management area.
func filepathJoinClickState(claudeHome, name string) string {
	return filepath.Join(claudeHome, "click-ai-devkit", name)
}
