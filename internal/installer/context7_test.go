package installer

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

// TestSyncContext7_InstallsWhenNotPresent covers the common case: a developer who has never
// touched Context7 runs `click install`. SyncContext7 must issue the exact
// `claude mcp add --transport http --scope user context7 <url>` command and record click-owned
// state, mirroring SyncEngram's own contract (engram.go).
func TestSyncContext7_InstallsWhenNotPresent(t *testing.T) {
	claudeHome := t.TempDir()
	cfg := Config{ClaudeHome: claudeHome}
	runner := newFakeCommandRunner(cfg)
	restoreRunner := SetCommandRunnerFactoryForTests(func() CommandRunner { return runner })
	defer restoreRunner()

	alreadyPresent, err := SyncContext7(cfg)
	if err != nil {
		t.Fatalf("SyncContext7() error = %v", err)
	}
	if alreadyPresent {
		t.Fatal("SyncContext7() alreadyPresent = true on a fresh home, want false")
	}

	want := []commandInvocation{
		{Name: "claude", Args: []string{"mcp", "add", "--transport", "http", "--scope", "user", "context7", context7ServerURL}},
	}
	if !reflect.DeepEqual(runner.commands, want) {
		t.Fatalf("runner.commands = %#v, want %#v", runner.commands, want)
	}

	present, err := HasContext7(cfg)
	if err != nil {
		t.Fatalf("HasContext7() error = %v", err)
	}
	if !present {
		t.Fatal("SyncContext7() did not actually register context7 (HasContext7 = false afterward)")
	}

	stateData, err := os.ReadFile(cfg.Context7StatePath())
	if err != nil {
		t.Fatalf("ReadFile(Context7StatePath) error = %v", err)
	}
	var state context7State
	if err := json.Unmarshal(stateData, &state); err != nil {
		t.Fatalf("json.Unmarshal(state) error = %v", err)
	}
	if !state.InstalledByClick {
		t.Fatal("state.InstalledByClick = false after a fresh SyncContext7(), want true")
	}
}

// TestSyncContext7_SkipsWhenAlreadyPresent is the "respect an existing developer setup" contract:
// if a developer already ran `claude mcp add ... context7` themselves, click must never re-add or
// clobber it — just detect it and record that click did NOT own this install.
func TestSyncContext7_SkipsWhenAlreadyPresent(t *testing.T) {
	claudeHome := t.TempDir()
	cfg := Config{ClaudeHome: claudeHome}
	runner := newFakeCommandRunner(cfg)
	restoreRunner := SetCommandRunnerFactoryForTests(func() CommandRunner { return runner })
	defer restoreRunner()

	seedContext7AlreadyPresent(t, cfg)

	alreadyPresent, err := SyncContext7(cfg)
	if err != nil {
		t.Fatalf("SyncContext7() error = %v", err)
	}
	if !alreadyPresent {
		t.Fatal("SyncContext7() alreadyPresent = false when context7 was pre-seeded, want true")
	}
	if len(runner.commands) != 0 {
		t.Fatalf("SyncContext7() issued commands %#v against an already-present context7, want zero (no reinstall/clobber)", runner.commands)
	}

	stateData, err := os.ReadFile(cfg.Context7StatePath())
	if err != nil {
		t.Fatalf("ReadFile(Context7StatePath) error = %v", err)
	}
	var state context7State
	if err := json.Unmarshal(stateData, &state); err != nil {
		t.Fatalf("json.Unmarshal(state) error = %v", err)
	}
	if state.InstalledByClick {
		t.Fatal("state.InstalledByClick = true for a pre-existing install click never touched, want false")
	}
}

// TestSyncContext7_SecondRunPreservesClickOwnership is the same regression class fixed for Engram
// (TestSyncEngram_SecondRunPreservesClickOwnership): SyncContext7 is called on every `click
// install`/`click update` run, so by the SECOND run context7 is already present — because click
// itself added it. Ownership must be decided once and preserved, not re-derived from
// "!alreadyPresent" every time.
func TestSyncContext7_SecondRunPreservesClickOwnership(t *testing.T) {
	claudeHome := t.TempDir()
	cfg := Config{ClaudeHome: claudeHome}
	runner := newFakeCommandRunner(cfg)
	restoreRunner := SetCommandRunnerFactoryForTests(func() CommandRunner { return runner })
	defer restoreRunner()

	if _, err := SyncContext7(cfg); err != nil {
		t.Fatalf("first SyncContext7() error = %v", err)
	}
	alreadyPresent, err := SyncContext7(cfg)
	if err != nil {
		t.Fatalf("second SyncContext7() error = %v", err)
	}
	if !alreadyPresent {
		t.Fatal("second SyncContext7() alreadyPresent = false, want true (idempotent skip)")
	}

	stateData, err := os.ReadFile(cfg.Context7StatePath())
	if err != nil {
		t.Fatalf("ReadFile(Context7StatePath) error = %v", err)
	}
	var state context7State
	if err := json.Unmarshal(stateData, &state); err != nil {
		t.Fatalf("json.Unmarshal(state) error = %v", err)
	}
	if !state.InstalledByClick {
		t.Fatal("state.InstalledByClick flipped to false after a second, idempotent SyncContext7() run — ownership must be preserved, not re-derived")
	}

	if err := RemoveContext7(cfg); err != nil {
		t.Fatalf("RemoveContext7() error = %v", err)
	}
	present, err := HasContext7(cfg)
	if err != nil {
		t.Fatalf("HasContext7() error = %v", err)
	}
	if present {
		t.Fatal("RemoveContext7() left a click-owned context7 registration in place after two SyncContext7() runs")
	}
}

// TestRemoveContext7_RemovesWhenClickInstalledIt covers the normal uninstall path.
func TestRemoveContext7_RemovesWhenClickInstalledIt(t *testing.T) {
	claudeHome := t.TempDir()
	cfg := Config{ClaudeHome: claudeHome}
	runner := newFakeCommandRunner(cfg)
	restoreRunner := SetCommandRunnerFactoryForTests(func() CommandRunner { return runner })
	defer restoreRunner()

	if _, err := SyncContext7(cfg); err != nil {
		t.Fatalf("SyncContext7() error = %v", err)
	}

	if err := RemoveContext7(cfg); err != nil {
		t.Fatalf("RemoveContext7() error = %v", err)
	}

	present, err := HasContext7(cfg)
	if err != nil {
		t.Fatalf("HasContext7() error = %v", err)
	}
	if present {
		t.Fatal("RemoveContext7() left context7 registered after click owned the install")
	}
	if _, err := os.Stat(cfg.Context7StatePath()); !os.IsNotExist(err) {
		t.Fatalf("RemoveContext7() left the context7 state file behind (err = %v)", err)
	}
}

// TestRemoveContext7_RespectsPreExistingInstall is the flip side: if context7 was already present
// before click touched this machine, `click uninstall` must NOT remove it.
func TestRemoveContext7_RespectsPreExistingInstall(t *testing.T) {
	claudeHome := t.TempDir()
	cfg := Config{ClaudeHome: claudeHome}
	runner := newFakeCommandRunner(cfg)
	restoreRunner := SetCommandRunnerFactoryForTests(func() CommandRunner { return runner })
	defer restoreRunner()

	seedContext7AlreadyPresent(t, cfg)

	if _, err := SyncContext7(cfg); err != nil {
		t.Fatalf("SyncContext7() error = %v", err)
	}

	if err := RemoveContext7(cfg); err != nil {
		t.Fatalf("RemoveContext7() error = %v", err)
	}

	if len(runner.commands) != 0 {
		t.Fatalf("RemoveContext7() issued commands %#v against a pre-existing install, want zero", runner.commands)
	}
	present, err := HasContext7(cfg)
	if err != nil {
		t.Fatalf("HasContext7() error = %v", err)
	}
	if !present {
		t.Fatal("RemoveContext7() removed a pre-existing context7 registration it never owned")
	}
	if _, err := os.Stat(cfg.Context7StatePath()); !os.IsNotExist(err) {
		t.Fatalf("RemoveContext7() left click's own state file behind after respecting a pre-existing install (err = %v)", err)
	}
}

// TestRemoveContext7_NoopWhenNeverSynced covers a `click uninstall` run against a home where
// `click install` never ran: nothing to reverse, no error.
func TestRemoveContext7_NoopWhenNeverSynced(t *testing.T) {
	claudeHome := t.TempDir()
	cfg := Config{ClaudeHome: claudeHome}
	runner := newFakeCommandRunner(cfg)
	restoreRunner := SetCommandRunnerFactoryForTests(func() CommandRunner { return runner })
	defer restoreRunner()

	if err := RemoveContext7(cfg); err != nil {
		t.Fatalf("RemoveContext7() on a never-synced home error = %v, want nil", err)
	}
	if len(runner.commands) != 0 {
		t.Fatalf("RemoveContext7() issued commands %#v on a never-synced home, want zero", runner.commands)
	}
}

// TestHasContext7_MissingConfigFile covers the "never configured anything" case: no
// .claude.json at all yet, which must read as "not present", not an error.
func TestHasContext7_MissingConfigFile(t *testing.T) {
	cfg := Config{ClaudeHome: t.TempDir()}

	present, err := HasContext7(cfg)
	if err != nil {
		t.Fatalf("HasContext7() error = %v, want nil for a missing .claude.json", err)
	}
	if present {
		t.Fatal("HasContext7() = true when .claude.json does not exist yet")
	}
}

func seedContext7AlreadyPresent(t *testing.T, cfg Config) {
	t.Helper()
	data := map[string]any{
		"mcpServers": map[string]any{
			"context7": map[string]any{"type": "http", "url": context7ServerURL},
		},
	}
	if err := writeJSONFile(cfg.Context7ConfigPath(), data); err != nil {
		t.Fatalf("writeJSONFile(Context7ConfigPath) error = %v", err)
	}
}
