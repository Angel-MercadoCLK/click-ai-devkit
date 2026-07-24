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

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/ui"
	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/version"
)

// Action keys identify what a selected active item should do once the menu program exits.
// Inert (coming-soon) items carry no Action — selecting one never sets Model.Chosen.
const (
	ActionInstall                = "install"
	ActionUpdate                 = "update"
	ActionConfigureModels        = "configure-models"
	ActionAgentBuilder           = "agent-builder"
	ActionDoctor                 = "doctor"
	ActionTargets                = "targets"
	ActionConfigureTargets       = "configure-targets"
	ActionConfigureOpenClawModel = "configure-openclaw-model"
	ActionPlugins                = "plugins"
	ActionUninstall              = "uninstall"
	ActionManageBackups          = "manage-backups"
	ActionRollback               = "rollback"
	ActionQuit                   = "quit"
)

// Item is one row of the menu: a label, an optional dispatch Action, and whether it's active
// (dispatchable) or an inert "coming soon" placeholder.
type Item struct {
	Label         string
	Action        string
	Active        bool
	Group         string
	Hint          string
	InactiveLabel string
}

// Items is the fixed, ordered menu item list. Every current row is active — the two remaining
// inert "coming soon" placeholders ("Presets de instalación", "Sincronizar configuración") were
// removed, and "Gestionar backups" was activated, per the neutral-spanish-and-backups change. The
// inert-item mechanism itself (Item.Active, selectCurrent's comingSoonMsg branch, View's
// "(próximamente)" suffix) is kept intact for any future placeholder row. j/k cursor movement
// treats every row the same way — including inert ones, if any are ever added back — so cursor
// math never needs to skip rows.
var Items = []Item{
	{Label: "Iniciar instalación", Action: ActionInstall, Active: true, Group: "Instalación y mantenimiento"},
	{Label: "Actualizar herramientas", Action: ActionUpdate, Active: true, Group: "Instalación y mantenimiento"},
	{Label: "Desinstalar", Action: ActionUninstall, Active: true, Group: "Instalación y mantenimiento"},
	{Label: "Gestionar backups", Action: ActionManageBackups, Active: true, Group: "Instalación y mantenimiento"},
	{Label: "Restaurar último respaldo", Action: ActionRollback, Active: true, Group: "Instalación y mantenimiento"},
	{Label: "Configurar modelos", Action: ActionConfigureModels, Active: true, Group: "Configuración"},
	{Label: "Configurar runtimes", Action: ActionConfigureTargets, Active: true, Group: "Configuración"},
	{Label: "Configurar modelo nativo de OpenClaw", Action: ActionConfigureOpenClawModel, Active: true, Group: "Configuración"},
	{Label: "Plugins", Action: ActionPlugins, Active: true, Group: "Configuración"},
	{Label: "Ejecutar diagnóstico", Action: ActionDoctor, Active: true, Group: "Diagnóstico y soporte"},
	{Label: "Detectar runtimes compatibles", Action: ActionTargets, Active: true, Group: "Diagnóstico y soporte"},
	{Label: "Crear agente propio", Action: ActionAgentBuilder, Active: true, Group: "Diagnóstico y soporte"},
	{Label: "Salir", Action: ActionQuit, Active: true, Group: "Diagnóstico y soporte"},
}

// comingSoonMsg is the transient status line shown when Enter is pressed on an inert item.
const comingSoonMsg = "próximamente — todavía no implementado"

// --- branded header + menu theming (per the approved menu redesign) ---

// wordmarkRamp colors the CLICK-AI wordmark top-to-bottom, white fading into orange — one color
// per art line. This is the menu's own palette, deliberately distinct from the install banner's
// cyan→blue ramp in internal/ui; both share the same raw glyphs via ui.BannerArt().
var wordmarkRamp = []string{"#ffffff", "#ffe2c4", "#ffc48c", "#ffa654", "#ff8c2e", "#ff7a1a"}

const (
	menuOrange = "#ff8c2e" // accent: spark core, active row, pointer, footer keys
	menuRay    = "#b5701f" // spark rays
	menuWhite  = "#ffffff" // spark companion star
	menuDust   = "#6b5138" // spark dust dot
	menuLabel  = "#d3dae2" // inactive item label
	menuNum    = "#4b5663" // item number
	menuBorder = "#3d4753" // menu box border
	menuTitle  = "#b98a5a" // "MENÚ" caption
	menuDim    = "#5a6470" // tagline, footer text, coming-soon notice
)

// sparkLogo is the AI-spark brand mark shown to the left of the wordmark: a four-point star with
// its burst rays, a small companion star, and a dust dot. Rendered in renderSpark with color.
var sparkLogo = struct{ star2, rayTop, rayMidL, star1, rayMidR, rayBot, dust string }{
	star2:   "        ✧",
	rayTop:  "      ╲ │ ╱",
	rayMidL: "     ─  ",
	star1:   "✦",
	rayMidR: "  ─",
	rayBot:  "      ╱ │ ╲",
	dust:    "        ·",
}

// Model is the bubbletea model driving the standing menu.
type Model struct {
	Items []Item
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
	return NewModelWithItems(DefaultItems())
}

func NewModelWithItems(items []Item) Model {
	if len(items) == 0 {
		items = DefaultItems()
	}
	return Model{Items: items}
}

func DefaultItems() []Item {
	return append([]Item(nil), Items...)
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
	n := len(m.Items)
	m.Cursor = ((m.Cursor+delta)%n + n) % n
	m.StatusMsg = ""
}

// selectCurrent handles Enter on the highlighted row: active items record their Action and quit
// the program; inert items show a transient status message and stay in the menu.
func (m Model) selectCurrent() (tea.Model, tea.Cmd) {
	item := m.Items[m.Cursor]
	if !item.Active {
		if item.Hint != "" {
			m.StatusMsg = item.Hint
		} else {
			m.StatusMsg = comingSoonMsg
		}
		return m, nil
	}
	m.Chosen = item.Action
	return m, tea.Quit
}

// renderSpark renders the AI-spark logo block (5 lines), colored: white companion star, orange
// core with dim-orange burst rays, and a dust dot.
func renderSpark() string {
	white := lipgloss.NewStyle().Foreground(lipgloss.Color(menuWhite))
	ray := lipgloss.NewStyle().Foreground(lipgloss.Color(menuRay))
	star := lipgloss.NewStyle().Foreground(lipgloss.Color(menuOrange)).Bold(true)
	dust := lipgloss.NewStyle().Foreground(lipgloss.Color(menuDust))
	return strings.Join([]string{
		white.Render(sparkLogo.star2),
		ray.Render(sparkLogo.rayTop),
		ray.Render(sparkLogo.rayMidL) + star.Render(sparkLogo.star1) + ray.Render(sparkLogo.rayMidR),
		ray.Render(sparkLogo.rayBot),
		dust.Render(sparkLogo.dust),
	}, "\n")
}

// renderWordmark renders the CLICK-AI wordmark in the white→orange ramp, one color per art line,
// reusing the shared raw glyphs from internal/ui.
func renderWordmark() string {
	lines := strings.Split(ui.BannerArt(), "\n")
	out := make([]string, len(lines))
	for i, line := range lines {
		color := wordmarkRamp[i%len(wordmarkRamp)]
		out[i] = lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Render(line)
	}
	return strings.Join(out, "\n")
}

// renderHeader joins the spark logo and the wordmark side by side (vertically centered) and adds
// the dim tagline with the live build version below.
func renderHeader() string {
	brand := lipgloss.JoinHorizontal(lipgloss.Center, renderSpark(), "   ", renderWordmark())
	tagline := "AI Devkit · Click Seguros · " + version.Version
	tag := lipgloss.NewStyle().Foreground(lipgloss.Color(menuDim)).Render(tagline)
	return brand + "\n\n" + tag
}

// renderRow renders one menu row: the active-row pointer (▸ under the cursor, blank otherwise),
// a dim item number, and the label. The active row's label is orange/bold; inert rows are dim
// and carry the "(próximamente)" suffix.
func (m Model) renderRow(index int, item Item) string {
	pointer := " "
	if index == m.Cursor {
		pointer = "▸"
	}
	pointerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(menuOrange)).Bold(true)
	numStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(menuNum))

	label := item.Label
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(menuLabel))
	switch {
	case !item.Active:
		suffix := item.InactiveLabel
		if suffix == "" {
			suffix = "próximamente"
		}
		label += " (" + suffix + ")"
		labelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(menuDim)).Faint(true)
	case index == m.Cursor:
		labelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(menuOrange)).Bold(true)
	}

	return fmt.Sprintf("%s %s  %s",
		pointerStyle.Render(pointer),
		numStyle.Render(fmt.Sprintf("%d", index+1)),
		labelStyle.Render(label),
	)
}

// renderMenu renders the "MENÚ" caption and the rounded box that frames every item row.
func (m Model) renderMenu() string {
	rows := make([]string, 0, len(m.Items)+4)
	currentGroup := ""
	groupStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(menuTitle)).Bold(true)
	for i, item := range m.Items {
		if item.Group != "" && item.Group != currentGroup {
			if len(rows) > 0 {
				rows = append(rows, "")
			}
			rows = append(rows, groupStyle.Render(item.Group))
			currentGroup = item.Group
		}
		rows = append(rows, m.renderRow(i, item))
	}
	title := lipgloss.NewStyle().Foreground(lipgloss.Color(menuTitle)).Render("MENÚ")
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(menuBorder)).
		Padding(0, 3).
		Render(strings.Join(rows, "\n"))
	return title + "\n" + box
}

// renderFooter renders the dim key-help line with the accent-colored key chords.
func renderFooter() string {
	key := lipgloss.NewStyle().Foreground(lipgloss.Color(menuOrange)).Bold(true)
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color(menuDim))
	return dim.Render("  ") + key.Render("↑/↓ · j/k") + dim.Render(" mover    ") +
		key.Render("enter") + dim.Render(" elegir    ") +
		key.Render("q · esc") + dim.Render(" salir")
}

// View satisfies tea.Model, rendering the branded header (spark logo + wordmark + tagline), the
// boxed item list with the active row highlighted, an optional transient status line, and the
// key-help footer.
func (m Model) View() string {
	var b strings.Builder
	b.WriteString(renderHeader())
	b.WriteString("\n\n")
	b.WriteString(m.renderMenu())
	if m.StatusMsg != "" {
		b.WriteString("\n\n")
		b.WriteString(lipgloss.NewStyle().Faint(true).Render(m.StatusMsg))
	}
	b.WriteString("\n\n")
	b.WriteString(renderFooter())
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
	case ActionTargets:
		return []string{"targets"}
	case ActionConfigureTargets:
		return []string{"configure-targets"}
	case ActionConfigureOpenClawModel:
		return []string{"configure-openclaw-model"}
	case ActionPlugins:
		return []string{"plugins"}
	case ActionUninstall:
		return []string{"uninstall"}
	case ActionManageBackups:
		return []string{"manage-backups"}
	case ActionRollback:
		return []string{"rollback"}
	default:
		return nil
	}
}
