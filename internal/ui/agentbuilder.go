package ui

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/agentbuilder"
	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/modelconfig"
)

type Step int

const (
	StepEngine Step = iota
	StepDescription
	StepSDDMode
	StepPhase
	StepThemes
	StepPreview
	StepPlacement
	StepDone
)

type AgentBuilderModel struct {
	Step           Step
	Spec           agentbuilder.AgentSpec
	ThemeIndex     int
	PreviewContent string
	PreviewError   string
	EditingPreview bool
	Confirmed      bool
	Cancelled      bool

	engines []agentbuilder.Engine
	cursor  int
	input   string
}

var agentBuilderSDDModeOptions = []struct {
	label string
	mode  agentbuilder.SDDMode
}{
	{label: "Standalone", mode: agentbuilder.SDDStandalone},
	{label: "Phase Support", mode: agentbuilder.SDDPhaseSupport},
}

var agentBuilderThemePrompts = []struct {
	title string
	apply func(*agentbuilder.AgentSpec, string)
}{
	{title: "Propósito / objetivo", apply: func(spec *agentbuilder.AgentSpec, value string) { spec.Purpose = value }},
	{title: "Tareas exactas", apply: func(spec *agentbuilder.AgentSpec, value string) { spec.Tasks = value }},
	{title: "Situaciones o frases que lo activan", apply: func(spec *agentbuilder.AgentSpec, value string) { spec.Triggers = value }},
	{title: "Reglas duras / restricciones", apply: func(spec *agentbuilder.AgentSpec, value string) { spec.Rules = value }},
	{title: "Herramientas necesarias", apply: func(spec *agentbuilder.AgentSpec, value string) { spec.Tools = value }},
	{title: "Modelo", apply: func(spec *agentbuilder.AgentSpec, value string) { spec.Model = value }},
	{title: "Tono / persona", apply: func(spec *agentbuilder.AgentSpec, value string) { spec.Tone = value }},
	{title: "Conocimiento de dominio", apply: func(spec *agentbuilder.AgentSpec, value string) { spec.Domain = value }},
	{title: "Ejemplo de buen resultado", apply: func(spec *agentbuilder.AgentSpec, value string) { spec.GoodOutput = value }},
}

var agentBuilderPreviewActions = []string{"Instalar", "Editar", "Regenerar", "Volver"}

var agentBuilderGeneratedNamePattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$`)

var agentBuilderPlacementOptions = []struct {
	label     string
	placement agentbuilder.Placement
}{
	{label: "Personal", placement: agentbuilder.PlacementPersonal},
	{label: "Shareable", placement: agentbuilder.PlacementShareable},
}

func NewAgentBuilderModel(engines []agentbuilder.Engine) AgentBuilderModel {
	if len(engines) == 0 {
		engines = agentbuilder.Engines()
	}
	m := AgentBuilderModel{engines: append([]agentbuilder.Engine(nil), engines...)}
	if len(m.engines) == 1 {
		m.Spec.Engine = m.engines[0]
		m.Step = StepDescription
		return m
	}
	m.Step = StepEngine
	return m
}

func (m AgentBuilderModel) Init() tea.Cmd { return nil }

func (m AgentBuilderModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	if m.EditingPreview || m.Step == StepDescription || m.Step == StepThemes {
		if isAgentBuilderHardCancelKey(keyMsg) {
			m.Cancelled = true
			return m, tea.Quit
		}
		if m.EditingPreview {
			return m.updatePreviewEdit(keyMsg)
		}
	} else if isAgentBuilderCancelKey(keyMsg) {
		m.Cancelled = true
		return m, tea.Quit
	}

	switch m.Step {
	case StepEngine:
		return m.updateEngine(keyMsg)
	case StepDescription:
		return m.updateDescription(keyMsg)
	case StepSDDMode:
		return m.updateSDDMode(keyMsg)
	case StepPhase:
		return m.updatePhase(keyMsg)
	case StepThemes:
		return m.updateThemes(keyMsg)
	case StepPreview:
		return m.updatePreview(keyMsg)
	case StepPlacement:
		return m.updatePlacement(keyMsg)
	default:
		return m, nil
	}
}

func (m AgentBuilderModel) updateEngine(keyMsg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch keyName(keyMsg) {
	case "up", "k":
		m.moveCursor(-1, len(m.engines))
	case "down", "j":
		m.moveCursor(1, len(m.engines))
	case "enter":
		m.Spec.Engine = m.engines[m.cursor]
		m.Step = StepDescription
		m.cursor = 0
	}
	return m, nil
}

func (m AgentBuilderModel) updateDescription(keyMsg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if keyName(keyMsg) == "enter" {
		description := strings.TrimSpace(m.input)
		name := deriveAgentName(description)
		if err := validateAgentBuilderGeneratedName(name); err != nil {
			m.PreviewError = err.Error()
			return m, nil
		}
		m.Spec.Description = description
		m.Spec.Name = name
		m.input = ""
		m.PreviewError = ""
		m.Step = StepSDDMode
		m.cursor = 0
		return m, nil
	}
	m.updateInput(keyMsg)
	return m, nil
}

func (m AgentBuilderModel) updateSDDMode(keyMsg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch keyName(keyMsg) {
	case "up", "k":
		m.moveCursor(-1, len(agentBuilderSDDModeOptions))
	case "down", "j":
		m.moveCursor(1, len(agentBuilderSDDModeOptions))
	case "enter":
		m.Spec.SDDMode = agentBuilderSDDModeOptions[m.cursor].mode
		m.cursor = 0
		if m.Spec.SDDMode == agentbuilder.SDDPhaseSupport {
			m.Step = StepPhase
			return m, nil
		}
		m.Step = StepThemes
	}
	return m, nil
}

func (m AgentBuilderModel) updatePhase(keyMsg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch keyName(keyMsg) {
	case "up", "k":
		m.moveCursor(-1, len(modelconfig.Phases))
	case "down", "j":
		m.moveCursor(1, len(modelconfig.Phases))
	case "enter":
		m.Spec.Phase = modelconfig.Phases[m.cursor]
		m.cursor = 0
		m.Step = StepThemes
	}
	return m, nil
}

func (m AgentBuilderModel) updateThemes(keyMsg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if keyName(keyMsg) == "enter" {
		answer := strings.TrimSpace(m.input)
		if err := validateAgentBuilderThemeAnswer(m.ThemeIndex, answer); err != nil {
			m.PreviewError = err.Error()
			return m, nil
		}
		agentBuilderThemePrompts[m.ThemeIndex].apply(&m.Spec, answer)
		m.input = ""
		m.PreviewError = ""
		if m.ThemeIndex == len(agentBuilderThemePrompts)-1 {
			m.refreshPreview()
			m.Step = StepPreview
			m.cursor = 0
			return m, nil
		}
		m.ThemeIndex++
		return m, nil
	}
	m.updateInput(keyMsg)
	return m, nil
}

func (m AgentBuilderModel) updatePreview(keyMsg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch keyName(keyMsg) {
	case "up", "k":
		m.moveCursor(-1, len(agentBuilderPreviewActions))
	case "down", "j":
		m.moveCursor(1, len(agentBuilderPreviewActions))
	case "enter":
		switch agentBuilderPreviewActions[m.cursor] {
		case "Instalar":
			if err := m.validatePreviewSpec(); err != nil {
				return m, nil
			}
			m.Step = StepPlacement
			m.cursor = 0
		case "Editar":
			m.EditingPreview = true
			m.input = m.PreviewContent
		case "Regenerar":
			m.refreshPreview()
		case "Volver":
			m.Step = StepThemes
			m.ThemeIndex = len(agentBuilderThemePrompts) - 1
			m.input = m.Spec.GoodOutput
		}
	}
	return m, nil
}

func (m AgentBuilderModel) updatePreviewEdit(keyMsg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if keyName(keyMsg) == "enter" {
		m.PreviewContent = m.input
		m.input = ""
		m.EditingPreview = false
		return m, nil
	}
	m.updateInput(keyMsg)
	return m, nil
}

func (m AgentBuilderModel) updatePlacement(keyMsg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch keyName(keyMsg) {
	case "up", "k":
		m.moveCursor(-1, len(agentBuilderPlacementOptions))
	case "down", "j":
		m.moveCursor(1, len(agentBuilderPlacementOptions))
	case "enter":
		if err := m.validatePreviewSpec(); err != nil {
			m.Step = StepPreview
			m.cursor = 0
			return m, nil
		}
		m.Spec.Placement = agentBuilderPlacementOptions[m.cursor].placement
		m.Confirmed = true
		m.Step = StepDone
		return m, tea.Quit
	}
	return m, nil
}

func (m AgentBuilderModel) View() string {
	switch m.Step {
	case StepEngine:
		return m.renderList("Elegí el motor del agente", engineLabels(m.engines), "↑/↓ mover · enter confirmar · q/esc cancelar")
	case StepDescription:
		return renderInputWithError("Describí el agente que querés crear", m.input, m.PreviewError)
	case StepSDDMode:
		return m.renderList("Elegí la integración SDD", sddModeLabels(), "↑/↓ mover · enter confirmar · q/esc cancelar")
	case StepPhase:
		return m.renderList("Elegí la fase SDD que este agente va a apoyar", phaseLabelsForAgentBuilder(), "↑/↓ mover · enter confirmar · q/esc cancelar")
	case StepThemes:
		prompt := agentBuilderThemePrompts[m.ThemeIndex]
		return renderInputWithError(fmt.Sprintf("%d/9 · %s", m.ThemeIndex+1, prompt.title), m.input, m.PreviewError)
	case StepPreview:
		if m.EditingPreview {
			return renderInput("Editá el Markdown final", m.input)
		}
		return m.renderPreview()
	case StepPlacement:
		return m.renderList("Elegí dónde instalar el agente", placementLabels(), "↑/↓ mover · enter instalar · q/esc cancelar")
	case StepDone:
		return "Agente confirmado"
	default:
		return ""
	}
}

func (m AgentBuilderModel) renderList(title string, rows []string, help string) string {
	var b strings.Builder
	b.WriteString(styleRenderer.NewStyle().Bold(true).Render(title))
	b.WriteString("\n\n")
	for i, row := range rows {
		marker := "  "
		line := marker + row
		if i == m.cursor {
			line = "> " + row
			line = styleRenderer.NewStyle().Foreground(lipgloss.Color("6")).Render(line)
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(styleRenderer.NewStyle().Faint(true).Render(help))
	return b.String()
}

func (m AgentBuilderModel) renderPreview() string {
	var b strings.Builder
	b.WriteString(styleRenderer.NewStyle().Bold(true).Render("Revisá el agente antes de instalar"))
	b.WriteString("\n\n")
	b.WriteString(m.PreviewContent)
	b.WriteString("\n")
	for i, action := range agentBuilderPreviewActions {
		line := "  " + action
		if i == m.cursor {
			line = styleRenderer.NewStyle().Foreground(lipgloss.Color("6")).Render("> " + action)
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(styleRenderer.NewStyle().Faint(true).Render("↑/↓ mover · enter elegir · q/esc cancelar"))
	return b.String()
}

func renderInput(title, value string) string {
	return renderInputWithError(title, value, "")
}

func renderInputWithError(title, value, errorMessage string) string {
	var b strings.Builder
	b.WriteString(styleRenderer.NewStyle().Bold(true).Render(title))
	b.WriteString("\n\n")
	if errorMessage != "" {
		b.WriteString(styleRenderer.NewStyle().Foreground(lipgloss.Color("9")).Render(errorMessage))
		b.WriteString("\n\n")
	}
	b.WriteString(value)
	b.WriteString("\n\n")
	b.WriteString(styleRenderer.NewStyle().Faint(true).Render("escribí tu respuesta · enter continuar · esc cancelar"))
	return b.String()
}

func validateAgentBuilderGeneratedName(name string) error {
	if !agentBuilderGeneratedNamePattern.MatchString(name) {
		return fmt.Errorf("agentbuilder: invalid generated agent name %q; use ASCII letters or numbers in the description", name)
	}
	return nil
}

func validateAgentBuilderThemeAnswer(themeIndex int, answer string) error {
	switch themeIndex {
	case 4:
		if strings.TrimSpace(answer) == "" {
			return fmt.Errorf("agentbuilder: agent frontmatter field tools is required")
		}
	case 5:
		if strings.TrimSpace(answer) == "" {
			return fmt.Errorf("agentbuilder: agent frontmatter field model is required")
		}
	}
	return nil
}

func (m *AgentBuilderModel) moveCursor(delta, size int) {
	if size == 0 {
		m.cursor = 0
		return
	}
	m.cursor = ((m.cursor+delta)%size + size) % size
}

func (m *AgentBuilderModel) updateInput(keyMsg tea.KeyMsg) {
	switch keyMsg.Type {
	case tea.KeyRunes:
		m.input += string(keyMsg.Runes)
	case tea.KeySpace:
		m.input += " "
	case tea.KeyBackspace, tea.KeyCtrlH:
		runes := []rune(m.input)
		if len(runes) > 0 {
			m.input = string(runes[:len(runes)-1])
		}
	}
}

func (m *AgentBuilderModel) refreshPreview() {
	content, err := agentbuilder.RenderAgentMarkdown(m.Spec)
	if err != nil {
		m.PreviewError = err.Error()
		m.PreviewContent = fmt.Sprintf("No se pudo generar el preview: %v", err)
		return
	}
	m.PreviewError = ""
	m.PreviewContent = content
}

func (m *AgentBuilderModel) validatePreviewSpec() error {
	_, err := agentbuilder.RenderAgentMarkdown(m.Spec)
	if err != nil {
		m.PreviewError = err.Error()
		m.PreviewContent = fmt.Sprintf("No se pudo generar el preview: %v", err)
		return err
	}
	m.PreviewError = ""
	return nil
}

func isAgentBuilderHardCancelKey(keyMsg tea.KeyMsg) bool {
	switch keyMsg.Type {
	case tea.KeyEsc, tea.KeyCtrlC:
		return true
	default:
		return false
	}
}

func isAgentBuilderCancelKey(keyMsg tea.KeyMsg) bool {
	switch keyMsg.Type {
	case tea.KeyEsc, tea.KeyCtrlC:
		return true
	case tea.KeyRunes:
		return string(keyMsg.Runes) == "q"
	default:
		return false
	}
}

func keyName(keyMsg tea.KeyMsg) string {
	switch keyMsg.Type {
	case tea.KeyUp:
		return "up"
	case tea.KeyDown:
		return "down"
	case tea.KeyEnter:
		return "enter"
	case tea.KeyRunes:
		return string(keyMsg.Runes)
	default:
		return ""
	}
}

func engineLabels(engines []agentbuilder.Engine) []string {
	labels := make([]string, len(engines))
	for i, engine := range engines {
		labels[i] = engine.Label
	}
	return labels
}

func sddModeLabels() []string {
	labels := make([]string, len(agentBuilderSDDModeOptions))
	for i, option := range agentBuilderSDDModeOptions {
		labels[i] = option.label
	}
	return labels
}

func phaseLabelsForAgentBuilder() []string {
	labels := make([]string, len(modelconfig.Phases))
	for i, phase := range modelconfig.Phases {
		labels[i] = string(phase)
	}
	return labels
}

func placementLabels() []string {
	labels := make([]string, len(agentBuilderPlacementOptions))
	for i, option := range agentBuilderPlacementOptions {
		labels[i] = option.label
	}
	return labels
}

func deriveAgentName(description string) string {
	var words []string
	var current strings.Builder
	flush := func() {
		if current.Len() == 0 {
			return
		}
		words = append(words, current.String())
		current.Reset()
	}
	for _, r := range strings.ToLower(description) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			current.WriteRune(r)
		default:
			flush()
		}
		if len(words) == 5 {
			break
		}
	}
	flush()
	if len(words) == 0 {
		return "custom-agent"
	}
	if len(words) > 5 {
		words = words[:5]
	}
	return strings.Join(words, "-")
}
