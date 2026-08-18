package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestRow_PointerOnlyOnFocusedRow(t *testing.T) {
	focused := Row(true, "instalar")
	blurred := Row(false, "instalar")

	if !strings.Contains(focused, pointerGlyph) {
		t.Errorf("focused row %q is missing the %q pointer", focused, pointerGlyph)
	}
	if strings.Contains(blurred, pointerGlyph) {
		t.Errorf("unfocused row %q must not draw the pointer", blurred)
	}
	if focused == blurred {
		t.Error("focused and unfocused rows render identically; the cursor would be invisible")
	}
}

func TestRow_ContinuationLinesAlignUnderTheLabel(t *testing.T) {
	row := Row(true, "Claude Code", "Capacidades: plugins nativos")

	lines := strings.Split(row, "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d: %q", len(lines), row)
	}
	if !strings.HasPrefix(lines[1], rowIndent) {
		t.Errorf("continuation line %q must be indented to sit under the label, not under the pointer", lines[1])
	}
	if strings.Contains(lines[1], pointerGlyph) {
		t.Errorf("continuation line %q must not repeat the pointer", lines[1])
	}
}

// TestRowWithMarker_LabelKeepsItsStyling guards a real rendering bug: lipgloss closes every Render
// with a full reset, so composing an already-styled marker INSIDE styled label text silently
// stripped the styling from everything after the marker — the focused row's name rendered plain.
func TestRowWithMarker_LabelKeepsItsStyling(t *testing.T) {
	label := "Claude Code (detectado)"
	row := RowWithMarker(true, Marker(true), label)

	wantStyledLabel := brandStyle(BrandAccent).Bold(true).Render(label)
	if !strings.Contains(row, wantStyledLabel) {
		t.Errorf("focused row lost its label styling.\n got: %q\nwant it to contain: %q", row, wantStyledLabel)
	}
}

func TestRowWithMarker_MarkerReflectsSelectionIndependentlyOfFocus(t *testing.T) {
	// A checked box on an unfocused row must still read as checked: selection and cursor are
	// different pieces of information.
	unfocusedChecked := RowWithMarker(false, Marker(true), "OpenClaw")
	unfocusedUnchecked := RowWithMarker(false, Marker(false), "OpenClaw")

	if unfocusedChecked == unfocusedUnchecked {
		t.Error("checked and unchecked unfocused rows render identically; selection would be invisible")
	}
	if !strings.Contains(unfocusedChecked, "[x]") {
		t.Errorf("row %q should show a checked marker", unfocusedChecked)
	}
	if !strings.Contains(unfocusedUnchecked, "[ ]") {
		t.Errorf("row %q should show an unchecked marker", unfocusedUnchecked)
	}
}

func TestPadLabel_AlignsColumnsForNonASCIILabels(t *testing.T) {
	// len() would report 20 bytes for this 18-column label and under-pad it.
	labels := []string{"explore", "review-readability", "diseño"}
	width := WidestLabel(labels)
	if width != len("review-readability") {
		t.Fatalf("WidestLabel() = %d, want %d", width, len("review-readability"))
	}

	for _, label := range labels {
		padded := PadLabel(label, width)
		if got := lipgloss.Width(padded); got != width {
			t.Errorf("PadLabel(%q, %d) has display width %d, want %d", label, width, got, width)
		}
	}
}

func TestPadLabel_LeavesOverlongLabelsIntact(t *testing.T) {
	if got := PadLabel("muy-largo", 3); got != "muy-largo" {
		t.Errorf("PadLabel truncated or padded an overlong label: %q", got)
	}
}

func TestHints_PairsKeysWithDescriptions(t *testing.T) {
	hints := Hints("enter", "elegir", "q · esc", "salir")

	for _, want := range []string{"enter", "elegir", "q · esc", "salir"} {
		if !strings.Contains(hints, want) {
			t.Errorf("hints %q missing %q", hints, want)
		}
	}
	if !strings.Contains(hints, brandStyle(BrandAccent).Bold(true).Render("enter")) {
		t.Error("key chords should be rendered in the accent color")
	}
}

func TestHints_TrailingOddArgumentIsRenderedAsDescription(t *testing.T) {
	hints := Hints("enter", "elegir", "solo lectura")
	if !strings.Contains(hints, "solo lectura") {
		t.Errorf("hints %q dropped the trailing description", hints)
	}
}

func TestScreen_WrapsRowsInAPanelUnderTheCaption(t *testing.T) {
	out := Screen("MENÚ", []string{Row(true, "instalar"), Row(false, "actualizar")})

	if !strings.Contains(out, "MENÚ") {
		t.Error("screen is missing its caption")
	}
	// The rounded border is the shared visual signature every screen must carry.
	if !strings.Contains(out, "╭") || !strings.Contains(out, "╰") {
		t.Errorf("screen is not wrapped in the shared rounded panel:\n%s", out)
	}
	for _, want := range []string{"instalar", "actualizar"} {
		if !strings.Contains(out, want) {
			t.Errorf("screen is missing row %q", want)
		}
	}
}
