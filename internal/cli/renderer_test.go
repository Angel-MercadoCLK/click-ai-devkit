package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/ui"
)

// TestSurfacePathWarning_EmptyIsNoop covers the "not attempted / succeeded" case: install.go and
// update.go call surfacePathWarning unconditionally after SyncEngram returns, and an empty
// pathWarning (no PATH-persistence attempt was made, or it succeeded) must produce zero output —
// nothing extra to show the developer.
func TestSurfacePathWarning_EmptyIsNoop(t *testing.T) {
	var buf bytes.Buffer
	r := &ui.Renderer{Color: false, Out: &buf}

	surfacePathWarning(&buf, r, "")

	if buf.Len() != 0 {
		t.Fatalf("surfacePathWarning(\"\") wrote %q, want zero output", buf.String())
	}
}

// TestSurfacePathWarning_NonEmptyCallsWarn covers the strict-TDD requirement (d): each call site
// must actually surface a non-empty pathWarning via r.Warn, not swallow it silently — this is the
// exact bug this PR closes (a silent PATH problem masquerading as install success).
func TestSurfacePathWarning_NonEmptyCallsWarn(t *testing.T) {
	var buf bytes.Buffer
	r := &ui.Renderer{Color: false, Out: &buf}

	surfacePathWarning(&buf, r, "no se pudo agregar C:\\Users\\dev\\go\\bin al PATH persistente")

	out := buf.String()
	if !strings.HasPrefix(strings.TrimSpace(out), "[warn]") {
		t.Fatalf("surfacePathWarning() output = %q, want it to start with a [warn] marker (via r.Warn)", out)
	}
	if !strings.Contains(out, "no se pudo agregar") {
		t.Fatalf("surfacePathWarning() output = %q, want it to contain the pathWarning message", out)
	}
}
