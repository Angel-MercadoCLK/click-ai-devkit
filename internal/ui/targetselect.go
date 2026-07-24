package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// TargetSelectModel is the small multi-select screen for Click-managed runtimes.
type TargetSelectModel struct {
	Cursor        int
	Claude        bool
	OpenClaw      bool
	ClaudeFound   bool
	OpenClawFound bool
	Codex         bool
	CodexFound    bool
	Confirmed     bool
	Cancelled     bool
}

func NewTargetSelectModel(claudeFound, openClawFound, claudeSelected, openClawSelected bool, codex ...bool) TargetSelectModel {
	codexFound, codexSelected := false, false
	if len(codex) > 0 {
		codexFound = codex[0]
	}
	if len(codex) > 1 {
		codexSelected = codex[1]
	}
	return TargetSelectModel{ClaudeFound: claudeFound, OpenClawFound: openClawFound, CodexFound: codexFound, Claude: claudeFound && claudeSelected, OpenClaw: openClawFound && openClawSelected, Codex: codexFound && codexSelected}
}

func (m TargetSelectModel) Init() tea.Cmd { return nil }

func (m TargetSelectModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.Type {
	case tea.KeyUp:
		m.moveCursor(-1)
	case tea.KeyDown:
		m.moveCursor(1)
	case tea.KeySpace:
		m.toggleCurrent()
	case tea.KeyEnter:
		m.Confirmed = true
		return m, tea.Quit
	case tea.KeyEsc, tea.KeyCtrlC:
		m.Cancelled = true
		return m, tea.Quit
	case tea.KeyRunes:
		switch string(key.Runes) {
		case "j":
			m.moveCursor(1)
		case "k":
			m.moveCursor(-1)
		case " ":
			m.toggleCurrent()
		case "q":
			m.Cancelled = true
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m *TargetSelectModel) toggleCurrent() {
	switch m.Cursor {
	case 1:
		if m.OpenClawFound {
			m.OpenClaw = !m.OpenClaw
		}
	case 2:
		if m.CodexFound {
			m.Codex = !m.Codex
		}
	case 0:
		if m.ClaudeFound {
			m.Claude = !m.Claude
		}
	}
}

func (m *TargetSelectModel) moveCursor(delta int) {
	for i := 0; i < 3; i++ {
		m.Cursor = (m.Cursor + delta + 3) % 3
		if (m.Cursor == 0 && m.ClaudeFound) || (m.Cursor == 1 && m.OpenClawFound) || (m.Cursor == 2 && m.CodexFound) {
			return
		}
	}
}

func (m TargetSelectModel) View() string {
	var b strings.Builder
	b.WriteString(styleRenderer.NewStyle().Bold(true).Render("Seleccione los runtimes de instalación"))
	b.WriteString("\n\n")
	rows := []string{}
	if m.ClaudeFound {
		rows = append(rows, m.row(0, "Claude Code", m.Claude, true, "plugins nativos, SDD y modelos por fase"))
	}
	if m.OpenClawFound {
		rows = append(rows, m.row(1, "OpenClaw", m.OpenClaw, true, "SDD portable, Engram, memory guard y modelo nativo"))
	}
	if m.CodexFound {
		rows = append(rows, m.row(2, "Codex CLI", m.Codex, true, "AGENTS.md gestionado y modelo nativo de config.toml"))
	}
	b.WriteString(strings.Join(rows, "\n"))
	b.WriteString("\n\nOtros runtimes: no soportados todavía; no se pueden seleccionar.\n")
	b.WriteString("\n↑/↓ mover · espacio alternar · enter continuar · q/esc cancelar")
	return b.String()
}

func (m TargetSelectModel) row(index int, name string, selected, detected bool, capabilities string) string {
	marker := "  "
	cursor := index == m.Cursor
	if cursor {
		marker = "> "
	}
	state := "no detectado"
	if detected {
		state = "detectado"
	}
	content := fmt.Sprintf("%s[%s] %s (%s)\n   Capacidades: %s", marker, boolMarker(selected), name, state, capabilities)
	if cursor {
		// Real visual weight for the cursor row: the existing cyan (6) foreground role, plus a
		// complementing blue (4) background — both already this package's own established colors
		// (see renderer.go's Step/Info roles), no new hex/color invented. Kept consistent with
		// ProfileSelectModel/ModelSelectModel's own cursor-row styling (design consistency across
		// the screens composed together in InstallWizardModel).
		return styleRenderer.NewStyle().Foreground(lipgloss.Color("6")).Background(lipgloss.Color("4")).Bold(true).Render(content)
	}
	// Dim every non-selected row so the cursor row reads with real contrast instead of flat,
	// same-weight text — reuses the Faint(true) convention already used for help text.
	return styleRenderer.NewStyle().Faint(true).Render(content)
}

func boolMarker(value bool) string {
	if value {
		return "x"
	}
	return " "
}
