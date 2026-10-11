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
)

// writeRollbackTestFile is rollback_test.go's own small file-seeding helper (mirrors the
// installer package's own writeTestFile, duplicated here rather than exported across packages for
// one small helper).
func writeRollbackTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", path, err)
	}
}

// rollbackManagedBlockMD builds a CLAUDE.md-shaped file whose click-managed block contains
// managed, with editable developer content around the markers. The marker literals mirror
// installer's unexported managedBeginMarker/managedEndMarker (claudemd.go).
func rollbackManagedBlockMD(managed string) string {
	return "# developer header\n" +
		"# >>> click-ai-devkit (managed) >>>\n" + managed + "\n" +
		"# <<< click-ai-devkit (managed) <<<\n" +
		"developer tail\n"
}

// rollbackManifestEntry mirrors installer's unexported manifestEntry JSON shape so tests can add
// hand-crafted entries (e.g. a non-veto shared file) to a real snapshot's manifest.
type rollbackManifestEntry struct {
	OriginalPath        string `json:"originalPath"`
	BackupFile          string `json:"backupFile"`
	Existed             bool   `json:"existed"`
	ExpectedPostRunHash string `json:"expectedPostRunHash,omitempty"`
	DriftPolicy         string `json:"driftPolicy,omitempty"`
}

// appendRollbackManifestEntry adds one entry to the real manifest.json of cfg's latest snapshot
// and writes its backup file — the honest way to give rollback's ClaudeHome-only cfg a non-veto
// entry (its real snapshotSources only ever produces managed-content-veto entries for Claude).
func appendRollbackManifestEntry(t *testing.T, cfg installer.Config, entry rollbackManifestEntry, backupContent string) {
	t.Helper()
	latestDir := filepath.Join(cfg.BackupDir(), "latest")
	manifestPath := filepath.Join(latestDir, "manifest.json")
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("ReadFile(manifest.json) error = %v", err)
	}
	var doc struct {
		Entries []rollbackManifestEntry `json:"entries"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("json.Unmarshal(manifest.json) error = %v", err)
	}
	doc.Entries = append(doc.Entries, entry)
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatalf("json.MarshalIndent(manifest) error = %v", err)
	}
	if err := os.WriteFile(manifestPath, append(data, '\n'), 0o600); err != nil {
		t.Fatalf("WriteFile(manifest.json) error = %v", err)
	}
	if entry.Existed && entry.BackupFile != "" {
		if err := os.WriteFile(filepath.Join(latestDir, entry.BackupFile), []byte(backupContent), 0o600); err != nil {
			t.Fatalf("WriteFile(backup %s) error = %v", entry.BackupFile, err)
		}
	}
}

// execRollbackWithStdin runs the real `rollback` command through the real root cobra tree with an
// explicit stdin — the exact shape the standing menu's dispatch produces ([]string{"rollback"}
// with the caller's streams), unlike execRoot which always pins stdin to an empty buffer.
// stateHome must be the same one the test seeded its snapshot under: BackupDir() is rooted at
// ClickStateHome, so a throwaway one here would hide the snapshot from the command.
func execRollbackWithStdin(t *testing.T, claudeHome, stateHome, stdin string, args ...string) (string, error) {
	t.Helper()
	t.Setenv("CLICK_CLAUDE_HOME", claudeHome)
	t.Setenv("CLICK_STATE_HOME", stateHome)
	t.Setenv("CLICK_OPENCLAW_HOME", t.TempDir())
	t.Setenv("CODEX_HOME", t.TempDir())

	root := NewRootCommand()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetIn(strings.NewReader(stdin))
	root.SetArgs(args)
	err := root.Execute()
	return buf.String(), err
}

// seedSharedNativeConfig adds a current native config to a snapshot and confirms rollback must
// preserve it rather than replay its historical bytes.
func seedSharedNativeConfig(t *testing.T, cfg installer.Config) string {
	t.Helper()
	sharedPath := cfg.Context7ConfigPath()
	writeRollbackTestFile(t, cfg.ClaudeMDPath(), rollbackManagedBlockMD("managed v1"))
	writeRollbackTestFile(t, cfg.SettingsPath(), `{"original":true}`)
	writeRollbackTestFile(t, sharedPath, `{"mcpServers":{"foreign":{"command":"before"}}}`)
	if err := installer.SnapshotRun(cfg); err != nil {
		t.Fatalf("installer.SnapshotRun() error = %v", err)
	}
	appendRollbackManifestEntry(t, cfg, rollbackManifestEntry{
		OriginalPath: sharedPath,
		BackupFile:   "shared.json",
		Existed:      true,
		DriftPolicy:  "non-veto",
	}, `{"mcpServers":{"foreign":{"command":"backup"}}}`)
	writeRollbackTestFile(t, sharedPath, `{"mcpServers":{"foreign":{"command":"current"}}}`)
	return sharedPath
}

// TestRollback_NoSnapshotEver_ReportsNothingToRestoreNoError guards the spec install-rollback "No
// snapshot exists" scenario when SnapshotRun has never even run: rollback must not error, must not
// fabricate content, and must report cleanly.
func TestRollback_NoSnapshotEver_ReportsNothingToRestoreNoError(t *testing.T) {
	home := t.TempDir()

	// No cfg and no seeded snapshot here on purpose: this is the "SnapshotRun never ran" case, so
	// execRoot's own throwaway state home is exactly right.
	out, err := execRoot(t, home, "rollback")
	if err != nil {
		t.Fatalf("rollback error = %v, want nil when no snapshot ever ran, output:\n%s", err, out)
	}
	if !strings.Contains(out, "No hay ningún respaldo") {
		t.Fatalf("rollback output = %q, want the nothing-to-restore informational message", out)
	}
}

// TestRollback_SnapshotAllNoPriorState_ReportsNothingToRestoreNoError guards the finer half of the
// same spec scenario: a real snapshot ran, but both files were absent at snapshot time (pure
// no-prior-state markers) -> still nothing to restore.
func TestRollback_SnapshotAllNoPriorState_ReportsNothingToRestoreNoError(t *testing.T) {
	home := t.TempDir()
	stateHome := t.TempDir()
	cfg := installer.Config{ClaudeHome: home, ClickStateHome: stateHome}
	if err := installer.SnapshotRun(cfg); err != nil {
		t.Fatalf("installer.SnapshotRun() error = %v", err)
	}

	out, err := execRootWithStateHome(t, home, stateHome, "rollback")
	if err != nil {
		t.Fatalf("rollback error = %v, want nil, output:\n%s", err, out)
	}
	if !strings.Contains(out, "No hay ningún respaldo") {
		t.Fatalf("rollback output = %q, want the nothing-to-restore informational message", out)
	}
}

// TestRollback_MatchingHash_NoForce_RestoresBothFilesAndSnapshotSurvives guards the spec
// "Successful restore" scenario: no drift since the snapshot means rollback proceeds even without
// --force, restores both files, and the snapshot itself survives (not consumed). With nothing
// vetoing and nothing warnable, no consent is needed even on a non-TTY.
func TestRollback_MatchingHash_NoForce_RestoresBothFilesAndSnapshotSurvives(t *testing.T) {
	home := t.TempDir()
	stateHome := t.TempDir()
	cfg := installer.Config{ClaudeHome: home, ClickStateHome: stateHome}
	writeRollbackTestFile(t, cfg.ClaudeMDPath(), "snapshot content\n")
	writeRollbackTestFile(t, cfg.SettingsPath(), `{"snapshot":true}`)
	if err := installer.SnapshotRun(cfg); err != nil {
		t.Fatalf("installer.SnapshotRun() error = %v", err)
	}

	out, err := execRootWithStateHome(t, home, stateHome, "rollback")
	if err != nil {
		t.Fatalf("rollback error = %v, want nil, output:\n%s", err, out)
	}
	if !strings.Contains(out, "Archivos gestionados por click restaurados desde el último respaldo.") {
		t.Fatalf("rollback output = %q, want the managed-files restore success message", out)
	}

	gotClaude, err := os.ReadFile(cfg.ClaudeMDPath())
	if err != nil {
		t.Fatalf("read CLAUDE.md after rollback: %v", err)
	}
	if string(gotClaude) != "snapshot content\n" {
		t.Fatalf("CLAUDE.md after rollback = %q, want %q", gotClaude, "snapshot content\n")
	}

	has, err := installer.HasRunSnapshot(cfg)
	if err != nil {
		t.Fatalf("installer.HasRunSnapshot() error = %v", err)
	}
	if !has {
		t.Fatal("HasRunSnapshot() = false after rollback, want the snapshot to survive (not consumed)")
	}
}

// TestRollback_DriftNoForce_RefusesAndMakesZeroWrites guards spec install-rollback Decision 3
// (refuse-by-default), ownership-scoped: a hand-edit INSIDE click's managed block since the
// snapshot must produce an error, report the drifted path, and leave BOTH files exactly as they
// currently are (zero writes).
func TestRollback_DriftNoForce_RefusesAndMakesZeroWrites(t *testing.T) {
	home := t.TempDir()
	stateHome := t.TempDir()
	cfg := installer.Config{ClaudeHome: home, ClickStateHome: stateHome}
	writeRollbackTestFile(t, cfg.ClaudeMDPath(), rollbackManagedBlockMD("managed v1"))
	writeRollbackTestFile(t, cfg.SettingsPath(), `{"original":true}`)
	if err := installer.SnapshotRun(cfg); err != nil {
		t.Fatalf("installer.SnapshotRun() error = %v", err)
	}
	writeRollbackTestFile(t, cfg.ClaudeMDPath(), rollbackManagedBlockMD("hand-edited managed block"))
	unknownPath := filepath.Join(home, "custom-history.json")
	writeRollbackTestFile(t, unknownPath, `{"value":"snapshot"}`)
	appendRollbackManifestEntry(t, cfg, rollbackManifestEntry{
		OriginalPath: unknownPath,
		BackupFile:   "custom-history.json",
		Existed:      true,
		DriftPolicy:  "whole-file-veto",
	}, `{"value":"backup"}`)
	writeRollbackTestFile(t, unknownPath, `{"value":"current"}`)

	out, err := execRootWithStateHome(t, home, stateHome, "rollback")
	if err == nil {
		t.Fatalf("rollback error = nil, want a refusal error when click-owned content drifted, output:\n%s", out)
	}
	if !strings.Contains(out, "Los siguientes archivos se editaron manualmente desde el respaldo; rollback rechazado:") {
		t.Fatalf("rollback output = %q, want the existing veto-refusal message shape", out)
	}
	if !strings.Contains(out, cfg.ClaudeMDPath()) {
		t.Fatalf("rollback output = %q, want it to name the drifted file %s", out, cfg.ClaudeMDPath())
	}
	if !strings.Contains(out, "click rollback --force") {
		t.Fatalf("rollback output = %q, want the --force guidance", out)
	}
	if !strings.Contains(out, "Se conservaron") || !strings.Contains(out, unknownPath) {
		t.Fatalf("rollback output = %q, want skipped unknown path reported despite veto", out)
	}

	gotClaude, err := os.ReadFile(cfg.ClaudeMDPath())
	if err != nil {
		t.Fatalf("read CLAUDE.md after refused rollback: %v", err)
	}
	if string(gotClaude) != rollbackManagedBlockMD("hand-edited managed block") {
		t.Fatalf("CLAUDE.md after refused rollback = %q, want untouched hand-edited content", gotClaude)
	}
	gotSettings, err := os.ReadFile(cfg.SettingsPath())
	if err != nil {
		t.Fatalf("read settings.json after refused rollback: %v", err)
	}
	if string(gotSettings) != `{"original":true}` {
		t.Fatalf("settings.json after refused rollback = %q, want untouched %q", gotSettings, `{"original":true}`)
	}
	if gotUnknown := string(mustReadRollbackFile(t, unknownPath)); gotUnknown != `{"value":"current"}` {
		t.Fatalf("unknown path after refused rollback = %q, want unchanged", gotUnknown)
	}
}

// TestRollback_DriftWithForce_Proceeds guards the explicit override: the same managed-block drift
// as above, but with --force, must proceed and restore the snapshotted content (no warnable
// entries pending, so no consent is required).
func TestRollback_DriftWithForce_Proceeds(t *testing.T) {
	home := t.TempDir()
	stateHome := t.TempDir()
	cfg := installer.Config{ClaudeHome: home, ClickStateHome: stateHome}
	writeRollbackTestFile(t, cfg.ClaudeMDPath(), rollbackManagedBlockMD("managed v1"))
	writeRollbackTestFile(t, cfg.SettingsPath(), `{"original":true}`)
	if err := installer.SnapshotRun(cfg); err != nil {
		t.Fatalf("installer.SnapshotRun() error = %v", err)
	}
	writeRollbackTestFile(t, cfg.ClaudeMDPath(), rollbackManagedBlockMD("hand-edited managed block"))

	out, err := execRootWithStateHome(t, home, stateHome, "rollback", "--force")
	if err != nil {
		t.Fatalf("rollback --force error = %v, want nil, output:\n%s", err, out)
	}

	gotClaude, err := os.ReadFile(cfg.ClaudeMDPath())
	if err != nil {
		t.Fatalf("read CLAUDE.md after forced rollback: %v", err)
	}
	if string(gotClaude) != rollbackManagedBlockMD("managed v1") {
		t.Fatalf("CLAUDE.md after forced rollback = %q, want the restored snapshot content", gotClaude)
	}
}

// TestRollback_MenuDispatchShape_PreservesSharedNativeConfig proves the live menu row reports and
// preserves a shared native config without a consent prompt.
func TestRollback_MenuDispatchShape_PreservesSharedNativeConfig(t *testing.T) {
	forceTerminalDetection(t, true, true)
	home := t.TempDir()
	stateHome := t.TempDir()
	cfg := installer.Config{ClaudeHome: home, ClickStateHome: stateHome}
	sharedPath := seedSharedNativeConfig(t, cfg)

	out, err := execRollbackWithStdin(t, home, stateHome, "y\n", "rollback")
	if err != nil {
		t.Fatalf("rollback error = %v, want nil with shared native config preserved, output:\n%s", err, out)
	}
	if !strings.Contains(out, "Se conservaron") || !strings.Contains(out, sharedPath) {
		t.Fatalf("rollback output = %q, want a preservation notice naming %s", out, sharedPath)
	}
	if !strings.Contains(out, "Archivos gestionados por click restaurados desde el último respaldo.") {
		t.Fatalf("rollback output = %q, want the restore success message", out)
	}
	got, err := os.ReadFile(sharedPath)
	if err != nil {
		t.Fatalf("read shared.txt after rollback: %v", err)
	}
	if string(got) != `{"mcpServers":{"foreign":{"command":"current"}}}` {
		t.Fatalf("shared config after rollback = %q, want current native content preserved", got)
	}
}

// TestRollback_InteractiveInputCannotAuthorizeSharedOverwrite verifies that neither an affirmative
// nor a negative stdin answer can cause a shared native configuration to be rewritten.
func TestRollback_InteractiveInputCannotAuthorizeSharedOverwrite(t *testing.T) {
	for _, tc := range []struct {
		name  string
		stdin string
	}{
		{name: "explicit n", stdin: "n\n"},
		{name: "immediate EOF", stdin: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			forceTerminalDetection(t, true, true)
			home := t.TempDir()
			stateHome := t.TempDir()
			cfg := installer.Config{ClaudeHome: home, ClickStateHome: stateHome}
			sharedPath := seedSharedNativeConfig(t, cfg)

			out, err := execRollbackWithStdin(t, home, stateHome, tc.stdin, "rollback")
			if err != nil {
				t.Fatalf("rollback error = %v, want nil (declining is a clean outcome), output:\n%s", err, out)
			}
			if !strings.Contains(out, "Se conservaron") {
				t.Fatalf("rollback output = %q, want a shared-state preservation notice", out)
			}
			got, err := os.ReadFile(sharedPath)
			if err != nil {
				t.Fatalf("read shared.txt after declined rollback: %v", err)
			}
			if string(got) != `{"mcpServers":{"foreign":{"command":"current"}}}` {
				t.Fatalf("shared config after rollback = %q, want current native content preserved", got)
			}
			gotClaude, err := os.ReadFile(cfg.ClaudeMDPath())
			if err != nil {
				t.Fatalf("read CLAUDE.md after declined rollback: %v", err)
			}
			if string(gotClaude) != rollbackManagedBlockMD("managed v1") {
				t.Fatalf("CLAUDE.md after rollback = %q, want unchanged managed block", gotClaude)
			}
		})
	}
}

// TestRollback_NonInteractiveSharedConfigDoesNotRequireYes confirms preservation of shared native
// configuration is not gated on consent, even in non-interactive mode.
func TestRollback_NonInteractiveSharedConfigDoesNotRequireYes(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{name: "non-TTY streams", args: []string{"rollback"}},
		{name: "explicit --non-interactive", args: []string{"rollback", "--non-interactive"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			stateHome := t.TempDir()
			cfg := installer.Config{ClaudeHome: home, ClickStateHome: stateHome}
			sharedPath := seedSharedNativeConfig(t, cfg)

			out, err := execRootWithStateHome(t, home, stateHome, tc.args...)
			if err != nil {
				t.Fatalf("rollback error = %v, want shared config preserved without consent, output:\n%s", err, out)
			}
			if !strings.Contains(out, "Se conservaron") || strings.Contains(out, "--yes") {
				t.Fatalf("rollback output = %q, want preservation notice without a --yes prompt", out)
			}
			got, err := os.ReadFile(sharedPath)
			if err != nil {
				t.Fatalf("read shared.txt after refused rollback: %v", err)
			}
			if string(got) != `{"mcpServers":{"foreign":{"command":"current"}}}` {
				t.Fatalf("shared config after rollback = %q, want current native content preserved", got)
			}
		})
	}
}

// TestRollback_YesDoesNotOverwriteSharedConfig verifies --yes cannot broaden Click ownership.
func TestRollback_YesDoesNotOverwriteSharedConfig(t *testing.T) {
	home := t.TempDir()
	stateHome := t.TempDir()
	cfg := installer.Config{ClaudeHome: home, ClickStateHome: stateHome}
	sharedPath := seedSharedNativeConfig(t, cfg)

	out, err := execRootWithStateHome(t, home, stateHome, "rollback", "--yes")
	if err != nil {
		t.Fatalf("rollback --yes error = %v, want nil, output:\n%s", err, out)
	}
	if strings.Contains(out, "¿Continuar?") {
		t.Fatalf("rollback --yes output = %q, want NO confirmation prompt", out)
	}
	got, err := os.ReadFile(sharedPath)
	if err != nil {
		t.Fatalf("read shared.txt after rollback --yes: %v", err)
	}
	if string(got) != `{"mcpServers":{"foreign":{"command":"current"}}}` {
		t.Fatalf("shared config after rollback --yes = %q, want current native content preserved", got)
	}
}

// TestRollback_UnknownHistoricalPathsArePreserved reports and preserves paths that are not
// currently resolved by Config, regardless of their historical drift policy or hashes.
func TestRollback_UnknownHistoricalPathsArePreserved(t *testing.T) {
	home := t.TempDir()
	stateHome := t.TempDir()
	cfg := installer.Config{ClaudeHome: home, ClickStateHome: stateHome}
	uneditedPath := filepath.Join(home, "unedited.txt")
	postRunPath := filepath.Join(home, "postrun.txt")
	writeRollbackTestFile(t, cfg.ClaudeMDPath(), rollbackManagedBlockMD("managed v1"))
	writeRollbackTestFile(t, cfg.SettingsPath(), `{"original":true}`)
	writeRollbackTestFile(t, uneditedPath, "same as backup\n")
	writeRollbackTestFile(t, postRunPath, "post-run state\n")
	if err := installer.SnapshotRun(cfg); err != nil {
		t.Fatalf("installer.SnapshotRun() error = %v", err)
	}
	appendRollbackManifestEntry(t, cfg, rollbackManifestEntry{
		OriginalPath: uneditedPath,
		BackupFile:   "unedited.txt",
		Existed:      true,
		DriftPolicy:  "non-veto",
	}, "same as backup\n")
	appendRollbackManifestEntry(t, cfg, rollbackManifestEntry{
		OriginalPath:        postRunPath,
		BackupFile:          "postrun.txt",
		Existed:             true,
		ExpectedPostRunHash: installer.CanonicalContentHash("post-run state\n"),
		DriftPolicy:         "non-veto",
	}, "pre-run state\n")

	out, err := execRootWithStateHome(t, home, stateHome, "rollback")
	if err != nil {
		t.Fatalf("rollback error = %v, want nil (nothing requires consent), output:\n%s", err, out)
	}
	if !strings.Contains(out, "Se conservaron") || !strings.Contains(out, uneditedPath) || !strings.Contains(out, postRunPath) {
		t.Fatalf("rollback output = %q, want both unknown paths explicitly reported as preserved", out)
	}
	if !strings.Contains(out, "Archivos gestionados por click restaurados desde el último respaldo.") {
		t.Fatalf("rollback output = %q, want the restore success message", out)
	}
	for path, want := range map[string]string{
		uneditedPath: "same as backup\n",
		postRunPath:  "post-run state\n",
	} {
		got, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatalf("read preserved unknown path %s: %v", path, readErr)
		}
		if string(got) != want {
			t.Errorf("unknown path %s = %q, want unchanged %q", path, got, want)
		}
	}
}

// TestRollback_ForceNeverOverwritesSharedConfig verifies --force only applies to proven Click-owned
// drift and never turns shared-state preservation into overwrite permission.
func TestRollback_ForceNeverOverwritesSharedConfig(t *testing.T) {
	home := t.TempDir()
	stateHome := t.TempDir()
	cfg := installer.Config{ClaudeHome: home, ClickStateHome: stateHome}
	sharedPath := seedSharedNativeConfig(t, cfg)
	writeRollbackTestFile(t, cfg.ClaudeMDPath(), rollbackManagedBlockMD("hand-edited managed block"))

	out, err := execRootWithStateHome(t, home, stateHome, "rollback", "--force")
	if err != nil {
		t.Fatalf("rollback --force error = %v, want managed drift forced and shared config skipped, output:\n%s", err, out)
	}
	if !strings.Contains(out, "Se conservaron") {
		t.Fatalf("rollback --force output = %q, want preservation notice", out)
	}
	got, err := os.ReadFile(sharedPath)
	if err != nil {
		t.Fatalf("read shared.txt after refused rollback: %v", err)
	}
	if string(got) != `{"mcpServers":{"foreign":{"command":"current"}}}` {
		t.Fatalf("shared config after --force rollback = %q, want current native content preserved", got)
	}

	out, err = execRootWithStateHome(t, home, stateHome, "rollback", "--force", "--yes")
	if err != nil {
		t.Fatalf("rollback --force --yes error = %v, want nil, output:\n%s", err, out)
	}
	got, err = os.ReadFile(sharedPath)
	if err != nil {
		t.Fatalf("read shared.txt after --force --yes rollback: %v", err)
	}
	if string(got) != `{"mcpServers":{"foreign":{"command":"current"}}}` {
		t.Fatalf("shared config after --force --yes rollback = %q, want current native content preserved", got)
	}
	gotClaude, err := os.ReadFile(cfg.ClaudeMDPath())
	if err != nil {
		t.Fatalf("read CLAUDE.md after --force --yes rollback: %v", err)
	}
	if string(gotClaude) != rollbackManagedBlockMD("managed v1") {
		t.Fatalf("CLAUDE.md after --force --yes rollback = %q, want the restored snapshot content", gotClaude)
	}
}

func TestRollback_ForceRestoresGuardButPreservesCurrentCloudToken(t *testing.T) {
	home, stateHome := t.TempDir(), t.TempDir()
	cfg := installer.Config{ClaudeHome: home, ClickStateHome: stateHome}
	matcher := installer.MemoryGuardToolMatcher()
	before := `{"env":{"ENGRAM_CLOUD_TOKEN":"old-secret","KEEP":"before"},"hooks":{"PreToolUse":[{"matcher":"` + matcher + `","hooks":[{"type":"command","command":"` + installer.MemoryGuardCommand + `"},{"type":"command","command":"foreign"}]}],"SessionStart":[{"hooks":[{"type":"command","command":"cloud-import"}]}]},"custom":"before"}`
	writeRollbackTestFile(t, cfg.SettingsPath(), before)
	writeRollbackTestFile(t, cfg.ClaudeMDPath(), rollbackManagedBlockMD("stable guidance"))
	if err := installer.SnapshotRun(cfg); err != nil {
		t.Fatal(err)
	}
	current := `{"env":{"ENGRAM_CLOUD_TOKEN":"current-secret","KEEP":"current"},"hooks":{"PreToolUse":[{"matcher":"hand-edited matcher","hooks":[{"type":"command","command":"` + installer.MemoryGuardCommand + `"},{"type":"command","command":"foreign"}]}],"SessionStart":[{"hooks":[{"type":"command","command":"cloud-import-current"}]}]},"custom":"current"}`
	writeRollbackTestFile(t, cfg.SettingsPath(), current)
	out, err := execRollbackWithStdin(t, home, stateHome, "", "rollback", "--force")
	if err != nil {
		t.Fatalf("rollback --force error = %v, output:\n%s", err, out)
	}
	after, err := os.ReadFile(cfg.SettingsPath())
	if err != nil {
		t.Fatal(err)
	}
	var settings map[string]any
	if err := json.Unmarshal(after, &settings); err != nil {
		t.Fatal(err)
	}
	env := settings["env"].(map[string]any)
	if env["ENGRAM_CLOUD_TOKEN"] != "current-secret" || env["KEEP"] != "current" || settings["custom"] != "current" {
		t.Fatalf("rollback replaced current Cloud or foreign settings: %s", after)
	}
	preTool := settings["hooks"].(map[string]any)["PreToolUse"].([]any)
	guardRestored, foreignPreserved := false, false
	for _, raw := range preTool {
		entry := raw.(map[string]any)
		matcherValue, _ := entry["matcher"].(string)
		for _, rawHook := range entry["hooks"].([]any) {
			command := rawHook.(map[string]any)["command"]
			guardRestored = guardRestored || (command == installer.MemoryGuardCommand && matcherValue == matcher)
			foreignPreserved = foreignPreserved || (command == "foreign" && matcherValue == "hand-edited matcher")
		}
	}
	if !guardRestored || !foreignPreserved {
		t.Fatalf("Click guard was not restored or foreign hook changed: %s", after)
	}
	if strings.Contains(string(after), "[REDACTED]") || strings.Contains(out, "token de Engram Cloud") {
		t.Fatalf("rollback exposed a redacted placeholder or ran legacy token repair: output=%q settings=%s", out, after)
	}
}

func TestRollback_ForceYesPreservesResolvedSharedAndUnknownPaths(t *testing.T) {
	claudeHome, codexHome, openClawHome, stateHome := t.TempDir(), t.TempDir(), t.TempDir(), t.TempDir()
	cfg := installer.Config{ClaudeHome: claudeHome, CodexHome: codexHome, OpenClawHome: openClawHome, ClickStateHome: stateHome}
	writeRollbackTestFile(t, cfg.ClaudeMDPath(), rollbackManagedBlockMD("guidance"))
	writeRollbackTestFile(t, cfg.SettingsPath(), `{"foreign":true}`)
	shared := []string{cfg.Context7ConfigPath(), cfg.KnownMarketplacesPath(), cfg.InstalledPluginsPath(), cfg.CodexConfigPath(), cfg.OpenClawMCPConfigPath()}
	for i, path := range shared {
		writeRollbackTestFile(t, path, fmt.Sprintf(`{"before":%d}`, i))
	}
	if err := installer.SnapshotRun(cfg); err != nil {
		t.Fatal(err)
	}
	for i, path := range shared {
		writeRollbackTestFile(t, path, fmt.Sprintf(`{"current":%d}`, i))
	}
	unknown := filepath.Join(claudeHome, "custom-history.json")
	writeRollbackTestFile(t, unknown, `{"current":"preserve"}`)
	for i, path := range append(append([]string(nil), shared[:3]...), unknown) {
		appendRollbackManifestEntry(t, cfg, rollbackManifestEntry{
			OriginalPath: path, BackupFile: fmt.Sprintf("custom-%d.json", i), Existed: true, DriftPolicy: "whole-file-veto",
		}, fmt.Sprintf(`{"backup":%d}`, i))
	}

	processHome := t.TempDir()
	t.Setenv("HOME", processHome)
	t.Setenv("USERPROFILE", processHome)
	t.Setenv("CLICK_CLAUDE_HOME", claudeHome)
	t.Setenv("CODEX_HOME", codexHome)
	t.Setenv("CLICK_OPENCLAW_HOME", openClawHome)
	t.Setenv("CLICK_STATE_HOME", stateHome)
	root := NewRootCommand()
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetErr(&output)
	root.SetArgs([]string{"rollback", "--force", "--yes"})
	if err := root.Execute(); err != nil {
		t.Fatalf("rollback --force --yes error = %v, output:\n%s", err, output.String())
	}
	for i, path := range shared {
		if got, err := os.ReadFile(path); err != nil || string(got) != fmt.Sprintf(`{"current":%d}`, i) {
			t.Errorf("shared config %s = %q, %v, want current value retained", path, got, err)
		}
		if !strings.Contains(output.String(), path) {
			t.Errorf("rollback output did not report preserved shared path %s", path)
		}
	}
	if got := string(mustReadRollbackFile(t, unknown)); got != `{"current":"preserve"}` {
		t.Fatalf("unknown historical path = %q, want unchanged", got)
	}
	if !strings.Contains(output.String(), unknown) {
		t.Fatalf("rollback output did not report unknown historical path: %s", output.String())
	}
}

func TestRollback_ForceYesPreservesUnprovenNoPriorPrivateArtifacts(t *testing.T) {
	home, stateHome := t.TempDir(), t.TempDir()
	cfg := installer.Config{ClaudeHome: home, ClickStateHome: stateHome}
	writeRollbackTestFile(t, cfg.ClaudeMDPath(), rollbackManagedBlockMD("guidance"))
	writeRollbackTestFile(t, cfg.SettingsPath(), `{"foreign":true}`)
	if err := installer.SnapshotRun(cfg); err != nil {
		t.Fatal(err)
	}
	paths := []string{cfg.ModelsPath(), cfg.TargetSelectionPath()}
	contents := []string{`{"profile":"user"}`, `{"configured":true,"targets":["user"]}`}
	for i, path := range paths {
		writeRollbackTestFile(t, path, contents[i])
		appendRollbackManifestEntry(t, cfg, rollbackManifestEntry{
			OriginalPath: path,
			Existed:      false,
			DriftPolicy:  "whole-file-veto",
		}, "")
	}

	out, err := execRootWithStateHome(t, home, stateHome, "rollback", "--force", "--yes")
	if err != nil {
		t.Fatalf("rollback --force --yes error = %v, output:\n%s", err, out)
	}
	for i, path := range paths {
		if got := string(mustReadRollbackFile(t, path)); got != contents[i] {
			t.Errorf("unproven no-prior artifact %s = %q, want unchanged %q", path, got, contents[i])
		}
		if !strings.Contains(out, path) {
			t.Errorf("rollback output did not report preserved artifact %s: %s", path, out)
		}
	}
}

func mustReadRollbackFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	return data
}

// TestRollback_HiddenFromHelp guards that `click rollback` follows manage-backups/
// configure-models' precedent: reachable directly but hidden from `click --help`.
func TestRollback_HiddenFromHelp(t *testing.T) {
	home := t.TempDir()
	cmd := newRollbackCommand()
	if cmd.Short != "Restaurar los archivos gestionados por click desde el último respaldo de instalación/actualización" {
		t.Fatalf("rollback Short = %q, want the managed-files description", cmd.Short)
	}
	if !strings.Contains(cmd.Long, "Conserva las configuraciones nativas compartidas") {
		t.Fatalf("rollback Long = %q, want explicit shared-config preservation boundary", cmd.Long)
	}

	out, err := execRoot(t, home, "--help")
	if err != nil {
		t.Fatalf("--help error = %v", err)
	}
	if strings.Contains(out, "rollback") {
		t.Fatalf("--help output = %q, want `rollback` hidden from help text", out)
	}
}
