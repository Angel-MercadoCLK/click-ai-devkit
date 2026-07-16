package doctor

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/installer"
	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/manifest"
)

// fakeGitLookup fakes PATH resolution for checkGit's installer.GitPath/GitAvailable dependency,
// mirroring the same injectable BinaryLookup pattern used for the Engram binary lookup
// (internal/installer/engram_test.go's fakeBinaryLookup) — so these doctor tests never depend on
// whether the real test machine actually has git installed.
type fakeGitLookup struct {
	resolved map[string]string
}

func (f fakeGitLookup) LookPath(name string) (string, error) {
	if path, ok := f.resolved[name]; ok {
		return path, nil
	}
	return "", errors.New("fakeGitLookup: not found: " + name)
}

// seedResolvableGit makes installer.GitAvailable/GitPath AND installer.ClaudeAvailable/ClaudePath
// see git and claude as resolvable on a fake PATH, so doctor tests that assert overall
// Report.Healthy() are deterministic regardless of the real test machine's PATH. Both binaries are
// seeded because Run() now includes both checkGit and checkClaude; leaving claude out would make
// TestRun_AfterInstall_ReportsHealthy flake on the claude check. Returns the restore func so
// callers can defer it.
func seedResolvableGit(t *testing.T) func() {
	t.Helper()
	return installer.SetBinaryLookupFactoryForTests(func() installer.BinaryLookup {
		return fakeGitLookup{resolved: map[string]string{"git": "/usr/bin/git", "claude": "/usr/bin/claude"}}
	})
}

func TestRun_BeforeInstall_ReportsUnhealthy(t *testing.T) {
	cfg := installer.Config{ClaudeHome: t.TempDir()}

	report := Run(cfg)

	if report.Healthy() {
		t.Fatal("Run() on a fresh, never-installed ClaudeHome reports healthy, want unhealthy")
	}
	if len(report.Checks) == 0 {
		t.Fatal("Run() returned zero checks")
	}
}

func TestRun_AfterInstall_ReportsHealthy(t *testing.T) {
	cfg := installer.Config{ClaudeHome: t.TempDir()}

	restoreGit := seedResolvableGit(t)
	defer restoreGit()
	seedInstalledState(t, cfg)
	if err := installer.WriteManagedBlock(cfg.ClaudeMDPath(), installer.DefaultManagedContent); err != nil {
		t.Fatalf("WriteManagedBlock() error = %v", err)
	}
	if err := installer.RegisterMemoryGuardHook(cfg); err != nil {
		t.Fatalf("RegisterMemoryGuardHook() error = %v", err)
	}
	binaryPath := filepath.Join(t.TempDir(), "engram.exe")
	if err := os.WriteFile(binaryPath, []byte("binary"), 0o644); err != nil {
		t.Fatalf("WriteFile(binary) error = %v", err)
	}
	t.Setenv("CLICK_ENGRAM_BINARY_PATH", binaryPath)
	restoreEngramPath := seedEngramPathHealthy(t)
	defer restoreEngramPath()
	seedContext7Registered(t, cfg)

	report := Run(cfg)

	if !report.Healthy() {
		t.Fatalf("Run() after Install() reports unhealthy, want healthy: %+v", report.Checks)
	}
	for _, c := range report.Checks {
		if !c.Healthy {
			t.Errorf("check %q reported unhealthy after Install(): %s", c.Name, c.Detail)
		}
	}
}

func TestRun_AfterUninstall_ReportsUnhealthy(t *testing.T) {
	cfg := installer.Config{ClaudeHome: t.TempDir()}

	seedInstalledState(t, cfg)
	if err := installer.WriteManagedBlock(cfg.ClaudeMDPath(), installer.DefaultManagedContent); err != nil {
		t.Fatalf("WriteManagedBlock() error = %v", err)
	}
	if err := installer.RegisterMemoryGuardHook(cfg); err != nil {
		t.Fatalf("RegisterMemoryGuardHook() error = %v", err)
	}
	if err := installer.StripManagedBlock(cfg.ClaudeMDPath()); err != nil {
		t.Fatalf("StripManagedBlock() error = %v", err)
	}
	if err := os.Remove(cfg.InstalledPluginsPath()); err != nil {
		t.Fatalf("Remove(installed_plugins.json) error = %v", err)
	}
	if err := installer.UnregisterMemoryGuardHook(cfg); err != nil {
		t.Fatalf("UnregisterMemoryGuardHook() error = %v", err)
	}

	report := Run(cfg)

	if report.Healthy() {
		t.Fatal("Run() after Uninstall() reports healthy, want unhealthy")
	}
}

func TestRun_ChecksHavePluginAndClaudeMD(t *testing.T) {
	cfg := installer.Config{ClaudeHome: t.TempDir()}
	report := Run(cfg)

	const wantChecks = 10 + EngramChecksCount + Context7ChecksCount
	if len(report.Checks) != wantChecks {
		t.Fatalf("Run() returned %d checks, want %d (git, claude, click-sdd plugin, click-memory plugin, click-review plugin, click-skills plugin, CLAUDE.md, memory-guard hook, models.json schema, click-sdd applied plugin config, engram plugin, engram binary, engram PATH persistence, context7 MCP)", len(report.Checks), wantChecks)
	}
}

// TestCheckGit_ReportsHealthyWhenResolvable guards the "present" branch: git resolvable on PATH
// must report healthy, with the resolved path surfaced in Detail (mirroring checkEngramBinary's
// "resuelto en <path>" shape).
func TestCheckGit_ReportsHealthyWhenResolvable(t *testing.T) {
	restore := seedResolvableGit(t)
	defer restore()
	cfg := installer.Config{ClaudeHome: t.TempDir()}

	report := Run(cfg)

	var checked bool
	for _, c := range report.Checks {
		if c.Name != "git" {
			continue
		}
		checked = true
		if !c.Healthy {
			t.Fatalf("checkGit reports unhealthy when git is resolvable: %s", c.Detail)
		}
		if !strings.Contains(c.Detail, "/usr/bin/git") {
			t.Fatalf("checkGit Detail = %q, want it to contain the resolved path", c.Detail)
		}
	}
	if !checked {
		t.Fatal(`Run() did not include a "git" check`)
	}
}

// TestCheckGit_ReportsUnhealthyWithActionableMessageWhenMissing is the RED-then-GREEN core of the
// doctor half of this fix: reproduces the fresh-machine "no git on PATH" scenario and asserts
// checkGit's Detail carries the exact same actionable message `click install`'s preflight uses
// (installer.GitMissingMessage) — doctor and install must never give conflicting instructions.
func TestCheckGit_ReportsUnhealthyWithActionableMessageWhenMissing(t *testing.T) {
	restore := installer.SetBinaryLookupFactoryForTests(func() installer.BinaryLookup {
		return fakeGitLookup{resolved: map[string]string{}}
	})
	defer restore()
	cfg := installer.Config{ClaudeHome: t.TempDir()}

	report := Run(cfg)

	var checked bool
	for _, c := range report.Checks {
		if c.Name != "git" {
			continue
		}
		checked = true
		if c.Healthy {
			t.Fatal("checkGit reports healthy when git is not resolvable on PATH, want unhealthy")
		}
		if c.Detail != installer.GitMissingMessage {
			t.Fatalf("checkGit Detail = %q, want exactly installer.GitMissingMessage %q", c.Detail, installer.GitMissingMessage)
		}
	}
	if !checked {
		t.Fatal(`Run() did not include a "git" check`)
	}
}

// TestCheckClaude_ReportsHealthyWhenResolvable guards the "present" branch: claude resolvable on
// PATH must report healthy, with the resolved path surfaced in Detail — mirroring checkGit's shape.
func TestCheckClaude_ReportsHealthyWhenResolvable(t *testing.T) {
	restore := installer.SetBinaryLookupFactoryForTests(func() installer.BinaryLookup {
		return fakeGitLookup{resolved: map[string]string{"claude": "/usr/bin/claude"}}
	})
	defer restore()
	cfg := installer.Config{ClaudeHome: t.TempDir()}

	report := Run(cfg)

	var checked bool
	for _, c := range report.Checks {
		if c.Name != "claude" {
			continue
		}
		checked = true
		if !c.Healthy {
			t.Fatalf("checkClaude reports unhealthy when claude is resolvable: %s", c.Detail)
		}
		if !strings.Contains(c.Detail, "/usr/bin/claude") {
			t.Fatalf("checkClaude Detail = %q, want it to contain the resolved path", c.Detail)
		}
	}
	if !checked {
		t.Fatal(`Run() did not include a "claude" check`)
	}
}

// TestCheckClaude_ReportsUnhealthyWithActionableMessageWhenMissing is the doctor counterpart to
// PreflightClaude: reproduces the "no claude on PATH" scenario and asserts checkClaude's Detail
// carries the exact same actionable message `click install`/`click update`'s preflight uses
// (installer.ClaudeMissingMessage) — doctor and install/update must never give conflicting
// instructions, the same shared-message contract checkGit already holds for git.
func TestCheckClaude_ReportsUnhealthyWithActionableMessageWhenMissing(t *testing.T) {
	restore := installer.SetBinaryLookupFactoryForTests(func() installer.BinaryLookup {
		return fakeGitLookup{resolved: map[string]string{}}
	})
	defer restore()
	cfg := installer.Config{ClaudeHome: t.TempDir()}

	report := Run(cfg)

	var checked bool
	for _, c := range report.Checks {
		if c.Name != "claude" {
			continue
		}
		checked = true
		if c.Healthy {
			t.Fatal("checkClaude reports healthy when claude is not resolvable on PATH, want unhealthy")
		}
		if c.Detail != installer.ClaudeMissingMessage {
			t.Fatalf("checkClaude Detail = %q, want exactly installer.ClaudeMissingMessage %q", c.Detail, installer.ClaudeMissingMessage)
		}
	}
	if !checked {
		t.Fatal(`Run() did not include a "claude" check`)
	}
}

// TestCheckModelsConfig_AbsentFile_ReportsHealthy guards the "absent = healthy" contract from the
// taxonomy-migration spec: a home where `click install` never ran must not be flagged unhealthy for
// models.json — it just means defaults will be generated on the next install/update.
func TestCheckModelsConfig_AbsentFile_ReportsHealthy(t *testing.T) {
	cfg := installer.Config{ClaudeHome: t.TempDir()}

	report := Run(cfg)

	var checked bool
	for _, c := range report.Checks {
		if c.Name != "models.json schema" {
			continue
		}
		checked = true
		if !c.Healthy {
			t.Fatalf("checkModelsConfig reports unhealthy for an absent models.json: %s", c.Detail)
		}
	}
	if !checked {
		t.Fatal(`Run() did not include a "models.json schema" check`)
	}
}

// TestCheckModelsConfig_StaleFile_ReportsUnhealthy guards doctor's detection of a stale
// (pre-taxonomy-realignment) models.json, and that doctor never mutates it (NFR-012: read-only) —
// the raw file content must be unchanged after Run().
func TestCheckModelsConfig_StaleFile_ReportsUnhealthy(t *testing.T) {
	cfg := installer.Config{ClaudeHome: t.TempDir()}
	legacy := map[string]string{"orchestrator": "opus", "prd_writer": "opus", "architect": "opus", "reviewer": "opus", "memory_curator": "sonnet"}
	rawBefore, err := json.Marshal(legacy)
	if err != nil {
		t.Fatalf("json.Marshal(legacy) error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(cfg.ModelsPath()), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(cfg.ModelsPath(), rawBefore, 0o600); err != nil {
		t.Fatalf("WriteFile(legacy models.json) error = %v", err)
	}

	report := Run(cfg)

	var checked bool
	for _, c := range report.Checks {
		if c.Name != "models.json schema" {
			continue
		}
		checked = true
		if c.Healthy {
			t.Fatal("checkModelsConfig reports healthy for a stale legacy models.json, want unhealthy")
		}
	}
	if !checked {
		t.Fatal(`Run() did not include a "models.json schema" check`)
	}

	rawAfter, err := os.ReadFile(cfg.ModelsPath())
	if err != nil {
		t.Fatalf("ReadFile(models.json) after Run() error = %v", err)
	}
	if string(rawAfter) != string(rawBefore) {
		t.Fatalf("Run() mutated models.json (doctor must be read-only, NFR-012): before=%s after=%s", rawBefore, rawAfter)
	}
	if _, err := os.Stat(cfg.ModelsPath() + ".bak"); !os.IsNotExist(err) {
		t.Fatalf("Run() created a .bak backup file (doctor must be read-only, NFR-012), err = %v", err)
	}
}

// TestCheckEngramBinary_ReportsRemediationWhenMissing guards the shared-message contract from
// Slice 3b: when the engram binary is missing, doctor's Detail must include the exact same `go
// install` remediation command that `click install`'s own non-fatal provisioning fallback shows
// (installer.EngramBinaryRemediationMessage) — not a bare "missing" note that leaves the developer
// guessing the next step.
func TestCheckEngramBinary_ReportsRemediationWhenMissing(t *testing.T) {
	cfg := installer.Config{ClaudeHome: t.TempDir()}
	t.Setenv("CLICK_ENGRAM_BINARY_PATH", filepath.Join(t.TempDir(), "does-not-exist", "engram.exe"))

	m, err := manifest.Load()
	if err != nil {
		t.Fatalf("manifest.Load() error = %v", err)
	}

	report := Run(cfg)

	var checked bool
	for _, c := range report.Checks {
		if c.Name != "engram binary" {
			continue
		}
		checked = true
		if c.Healthy {
			t.Fatal("checkEngramBinary reports healthy for a binary path that does not exist on disk")
		}
		wantCmd := installer.EngramInstallCommand(m.Engram.Version)
		if !strings.Contains(c.Detail, wantCmd) {
			t.Fatalf("checkEngramBinary Detail = %q, want it to contain the remediation command %q", c.Detail, wantCmd)
		}
	}
	if !checked {
		t.Fatal(`Run() did not include an "engram binary" check`)
	}
}

// TestCheckContext7_ReportsMissingByDefault guards the "not registered yet" case: a fresh
// ClaudeHome where `click install` never ran (or context7 was never synced) must report
// unhealthy, not silently skip the check.
func TestCheckContext7_ReportsMissingByDefault(t *testing.T) {
	cfg := installer.Config{ClaudeHome: t.TempDir()}

	report := Run(cfg)

	var checked bool
	for _, c := range report.Checks {
		if c.Name != "context7 MCP" {
			continue
		}
		checked = true
		if c.Healthy {
			t.Fatal("checkContext7 reports healthy on a fresh home with no context7 registration")
		}
	}
	if !checked {
		t.Fatal(`Run() did not include a "context7 MCP" check`)
	}
}

// TestCheckContext7_ReportsHealthyWhenRegistered covers the "registered" case, reading directly
// from Claude Code's own user config file (the same file `claude mcp add --scope user` writes,
// per documentacion/spikes/spike-g-context7.md) rather than shelling out — matching every other
// check in this package.
func TestCheckContext7_ReportsHealthyWhenRegistered(t *testing.T) {
	cfg := installer.Config{ClaudeHome: t.TempDir()}
	seedContext7Registered(t, cfg)

	report := Run(cfg)

	var checked bool
	for _, c := range report.Checks {
		if c.Name != "context7 MCP" {
			continue
		}
		checked = true
		if !c.Healthy {
			t.Fatalf("checkContext7 reports unhealthy for a registered context7: %s", c.Detail)
		}
	}
	if !checked {
		t.Fatal(`Run() did not include a "context7 MCP" check`)
	}
}

// fakeGoEnvCommandRunner fakes `go env GOBIN`/`go env GOPATH` resolution for
// installer.GoBinDir's CommandRunner dependency, mirroring fakeGitLookup's role for checkGit's
// BinaryLookup dependency — so checkEngramPath's tests never depend on the real test machine's
// actual Go toolchain configuration. An empty gobin (and therefore empty GOBIN AND GOPATH, since
// this fake answers every `go env` query with gobin) reproduces GoBinDir's "neither resolves"
// error path.
type fakeGoEnvCommandRunner struct {
	gobin string
}

func (f fakeGoEnvCommandRunner) Run(name string, args ...string) error { return nil }

func (f fakeGoEnvCommandRunner) Output(name string, args ...string) ([]byte, error) {
	if name == "go" && len(args) == 2 && args[0] == "env" && args[1] == "GOBIN" {
		return []byte(f.gobin + "\n"), nil
	}
	return []byte("\n"), nil
}

// seedResolvableGoBinDir makes installer.GoBinDir resolve deterministically to gobin, regardless of
// the real test machine's actual Go toolchain configuration. Returns the restore func.
func seedResolvableGoBinDir(t *testing.T, gobin string) func() {
	t.Helper()
	return installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner {
		return fakeGoEnvCommandRunner{gobin: gobin}
	})
}

// setEngramPathProbes overrides checkEngramPath's two dependencies — installer.PathPersisted and
// installer.LivePathContains — via their exported cross-package test seams, so its four
// PERSISTED/LIVE combinations are deterministic regardless of the real test machine's
// registry/shell-rc/live-PATH state. Returns the combined restore func.
func setEngramPathProbes(persisted, live bool) func() {
	restorePersisted := installer.SetPathPersistedProbeForTests(func(dir string) (bool, error) { return persisted, nil })
	restoreLive := installer.SetLivePathContainsProbeForTests(func(dir string) bool { return live })
	return func() {
		restoreLive()
		restorePersisted()
	}
}

// seedEngramPathHealthy makes checkEngramPath deterministically report healthy (persisted AND
// live), mirroring seedResolvableGit's role for checkGit in TestRun_AfterInstall_ReportsHealthy.
// Returns the restore func.
func seedEngramPathHealthy(t *testing.T) func() {
	t.Helper()
	restoreRunner := seedResolvableGoBinDir(t, filepath.Join(t.TempDir(), "gobin"))
	restoreProbes := setEngramPathProbes(true, true)
	return func() {
		restoreProbes()
		restoreRunner()
	}
}

// TestCheckEngramPath_PersistedAndLive_ReportsHealthy covers state 1/4: everything lines up right
// now (a fresh install/update ran and this session already sees the result) — healthy.
func TestCheckEngramPath_PersistedAndLive_ReportsHealthy(t *testing.T) {
	restoreRunner := seedResolvableGoBinDir(t, filepath.Join(t.TempDir(), "gobin"))
	defer restoreRunner()
	restoreProbes := setEngramPathProbes(true, true)
	defer restoreProbes()

	cfg := installer.Config{ClaudeHome: t.TempDir()}
	c := checkEngramPath(cfg)

	if !c.Healthy {
		t.Fatalf("checkEngramPath() Healthy = false, want true when persisted AND live: %s", c.Detail)
	}
}

// TestCheckEngramPath_PersistedButNotLive_ReportsHealthyWithRestartMessage covers state 2/4 — the
// exact bug class this change targets: a PATH fix was already persisted (a prior install/update
// succeeded) but THIS session (or any already-running Claude Code) still has a stale live PATH.
// This must be non-fatal (Healthy: true, the persisted state is correct going forward) but visible
// (an actionable restart message in Detail) — never a hard doctor failure.
func TestCheckEngramPath_PersistedButNotLive_ReportsHealthyWithRestartMessage(t *testing.T) {
	restoreRunner := seedResolvableGoBinDir(t, filepath.Join(t.TempDir(), "gobin"))
	defer restoreRunner()
	restoreProbes := setEngramPathProbes(true, false)
	defer restoreProbes()

	cfg := installer.Config{ClaudeHome: t.TempDir()}
	c := checkEngramPath(cfg)

	if !c.Healthy {
		t.Fatalf("checkEngramPath() Healthy = false, want true (non-fatal) for persisted-but-not-live drift: %s", c.Detail)
	}
	if !strings.Contains(c.Detail, "reinicie") {
		t.Fatalf("checkEngramPath() Detail = %q, want an actionable message telling the user to restart their terminal/Claude Code", c.Detail)
	}
}

// TestCheckEngramPath_NotPersistedButLive_ReportsHealthy covers state 3/4, the edge case: dir
// resolves in THIS session's live PATH even though click never persisted it (e.g. a developer's own
// manual `export PATH=...`). It resolves right now, so it is healthy — click just didn't put it
// there, which is not itself a problem this check should flag.
func TestCheckEngramPath_NotPersistedButLive_ReportsHealthy(t *testing.T) {
	restoreRunner := seedResolvableGoBinDir(t, filepath.Join(t.TempDir(), "gobin"))
	defer restoreRunner()
	restoreProbes := setEngramPathProbes(false, true)
	defer restoreProbes()

	cfg := installer.Config{ClaudeHome: t.TempDir()}
	c := checkEngramPath(cfg)

	if !c.Healthy {
		t.Fatalf("checkEngramPath() Healthy = false, want true when live even though not persisted: %s", c.Detail)
	}
}

// TestCheckEngramPath_NeitherPersistedNorLive_ReportsUnhealthy covers state 4/4: genuinely not
// configured at all — the only state that must fail `click doctor`.
func TestCheckEngramPath_NeitherPersistedNorLive_ReportsUnhealthy(t *testing.T) {
	restoreRunner := seedResolvableGoBinDir(t, filepath.Join(t.TempDir(), "gobin"))
	defer restoreRunner()
	restoreProbes := setEngramPathProbes(false, false)
	defer restoreProbes()

	cfg := installer.Config{ClaudeHome: t.TempDir()}
	c := checkEngramPath(cfg)

	if c.Healthy {
		t.Fatal("checkEngramPath() Healthy = true, want false when neither persisted nor live")
	}
}

// TestCheckEngramPath_PersistedProbeError_ReportsUnhealthy guards the persisted-PATH probe's own
// error path (e.g. a broken registry read / unreadable rc file) — this must surface as unhealthy,
// not be silently swallowed or misreported as a healthy drift state.
func TestCheckEngramPath_PersistedProbeError_ReportsUnhealthy(t *testing.T) {
	restoreRunner := seedResolvableGoBinDir(t, filepath.Join(t.TempDir(), "gobin"))
	defer restoreRunner()

	wantErr := errors.New("registry closed")
	restorePersisted := installer.SetPathPersistedProbeForTests(func(dir string) (bool, error) { return false, wantErr })
	defer restorePersisted()
	restoreLive := installer.SetLivePathContainsProbeForTests(func(dir string) bool { return true })
	defer restoreLive()

	cfg := installer.Config{ClaudeHome: t.TempDir()}
	c := checkEngramPath(cfg)

	if c.Healthy {
		t.Fatal("checkEngramPath() Healthy = true, want false when the persisted-PATH probe itself errors")
	}
	if !strings.Contains(c.Detail, wantErr.Error()) {
		t.Fatalf("checkEngramPath() Detail = %q, want it to surface the probe error %q", c.Detail, wantErr.Error())
	}
}

// TestCheckEngramPath_GoBinDirError_ReportsUnhealthy guards the "can't even resolve the Go bin dir"
// path (no go toolchain / no GOBIN / no GOPATH) — checkEngramPath must fail closed, not panic or
// silently report healthy.
func TestCheckEngramPath_GoBinDirError_ReportsUnhealthy(t *testing.T) {
	restore := installer.SetCommandRunnerFactoryForTests(func() installer.CommandRunner {
		return fakeGoEnvCommandRunner{gobin: ""}
	})
	defer restore()

	cfg := installer.Config{ClaudeHome: t.TempDir()}
	c := checkEngramPath(cfg)

	if c.Healthy {
		t.Fatal("checkEngramPath() Healthy = true, want false when GoBinDir itself cannot resolve")
	}
}

func seedContext7Registered(t *testing.T, cfg installer.Config) {
	t.Helper()
	data := map[string]any{
		"mcpServers": map[string]any{
			"context7": map[string]any{"type": "http", "url": "https://mcp.context7.com/mcp"},
		},
	}
	raw, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("json.Marshal(context7 config) error = %v", err)
	}
	if err := os.WriteFile(cfg.Context7ConfigPath(), raw, 0o600); err != nil {
		t.Fatalf("WriteFile(Context7ConfigPath) error = %v", err)
	}
}

func seedInstalledState(t *testing.T, cfg installer.Config) {
	t.Helper()
	type pluginsRegistry struct {
		Version int                         `json:"version"`
		Plugins map[string][]map[string]any `json:"plugins"`
	}
	registry := pluginsRegistry{
		Version: 2,
		Plugins: map[string][]map[string]any{
			"click-sdd@click-ai-devkit":    {{}},
			"click-memory@click-ai-devkit": {{}},
			"click-review@click-ai-devkit": {{}},
			"click-skills@click-ai-devkit": {{}},
			installer.EngramPluginID:       {{}},
		},
	}
	data, err := json.Marshal(registry)
	if err != nil {
		t.Fatalf("json.Marshal(registry) error = %v", err)
	}
	if err := os.MkdirAll(filepathDir(cfg.InstalledPluginsPath()), 0o755); err != nil {
		t.Fatalf("MkdirAll(installed plugins dir) error = %v", err)
	}
	if err := os.WriteFile(cfg.InstalledPluginsPath(), data, 0o600); err != nil {
		t.Fatalf("WriteFile(installed_plugins.json) error = %v", err)
	}
	settings := map[string]any{
		"enabledPlugins": map[string]bool{
			"click-sdd@click-ai-devkit":    true,
			"click-memory@click-ai-devkit": true,
			"click-review@click-ai-devkit": true,
			"click-skills@click-ai-devkit": true,
			installer.EngramPluginID:       true,
		},
		"pluginConfigs": map[string]any{
			installer.ClickSDDPluginID: map[string]any{"options": allExpectedAppliedOptions()},
		},
	}
	settingsData, err := json.Marshal(settings)
	if err != nil {
		t.Fatalf("json.Marshal(settings) error = %v", err)
	}
	if err := os.WriteFile(cfg.SettingsPath(), settingsData, 0o600); err != nil {
		t.Fatalf("WriteFile(settings.json) error = %v", err)
	}
}

func filepathDir(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '\\' || path[i] == '/' {
			return path[:i]
		}
	}
	return "."
}
