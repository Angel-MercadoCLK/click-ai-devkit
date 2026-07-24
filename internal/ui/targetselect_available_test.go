package ui

import (
	"strings"
	"testing"
)

func TestTargetSelectModel_AbsentTargetsAreNotSelectableOrSelected(t *testing.T) {
	m := NewTargetSelectModel(false, true, true, true, true, true)
	if m.Claude || !m.OpenClaw || !m.Codex {
		t.Fatalf("selection = Claude %t, OpenClaw %t, Codex %t; absent Claude must not be selected", m.Claude, m.OpenClaw, m.Codex)
	}
	if strings.Contains(m.View(), "Claude Code") {
		t.Fatalf("View() contains absent Claude target: %q", m.View())
	}
}
