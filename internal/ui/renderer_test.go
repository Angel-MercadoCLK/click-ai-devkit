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
