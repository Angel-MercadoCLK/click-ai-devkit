package installer

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

var dotHookWantTimeouts = map[string]int{
	"PermissionRequest":  120,
	"SessionStart":       10,
	"SessionEnd":         10,
	"UserPromptSubmit":   10,
	"PreToolUse":         10,
	"PostToolUse":        10,
	"PostToolUseFailure": 10,
	"Notification":       10,
	"Stop":               10,
}

func dotHooksCfg(t *testing.T) Config {
	t.Helper()
	return Config{ClaudeHome: t.TempDir()}
}

func readSettingsMap(t *testing.T, cfg Config) map[string]any {
	t.Helper()
	data, err := os.ReadFile(cfg.SettingsPath())
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("parse settings: %v", err)
	}
	return settings
}

func seedDotSettings(t *testing.T, cfg Config, raw string) {
	t.Helper()
	if err := os.MkdirAll(cfg.ClaudeHome, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg.SettingsPath(), []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
}

// ownedDotHooks returns every Click Dot hook in settings as event -> list of {matcherPresent, command, timeout}.
func ownedDotHooks(t *testing.T, settings map[string]any) map[string][]map[string]any {
	t.Helper()
	result := map[string][]map[string]any{}
	hooks, _ := settings["hooks"].(map[string]any)
	for event, raw := range hooks {
		entries, _ := raw.([]any)
		for _, rawEntry := range entries {
			entry, _ := rawEntry.(map[string]any)
			list, _ := entry["hooks"].([]any)
			for _, rawHook := range list {
				hook, _ := rawHook.(map[string]any)
				command, _ := hook["command"].(string)
				if strings.HasPrefix(command, "click dot-hook") {
					_, hasMatcher := entry["matcher"]
					result[event] = append(result[event], map[string]any{
						"hasMatcher": hasMatcher, "type": hook["type"], "command": command, "timeout": hook["timeout"], "groupSize": len(list),
					})
				}
			}
		}
	}
	return result
}

func TestRegisterDotHooks_WritesOneUnmatchedGroupPerEventWithExactEntries(t *testing.T) {
	cfg := dotHooksCfg(t)
	if err := RegisterDotHooks(cfg); err != nil {
		t.Fatalf("RegisterDotHooks() error = %v", err)
	}
	owned := ownedDotHooks(t, readSettingsMap(t, cfg))
	if len(owned) != len(dotHookWantTimeouts) {
		t.Fatalf("registered events = %d, want %d: %v", len(owned), len(dotHookWantTimeouts), owned)
	}
	for event, timeout := range dotHookWantTimeouts {
		got := owned[event]
		if len(got) != 1 {
			t.Fatalf("%s: got %d Click Dot hooks, want exactly 1", event, len(got))
		}
		hook := got[0]
		if hook["hasMatcher"] != false || hook["type"] != "command" || hook["command"] != "click dot-hook "+event || hook["groupSize"] != 1 {
			t.Fatalf("%s: unexpected entry %v", event, hook)
		}
		if hook["timeout"] != float64(timeout) {
			t.Fatalf("%s: timeout = %v, want %d", event, hook["timeout"], timeout)
		}
	}
}

func TestRegisterDotHooks_NeverUsesOnFailureBlock(t *testing.T) {
	cfg := dotHooksCfg(t)
	if err := RegisterDotHooks(cfg); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(cfg.SettingsPath())
	if bytes.Contains(data, []byte("onFailure")) {
		t.Fatalf("settings must not carry onFailure: %s", data)
	}
}

func TestRegisterDotHooks_IsIdempotentAndDoesNotRewriteAnUnchangedFile(t *testing.T) {
	cfg := dotHooksCfg(t)
	if err := RegisterDotHooks(cfg); err != nil {
		t.Fatal(err)
	}
	first, _ := os.ReadFile(cfg.SettingsPath())
	// Make an unchanged rewrite observable: a rewrite would normalize this trailing marker away.
	marked := append(append([]byte(nil), first...), []byte("\n")...)
	if err := os.WriteFile(cfg.SettingsPath(), marked, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RegisterDotHooks(cfg); err != nil {
		t.Fatal(err)
	}
	second, _ := os.ReadFile(cfg.SettingsPath())
	if !bytes.Equal(second, marked) {
		t.Fatal("second RegisterDotHooks must not rewrite an already-registered settings.json")
	}
	owned := ownedDotHooks(t, readSettingsMap(t, cfg))
	for event := range dotHookWantTimeouts {
		if len(owned[event]) != 1 {
			t.Fatalf("%s duplicated after second run: %v", event, owned[event])
		}
	}
}

func TestRegisterDotHooks_RepairsDuplicatesWrongTimeoutAndStaleEvents(t *testing.T) {
	cfg := dotHooksCfg(t)
	seedDotSettings(t, cfg, `{"hooks":{
	  "Stop":[{"hooks":[{"type":"command","command":"click dot-hook Stop","timeout":99}]},{"hooks":[{"type":"command","command":"click dot-hook Stop","timeout":10}]}],
	  "FutureEvent":[{"hooks":[{"type":"command","command":"click dot-hook FutureEvent","timeout":10}]}]
	}}`)
	if err := RegisterDotHooks(cfg); err != nil {
		t.Fatal(err)
	}
	settings := readSettingsMap(t, cfg)
	owned := ownedDotHooks(t, settings)
	if len(owned["Stop"]) != 1 || owned["Stop"][0]["timeout"] != float64(10) {
		t.Fatalf("Stop not normalized to one entry with timeout 10: %v", owned["Stop"])
	}
	if _, stale := owned["FutureEvent"]; stale {
		t.Fatalf("stale Click Dot hook under an unknown event must be removed: %v", owned["FutureEvent"])
	}
	if _, present := settings["hooks"].(map[string]any)["FutureEvent"]; present {
		t.Fatal("emptied stale event key must be pruned")
	}
}

func TestRegisterDotHooks_CoexistsWithMemoryGuardEngramCloudAndThirdParty(t *testing.T) {
	cfg := dotHooksCfg(t)
	if err := RegisterMemoryGuardHook(cfg); err != nil {
		t.Fatal(err)
	}
	writeThirdParty := `{"hooks":{
	  "SessionStart":[
	    {"matcher":"","hooks":[{"type":"command","command":"click engram-cloud-import --project-b64 YWJj || true"}]},
	    {"matcher":"startup","hooks":[{"type":"command","command":"third-party start"}]}
	  ],
	  "Stop":[{"hooks":[{"type":"command","command":"third-party stop"},{"type":"command","command":"click dot-hook Stop","timeout":5}]}]
	}}`
	guardSettings := readSettingsMap(t, cfg)
	var thirdParty map[string]any
	_ = json.Unmarshal([]byte(writeThirdParty), &thirdParty)
	thirdHooks := thirdParty["hooks"].(map[string]any)
	guardSettings["hooks"].(map[string]any)["SessionStart"] = thirdHooks["SessionStart"]
	guardSettings["hooks"].(map[string]any)["Stop"] = thirdHooks["Stop"]
	if err := writeSettingsFile(cfg.SettingsPath(), guardSettings); err != nil {
		t.Fatal(err)
	}

	if err := RegisterDotHooks(cfg); err != nil {
		t.Fatal(err)
	}
	settings := readSettingsMap(t, cfg)
	hooks := settings["hooks"].(map[string]any)

	if !hasHookEntry(getPreToolUseEntries(settings), MemoryGuardToolMatcher(), MemoryGuardCommand) {
		t.Fatal("memory guard entry must survive")
	}
	sessionStart, _ := json.Marshal(hooks["SessionStart"])
	for _, want := range []string{"click engram-cloud-import --project-b64 YWJj || true", "third-party start", "click dot-hook SessionStart"} {
		if !strings.Contains(string(sessionStart), want) {
			t.Fatalf("SessionStart lost %q: %s", want, sessionStart)
		}
	}
	stop, _ := json.Marshal(hooks["Stop"])
	if !strings.Contains(string(stop), "third-party stop") {
		t.Fatalf("third-party Stop hook sharing a group with a Click Dot hook must survive: %s", stop)
	}
	if got := ownedDotHooks(t, settings)["Stop"]; len(got) != 1 || got[0]["timeout"] != float64(10) {
		t.Fatalf("Stop Click Dot hook not normalized: %v", got)
	}
}

func TestUnregisterDotHooks_RemovesOnlyClickDotEntriesAndPrunesEmptyKeys(t *testing.T) {
	cfg := dotHooksCfg(t)
	if err := RegisterMemoryGuardHook(cfg); err != nil {
		t.Fatal(err)
	}
	if err := RegisterDotHooks(cfg); err != nil {
		t.Fatal(err)
	}
	settings := readSettingsMap(t, cfg)
	hooks := settings["hooks"].(map[string]any)
	hooks["Stop"] = append(hooks["Stop"].([]any), map[string]any{"hooks": []any{map[string]any{"type": "command", "command": "third-party stop"}}})
	if err := writeSettingsFile(cfg.SettingsPath(), settings); err != nil {
		t.Fatal(err)
	}

	if err := UnregisterDotHooks(cfg); err != nil {
		t.Fatalf("UnregisterDotHooks() error = %v", err)
	}
	after := readSettingsMap(t, cfg)
	if owned := ownedDotHooks(t, after); len(owned) != 0 {
		t.Fatalf("Click Dot hooks remain: %v", owned)
	}
	if !hasHookEntry(getPreToolUseEntries(after), MemoryGuardToolMatcher(), MemoryGuardCommand) {
		t.Fatal("memory guard must survive Click Dot unregister")
	}
	afterHooks := after["hooks"].(map[string]any)
	if _, ok := afterHooks["Stop"]; !ok {
		t.Fatal("Stop with a third-party hook must remain")
	}
	for _, emptied := range []string{"PermissionRequest", "SessionStart", "SessionEnd", "UserPromptSubmit", "PostToolUse", "PostToolUseFailure", "Notification"} {
		if _, ok := afterHooks[emptied]; ok {
			t.Fatalf("emptied %s key must be pruned", emptied)
		}
	}
}

func TestUnregisterDotHooks_PrunesHooksKeyWhenNothingElseRemainsAndSkipsWritesWhenAbsent(t *testing.T) {
	cfg := dotHooksCfg(t)
	if err := UnregisterDotHooks(cfg); err != nil {
		t.Fatalf("missing settings must be a no-op: %v", err)
	}
	if _, err := os.Stat(cfg.SettingsPath()); !os.IsNotExist(err) {
		t.Fatal("unregister must not create settings.json")
	}
	seedDotSettings(t, cfg, "{\"theme\":\"dark\"}\n")
	if err := RegisterDotHooks(cfg); err != nil {
		t.Fatal(err)
	}
	if err := UnregisterDotHooks(cfg); err != nil {
		t.Fatal(err)
	}
	after := readSettingsMap(t, cfg)
	if _, ok := after["hooks"]; ok || after["theme"] != "dark" {
		t.Fatalf("hooks key must be pruned and foreign keys kept: %v", after)
	}

	seedDotSettings(t, cfg, "{\"theme\":\"dark\"}   \n")
	before, _ := os.ReadFile(cfg.SettingsPath())
	if err := UnregisterDotHooks(cfg); err != nil {
		t.Fatal(err)
	}
	now, _ := os.ReadFile(cfg.SettingsPath())
	if !bytes.Equal(before, now) {
		t.Fatal("unregister must not rewrite a settings.json that carries no Click Dot hook")
	}
}

func TestRestore_RemovesClickDotHooksAbsentFromBackupAndRestoresTheOnesInIt(t *testing.T) {
	dotGroup := `{"hooks":[{"type":"command","command":"click dot-hook Stop","timeout":10}]}`
	current := []byte(`{"hooks":{"Stop":[` + dotGroup + `,{"hooks":[{"type":"command","command":"third-party stop"}]}],"SessionEnd":[` + strings.ReplaceAll(dotGroup, "Stop", "SessionEnd") + `]}}`)

	// Backup without Click Dot hooks: rollback of an install removes ours and keeps the third party.
	backupWithout := []byte(`{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"third-party stop"}]}]}}`)
	data, write, remove, err := prepareSettingsRestore(current, backupWithout, true)
	if err != nil || !write || remove {
		t.Fatalf("prepareSettingsRestore() = write %v remove %v err %v", write, remove, err)
	}
	var restored map[string]any
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	if owned := ownedDotHooks(t, restored); len(owned) != 0 {
		t.Fatalf("Click Dot hooks must be removed on rollback: %v", owned)
	}
	if !strings.Contains(string(data), "third-party stop") || strings.Contains(string(data), "SessionEnd") {
		t.Fatalf("third party kept and emptied key pruned: %s", data)
	}

	// Backup carrying Click Dot hooks: they come back (current had them stripped by the user).
	currentBare := []byte(`{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"third-party stop"}]}]}}`)
	backupWith := []byte(`{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"third-party stop"}]},` + dotGroup + `]}}`)
	data, write, _, err = prepareSettingsRestore(currentBare, backupWith, true)
	if err != nil || !write {
		t.Fatalf("restore from backup with Click Dot hooks: write %v err %v", write, err)
	}
	restored = nil
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	if got := ownedDotHooks(t, restored)["Stop"]; len(got) != 1 {
		t.Fatalf("Click Dot Stop hook not restored: %v", got)
	}
	if !strings.Contains(string(data), "third-party stop") {
		t.Fatalf("third party lost: %s", data)
	}
}

func TestRestore_ValidatesClickDotEventStructureInBackup(t *testing.T) {
	good := []byte(`{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"click dot-hook Stop"}]}]}}`)
	if _, err := parseSettingsStrict(good); err != nil {
		t.Fatalf("valid Click Dot hooks rejected: %v", err)
	}
	for name, raw := range map[string]string{
		"event not an array":    `{"hooks":{"Stop":{"hooks":[]}}}`,
		"entry not an object":   `{"hooks":{"PermissionRequest":["x"]}}`,
		"matcher not a string":  `{"hooks":{"SessionStart":[{"matcher":3,"hooks":[]}]}}`,
		"hooks list not a list": `{"hooks":{"Notification":[{"hooks":"x"}]}}`,
		"pretooluse still bad":  `{"hooks":{"PreToolUse":[1]}}`,
	} {
		if _, err := parseSettingsStrict([]byte(raw)); err == nil {
			t.Fatalf("%s: parseSettingsStrict must reject %s", name, raw)
		}
	}
}

func TestBuildTargetPlan_DotHooksActionsOnlyWhenRequested(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir()}
	selection := TargetSelection{Configured: true, Claude: true}

	without := BuildTargetPlan(cfg, selection, PlanOptions{})
	for _, kinds := range [][]StepActionKind{without.InstallActionKinds(), without.UpdateActionKinds(), without.UninstallActionKinds()} {
		for _, kind := range kinds {
			if kind == StepActionRegisterDotHooks || kind == StepActionUnregisterDotHooks {
				t.Fatalf("Click Dot hook actions must be absent without PlanOptions.DotHooks: %v", kinds)
			}
		}
	}

	with := BuildTargetPlan(cfg, selection, PlanOptions{DotHooks: true})
	if !reflect.DeepEqual(filterDotHookKinds(with.InstallActionKinds()), []StepActionKind{StepActionRegisterDotHooks}) ||
		!reflect.DeepEqual(filterDotHookKinds(with.UpdateActionKinds()), []StepActionKind{StepActionRegisterDotHooks}) ||
		!reflect.DeepEqual(filterDotHookKinds(with.UninstallActionKinds()), []StepActionKind{StepActionUnregisterDotHooks}) {
		t.Fatalf("install/update/uninstall must carry register/register/unregister: %v %v %v", with.InstallActionKinds(), with.UpdateActionKinds(), with.UninstallActionKinds())
	}

	codexOnly := BuildTargetPlan(Config{CodexHome: t.TempDir()}, TargetSelection{Configured: true, Codex: true}, PlanOptions{DotHooks: true})
	if len(filterDotHookKinds(codexOnly.InstallActionKinds())) != 0 {
		t.Fatal("a Codex-only selection must never register Claude Code hooks")
	}
}

func filterDotHookKinds(kinds []StepActionKind) []StepActionKind {
	var out []StepActionKind
	for _, kind := range kinds {
		if kind == StepActionRegisterDotHooks || kind == StepActionUnregisterDotHooks {
			out = append(out, kind)
		}
	}
	return out
}

func TestDotHooksApplicable_RequiresWindowsAndAReleaseOrExistingInstall(t *testing.T) {
	cases := []struct {
		name    string
		goos    string
		version string
		exe     bool
		want    bool
	}{
		{"windows release", "windows", "1.2.3", false, true},
		{"windows dev build without Dot", "windows", "dev", false, false},
		{"windows dev build with Dot present", "windows", "dev", true, true},
		{"linux release", "linux", "1.2.3", false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			local := t.TempDir()
			appData := t.TempDir()
			manager := DotManager{GOOS: tc.goos, GOARCH: "amd64", Getenv: func(k string) string {
				return map[string]string{"LOCALAPPDATA": local, "APPDATA": appData}[k]
			}}
			if tc.exe {
				p, _ := manager.paths()
				if err := os.MkdirAll(p.installDir, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(p.exe, []byte("x"), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			t.Cleanup(SetDotManagerFactoryForTests(func() DotManager { return manager }))
			if got := DotHooksApplicable(tc.version); got != tc.want {
				t.Fatalf("DotHooksApplicable(%q) = %v, want %v", tc.version, got, tc.want)
			}
		})
	}
}
