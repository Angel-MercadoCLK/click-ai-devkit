package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/doctor"
	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/installer"
	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/manifest"
	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/modelconfig"
	toml "github.com/pelletier/go-toml/v2"
	"github.com/spf13/cobra"
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

func TestInstall_MalformedSettingsDoesNotAbortRun(t *testing.T) {
	home := t.TempDir()
	settingsPath := filepath.Join(home, "settings.json")
	if err := os.WriteFile(settingsPath, []byte("{not valid json"), 0o600); err != nil {
		t.Fatalf("seed malformed settings.json: %v", err)
	}

	runner := newTestCommandRunner(home)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()
	seedResolvableEngram(t)

	out, err := execRoot(t, home, "install")
	if err != nil {
		t.Fatalf("install command error = %v, want nil; output:\n%s", err, out)
	}
	installedSettingsPath := filepath.Join(os.Getenv("CLICK_CLAUDE_HOME"), "settings.json")
	if !strings.Contains(out, "Se omitió el respaldo de settings.json") || !strings.Contains(out, installedSettingsPath) {
		t.Fatalf("install output = %q, want malformed-settings backup warning naming %q", out, installedSettingsPath)
	}
	if !strings.Contains(out, "CLAUDE.md actualizado") {
		t.Fatalf("install output = %q, want a later configured step to run", out)
	}
}

// TestInstallCommand_CloudNotConfigured_SkipsCloudStep is task 4.3's no-config backward-compat test:
// when cloud config is incomplete, `click install` must not call SyncEngramCloud and must not add
// any cloud-related preview or runtime step.
// The shipped manifest.yaml bakes in the real (non-secret) engram_cloud.server/project defaults
// (manifest.EngramCloud{}), so "not configured" can no longer mean "click has no idea a cloud
// exists" for a normal build — it means the token specifically is absent, which is exactly
// TestInstallCommand_CloudConfigured_PartialTokenMissing_SkipsCloudStep's scenario below: an
// informative Spanish skip message is expected now, not total silence about cloud.
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
		t.Fatalf("SyncEngramCloud called %d times, want 0 when the token is absent", cloudCalls)
	}
	if !strings.Contains(out, "falta ENGRAM_CLOUD_TOKEN") {
		t.Fatalf("install output = %q, want it to report missing ENGRAM_CLOUD_TOKEN (shipped manifest defaults now supply server/project)", out)
	}
	if strings.Contains(out, "Enrolando Engram Cloud") || strings.Contains(out, "Engram Cloud enrolado") {
		t.Fatalf("install output = %q, must not show cloud enrollment step labels when token is missing", out)
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

// TestInstall_ConfigureFailureWarnsAndContinues is task 5.15's RED test: a failing
// configureEngramCloudSessionSyncFunc must leave install exiting 0 with a Spanish warning (ECS-10.1,
// ECS-10.2), and a recording order assertion shows configure runs before enrollment (DD-4 ordering).
// It covers both the CloudResolvable-without-token dispatch path (the env block and hook must still
// be written so a later token export activates everything — the whole point of the rename) and the
// persisted-token path (configure before enroll).
func TestInstall_ConfigureFailureWarnsAndContinues(t *testing.T) {
	home := t.TempDir()
	configureCLIExecutionHomes(t, home, "", "", []string{"install"})
	t.Setenv("CLICK_STATE_HOME", t.TempDir())
	seedResolvableGit(t)

	runner := newTestCommandRunner(home)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()

	configureCalls := 0
	restoreConfigure := installer.SetConfigureEngramCloudSessionSyncFuncForTests(func(cfg installer.Config, m *manifest.Manifest, mode installer.CloudTokenPersistence, token string) error {
		configureCalls++
		return errTestCloudSettings
	})
	defer restoreConfigure()

	cloudCalls := 0
	restoreCloud := SetSyncEngramCloudFuncForTests(func(cfg installer.Config, m *manifest.Manifest) error {
		cloudCalls++
		return nil
	})
	defer restoreCloud()

	t.Run("token-absent-still-writes-session-sync-via-dispatch", func(t *testing.T) {
		configureCalls = 0
		cloudCalls = 0
		t.Setenv("CLICK_ENGRAM_CLOUD_SERVER", "http://127.0.0.1:18080")
		t.Setenv("CLICK_ENGRAM_CLOUD_PROJECT", "click-ai-devkit")
		// ENGRAM_CLOUD_TOKEN intentionally absent.
		root := NewRootCommand()
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&out)
		root.SetIn(&bytes.Buffer{})
		root.SetArgs([]string{"install"})

		if err := root.Execute(); err != nil {
			t.Fatalf("install command error = %v, want nil (cloud settings failure must be non-fatal), output:\n%s", err, out.String())
		}
		if err := syncStandardTargetFixtures(); err != nil {
			t.Fatal(err)
		}
		if configureCalls != 1 {
			t.Fatalf("ConfigureEngramCloudSessionSync called %d times, want 1 (CloudResolvable without token must still write env block and hook)", configureCalls)
		}
		if !strings.Contains(out.String(), errTestCloudSettings.Error()) {
			t.Fatalf("install output = %q, want the warning to include the underlying error %q", out.String(), errTestCloudSettings.Error())
		}
		if !strings.Contains(out.String(), "Instalación completa.") {
			t.Fatalf("install output = %q, want the command to continue to completion after the configure failure", out.String())
		}
	})

	t.Run("persist-order-configure-before-enrollment", func(t *testing.T) {
		configureCalls = 0
		cloudCalls = 0
		t.Setenv("CLICK_ENGRAM_CLOUD_SERVER", "http://127.0.0.1:18080")
		t.Setenv("CLICK_ENGRAM_CLOUD_PROJECT", "click-ai-devkit")
		t.Setenv("ENGRAM_CLOUD_TOKEN", "test-token")

		var order []string
		restoreConfigure2 := installer.SetConfigureEngramCloudSessionSyncFuncForTests(func(cfg installer.Config, m *manifest.Manifest, mode installer.CloudTokenPersistence, token string) error {
			configureCalls++
			order = append(order, "configure")
			return errTestCloudSettings
		})
		defer restoreConfigure2()
		restoreCloud2 := SetSyncEngramCloudFuncForTests(func(cfg installer.Config, m *manifest.Manifest) error {
			cloudCalls++
			order = append(order, "enrollment")
			return nil
		})
		defer restoreCloud2()

		root := NewRootCommand()
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&out)
		root.SetIn(&bytes.Buffer{})
		root.SetArgs([]string{"install", "--" + persistEngramCloudTokenFlag})

		if err := root.Execute(); err != nil {
			t.Fatalf("install command error = %v, want nil (cloud settings failure must be non-fatal)", err)
		}
		if err := syncStandardTargetFixtures(); err != nil {
			t.Fatal(err)
		}
		if len(order) != 2 || order[0] != "configure" || order[1] != "enrollment" {
			t.Fatalf("recorded order = %v, want [configure enrollment]", order)
		}
		if configureCalls != 1 {
			t.Fatalf("ConfigureEngramCloudSessionSync called %d times, want exactly 1", configureCalls)
		}
		if !strings.Contains(out.String(), "No se pudo configurar Engram Cloud Session Sync") {
			t.Fatalf("install output missing the Spanish cloud-settings warning")
		}
	})
}

var errTestCloudSettings = &cloudSettingsError{msg: "cloud settings write failed"}

type cloudSettingsError struct{ msg string }

func (e *cloudSettingsError) Error() string { return e.msg }

// TestInstall_EnrollmentRunnerFailureStillExitZero is task 5.17's ECS-10.7 proof: an Engram Cloud
// enrollment runner error leaves install exiting 0 with the Spanish warning and the local install
// complete. The dedicated opt-in authorizes the enrollment to run (DD-3).
func TestInstall_EnrollmentRunnerFailureStillExitZero(t *testing.T) {
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

	out, err := execRoot(t, home, "install", "--"+persistEngramCloudTokenFlag)
	if err != nil {
		t.Fatalf("install command error = %v, want nil (enrollment runner failure must be non-fatal), output:\n%s", err, out)
	}
	if !strings.Contains(out, "No se pudo sincronizar Engram Cloud") {
		t.Fatalf("install output = %q, want it to contain the Spanish enrollment-failure warning", out)
	}
	if !strings.Contains(out, "Instalación completa.") {
		t.Fatalf("install output = %q, want the command to continue to completion after the enrollment failure", out)
	}
}

// TestInstallCommand_DoesNotRegisterCodexMCP proves the pinned upstream install owns Codex MCP.
func TestInstallCommand_DoesNotRegisterCodexMCP(t *testing.T) {
	claudeHome := t.TempDir()
	stateHome := t.TempDir()
	runner := newTestCommandRunner(claudeHome)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()
	lookup := cliFakeBinaryLookup{resolved: map[string]string{"codex": "/usr/bin/codex"}}

	guidanceCalls := 0
	restoreGuidance := SetSyncCodexGuidanceFuncForTests(func(cfg installer.Config) error {
		guidanceCalls++
		return nil
	})
	defer restoreGuidance()

	mcpCalls := 0
	restoreMCP := SetSyncCodexMCPFuncForTests(func(cfg installer.Config) error {
		mcpCalls++
		return errTestCodexMCP
	})
	defer restoreMCP()

	out, err := execRootWithHomesAndLookup(t, claudeHome, stateHome, t.TempDir(), t.TempDir(), lookup, "install")
	if err != nil {
		t.Fatalf("install command error = %v, want nil (Codex MCP failure must be non-fatal), output:\n%s", err, out)
	}
	if !strings.Contains(out, "Instalación completa.") {
		t.Fatalf("install output = %q, want the command to complete", out)
	}
	if guidanceCalls != 1 {
		t.Fatalf("SyncCodexGuidance called %d times, want 1", guidanceCalls)
	}
	if mcpCalls != 0 {
		t.Fatalf("SyncCodexMCP called %d times, want no Click-owned Codex registration", mcpCalls)
	}
}

var errTestCodexMCP = &codexMCPError{msg: "codex mcp add failed"}

type codexMCPError struct{ msg string }

func (e *codexMCPError) Error() string { return e.msg }

func TestInstall_SharedReaderConsentBeforeTokenWrite(t *testing.T) {
	home := t.TempDir()
	configureCLIExecutionHomes(t, home, "", "", []string{"install"})
	t.Setenv("CLICK_STATE_HOME", t.TempDir())
	t.Setenv("CLICK_ENGRAM_CLOUD_SERVER", "http://127.0.0.1:18080")
	t.Setenv("CLICK_ENGRAM_CLOUD_PROJECT", "click-ai-devkit")

	t.Setenv("ENGRAM_CLOUD_TOKEN", "test-token-for-consent-ordering")
	seedResolvableEngram(t)
	restoreLookup := installer.SetBinaryLookupFactoryForTests(func() installer.BinaryLookup {
		return cliFakeBinaryLookup{resolved: map[string]string{"claude": "/usr/bin/claude", "git": "/usr/bin/git"}}
	})
	defer restoreLookup()
	runner := newTestCommandRunner(home)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()
	forceTerminalDetection(t, true, true)
	previousTargetSelector := runInstallTargetSelectTUI
	runInstallTargetSelectTUI = func(_ *cobra.Command, _ bool, _ bool, _ bool, selection installer.TargetSelection) (installer.TargetSelection, bool, error) {
		return selection, false, nil
	}
	t.Cleanup(func() { runInstallTargetSelectTUI = previousTargetSelector })
	previousModelSelector := runInstallSelectTUI
	runInstallSelectTUI = func(_ *cobra.Command, _ modelconfig.ProfileName) (modelconfig.ProfileName, map[modelconfig.Phase]string, bool, error) {
		resolved := modelconfig.ResolveProfile("")
		return resolved.Name, resolved.Models, false, nil
	}
	t.Cleanup(func() { runInstallSelectTUI = previousModelSelector })

	consentCalled := false
	restoreConsent := SetReadCloudTokenConsentFuncForTests(func(reader *bufio.Reader) (bool, error) {
		consentCalled = true

		settingsPath := filepath.Join(home, "settings.json")
		settings, err := os.ReadFile(settingsPath)
		if err != nil && !os.IsNotExist(err) {
			return false, err
		}

		if bytes.Contains(settings, []byte("ENGRAM_CLOUD_TOKEN")) {
			t.Fatal("settings.json contained the token before affirmative consent was read")
		}

		line, err := reader.ReadString('\n')
		if err != nil {
			return false, err
		}
		return strings.EqualFold(strings.TrimSpace(line), "y"), nil
	})
	defer restoreConsent()

	root := NewRootCommand()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetIn(strings.NewReader("y\ny\n"))
	root.SetArgs([]string{"install"})
	if err := root.Execute(); err != nil {
		// SECURITY (DD-7): Do NOT interpolate out.String() in the failure message.
		// If a regression causes this test to fail and out contains the token,
		// printing it would leak the secret into CI logs.
		t.Fatalf("install command failed unexpectedly; see test setup for expected consent flow")
	}
	if err := syncStandardTargetFixtures(); err != nil {
		t.Fatal(err)
	}
	if !consentCalled {
		t.Fatal("consent reader was not called by runInstall")
	}

	settingsPath := filepath.Join(os.Getenv("CLICK_CLAUDE_HOME"), "settings.json")
	settings, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("ReadFile(settings.json) error = %v", err)
	}

	var settingsJSON map[string]any
	if err := json.Unmarshal(settings, &settingsJSON); err != nil {
		t.Fatalf("parse settings.json: %v", err)
	}

	env, ok := settingsJSON["env"].(map[string]any)
	if !ok {
		t.Fatal("settings[\"env\"] not present or not a map")
	}

	if _, hasToken := env["ENGRAM_CLOUD_TOKEN"]; !hasToken {
		t.Fatal("settings.json did not contain ENGRAM_CLOUD_TOKEN after affirmative consent")
	}
}

func TestInstall_ManifestOnlyConfigPersistsTokenWithFlag(t *testing.T) {
	home := t.TempDir()
	configureCLIExecutionHomes(t, home, "", "", []string{"install"})
	t.Setenv("CLICK_STATE_HOME", t.TempDir())
	t.Setenv("CLICK_ENGRAM_CLOUD_SERVER", "")
	t.Setenv("CLICK_ENGRAM_CLOUD_PROJECT", "")
	t.Setenv("ENGRAM_CLOUD_TOKEN", "test-token")
	seedResolvableGit(t)
	runner := newTestCommandRunner(home)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()
	m, err := manifest.Load()
	if err != nil {
		t.Fatalf("manifest.Load() error = %v", err)
	}
	m.EngramCloud.Server, m.EngramCloud.Project = "https://cloud.example.com", "team-hive"
	t.Cleanup(manifest.SetManifestForTests(*m))

	root := NewRootCommand()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"install", "--persist-engram-cloud-token"})
	if err := root.Execute(); err != nil {
		t.Fatalf("install error = %v", err)
	}
	if err := syncStandardTargetFixtures(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(home, "settings.json"))
	if err != nil {
		t.Fatalf("ReadFile(settings.json) error = %v", err)
	}
	if !bytes.Contains(data, []byte("ENGRAM_CLOUD_TOKEN")) {
		t.Fatal("settings.json did not contain persisted token")
	}
}

// TestInstall_DeclinedPersistenceStillEnrolls is the slice-5 correction's RED test: D40 enrollment
// and token persistence are independent concerns. When server, project and ENGRAM_CLOUD_TOKEN are all
// present in the environment and token persistence is DECLINED (non-interactive, no
// --persist-engram-cloud-token), SyncEngramCloud must still run exactly once — the token is right
// there in the process environment for the engram subprocess to inherit — while
// ConfigureEngramCloudSessionSync receives CloudTokenPersistenceDecline so the token is never written
// to settings.json. The process exits 0.
func TestInstall_DeclinedPersistenceStillEnrolls(t *testing.T) {
	home := t.TempDir()
	configureCLIExecutionHomes(t, home, "", "", []string{"install"})
	t.Setenv("CLICK_STATE_HOME", t.TempDir())
	seedResolvableGit(t)

	runner := newTestCommandRunner(home)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()

	configureModes := []installer.CloudTokenPersistence{}
	restoreConfigure := installer.SetConfigureEngramCloudSessionSyncFuncForTests(func(cfg installer.Config, m *manifest.Manifest, mode installer.CloudTokenPersistence, token string) error {
		configureModes = append(configureModes, mode)
		return nil
	})
	defer restoreConfigure()

	cloudCalls := 0
	restoreCloud := SetSyncEngramCloudFuncForTests(func(cfg installer.Config, m *manifest.Manifest) error {
		cloudCalls++
		return nil
	})
	defer restoreCloud()

	t.Setenv("ENGRAM_CLOUD_TOKEN", "test-token")
	t.Setenv("CLICK_ENGRAM_CLOUD_SERVER", "http://127.0.0.1:18080")
	t.Setenv("CLICK_ENGRAM_CLOUD_PROJECT", "click-ai-devkit")

	root := NewRootCommand()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetIn(&bytes.Buffer{})
	root.SetArgs([]string{"install"})

	if err := root.Execute(); err != nil {
		t.Fatalf("install command error = %v, want nil (declined persistence still exits 0)", err)
	}
	if err := syncStandardTargetFixtures(); err != nil {
		t.Fatal(err)
	}

	if cloudCalls != 1 {
		t.Fatalf("SyncEngramCloud called %d times, want 1 (D40 enrollment must run with the token present in the environment, even when persistence is declined)", cloudCalls)
	}
	if len(configureModes) != 1 {
		t.Fatalf("ConfigureEngramCloudSessionSync called %d times, want 1", len(configureModes))
	}
	if configureModes[0] != installer.CloudTokenPersistenceDecline {
		t.Fatalf("ConfigureEngramCloudSessionSync mode = %v, want CloudTokenPersistenceDecline", configureModes[0])
	}
	if !strings.Contains(out.String(), "Instalación completa.") {
		t.Fatalf("install output missing the completion message")
	}
}

// TestInstall_EnvOverridesWriteEnvBlockAndHook is the F1 regression guard: when CLICK_ENGRAM_CLOUD_SERVER,
// CLICK_ENGRAM_CLOUD_PROJECT and ENGRAM_CLOUD_TOKEN are set via environment overrides with an empty manifest,
// and consent is given, the real settings file on disk must contain the env block with all three keys and the
// managed SessionStart hook with the resolved project. This test does NOT mock ConfigureEngramCloudSessionSync.
func TestInstall_EnvOverridesWriteEnvBlockAndHook(t *testing.T) {
	home := t.TempDir()
	runner := newTestCommandRunner(home)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()
	seedResolvableEngram(t)
	seedResolvableGit(t)

	// Set env overrides - the manifest is deliberately empty
	serverOverride := "http://127.0.0.1:18080"
	projectOverride := "click-ai-devkit"
	tokenOverride := "consented-token-123"
	t.Setenv("CLICK_ENGRAM_CLOUD_SERVER", serverOverride)
	t.Setenv("CLICK_ENGRAM_CLOUD_PROJECT", projectOverride)
	t.Setenv("ENGRAM_CLOUD_TOKEN", tokenOverride)
	configureCLIExecutionHomes(t, home, "", "", []string{"install"})
	t.Setenv("CLICK_STATE_HOME", t.TempDir())

	// Run install with --persist-engram-cloud-token to give consent
	root := NewRootCommand()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetIn(&bytes.Buffer{})
	root.SetArgs([]string{"install", "--persist-engram-cloud-token"})

	if err := root.Execute(); err != nil {
		t.Fatalf("install command error = %v, output:\n%s", err, out.String())
	}
	if err := syncStandardTargetFixtures(); err != nil {
		t.Fatal(err)
	}

	// Read the actual settings.json file on disk
	settingsPath := filepath.Join(home, "settings.json")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings.json: %v", err)
	}

	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("parse settings.json: %v", err)
	}

	// Assert the env block exists with all three keys
	env, ok := settings["env"].(map[string]any)
	if !ok {
		t.Fatalf("settings[\"env\"] not present or not a map")
	}

	// Check ENGRAM_CLOUD_AUTOSYNC
	if got, want := env["ENGRAM_CLOUD_AUTOSYNC"], "1"; got != want {
		t.Fatalf("env[\"ENGRAM_CLOUD_AUTOSYNC\"] = %v, want %q", got, want)
	}

	// Check ENGRAM_CLOUD_SERVER (must use override, not empty manifest)
	if got := env["ENGRAM_CLOUD_SERVER"]; got != serverOverride {
		t.Fatalf("env[\"ENGRAM_CLOUD_SERVER\"] = %v, want %q (from override, not manifest)", got, serverOverride)
	}

	// Check ENGRAM_CLOUD_TOKEN (persisted via consent)
	if got := env["ENGRAM_CLOUD_TOKEN"]; got != tokenOverride {
		t.Fatal("persisted cloud token does not match the supplied token")
	}

	// Assert the managed SessionStart hook exists
	hooks, ok := settings["hooks"].(map[string]any)
	if !ok {
		t.Fatalf("settings[\"hooks\"] not present or not a map")
	}

	sessionStart, ok := hooks["SessionStart"].([]any)
	if !ok {
		t.Fatalf("hooks[\"SessionStart\"] not present or not an array")
	}

	// Find the managed hook entry (matcher "" with our command)
	foundManagedHook := false
	for _, rawEntry := range sessionStart {
		entry, ok := rawEntry.(map[string]any)
		if !ok {
			continue
		}
		matcher, _ := entry["matcher"].(string)
		if matcher != "" {
			continue
		}
		entryHooks, _ := entry["hooks"].([]any)
		for _, rawHook := range entryHooks {
			hook, ok := rawHook.(map[string]any)
			if !ok {
				continue
			}
			hookType, _ := hook["type"].(string)
			if hookType != "command" {
				continue
			}
			command, _ := hook["command"].(string)
			// Use the installer's hook validation function
			if installer.IsManagedEngramCloudHookCommand(command) {
				foundManagedHook = true
				break
			}
		}
		if foundManagedHook {
			break
		}
	}

	if !foundManagedHook {
		t.Fatalf("managed SessionStart hook not found in settings.json")
	}
}

// TestInstall_NonInteractiveDoesNotPrintConsentPrompt validates that when --yes is passed,
// the consent prompt is NOT printed to output, even when cloud server/project/token are all present.
// This is F7: emitConsentPrompt was called before the noninteractive check, causing the prompt
// to appear in non-interactive runs (CI, scripts, --yes, --non-interactive, non-TTY).
func TestInstall_NonInteractiveDoesNotPrintConsentPrompt(t *testing.T) {
	home := t.TempDir()
	runner := newTestCommandRunner(home)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()
	seedResolvableEngram(t)

	restoreCloud := SetSyncEngramCloudFuncForTests(func(cfg installer.Config, m *manifest.Manifest) error {
		return nil
	})
	defer restoreCloud()

	t.Setenv("ENGRAM_CLOUD_TOKEN", "test-token")
	t.Setenv("CLICK_ENGRAM_CLOUD_SERVER", "http://127.0.0.1:18080")
	t.Setenv("CLICK_ENGRAM_CLOUD_PROJECT", "click-ai-devkit")

	out, err := execRoot(t, home, "install", "--yes")
	if err != nil {
		t.Fatalf("install --yes error = %v, output:\n%s", err, out)
	}

	if strings.Contains(out, consentPrompt) {
		t.Fatalf("install --yes output contains consent prompt when it should not: %q", out)
	}
}

// TestInstall_PersistFlagDoesNotPrintConsentPrompt validates that when --persist-engram-cloud-token
// is passed, the consent prompt is NOT printed, but the token IS persisted to settings.json.
// This is F7: the persist flag should bypass prompting entirely, not print the prompt and ignore it.
func TestInstall_PersistFlagDoesNotPrintConsentPrompt(t *testing.T) {
	home := t.TempDir()
	runner := newTestCommandRunner(home)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()
	seedResolvableEngram(t)

	restoreCloud := SetSyncEngramCloudFuncForTests(func(cfg installer.Config, m *manifest.Manifest) error {
		return nil
	})
	defer restoreCloud()

	t.Setenv("ENGRAM_CLOUD_TOKEN", "test-token")
	t.Setenv("CLICK_ENGRAM_CLOUD_SERVER", "http://127.0.0.1:18080")
	t.Setenv("CLICK_ENGRAM_CLOUD_PROJECT", "click-ai-devkit")

	out, err := execRoot(t, home, "install", "--"+persistEngramCloudTokenFlag)
	if err != nil {
		t.Fatalf("install --persist-engram-cloud-token error = %v, output:\n%s", err, out)
	}

	if strings.Contains(out, consentPrompt) {
		t.Fatalf("install --persist-engram-cloud-token output contains consent prompt when it should not: %q", out)
	}

	settingsPath := filepath.Join(home, "settings.json")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings.json: %v", err)
	}

	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("parse settings.json: %v", err)
	}

	env, ok := settings["env"].(map[string]any)
	if !ok {
		t.Fatalf("settings[\"env\"] not present or not a map")
	}

	if got := env["ENGRAM_CLOUD_TOKEN"]; got != "test-token" {
		t.Fatal("cloud token should be persisted with persist flag")
	}
}

// TestMain supplies a fail-closed, isolated upstream runner for every legacy CLI
// test too. Dedicated gentle-ai tests replace it with their own bounded fixtures.
func TestMain(m *testing.M) {
	home, err := os.MkdirTemp("", "click-cli-gentle-ai-test-*")
	if err != nil {
		panic(err)
	}
	pin, err := manifest.Load()
	if err != nil {
		panic(err)
	}
	for key, value := range map[string]string{
		"HOME": home, "USERPROFILE": home, "XDG_CONFIG_HOME": filepath.Join(home, ".config"),
		"CLICK_CLAUDE_HOME": "", "CODEX_HOME": "", "CLICK_OPENCLAW_HOME": "", "CLICK_STATE_HOME": "",
	} {
		if err := os.Setenv(key, value); err != nil {
			panic(err)
		}
	}
	p := installer.NewGentleAIProvisioner()
	p.EnsureOnPath = func(string) (bool, error) { return false, nil }
	p.HomeDir = func() (string, error) { return home, nil }
	path, err := p.ManagedPath()
	if err != nil {
		panic(err)
	}
	if err = os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		panic(err)
	}
	if err = os.WriteFile(path, []byte("fixture"), 0o755); err != nil {
		panic(err)
	}
	p.Environ = func() []string { return []string{"GENTLE_AI_NO_SELF_UPDATE=0"} }
	p.Run = func(_ context.Context, env []string, name string, args ...string) ([]byte, error) {
		if name != path || len(args) == 0 {
			panic("unexpected upstream command")
		}
		if args[0] == "version" {
			return []byte("gentle-ai " + pin.GentleAI.Version), nil
		}
		if args[0] == "install" {
			if err := writeGentleAIInstallFixture(args); err != nil {
				return nil, err
			}
			return nil, nil
		}
		panic("unexpected upstream operation")
	}
	p.Fetch = func(context.Context, string) (io.ReadCloser, error) { panic("unexpected network access") }
	p.WriteBinary = func(string, []byte) error { panic("unexpected provisioning") }
	restore := installer.SetGentleAIProvisionerFactoryForTests(func() installer.GentleAIProvisioner { return p })
	restoreDoctor := doctor.SetManagedGentleAIProvisionerFactoryForTests(func() installer.GentleAIProvisioner { return p })
	code := m.Run()
	restoreDoctor()
	restore()
	os.RemoveAll(home)
	os.Exit(code)
}

func writeGentleAIInstallFixture(args []string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	agents, components := "", ""
	for i := 0; i < len(args)-1; i++ {
		switch args[i] {
		case "--agents":
			agents = args[i+1]
		case "--components":
			components = args[i+1]
		}
	}
	engram := map[string]any{"command": "engram", "args": []string{"mcp", "--tools=agent"}}
	context7 := map[string]any{"command": "npx", "args": []string{"-y", "--package=@upstash/context7-mcp@2.2.5", "--", "context7-mcp"}}
	contains := func(list, value string) bool {
		for _, item := range strings.Split(list, ",") {
			if item == value {
				return true
			}
		}
		return false
	}
	entries := map[string]any{}
	if contains(components, "engram") {
		entries["engram"] = engram
	}
	if contains(components, "context7") {
		entries["context7"] = context7
	}
	if len(entries) == 0 {
		return nil
	}
	if contains(agents, "claude-code") {
		if err := updateJSONMCPFixture(filepath.Join(home, ".claude.json"), "mcpServers", entries); err != nil {
			return err
		}
	}
	if contains(agents, "codex") {
		codexEntries := map[string]any{}
		if _, ok := entries["engram"]; ok {
			codexEntries["engram"] = engram
		}
		if _, ok := entries["context7"]; ok {
			codexEntries["context7"] = map[string]any{"url": "https://mcp.context7.com/mcp"}
		}
		if err := updateCodexMCPFixture(filepath.Join(home, ".codex", "config.toml"), codexEntries); err != nil {
			return err
		}
	}
	if contains(agents, "openclaw") {
		if err := updateOpenClawMCPFixture(filepath.Join(home, ".openclaw", "openclaw.json"), entries); err != nil {
			return err
		}
	}
	return nil
}

func updateJSONMCPFixture(path, key string, entries map[string]any) error {
	root := map[string]any{}
	if data, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(data, &root); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	servers, _ := root[key].(map[string]any)
	if servers == nil {
		servers = map[string]any{}
	}
	for name, entry := range entries {
		servers[name] = entry
	}
	root[key] = servers
	data, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}

func updateOpenClawMCPFixture(path string, entries map[string]any) error {
	root := map[string]any{}
	if data, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(data, &root); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	mcp, _ := root["mcp"].(map[string]any)
	if mcp == nil {
		mcp = map[string]any{}
	}
	servers, _ := mcp["servers"].(map[string]any)
	if servers == nil {
		servers = map[string]any{}
	}
	if legacy, ok := root["mcpServers"].(map[string]any); ok {
		for name, entry := range legacy {
			if _, exists := servers[name]; !exists {
				servers[name] = entry
			}
		}
		delete(root, "mcpServers")
	}
	for name, entry := range entries {
		servers[name] = entry
	}
	mcp["servers"] = servers
	root["mcp"] = mcp
	data, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}

func updateCodexMCPFixture(path string, entries map[string]any) error {
	root := map[string]any{}
	if data, err := os.ReadFile(path); err == nil {
		if err := toml.Unmarshal(data, &root); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	servers, _ := root["mcp_servers"].(map[string]any)
	if servers == nil {
		servers = map[string]any{}
	}
	for name, entry := range entries {
		servers[name] = entry
	}
	root["mcp_servers"] = servers
	data, err := toml.Marshal(root)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func TestGentleAIStep_UnsupportedRootStopsBeforeProvisioning(t *testing.T) {
	codexHome := t.TempDir()
	provisionCalls := 0
	p := installer.NewGentleAIProvisioner()
	managedHome := t.TempDir()
	p.HomeDir = func() (string, error) { return managedHome, nil }
	p.Lstat = func(string) (os.FileInfo, error) { return nil, os.ErrNotExist }
	p.Run = func(context.Context, []string, string, ...string) ([]byte, error) {
		provisionCalls++
		return nil, nil
	}
	t.Cleanup(installer.SetGentleAIProvisionerFactoryForTests(func() installer.GentleAIProvisioner { return p }))
	cmd := newInstallCommand()
	cmd.SetContext(context.Background())
	var out bytes.Buffer
	r := rendererFor(cmd, &out)
	m, err := manifest.Load()
	if err != nil {
		t.Fatal(err)
	}
	err = runGentleAIStep(cmd, &out, r, m, installer.Config{CodexHome: codexHome}, installer.TargetSelection{Codex: true})
	if err == nil || !strings.Contains(err.Error(), "directorio personalizado") {
		t.Fatalf("runGentleAIStep() error = %v, want unsupported-root safety failure", err)
	}
	if provisionCalls != 0 {
		t.Fatalf("provision called %d times, want root preflight to stop before upstream execution", provisionCalls)
	}
}

func TestInstallAndUpdate_UnsupportedRootStopBeforeWritesOrCommands(t *testing.T) {
	for _, command := range []string{"install", "update"} {
		t.Run(command, func(t *testing.T) {
			processHome := t.TempDir()
			stateHome := t.TempDir()
			claudeHome := t.TempDir()
			t.Setenv("HOME", processHome)
			t.Setenv("USERPROFILE", processHome)
			t.Setenv("CLICK_CLAUDE_HOME", claudeHome)
			t.Setenv("CLICK_STATE_HOME", stateHome)
			t.Setenv("CLICK_OPENCLAW_HOME", "")
			t.Setenv("CODEX_HOME", "")

			runner := newTestCommandRunner(claudeHome)
			baseRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
			defer baseRunner()
			restoreBinaryLookup := installer.SetBinaryLookupFactoryForTests(func() installer.BinaryLookup {
				return cliFakeBinaryLookup{resolved: map[string]string{"git": "/fixture/git", "claude": "/fixture/claude"}}
			})
			defer restoreBinaryLookup()
			if command == "update" {
				if err := installer.SaveTargetSelection(installer.Config{ClickStateHome: stateHome}, installer.TargetSelection{Configured: true, Claude: true}); err != nil {
					t.Fatalf("save update target selection: %v", err)
				}
			}
			selectionBefore := installer.TargetSelection{}
			if command == "update" {
				var err error
				var found bool
				selectionBefore, found, err = installer.LoadTargetSelection(installer.Config{ClickStateHome: stateHome})
				if err != nil || !found {
					t.Fatalf("load update target selection: %v", err)
				}
			}
			if err := os.WriteFile(filepath.Join(claudeHome, "untouched.json"), []byte(`{"preserve":true}`), 0o600); err != nil {
				t.Fatalf("seed selected target home: %v", err)
			}

			provisionerCalls := 0
			provisioner := installer.NewGentleAIProvisioner()
			provisioner.HomeDir = func() (string, error) { return t.TempDir(), nil }
			provisioner.EnsureOnPath = func(string) (bool, error) { return false, nil }
			provisioner.Run = func(context.Context, []string, string, ...string) ([]byte, error) {
				provisionerCalls++
				return nil, nil
			}
			restoreProvisioner := installer.SetGentleAIProvisionerFactoryForTests(func() installer.GentleAIProvisioner { return provisioner })
			defer restoreProvisioner()

			root := NewRootCommand()
			var out bytes.Buffer
			root.SetOut(&out)
			root.SetErr(&out)
			root.SetIn(&bytes.Buffer{})
			root.SetArgs([]string{command, "--yes"})
			err := root.Execute()
			if err == nil || !strings.Contains(err.Error(), "directorio personalizado") {
				t.Fatalf("%s error = %v, want unsupported-root preflight failure; output:\n%s", command, err, out.String())
			}
			if len(runner.commands) != 0 {
				t.Fatalf("command runner calls = %v, want zero before unsupported-root rejection", runner.commands)
			}
			if provisionerCalls != 0 {
				t.Fatalf("gentle-ai calls = %d, want zero before unsupported-root rejection", provisionerCalls)
			}
			entries, readErr := os.ReadDir(stateHome)
			if readErr != nil {
				t.Fatalf("read state home: %v", readErr)
			}
			if command == "install" && len(entries) != 0 {
				t.Fatalf("state home entries = %v, want no snapshot or bookkeeping writes before rejection", entries)
			}
			if command == "update" {
				selectionAfter, found, err := installer.LoadTargetSelection(installer.Config{ClickStateHome: stateHome})
				if err != nil || !found || selectionAfter != selectionBefore {
					t.Fatalf("target selection after rejection = %#v, %v; want unchanged %#v", selectionAfter, err, selectionBefore)
				}
				for _, entry := range entries {
					if entry.Name() == "backups" {
						t.Fatalf("state home entries = %v, want no snapshot before rejection", entries)
					}
				}
			}
			entries, readErr = os.ReadDir(claudeHome)
			if readErr != nil {
				t.Fatalf("read selected Claude home: %v", readErr)
			}
			if len(entries) != 1 || entries[0].Name() != "untouched.json" {
				t.Fatalf("selected Claude home entries = %v, want only the preserved sentinel", entries)
			}
			contents, readErr := os.ReadFile(filepath.Join(claudeHome, "untouched.json"))
			if readErr != nil || string(contents) != `{"preserve":true}` {
				t.Fatalf("selected Claude sentinel = %q, %v; want unchanged", contents, readErr)
			}
		})
	}
}

func TestGentleAICommands_SelectedTargetsAndFailStop(t *testing.T) {
	for _, command := range []string{"install", "update"} {
		for mask := 1; mask < 8; mask++ {
			t.Run(fmt.Sprintf("%s/%d", command, mask), func(t *testing.T) {
				selection := installer.TargetSelection{Claude: mask&1 != 0, Codex: mask&2 != 0, OpenClaw: mask&4 != 0}
				resolved := map[string]string{"git": "/fixture/git"}
				if selection.Claude {
					resolved["claude"] = "/fixture/claude"
				}
				if selection.Codex {
					resolved["codex"] = "/fixture/codex"
				}
				if selection.OpenClaw {
					resolved["openclaw"] = "/fixture/openclaw"
				}
				home := t.TempDir()
				runner := newTestCommandRunner(home)
				t.Cleanup(installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner }))
				p := installer.NewGentleAIProvisioner()
				p.EnsureOnPath = func(string) (bool, error) { return false, nil }
				// Use a fixed managed fixture root, never a runtime or real user home.
				managedHome := t.TempDir()
				p.HomeDir = func() (string, error) { return managedHome, nil }
				path, err := p.ManagedPath()
				if err != nil {
					t.Fatal(err)
				}
				if err = os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err = os.WriteFile(path, []byte("fixture"), 0o755); err != nil {
					t.Fatal(err)
				}
				calls := 0
				p.Environ = func() []string { return nil }
				p.Run = func(_ context.Context, env []string, name string, args ...string) ([]byte, error) {
					if name != path {
						t.Fatalf("binary = %q", name)
					}
					if args[0] == "version" {
						return []byte("gentle-ai v4.0.0"), nil
					}
					calls++
					if !reflect.DeepEqual(args, installer.GentleAIInstallArgs(selection)) {
						t.Fatalf("argv=%q", args)
					}
					if !strings.Contains(strings.Join(env, "\n"), "GENTLE_AI_NO_SELF_UPDATE=1") {
						t.Fatal("missing suppression")
					}
					return nil, errors.New("upstream failure")
				}
				t.Cleanup(installer.SetGentleAIProvisionerFactoryForTests(func() installer.GentleAIProvisioner { return p }))
				state := t.TempDir()
				if command == "update" {
					selection.Configured = true
					if err := installer.SaveTargetSelection(installer.Config{ClickStateHome: state}, selection); err != nil {
						t.Fatal(err)
					}
				}
				out, err := execRootWithHomesAndLookup(t, home, state, t.TempDir(), t.TempDir(), cliFakeBinaryLookup{resolved: resolved}, command, "--yes")
				if err == nil || calls != 1 || !strings.Contains(out, "No se pudo configurar gentle-ai") {
					t.Fatalf("error=%v calls=%d output=%s", err, calls, out)
				}
				if len(runner.commands) != 0 {
					t.Fatalf("continued after upstream failure: %q", runner.commands)
				}
			})
		}
	}
}

func TestGentleAIStep_DriftUsesWarningAndMissingUsesInfo(t *testing.T) {
	for _, tt := range []struct{ version, marker string }{{"v4.1.0", "[warn] gentle-ai difiere"}, {"", "[i] gentle-ai no está instalado"}} {
		t.Run(tt.version, func(t *testing.T) {
			processHome := t.TempDir()
			t.Setenv("HOME", processHome)
			t.Setenv("USERPROFILE", processHome)
			p := installer.NewGentleAIProvisioner()
			p.EnsureOnPath = func(string) (bool, error) { return false, nil }
			home := t.TempDir()
			p.HomeDir = func() (string, error) { return home, nil }
			p.Environ = func() []string { return nil }
			if tt.version != "" {
				path, err := p.ManagedPath()
				if err != nil {
					t.Fatal(err)
				}
				if err = os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err = os.WriteFile(path, []byte("fixture"), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			p.Run = func(context.Context, []string, string, ...string) ([]byte, error) {
				return []byte("gentle-ai " + tt.version), nil
			}
			p.Fetch = func(context.Context, string) (io.ReadCloser, error) { return nil, errors.New("fixture failure") }
			t.Cleanup(installer.SetGentleAIProvisionerFactoryForTests(func() installer.GentleAIProvisioner { return p }))
			cmd := newInstallCommand()
			cmd.SetContext(context.Background())
			var out bytes.Buffer
			r := rendererFor(cmd, &out)
			m, err := manifest.Load()
			if err != nil {
				t.Fatal(err)
			}
			if err := runGentleAIStep(cmd, &out, r, m, installer.Config{CodexHome: filepath.Join(processHome, ".codex")}, installer.TargetSelection{Codex: true}); err == nil {
				t.Fatal("provisioning failure returned success")
			}
			if !strings.Contains(out.String(), tt.marker) {
				t.Fatalf("missing %q: %s", tt.marker, out.String())
			}
		})
	}
}

func TestInstall_GentleAIFailurePreservesPreexistingState(t *testing.T) {
	testGentleAIFailurePreservesPreexistingState(t, "install")
}

func testGentleAIFailurePreservesPreexistingState(t *testing.T, command string) {
	t.Helper()
	for _, failure := range []string{"provision", "install"} {
		t.Run(failure, func(t *testing.T) {
			home, state := t.TempDir(), t.TempDir()
			cfg := installer.Config{ClaudeHome: home, ClickStateHome: state}
			models := []byte("{\n  \"schema_version\": 1, \"models\": {\"default\": \"user-choice\"}\n}\n")
			// Distinct formatting and a previously selected, now-unavailable target detect
			// even a semantically similar rewrite of the persistent selection.
			targets := []byte("{\"schemaVersion\":1, \"configured\":true, \"claude\":true, \"codex\":true, \"openclaw\":false}\n")
			if err := os.MkdirAll(filepath.Dir(cfg.ModelsPath()), 0o755); err != nil {
				t.Fatal(err)
			}
			for path, data := range map[string][]byte{cfg.ModelsPath(): models, cfg.TargetSelectionPath(): targets} {
				if err := os.WriteFile(path, data, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			runner := newTestCommandRunner(home)
			t.Cleanup(installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner }))
			bookkeeping := 0
			t.Cleanup(SetRecordSnapshotPostRunFuncForTests(func(installer.Config) error { bookkeeping++; return nil }))
			checkSnapshot := func() {
				t.Helper()
				if found, err := installer.HasRunSnapshot(cfg); err != nil || !found {
					t.Fatalf("gentle-ai ran before snapshot: found=%t error=%v", found, err)
				}
			}
			p := installer.NewGentleAIProvisioner()
			p.EnsureOnPath = func(string) (bool, error) { return false, nil }
			managedHome := t.TempDir()
			p.HomeDir = func() (string, error) { return managedHome, nil }
			p.Environ = func() []string { return nil }
			provisions, installs := 0, 0
			p.Fetch = func(context.Context, string) (io.ReadCloser, error) {
				checkSnapshot()
				provisions++
				return nil, errors.New("offline fixture")
			}
			p.WriteBinary = func(string, []byte) error { t.Fatal("unexpected binary write"); return nil }
			if failure == "install" {
				path, err := p.ManagedPath()
				if err != nil {
					t.Fatal(err)
				}
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte("fixture"), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			p.Run = func(_ context.Context, _ []string, name string, args ...string) ([]byte, error) {
				checkSnapshot()
				if failure == "provision" {
					// Windows provisions via Go rather than fetching a release archive.
					if p.GOOS != "windows" || name != "go" || args[0] != "version" {
						t.Fatal("unexpected subprocess during provisioning")
					}
					provisions++
					return nil, errors.New("Go toolchain fixture failure")
				}
				if args[0] == "version" {
					return []byte("gentle-ai v4.0.0"), nil
				}
				installs++
				return nil, errors.New("install fixture failure")
			}
			t.Cleanup(installer.SetGentleAIProvisionerFactoryForTests(func() installer.GentleAIProvisioner { return p }))
			out, err := execRootWithHomesAndLookup(t, home, state, t.TempDir(), t.TempDir(), cliFakeBinaryLookup{resolved: map[string]string{"claude": "/fixture/claude", "git": "/fixture/git"}}, command, "--yes")
			wantError := "No se pudo configurar gentle-ai"
			if failure == "provision" {
				wantError = "No se pudo instalar la versión fijada"
			}
			if err == nil || !strings.Contains(out, wantError) {
				t.Fatalf("error=%v output=%s, want %q", err, out, wantError)
			}
			if (failure == "provision" && (provisions != 1 || installs != 0)) || (failure == "install" && (provisions != 0 || installs != 1)) {
				t.Errorf("provisions=%d installs=%d, want exactly one failing gentle-ai step", provisions, installs)
			}
			for path, want := range map[string][]byte{cfg.ModelsPath(): models, cfg.TargetSelectionPath(): targets} {
				if got, err := os.ReadFile(path); err != nil || !bytes.Equal(got, want) {
					t.Errorf("gentle-ai failure changed %s: got=%q want=%q error=%v", filepath.Base(path), got, want, err)
				}
			}
			if _, err := os.Stat(cfg.ModelsPath() + ".bak"); !os.IsNotExist(err) {
				t.Errorf("model migration ran before gentle-ai: backup error=%v", err)
			}
			if bookkeeping != 1 || len(runner.commands) != 0 {
				t.Errorf("bookkeeping=%d local commands=%q", bookkeeping, runner.commands)
			}
		})
	}
}
