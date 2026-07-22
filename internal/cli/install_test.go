package cli

import (
	"strings"
	"testing"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/installer"
	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/manifest"
)

// TestInstallCommand_CloudConfigured_RunsCloudStepAfterEngram is task 4.3's RED test: when cloud
// server/project/token are all present, `click install` must run the Engram Cloud enrollment step
// right after the local Engram step, using Spanish user-facing labels.
func TestInstallCommand_CloudConfigured_RunsCloudStepAfterEngram(t *testing.T) {
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

	out, err := execRoot(t, home, "install")
	if err != nil {
		t.Fatalf("install command error = %v, output:\n%s", err, out)
	}
	if cloudCalls != 1 {
		t.Fatalf("SyncEngramCloud called %d times, want 1", cloudCalls)
	}
	if !strings.Contains(out, "Enrolando Engram Cloud") {
		t.Fatalf("install output = %q, want it to contain the Engram Cloud running label", out)
	}
	if !strings.Contains(out, "Engram Cloud enrolado") {
		t.Fatalf("install output = %q, want it to contain the Engram Cloud success label", out)
	}
}

// TestInstallCommand_CloudNotConfigured_SkipsCloudStep is task 4.3's no-config backward-compat test:
// when cloud config is incomplete, `click install` must not call SyncEngramCloud and must not add
// any cloud-related preview or runtime step.
func TestInstallCommand_CloudNotConfigured_SkipsCloudStep(t *testing.T) {
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

	out, err := execRoot(t, home, "install")
	if err != nil {
		t.Fatalf("install command error = %v, output:\n%s", err, out)
	}
	if cloudCalls != 0 {
		t.Fatalf("SyncEngramCloud called %d times, want 0 when cloud config is absent", cloudCalls)
	}
	if strings.Contains(out, "Cloud") {
		t.Fatalf("install output contains cloud-related text when not configured: %q", out)
	}
}

// TestInstallCommand_CloudConfigured_PartialTokenMissing_SkipsCloudStep is task 4.3's partial-config
// test: server+project without token must be treated as not-enrolled, with zero cloud calls.
func TestInstallCommand_CloudConfigured_PartialTokenMissing_SkipsCloudStep(t *testing.T) {
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

	out, err := execRoot(t, home, "install")
	if err != nil {
		t.Fatalf("install command error = %v, output:\n%s", err, out)
	}
	if cloudCalls != 0 {
		t.Fatalf("SyncEngramCloud called %d times, want 0 when token is missing", cloudCalls)
	}
	if !strings.Contains(out, "falta ENGRAM_CLOUD_TOKEN") {
		t.Fatalf("install output = %q, want it to report missing ENGRAM_CLOUD_TOKEN", out)
	}
	if !strings.Contains(out, "Se omite la inscripción en la nube") {
		t.Fatalf("install output = %q, want it to report skipped cloud enrollment", out)
	}
	if strings.Contains(out, "Enrolando Engram Cloud") || strings.Contains(out, "Engram Cloud enrolado") {
		t.Fatalf("install output = %q, must not show cloud enrollment step labels when token is missing", out)
	}
}

// TestInstallCommand_CloudConfigured_EnrollmentFailureIsNonFatal is resilience fix W1: an Engram
// Cloud enrollment failure must be NON-FATAL to `click install`. Cloud is opt-in and supplementary,
// so a flaky/unreachable cloud server must never abort an otherwise-valid local install. The command
// must (a) return nil, (b) surface a Spanish warning containing the underlying error, and (c) still
// run the purely-local steps that follow the cloud step (CLAUDE.md managed block, completion line).
func TestInstallCommand_CloudConfigured_EnrollmentFailureIsNonFatal(t *testing.T) {
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

	out, err := execRoot(t, home, "install")
	if err != nil {
		t.Fatalf("install command error = %v, want nil (cloud failure must be non-fatal), output:\n%s", err, out)
	}
	if !strings.Contains(out, "No se pudo sincronizar Engram Cloud") {
		t.Fatalf("install output = %q, want it to contain the Spanish cloud-failure warning", out)
	}
	if !strings.Contains(out, errTestEngramCloud.Error()) {
		t.Fatalf("install output = %q, want the warning to include the underlying error %q", out, errTestEngramCloud.Error())
	}
	if !strings.Contains(out, "Instalación completa.") {
		t.Fatalf("install output = %q, want the command to continue to completion after cloud failure", out)
	}
	// The CLAUDE.md managed block is written AFTER the cloud step in runInstall — its presence proves
	// the local steps still ran despite the cloud failure.
	has, hErr := installer.HasManagedBlock(installer.Config{ClaudeHome: home}.ClaudeMDPath())
	if hErr != nil {
		t.Fatalf("HasManagedBlock error = %v", hErr)
	}
	if !has {
		t.Fatalf("CLAUDE.md managed block missing after cloud failure — local steps did not run")
	}
}

var errTestEngramCloud = &cloudError{msg: "engram cloud enrollment failed"}

type cloudError struct{ msg string }

func (e *cloudError) Error() string { return e.msg }
