// Package menu implements click's standing interactive menu: the bubbletea program launched
// when `click` runs with no subcommand on a TTY (see internal/cli's default root action). It is
// deliberately separate from internal/ui, which hosts one-shot, value-returning screens (the
// install-time model-selection TUI) — menu hosts a persistent, navigable program with its own
// item list, cursor, and dispatch contract.
//
// Per the interactive-menu-and-model-taxonomy design: selecting an active item never runs the
// underlying command from inside Update. Update only records the chosen action on the model and
// quits the program; the caller (internal/cli) reads the final Model after the bubbletea program
// returns and maps Chosen to CLI args via ActionArgs, then dispatches through a fresh cobra
// command tree. This keeps Update pure and unit-testable without a real bubbletea program.
package menu

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Action keys identify what a selected active item should do once the menu program exits.
// Inert (coming-soon) items carry no Action — selecting one never sets Model.Chosen.
const (
	ActionInstall         = "install"
	ActionUpdate          = "update"
	ActionConfigureModels = "configure-models"
	ActionAgentBuilder    = "agent-builder"
	ActionDoctor          = "doctor"
	ActionUninstall       = "uninstall"
	ActionQuit            = "quit"
)

// Item is one row of the menu: a label, an optional dispatch Action, and whether it's active
// (dispatchable) or an inert "coming soon" placeholder.
type Item struct {
	Label  string
	Action string
	Active bool
}

// Items is the fixed, ordered menu item list: active items first (they dispatch to real click
// subcommands or exit the menu), then the inert ecosystem-parity placeholders, per the approved
// proposal's menu shape. j/k cursor movement treats every row the same way — including inert
// ones — so cursor math never needs to skip rows.
var Items = []Item{
	{Label: "Iniciar instalación", Action: ActionInstall, Active: true},
	{Label: "Actualizar herramientas", Action: ActionUpdate, Active: true},
	{Label: "Configurar modelos", Action: ActionConfigureModels, Active: true},
	{Label: "Ejecutar diagnóstico", Action: ActionDoctor, Active: true},
	{Label: "Desinstalar", Action: ActionUninstall, Active: true},
	{Label: "Presets de instalación", Active: false},
	{Label: "Crear tu propio agente", Action: ActionAgentBuilder, Active: true},
	{Label: "Sincronizar configuración", Active: false},
	{Label: "Gestionar backups", Active: false},
	{Label: "Salir", Action: ActionQuit, Active: true},
}

// headerVersion is a static placeholder for the menu's updates-indicator header line. Real
// update-availability checking is out of scope for this change (no network/version-compare
// logic was specified) — this line only establishes the header's structural position for a
// future task to wire real data into. See the apply-progress note for this simplification.
const headerVersion = "click-ai-devkit (actualizaciones: sin verificar)"

// comingSoonMsg is the transient status line shown when Enter is pressed on an inert item.
const comingSoonMsg = "próximamente — todavía no implementado"

// Model is the bubbletea model driving the standing menu.
type Model struct {
	// Cursor is the index into Items currently highlighted.
	Cursor int
	// Chosen is set to the selected active item's Action once Update decides to quit the
	// program (Enter on an active item, or q/esc/ctrl+c which set ActionQuit). Empty means no
	// selection was made yet.
	Chosen string
	// StatusMsg is a transient message shown below the item list — currently only used for the
	// inert-item "coming soon" notice. It is cleared on the next cursor movement.
	StatusMsg string
}

// NewModel builds a fresh Model with the cursor on the first item and no selection made.
func NewModel() Model {
	return Model{}
}

// Init satisfies tea.Model. The menu needs no startup command.
func (m Model) Init() tea.Cmd { return nil }

// Update satisfies tea.Model, handling only keyboard input: every other message is a no-op so
// the menu stays static under resizes, ticks, etc.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
		return m.selectCurrent()
	case tea.KeyEsc, tea.KeyCtrlC:
		m.Chosen = ActionQuit
		return m, tea.Quit
	case tea.KeyRunes:
		switch string(keyMsg.Runes) {
		case "k":
			m.moveCursor(-1)
		case "j":
			m.moveCursor(1)
		case "q":
			m.Chosen = ActionQuit
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m *Model) moveCursor(delta int) {
	n := len(Items)
	m.Cursor = ((m.Cursor+delta)%n + n) % n
	m.StatusMsg = ""
}

// selectCurrent handles Enter on the highlighted row: active items record their Action and quit
// the program; inert items show a transient status message and stay in the menu.
func (m Model) selectCurrent() (tea.Model, tea.Cmd) {
	item := Items[m.Cursor]
	if !item.Active {
		m.StatusMsg = comingSoonMsg
		return m, nil
	}
	m.Chosen = item.Action
	return m, tea.Quit
}

// View satisfies tea.Model, rendering the header, one row per item (dimmed + "(próximamente)" for
// inert ones), and the transient status line if set.
func (m Model) View() string {
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Bold(true).Render(headerVersion))
	b.WriteString("\n\n")

	for i, item := range Items {
		marker := "  "
		if i == m.Cursor {
			marker = "> "
		}
		label := item.Label
		if !item.Active {
			label = fmt.Sprintf("%s (próximamente)", label)
		}
		line := marker + label

		switch {
		case !item.Active:
			line = lipgloss.NewStyle().Faint(true).Render(line)
		case i == m.Cursor:
			line = lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Render(line)
		}
		b.WriteString(line)
		b.WriteString("\n")
	}

	if m.StatusMsg != "" {
		b.WriteString("\n")
		b.WriteString(lipgloss.NewStyle().Faint(true).Render(m.StatusMsg))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Faint(true).Render(
		"j/k mover · enter seleccionar · q/esc salir",
	))
	return b.String()
}

// ActionArgs maps a chosen active item's Action to the []string args that should be passed to a
// fresh cli.NewRootCommand().SetArgs(args).Execute() call. ActionQuit, the empty action (no
// selection made), and any unrecognized action all return nil — the caller treats a nil/empty
// result as "nothing to dispatch, exit cleanly."
func ActionArgs(action string) []string {
	switch action {
	case ActionInstall:
		return []string{"install"}
	case ActionUpdate:
		return []string{"update"}
	case ActionConfigureModels:
		return []string{"configure-models"}
	case ActionAgentBuilder:
		return []string{"agent-builder"}
	case ActionDoctor:
		return []string{"doctor"}
	case ActionUninstall:
		return []string{"uninstall"}
	default:
		return nil
	}
}
