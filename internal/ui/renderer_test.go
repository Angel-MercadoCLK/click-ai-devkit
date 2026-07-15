package ui

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

// esc is the ANSI escape byte lipgloss/termenv use to start a color sequence.
const esc = "\x1b["

// errBoom is a sentinel error used across RunStep tests to assert the underlying step function's
// error is propagated unchanged by RunStep.
var errBoom = errors.New("boom")

func containsANSI(s string) bool {
	return strings.Contains(s, esc)
}

func TestRenderer_PlainMode_NoANSI(t *testing.T) {
	r := &Renderer{Color: false}

	outputs := map[string]string{
		"Banner":  r.Banner(),
		"Success": r.Success("Plugin copiado"),
		"Fail":    r.Fail("Plugin no copiado"),
		"Step":    r.Step("Copiando plugin"),
		"Info":    r.Info("Instalación completa"),
		"Warn":    r.Warn("PATH no persistido"),
	}

	for name, out := range outputs {
		if containsANSI(out) {
			t.Errorf("%s output contains an ANSI escape sequence in plain mode: %q", name, out)
		}
	}
}

func TestRenderer_ColorMode_ContainsANSI(t *testing.T) {
	r := &Renderer{Color: true}

	outputs := map[string]string{
		"Banner":  r.Banner(),
		"Success": r.Success("Plugin copiado"),
		"Fail":    r.Fail("Plugin no copiado"),
		"Warn":    r.Warn("PATH no persistido"),
	}

	for name, out := range outputs {
		if !containsANSI(out) {
			t.Errorf("%s output does not contain an ANSI escape sequence in color mode: %q", name, out)
		}
	}
}

func TestRenderer_Success_PlainMode_HasPlainMarker(t *testing.T) {
	r := &Renderer{Color: false}
	got := r.Success("todo bien")
	if !strings.Contains(got, "todo bien") {
		t.Fatalf("Success() = %q, want it to contain the message", got)
	}
	if !strings.HasPrefix(got, "[OK]") {
		t.Fatalf("Success() = %q, want a plain [OK] prefix in plain mode", got)
	}
}

func TestRenderer_Fail_PlainMode_HasPlainMarker(t *testing.T) {
	r := &Renderer{Color: false}
	got := r.Fail("algo se rompió")
	if !strings.HasPrefix(got, "[FAIL]") {
		t.Fatalf("Fail() = %q, want a plain [FAIL] prefix in plain mode", got)
	}
}

func TestRenderer_Success_ColorMode_HasCheckmark(t *testing.T) {
	r := &Renderer{Color: true}
	got := r.Success("todo bien")
	if !strings.Contains(got, "✓") {
		t.Fatalf("Success() = %q, want a ✓ marker in color mode", got)
	}
}

func TestRenderer_Fail_ColorMode_HasCross(t *testing.T) {
	r := &Renderer{Color: true}
	got := r.Fail("algo se rompió")
	if !strings.Contains(got, "✗") {
		t.Fatalf("Fail() = %q, want a ✗ marker in color mode", got)
	}
}

// TestRenderer_Warn_PlainMode_HasPlainMarker covers D-5's Warn method (design obs #1436): a
// non-fatal warning (e.g. PATH persistence failed but the Engram binary is still resolvable) must
// be visually distinct from Fail's "[FAIL]" — using the design's committed "[warn] " lowercase
// plain-mode prefix — so a developer scanning install output doesn't mistake it for a hard failure.
func TestRenderer_Warn_PlainMode_HasPlainMarker(t *testing.T) {
	r := &Renderer{Color: false}
	got := r.Warn("no se pudo persistir el PATH")
	if !strings.Contains(got, "no se pudo persistir el PATH") {
		t.Fatalf("Warn() = %q, want it to contain the message", got)
	}
	if !strings.HasPrefix(got, "[warn] ") {
		t.Fatalf("Warn() = %q, want a plain \"[warn] \" prefix in plain mode", got)
	}
}

// TestRenderer_Warn_ColorMode_HasWarningMarker proves Warn uses its own visual treatment (a ⚠
// marker with lipgloss color 3/yellow, per D-5) rather than literally reusing Fail's ✗/red error
// styling — a warning must read as "heads up", not "something broke".
func TestRenderer_Warn_ColorMode_HasWarningMarker(t *testing.T) {
	r := &Renderer{Color: true}
	got := r.Warn("no se pudo persistir el PATH")
	if !strings.Contains(got, "⚠") {
		t.Fatalf("Warn() = %q, want a ⚠ marker in color mode", got)
	}
	if strings.Contains(got, "✗") {
		t.Fatalf("Warn() = %q, must not reuse Fail's ✗ marker", got)
	}
}

// TestRenderer_Warn_PlainMode_NoANSI is an explicit companion to the shared plain-mode ANSI sweep
// above, guarding Warn specifically since it's new in this batch.
func TestRenderer_Warn_PlainMode_NoANSI(t *testing.T) {
	r := &Renderer{Color: false}
	got := r.Warn("no se pudo persistir el PATH")
	if containsANSI(got) {
		t.Fatalf("Warn() plain mode = %q, contains an ANSI escape sequence", got)
	}
}

func TestRenderer_RunStep_PlainMode_Success(t *testing.T) {
	var buf bytes.Buffer
	r := &Renderer{Color: false, Out: &buf}

	err := r.RunStep("Copiando plugin…", "Plugin copiado", func() error {
		return nil
	})
	if err != nil {
		t.Fatalf("RunStep() error = %v, want nil", err)
	}

	out := buf.String()
	if containsANSI(out) {
		t.Errorf("RunStep plain-mode output contains an ANSI escape sequence: %q", out)
	}
	if !strings.Contains(out, "Plugin copiado") {
		t.Errorf("RunStep output = %q, want it to contain the done label", out)
	}
}

func TestRenderer_RunStep_PlainMode_Failure(t *testing.T) {
	var buf bytes.Buffer
	r := &Renderer{Color: false, Out: &buf}

	sentinel := errBoom
	err := r.RunStep("Copiando plugin…", "Plugin copiado", func() error {
		return sentinel
	})
	if err != sentinel {
		t.Fatalf("RunStep() error = %v, want %v", err, sentinel)
	}

	out := buf.String()
	if containsANSI(out) {
		t.Errorf("RunStep plain-mode output contains an ANSI escape sequence: %q", out)
	}
	if !strings.Contains(out, "[FAIL]") {
		t.Errorf("RunStep output = %q, want a [FAIL] line on error", out)
	}
}

func TestRenderer_RunStep_ColorMode_ReturnsFnError(t *testing.T) {
	var buf bytes.Buffer
	r := &Renderer{Color: true, Out: &buf}

	sentinel := errBoom
	err := r.RunStep("Configurando…", "Configurado", func() error {
		return sentinel
	})
	if err != sentinel {
		t.Fatalf("RunStep() error = %v, want %v", err, sentinel)
	}
}

func TestNewRenderer_NoColorFlagForcesPlain(t *testing.T) {
	var buf bytes.Buffer
	r := NewRenderer(&buf, true)
	if r.Color {
		t.Fatal("NewRenderer(noColorFlag=true) produced a color-enabled renderer")
	}
}

func TestNewRenderer_NoColorEnvForcesPlain(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	var buf bytes.Buffer
	r := NewRenderer(&buf, false)
	if r.Color {
		t.Fatal("NewRenderer() with NO_COLOR set produced a color-enabled renderer")
	}
}

func TestNewRenderer_NonFileWriterForcesPlain(t *testing.T) {
	var buf bytes.Buffer
	r := NewRenderer(&buf, false)
	if r.Color {
		t.Fatal("NewRenderer() with a non-*os.File writer (never a TTY) produced a color-enabled renderer")
	}
}
