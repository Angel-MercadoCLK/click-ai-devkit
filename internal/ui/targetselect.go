package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
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
	return TargetSelectModel{ClaudeFound: claudeFound, OpenClawFound: openClawFound, CodexFound: codexFound, Claude: claudeSelected, OpenClaw: openClawSelected, Codex: codexSelected}
}

func (m TargetSelectModel) Init() tea.Cmd { return nil }

func (m TargetSelectModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.Type {
	case tea.KeyUp:
		m.Cursor = (m.Cursor - 1 + 3) % 3
	case tea.KeyDown:
		m.Cursor = (m.Cursor + 1) % 3
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
			m.Cursor = (m.Cursor + 1) % 3
		case "k":
			m.Cursor = (m.Cursor - 1 + 3) % 3
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
		m.OpenClaw = !m.OpenClaw
	case 2:
		m.Codex = !m.Codex
	}
}

func (m TargetSelectModel) View() string {
	var b strings.Builder
	b.WriteString(styleRenderer.NewStyle().Bold(true).Render("Seleccione los runtimes de instalación"))
	b.WriteString("\n\n")
	b.WriteString(m.row(0, "Claude Code", m.Claude, m.ClaudeFound, "flujo completo de plugins, SDD y modelos"))
	b.WriteString("\n")
	b.WriteString(m.row(1, "OpenClaw", m.OpenClaw, m.OpenClawFound, "SDD portable, Engram, memory guard y recomendación de modelos"))
	b.WriteString("\n")
	b.WriteString(m.row(2, "Codex CLI", m.Codex, m.CodexFound, "AGENTS.md gestionado y flujo SDD portable; sin config.toml ni modelo"))
	b.WriteString("\n\nOtros runtimes: no soportados todavía; no se pueden seleccionar.\n")
	b.WriteString("\nClaude Code es el target primario · ↑/↓ mover · espacio alternar OpenClaw/Codex · enter guardar · q/esc cancelar")
	return b.String()
}

func (m TargetSelectModel) row(index int, name string, selected, detected bool, capabilities string) string {
	marker := "  "
	if index == m.Cursor {
		marker = "> "
	}
	state := "no detectado"
	if detected {
		state = "detectado"
	}
	return fmt.Sprintf("%s[%s] %s (%s)\n   Capacidades: %s", marker, boolMarker(selected), name, state, capabilities)
}

func boolMarker(value bool) string {
	if value {
		return "x"
	}
	return " "
}
