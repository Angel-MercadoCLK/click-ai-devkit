package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Shared brand palette for every full-screen TUI.
//
// The root menu (internal/menu) established this look; these constants are the single definition
// of it so the selection screens composed around it read as the same product rather than as a
// different, cruder app taking over the terminal. internal/menu consumes them too — the palette
// deliberately lives here because internal/menu imports internal/ui, not the other way round.
const (
	BrandAccent = "#ff8c2e" // pointer, focused row, key chords
	BrandLabel  = "#d3dae2" // ordinary row text
	BrandMuted  = "#4b5663" // secondary marks (row numbers, checkboxes)
	BrandBorder = "#3d4753" // panel border
	BrandTitle  = "#b98a5a" // screen caption
	BrandDim    = "#5a6470" // help text, notes, disabled rows
	BrandDanger = "#e5534b" // inline error text inside a screen
)

// pointerGlyph marks the focused row. Matches the root menu exactly.
const pointerGlyph = "▸"

// rowIndent keeps continuation lines aligned under a row's label rather than under its pointer.
const rowIndent = "  "

func brandStyle(color string) lipgloss.Style {
	return styleRenderer.NewStyle().Foreground(lipgloss.Color(color))
}

// Heading renders a screen caption, filling the same role as the root menu's "MENÚ" line.
func Heading(text string) string {
	return brandStyle(BrandTitle).Render(text)
}

// Panel wraps content in the shared rounded box used by the root menu.
func Panel(content string) string {
	return styleRenderer.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(BrandBorder)).
		Padding(0, 3).
		Render(content)
}

// Screen composes a caption and a panel of rows the way every screen should present itself.
func Screen(caption string, rows []string) string {
	return Heading(caption) + "\n" + Panel(strings.Join(rows, "\n"))
}

// Row renders one selectable row. The focused row gets an accent pointer and accented, bold text;
// unfocused rows stay in the ordinary label color. Extra lines are continuation text for the same
// row and are indented to sit under the label, never under the pointer — which is what made the
// previous multi-line rows look ragged.
func Row(focused bool, lines ...string) string {
	if len(lines) == 0 {
		return ""
	}

	pointer := " "
	textStyle := brandStyle(BrandLabel)
	if focused {
		pointer = pointerGlyph
		textStyle = brandStyle(BrandAccent).Bold(true)
	}
	pointerStyle := brandStyle(BrandAccent).Bold(true)

	out := make([]string, 0, len(lines))
	out = append(out, pointerStyle.Render(pointer)+" "+textStyle.Render(lines[0]))
	continuation := brandStyle(BrandDim)
	if focused {
		continuation = brandStyle(BrandLabel)
	}
	for _, line := range lines[1:] {
		out = append(out, rowIndent+continuation.Render(line))
	}
	return strings.Join(out, "\n")
}

// RowWithMarker renders a row whose first line starts with an independently styled marker, such as
// a selection checkbox that must stay legible whether or not the row currently has focus.
//
// The marker is composed alongside the label rather than inside it on purpose: lipgloss closes
// each Render with a full reset, so nesting a styled marker inside styled label text silently
// strips the styling from everything after the marker.
func RowWithMarker(focused bool, marker, label string, extra ...string) string {
	pointer := " "
	textStyle := brandStyle(BrandLabel)
	if focused {
		pointer = pointerGlyph
		textStyle = brandStyle(BrandAccent).Bold(true)
	}

	head := brandStyle(BrandAccent).Bold(true).Render(pointer) + " " + marker + " " + textStyle.Render(label)

	out := []string{head}
	continuation := brandStyle(BrandDim)
	if focused {
		continuation = brandStyle(BrandLabel)
	}
	for _, line := range extra {
		out = append(out, rowIndent+continuation.Render(line))
	}
	return strings.Join(out, "\n")
}

// DisabledRow renders a row that cannot be chosen: no pointer, fully dimmed.
func DisabledRow(lines ...string) string {
	if len(lines) == 0 {
		return ""
	}
	dim := brandStyle(BrandDim).Faint(true)
	out := make([]string, 0, len(lines))
	out = append(out, "  "+dim.Render(lines[0]))
	for _, line := range lines[1:] {
		out = append(out, rowIndent+dim.Render(line))
	}
	return strings.Join(out, "\n")
}

// Marker renders a checkbox-style marker in the shared palette: accented when set, muted when not.
func Marker(set bool) string {
	if set {
		return brandStyle(BrandAccent).Bold(true).Render("[x]")
	}
	return brandStyle(BrandMuted).Render("[ ]")
}

// Note renders dim explanatory text, for the lines that sit between a panel and its key hints.
func Note(text string) string {
	return brandStyle(BrandDim).Render(text)
}

// Danger renders an inline error message inside a screen. It keeps the red role the Renderer's
// Fail already uses for command output, so an error reads the same wherever it appears.
func Danger(text string) string {
	return brandStyle(BrandDanger).Render(text)
}

// Hints renders the footer key help in the root menu's style: accent-colored key chords with dim
// descriptions. Arguments alternate key, description — an odd trailing argument is rendered as a
// bare description.
func Hints(pairs ...string) string {
	key := brandStyle(BrandAccent).Bold(true)
	dim := brandStyle(BrandDim)

	var b strings.Builder
	b.WriteString(dim.Render("  "))
	for i := 0; i < len(pairs); i += 2 {
		if i > 0 {
			b.WriteString(dim.Render("    "))
		}
		if i+1 >= len(pairs) {
			b.WriteString(dim.Render(pairs[i]))
			break
		}
		b.WriteString(key.Render(pairs[i]))
		b.WriteString(dim.Render(" " + pairs[i+1]))
	}
	return b.String()
}

// PadLabel right-pads label to width display columns so a screen's second column lines up. Uses
// lipgloss's width so it stays correct for non-ASCII labels, which plain len() gets wrong.
func PadLabel(label string, width int) string {
	pad := width - lipgloss.Width(label)
	if pad <= 0 {
		return label
	}
	return label + strings.Repeat(" ", pad)
}

// WidestLabel returns the display width of the longest label, for column alignment.
func WidestLabel(labels []string) int {
	widest := 0
	for _, l := range labels {
		if w := lipgloss.Width(l); w > widest {
			widest = w
		}
	}
	return widest
}
