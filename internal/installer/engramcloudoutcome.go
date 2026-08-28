package installer

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

const (
	EngramCloudImportOutcomeSuccess = "success"
	EngramCloudImportOutcomeFailure = "failure"
	EngramCloudImportOutcomeTimeout = "timeout"
)

// EngramCloudImportOutcome is the last non-secret result of the managed cloud import.
// Reason is deliberately selected by Click rather than copied from subprocess output, which
// could contain a token or another secret echoed by a remote server.
type EngramCloudImportOutcome struct {
	Timestamp time.Time `json:"timestamp"`
	Status    string    `json:"status"`
	Reason    string    `json:"reason,omitempty"`
}

func normalizedEngramCloudImportOutcome(outcome EngramCloudImportOutcome) EngramCloudImportOutcome {
	switch outcome.Status {
	case EngramCloudImportOutcomeSuccess:
		outcome.Reason = ""
	case EngramCloudImportOutcomeFailure:
		outcome.Reason = "import command failed"
	case EngramCloudImportOutcomeTimeout:
		outcome.Reason = "import timed out"
	default:
		outcome.Reason = ""
	}
	return outcome
}

// WriteEngramCloudImportOutcome atomically stores Click's local import result.
func WriteEngramCloudImportOutcome(cfg Config, outcome EngramCloudImportOutcome) error {
	path := cfg.EngramCloudImportOutcomePath()
	if path == "" {
		return nil
	}
	return writeJSONFile(path, normalizedEngramCloudImportOutcome(outcome))
}

// LoadEngramCloudImportOutcome reads the latest Click-owned import result.
func LoadEngramCloudImportOutcome(cfg Config) (EngramCloudImportOutcome, bool, error) {
	path := cfg.EngramCloudImportOutcomePath()
	if path == "" {
		return EngramCloudImportOutcome{}, false, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return EngramCloudImportOutcome{}, false, nil
		}
		return EngramCloudImportOutcome{}, false, fmt.Errorf("installer: read engram cloud import outcome: %w", err)
	}
	var outcome EngramCloudImportOutcome
	if err := json.Unmarshal(data, &outcome); err != nil {
		return EngramCloudImportOutcome{}, false, fmt.Errorf("installer: parse engram cloud import outcome: %w", err)
	}
	return outcome, true, nil
}

// RemoveEngramCloudImportOutcome removes only Click's local import bookkeeping.
func RemoveEngramCloudImportOutcome(cfg Config) error {
	path := cfg.EngramCloudImportOutcomePath()
	if path == "" {
		return nil
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("installer: remove engram cloud import outcome: %w", err)
	}
	return nil
}
