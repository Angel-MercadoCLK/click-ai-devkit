package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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

// TestInstall_ConfigureFailureWarnsAndContinues is task 5.15's RED test: a failing
// configureEngramCloudSessionSyncFunc must leave install exiting 0 with a Spanish warning (ECS-10.1,
// ECS-10.2), and a recording order assertion shows configure runs before enrollment (DD-4 ordering).
// It covers both the CloudResolvable-without-token dispatch path (the env block and hook must still
// be written so a later token export activates everything — the whole point of the rename) and the
// persisted-token path (configure before enroll).
func TestInstall_ConfigureFailureWarnsAndContinues(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CLICK_CLAUDE_HOME", home)
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

// TestInstallCommand_CodexMCPFailureIsNonFatal is FIX 2's non-fatal contract, mirroring
// TestInstallCommand_CloudConfigured_EnrollmentFailureIsNonFatal for Codex's Engram MCP
// registration (D45 "supplementary integrations are non-fatal" pattern): a failure registering
// Engram's MCP server with Codex must never abort an otherwise-good local install. The command must
// (a) return nil, (b) surface a Spanish warning containing the underlying error, and (c) still
// complete the remaining steps (the Codex AGENTS.md guidance write already ran before this step,
// and the install still reaches its completion line).
func TestInstallCommand_CodexMCPFailureIsNonFatal(t *testing.T) {
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

	restoreMCP := SetSyncCodexMCPFuncForTests(func(cfg installer.Config) error {
		return errTestCodexMCP
	})
	defer restoreMCP()

	out, err := execRootWithHomesAndLookup(t, claudeHome, stateHome, t.TempDir(), t.TempDir(), lookup, "install")
	if err != nil {
		t.Fatalf("install command error = %v, want nil (Codex MCP failure must be non-fatal), output:\n%s", err, out)
	}
	if !strings.Contains(out, "No se pudo registrar Engram en Codex") {
		t.Fatalf("install output = %q, want it to contain the Spanish Codex MCP failure warning", out)
	}
	if !strings.Contains(out, errTestCodexMCP.Error()) {
		t.Fatalf("install output = %q, want the warning to include the underlying error %q", out, errTestCodexMCP.Error())
	}
	if !strings.Contains(out, "Instalación completa.") {
		t.Fatalf("install output = %q, want the command to continue to completion after the Codex MCP failure", out)
	}
	if guidanceCalls != 1 {
		t.Fatalf("SyncCodexGuidance called %d times, want 1 — the AGENTS.md write step must still have run before the failed MCP step", guidanceCalls)
	}
}

var errTestCodexMCP = &codexMCPError{msg: "codex mcp add failed"}

type codexMCPError struct{ msg string }

func (e *codexMCPError) Error() string { return e.msg }

func TestInstall_SharedReaderConsentBeforeTokenWrite(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CLICK_CLAUDE_HOME", home)
	t.Setenv("CLICK_STATE_HOME", t.TempDir())
	seedResolvableGit(t)

	runner := newTestCommandRunner(home)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()

	configureCalls := []string{}
	restoreConfigure := installer.SetConfigureEngramCloudSessionSyncFuncForTests(func(cfg installer.Config, m *manifest.Manifest, mode installer.CloudTokenPersistence, token string) error {
		configureCalls = append(configureCalls, fmt.Sprintf("mode=%d,token=%s", mode, token))
		return nil
	})
	defer restoreConfigure()

	t.Setenv("ENGRAM_CLOUD_TOKEN", "test-token")
	t.Setenv("CLICK_ENGRAM_CLOUD_SERVER", "http://127.0.0.1:18080")
	t.Setenv("CLICK_ENGRAM_CLOUD_PROJECT", "click-ai-devkit")

	root := NewRootCommand()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	stdin := bytes.NewBufferString("y\n")
	root.SetIn(stdin)
	root.SetArgs([]string{"install"})

	err := root.Execute()
	if err != nil {
		t.Fatalf("install command error = %v, output:\n%s", err, out.String())
	}

	if len(configureCalls) != 1 {
		t.Fatalf("ConfigureEngramCloudSessionSync called %d times, want 1", len(configureCalls))
	}
	expectedCall := fmt.Sprintf("mode=%d,token=test-token", installer.CloudTokenPersistenceDecline)
	if configureCalls[0] != expectedCall {
		t.Fatalf("ConfigureEngramCloudSessionSync call = %q, want %q", configureCalls[0], expectedCall)
	}

	output := out.String()
	if !strings.Contains(output, "Instalación completa.") {
		t.Fatalf("install output = %q, want it to contain completion message", output)
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
	t.Setenv("CLICK_CLAUDE_HOME", home)
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

	// Set env overrides - the manifest is deliberately empty
	serverOverride := "http://127.0.0.1:18080"
	projectOverride := "click-ai-devkit"
	tokenOverride := "consented-token-123"
	t.Setenv("CLICK_ENGRAM_CLOUD_SERVER", serverOverride)
	t.Setenv("CLICK_ENGRAM_CLOUD_PROJECT", projectOverride)
	t.Setenv("ENGRAM_CLOUD_TOKEN", tokenOverride)
	t.Setenv("CLICK_CLAUDE_HOME", home)
	t.Setenv("CLICK_STATE_HOME", t.TempDir())
	t.Setenv("CODEX_HOME", t.TempDir())

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
