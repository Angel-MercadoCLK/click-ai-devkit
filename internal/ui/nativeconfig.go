package ui

import (
	"errors"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// NativeModelConfig is the result of a target-native model screen.
type NativeModelConfig struct {
	Primary   string
	Fallbacks []string
	Confirmed bool
	Cancelled bool
}

// NativeModelConfigModel collects a native model reference and, for OpenClaw, optional fallbacks.
// It is intentionally a small Bubble Tea model so the install wizard can test it without a TTY.
type NativeModelConfigModel struct {
	kind      string
	step      int
	input     textinput.Model
	primary   string
	fallbacks []string
	result    NativeModelConfig
	errorText string
}

func (m NativeModelConfigModel) Result() NativeModelConfig { return m.result }

func NewNativeModelConfigModel(kind, primary string, fallbacks []string) NativeModelConfigModel {
	in := textinput.New()
	in.Prompt = "> "
	in.Focus()
	in.SetValue(primary)
	return NativeModelConfigModel{kind: kind, input: in, primary: primary, fallbacks: append([]string(nil), fallbacks...)}
}

func (m NativeModelConfigModel) Init() tea.Cmd { return textinput.Blink }

func (m NativeModelConfigModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	if key.Type == tea.KeyEsc || key.Type == tea.KeyCtrlC || (key.Type == tea.KeyRunes && string(key.Runes) == "q") {
		m.result.Cancelled = true
		return m, tea.Quit
	}
	if key.Type == tea.KeyEnter {
		value := strings.TrimSpace(m.input.Value())
		switch m.step {
		case 0:
			if err := validateNativeReference(m.kind, value); err != nil {
				m.errorText = err.Error()
				return m, nil
			}
			m.primary = value
			if m.kind == "Codex" {
				m.step = 2
			} else {
				m.step = 1
				m.input.Reset()
				m.input.SetValue(strings.Join(m.fallbacks, ", "))
			}
		case 1:
			fallbacks, err := parseFallbacks(value)
			if err != nil {
				m.errorText = err.Error()
				return m, nil
			}
			m.fallbacks = fallbacks
			m.step = 2
			m.input.Blur()
		case 2:
			m.result = NativeModelConfig{Primary: m.primary, Fallbacks: append([]string(nil), m.fallbacks...), Confirmed: true}
			return m, tea.Quit
		}
		m.errorText = ""
		return m, nil
	}
	updated, cmd := m.input.Update(msg)
	m.input = updated
	return m, cmd
}

func (m NativeModelConfigModel) View() string {
	var prompt string
	switch m.step {
	case 0:
		prompt = fmt.Sprintf("Configure el modelo nativo de %s\n\nReferencia: %s", m.kind, m.input.View())
	case 1:
		prompt = "Configure fallbacks opcionales de OpenClaw\n\nSepare referencias provider/model con comas (Enter para omitir): " + m.input.View()
	case 2:
		prompt = fmt.Sprintf("Confirme la configuración nativa\n\nPrimario: %s\nFallbacks: %s\n\nEnter confirmar · Esc cancelar", m.primary, strings.Join(m.fallbacks, ", "))
	}
	if m.errorText != "" {
		prompt += "\n\nError: " + m.errorText
	}
	return prompt
}

func validateNativeReference(kind, value string) error {
	if value == "" || strings.ContainsAny(value, "\r\n") {
		return errors.New("la referencia de modelo no puede estar vacía ni contener saltos de línea")
	}
	if kind == "OpenClaw" && !strings.Contains(value, "/") {
		return errors.New("OpenClaw requiere una referencia provider/model")
	}
	return nil
}

func parseFallbacks(value string) ([]string, error) {
	if value == "" {
		return nil, nil
	}
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if err := validateNativeReference("OpenClaw", part); err != nil {
			return nil, fmt.Errorf("fallback inválido: %w", err)
		}
		result = append(result, part)
	}
	return result, nil
}
