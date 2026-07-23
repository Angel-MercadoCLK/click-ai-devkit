package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestTargetSelectModel_TogglesOnlySupportedTargets(t *testing.T) {
	m := NewTargetSelectModel(true, true, true, true, true)
	m, _ = updateTargetModel(m, tea.KeyMsg{Type: tea.KeyDown})
	m, _ = updateTargetModel(m, tea.KeyMsg{Type: tea.KeySpace})
	if m.OpenClaw {
		t.Fatal("OpenClaw remained selected after space toggle")
	}
	m, _ = updateTargetModel(m, tea.KeyMsg{Type: tea.KeyDown})
	m, _ = updateTargetModel(m, tea.KeyMsg{Type: tea.KeySpace})
	if !m.Codex {
		t.Fatal("Codex remained unselected after space toggle")
	}
	m, cmd := updateTargetModel(m, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil || !m.Confirmed || m.Cancelled {
		t.Fatalf("confirm state = %+v, cmd = %v", m, cmd)
	}
}

func TestTargetSelectModel_MovementDirections(t *testing.T) {
	tests := []struct {
		name       string
		start      int
		key        tea.KeyMsg
		wantCursor int
	}{
		{name: "up moves backward", start: 1, key: tea.KeyMsg{Type: tea.KeyUp}, wantCursor: 0},
		{name: "down moves forward", start: 0, key: tea.KeyMsg{Type: tea.KeyDown}, wantCursor: 1},
		{name: "k moves backward", start: 1, key: tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")}, wantCursor: 0},
		{name: "j moves forward", start: 0, key: tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")}, wantCursor: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewTargetSelectModel(true, true, true, true, true)
			m.Cursor = tt.start
			m, _ = updateTargetModel(m, tt.key)
			if m.Cursor != tt.wantCursor {
				t.Fatalf("cursor = %d, want %d", m.Cursor, tt.wantCursor)
			}
		})
	}
}

func TestTargetSelectModel_ViewShowsDetectionCapabilitiesAndUnsupportedRuntime(t *testing.T) {
	view := NewTargetSelectModel(true, true, true, false, true).View()
	for _, want := range []string{"Claude Code", "OpenClaw", "Codex", "detectado", "SDD portable", "AGENTS.md", "Otros runtimes: no soportados"} {
		if !strings.Contains(view, want) {
			t.Errorf("target selection view missing %q:\n%s", want, view)
		}
	}
}

func updateTargetModel(m TargetSelectModel, msg tea.Msg) (TargetSelectModel, tea.Cmd) {
	updated, cmd := m.Update(msg)
	return updated.(TargetSelectModel), cmd
}
