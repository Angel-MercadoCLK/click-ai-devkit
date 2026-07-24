package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/installer"
)

func TestTargetsCommand_ReportsSupportedDetectionAndTruthfulCapabilities(t *testing.T) {
	restore := installer.SetBinaryLookupFactoryForTests(func() installer.BinaryLookup {
		return targetLookup{paths: map[string]string{
			"claude":   "/usr/bin/claude",
			"openclaw": "/usr/bin/openclaw",
			"codex":    "/usr/bin/codex",
		}}
	})
	defer restore()

	var out bytes.Buffer
	cmd := NewRootCommand()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"targets", "--no-color"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("click targets error = %v", err)
	}

	report := out.String()
	for _, want := range []string{
		"Claude Code: detectado",
		"flujo completo de plugins, SDD y modelos",
		"OpenClaw: detectado",
		"SDD portable, Engram, memory guard y recomendación de modelos",
		"no aplica modelos nativamente allí",
		"Codex: detectado",
		"AGENTS.md gestionado",
		"no modifica config.toml ni el modelo",
		"Otros runtimes: no soportados todavía",
	} {
		if !strings.Contains(report, want) {
			t.Errorf("targets output missing %q:\n%s", want, report)
		}
	}
}

func TestTargetsCommand_ReportsAbsentTargetsWithoutWriting(t *testing.T) {
	restore := installer.SetBinaryLookupFactoryForTests(func() installer.BinaryLookup {
		return targetLookup{paths: map[string]string{}}
	})
	defer restore()

	var out bytes.Buffer
	cmd := NewRootCommand()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"targets", "--no-color"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("click targets error = %v", err)
	}

	report := out.String()
	if !strings.Contains(report, "Claude Code: no detectado") || !strings.Contains(report, "OpenClaw: no detectado") || !strings.Contains(report, "Codex: no detectado") {
		t.Fatalf("targets output missing absent status:\n%s", report)
	}
	if strings.Contains(report, "instalado") || strings.Contains(report, "gestionado") {
		t.Fatalf("targets output overclaims unsupported or absent runtime state:\n%s", report)
	}
}

func TestConfigureTargetsCommand_NonTTY_DoesNotStartTUI(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CLICK_CLAUDE_HOME", home)
	var out bytes.Buffer
	cmd := NewRootCommand()
	cmd.SetIn(strings.NewReader("\r"))
	cmd.SetOut(&out)
	cmd.SetErr(cmd.OutOrStderr())
	cmd.SetArgs([]string{"configure-targets"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("configure-targets error = %v", err)
	}
	if !strings.Contains(out.String(), "No hay terminal interactiva disponible") {
		t.Fatalf("configure-targets non-TTY output = %q, want no-terminal message", out.String())
	}
	if _, found, err := installer.LoadTargetSelection(installer.Config{ClaudeHome: home}); err != nil {
		t.Fatalf("LoadTargetSelection() error = %v", err)
	} else if found {
		t.Fatal("configure-targets wrote a selection in non-TTY mode")
	}
}

func TestResolveTargetConfig_ExplicitSelectionAndSkipOverride(t *testing.T) {
	home := t.TempDir()
	cfg := installer.Config{ClaudeHome: home}
	if err := installer.SaveTargetSelection(cfg, installer.TargetSelection{Configured: true, Claude: true, OpenClaw: true}); err != nil {
		t.Fatalf("SaveTargetSelection() error = %v", err)
	}
	restore := installer.SetBinaryLookupFactoryForTests(func() installer.BinaryLookup {
		return targetLookup{paths: map[string]string{"openclaw": "/usr/bin/openclaw"}}
	})
	defer restore()

	var out bytes.Buffer
	r := rendererFor(NewRootCommand(), &out)
	_, withOpenClaw, err := resolveTargetConfig(cfg, false, &out, r)
	if err != nil {
		t.Fatalf("resolveTargetConfig() error = %v", err)
	}
	if withOpenClaw.OpenClawHome == "" {
		t.Fatal("explicit OpenClaw selection did not add OpenClawHome")
	}

	_, withoutOpenClaw, err := resolveTargetConfig(cfg, true, &out, r)
	if err != nil {
		t.Fatalf("resolveTargetConfig(skip) error = %v", err)
	}
	if withoutOpenClaw.OpenClawHome != "" {
		t.Fatal("--skip-openclaw did not override saved selection")
	}
}

func TestResolveTargetConfig_SelectedButAbsentOpenClawIsNonFatal(t *testing.T) {
	home := t.TempDir()
	cfg := installer.Config{ClaudeHome: home}
	if err := installer.SaveTargetSelection(cfg, installer.TargetSelection{Configured: true, Claude: true, OpenClaw: true}); err != nil {
		t.Fatalf("SaveTargetSelection() error = %v", err)
	}
	restore := installer.SetBinaryLookupFactoryForTests(func() installer.BinaryLookup { return targetLookup{paths: map[string]string{}} })
	defer restore()
	var out bytes.Buffer
	r := rendererFor(NewRootCommand(), &out)
	_, got, err := resolveTargetConfig(cfg, false, &out, r)
	if err != nil {
		t.Fatalf("resolveTargetConfig() error = %v, want non-fatal absence", err)
	}
	if got.OpenClawHome != "" || !strings.Contains(out.String(), "no está disponible") {
		t.Fatalf("resolved config/output = (%+v, %q), want skipped OpenClaw warning", got, out.String())
	}
}

func TestResolveTargetConfig_SelectedCodexResolvesHomeWithoutOpenClaw(t *testing.T) {
	claudeHome := t.TempDir()
	codexHome := t.TempDir()
	t.Setenv("CODEX_HOME", codexHome)
	cfg := installer.Config{ClaudeHome: claudeHome}
	if err := installer.SaveTargetSelection(cfg, installer.TargetSelection{Configured: true, Claude: true, Codex: true}); err != nil {
		t.Fatal(err)
	}
	restore := installer.SetBinaryLookupFactoryForTests(func() installer.BinaryLookup {
		return targetLookup{paths: map[string]string{"codex": "/usr/bin/codex"}}
	})
	defer restore()
	var out bytes.Buffer
	r := rendererFor(NewRootCommand(), &out)
	_, got, err := resolveTargetConfig(cfg, true, &out, r)
	if err != nil {
		t.Fatal(err)
	}
	if got.CodexHome != codexHome || got.OpenClawHome != "" {
		t.Fatalf("resolved config = %+v, want Codex only", got)
	}
}

// TestResolveTargetConfig_MalformedTargetsJSON_DegradesToSafeDefault is a FIX 3 guard: a targets.json
// that exists but is invalid JSON must NOT abort install/update. resolveTargetConfig must warn and
// fall back to the safe default (Claude on, OpenClaw follows detection), returning a nil error.
func TestResolveTargetConfig_MalformedTargetsJSON_DegradesToSafeDefault(t *testing.T) {
	assertResolveTargetConfigDegrades(t, "{ this is not valid json ")
}

// TestResolveTargetConfig_SchemaMismatchTargetsJSON_DegradesToSafeDefault is the FIX 3 guard for a
// well-formed but unsupported-schema targets.json: same graceful degradation, no aborted run.
func TestResolveTargetConfig_SchemaMismatchTargetsJSON_DegradesToSafeDefault(t *testing.T) {
	assertResolveTargetConfigDegrades(t, `{"schemaVersion": 999, "configured": true, "claude": true, "openclaw": false}`)
}

func assertResolveTargetConfigDegrades(t *testing.T, targetsContent string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("CLICK_OPENCLAW_HOME", t.TempDir())
	cfg := installer.Config{ClaudeHome: home}
	writeRawTargetSelection(t, cfg, targetsContent)
	restore := installer.SetBinaryLookupFactoryForTests(func() installer.BinaryLookup {
		return targetLookup{paths: map[string]string{"openclaw": "/usr/bin/openclaw"}}
	})
	defer restore()

	var out bytes.Buffer
	r := rendererFor(NewRootCommand(), &out)
	_, got, err := resolveTargetConfig(cfg, false, &out, r)
	if err != nil {
		t.Fatalf("resolveTargetConfig(bad targets.json) error = %v, want nil (must not brick install/update)", err)
	}
	if got.OpenClawHome == "" {
		t.Fatal("safe default should follow OpenClaw detection, but OpenClawHome was empty")
	}
	if !strings.Contains(out.String(), "detección por defecto") {
		t.Fatalf("resolveTargetConfig(bad targets.json) output = %q, want a degradation warning", out.String())
	}
}

func writeRawTargetSelection(t *testing.T, cfg installer.Config, content string) {
	t.Helper()
	path := cfg.TargetSelectionPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}

type targetLookup struct {
	paths map[string]string
}

func (l targetLookup) LookPath(name string) (string, error) {
	path, ok := l.paths[name]
	if !ok {
		return "", errTargetNotFound{name: name}
	}
	return path, nil
}

type errTargetNotFound struct {
	name string
}

func (e errTargetNotFound) Error() string { return "target not found: " + e.name }
