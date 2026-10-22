package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/installer"
)

// dotHookSettings reads settings.json under the Claude fixture home.
func dotHookSettings(t *testing.T, home string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(home, "settings.json"))
	if err != nil {
		t.Fatalf("read settings.json: %v", err)
	}
	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("parse settings.json: %v", err)
	}
	return settings
}

// clickDotHookCommands lists event -> commands of every Click Dot hook in settings.
func clickDotHookCommands(settings map[string]any) map[string][]string {
	result := map[string][]string{}
	hooks, _ := settings["hooks"].(map[string]any)
	for event, raw := range hooks {
		entries, _ := raw.([]any)
		for _, rawEntry := range entries {
			entry, _ := rawEntry.(map[string]any)
			list, _ := entry["hooks"].([]any)
			for _, rawHook := range list {
				hook, _ := rawHook.(map[string]any)
				if command, _ := hook["command"].(string); strings.HasPrefix(command, "click dot-hook") {
					result[event] = append(result[event], command)
				}
			}
		}
	}
	return result
}

func seedThirdPartyStopHook(t *testing.T, home string) {
	t.Helper()
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	seed := `{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"third-party stop"}]}]}}` + "\n"
	if err := os.WriteFile(filepath.Join(home, "settings.json"), []byte(seed), 0o600); err != nil {
		t.Fatal(err)
	}
}

func hasMemoryGuardEntry(settings map[string]any) bool {
	hooks, _ := settings["hooks"].(map[string]any)
	data, _ := json.Marshal(hooks["PreToolUse"])
	return strings.Contains(string(data), installer.MemoryGuardCommand)
}

func TestInstallRegistersClickDotHooksOnWindowsAfterDotAndKeepsOtherHooks(t *testing.T) {
	setCLIVersion(t, cliDotVersion)
	rig := newCLIDotRig(t, "windows")
	rig.publish(cliDotVersion)
	home := dotCLIHome(t)
	seedThirdPartyStopHook(t, home)

	out, err := execRoot(t, home, "install")
	if err != nil {
		t.Fatalf("install error = %v\n%s", err, out)
	}
	settings := dotHookSettings(t, home)
	commands := clickDotHookCommands(settings)
	for _, event := range []string{"PermissionRequest", "SessionStart", "SessionEnd", "UserPromptSubmit", "PreToolUse", "PostToolUse", "PostToolUseFailure", "Notification", "Stop"} {
		if len(commands[event]) != 1 || commands[event][0] != "click dot-hook "+event {
			t.Fatalf("%s: Click Dot hook = %v", event, commands[event])
		}
	}
	if !hasMemoryGuardEntry(settings) {
		t.Fatal("the memory guard hook must still be registered")
	}
	stop, _ := json.Marshal(settings["hooks"].(map[string]any)["Stop"])
	if !strings.Contains(string(stop), "third-party stop") {
		t.Fatalf("third-party hook lost: %s", stop)
	}
	if !strings.Contains(out, "Registrando hooks de Click Dot") || strings.Index(out, "Registrando hooks de Click Dot") < strings.Index(out, "[OK] Click Dot "+cliDotVersion+" instalado") {
		t.Fatalf("the hook step must follow the Click Dot install:\n%s", out)
	}
}

func TestInstallDoesNotRegisterClickDotHooksWhenDotCouldNotBeInstalled(t *testing.T) {
	setCLIVersion(t, cliDotVersion)
	rig := newCLIDotRig(t, "windows")
	rig.fetchOK = false
	home := dotCLIHome(t)

	out, err := execRoot(t, home, "install")
	if err != nil {
		t.Fatalf("a Dot failure must not fail install: %v\n%s", err, out)
	}
	if commands := clickDotHookCommands(dotHookSettings(t, home)); len(commands) != 0 {
		t.Fatalf("no Click Dot hook may be registered without Click Dot: %v", commands)
	}
	if !strings.Contains(out, "No se registraron los hooks de Click Dot") {
		t.Fatalf("output must explain the skipped hook step:\n%s", out)
	}
}

func TestInstallOffWindowsRegistersNoClickDotHooksAndKeepsOutput(t *testing.T) {
	setCLIVersion(t, cliDotVersion)
	newCLIDotRig(t, "linux")
	home := dotCLIHome(t)

	out, err := execRoot(t, home, "install")
	if err != nil {
		t.Fatalf("install error = %v\n%s", err, out)
	}
	if commands := clickDotHookCommands(dotHookSettings(t, home)); len(commands) != 0 {
		t.Fatalf("nothing may be registered off Windows: %v", commands)
	}
	if strings.Contains(out, "hooks de Click Dot") {
		t.Fatalf("off-Windows output must not mention the hooks:\n%s", out)
	}
	if !hasMemoryGuardEntry(dotHookSettings(t, home)) {
		t.Fatal("the memory guard registration must be unchanged off Windows")
	}
}

func TestInstallDryRunPreviewsClickDotHooksWithoutWritingSettings(t *testing.T) {
	setCLIVersion(t, cliDotVersion)
	rig := newCLIDotRig(t, "windows")
	home := dotCLIHome(t)

	out, err := execRoot(t, home, "install", "--dry-run")
	if err != nil {
		t.Fatalf("dry-run error = %v\n%s", err, out)
	}
	if !strings.Contains(out, "hooks de Click Dot") {
		t.Fatalf("dry-run must list the hook step:\n%s", out)
	}
	if len(rig.fetched) != 0 {
		t.Fatal("dry-run must not download")
	}
	if _, statErr := os.Stat(filepath.Join(home, "settings.json")); !os.IsNotExist(statErr) {
		t.Fatal("dry-run must not write settings.json")
	}
}

func TestUpdateRegistersClickDotHooksIdempotently(t *testing.T) {
	setCLIVersion(t, cliDotVersion)
	rig := newCLIDotRig(t, "windows")
	rig.publish(cliDotVersion)
	home := dotCLIHome(t)

	for i := 0; i < 2; i++ {
		out, err := execRoot(t, home, "update")
		if err != nil {
			t.Fatalf("update #%d error = %v\n%s", i+1, err, out)
		}
		commands := clickDotHookCommands(dotHookSettings(t, home))
		if len(commands) != 9 {
			t.Fatalf("update #%d registered %d events, want 9: %v", i+1, len(commands), commands)
		}
		for event, list := range commands {
			if len(list) != 1 {
				t.Fatalf("update #%d: %s has %d Click Dot hooks", i+1, event, len(list))
			}
		}
	}
	if !hasMemoryGuardEntry(dotHookSettings(t, home)) {
		t.Fatal("update must keep re-registering the memory guard")
	}
}

func TestUpdateOffWindowsRegistersNoClickDotHooks(t *testing.T) {
	setCLIVersion(t, cliDotVersion)
	newCLIDotRig(t, "linux")
	home := dotCLIHome(t)

	out, err := execRoot(t, home, "update")
	if err != nil {
		t.Fatalf("update error = %v\n%s", err, out)
	}
	if commands := clickDotHookCommands(dotHookSettings(t, home)); len(commands) != 0 {
		t.Fatalf("nothing may be registered off Windows: %v", commands)
	}
	if strings.Contains(out, "hooks de Click Dot") {
		t.Fatalf("off-Windows output must not mention the hooks:\n%s", out)
	}
}

func TestUninstallRemovesOnlyClickDotHooks(t *testing.T) {
	setCLIVersion(t, cliDotVersion)
	rig := newCLIDotRig(t, "windows")
	rig.publish(cliDotVersion)
	home := dotCLIHome(t)
	seedThirdPartyStopHook(t, home)
	if out, err := execRoot(t, home, "install"); err != nil {
		t.Fatalf("install error = %v\n%s", err, out)
	}

	out, err := execRoot(t, home, "uninstall")
	if err != nil {
		t.Fatalf("uninstall error = %v\n%s", err, out)
	}
	settings := dotHookSettings(t, home)
	if commands := clickDotHookCommands(settings); len(commands) != 0 {
		t.Fatalf("Click Dot hooks must be removed: %v", commands)
	}
	hooks, _ := settings["hooks"].(map[string]any)
	stop, _ := json.Marshal(hooks["Stop"])
	if !strings.Contains(string(stop), "third-party stop") {
		t.Fatalf("third-party hook must survive uninstall: %s", stop)
	}
	for _, emptied := range []string{"PermissionRequest", "SessionEnd", "UserPromptSubmit", "Notification"} {
		if _, ok := hooks[emptied]; ok {
			t.Fatalf("emptied %s key must be pruned", emptied)
		}
	}
	if !strings.Contains(out, "Quitando hooks de Click Dot") {
		t.Fatalf("uninstall output lacks the hook step:\n%s", out)
	}
}

func TestUninstallOffWindowsDoesNotTouchOrMentionClickDotHooks(t *testing.T) {
	setCLIVersion(t, cliDotVersion)
	newCLIDotRig(t, "linux")
	home := dotCLIHome(t)

	out, err := execRoot(t, home, "uninstall")
	if err != nil {
		t.Fatalf("uninstall error = %v\n%s", err, out)
	}
	if strings.Contains(out, "hooks de Click Dot") {
		t.Fatalf("off-Windows uninstall must not mention the hooks:\n%s", out)
	}
}

func TestRollbackRestoresSettingsWithoutClickDotHooksAndKeepsThirdParty(t *testing.T) {
	setCLIVersion(t, cliDotVersion)
	rig := newCLIDotRig(t, "windows")
	rig.publish(cliDotVersion)
	home := dotCLIHome(t)
	seedThirdPartyStopHook(t, home)
	stateHome := t.TempDir()
	if out, err := execRootWithStateHome(t, home, stateHome, "install"); err != nil {
		t.Fatalf("install error = %v\n%s", err, out)
	}
	if len(clickDotHookCommands(dotHookSettings(t, home))) != 9 {
		t.Fatal("precondition: install must register the hooks")
	}

	out, err := execRootWithStateHome(t, home, stateHome, "rollback")
	if err != nil {
		t.Fatalf("rollback error = %v\n%s", err, out)
	}
	settings := dotHookSettings(t, home)
	if commands := clickDotHookCommands(settings); len(commands) != 0 {
		t.Fatalf("rollback must remove the Click Dot hooks it did not find in the backup: %v\n%s", commands, out)
	}
	stop, _ := json.Marshal(settings["hooks"].(map[string]any)["Stop"])
	if !strings.Contains(string(stop), "third-party stop") {
		t.Fatalf("third-party hook lost on rollback: %s", stop)
	}
}
