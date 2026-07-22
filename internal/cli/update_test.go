package cli

import (
	"strings"
	"testing"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/installer"
	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/manifest"
)

// TestUpdateCommand_CloudConfigured_RunsCloudStepAfterEngram is task 4.5's RED test: when cloud
// server/project/token are all present, `click update` must re-sync Engram Cloud right after the
// local Engram pin step, using Spanish user-facing labels.
func TestUpdateCommand_CloudConfigured_RunsCloudStepAfterEngram(t *testing.T) {
	home := t.TempDir()
	runner := newTestCommandRunner(home)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()
	seedResolvableEngram(t)

	cloudCalls := 0
	restoreCloud := SetSyncEngramCloudFuncForTests(func(cfg installer.Config, m *manifest.Manifest) error {
		cloudCalls++
		return nil
	})
	defer restoreCloud()

	t.Setenv("ENGRAM_CLOUD_TOKEN", "cloud-token")
	t.Setenv("CLICK_ENGRAM_CLOUD_SERVER", "http://127.0.0.1:18080")
	t.Setenv("CLICK_ENGRAM_CLOUD_PROJECT", "click-ai-devkit")

	out, err := execRoot(t, home, "update")
	if err != nil {
		t.Fatalf("update command error = %v, output:\n%s", err, out)
	}
	if cloudCalls != 1 {
		t.Fatalf("SyncEngramCloud called %d times, want 1", cloudCalls)
	}
	if !strings.Contains(out, "Sincronizando Engram Cloud") {
		t.Fatalf("update output = %q, want it to contain the Engram Cloud running label", out)
	}
	if !strings.Contains(out, "Engram Cloud sincronizado") {
		t.Fatalf("update output = %q, want it to contain the Engram Cloud success label", out)
	}
}

// TestUpdateCommand_CloudNotConfigured_SkipsCloudStep is task 4.5's no-config backward-compat test.
func TestUpdateCommand_CloudNotConfigured_SkipsCloudStep(t *testing.T) {
	home := t.TempDir()
	runner := newTestCommandRunner(home)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()

	cloudCalls := 0
	restoreCloud := SetSyncEngramCloudFuncForTests(func(cfg installer.Config, m *manifest.Manifest) error {
		cloudCalls++
		return nil
	})
	defer restoreCloud()

	out, err := execRoot(t, home, "update")
	if err != nil {
		t.Fatalf("update command error = %v, output:\n%s", err, out)
	}
	if cloudCalls != 0 {
		t.Fatalf("SyncEngramCloud called %d times, want 0 when cloud config is absent", cloudCalls)
	}
	if strings.Contains(out, "Cloud") {
		t.Fatalf("update output contains cloud-related text when not configured: %q", out)
	}
}

// TestUpdateCommand_CloudConfigured_PartialTokenMissing_SkipsCloudStep is task 4.5's partial-config
// test: server+project without token must be treated as not-enrolled.
func TestUpdateCommand_CloudConfigured_PartialTokenMissing_SkipsCloudStep(t *testing.T) {
	home := t.TempDir()
	runner := newTestCommandRunner(home)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()

	cloudCalls := 0
	restoreCloud := SetSyncEngramCloudFuncForTests(func(cfg installer.Config, m *manifest.Manifest) error {
		cloudCalls++
		return nil
	})
	defer restoreCloud()

	t.Setenv("CLICK_ENGRAM_CLOUD_SERVER", "http://127.0.0.1:18080")
	t.Setenv("CLICK_ENGRAM_CLOUD_PROJECT", "click-ai-devkit")
	// ENGRAM_CLOUD_TOKEN intentionally absent.

	out, err := execRoot(t, home, "update")
	if err != nil {
		t.Fatalf("update command error = %v, output:\n%s", err, out)
	}
	if cloudCalls != 0 {
		t.Fatalf("SyncEngramCloud called %d times, want 0 when token is missing", cloudCalls)
	}
	if !strings.Contains(out, "falta ENGRAM_CLOUD_TOKEN") {
		t.Fatalf("update output = %q, want it to report missing ENGRAM_CLOUD_TOKEN", out)
	}
	if !strings.Contains(out, "Se omite la inscripción en la nube") {
		t.Fatalf("update output = %q, want it to report skipped cloud enrollment", out)
	}
	if strings.Contains(out, "Sincronizando Engram Cloud") || strings.Contains(out, "Engram Cloud sincronizado") {
		t.Fatalf("update output = %q, must not show cloud re-sync step labels when token is missing", out)
	}
}

// TestUpdateCommand_CloudConfigured_ReSyncFailureIsNonFatal is resilience fix W1: an Engram Cloud
// re-sync failure must be NON-FATAL to `click update`. The command must (a) return nil, (b) surface a
// Spanish warning containing the underlying error, and (c) still run the remaining steps through to
// completion (Context7 sync and the completion line follow the cloud step in runUpdate).
func TestUpdateCommand_CloudConfigured_ReSyncFailureIsNonFatal(t *testing.T) {
	home := t.TempDir()
	runner := newTestCommandRunner(home)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()
	seedResolvableEngram(t)

	restoreCloud := SetSyncEngramCloudFuncForTests(func(cfg installer.Config, m *manifest.Manifest) error {
		return errTestEngramCloud
	})
	defer restoreCloud()

	t.Setenv("ENGRAM_CLOUD_TOKEN", "cloud-token")
	t.Setenv("CLICK_ENGRAM_CLOUD_SERVER", "http://127.0.0.1:18080")
	t.Setenv("CLICK_ENGRAM_CLOUD_PROJECT", "click-ai-devkit")

	out, err := execRoot(t, home, "update")
	if err != nil {
		t.Fatalf("update command error = %v, want nil (cloud failure must be non-fatal), output:\n%s", err, out)
	}
	if !strings.Contains(out, "No se pudo sincronizar Engram Cloud") {
		t.Fatalf("update output = %q, want it to contain the Spanish cloud-failure warning", out)
	}
	if !strings.Contains(out, errTestEngramCloud.Error()) {
		t.Fatalf("update output = %q, want the warning to include the underlying error %q", out, errTestEngramCloud.Error())
	}
	if !strings.Contains(out, "Context7 sincronizado") {
		t.Fatalf("update output = %q, want the steps after the cloud step to still run", out)
	}
	if !strings.Contains(out, "Update completo.") {
		t.Fatalf("update output = %q, want the command to continue to completion after cloud failure", out)
	}
	// The CLAUDE.md managed block is written by runUpdate too — its presence confirms the local
	// pipeline completed regardless of the cloud failure.
	has, hErr := installer.HasManagedBlock(installer.Config{ClaudeHome: home}.ClaudeMDPath())
	if hErr != nil {
		t.Fatalf("HasManagedBlock error = %v", hErr)
	}
	if !has {
		t.Fatalf("CLAUDE.md managed block missing after cloud failure — local steps did not run")
	}
}
