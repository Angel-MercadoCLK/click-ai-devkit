package installer

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrepareRestore_UsesCurrentOwnedContentOnly(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir(), CodexHome: t.TempDir(), OpenClawHome: t.TempDir(), ClickStateHome: t.TempDir()}
	clickBlock := func(body string) string {
		return "# >>> click-ai-devkit (managed) >>>\n" + body + "\n# <<< click-ai-devkit (managed) <<<\n"
	}
	claudeBefore := clickBlock("old Click guidance")
	claudeNow := "new user before\n" + clickBlock("new Click guidance") + "new user after\n"
	writeTestFile(t, cfg.ClaudeMDPath(), claudeNow)
	codexNow := "codex user text\n" + clickBlock("new Codex managed text") + "codex tail\n"
	openClawAgentsNow := "OpenClaw user text\n" + clickBlock("new OpenClaw managed text")
	openClawSoulNow := "soul header\n" + clickBlock("new Soul text") + "soul tail\n"
	writeTestFile(t, cfg.CodexAgentsMDPath(), codexNow)
	writeTestFile(t, cfg.OpenClawAgentsMDPath(), openClawAgentsNow)
	writeTestFile(t, cfg.OpenClawSoulMDPath(), openClawSoulNow)
	writeTestFile(t, cfg.SettingsPath(), "{\"env\":{\"ENGRAM_CLOUD_TOKEN\":\"current-secret\",\"KEEP\":\"now\"},\"hooks\":{\"PreToolUse\":[{\"matcher\":\"x\",\"hooks\":[{\"type\":\"command\",\"command\":\"click memory-guard\"},{\"type\":\"command\",\"command\":\"foreign\"}]}],\"SessionStart\":[{\"hooks\":[{\"type\":\"command\",\"command\":\"cloud-import\"}]}]},\"custom\":\"current\"}")
	shared := cfg.Context7ConfigPath()
	writeTestFile(t, shared, "{\"mcpServers\":{\"foreign\":{\"command\":\"current\"}}}")
	unknown := filepath.Join(cfg.ClaudeHome, "unrecognized.json")
	writeTestFile(t, unknown, "{\"foreign\":\"current\"}")

	settingsBefore := "{\"env\":{\"ENGRAM_CLOUD_TOKEN\":\"[REDACTED]\",\"KEEP\":\"old\"},\"hooks\":{\"PreToolUse\":[{\"matcher\":\"old\",\"hooks\":[{\"type\":\"command\",\"command\":\"click memory-guard\"},{\"type\":\"command\",\"command\":\"old-foreign\"}]}],\"SessionStart\":[]},\"custom\":\"old\"}"
	entries := []manifestEntry{
		{OriginalPath: cfg.ClaudeMDPath(), BackupFile: "claude.md", Existed: true, DriftPolicy: DriftPolicyManagedContentVeto},
		{OriginalPath: cfg.CodexAgentsMDPath(), BackupFile: "codex.md", Existed: true, DriftPolicy: DriftPolicyManagedContentVeto},
		{OriginalPath: cfg.OpenClawAgentsMDPath(), BackupFile: "oc-agents.md", Existed: true, DriftPolicy: DriftPolicyManagedContentVeto},
		{OriginalPath: cfg.OpenClawSoulMDPath(), BackupFile: "oc-soul.md", Existed: true, DriftPolicy: DriftPolicyWholeFileVeto},
		{OriginalPath: cfg.SettingsPath(), BackupFile: "settings.json", Existed: true, DriftPolicy: DriftPolicyManagedContentVeto},
		{OriginalPath: shared, BackupFile: "shared.json", Existed: true, DriftPolicy: DriftPolicyNonVeto},
		{OriginalPath: unknown, BackupFile: "unknown.json", Existed: true, DriftPolicy: DriftPolicyWholeFileVeto},
	}
	writeManifestDirect(t, cfg, runManifest{Entries: entries})
	for name, content := range map[string]string{
		"claude.md": claudeBefore, "codex.md": "old Codex block ignored if absent\n" + clickBlock("old Codex managed text"),
		"oc-agents.md": clickBlock("old OpenClaw managed text"), "oc-soul.md": clickBlock("old Soul text"),
		"settings.json": settingsBefore,
		"shared.json":   "{\"mcpServers\":{\"foreign\":{\"command\":\"backup\"}}}",
		"unknown.json":  "{\"foreign\":\"backup\"}",
	} {
		writeTestFile(t, filepath.Join(snapshotLatestDir(cfg), name), content)
	}

	prepared, err := PrepareRestore(cfg)
	if err != nil {
		t.Fatalf("PrepareRestore() error = %v", err)
	}
	if err := ApplyPreparedRestore(prepared); err != nil {
		t.Fatalf("ApplyPreparedRestore() error = %v", err)
	}
	got, _ := os.ReadFile(cfg.ClaudeMDPath())
	want := "new user before\n" + clickBlock("old Click guidance") + "new user after\n"
	if string(got) != want {
		t.Fatalf("merged markdown = %q, want %q", got, want)
	}
	for _, tc := range []struct{ path, want string }{
		{cfg.CodexAgentsMDPath(), "codex user text\n" + clickBlock("old Codex managed text") + "codex tail\n"},
		{cfg.OpenClawAgentsMDPath(), "OpenClaw user text\n" + clickBlock("old OpenClaw managed text")},
		{cfg.OpenClawSoulMDPath(), "soul header\n" + clickBlock("old Soul text") + "soul tail\n"},
	} {
		if got := string(mustReadFile(t, tc.path)); got != tc.want {
			t.Errorf("merged document %s = %q, want %q", tc.path, got, tc.want)
		}
	}
	got, _ = os.ReadFile(cfg.SettingsPath())
	var settings map[string]any
	if err := json.Unmarshal(got, &settings); err != nil {
		t.Fatal(err)
	}
	env := settings["env"].(map[string]any)
	if env["ENGRAM_CLOUD_TOKEN"] != "current-secret" || env["KEEP"] != "now" || settings["custom"] != "current" {
		t.Fatalf("current settings-owned/foreign values changed: %s", got)
	}
	if string(mustReadFile(t, shared)) != "{\"mcpServers\":{\"foreign\":{\"command\":\"current\"}}}" {
		t.Fatal("shared native config was restored")
	}
	if string(mustReadFile(t, unknown)) != "{\"foreign\":\"current\"}" {
		t.Fatal("unknown historical path was restored")
	}
}

func TestPrepareRestore_RejectsAmbiguousManagedMarkdownBeforeAnyApply(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir(), ClickStateHome: t.TempDir()}
	malformed := "before\n# >>> click-ai-devkit (managed) >>>\nambiguous\n"
	writeTestFile(t, cfg.ClaudeMDPath(), malformed)
	writeTestFile(t, cfg.EngramStatePath(), "{\"current\":true}")
	writeTestFile(t, filepath.Join(snapshotLatestDir(cfg), "private.json"), "{\"backup\":true}")
	writeManifestDirect(t, cfg, runManifest{Entries: []manifestEntry{
		{OriginalPath: cfg.ClaudeMDPath(), BackupFile: "claude.md", Existed: true, DriftPolicy: DriftPolicyManagedContentVeto},
		{OriginalPath: cfg.EngramStatePath(), BackupFile: "private.json", Existed: true, DriftPolicy: DriftPolicyWholeFileVeto},
	}})
	writeTestFile(t, filepath.Join(snapshotLatestDir(cfg), "claude.md"), managedBlockMD("backup"))

	if _, err := PrepareRestore(cfg); err == nil {
		t.Fatal("PrepareRestore() error = nil, want malformed managed markers rejected during preparation")
	}
	if got := string(mustReadFile(t, cfg.EngramStatePath())); got != "{\"current\":true}" {
		t.Fatalf("preflight failure changed private state: %s", got)
	}
}

func TestPrepareRestore_RejectsNoncanonicalManifestPaths(t *testing.T) {
	t.Run("destination with dotdot alias", func(t *testing.T) {
		cfg := Config{ClaudeHome: t.TempDir(), ClickStateHome: t.TempDir()}
		if err := os.Mkdir(filepath.Join(cfg.ClaudeHome, "nested"), 0o755); err != nil {
			t.Fatal(err)
		}
		canonical := cfg.ClaudeMDPath()
		alias := cfg.ClaudeHome + string(os.PathSeparator) + "nested" + string(os.PathSeparator) + ".." + string(os.PathSeparator) + filepath.Base(canonical)
		writeTestFile(t, canonical, managedBlockMD("current"))
		writeTestFile(t, filepath.Join(snapshotLatestDir(cfg), "claude.md"), managedBlockMD("backup"))
		writeManifestDirect(t, cfg, runManifest{Entries: []manifestEntry{{
			OriginalPath: alias, BackupFile: "claude.md", Existed: true, DriftPolicy: DriftPolicyManagedContentVeto,
		}}})

		prepared, err := PrepareRestore(cfg)
		if err == nil {
			if applyErr := ApplyPreparedRestore(prepared); applyErr != nil {
				t.Fatalf("ApplyPreparedRestore() error = %v", applyErr)
			}
			t.Fatal("PrepareRestore() accepted a noncanonical destination alias")
		}
		if got := string(mustReadFile(t, canonical)); got != managedBlockMD("current") {
			t.Fatalf("canonical destination changed after rejecting alias: %q", got)
		}
	})

	t.Run("destination symlink dotdot cannot escape", func(t *testing.T) {
		cfg := Config{ClaudeHome: t.TempDir(), ClickStateHome: t.TempDir()}
		foreign := t.TempDir()
		if err := os.Mkdir(filepath.Join(foreign, "child"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(foreign+string(os.PathSeparator)+"child", filepath.Join(cfg.ClaudeHome, "nested")); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		canonical := cfg.ClaudeMDPath()
		foreignTarget := filepath.Join(foreign, "CLAUDE.md")
		alias := cfg.ClaudeHome + string(os.PathSeparator) + "nested" + string(os.PathSeparator) + ".." + string(os.PathSeparator) + filepath.Base(canonical)
		writeTestFile(t, canonical, managedBlockMD("canonical"))
		writeTestFile(t, foreignTarget, managedBlockMD("foreign"))
		writeTestFile(t, filepath.Join(snapshotLatestDir(cfg), "claude.md"), managedBlockMD("backup"))
		writeManifestDirect(t, cfg, runManifest{Entries: []manifestEntry{{
			OriginalPath: alias, BackupFile: "claude.md", Existed: true, DriftPolicy: DriftPolicyManagedContentVeto,
		}}})

		if _, err := PrepareRestore(cfg); err == nil {
			t.Fatal("PrepareRestore() accepted a symlink/dotdot destination alias")
		}
		if got := string(mustReadFile(t, canonical)); got != managedBlockMD("canonical") {
			t.Fatalf("canonical destination changed: %q", got)
		}
		if got := string(mustReadFile(t, foreignTarget)); got != managedBlockMD("foreign") {
			t.Fatalf("foreign symlink target changed: %q", got)
		}
	})

	t.Run("backup path alias", func(t *testing.T) {
		cfg := Config{ClaudeHome: t.TempDir(), ClickStateHome: t.TempDir()}
		writeTestFile(t, cfg.ClaudeMDPath(), managedBlockMD("current"))
		writeTestFile(t, filepath.Join(snapshotLatestDir(cfg), "claude.md"), managedBlockMD("backup"))
		writeManifestDirect(t, cfg, runManifest{Entries: []manifestEntry{{
			OriginalPath: cfg.ClaudeMDPath(), BackupFile: "nested/../claude.md", Existed: true, DriftPolicy: DriftPolicyManagedContentVeto,
		}}})
		if _, err := PrepareRestore(cfg); err == nil {
			t.Fatal("PrepareRestore() accepted a noncanonical backup path")
		}
		if got := string(mustReadFile(t, cfg.ClaudeMDPath())); got != managedBlockMD("current") {
			t.Fatalf("destination changed after rejecting backup alias: %q", got)
		}
	})
}

func TestPrepareRestore_RejectsDuplicateSettingsKeysBeforeAnyApply(t *testing.T) {
	for _, tc := range []struct {
		name    string
		current string
		backup  string
	}{
		{
			name:    "duplicate nested key in current settings",
			current: `{"env":{"ENGRAM_CLOUD_TOKEN":"current"},"foreign":{"nested":{"value":1,"value":2}},"hooks":{"PreToolUse":[{"matcher":"x","hooks":[{"type":"command","command":"click memory-guard"}]}]}}`,
			backup:  `{"hooks":{"PreToolUse":[{"matcher":"old","hooks":[{"type":"command","command":"click memory-guard"}]}]}}`,
		},
		{
			name:    "duplicate env key in current settings",
			current: `{"env":{"ENGRAM_CLOUD_TOKEN":"first","ENGRAM_CLOUD_TOKEN":"second"},"hooks":{"PreToolUse":[{"matcher":"x","hooks":[{"type":"command","command":"click memory-guard"}]}]}}`,
			backup:  `{"hooks":{"PreToolUse":[{"matcher":"old","hooks":[{"type":"command","command":"click memory-guard"}]}]}}`,
		},
		{
			name:    "duplicate nested key in backup settings",
			current: `{"env":{"ENGRAM_CLOUD_TOKEN":"current"},"hooks":{"PreToolUse":[{"matcher":"x","hooks":[{"type":"command","command":"click memory-guard"}]}]}}`,
			backup:  `{"hooks":{"PreToolUse":[{"matcher":"old","hooks":[{"type":"command","command":"click memory-guard"}]}],"foreign":{"nested":{"value":1,"value":2}}}}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if !json.Valid([]byte(tc.current)) || !json.Valid([]byte(tc.backup)) {
				t.Fatal("duplicate-key fixture must be syntactically valid JSON")
			}
			cfg := Config{ClaudeHome: t.TempDir(), ClickStateHome: t.TempDir()}
			currentMarkdown := "user before\n" + managedBlockMD("current") + "user after\n"
			writeTestFile(t, cfg.ClaudeMDPath(), currentMarkdown)
			writeTestFile(t, cfg.SettingsPath(), tc.current)
			writeTestFile(t, filepath.Join(snapshotLatestDir(cfg), "claude.md"), managedBlockMD("backup"))
			writeTestFile(t, filepath.Join(snapshotLatestDir(cfg), "settings.json"), tc.backup)
			writeManifestDirect(t, cfg, runManifest{Entries: []manifestEntry{
				{OriginalPath: cfg.ClaudeMDPath(), BackupFile: "claude.md", Existed: true, DriftPolicy: DriftPolicyManagedContentVeto},
				{OriginalPath: cfg.SettingsPath(), BackupFile: "settings.json", Existed: true, DriftPolicy: DriftPolicyManagedContentVeto},
			}})
			if _, err := PrepareRestore(cfg); err == nil {
				t.Fatal("PrepareRestore() error = nil, want duplicate JSON keys rejected before any apply")
			}
			if got := string(mustReadFile(t, cfg.ClaudeMDPath())); got != currentMarkdown {
				t.Fatalf("an eligible Markdown destination changed after settings preflight failure: %q", got)
			}
		})
	}
}

func TestApplyPreparedRestore_PreservesExactCurrentNumbersInSettings(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir(), ClickStateHome: t.TempDir()}
	current := `{"hooks":{"PreToolUse":[{"matcher":"x","hooks":[{"type":"command","command":"click memory-guard"}]}]},"foreign":{"large":9007199254740993,"decimal":1.2300,"exponent":1e+02}}`
	backup := `{"hooks":{"PreToolUse":[{"matcher":"old","hooks":[{"type":"command","command":"click memory-guard"}]}]}}`
	writeTestFile(t, cfg.SettingsPath(), current)
	writeTestFile(t, filepath.Join(snapshotLatestDir(cfg), "settings.json"), backup)
	writeManifestDirect(t, cfg, runManifest{Entries: []manifestEntry{{
		OriginalPath: cfg.SettingsPath(), BackupFile: "settings.json", Existed: true, DriftPolicy: DriftPolicyManagedContentVeto,
	}}})
	prepared, err := PrepareRestore(cfg)
	if err != nil {
		t.Fatalf("PrepareRestore() error = %v", err)
	}
	if err := ApplyPreparedRestore(prepared); err != nil {
		t.Fatalf("ApplyPreparedRestore() error = %v", err)
	}
	after := string(mustReadFile(t, cfg.SettingsPath()))
	for _, token := range []string{`"large": 9007199254740993`, `"decimal": 1.2300`, `"exponent": 1e+02`} {
		if !strings.Contains(after, token) {
			t.Errorf("settings lost exact numeric token %s after guard merge: %s", token, after)
		}
	}
}

func TestApplyPreparedRestore_SeparatesHistoricalMarkerFromCurrentSuffix(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir(), ClickStateHome: t.TempDir()}
	backupBlock := managedBeginMarker + "\n" + "historical\n" + managedEndMarker
	current := "user header\n" + managedBlockMD("current") + "user suffix\n"
	writeTestFile(t, cfg.ClaudeMDPath(), current)
	writeTestFile(t, filepath.Join(snapshotLatestDir(cfg), "claude.md"), backupBlock)
	writeManifestDirect(t, cfg, runManifest{Entries: []manifestEntry{{
		OriginalPath: cfg.ClaudeMDPath(), BackupFile: "claude.md", Existed: true, DriftPolicy: DriftPolicyManagedContentVeto,
	}}})
	for i := 0; i < 2; i++ {
		prepared, err := PrepareRestore(cfg)
		if err != nil {
			t.Fatalf("PrepareRestore() iteration %d error = %v", i, err)
		}
		if err := ApplyPreparedRestore(prepared); err != nil {
			t.Fatalf("ApplyPreparedRestore() iteration %d error = %v", i, err)
		}
	}
	want := "user header\n# developer header\n" + backupBlock + "\ndeveloper tail\nuser suffix\n"
	if got := string(mustReadFile(t, cfg.ClaudeMDPath())); got != want {
		t.Fatalf("restored Markdown = %q, want separated historical marker and current suffix %q", got, want)
	}
}

func TestApplyPreparedRestore_NoPriorRemovesOnlyClickMarkersAndGuard(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir(), ClickStateHome: t.TempDir()}
	managed := "# >>> click-ai-devkit (managed) >>>\r\nold\r\n# <<< click-ai-devkit (managed) <<<\r\n"
	currentDoc := "foreign before\r\n" + managed + "foreign after\r\n"
	writeTestFile(t, cfg.ClaudeMDPath(), currentDoc)
	currentSettings := `{"env":{"ENGRAM_CLOUD_TOKEN":"still-current"},"hooks":{"PreToolUse":[{"matcher":"mixed","hooks":[{"type":"command","command":"click memory-guard"},{"type":"command","command":"foreign-hook"}]}]},"foreign":true}`
	writeTestFile(t, cfg.SettingsPath(), currentSettings)
	writeManifestDirect(t, cfg, runManifest{Entries: []manifestEntry{
		{OriginalPath: cfg.ClaudeMDPath(), Existed: false, DriftPolicy: DriftPolicyManagedContentVeto},
		{OriginalPath: cfg.SettingsPath(), Existed: false, DriftPolicy: DriftPolicyManagedContentVeto},
	}})
	prepared, err := PrepareRestore(cfg)
	if err != nil {
		t.Fatalf("PrepareRestore() error = %v", err)
	}
	if err := ApplyPreparedRestore(prepared); err != nil {
		t.Fatalf("ApplyPreparedRestore() error = %v", err)
	}
	if got := string(mustReadFile(t, cfg.ClaudeMDPath())); got != "foreign before\r\nforeign after\r\n" {
		t.Fatalf("no-prior Markdown result = %q, want exact surrounding bytes retained", got)
	}
	var settings map[string]any
	if err := json.Unmarshal(mustReadFile(t, cfg.SettingsPath()), &settings); err != nil {
		t.Fatal(err)
	}
	if settings["foreign"] != true || settings["env"].(map[string]any)["ENGRAM_CLOUD_TOKEN"] != "still-current" {
		t.Fatalf("no-prior settings restore lost current foreign data or Cloud token: %v", settings)
	}
	preTool := settings["hooks"].(map[string]any)["PreToolUse"].([]any)
	if len(preTool) != 1 || preTool[0].(map[string]any)["hooks"].([]any)[0].(map[string]any)["command"] != "foreign-hook" {
		t.Fatalf("foreign hook was not preserved after Click guard removal: %v", preTool)
	}
}

func TestApplyPreparedRestore_PreservesRawMarkdownOutsideMarkers(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir(), ClickStateHome: t.TempDir()}
	current := "foreign  \r\n# >>> click-ai-devkit (managed) >>>\r\nnew\r\n# <<< click-ai-devkit (managed) <<<\r\ntrailing whitespace  \r\n"
	backupBlock := "# >>> click-ai-devkit (managed) >>>\nrestored\n# <<< click-ai-devkit (managed) <<<\n"
	writeTestFile(t, cfg.ClaudeMDPath(), current)
	writeManifestDirect(t, cfg, runManifest{Entries: []manifestEntry{{
		OriginalPath: cfg.ClaudeMDPath(), BackupFile: "claude.md", Existed: true, DriftPolicy: DriftPolicyManagedContentVeto,
	}}})
	writeTestFile(t, filepath.Join(snapshotLatestDir(cfg), "claude.md"), "foreign backup prose\n"+backupBlock+"old tail\n")
	prepared, err := PrepareRestore(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := ApplyPreparedRestore(prepared); err != nil {
		t.Fatal(err)
	}
	want := "foreign  \r\n" + backupBlock + "trailing whitespace  \r\n"
	if got := string(mustReadFile(t, cfg.ClaudeMDPath())); got != want {
		t.Fatalf("Markdown merge = %q, want outside bytes unchanged and only marker block restored as %q", got, want)
	}
}

func TestPrepareRestore_MissingCurrentMarkdownRecreatesOnlyManagedBlock(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir(), ClickStateHome: t.TempDir()}
	backup := "foreign backup prose\n" + managedBlockMD("legacy content") + "backup tail\n"
	writeTestFile(t, filepath.Join(snapshotLatestDir(cfg), "claude.md"), backup)
	writeManifestDirect(t, cfg, runManifest{Entries: []manifestEntry{{
		OriginalPath: cfg.ClaudeMDPath(), BackupFile: "claude.md", Existed: true, DriftPolicy: DriftPolicyManagedContentVeto,
	}}})
	prepared, err := PrepareRestore(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := ApplyPreparedRestore(prepared); err != nil {
		t.Fatal(err)
	}
	got := string(mustReadFile(t, cfg.ClaudeMDPath()))
	lines := crlfAwareSplitLines(backup)
	begin, end := findMarkers(lines)
	want := joinWithLineEnding(lines[begin:end+1], detectLineEnding(backup))
	if got != want {
		t.Fatalf("recreated Markdown = %q, want managed block only %q", got, want)
	}
}

func TestPrepareRestore_RejectsMalformedSettingsAndRedirectedRoot(t *testing.T) {
	t.Run("malformed settings prevents every write", func(t *testing.T) {
		cfg := Config{ClaudeHome: t.TempDir(), ClickStateHome: t.TempDir()}
		writeTestFile(t, cfg.SettingsPath(), `{"hooks":{"PreToolUse":"bad"}}`)
		writeTestFile(t, cfg.EngramStatePath(), `{"current":true}`)
		writeTestFile(t, filepath.Join(snapshotLatestDir(cfg), "state.json"), `{"backup":true}`)
		writeManifestDirect(t, cfg, runManifest{Entries: []manifestEntry{
			{OriginalPath: cfg.SettingsPath(), BackupFile: "settings.json", Existed: true, DriftPolicy: DriftPolicyManagedContentVeto},
			{OriginalPath: cfg.EngramStatePath(), BackupFile: "state.json", Existed: true, DriftPolicy: DriftPolicyWholeFileVeto},
		}})
		writeTestFile(t, filepath.Join(snapshotLatestDir(cfg), "settings.json"), `{}`)
		if _, err := PrepareRestore(cfg); err == nil {
			t.Fatal("PrepareRestore() error = nil, want malformed settings rejected before apply")
		}
		if got := string(mustReadFile(t, cfg.EngramStatePath())); got != `{"current":true}` {
			t.Fatalf("state changed after preparation failure: %s", got)
		}
	})
	t.Run("Claude root symlink is rejected", func(t *testing.T) {
		outside := t.TempDir()
		writeTestFile(t, filepath.Join(outside, "CLAUDE.md"), "foreign\n")
		link := filepath.Join(t.TempDir(), "claude-home")
		if err := os.Symlink(outside, link); err != nil {
			t.Skipf("symlink creation unavailable: %v", err)
		}
		cfg := Config{ClaudeHome: link, ClickStateHome: t.TempDir()}
		writeTestFile(t, filepath.Join(snapshotLatestDir(cfg), "claude.md"), managedBlockMD("backup"))
		writeManifestDirect(t, cfg, runManifest{Entries: []manifestEntry{{
			OriginalPath: cfg.ClaudeMDPath(), BackupFile: "claude.md", Existed: true, DriftPolicy: DriftPolicyManagedContentVeto,
		}}})
		if _, err := PrepareRestore(cfg); err == nil {
			t.Fatal("PrepareRestore() error = nil, want redirected target root rejected")
		}
		if got := string(mustReadFile(t, filepath.Join(outside, "CLAUDE.md"))); got != "foreign\n" {
			t.Fatalf("redirect target changed: %q", got)
		}
	})
}

func TestApplyPreparedRestore_DoesNotRestoreRedactedCloudToken(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir(), ClickStateHome: t.TempDir()}
	matcher := MemoryGuardToolMatcher()
	guard := `{"matcher":"` + matcher + `","hooks":[{"type":"command","command":"` + MemoryGuardCommand + `"}]}`
	backup := `{"env":{"ENGRAM_CLOUD_TOKEN":"[REDACTED]","FOREIGN":"before"},"hooks":{"PreToolUse":[` + guard + `]},"other":true}`
	current := `{"env":{"FOREIGN":"current"},"hooks":{"PreToolUse":[` + guard + `]},"other":false}`
	writeTestFile(t, cfg.SettingsPath(), current)
	writeManifestDirect(t, cfg, runManifest{Entries: []manifestEntry{{
		OriginalPath: cfg.SettingsPath(), BackupFile: "settings.json", Existed: true, DriftPolicy: DriftPolicyManagedContentVeto,
	}}})
	writeTestFile(t, filepath.Join(snapshotLatestDir(cfg), "settings.json"), backup)
	prepared, err := PrepareRestore(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := ApplyPreparedRestore(prepared); err != nil {
		t.Fatal(err)
	}
	after := string(mustReadFile(t, cfg.SettingsPath()))
	var settings map[string]any
	if err := json.Unmarshal([]byte(after), &settings); err != nil {
		t.Fatal(err)
	}
	if _, exists := settings["env"].(map[string]any)["ENGRAM_CLOUD_TOKEN"]; exists || strings.Contains(after, "[REDACTED]") {
		t.Fatalf("restore introduced a token field or placeholder: %s", after)
	}
	if settings["other"] != false || settings["env"].(map[string]any)["FOREIGN"] != "current" {
		t.Fatalf("current foreign settings were not preserved: %s", after)
	}
}

func TestPrepareRestore_SkipsAllResolvedSharedNativeConfigs(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir(), CodexHome: t.TempDir(), OpenClawHome: t.TempDir(), ClickStateHome: t.TempDir()}
	sharedPaths := []string{cfg.Context7ConfigPath(), cfg.KnownMarketplacesPath(), cfg.InstalledPluginsPath(), cfg.CodexConfigPath(), cfg.OpenClawMCPConfigPath()}
	entries := make([]manifestEntry, 0, len(sharedPaths))
	for i, path := range sharedPaths {
		backupName := fmt.Sprintf("shared-%d.json", i)
		entries = append(entries, manifestEntry{OriginalPath: path, BackupFile: backupName, Existed: true, DriftPolicy: DriftPolicyNonVeto})
		writeTestFile(t, path, fmt.Sprintf(`{"current":%d}`, i))
		writeTestFile(t, filepath.Join(snapshotLatestDir(cfg), backupName), fmt.Sprintf(`{"backup":%d}`, i))
	}
	writeManifestDirect(t, cfg, runManifest{Entries: entries})
	prepared, err := PrepareRestore(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(prepared.Drift.Skipped) != len(sharedPaths) {
		t.Fatalf("skipped shared configs = %v, want all %d paths", prepared.Drift.Skipped, len(sharedPaths))
	}
	if err := ApplyPreparedRestore(prepared); err != nil {
		t.Fatal(err)
	}
	for i, path := range sharedPaths {
		want := fmt.Sprintf(`{"current":%d}`, i)
		if got := string(mustReadFile(t, path)); got != want {
			t.Errorf("shared config %s = %q, want current value %q", path, got, want)
		}
	}
}

func TestApplyPreparedRestore_RefusesChangedBackupBeforeAnyWrite(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir(), ClickStateHome: t.TempDir()}
	writeTestFile(t, cfg.ClaudeMDPath(), managedBlockMD("current"))
	writeManifestDirect(t, cfg, runManifest{Entries: []manifestEntry{{
		OriginalPath: cfg.ClaudeMDPath(), BackupFile: "claude.md", Existed: true, DriftPolicy: DriftPolicyManagedContentVeto,
	}}})
	backupPath := filepath.Join(snapshotLatestDir(cfg), "claude.md")
	writeTestFile(t, backupPath, managedBlockMD("original"))
	prepared, err := PrepareRestore(cfg)
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, backupPath, managedBlockMD("changed after preparation"))
	if err := ApplyPreparedRestore(prepared); err == nil || !strings.Contains(err.Error(), "respaldo cambió") {
		t.Fatalf("ApplyPreparedRestore() error = %v, want safe backup mutation refusal", err)
	}
	if got := string(mustReadFile(t, cfg.ClaudeMDPath())); got != managedBlockMD("current") {
		t.Fatalf("current document changed after backup TOCTOU refusal: %q", got)
	}
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	return data
}

// TestPrepareRestore_ProducesReportWithZeroWrites guards the prepare half of the preview/apply
// seam: PrepareRestore must classify drift (veto vs warnable) exactly like SnapshotDrift while
// writing NOTHING — every original file keeps its current content, so an interactive confirmation
// can safely sit between prepare and apply.
func TestPrepareRestore_ProducesReportWithZeroWrites(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir(), ClickStateHome: t.TempDir()}
	sharedPath := cfg.Context7ConfigPath()
	writeTestFile(t, cfg.ClaudeMDPath(), managedBlockMD("managed v1"))
	writeTestFile(t, cfg.SettingsPath(), `{"original":true}`)
	writeTestFile(t, sharedPath, "shared original\n")
	if err := SnapshotRun(cfg); err != nil {
		t.Fatalf("SnapshotRun() error = %v", err)
	}
	manifest, err := loadSnapshotManifest(cfg)
	if err != nil {
		t.Fatalf("loadSnapshotManifest() error = %v", err)
	}
	manifest.Entries = append(manifest.Entries, manifestEntry{
		OriginalPath: sharedPath,
		BackupFile:   "shared.txt",
		Existed:      true,
		DriftPolicy:  DriftPolicyNonVeto,
	})
	writeManifestDirect(t, cfg, manifest)
	writeTestFile(t, filepath.Join(snapshotLatestDir(cfg), "shared.txt"), "shared original\n")

	// Drift: a veto (managed block edit) and a warnable non-veto edit.
	writeTestFile(t, cfg.ClaudeMDPath(), managedBlockMD("hand-edited managed block"))
	writeTestFile(t, sharedPath, "shared edited by someone else\n")

	prepared, err := PrepareRestore(cfg)
	if err != nil {
		t.Fatalf("PrepareRestore() error = %v, want nil", err)
	}
	if len(prepared.Drift.Vetoes) != 1 || prepared.Drift.Vetoes[0] != cfg.ClaudeMDPath() {
		t.Fatalf("PrepareRestore().Drift.Vetoes = %v, want exactly [%s]", prepared.Drift.Vetoes, cfg.ClaudeMDPath())
	}
	if len(prepared.Drift.Skipped) != 1 || prepared.Drift.Skipped[0] != sharedPath {
		t.Fatalf("PrepareRestore().Drift.Skipped = %v, want exact shared config [%s]", prepared.Drift.Skipped, sharedPath)
	}

	// Zero writes: every original file must still hold its current (drifted) content.
	for path, want := range map[string]string{
		cfg.ClaudeMDPath(): managedBlockMD("hand-edited managed block"),
		cfg.SettingsPath(): `{"original":true}`,
		sharedPath:         "shared edited by someone else\n",
	} {
		got, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatalf("ReadFile(%s) after PrepareRestore error = %v", path, readErr)
		}
		if string(got) != want {
			t.Fatalf("%s after PrepareRestore = %q, want untouched %q (prepare must write nothing)", path, got, want)
		}
	}
}

// TestApplyPreparedRestore_RestoresOwnedContentAndSkipsSharedConfig verifies that apply restores
// only the known Click-owned portion and preserves shared native configuration.
func TestApplyPreparedRestore_RestoresOwnedContentAndSkipsSharedConfig(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir(), ClickStateHome: t.TempDir()}
	sharedPath := cfg.Context7ConfigPath()
	writeTestFile(t, cfg.ClaudeMDPath(), managedBlockMD("managed v1"))
	writeTestFile(t, cfg.SettingsPath(), `{"original":true}`)
	writeTestFile(t, sharedPath, `{"mcpServers":{"foreign":{"command":"before"}}}`)
	if err := SnapshotRun(cfg); err != nil {
		t.Fatalf("SnapshotRun() error = %v", err)
	}
	manifest, err := loadSnapshotManifest(cfg)
	if err != nil {
		t.Fatalf("loadSnapshotManifest() error = %v", err)
	}
	manifest.Entries = append(manifest.Entries, manifestEntry{
		OriginalPath: sharedPath,
		BackupFile:   "shared.txt",
		Existed:      true,
		DriftPolicy:  DriftPolicyNonVeto,
	})
	writeManifestDirect(t, cfg, manifest)
	writeTestFile(t, filepath.Join(snapshotLatestDir(cfg), "shared.txt"), `{"mcpServers":{"foreign":{"command":"backup"}}}`)

	writeTestFile(t, cfg.ClaudeMDPath(), managedBlockMD("hand-edited managed block"))
	writeTestFile(t, sharedPath, `{"mcpServers":{"foreign":{"command":"current"}}}`)

	prepared, err := PrepareRestore(cfg)
	if err != nil {
		t.Fatalf("PrepareRestore() error = %v", err)
	}
	if err := ApplyPreparedRestore(prepared); err != nil {
		t.Fatalf("ApplyPreparedRestore() error = %v, want nil", err)
	}
	if len(prepared.Drift.Skipped) != 1 || prepared.Drift.Skipped[0] != sharedPath {
		t.Fatalf("PreparedRestore.Drift.Skipped = %v, want [%s]", prepared.Drift.Skipped, sharedPath)
	}

	for path, want := range map[string]string{
		cfg.ClaudeMDPath(): managedBlockMD("managed v1"),
		cfg.SettingsPath(): `{"original":true}`,
		sharedPath:         `{"mcpServers":{"foreign":{"command":"current"}}}`,
	} {
		got, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatalf("ReadFile(%s) after apply error = %v", path, readErr)
		}
		if string(got) != want {
			t.Fatalf("%s after apply = %q, want restored pre-run content %q", path, got, want)
		}
	}
}

// TestApplyPreparedRestore_RefusesWhenFingerprintChanged guards the TOCTOU guard: if any file's
// on-disk state changed between PrepareRestore and ApplyPreparedRestore (e.g. while an interactive
// confirmation prompt sat in between), apply must refuse BEFORE any write — leaving every file
// untouched, not just the changed one.
func TestApplyPreparedRestore_RefusesWhenFingerprintChanged(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir(), ClickStateHome: t.TempDir()}
	writeTestFile(t, cfg.ClaudeMDPath(), managedBlockMD("managed v1"))
	writeTestFile(t, cfg.SettingsPath(), `{"original":true}`)
	if err := SnapshotRun(cfg); err != nil {
		t.Fatalf("SnapshotRun() error = %v", err)
	}
	writeTestFile(t, cfg.ClaudeMDPath(), managedBlockMD("hand-edited managed block"))

	prepared, err := PrepareRestore(cfg)
	if err != nil {
		t.Fatalf("PrepareRestore() error = %v", err)
	}

	// Something edits a file after prepare (the prompt window).
	writeTestFile(t, cfg.SettingsPath(), `{"original":true,"edited-during-prompt":true}`)

	err = ApplyPreparedRestore(prepared)
	if err == nil {
		t.Fatal("ApplyPreparedRestore() error = nil, want a refusal when a fingerprint changed since prepare")
	}
	if !strings.Contains(err.Error(), "cambió desde la preparación") {
		t.Fatalf("ApplyPreparedRestore() error = %v, want a safe Spanish mutation refusal", err)
	}

	// Zero writes: the veto-drifted CLAUDE.md must NOT have been restored either.
	got, readErr := os.ReadFile(cfg.ClaudeMDPath())
	if readErr != nil {
		t.Fatalf("ReadFile(CLAUDE.md) after refused apply error = %v", readErr)
	}
	if string(got) != managedBlockMD("hand-edited managed block") {
		t.Fatalf("CLAUDE.md after refused apply = %q, want untouched (refusal must precede ANY write)", got)
	}
}

// TestRestoreRun_ComposesPrepareAndApply guards the thin-wrapper contract: RestoreRun restores the
// full manifest (including non-veto entries) without any prompting or drift refusal of its own,
// exactly as install/update's automatic failure-recovery paths require.
func TestRestoreRun_MergesManagedGuidanceAndPreservesForeignSettings(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir(), ClickStateHome: t.TempDir()}
	writeTestFile(t, cfg.ClaudeMDPath(), managedBlockMD("managed v1"))
	writeTestFile(t, cfg.SettingsPath(), `{"original":true}`)
	if err := SnapshotRun(cfg); err != nil {
		t.Fatalf("SnapshotRun() error = %v", err)
	}
	writeTestFile(t, cfg.ClaudeMDPath(), managedBlockMD("drifted, would veto an interactive rollback"))
	writeTestFile(t, cfg.SettingsPath(), `{"drifted":true}`)

	if err := RestoreRun(cfg); err != nil {
		t.Fatalf("RestoreRun() error = %v, want nil (automatic recovery never refuses on drift)", err)
	}
	got, err := os.ReadFile(cfg.ClaudeMDPath())
	if err != nil {
		t.Fatalf("ReadFile(CLAUDE.md) error = %v", err)
	}
	if string(got) != managedBlockMD("managed v1") {
		t.Fatalf("CLAUDE.md after RestoreRun = %q, want restored pre-run content", got)
	}
	if got := string(mustReadFile(t, cfg.SettingsPath())); got != `{"drifted":true}` {
		t.Fatalf("settings after RestoreRun = %q, want non-Click fields preserved", got)
	}
}

// TestPrepareRestore_NoPriorStateNonVetoEntry_WarnsBeforeDeletion is a regression test for a
// review-risk finding: a manifest entry with Existed=false (the file was absent when the run-start
// snapshot was taken) was never classified at all — PrepareRestore's `if entry.Existed` guard
// skipped classifyEntryDrift entirely, so such an entry could never appear in either Vetoes or
// WarnableNonVeto. ApplyPreparedRestore then unconditionally os.Removes it. For a DriftPolicyNonVeto
// entry (a registry file click does not own outright, e.g. known_marketplaces.json/
// installed_plugins.json/Codex config.toml), that meant a real, legitimate file the external
// `claude`/`codex` CLI populated with genuine data AFTER the snapshot was silently deleted by
// `click rollback` with zero warning and zero consent — reproduced empirically before this fix.
//
// The corrected behavior: a present, DriftPolicyNonVeto, Existed=false entry is classified into
// WarnableNonVeto (never Vetoes — there is no baseline to refuse against, only a decision to
// overwrite/delete or not), so the rollback command's existing warn+consent gate covers it exactly
// like a present-and-drifted non-veto entry already does.
func TestPrepareRestore_NoPriorStateSharedRegistryIsSkipped(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir(), ClickStateHome: t.TempDir()}
	writeTestFile(t, cfg.ClaudeMDPath(), managedBlockMD("managed v1"))
	writeTestFile(t, cfg.SettingsPath(), `{"original":true}`)
	if err := SnapshotRun(cfg); err != nil {
		t.Fatalf("SnapshotRun() error = %v", err)
	}

	// A registry file that did NOT exist when the snapshot was taken.
	registryPath := filepath.Join(cfg.ClaudeHome, "plugins", "known_marketplaces.json")
	manifest, err := loadSnapshotManifest(cfg)
	if err != nil {
		t.Fatalf("loadSnapshotManifest() error = %v", err)
	}
	manifest.Entries = append(manifest.Entries, manifestEntry{
		OriginalPath: registryPath,
		BackupFile:   "",
		Existed:      false,
		DriftPolicy:  DriftPolicyNonVeto,
	})
	writeManifestDirect(t, cfg, manifest)

	// AFTER the snapshot, the external `claude` CLI populates it with real data.
	writeTestFile(t, registryPath, `{"marketplaces":{"user-owned":"important real data"}}`)

	prepared, err := PrepareRestore(cfg)
	if err != nil {
		t.Fatalf("PrepareRestore() error = %v, want nil", err)
	}
	if len(prepared.Drift.Vetoes) != 0 {
		t.Fatalf("PrepareRestore().Drift.Vetoes = %v, want empty (a no-prior-state entry never vetoes)", prepared.Drift.Vetoes)
	}
	found := false
	for _, path := range prepared.Drift.Skipped {
		if path == registryPath {
			found = true
		}
	}
	if !found {
		t.Fatalf("PrepareRestore().Drift.Skipped = %v, want shared registry %q preserved", prepared.Drift.Skipped, registryPath)
	}

	// Confirm nothing was written by PrepareRestore itself.
	after, readErr := os.ReadFile(registryPath)
	if readErr != nil {
		t.Fatalf("ReadFile(registryPath) error = %v, want file untouched by PrepareRestore", readErr)
	}
	if string(after) != `{"marketplaces":{"user-owned":"important real data"}}` {
		t.Fatalf("registry file content changed during PrepareRestore, want zero writes")
	}
}

// A no-prior-state marker does not prove that a later file is Click-owned. Only a matching
// recorded post-run hash may authorize deletion of a currently present private artifact.
func TestApplyPreparedRestore_NoPriorPrivateFilesRequirePostRunProof(t *testing.T) {
	for _, target := range []struct {
		name string
		path func(Config) string
	}{
		{name: "models", path: func(cfg Config) string { return cfg.ModelsPath() }},
		{name: "target selection", path: func(cfg Config) string { return cfg.TargetSelectionPath() }},
	} {
		for _, proof := range []string{"missing", "mismatched", "matching"} {
			t.Run(target.name+"/"+proof, func(t *testing.T) {
				cfg := Config{ClaudeHome: t.TempDir(), ClickStateHome: t.TempDir()}
				path := target.path(cfg)
				current := `{"user":"current"}`
				writeTestFile(t, path, current)
				entry := manifestEntry{OriginalPath: path, Existed: false, DriftPolicy: DriftPolicyWholeFileVeto}
				switch proof {
				case "mismatched":
					entry.ExpectedPostRunHash = CanonicalContentHash(`{"user":"other"}`)
				case "matching":
					entry.ExpectedPostRunHash = CanonicalContentHash(current)
				}
				writeManifestDirect(t, cfg, runManifest{Entries: []manifestEntry{entry}})

				prepared, err := PrepareRestore(cfg)
				if err != nil {
					t.Fatalf("PrepareRestore() error = %v", err)
				}
				if err := ApplyPreparedRestore(prepared); err != nil {
					t.Fatalf("ApplyPreparedRestore() error = %v", err)
				}
				_, statErr := os.Lstat(path)
				if proof == "matching" {
					if !os.IsNotExist(statErr) {
						t.Fatalf("file remains after proven no-prior rollback, Lstat error = %v", statErr)
					}
					return
				}
				if statErr != nil {
					t.Fatalf("file Lstat error = %v, want unproven current file preserved", statErr)
				}
				if got := string(mustReadFile(t, path)); got != current {
					t.Fatalf("unproven current file = %q, want unchanged %q", got, current)
				}
				skipped := false
				for _, skippedPath := range prepared.Drift.Skipped {
					skipped = skipped || skippedPath == path
				}
				if !skipped {
					t.Fatalf("Skipped = %v, want unproven current private file %q", prepared.Drift.Skipped, path)
				}
			})
		}
	}
}
