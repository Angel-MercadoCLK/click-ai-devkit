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
	modelconfig.PhaseExplore:    "explore",
	modelconfig.PhasePropose:    "propose",
	modelconfig.PhaseSpec:       "spec",
	modelconfig.PhaseDesign:     "design",
	modelconfig.PhaseTasks:      "tasks",
	modelconfig.PhaseApply:      "apply",
	modelconfig.PhaseVerify:     "verify",
	modelconfig.PhaseArchive:    "archive",
	modelconfig.PhaseOnboard:    "onboard",
	modelconfig.PhaseJDJudgeA:   "jd-judge-a",
	modelconfig.PhaseJDJudgeB:   "jd-judge-b",
	modelconfig.PhaseJDFixAgent: "jd-fix-agent",
	modelconfig.PhaseDefault:    "default",
}

// ModelSelectModel is the bubbletea model that drives `click install`'s interactive per-phase
// model selection screen (D25): one row per click-sdd SDD phase, defaults pre-selected. Arrow
// keys (or j/k) move the cursor between rows; left/right (or h/l) cycle the highlighted row's
// model through modelconfig.Models. Pressing enter immediately — with no edits — confirms all
// defaults in one key, matching the "accept all defaults with one key" requirement; pressing
// enter after edits confirms whatever is currently selected. Esc/q/ctrl+c cancels the install.
type ModelSelectModel struct {
	Cursor    int
	Selection map[modelconfig.Phase]string
	Confirmed bool
	Cancelled bool
}

// NewModelSelectModel builds a ModelSelectModel with every phase pre-selected to its D25 default.
func NewModelSelectModel() ModelSelectModel {
	defaults := modelconfig.Defaults()
	selection := make(map[modelconfig.Phase]string, len(defaults))
	for phase, model := range defaults {
		selection[phase] = model
	}
	return ModelSelectModel{Selection: selection}
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
	n := len(modelconfig.Phases)
	m.Cursor = ((m.Cursor+delta)%n + n) % n
}

func (m *ModelSelectModel) cycleCurrent(delta int) {
	phase := modelconfig.Phases[m.Cursor]
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

// View satisfies tea.Model, rendering one row per phase with a cursor marker and the currently
// selected model, plus a short Spanish key-help line (D10: dev-facing CLI text may be Spanish).
func (m ModelSelectModel) View() string {
	var b strings.Builder
	b.WriteString(styleRenderer.NewStyle().Bold(true).Render("Elegí el modelo por fase para click-sdd"))
	b.WriteString("\n\n")

	for i, phase := range modelconfig.Phases {
		marker := "  "
		if i == m.Cursor {
			marker = "> "
		}
		line := fmt.Sprintf("%s%-16s %s", marker, phaseLabels[phase], m.Selection[phase])
		if i == m.Cursor {
			line = styleRenderer.NewStyle().Foreground(lipgloss.Color("6")).Render(line)
		}
		b.WriteString(line)
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(styleRenderer.NewStyle().Faint(true).Render(
		"↑/↓ mover · ←/→ cambiar modelo · enter confirmar (sin cambios = defaults) · q/esc cancelar",
	))
	return b.String()
}
