package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/modelconfig"
)

// phaseLabels gives each modelconfig.Phase a short, human-readable row label for the model
// selection screen. Kept here (not in modelconfig) because it's presentation, not domain data.
var phaseLabels = map[modelconfig.Phase]string{
	modelconfig.PhaseOrchestrator:  "orchestrator",
	modelconfig.PhasePRDWriter:     "prd_writer",
	modelconfig.PhaseArchitect:     "architect",
	modelconfig.PhaseReviewer:      "reviewer",
	modelconfig.PhaseMemoryCurator: "memory_curator",
}

// ModelSelectModel is the bubbletea model that drives `click install`'s interactive profile-aware
// click-sdd configuration screen: the profile row is first, then one row per phase agent with the
// active profile's defaults pre-selected. Arrow keys (or j/k) move the cursor between rows;
// left/right (or h/l) cycles profiles on the profile row and models on phase rows. Pressing enter
// immediately confirms the active profile defaults; pressing enter after edits confirms the current
// profile plus per-phase overrides. Esc/q/ctrl+c cancels the install.
type ModelSelectModel struct {
	Cursor    int
	Profile   modelconfig.RuntimeProfile
	Selection map[modelconfig.Phase]string
	Confirmed bool
	Cancelled bool
}

// NewModelSelectModel builds a ModelSelectModel with the built-in default profile selected and every
// phase pre-selected from that profile before any per-phase adjustments.
func NewModelSelectModel() ModelSelectModel {
	profile := modelconfig.ResolveProfile("")
	selection := make(map[modelconfig.Phase]string, len(profile.Models))
	for phase, model := range profile.Models {
		selection[phase] = model
	}
	return ModelSelectModel{Profile: profile, Selection: selection}
}

// Init satisfies tea.Model. The selection screen needs no startup command.
func (m ModelSelectModel) Init() tea.Cmd { return nil }

// Update satisfies tea.Model, handling only keyboard input: every other message is a no-op so the
// screen stays static under resizes, ticks, etc.
func (m ModelSelectModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	switch keyMsg.Type {
	case tea.KeyUp:
		m.moveCursor(-1)
	case tea.KeyDown:
		m.moveCursor(1)
	case tea.KeyLeft:
		m.cycleCurrent(-1)
	case tea.KeyRight:
		m.cycleCurrent(1)
	case tea.KeyEnter:
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
		case "h":
			m.cycleCurrent(-1)
		case "l":
			m.cycleCurrent(1)
		case "q":
			m.Cancelled = true
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m *ModelSelectModel) moveCursor(delta int) {
	n := len(modelconfig.Phases) + 1
	m.Cursor = ((m.Cursor+delta)%n + n) % n
}

func (m *ModelSelectModel) cycleCurrent(delta int) {
	if m.Cursor == 0 {
		m.cycleProfile(delta)
		return
	}
	phase := modelconfig.Phases[m.Cursor-1]
	n := len(modelconfig.Models)
	idx := 0
	for i, model := range modelconfig.Models {
		if model == m.Selection[phase] {
			idx = i
			break
		}
	}
	idx = ((idx+delta)%n + n) % n
	m.Selection[phase] = modelconfig.Models[idx]
}

func (m *ModelSelectModel) cycleProfile(delta int) {
	profiles := modelconfig.Profiles()
	if len(profiles) < 2 {
		return
	}
	idx := 0
	for i, profile := range profiles {
		if profile.Name == m.Profile.Name {
			idx = i
			break
		}
	}
	idx = ((idx+delta)%len(profiles) + len(profiles)) % len(profiles)
	m.Profile = profiles[idx]
	m.Selection = modelconfig.ResolveForProfile(m.Profile, nil)
}

// View satisfies tea.Model, rendering one row per phase with a cursor marker and the currently
// selected model, plus a short Spanish key-help line (D10: dev-facing CLI text may be Spanish).
func (m ModelSelectModel) View() string {
	var b strings.Builder
	b.WriteString(styleRenderer.NewStyle().Bold(true).Render("Elegí el perfil y los modelos para click-sdd"))
	b.WriteString("\n\n")

	profileLine := fmt.Sprintf("%s%-24s %s", markerForCursor(m.Cursor, 0), "Perfil de orquestación", m.Profile.Name)
	if m.Cursor == 0 {
		profileLine = styleRenderer.NewStyle().Foreground(lipgloss.Color("6")).Render(profileLine)
	}
	b.WriteString(profileLine)
	b.WriteString("\n")

	for i, phase := range modelconfig.Phases {
		cursor := i + 1
		line := fmt.Sprintf("%s%-24s %s", markerForCursor(m.Cursor, cursor), phaseLabels[phase], m.Selection[phase])
		if cursor == m.Cursor {
			line = styleRenderer.NewStyle().Foreground(lipgloss.Color("6")).Render(line)
		}
		b.WriteString(line)
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(styleRenderer.NewStyle().Faint(true).Render(
		"↑/↓ mover · ←/→ cambiar perfil/modelo · enter confirmar (sin cambios = defaults) · q/esc cancelar",
	))
	return b.String()
}

func markerForCursor(current, row int) string {
	if current == row {
		return "> "
	}
	return "  "
}
