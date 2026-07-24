package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/installer"
)

// warnCapturingRenderer implements the minimal `interface{ Warn(string) string }` resolveTargetConfig
// expects, passing the warning text through verbatim so tests can assert on it.
type warnCapturingRenderer struct{}

func (warnCapturingRenderer) Warn(s string) string { return s }

// TestResolveTargetConfig_OpenClawAvailableButPreviouslyExcluded_Warns is FIX 5's regression: when a
// prior explicit selection persisted OpenClaw=false (e.g. `click install --skip-openclaw`) but
// OpenClaw is installed and detected now, the shared install/update path must surface a Spanish,
// non-fatal warning telling the developer it is available and how to re-enable it — without changing
// the persisted selection or aborting.
func TestResolveTargetConfig_OpenClawAvailableButPreviouslyExcluded_Warns(t *testing.T) {
	stateHome := t.TempDir()
	t.Setenv("CLICK_STATE_HOME", stateHome)
	cfg := installer.Config{ClickStateHome: stateHome}

	// A prior configured selection that explicitly excluded OpenClaw.
	if err := installer.SaveTargetSelection(cfg, installer.TargetSelection{Configured: true, Claude: true, OpenClaw: false}); err != nil {
		t.Fatalf("SaveTargetSelection() error = %v", err)
	}

	// OpenClaw IS installed and detectable now.
	restore := installer.SetBinaryLookupFactoryForTests(func() installer.BinaryLookup {
		return cliFakeBinaryLookup{resolved: map[string]string{"openclaw": "/usr/bin/openclaw"}}
	})
	defer restore()

	var buf bytes.Buffer
	selection, resolved, err := resolveTargetConfig(cfg, false, &buf, warnCapturingRenderer{})
	if err != nil {
		t.Fatalf("resolveTargetConfig() error = %v", err)
	}
	// The persisted exclusion is respected: OpenClawHome must stay empty (integration still skipped).
	if resolved.OpenClawHome != "" {
		t.Fatalf("resolved.OpenClawHome = %q, want empty — a prior exclusion must NOT be silently re-enabled here", resolved.OpenClawHome)
	}
	if selection.OpenClaw {
		t.Fatalf("selection.OpenClaw = true, want the persisted false selection returned unchanged")
	}
	if !strings.Contains(buf.String(), "disponible") || !strings.Contains(buf.String(), "click configure-targets") {
		t.Fatalf("output = %q, want the available-but-excluded warning pointing at `click configure-targets`", buf.String())
	}
}

// TestResolveTargetConfig_OpenClawExcludedAndAbsent_NoAvailabilityWarning proves the new warning is
// scoped to the "available now" case: when OpenClaw was excluded AND is not detected, the
// available-but-excluded warning must NOT fire (there is nothing to re-enable).
func TestResolveTargetConfig_OpenClawExcludedAndAbsent_NoAvailabilityWarning(t *testing.T) {
	stateHome := t.TempDir()
	t.Setenv("CLICK_STATE_HOME", stateHome)
	cfg := installer.Config{ClickStateHome: stateHome}

	if err := installer.SaveTargetSelection(cfg, installer.TargetSelection{Configured: true, Claude: true, OpenClaw: false}); err != nil {
		t.Fatalf("SaveTargetSelection() error = %v", err)
	}

	// No openclaw on PATH -> not detected.
	restore := installer.SetBinaryLookupFactoryForTests(func() installer.BinaryLookup {
		return cliFakeBinaryLookup{resolved: map[string]string{}}
	})
	defer restore()

	var buf bytes.Buffer
	if _, _, err := resolveTargetConfig(cfg, false, &buf, warnCapturingRenderer{}); err != nil {
		t.Fatalf("resolveTargetConfig() error = %v", err)
	}
	if strings.Contains(buf.String(), "disponible en este equipo") {
		t.Fatalf("output = %q, want NO available-but-excluded warning when OpenClaw is not detected", buf.String())
	}
}
