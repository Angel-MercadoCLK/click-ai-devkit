package ui

import (
	"fmt"
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
	FinalMarkdown  string
	PreviewError   string
	EditingPreview bool
	Confirmed      bool
	Cancelled      bool

	engines            []agentbuilder.Engine
	cursor             int
	input              string
	checkNameAvailable func(agentbuilder.AgentSpec) error
}

var agentBuilderSDDModeOptions = []struct {
	label string
	mode  agentbuilder.SDDMode
}{
	{label: "Standalone", mode: agentbuilder.SDDStandalone},
	{label: "Phase Support", mode: agentbuilder.SDDPhaseSupport},
}

// agentBuilderThemeKind tags what a theme prompt's answer is used for, so validation
// can dispatch on an explicit, declared role instead of the prompt's raw position in
// agentBuilderThemePrompts (R2-006). Reordering the slice below can never silently
// point validation at the wrong field, because each prompt still declares its own kind.
type agentBuilderThemeKind int

const (
	agentBuilderThemeFreeText agentBuilderThemeKind = iota
	agentBuilderThemeTools
	agentBuilderThemeModel
)

var agentBuilderThemePrompts = []struct {
	title string
	kind  agentBuilderThemeKind
	apply func(*agentbuilder.AgentSpec, string)
}{
	{title: "Propósito / objetivo", kind: agentBuilderThemeFreeText, apply: func(spec *agentbuilder.AgentSpec, value string) { spec.Purpose = value }},
	{title: "Tareas exactas", kind: agentBuilderThemeFreeText, apply: func(spec *agentbuilder.AgentSpec, value string) { spec.Tasks = value }},
	{title: "Situaciones o frases que lo activan", kind: agentBuilderThemeFreeText, apply: func(spec *agentbuilder.AgentSpec, value string) { spec.Triggers = value }},
	{title: "Reglas duras / restricciones", kind: agentBuilderThemeFreeText, apply: func(spec *agentbuilder.AgentSpec, value string) { spec.Rules = value }},
	{title: "Herramientas necesarias", kind: agentBuilderThemeTools, apply: func(spec *agentbuilder.AgentSpec, value string) { spec.Tools = value }},
	{title: "Modelo", kind: agentBuilderThemeModel, apply: func(spec *agentbuilder.AgentSpec, value string) { spec.Model = value }},
	{title: "Tono / persona", kind: agentBuilderThemeFreeText, apply: func(spec *agentbuilder.AgentSpec, value string) { spec.Tone = value }},
	{title: "Conocimiento de dominio", kind: agentBuilderThemeFreeText, apply: func(spec *agentbuilder.AgentSpec, value string) { spec.Domain = value }},
	{title: "Ejemplo de buen resultado", kind: agentBuilderThemeFreeText, apply: func(spec *agentbuilder.AgentSpec, value string) { spec.GoodOutput = value }},
}

var agentBuilderPreviewActions = []string{"Instalar", "Editar", "Regenerar", "Volver"}

var agentBuilderPlacementOptions = []struct {
	label     string
	placement agentbuilder.Placement
}{
	{label: "Personal", placement: agentbuilder.PlacementPersonal},
	{label: "Shareable", placement: agentbuilder.PlacementShareable},
}

// AgentBuilderOption configures optional, injectable behavior on AgentBuilderModel.
// Using a variadic option here (rather than growing NewAgentBuilderModel's required
// parameter list) keeps every existing call site source-compatible.
type AgentBuilderOption func(*AgentBuilderModel)

// WithNameAvailabilityCheck injects a read-only collision probe the model calls right
// before confirming the wizard (see updatePlacement). If unset, no collision check runs
// here at all — the model's behavior is unchanged from before this option existed.
//
// This exists so a name collision can be surfaced WHILE the wizard is still running,
// with every answer still held in Spec/PreviewContent, instead of only being
// discovered by the caller after the wizard has already exited with Confirmed=true and
// all of that state has been discarded (R4-003).
func WithNameAvailabilityCheck(check func(agentbuilder.AgentSpec) error) AgentBuilderOption {
	return func(m *AgentBuilderModel) { m.checkNameAvailable = check }
}

func NewAgentBuilderModel(engines []agentbuilder.Engine, opts ...AgentBuilderOption) AgentBuilderModel {
	if len(engines) == 0 {
		engines = agentbuilder.Engines()
	}
	m := AgentBuilderModel{engines: append([]agentbuilder.Engine(nil), engines...)}
	for _, opt := range opts {
		opt(&m)
	}
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
		if err := agentbuilder.ValidateFrontmatterScalar("description", description); err != nil {
			m.PreviewError = translateAgentBuilderError(err)
			return m, nil
		}
		name := deriveAgentName(description)
		if err := agentbuilder.ValidateAgentName(name); err != nil {
			m.PreviewError = translateAgentBuilderError(err)
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
			m.PreviewError = translateAgentBuilderError(err)
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
		m.FinalMarkdown = ""
		m.input = ""
		m.EditingPreview = false
		if err := agentbuilder.ValidateFinalMarkdown(m.PreviewContent); err != nil {
			m.PreviewError = translateAgentBuilderError(err)
			return m, nil
		}
		m.PreviewError = ""
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
		if m.checkNameAvailable != nil {
			if err := m.checkNameAvailable(m.Spec); err != nil {
				// Recoverable: bounce back to Preview (not a terminal error) with every
				// answer still held in Spec/PreviewContent, instead of dying after
				// Confirmed=true (R4-003). NOTE: this does NOT let the user rename
				// in-place — Editar rejects a frontmatter name that diverges from the
				// generated one (see the "must match generated name" check), so the
				// only real recovery today is cancelling and retrying with a
				// different description, or clearing the conflicting file first. The
				// error text shown to the user (translateAgentBuilderError's
				// "already exists" case) says so honestly; don't reintroduce a claim
				// that Editar can change the name (RRAB-001).
				m.PreviewError = translateAgentBuilderError(err)
				m.Step = StepPreview
				m.cursor = 0
				return m, nil
			}
		}
		m.FinalMarkdown = m.PreviewContent
		m.Confirmed = true
		m.Step = StepDone
		return m, tea.Quit
	}
	return m, nil
}

func (m AgentBuilderModel) View() string {
	switch m.Step {
	case StepEngine:
		return m.renderList("Elija el motor del agente", engineLabels(m.engines), "↑/↓ mover · enter confirmar · q/esc cancelar")
	case StepDescription:
		return renderInputWithError("Describa el agente que quiere crear", m.input, m.PreviewError)
	case StepSDDMode:
		return m.renderList("Elija la integración SDD", sddModeLabels(), "↑/↓ mover · enter confirmar · q/esc cancelar")
	case StepPhase:
		return m.renderList("Elija la fase SDD que este agente va a apoyar", phaseLabelsForAgentBuilder(), "↑/↓ mover · enter confirmar · q/esc cancelar")
	case StepThemes:
		prompt := agentBuilderThemePrompts[m.ThemeIndex]
		return renderInputWithError(fmt.Sprintf("%d/%d · %s", m.ThemeIndex+1, len(agentBuilderThemePrompts), prompt.title), m.input, m.PreviewError)
	case StepPreview:
		if m.EditingPreview {
			return renderInput("Edite el Markdown final", m.input)
		}
		return m.renderPreview()
	case StepPlacement:
		return m.renderList("Elija dónde instalar el agente", placementLabels(), "↑/↓ mover · enter instalar · q/esc cancelar")
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
	b.WriteString(styleRenderer.NewStyle().Bold(true).Render("Revise el agente antes de instalar"))
	b.WriteString("\n\n")
	if m.PreviewError != "" {
		b.WriteString(styleRenderer.NewStyle().Foreground(lipgloss.Color("9")).Render(m.PreviewError))
		b.WriteString("\n\n")
	}
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
	b.WriteString(styleRenderer.NewStyle().Faint(true).Render("escriba su respuesta · enter continuar · esc cancelar"))
	return b.String()
}

// validateAgentBuilderThemeAnswer delegates to the domain package's canonical
// frontmatter scalar validator instead of keeping a UI-side duplicate (R1-001,
// R2-005), and dispatches on the prompt's declared agentBuilderThemeKind rather than
// its raw index (R2-006).
func validateAgentBuilderThemeAnswer(themeIndex int, answer string) error {
	switch agentBuilderThemePrompts[themeIndex].kind {
	case agentBuilderThemeTools:
		return agentbuilder.ValidateFrontmatterScalar("tools", answer)
	case agentBuilderThemeModel:
		return agentbuilder.ValidateFrontmatterScalar("model", answer)
	}
	return nil
}

// translateAgentBuilderError converts a domain/internal error into Spanish user-facing
// text for display in the wizard.
//
// Per locked decision D10 (dev-facing CLI/TUI string literals are Spanish), the
// underlying Go error values returned by internal/agentbuilder stay in English (for
// logs and %w-wrapping) — only the text actually rendered to the user goes through
// this translator, consistent with the rest of this repo's UI (e.g.
// internal/ui/profileselect.go, internal/cli/doctor.go).
func translateAgentBuilderError(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	detail := stripAgentBuilderErrorPrefixes(msg)
	switch {
	case strings.Contains(msg, "must match generated name"):
		return "El nombre del frontmatter editado no coincide con el nombre del agente generado por el wizard."
	case strings.Contains(msg, "already exists"):
		// The wizard has no in-flow way to rename (Editar rejects a frontmatter name
		// that doesn't match the generated one — see the "must match generated name"
		// case above), so the honest recovery path is: cancel and retry with a
		// different description, or clear the conflicting file first. Do not imply an
		// in-place rename is possible here (RRAB-001).
		return fmt.Sprintf("Ya existe un agente o plugin con ese nombre. Cancele (Esc) y vuelva a intentar con una descripción distinta, o elimine el agente/plugin existente antes de instalar. (Detalle técnico: %s)", detail)
	case strings.Contains(msg, "invalid agent name") || strings.Contains(msg, "invalid generated agent name"):
		return "El nombre del agente no es válido: use letras minúsculas, números y guiones (sin espacios ni tildes)."
	case strings.Contains(msg, "contains a newline"):
		return "El valor ingresado no puede contener saltos de línea."
	case strings.Contains(msg, "is required"):
		return "Falta completar un campo obligatorio."
	case strings.Contains(msg, "must start with YAML frontmatter") || strings.Contains(msg, "must close YAML frontmatter"):
		return "El markdown final no tiene un bloque de frontmatter YAML válido (debe empezar y cerrar con \"---\")."
	case strings.Contains(msg, "missing") && strings.Contains(msg, "section"):
		return fmt.Sprintf("Al markdown final le falta una sección obligatoria. (Detalle técnico: %s)", detail)
	case strings.Contains(msg, "not allowed") || strings.Contains(msg, "must be a top-level native Claude agent field") || strings.Contains(msg, "must use a valid top-level field name"):
		return "El frontmatter tiene un campo que no está permitido."
	case strings.Contains(msg, "must be unique"):
		return "El frontmatter tiene un campo duplicado."
	case strings.Contains(msg, "indented continuation lines"):
		return "El frontmatter no puede tener líneas indentadas."
	default:
		// Fallback: still Spanish-language user-facing text, with the raw technical
		// detail appended (prefix stripped) for troubleshooting rather than shown as
		// the whole message.
		return fmt.Sprintf("Hubo un problema al validar el agente. (Detalle técnico: %s)", detail)
	}
}

// stripAgentBuilderErrorPrefixes removes the internal package-name prefixes
// ("agentbuilder: ", "cli: ") from an error message before it is embedded as a
// technical-detail suffix in a Spanish user-facing message (D10): the prefix itself is
// an internal Go-package label, not meaningful content for the user.
func stripAgentBuilderErrorPrefixes(msg string) string {
	for _, prefix := range []string{"agentbuilder: ", "cli: "} {
		msg = strings.TrimPrefix(msg, prefix)
	}
	return msg
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
	m.FinalMarkdown = ""
	if err != nil {
		m.PreviewError = translateAgentBuilderError(err)
		m.PreviewContent = fmt.Sprintf("No se pudo generar el preview: %s", translateAgentBuilderError(err))
		return
	}
	m.PreviewError = ""
	m.PreviewContent = content
}

func (m *AgentBuilderModel) validatePreviewSpec() error {
	_, err := agentbuilder.RenderAgentMarkdown(m.Spec)
	if err != nil {
		m.PreviewError = translateAgentBuilderError(err)
		m.PreviewContent = fmt.Sprintf("No se pudo generar el preview: %s", translateAgentBuilderError(err))
		return err
	}
	if err := agentbuilder.ValidateFinalMarkdown(m.PreviewContent, m.Spec.Name); err != nil {
		m.PreviewError = translateAgentBuilderError(err)
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
