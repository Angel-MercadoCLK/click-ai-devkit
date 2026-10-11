package installer

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type sddRetirementRunner struct {
	listOutput   []byte
	listErr      error
	uninstallErr error
	commands     []commandInvocation
	beforeRun    func()
}

func (r *sddRetirementRunner) Output(name string, args ...string) ([]byte, error) {
	r.commands = append(r.commands, commandInvocation{Name: name, Args: append([]string(nil), args...)})
	return append([]byte(nil), r.listOutput...), r.listErr
}

func (r *sddRetirementRunner) Run(name string, args ...string) error {
	r.commands = append(r.commands, commandInvocation{Name: name, Args: append([]string(nil), args...)})
	if r.beforeRun != nil {
		r.beforeRun()
	}
	return r.uninstallErr
}

func TestRetireLegacyClickSDD_BackupsUserConfigurationBeforeUninstall(t *testing.T) {
	claudeHome, stateHome := t.TempDir(), t.TempDir()
	cfg := Config{ClaudeHome: claudeHome, ClickStateHome: stateHome}
	settings := []byte(`{"model":"keep","pluginConfigs":{"click-sdd@click-ai-devkit":{"options":{"legacy_model":"opus","future":7},"custom":{"items":[true,3]}},"click-memory@click-ai-devkit":{"options":{"guard":true}}},"hooks":{"SessionStart":[{"command":"foreign-hook"}]}}`)
	if err := os.WriteFile(cfg.SettingsPath(), settings, 0o600); err != nil {
		t.Fatal(err)
	}
	fileSynced := false
	oldFileSync := retirementFileSync
	retirementFileSync = func(file *os.File) error {
		err := file.Sync()
		if err == nil {
			fileSynced = true
		}
		return err
	}
	t.Cleanup(func() { retirementFileSync = oldFileSync })
	runner := &sddRetirementRunner{listOutput: []byte(`[{"id":"click-sdd@click-ai-devkit","scope":"user","enabled":false},{"id":"click-memory@click-ai-devkit","scope":"user","enabled":true}]`)}
	runner.beforeRun = func() {
		if !fileSynced {
			t.Error("backup file was not synced before uninstall")
		}
		files, globErr := filepath.Glob(filepath.Join(stateHome, clickSDDRetirementDir, "click-sdd-*.json"))
		if globErr != nil || len(files) != 1 {
			t.Errorf("backup files before uninstall = %v, error = %v; want exactly one durable backup", files, globErr)
			return
		}
		if _, statErr := os.Stat(files[0]); statErr != nil {
			t.Errorf("backup before uninstall: %v", statErr)
		}
	}
	backup, err := retireLegacyClickSDDWithRunner(cfg, runner)
	if err != nil {
		t.Fatalf("retireLegacyClickSDDWithRunner() error = %v", err)
	}
	if len(runner.commands) != 2 || runner.commands[0].Name != "claude" || strings.Join(runner.commands[0].Args, " ") != "plugin list --json" ||
		runner.commands[1].Name != "claude" || strings.Join(runner.commands[1].Args, " ") != "plugin uninstall click-sdd@click-ai-devkit --scope user --keep-data" {
		t.Fatalf("commands = %#v, want list then exact user-scope keep-data uninstall", runner.commands)
	}
	backupData, err := os.ReadFile(backup)
	if err != nil {
		t.Fatalf("read durable backup: %v", err)
	}
	var saved clickSDDRetirementBackup
	if err := json.Unmarshal(backupData, &saved); err != nil {
		t.Fatalf("decode backup: %v", err)
	}
	if saved.PluginID != ClickSDDPluginID || saved.Scope != "user" || !saved.ConfigExists {
		t.Errorf("backup metadata = %+v, want exact plugin identity, user scope, and present config", saved)
	}
	var savedConfig map[string]any
	if err := json.Unmarshal(saved.PluginConfig, &savedConfig); err != nil {
		t.Fatalf("decode backed-up plugin config: %v", err)
	}
	options, ok := savedConfig["options"].(map[string]any)
	if !ok || options["legacy_model"] != "opus" || options["future"] != float64(7) {
		t.Errorf("backed-up plugin config = %#v, want complete option values including unknown fields", savedConfig)
	}
	custom, ok := savedConfig["custom"].(map[string]any)
	if !ok || custom["items"] == nil {
		t.Errorf("backed-up plugin config = %#v, want the complete config entry beyond options", savedConfig)
	}
	if strings.Contains(backup, filepath.Join("backups", "latest")) {
		t.Errorf("persistent retirement backup is inside rotating snapshot tree: %s", backup)
	}
	info, err := os.Stat(backup)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Errorf("backup mode = %o, want 600", info.Mode().Perm())
	}
	dirInfo, err := os.Stat(filepath.Dir(backup))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && dirInfo.Mode().Perm()&0o077 != 0 {
		t.Errorf("backup directory mode = %o, want no group/other permissions", dirInfo.Mode().Perm())
	}
	if got, err := os.ReadFile(cfg.SettingsPath()); err != nil || !bytes.Equal(got, settings) {
		t.Errorf("settings changed: err=%v, got=%s", err, got)
	}
}

func TestRetireLegacyClickSDD_PreservesOtherScopesAndSkipsAbsentUserPlugin(t *testing.T) {
	tests := []struct {
		name          string
		items         string
		wantUninstall bool
	}{
		{name: "project", items: `[{"id":"click-sdd@click-ai-devkit","scope":"project"}]`},
		{name: "local", items: `[{"id":"click-sdd@click-ai-devkit","scope":"local"}]`},
		{name: "managed", items: `[{"id":"click-sdd@click-ai-devkit","scope":"managed"}]`},
		{name: "session", items: `[{"id":"click-sdd@click-ai-devkit","scope":"session"}]`},
		{name: "synced", items: `[{"id":"click-sdd@click-ai-devkit","scope":"synced"}]`},
		{name: "multiple project installs", items: `[{"id":"click-sdd@click-ai-devkit","scope":"project","projectPath":"/one"},{"id":"click-sdd@click-ai-devkit","scope":"project","projectPath":"/two"}]`},
		{name: "multiple local installs", items: `[{"id":"click-sdd@click-ai-devkit","scope":"local","projectPath":"/one"},{"id":"click-sdd@click-ai-devkit","scope":"local","projectPath":"/two"}]`},
		{name: "absent", items: `[{"id":"click-memory@click-ai-devkit","scope":"user"}]`},
		{name: "same plugin in foreign scope and user scope", items: `[{"id":"click-sdd@click-ai-devkit","scope":"project"},{"id":"click-sdd@click-ai-devkit","scope":"user"}]`, wantUninstall: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := &sddRetirementRunner{listOutput: []byte(tt.items)}
			backup, err := retireLegacyClickSDDWithRunner(Config{ClaudeHome: t.TempDir(), ClickStateHome: t.TempDir()}, runner)
			if err != nil {
				t.Fatalf("retireLegacyClickSDDWithRunner() error = %v", err)
			}
			if tt.wantUninstall {
				if len(runner.commands) != 2 || strings.Join(runner.commands[1].Args, " ") != "plugin uninstall click-sdd@click-ai-devkit --scope user --keep-data" {
					t.Fatalf("commands = %#v, want user-scope uninstall only", runner.commands)
				}
				if backup == "" {
					t.Fatal("backup path is empty for user installation")
				}
			} else {
				if len(runner.commands) != 1 || runner.commands[0].Args[0] != "plugin" || runner.commands[0].Args[1] != "list" {
					t.Fatalf("commands = %#v, want inventory only", runner.commands)
				}
				if backup != "" {
					t.Errorf("backup = %q, want empty when no user installation exists", backup)
				}
			}
		})
	}
}

func TestRetireLegacyClickSDD_SanitizesRunnerErrorsAndPreservesCause(t *testing.T) {
	secret := errors.New("sensitive option sentinel")
	tests := []struct {
		name   string
		runner *sddRetirementRunner
	}{
		{name: "inventory", runner: &sddRetirementRunner{listErr: secret}},
		{name: "uninstall", runner: &sddRetirementRunner{listOutput: []byte(`[{"id":"click-sdd@click-ai-devkit","scope":"user"}]`), uninstallErr: secret}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := retireLegacyClickSDDWithRunner(Config{ClaudeHome: t.TempDir(), ClickStateHome: t.TempDir()}, tt.runner)
			if !errors.Is(err, secret) {
				t.Errorf("error = %v, want original cause to remain discoverable", err)
			}
			if err != nil && strings.Contains(err.Error(), secret.Error()) {
				t.Errorf("display error leaked subprocess output: %v", err)
			}
		})
	}
}

func TestRetirementBackupPrivacy_IsAppliedAndVerifiedPortably(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "private-dir")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := setRetirementPathPrivacy(dir, true); err != nil {
		t.Fatalf("set directory privacy: %v", err)
	}
	if ok, err := retirementPathPrivacySatisfied(dir, true); err != nil || !ok {
		t.Fatalf("directory privacy check = %v, %v; want private", ok, err)
	}
	file := filepath.Join(dir, "backup.json")
	if err := os.WriteFile(file, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := setRetirementPathPrivacy(file, false); err != nil {
		t.Fatalf("set file privacy: %v", err)
	}
	if ok, err := retirementPathPrivacySatisfied(file, false); err != nil || !ok {
		t.Fatalf("file privacy check = %v, %v; want private", ok, err)
	}
}

func TestRetireLegacyClickSDD_FailsClosedOnUntrustedInventoryOrSettings(t *testing.T) {
	tests := []struct {
		name      string
		inventory string
		settings  string
	}{
		{name: "invalid JSON", inventory: `{`},
		{name: "wrong top-level shape", inventory: `{}`},
		{name: "missing scope", inventory: `[{"id":"click-sdd@click-ai-devkit"}]`},
		{name: "missing identity", inventory: `[{"scope":"user"}]`},
		{name: "unknown scope", inventory: `[{"id":"click-sdd@click-ai-devkit","scope":"future"}]`},
		{name: "duplicate identity", inventory: `[{"id":"click-sdd@click-ai-devkit","scope":"user"},{"id":"click-sdd@click-ai-devkit","scope":"user"}]`},
		{name: "invalid settings JSON", inventory: `[{"id":"click-sdd@click-ai-devkit","scope":"user"}]`, settings: `{`},
		{name: "invalid pluginConfigs type", inventory: `[{"id":"click-sdd@click-ai-devkit","scope":"user"}]`, settings: `{"pluginConfigs":[]}`},
		{name: "invalid plugin config entry", inventory: `[{"id":"click-sdd@click-ai-devkit","scope":"user"}]`, settings: `{"pluginConfigs":{"click-sdd@click-ai-devkit":[]}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claudeHome := t.TempDir()
			if tt.settings != "" {
				if err := os.WriteFile(filepath.Join(claudeHome, "settings.json"), []byte(tt.settings), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			runner := &sddRetirementRunner{listOutput: []byte(tt.inventory)}
			_, err := retireLegacyClickSDDWithRunner(Config{ClaudeHome: claudeHome, ClickStateHome: t.TempDir()}, runner)
			if err == nil {
				t.Fatal("retirement error = nil, want fail-closed validation")
			}
			if len(runner.commands) != 1 {
				t.Errorf("commands = %#v, want inventory only; uninstall must not run", runner.commands)
			}
			if strings.Contains(err.Error(), "opus") || strings.Contains(err.Error(), "secret") {
				t.Errorf("error leaked plugin options: %v", err)
			}
		})
	}
}

func TestRetireLegacyClickSDD_RecordsAbsentSettingsAsAbsent(t *testing.T) {
	stateHome := t.TempDir()
	runner := &sddRetirementRunner{listOutput: []byte(`[{"id":"click-sdd@click-ai-devkit","scope":"user","enabled":true}]`)}
	backup, err := retireLegacyClickSDDWithRunner(Config{ClaudeHome: t.TempDir(), ClickStateHome: stateHome}, runner)
	if err != nil {
		t.Fatalf("retireLegacyClickSDDWithRunner() error = %v", err)
	}
	data, err := os.ReadFile(backup)
	if err != nil {
		t.Fatal(err)
	}
	var saved clickSDDRetirementBackup
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.PluginID != ClickSDDPluginID || saved.Scope != "user" || saved.ConfigExists || len(saved.PluginConfig) != 0 {
		t.Errorf("backup = %+v, want explicit absence of settings/plugin config", saved)
	}
}

func TestRetireLegacyClickSDD_SyncsParentForNewBackupDirectory(t *testing.T) {
	stateHome := t.TempDir()
	runner := &sddRetirementRunner{listOutput: []byte(`[{"id":"click-sdd@click-ai-devkit","scope":"user"}]`)}
	var syncedParent bool
	oldDirSync := retirementDirSync
	retirementDirSync = func(path string) error {
		if filepath.Clean(path) == filepath.Clean(stateHome) {
			syncedParent = true
		}
		return syncRetirementDirectory(path)
	}
	t.Cleanup(func() { retirementDirSync = oldDirSync })
	runner.beforeRun = func() {
		if !syncedParent {
			t.Error("backup directory parent was not synced before uninstall")
		}
	}
	if _, err := retireLegacyClickSDDWithRunner(Config{ClaudeHome: t.TempDir(), ClickStateHome: stateHome}, runner); err != nil {
		t.Fatalf("retireLegacyClickSDDWithRunner() error = %v", err)
	}
	if !syncedParent {
		t.Error("parent directory was not synced after creating the persistent backup directory")
	}
}

func TestRetireLegacyClickSDD_RequiresDurableBackupBeforeUninstall(t *testing.T) {
	claudeHome := t.TempDir()
	if err := os.WriteFile(filepath.Join(claudeHome, "settings.json"), []byte(`{"pluginConfigs":{"click-sdd@click-ai-devkit":{"options":{"private":"secret"}}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	stateHome := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(stateHome, []byte("block"), 0o600); err != nil {
		t.Fatal(err)
	}
	runner := &sddRetirementRunner{listOutput: []byte(`[{"id":"click-sdd@click-ai-devkit","scope":"user"}]`)}
	_, err := retireLegacyClickSDDWithRunner(Config{ClaudeHome: claudeHome, ClickStateHome: stateHome}, runner)
	if err == nil {
		t.Fatal("retirement error = nil, want backup failure")
	}
	if len(runner.commands) != 1 {
		t.Errorf("commands = %#v, want no uninstall after backup failure", runner.commands)
	}
	if strings.Contains(err.Error(), "secret") {
		t.Errorf("error leaked configuration data: %v", err)
	}
}

func TestRetireLegacyClickSDD_RejectsSymlinkBackupDirectory(t *testing.T) {
	stateHome := t.TempDir()
	claudeHome := t.TempDir()
	if err := os.WriteFile(filepath.Join(claudeHome, "settings.json"), []byte(`{"pluginConfigs":{"click-sdd@click-ai-devkit":{"options":{"profile":"legacy"}}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	otherDir := t.TempDir()
	if err := os.Symlink(otherDir, filepath.Join(stateHome, clickSDDRetirementDir)); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	runner := &sddRetirementRunner{listOutput: []byte(`[{"id":"click-sdd@click-ai-devkit","scope":"user"}]`)}
	_, err := retireLegacyClickSDDWithRunner(Config{ClaudeHome: claudeHome, ClickStateHome: stateHome}, runner)
	if err == nil {
		t.Fatal("retirement error = nil, want unsafe backup symlink rejection")
	}
	if len(runner.commands) != 1 {
		t.Errorf("commands = %#v, want no uninstall through symlink", runner.commands)
	}
}

func TestRetireLegacyClickSDD_ReuseExactBackupAndRejectCorruptBackup(t *testing.T) {
	claudeHome, stateHome := t.TempDir(), t.TempDir()
	settings := []byte(`{"pluginConfigs":{"click-sdd@click-ai-devkit":{"options":{"profile":"legacy"}}}}`)
	if err := os.WriteFile(filepath.Join(claudeHome, "settings.json"), settings, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := Config{ClaudeHome: claudeHome, ClickStateHome: stateHome}
	inventory := []byte(`[{"id":"click-sdd@click-ai-devkit","scope":"user"}]`)
	first := &sddRetirementRunner{listOutput: inventory}
	path, err := retireLegacyClickSDDWithRunner(cfg, first)
	if err != nil {
		t.Fatal(err)
	}
	second := &sddRetirementRunner{listOutput: inventory}
	var syncedFile bool
	oldFileSync := retirementFileSync
	retirementFileSync = func(file *os.File) error {
		if filepath.Clean(file.Name()) == filepath.Clean(path) {
			syncedFile = true
		}
		return file.Sync()
	}
	t.Cleanup(func() { retirementFileSync = oldFileSync })
	second.beforeRun = func() {
		if !syncedFile {
			t.Error("reused backup file was not synced before uninstall")
		}
	}
	path2, err := retireLegacyClickSDDWithRunner(cfg, second)
	if err != nil {
		t.Fatalf("repeat retirement: %v", err)
	}
	if path2 != path {
		t.Errorf("repeat backup path = %q, want stable %q", path2, path)
	}
	if len(second.commands) != 2 {
		t.Errorf("repeat commands = %#v, want inventory and idempotent uninstall", second.commands)
	}
	if !syncedFile {
		t.Error("reused backup file was not synced before uninstall")
	}
	if err := os.WriteFile(path, []byte(`{"corrupt":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	third := &sddRetirementRunner{listOutput: inventory}
	_, err = retireLegacyClickSDDWithRunner(cfg, third)
	if err == nil {
		t.Fatal("corrupt backup accepted; want fail closed")
	}
	if len(third.commands) != 1 {
		t.Errorf("commands after corrupt backup = %#v, want no uninstall", third.commands)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(stateHome, "unrelated")
	if err := os.WriteFile(target, []byte("unrelated"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	fourth := &sddRetirementRunner{listOutput: inventory}
	_, err = retireLegacyClickSDDWithRunner(cfg, fourth)
	if err == nil {
		t.Fatal("symlink backup accepted; want fail closed")
	}
	if len(fourth.commands) != 1 {
		t.Errorf("commands after symlink backup = %#v, want no uninstall", fourth.commands)
	}
}

func TestRetireLegacyClickSDD_LeavesBackupWhenUninstallFails(t *testing.T) {
	claudeHome, stateHome := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(claudeHome, "settings.json"), []byte(`{"pluginConfigs":{"click-sdd@click-ai-devkit":{"options":{"profile":"legacy"}}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	runner := &sddRetirementRunner{listOutput: []byte(`[{"id":"click-sdd@click-ai-devkit","scope":"user"}]`), uninstallErr: errors.New("external failure")}
	backup, err := retireLegacyClickSDDWithRunner(Config{ClaudeHome: claudeHome, ClickStateHome: stateHome}, runner)
	if err == nil {
		t.Fatal("retirement error = nil, want uninstall failure")
	}
	if len(runner.commands) != 2 {
		t.Fatalf("commands = %#v, want list then uninstall", runner.commands)
	}
	if _, statErr := os.Stat(backup); statErr != nil {
		t.Errorf("durable backup missing after uninstall failure: %v", statErr)
	}
	if strings.Contains(err.Error(), "legacy") {
		t.Errorf("error leaked plugin configuration value: %v", err)
	}
}

func TestRetireLegacyClickSDD_DoesNotUninstallWhenInventoryCommandFails(t *testing.T) {
	runner := &sddRetirementRunner{listErr: errors.New("list failed")}
	_, err := retireLegacyClickSDDWithRunner(Config{ClaudeHome: t.TempDir(), ClickStateHome: t.TempDir()}, runner)
	if err == nil {
		t.Fatal("retirement error = nil, want inventory command failure")
	}
	if len(runner.commands) != 1 || runner.commands[0].Args[1] != "list" {
		t.Errorf("commands = %#v, want only failed inventory command", runner.commands)
	}
}

func TestRetireLegacyClickSDD_DurabilityFailuresStopBeforeUninstall(t *testing.T) {
	const inventory = `[{"id":"click-sdd@click-ai-devkit","scope":"user"}]`
	tests := []struct {
		name                string
		reuseBackup         bool
		missingStateHome    bool
		failFileSync        bool
		failDirectorySyncAt string
		wantBackupPreserved bool
	}{
		{name: "new backup file sync failure", failFileSync: true},
		{name: "reused backup file sync failure", reuseBackup: true, failFileSync: true, wantBackupPreserved: true},
		{name: "new state home parent sync failure", missingStateHome: true, failDirectorySyncAt: "state-home-parent"},
		{name: "backup directory parent sync failure", failDirectorySyncAt: "state-home"},
		{name: "final backup directory sync failure", failDirectorySyncAt: "backup-dir"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claudeHome := t.TempDir()
			if err := os.WriteFile(filepath.Join(claudeHome, "settings.json"), []byte(`{"pluginConfigs":{"click-sdd@click-ai-devkit":{"options":{"profile":"sensitive-value"}}}}`), 0o600); err != nil {
				t.Fatal(err)
			}
			stateParent := t.TempDir()
			stateHome := stateParent
			if tt.missingStateHome {
				stateHome = filepath.Join(stateParent, "state-home")
			}
			cfg := Config{ClaudeHome: claudeHome, ClickStateHome: stateHome}
			var expectedBackup string
			var expectedData []byte
			if tt.reuseBackup {
				seedRunner := &sddRetirementRunner{listOutput: []byte(inventory)}
				var seedErr error
				expectedBackup, seedErr = retireLegacyClickSDDWithRunner(cfg, seedRunner)
				if seedErr != nil {
					t.Fatalf("seed retirement backup: %v", seedErr)
				}
				expectedData, seedErr = os.ReadFile(expectedBackup)
				if seedErr != nil {
					t.Fatal(seedErr)
				}
			}

			diskFault := errors.New("sensitive-value sync failure")
			oldFileSync := retirementFileSync
			oldDirSync := retirementDirSync
			retirementFileSync = func(file *os.File) error {
				if tt.failFileSync {
					return diskFault
				}
				return file.Sync()
			}
			retirementDirSync = func(path string) error {
				fail := false
				switch tt.failDirectorySyncAt {
				case "state-home-parent":
					fail = filepath.Clean(path) == filepath.Clean(stateParent)
				case "state-home":
					fail = filepath.Clean(path) == filepath.Clean(stateHome)
				case "backup-dir":
					fail = filepath.Clean(path) == filepath.Join(filepath.Clean(stateHome), clickSDDRetirementDir)
				}
				if fail {
					return diskFault
				}
				return syncRetirementDirectory(path)
			}
			t.Cleanup(func() {
				retirementFileSync = oldFileSync
				retirementDirSync = oldDirSync
			})

			runner := &sddRetirementRunner{listOutput: []byte(inventory)}
			backup, err := retireLegacyClickSDDWithRunner(cfg, runner)
			if err == nil {
				t.Fatal("retirement error = nil, want durability failure")
			}
			if strings.Contains(err.Error(), "sensitive-value") {
				t.Errorf("error leaked backed-up value: %v", err)
			}
			if len(runner.commands) != 1 || runner.commands[0].Args[1] != "list" {
				t.Errorf("commands = %#v, want no uninstall after durability failure", runner.commands)
			}
			if tt.wantBackupPreserved {
				if backup != "" && backup != expectedBackup {
					t.Errorf("returned backup = %q, want prior backup %q", backup, expectedBackup)
				}
				got, readErr := os.ReadFile(expectedBackup)
				if readErr != nil || !bytes.Equal(got, expectedData) {
					t.Errorf("existing backup changed: error=%v, bytes=%q", readErr, got)
				}
			} else if !tt.reuseBackup {
				files, globErr := filepath.Glob(filepath.Join(stateHome, clickSDDRetirementDir, "click-sdd-*.json"))
				if globErr != nil {
					t.Fatalf("list backup files: %v", globErr)
				}
				if tt.failDirectorySyncAt == "backup-dir" {
					if len(files) != 1 {
						t.Errorf("backup files after final directory sync failure = %v, want one intact recovery file", files)
					}
				} else if len(files) != 0 {
					t.Errorf("backup files after failed pre-write sync = %v, want none", files)
				}
			}
		})
	}
}

func TestRetireLegacyClickSDD_RetryReestablishesDirectoryDurabilityBeforeUninstall(t *testing.T) {
	const inventory = `[{"id":"click-sdd@click-ai-devkit","scope":"user"}]`
	tests := []struct {
		name             string
		missingStateHome bool
		expectedSyncPath func(stateHome string) string
	}{
		{
			name:             "state home ancestry after parent sync failure",
			missingStateHome: true,
			expectedSyncPath: func(stateHome string) string { return filepath.Dir(stateHome) },
		},
		{
			name:             "recovery directory ancestry after parent sync failure",
			expectedSyncPath: func(stateHome string) string { return stateHome },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stateParent := t.TempDir()
			stateHome := stateParent
			if tt.missingStateHome {
				stateHome = filepath.Join(stateParent, "state-home")
			}
			cfg := Config{ClaudeHome: t.TempDir(), ClickStateHome: stateHome}
			faultPath := filepath.Clean(tt.expectedSyncPath(stateHome))
			diskFault := errors.New("directory persistence fault")
			faultEnabled := true
			oldDirSync := retirementDirSync
			retirementDirSync = func(path string) error {
				if faultEnabled && filepath.Clean(path) == faultPath {
					return diskFault
				}
				return syncRetirementDirectory(path)
			}
			t.Cleanup(func() { retirementDirSync = oldDirSync })

			first := &sddRetirementRunner{listOutput: []byte(inventory)}
			if _, err := retireLegacyClickSDDWithRunner(cfg, first); err == nil {
				t.Fatal("first attempt error = nil, want injected parent sync failure")
			}
			if len(first.commands) != 1 {
				t.Errorf("first attempt commands = %#v, want inventory only", first.commands)
			}

			second := &sddRetirementRunner{listOutput: []byte(inventory)}
			if _, err := retireLegacyClickSDDWithRunner(cfg, second); err == nil {
				t.Fatal("retry error = nil while parent sync fault persists")
			}
			if len(second.commands) != 1 {
				t.Errorf("retry commands = %#v, want inventory only while parent sync keeps failing", second.commands)
			}

			faultEnabled = false
			syncedBeforeUninstall := false
			retirementDirSync = func(path string) error {
				if filepath.Clean(path) == faultPath {
					syncedBeforeUninstall = true
				}
				return syncRetirementDirectory(path)
			}
			third := &sddRetirementRunner{listOutput: []byte(inventory)}
			third.beforeRun = func() {
				if !syncedBeforeUninstall {
					t.Errorf("required parent %q was not synced before uninstall", faultPath)
				}
			}
			if _, err := retireLegacyClickSDDWithRunner(cfg, third); err != nil {
				t.Fatalf("successful retry error = %v", err)
			}
			if len(third.commands) != 2 || !syncedBeforeUninstall {
				t.Errorf("successful retry commands=%#v, parent synced=%v; want sync before user uninstall", third.commands, syncedBeforeUninstall)
			}
		})
	}
}
