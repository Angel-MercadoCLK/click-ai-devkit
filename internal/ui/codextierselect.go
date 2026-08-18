package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/installer"
)

// codexTierNames lists the three preset tiers in display order.
var codexTierNames = []string{"low-cost", "recommended", "powerful"}

// codexTierDescriptions gives each tier a short Spanish label for the select screen (D10:
// dev-facing CLI text is Spanish). It deliberately does NOT restate specific model/effort values —
// those can drift from the canonical assignments in codexmodelprofile.go (verify caught exactly such
// a drift). The exact per-role mix is rendered separately and accurately by codexTierSummary, which
// reads the canonical tier, so these labels stay a stable, non-duplicating vibe only.
var codexTierDescriptions = map[string]string{
	"low-cost":    "máximo ahorro",
	"recommended": "equilibrado (recomendado)",
	"powerful":    "máxima calidad",
}

// CodexTierSelectModel is the bubbletea model that drives `click install`'s Codex tier picker: a
// radio-style single-pick list of the three preset tiers. It mirrors ProfileSelectModel's exact
// keymap (j/k or arrow up/down to move, enter to confirm, esc/q/ctrl+c to cancel) and styling.
type CodexTierSelectModel struct {
	Cursor    int
	Selected  string
	Confirmed bool
	Cancelled bool
}

// NewCodexTierSelectModel builds a CodexTierSelectModel with the cursor on "recommended" — the
// default tier a developer gets by just pressing enter, matching the non-interactive fallback.
func NewCodexTierSelectModel() CodexTierSelectModel {
	return CodexTierSelectModel{Cursor: 1, Selected: "recommended"}
}

// Init satisfies tea.Model. The tier select screen needs no startup command.
func (m CodexTierSelectModel) Init() tea.Cmd { return nil }

// Update satisfies tea.Model, handling only keyboard input: every other message is a no-op so the
// screen stays static under resizes, ticks, etc.
func (m CodexTierSelectModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
		m.Selected = codexTierNames[m.Cursor]
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

func (m *CodexTierSelectModel) moveCursor(delta int) {
	n := len(codexTierNames)
	m.Cursor = ((m.Cursor+delta)%n + n) % n
	m.Selected = codexTierNames[m.Cursor]
}

// View satisfies tea.Model, rendering one row per tier with a cursor marker and a short Spanish
// summary, plus a short Spanish key-help line matching ProfileSelectModel's own.
func (m CodexTierSelectModel) View() string {
	var b strings.Builder

	labels := make([]string, 0, len(codexTierNames))
	for _, name := range codexTierNames {
		labels = append(labels, string(name))
	}
	width := WidestLabel(labels)

	// The per-phase model summary goes on its own continuation line: inlined it made each row wrap
	// well past any sane terminal width.
	rows := make([]string, 0, len(codexTierNames))
	for i, name := range codexTierNames {
		rows = append(rows, Row(i == m.Cursor,
			fmt.Sprintf("%s  %s", PadLabel(string(name), width), codexTierDescriptions[name]),
			codexTierSummary(name)))
	}

	b.WriteString(Screen("NIVEL DE MODELOS PARA CODEX", rows))
	b.WriteString("\n\n")
	b.WriteString(Hints(
		"↑/↓", "mover",
		"enter", "confirmar (sin cambios = recommended)",
		"q · esc", "cancelar",
	))
	return b.String()
}

// codexTierSummary builds the Spanish role-mix summary shown for a tier row, e.g.
// "Orquestador sol/low · Razonamiento sol/medium · Código terra/medium · Liviano luna/low".
func codexTierSummary(tier string) string {
	tierData := installer.CodexModelTierByNameOrDefault(tier)
	roles := []string{"orquestador", "razonamiento", "codigo", "liviano"}
	parts := make([]string, 0, len(roles))
	for _, role := range roles {
		assignment := tierData.Roles[role]
		parts = append(parts, fmt.Sprintf("%s %s/%s", titleCase(role), assignment.Model, assignment.Effort))
	}
	return strings.Join(parts, " · ")
}

// titleCase returns the role name with its first character uppercased for display.
func titleCase(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
