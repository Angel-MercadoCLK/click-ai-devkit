package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestNativeModelConfigModel_OpenClawCollectsAndConfirmsNativeRefs(t *testing.T) {
	m := NewNativeModelConfigModel("OpenClaw", "openai/gpt-5.6", nil)
	var model tea.Model = m
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("anthropic/claude-sonnet-4-6, openai/gpt-5.5")})
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := model.(NativeModelConfigModel).result
	if !got.Confirmed || got.Primary != "openai/gpt-5.6" || len(got.Fallbacks) != 2 {
		t.Fatalf("result = %#v, want confirmed primary and two fallbacks", got)
	}
}

func TestNativeModelConfigModel_CodexConfirmsNativeRef(t *testing.T) {
	m := NewNativeModelConfigModel("Codex", "gpt-5.6", nil)
	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !model.(NativeModelConfigModel).result.Confirmed {
		t.Fatal("Codex model was not confirmed")
	}
}
