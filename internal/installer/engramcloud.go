// Engram Cloud opt-in enrollment support. Composed alongside engram.go, not a rewrite.
// Runs only when server+project+ENGRAM_CLOUD_TOKEN are present; otherwise local-only no-op.
// Token is checked for presence only and never captured, argv'd, logged, or persisted.
package installer

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/manifest"
)

const (
	engramCloudServerEnvOverride  = "CLICK_ENGRAM_CLOUD_SERVER"
	engramCloudProjectEnvOverride = "CLICK_ENGRAM_CLOUD_PROJECT"
	engramCloudTokenEnv           = "ENGRAM_CLOUD_TOKEN"
)

// engramCloudState is click's own non-secret bookkeeping about Engram Cloud enrollment. It does
// NOT contain the cloud token; the token remains env-only.
type engramCloudState struct {
	Enrolled bool   `json:"enrolled"`
	Server   string `json:"server"`
	Project  string `json:"project"`
	LastSync string `json:"last_sync"`
}

// resolveEngramCloudConfig resolves the effective cloud server and project, and whether the cloud
// token is present in the environment. Manifest defaults are applied first, then env overrides
// (CLICK_ENGRAM_CLOUD_SERVER / CLICK_ENGRAM_CLOUD_PROJECT). The token is only checked for presence
// via os.Getenv; its value is never captured.
func resolveEngramCloudConfig(cfg Config, m *manifest.Manifest) (server, project string, tokenPresent bool) {
	server = m.EngramCloud.Server
	project = m.EngramCloud.Project
	if override := os.Getenv(engramCloudServerEnvOverride); override != "" {
		server = override
	}
	if override := os.Getenv(engramCloudProjectEnvOverride); override != "" {
		project = override
	}
	return server, project, os.Getenv(engramCloudTokenEnv) != ""
}

// SyncEngramCloud enrolls the local Engram client into the configured cloud project when cloud
// config and ENGRAM_CLOUD_TOKEN are all present. It is idempotent: the first call runs the full
// config -> enroll -> upgrade -> sync sequence, while later calls (when the local state says
// already enrolled) run only config -> sync. It returns a wrapped error and writes no state on any
// command failure, leaving local Engram state untouched.
func SyncEngramCloud(cfg Config, m *manifest.Manifest) error {
	server, project, tokenPresent := resolveEngramCloudConfig(cfg, m)
	if server == "" || project == "" || !tokenPresent {
		return nil
	}

	statePath := cfg.EngramCloudStatePath()
	if statePath == "" {
		// No ClaudeHome means there is nowhere to read or write state; no-op safely.
		return nil
	}

	existing, _, err := loadEngramCloudState(cfg)
	if err != nil {
		return err
	}

	runner := commandRunnerFactory()

	if !existing.Enrolled {
		// First-time enrollment with pre-existing local-only data: the Engram Cloud docs require an
		// explicit upgrade sequence before `engram sync --cloud` can safely push local observations
		// into the shared project. Automating it here guarantees the migration is not skipped.
		//
		// Re-entrancy contract (resilience W2): if a prior run died mid-sequence, state is NOT written
		// (Enrolled stays false), so the next run re-runs the FULL sequence below rather than the
		// short already-enrolled path. This is safe by design: the sequence runs
		// `engram cloud upgrade doctor -> repair -> bootstrap` before `sync`, and doctor+repair
		// reconcile any partial/inconsistent state left by the interrupted run. Combined with cloud
		// enrollment now being non-fatal at the CLI layer (resilience W1), a partial failure never
		// breaks the local install/update and is simply retried on the next `click update`.
		steps := []struct {
			name string
			args []string
		}{
			{"engram cloud config", []string{"cloud", "config", "--server", server}},
			{"engram cloud enroll", []string{"cloud", "enroll", project}},
			{"engram cloud upgrade doctor", []string{"cloud", "upgrade", "doctor"}},
			{"engram cloud upgrade repair", []string{"cloud", "upgrade", "repair"}},
			{"engram cloud upgrade bootstrap", []string{"cloud", "upgrade", "bootstrap"}},
			{"engram sync", []string{"sync", "--cloud", "--project", project}},
		}
		for _, step := range steps {
			if err := runEngramCloudStep(runner, step.name, step.args...); err != nil {
				return err
			}
		}

		state := engramCloudState{
			Enrolled: true,
			Server:   server,
			Project:  project,
			LastSync: time.Now().UTC().Format(time.RFC3339),
		}
		if err := writeJSONFile(statePath, state); err != nil {
			return fmt.Errorf("installer: write engram cloud state: %w", err)
		}
		return nil
	}

	// Already enrolled: re-apply server config and sync. No re-enroll, no re-upgrade.
	if err := runEngramCloudStep(runner, "engram cloud config", "cloud", "config", "--server", server); err != nil {
		return err
	}
	if err := runEngramCloudStep(runner, "engram sync", "sync", "--cloud", "--project", project); err != nil {
		return err
	}
	return nil
}

// runEngramCloudStep runs one engram subcommand through the injectable runner and returns a
// wrapped, human-readable error. It centralizes the "fail-stop on first error" behavior shared by
// both the first-time enrollment chain and the idempotent re-sync path.
func runEngramCloudStep(runner CommandRunner, stepName string, args ...string) error {
	if err := runner.Run("engram", args...); err != nil {
		return fmt.Errorf("installer: %s failed: %w", stepName, err)
	}
	return nil
}

// RemoveEngramCloudState reverses the only file SyncEngramCloud can write: click's own
// engram-cloud.json enrollment record. It is deliberately offline and non-destructive — it never
// shells out to `engram cloud` to un-enroll the shared project (that would require a token and would
// mutate the shared hive memory that other machines still depend on). It is idempotent: a missing
// file (or an installer with no ClaudeHome) is a silent no-op, matching the reversal contract of the
// other Remove* helpers Uninstall composes.
func RemoveEngramCloudState(cfg Config) error {
	path := cfg.EngramCloudStatePath()
	if path == "" {
		return nil
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("installer: remove engram cloud state: %w", err)
	}
	return nil
}

// loadEngramCloudState reads click's Engram Cloud enrollment state. It returns a zero state and
// found=false when the file is absent, matching loadEngramState's contract.
func loadEngramCloudState(cfg Config) (engramCloudState, bool, error) {
	path := cfg.EngramCloudStatePath()
	if path == "" {
		return engramCloudState{}, false, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return engramCloudState{}, false, nil
		}
		return engramCloudState{}, false, fmt.Errorf("installer: read engram cloud state: %w", err)
	}
	var state engramCloudState
	if err := json.Unmarshal(data, &state); err != nil {
		return engramCloudState{}, false, fmt.Errorf("installer: parse engram cloud state: %w", err)
	}
	return state, true, nil
}
