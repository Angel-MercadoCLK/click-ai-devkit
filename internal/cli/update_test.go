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

// TestUpdateCommand_YesAndNonInteractiveFlags_Parse is the regression for the "unknown flag" bug:
// runUpdate has always routed its confirm gate through isNonInteractiveInstall (install.go), which
// reads --yes/--non-interactive, but `click update` never DECLARED those flags — so the documented
// scripted escape hatch (`click update --yes`) died at flag-parse time with "unknown flag: --yes",
// before RunE ever ran. Both spellings must now parse and run to completion.
func TestUpdateCommand_YesAndNonInteractiveFlags_Parse(t *testing.T) {
	for _, flag := range []string{"--" + yesFlag, "--" + nonInteractiveFlag} {
		t.Run(flag, func(t *testing.T) {
			home := t.TempDir()
			runner := newTestCommandRunner(home)
			restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
			defer restoreRunner()
			seedResolvableEngram(t)

			out, err := execRoot(t, home, "update", flag)
			if err != nil {
				t.Fatalf("update %s error = %v, want nil (the flag must be declared on the command), output:\n%s", flag, err, out)
			}
			if strings.Contains(out, "unknown flag") {
				t.Fatalf("update %s output = %q, want no unknown-flag error", flag, out)
			}
			if !strings.Contains(out, "Update completo.") {
				t.Fatalf("update %s output = %q, want the command to run to completion", flag, out)
			}
		})
	}
}

// TestIsNonInteractiveUpdate_FlagsForceNonInteractiveOnFullTTY proves the flags are wired to the
// SAME gate `click install` uses, with the same false defaults: on a machine where BOTH streams are
// real terminals, a bare `click update` stays interactive (write plan + confirm prompt), and either
// flag flips it to the non-interactive path — no plan, no prompt, no TUI. Without the declaration
// the Set() call below fails outright, which is exactly the bug.
func TestIsNonInteractiveUpdate_FlagsForceNonInteractiveOnFullTTY(t *testing.T) {
	for _, flag := range []string{yesFlag, nonInteractiveFlag} {
		t.Run(flag, func(t *testing.T) {
			forceTerminalDetection(t, true /* stdout is a TTY */, true /* stdin is a TTY */)

			cmd := newUpdateCommand()
			cmd.SetIn(&bytes.Buffer{})

			if isNonInteractiveInstall(cmd, &bytes.Buffer{}) {
				t.Fatalf("isNonInteractiveInstall = true for a bare `click update` on a full TTY, want false (interactive confirm)")
			}
			if err := cmd.Flags().Set(flag, "true"); err != nil {
				t.Fatalf("cmd.Flags().Set(%q) error = %v, want the flag declared on `click update`", flag, err)
			}
			if !isNonInteractiveInstall(cmd, &bytes.Buffer{}) {
				t.Fatalf("isNonInteractiveInstall = false with --%s on a full TTY, want true (skip the confirm prompt)", flag)
			}
		})
	}
}

// TestUpdateCommand_CloudConfigured_RunsCloudStepAfterEngram is task 4.5's RED test: when cloud
// server/project/token are all present and the dedicated --persist-engram-cloud-token opt-in is
// given (DD-3 consent), `click update` must re-sync Engram Cloud right after the local Engram pin
// step, using Spanish user-facing labels.
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
// The --persist-engram-cloud-token opt-in authorizes the re-sync to run unattended (DD-3).
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

// TestUpdateCommand_CodexMCPFailureIsNonFatal is FIX 2's non-fatal contract for `click update`,
// mirroring TestUpdateCommand_CloudConfigured_ReSyncFailureIsNonFatal for Codex's Engram MCP
// registration (D45 "supplementary integrations are non-fatal" pattern): a failure re-registering
// Engram's MCP server with Codex must never abort an otherwise-good update. The command must (a)
// return nil, (b) surface a Spanish warning containing the underlying error, and (c) still reach
// completion.
func TestUpdateCommand_CodexMCPFailureIsNonFatal(t *testing.T) {
	claudeHome := t.TempDir()
	stateHome := t.TempDir()
	runner := newTestCommandRunner(claudeHome)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()
	lookup := cliFakeBinaryLookup{resolved: map[string]string{"codex": "/usr/bin/codex"}}
	if err := installer.SaveTargetSelection(installer.Config{ClaudeHome: claudeHome, ClickStateHome: stateHome}, installer.TargetSelection{Configured: true, Codex: true}); err != nil {
		t.Fatalf("SaveTargetSelection() error = %v", err)
	}

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

	out, err := execRootWithHomesAndLookup(t, claudeHome, stateHome, t.TempDir(), t.TempDir(), lookup, "update")
	if err != nil {
		t.Fatalf("update command error = %v, want nil (Codex MCP failure must be non-fatal), output:\n%s", err, out)
	}
	if !strings.Contains(out, "No se pudo registrar Engram en Codex") {
		t.Fatalf("update output = %q, want it to contain the Spanish Codex MCP failure warning", out)
	}
	if !strings.Contains(out, errTestCodexMCP.Error()) {
		t.Fatalf("update output = %q, want the warning to include the underlying error %q", out, errTestCodexMCP.Error())
	}
	if !strings.Contains(out, "Update completo.") {
		t.Fatalf("update output = %q, want the command to continue to completion after the Codex MCP failure", out)
	}
	if guidanceCalls != 1 {
		t.Fatalf("SyncCodexGuidance called %d times, want 1 — the AGENTS.md write step must still have run before the failed MCP step", guidanceCalls)
	}
}

// TestUpdate_ConfigureFailureWarnsAndContinues is task 5.17's RED test: update mirrors install's
// non-fatal configure contract (ECS-10.1, ECS-10.2, ECS-10.7): a failing configureEngramCloudSession
// SyncFunc leaves update exiting 0 with a Spanish warning, and a recording order assertion shows
// configure runs before enrollment.
func TestUpdate_ConfigureFailureWarnsAndContinues(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CLICK_CLAUDE_HOME", home)
	t.Setenv("CLICK_STATE_HOME", t.TempDir())
	seedResolvableGit(t)

	runner := newTestCommandRunner(home)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()

	t.Setenv("CLICK_ENGRAM_CLOUD_SERVER", "http://127.0.0.1:18080")
	t.Setenv("CLICK_ENGRAM_CLOUD_PROJECT", "click-ai-devkit")
	t.Setenv("ENGRAM_CLOUD_TOKEN", "test-token")

	var order []string
	restoreConfigure := installer.SetConfigureEngramCloudSessionSyncFuncForTests(func(cfg installer.Config, m *manifest.Manifest, mode installer.CloudTokenPersistence, token string) error {
		order = append(order, "configure")
		return errTestCloudSettings
	})
	defer restoreConfigure()
	restoreCloud := SetSyncEngramCloudFuncForTests(func(cfg installer.Config, m *manifest.Manifest) error {
		order = append(order, "enrollment")
		return nil
	})
	defer restoreCloud()

	root := NewRootCommand()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetIn(&bytes.Buffer{})
	root.SetArgs([]string{"update", "--" + persistEngramCloudTokenFlag})

	if err := root.Execute(); err != nil {
		t.Fatalf("update command error = %v, want nil (cloud settings failure must be non-fatal)", err)
	}
	if len(order) != 2 || order[0] != "configure" || order[1] != "enrollment" {
		t.Fatalf("recorded order = %v, want [configure enrollment]", order)
	}
	if !strings.Contains(out.String(), "No se pudo configurar Engram Cloud Session Sync") {
		t.Fatalf("update output missing the Spanish cloud-settings warning")
	}
	if !strings.Contains(out.String(), errTestCloudSettings.Error()) {
		t.Fatalf("update output missing the underlying error %q", errTestCloudSettings.Error())
	}
	if !strings.Contains(out.String(), "Update completo.") {
		t.Fatalf("update output missing the completion message")
	}
}

// TestUpdate_DeclineWarnsAutosyncDisabled is task 5.17's RED test: a declined persistence decision
// (non-interactive without the dedicated opt-in) prints the Spanish autosync-disabled warning and
// still exits 0 (ECS-3.3, ECS-3.4). Per D40, the consent decision governs ONLY token persistence:
// enrollment/re-sync still runs when ENGRAM_CLOUD_TOKEN is present in the environment.
func TestUpdate_DeclineWarnsAutosyncDisabled(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CLICK_CLAUDE_HOME", home)
	t.Setenv("CLICK_STATE_HOME", t.TempDir())
	seedResolvableGit(t)

	runner := newTestCommandRunner(home)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()

	t.Setenv("CLICK_ENGRAM_CLOUD_SERVER", "http://127.0.0.1:18080")
	t.Setenv("CLICK_ENGRAM_CLOUD_PROJECT", "click-ai-devkit")
	t.Setenv("ENGRAM_CLOUD_TOKEN", "test-token")

	root := NewRootCommand()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetIn(&bytes.Buffer{})
	root.SetArgs([]string{"update"})

	if err := root.Execute(); err != nil {
		t.Fatalf("update command error = %v, want nil (decline remains successful)", err)
	}
	if !strings.Contains(out.String(), "autosync desactivado") {
		t.Fatalf("update output missing the Spanish autosync-disabled warning (ECS-3.3)")
	}
	if !strings.Contains(out.String(), "Update completo.") {
		t.Fatalf("update output missing the completion message")
	}
}

// TestUpdate_EnvOverridesWriteEnvBlockAndHook is the F1 regression guard for update: when CLICK_ENGRAM_CLOUD_SERVER,
// CLICK_ENGRAM_CLOUD_PROJECT and ENGRAM_CLOUD_TOKEN are set via environment overrides with an empty manifest,
// and consent is given, the real settings file on disk must contain the env block with all three keys and the
// managed SessionStart hook with the resolved project. This test does NOT mock ConfigureEngramCloudSessionSync.
func TestUpdate_EnvOverridesWriteEnvBlockAndHook(t *testing.T) {
	home := t.TempDir()
	runner := newTestCommandRunner(home)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()
	seedResolvableGit(t)

	// Set env overrides - the manifest is deliberately empty
	serverOverride := "http://127.0.0.1:18080"
	projectOverride := "click-ai-devkit"
	tokenOverride := "consented-token-123"
	t.Setenv("CLICK_ENGRAM_CLOUD_SERVER", serverOverride)
	t.Setenv("CLICK_ENGRAM_CLOUD_PROJECT", projectOverride)
	t.Setenv("ENGRAM_CLOUD_TOKEN", tokenOverride)
	t.Setenv("CLICK_CLAUDE_HOME", home)
	t.Setenv("CLICK_STATE_HOME", t.TempDir())

	// Run update with --persist-engram-cloud-token to give consent
	root := NewRootCommand()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetIn(&bytes.Buffer{})
	root.SetArgs([]string{"update", "--persist-engram-cloud-token"})

	if err := root.Execute(); err != nil {
		t.Fatalf("update command error = %v, output:\n%s", err, out.String())
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
		t.Fatalf("env[\"ENGRAM_CLOUD_TOKEN\"] = %v, want %q", got, tokenOverride)
	}

	// Assert the managed SessionStart hook exists with the resolved project
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

// TestUpdate_NoProcessTokenPreservesStoredToken is the F2 regression guard: when settings.json already
// contains a click-owned ENGRAM_CLOUD_TOKEN and update runs without a token in the environment, the
// stored token must remain present and unchanged (NoOp mode - never delete).
func TestUpdate_NoProcessTokenPreservesStoredToken(t *testing.T) {
	home := t.TempDir()
	runner := newTestCommandRunner(home)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()
	seedResolvableGit(t)

	// Seed settings.json with a previously-consented token
	storedToken := "previously-consented-token-456"
	settingsPath := filepath.Join(home, "settings.json")
	initialSettings := fmt.Sprintf(`{
  "env": {
    "ENGRAM_CLOUD_TOKEN": "%s"
  }
}
`, storedToken)
	if err := os.WriteFile(settingsPath, []byte(initialSettings), 0o600); err != nil {
		t.Fatalf("seed settings.json: %v", err)
	}

	// Set server/project overrides but NO token in environment
	serverOverride := "http://127.0.0.1:18080"
	projectOverride := "click-ai-devkit"
	t.Setenv("CLICK_ENGRAM_CLOUD_SERVER", serverOverride)
	t.Setenv("CLICK_ENGRAM_CLOUD_PROJECT", projectOverride)
	// ENGRAM_CLOUD_TOKEN intentionally absent
	t.Setenv("CLICK_CLAUDE_HOME", home)
	t.Setenv("CLICK_STATE_HOME", t.TempDir())

	// Run update (non-interactive, no consent needed since no token present)
	root := NewRootCommand()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetIn(&bytes.Buffer{})
	root.SetArgs([]string{"update", "--yes"})

	if err := root.Execute(); err != nil {
		t.Fatalf("update command error = %v, output:\n%s", err, out.String())
	}

	// Read the actual settings.json file on disk
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings.json: %v", err)
	}

	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("parse settings.json: %v", err)
	}

	// Assert the stored token is still present and unchanged
	env, ok := settings["env"].(map[string]any)
	if !ok {
		t.Fatalf("settings[\"env\"] not present or not a map")
	}

	if got := env["ENGRAM_CLOUD_TOKEN"]; got != storedToken {
		t.Fatalf("env[\"ENGRAM_CLOUD_TOKEN\"] = %v, want %q (stored token must be preserved when no process token exists)", got, storedToken)
	}
}

// TestUpdate_DeclineWithProcessTokenRemovesStoredToken pins the F2 distinction: when a token is present
// in the environment and persistence is declined, the stored token must be removed (Decline mode).
// This ensures a future change cannot collapse the two states again.
func TestUpdate_DeclineWithProcessTokenRemovesStoredToken(t *testing.T) {
	home := t.TempDir()
	runner := newTestCommandRunner(home)
	restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
	defer restoreRunner()
	seedResolvableGit(t)

	// Seed settings.json with a previously-consented token
	storedToken := "previously-consented-token-789"
	settingsPath := filepath.Join(home, "settings.json")
	initialSettings := fmt.Sprintf(`{
  "env": {
    "ENGRAM_CLOUD_TOKEN": "%s"
  }
}
`, storedToken)
	if err := os.WriteFile(settingsPath, []byte(initialSettings), 0o600); err != nil {
		t.Fatalf("seed settings.json: %v", err)
	}

	// Set server/project overrides AND a NEW token in environment (simulating consent decline)
	serverOverride := "http://127.0.0.1:18080"
	projectOverride := "click-ai-devkit"
	processToken := "new-process-token-123"
	t.Setenv("CLICK_ENGRAM_CLOUD_SERVER", serverOverride)
	t.Setenv("CLICK_ENGRAM_CLOUD_PROJECT", projectOverride)
	t.Setenv("ENGRAM_CLOUD_TOKEN", processToken)
	t.Setenv("CLICK_CLAUDE_HOME", home)
	t.Setenv("CLICK_STATE_HOME", t.TempDir())

	// Run update WITHOUT --persist-engram-cloud-token (decline persistence)
	root := NewRootCommand()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetIn(&bytes.Buffer{})
	root.SetArgs([]string{"update", "--yes"})

	if err := root.Execute(); err != nil {
		t.Fatalf("update command error = %v, output:\n%s", err, out.String())
	}

	// Read the actual settings.json file on disk
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings.json: %v", err)
	}

	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("parse settings.json: %v", err)
	}

	// Assert the stored token was removed (Decline mode deletes it)
	env, ok := settings["env"].(map[string]any)
	if !ok {
		t.Fatalf("settings[\"env\"] not present or not a map")
	}

	if _, present := env["ENGRAM_CLOUD_TOKEN"]; present {
		t.Fatalf("env[\"ENGRAM_CLOUD_TOKEN\"] present, want absent (declined persistence must remove stored token)")
	}
}
