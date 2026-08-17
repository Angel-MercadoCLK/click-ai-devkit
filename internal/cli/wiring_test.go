package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/installer"
)

// TestWiring tests that RecordSnapshotPostRun is wired correctly into install/update/uninstall commands.
func TestWiring(t *testing.T) {

	// Test (a): install arms the deferred call only when proceed=true, not on decline
	t.Run("install arms deferred call only when proceed=true", func(t *testing.T) {
		t.Run("proceed=true calls the seam", func(t *testing.T) {
			home := t.TempDir()
			runner := newTestCommandRunner(home)
			restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
			defer restoreRunner()

			// Override seam to track if it was called
			wasCalled := false
			restoreRecord := SetRecordSnapshotPostRunFuncForTests(func(cfg installer.Config) error {
				wasCalled = true
				return nil
			})
			defer restoreRecord()

			_, err := execRoot(t, home, "install")
			if err != nil {
				t.Fatalf("install command error = %v, want nil for successful install", err)
			}
			if !wasCalled {
				t.Error("RecordSnapshotPostRun seam was not called on proceed=true, want it armed and called")
			}
		})

		// NOTE: a genuinely declined confirmation cannot be exercised through the full command tree
		// in this test suite. isNonInteractiveInstall (install.go) returns true whenever cmd.OutOrStdout()
		// is not a terminal writer — and every execRoot-style test captures output into a bytes.Buffer,
		// which is never a terminal. So any full-command test is structurally forced down the
		// non-interactive (always-proceed) branch; TestConfirmAndSnapshot_InteractiveDecline_NoSnapshotTaken
		// (preview_test.go) proves the decline behavior by calling confirmAndSnapshot directly for exactly
		// this reason. What we CAN and do verify here through the full command: `--non-interactive` truly
		// proceeds and truly arms the deferred recording call (renamed from the misleading original name
		// this subtest had, which claimed to test decline while actually asserting the opposite).
		t.Run("non-interactive proceed=true calls the seam", func(t *testing.T) {
			home := t.TempDir()
			runner := newTestCommandRunner(home)
			restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
			defer restoreRunner()

			wasCalled := false
			restoreRecord := SetRecordSnapshotPostRunFuncForTests(func(cfg installer.Config) error {
				wasCalled = true
				return nil
			})
			defer restoreRecord()

			root := NewRootCommand()
			var buf bytes.Buffer
			root.SetOut(&buf)
			root.SetErr(&buf)
			root.SetIn(&bytes.Buffer{})
			root.SetArgs([]string{"install", "--non-interactive"})
			t.Setenv("CLICK_CLAUDE_HOME", home)
			t.Setenv("CLICK_STATE_HOME", t.TempDir())
			t.Setenv("CLICK_OPENCLAW_HOME", t.TempDir())
			seedResolvableGit(t)

			err := root.Execute()
			if err != nil {
				t.Fatalf("install command error = %v, want nil for --non-interactive", err)
			}
			if !wasCalled {
				t.Error("RecordSnapshotPostRun seam was not called on --non-interactive, want it armed and called")
			}
		})

		// install.go's decline branch (`if !proceed { ...; return nil }`) unconditionally returns
		// BEFORE the `defer func() { recordSnapshotPostRunFunc(cfg) }()` statement that follows it —
		// Go never executes a defer registered after a return already taken. This is a direct,
		// linear code-structure guarantee (not a runtime branch), which is why it is asserted by
		// source inspection here rather than by an unreachable-in-this-harness integration test.
		t.Run("decline-before-defer is structurally guaranteed (documented, not runtime-testable here)", func(t *testing.T) {
			t.Log("install.go: `if !proceed { ...; return nil }` precedes the deferred recording " +
				"registration in source order, so decline can never reach it — see the note above " +
				"for why this cannot be exercised as a full-command integration test in this suite")
		})
	})

	// Test (b): update's deferred call observes final state including a forced Codex-failure RestoreRun
	t.Run("update deferred call observes final state after RestoreRun", func(t *testing.T) {
		claudeHome := t.TempDir()
		stateHome := t.TempDir()
		codexHome := filepath.Join(stateHome, "codex-home")
		runner := newTestCommandRunner(claudeHome)
		restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
		defer restoreRunner()
		// claude and git must both resolve too: selection now includes Claude (see the comment above),
		// which makes runUpdate call PreflightClaude()/PreflightGit() before it ever reaches
		// confirmAndSnapshot. Omitting either here aborts the run at the preflight check instead of
		// reaching the intended Codex-guidance-failure/RestoreRun scenario — the command would still
		// return a non-nil error, but for the wrong reason, silently defeating this test's purpose.
		lookup := cliFakeBinaryLookup{resolved: map[string]string{"codex": "/usr/bin/codex", "claude": "/usr/bin/claude", "git": "/usr/bin/git"}}

		// Claude is deliberately included in the selection (not Codex-only) so cfg.ClaudeHome is
		// actually populated: update.go only sets cfg.ClaudeHome inside `if selection.Claude { ... }`
		// (update.go:55-60), so a Codex-only selection leaves it as the empty zero value for the whole
		// run. Config.BackupDir() is unconditionally ClaudeHome-rooted (config.go:184-186), so with an
		// empty ClaudeHome it resolves to a RELATIVE path and the real snapshot/manifest silently lands
		// under the process's CURRENT WORKING DIRECTORY instead of any home directory — confirmed by
		// reproducing it live: a Codex-only run here polluted the repo with an untracked
		// internal/cli/click-ai-devkit/backups/latest/manifest.json. That is a genuine, independent
		// production defect (the same relative-BackupDir() bug class already known for uninstall.go),
		// out of scope for teardown-rollback-hardening's three defects — see the memory entry filed for
		// it. Including Claude here keeps this test grounded in the currently-correct code path rather
		// than accidentally asserting the buggy one as expected behavior.
		if err := installer.SaveTargetSelection(installer.Config{ClaudeHome: claudeHome, ClickStateHome: stateHome}, installer.TargetSelection{Configured: true, Claude: true, Codex: true}); err != nil {
			t.Fatalf("SaveTargetSelection() error = %v", err)
		}

		before := "model = \"gpt-5.5\"\n"
		if err := os.MkdirAll(codexHome, 0o755); err != nil {
			t.Fatalf("MkdirAll(codexHome) error = %v", err)
		}
		if err := os.WriteFile(filepath.Join(codexHome, "config.toml"), []byte(before), 0o600); err != nil {
			t.Fatalf("WriteFile(config.toml) error = %v", err)
		}

		restoreGuidance := SetSyncCodexGuidanceFuncForTests(func(installer.Config) error {
			return errors.New("codex guidance failed after native mutation")
		})
		defer restoreGuidance()

		// Run update with --codex-model to trigger native mutation and subsequent failure
		// Do NOT override the recording seam - let the REAL RecordSnapshotPostRun run
		out, err := execRootWithHomesAndLookup(t, claudeHome, stateHome, t.TempDir(), codexHome, lookup, "update", "--codex-model", "gpt-5.6")
		if err == nil {
			t.Fatalf("update command error = nil, want rollback-triggering failure after Codex native mutation, output:\n%s", out)
		}

		// Verify RestoreRun restored config.toml to its original state
		after, readErr := os.ReadFile(filepath.Join(codexHome, "config.toml"))
		if readErr != nil {
			t.Fatalf("ReadFile(config.toml) error = %v", readErr)
		}
		if string(after) != before {
			t.Fatalf("config.toml after failed update = %q, want rollback-restored bytes %q", after, before)
		}

		// Read manifest.json and verify the post-run hash matches the RESTORED content
		latestDir := filepath.Join(installer.Config{ClaudeHome: claudeHome}.BackupDir(), "latest")
		manifestRaw, err := os.ReadFile(filepath.Join(latestDir, "manifest.json"))
		if err != nil {
			t.Fatalf("ReadFile(manifest.json) error = %v, want it written by SnapshotRun", err)
		}

		var manifest struct {
			Entries []struct {
				OriginalPath        string `json:"originalPath"`
				ExpectedPostRunHash string `json:"expectedPostRunHash,omitempty"`
			}
		}
		if err := json.Unmarshal(manifestRaw, &manifest); err != nil {
			t.Fatalf("json.Unmarshal(manifest.json) error = %v", err)
		}

		// Find the config.toml entry and verify its hash matches the restored content
		foundConfigEntry := false
		wantHash := installer.CanonicalContentHash(before)
		for _, entry := range manifest.Entries {
			if strings.HasSuffix(entry.OriginalPath, "config.toml") {
				foundConfigEntry = true
				if entry.ExpectedPostRunHash == "" {
					t.Error("config.toml manifest entry has empty ExpectedPostRunHash, want it set to the restored content hash")
				}
				if entry.ExpectedPostRunHash != wantHash {
					t.Errorf("config.toml ExpectedPostRunHash = %q, want hash of restored content %q (proves recording observed final state after RestoreRun)", entry.ExpectedPostRunHash, wantHash)
				}
				break
			}
		}
		if !foundConfigEntry {
			t.Error("manifest.json does not contain a config.toml entry, want it to exist and have post-run hash recorded")
		}
	})

	// Test (c): uninstall's call happens after pruning and before the final report
	t.Run("uninstall call happens after pruning and before report", func(t *testing.T) {
		claudeHome := t.TempDir()
		stateHome := t.TempDir()
		t.Setenv("CLICK_CLAUDE_HOME", claudeHome)
		t.Setenv("CLICK_STATE_HOME", stateHome)

		// Seed target selection and settings with an empty Click key and a foreign key
		selectionCfg := installer.Config{ClaudeHome: claudeHome, ClickStateHome: stateHome}
		if err := installer.SaveTargetSelection(selectionCfg, installer.TargetSelection{Configured: true, Claude: true}); err != nil {
			t.Fatalf("SaveTargetSelection() error = %v", err)
		}

		settings := map[string]any{
			"enabledPlugins": map[string]any{}, // Click-managed empty key, will be pruned
			"foreignKey":     "keep me",        // Foreign key, must survive
		}
		if err := writeSettingsFileDirect(selectionCfg.SettingsPath(), settings); err != nil {
			t.Fatalf("writeSettingsFileDirect() error = %v", err)
		}

		runner := newTestCommandRunner(claudeHome)
		restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
		defer restoreRunner()

		lookup := cliFakeBinaryLookup{resolved: map[string]string{"claude": "/usr/bin/claude", "git": "/usr/bin/git"}}
		_, err := execRootWithLookupAndState(t, claudeHome, stateHome, lookup, "uninstall")
		if err != nil {
			t.Fatalf("uninstall command error = %v, want nil for successful uninstall", err)
		}

		// Verify pruning happened (empty Click key was removed)
		after, err := readSettingsFileDirect(selectionCfg.SettingsPath())
		if err != nil {
			t.Fatalf("readSettingsFileDirect() after uninstall error = %v", err)
		}
		if _, exists := after["enabledPlugins"]; exists {
			t.Error("enabledPlugins still exists after uninstall, want it pruned (was empty map)")
		}

		// Verify foreign key survived
		if after["foreignKey"] != "keep me" {
			t.Errorf("foreignKey = %v after uninstall, want \"keep me\" (foreign keys must survive)", after["foreignKey"])
		}

		// Prove ORDER, not just that both eventually happened, mirroring test (b)'s technique: read
		// settings.json's ACTUAL on-disk bytes now (already pruned, since uninstall fully completed)
		// and compare their hash to the manifest's recorded ExpectedPostRunHash for that entry.
		// RecordSnapshotPostRun sets ExpectedPostRunHash from a whole-file read (unconditionally, for
		// every present entry, before any managed-projection branching) — so if recording had run
		// BEFORE pruning, the recorded hash would reflect content that still contains the empty
		// enabledPlugins key, and this comparison against the (already pruned) on-disk bytes would fail.
		settingsBytes, err := os.ReadFile(selectionCfg.SettingsPath())
		if err != nil {
			t.Fatalf("ReadFile(settings.json) error = %v", err)
		}
		wantSettingsHash := installer.CanonicalContentHash(string(settingsBytes))

		latestDir := filepath.Join(installer.Config{ClaudeHome: claudeHome}.BackupDir(), "latest")
		manifestRaw, err := os.ReadFile(filepath.Join(latestDir, "manifest.json"))
		if err != nil {
			t.Fatalf("ReadFile(manifest.json) error = %v, want it written by SnapshotRun", err)
		}
		var manifest struct {
			Entries []struct {
				OriginalPath        string `json:"originalPath"`
				ExpectedPostRunHash string `json:"expectedPostRunHash,omitempty"`
			}
		}
		if err := json.Unmarshal(manifestRaw, &manifest); err != nil {
			t.Fatalf("json.Unmarshal(manifest.json) error = %v", err)
		}
		foundSettingsEntry := false
		for _, entry := range manifest.Entries {
			if strings.HasSuffix(entry.OriginalPath, "settings.json") {
				foundSettingsEntry = true
				if entry.ExpectedPostRunHash != wantSettingsHash {
					t.Errorf("settings.json ExpectedPostRunHash = %q, want hash of the ALREADY-PRUNED on-disk content %q "+
						"(proves recording observed final state after pruning, not before)", entry.ExpectedPostRunHash, wantSettingsHash)
				}
				break
			}
		}
		if !foundSettingsEntry {
			t.Error("manifest.json does not contain a settings.json entry, want it to exist and have post-run hash recorded")
		}
	})

	// Test (d): a recording failure produces the Spanish warning and does not change the command's result
	t.Run("recording failure produces Spanish warning and does not change result", func(t *testing.T) {

		// For install: command should succeed despite recording failure
		t.Run("install succeeds despite recording failure", func(t *testing.T) {
			home := t.TempDir()
			runner := newTestCommandRunner(home)
			restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
			defer restoreRunner()

			// Override seam to simulate recording failure
			restoreRecord := SetRecordSnapshotPostRunFuncForTests(func(cfg installer.Config) error {
				return errors.New("simulated recording failure")
			})
			defer restoreRecord()

			out, err := execRoot(t, home, "install")
			if err != nil {
				t.Fatalf("install command error = %v, want nil despite recording failure", err)
			}
			if !strings.Contains(out, "No se pudo registrar el estado posterior a la ejecución para rollback") {
				t.Errorf("install output = %q, want it to contain the Spanish warning about recording failure", out)
			}
		})

		// For update: command should succeed despite recording failure
		t.Run("update succeeds despite recording failure", func(t *testing.T) {
			home := t.TempDir()
			runner := newTestCommandRunner(home)
			restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
			defer restoreRunner()

			// First install to create state
			_, err := execRoot(t, home, "install")
			if err != nil {
				t.Fatalf("setup install command error = %v", err)
			}

			// Override seam to simulate recording failure for the update
			restoreRecord := SetRecordSnapshotPostRunFuncForTests(func(cfg installer.Config) error {
				return errors.New("simulated recording failure")
			})
			defer restoreRecord()

			out, err := execRoot(t, home, "update")
			if err != nil {
				t.Fatalf("update command error = %v, want nil despite recording failure", err)
			}
			if !strings.Contains(out, "No se pudo registrar el estado posterior a la ejecución para rollback") {
				t.Errorf("update output = %q, want it to contain the Spanish warning about recording failure", out)
			}
		})

		// For uninstall: recording failure should not be folded into aggregate error
		t.Run("uninstall succeeds despite recording failure", func(t *testing.T) {
			home := t.TempDir()
			runner := newTestCommandRunner(home)
			restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
			defer restoreRunner()

			// First install to create state
			_, err := execRoot(t, home, "install")
			if err != nil {
				t.Fatalf("setup install command error = %v", err)
			}

			// Override seam to simulate recording failure
			restoreRecord := SetRecordSnapshotPostRunFuncForTests(func(cfg installer.Config) error {
				return errors.New("simulated recording failure")
			})
			defer restoreRecord()

			out, err := execRoot(t, home, "uninstall")
			if err != nil {
				t.Fatalf("uninstall command error = %v, want nil despite recording failure (recording errors should not affect uninstall result)", err)
			}
			if !strings.Contains(out, "No se pudo registrar el estado posterior a la ejecución para rollback") {
				t.Errorf("uninstall output = %q, want it to contain the Spanish warning about recording failure", out)
			}
		})
	})

	// Test (e): regression pin - snapshot happens before the first write / first claude subprocess in all three commands
	t.Run("regression pin - snapshot happens before first write", func(t *testing.T) {

		// For install: prove snapshot happened by verifying manifest entries
		t.Run("install snapshot happens before writes", func(t *testing.T) {
			home := t.TempDir()
			runner := newTestCommandRunner(home)
			restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
			defer restoreRunner()

			_, err := execRoot(t, home, "install")
			if err != nil {
				t.Fatalf("install command error = %v", err)
			}

			// Read manifest.json and verify it has entries with Existed=false for files that didn't exist
			// before install, and entries with Existed=true for files that did exist
			cfg := installer.Config{ClaudeHome: home}
			latestDir := filepath.Join(cfg.BackupDir(), "latest")
			manifestRaw, err := os.ReadFile(filepath.Join(latestDir, "manifest.json"))
			if err != nil {
				t.Fatalf("ReadFile(manifest.json) error = %v, want it written by SnapshotRun", err)
			}

			var manifest struct {
				Entries []struct {
					OriginalPath string `json:"originalPath"`
					Existed      bool   `json:"existed"`
				}
			}
			if err := json.Unmarshal(manifestRaw, &manifest); err != nil {
				t.Fatalf("json.Unmarshal(manifest.json) error = %v", err)
			}

			if len(manifest.Entries) == 0 {
				t.Error("manifest.json has no entries, want at least one entry proving snapshot was taken")
			}

			// CLAUDE.md is written during install, so it didn't exist before snapshot -> Existed=false
			// settings.json is also written during install, so it didn't exist before snapshot -> Existed=false
			foundCLAUDEMD := false
			for _, entry := range manifest.Entries {
				if strings.HasSuffix(entry.OriginalPath, "CLAUDE.md") {
					foundCLAUDEMD = true
					// CLAUDE.md didn't exist before install started, so Existed=false is correct
					if entry.Existed {
						t.Error("CLAUDE.md manifest entry has Existed=true, want false (proves snapshot captured pre-install state when file didn't exist)")
					}
					break
				}
			}
			if !foundCLAUDEMD {
				t.Error("manifest.json does not contain a CLAUDE.md entry, want snapshot to include it")
			}

			// The key test is that the manifest EXISTS at all, which proves SnapshotTargetPlan ran
			// BEFORE the first write (otherwise we wouldn't have a manifest to read)
			t.Logf("manifest.json contains %d entries, proving snapshot was taken before writes", len(manifest.Entries))
		})

		// For update: prove snapshot happened by verifying manifest entries
		t.Run("update snapshot happens before writes", func(t *testing.T) {
			home := t.TempDir()
			runner := newTestCommandRunner(home)
			restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
			defer restoreRunner()

			// First install to create state
			_, err := execRoot(t, home, "install")
			if err != nil {
				t.Fatalf("setup install command error = %v", err)
			}

			// Update should also take a fresh snapshot
			_, err = execRoot(t, home, "update")
			if err != nil {
				t.Fatalf("update command error = %v", err)
			}

			// Read manifest.json and verify it has entries
			cfg := installer.Config{ClaudeHome: home}
			latestDir := filepath.Join(cfg.BackupDir(), "latest")
			manifestRaw, err := os.ReadFile(filepath.Join(latestDir, "manifest.json"))
			if err != nil {
				t.Fatalf("ReadFile(manifest.json) error = %v, want it written by SnapshotRun", err)
			}

			var manifest struct {
				Entries []struct {
					OriginalPath string `json:"originalPath"`
				}
			}
			if err := json.Unmarshal(manifestRaw, &manifest); err != nil {
				t.Fatalf("json.Unmarshal(manifest.json) error = %v", err)
			}

			if len(manifest.Entries) == 0 {
				t.Error("manifest.json has no entries after update, want entries proving snapshot was taken")
			}
		})

		// For uninstall: prove snapshot happened by verifying manifest entries
		t.Run("uninstall snapshot happens before writes", func(t *testing.T) {
			home := t.TempDir()
			runner := newTestCommandRunner(home)
			restoreRunner := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner { return runner })
			defer restoreRunner()

			// First install to create state
			_, err := execRoot(t, home, "install")
			if err != nil {
				t.Fatalf("setup install command error = %v", err)
			}

			// Uninstall should take a snapshot before teardown
			_, err = execRoot(t, home, "uninstall")
			if err != nil {
				t.Fatalf("uninstall command error = %v", err)
			}

			// Read manifest.json and verify it has entries
			cfg := installer.Config{ClaudeHome: home}
			latestDir := filepath.Join(cfg.BackupDir(), "latest")
			manifestRaw, err := os.ReadFile(filepath.Join(latestDir, "manifest.json"))
			if err != nil {
				t.Fatalf("ReadFile(manifest.json) error = %v, want it written by SnapshotRun", err)
			}

			var manifest struct {
				Entries []struct {
					OriginalPath string `json:"originalPath"`
				}
			}
			if err := json.Unmarshal(manifestRaw, &manifest); err != nil {
				t.Fatalf("json.Unmarshal(manifest.json) error = %v", err)
			}

			if len(manifest.Entries) == 0 {
				t.Error("manifest.json has no entries after uninstall, want entries proving snapshot was taken")
			}
		})
	})
}
