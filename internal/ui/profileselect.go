package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/modelconfig"
)

// profileSelectNames lists the rows shown on the profile-select screen: the three built-in presets
// (in modelconfig.Profiles() order) plus "custom" for a fully hand-tuned per-phase selection.
// modelconfig.Profiles() deliberately excludes "custom" — it has no preset Models map of its own
// (see profiles.go) — so it's appended here as the TUI's fourth, always-last row.
var profileSelectNames = []modelconfig.ProfileName{
	modelconfig.ProfileBalanced,
	modelconfig.ProfileCostSaver,
	modelconfig.ProfileQuality,
	modelconfig.ProfileCustom,
}

// profileDescriptions gives each ProfileName a short Spanish description for the select screen
// (D10: dev-facing CLI text may be Spanish, matching modelselect.go's own key-help line).
var profileDescriptions = map[modelconfig.ProfileName]string{
	modelconfig.ProfileBalanced:  "equilibrado: sonnet en la mayoría de fases, opus en las críticas",
	modelconfig.ProfileCostSaver: "económico: haiku en la mayoría de fases, opus solo en las críticas",
	modelconfig.ProfileQuality:   "calidad: opus en casi todas las fases",
	modelconfig.ProfileCustom:    "personalizado: elige el modelo fase por fase",
}

// ProfileSelectModel is the bubbletea model that drives `click install`'s profile-select step
// (design D4): one row per built-in profile plus "custom", shown BEFORE the per-phase editor so a
// developer can pick a preset in one keystroke instead of tuning all 18 phases by hand. Follows
// ModelSelectModel's exact keymap (j/k or arrow up/down to move, enter to confirm, esc/q/ctrl+c to
// cancel) for a consistent feel across both install screens.
type ProfileSelectModel struct {
	Cursor    int
	Selected  modelconfig.ProfileName
	Confirmed bool
	Cancelled bool
}

// NewProfileSelectModel builds a ProfileSelectModel with the cursor on "balanced" — the default
// preset a developer gets by just pressing enter, matching ModelSelectModel's "accept the default in
// one key" behavior. Equivalent to NewProfileSelectModelForProfile(modelconfig.ProfileBalanced).
func NewProfileSelectModel() ProfileSelectModel {
	return NewProfileSelectModelForProfile(modelconfig.ProfileBalanced)
}

// NewProfileSelectModelForProfile builds a ProfileSelectModel with the cursor seeded on initial
// (C2 fix): `click install --profile X` on a real terminal now preloads the interactive picker's
// cursor on X instead of always hardcoding balanced, matching the flag's own help text. The
// developer can still move off it before confirming. An empty, unrecognized, or otherwise
// unmatched name falls back to balanced (index 0), matching modelconfig.ResolveProfile's own
// fallback rule.
func NewProfileSelectModelForProfile(initial modelconfig.ProfileName) ProfileSelectModel {
	for i, name := range profileSelectNames {
		if name == initial {
			return ProfileSelectModel{Cursor: i, Selected: name}
		}
	}
	return ProfileSelectModel{Selected: profileSelectNames[0]}
}

// Init satisfies tea.Model. The selection screen needs no startup command.
func (m ProfileSelectModel) Init() tea.Cmd { return nil }

// Update satisfies tea.Model, handling only keyboard input: every other message is a no-op so the
// screen stays static under resizes, ticks, etc.
func (m ProfileSelectModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	switch keyMsg.Type {
	case tea.KeyUp:
		m.moveCursor(-1)
	case tea.KeyDown:
		m.moveCursor(1)
	case tea.KeyEnter:
		m.Selected = profileSelectNames[m.Cursor]
		m.Confirmed = true
		return m, tea.Quit
	case tea.KeyEsc, tea.KeyCtrlC:
		m.Cancelled = true
		return m, tea.Quit
	case tea.KeyRunes:
		switch string(keyMsg.Runes) {
		case "k":
			m.moveCursor(-1)
		case "j":
			m.moveCursor(1)
		case "q":
			m.Cancelled = true
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m *ProfileSelectModel) moveCursor(delta int) {
	n := len(profileSelectNames)
	m.Cursor = ((m.Cursor+delta)%n + n) % n
	m.Selected = profileSelectNames[m.Cursor]
}

// View satisfies tea.Model, rendering one row per profile with a cursor marker and a short
// description, plus a short Spanish key-help line matching modelselect.go's own.
func (m ProfileSelectModel) View() string {
	var b strings.Builder
	b.WriteString(styleRenderer.NewStyle().Bold(true).Render("Elegí un perfil de orquestación para click-sdd"))
	b.WriteString("\n\n")

	for i, name := range profileSelectNames {
		marker := "  "
		if i == m.Cursor {
			marker = "> "
		}
		line := fmt.Sprintf("%s%-12s %s", marker, name, profileDescriptions[name])
		if i == m.Cursor {
			line = styleRenderer.NewStyle().Foreground(lipgloss.Color("6")).Render(line)
		}
		b.WriteString(line)
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(styleRenderer.NewStyle().Faint(true).Render(
		"↑/↓ mover · enter confirmar (sin cambios = balanced) · q/esc cancelar",
	))
	return b.String()
}
