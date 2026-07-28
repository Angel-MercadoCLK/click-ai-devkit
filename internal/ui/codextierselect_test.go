package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestNewCodexTierSelectModel_StartsOnRecommended(t *testing.T) {
	m := NewCodexTierSelectModel()
	if m.Selected != "recommended" {
		t.Fatalf("initial Selected = %q, want \"recommended\"", m.Selected)
	}
	if m.Cursor != 1 {
		t.Fatalf("initial Cursor = %d, want 1", m.Cursor)
	}
	if m.Confirmed || m.Cancelled {
		t.Fatalf("initial state Confirmed=%v Cancelled=%v, want both false", m.Confirmed, m.Cancelled)
	}
}

func TestCodexTierSelectModel_Update_ArrowsMoveCursorAndWrapThroughTiers(t *testing.T) {
	m := NewCodexTierSelectModel()

	m, _ = updateCodexTierModel(m, keyMsg("down"))
	if m.Selected != "powerful" {
		t.Fatalf("Selected after one down = %q, want \"powerful\"", m.Selected)
	}

	// one more down wraps back to low-cost.
	m, _ = updateCodexTierModel(m, keyMsg("down"))
	if m.Selected != "low-cost" {
		t.Fatalf("Selected after wrapping down = %q, want \"low-cost\"", m.Selected)
	}

	// up from low-cost wraps to the last row (powerful).
	m, _ = updateCodexTierModel(m, keyMsg("up"))
	if m.Selected != "powerful" {
		t.Fatalf("Selected after wrapping up = %q, want \"powerful\"", m.Selected)
	}

	// up again lands back on recommended.
	m, _ = updateCodexTierModel(m, keyMsg("up"))
	if m.Selected != "recommended" {
		t.Fatalf("Selected after another up = %q, want \"recommended\"", m.Selected)
	}
}

func TestCodexTierSelectModel_Update_JKMoveCursorLikeArrows(t *testing.T) {
	m := NewCodexTierSelectModel()
	m, _ = updateCodexTierModel(m, keyMsg("j"))
	if m.Selected != "powerful" {
		t.Fatalf("Selected after j = %q, want \"powerful\"", m.Selected)
	}
	m, _ = updateCodexTierModel(m, keyMsg("k"))
	if m.Selected != "recommended" {
		t.Fatalf("Selected after k = %q, want \"recommended\"", m.Selected)
	}
}

func TestCodexTierSelectModel_Update_EnterConfirmsAndQuits(t *testing.T) {
	m := NewCodexTierSelectModel()
	m, _ = updateCodexTierModel(m, keyMsg("up"))
	m, cmd := updateCodexTierModel(m, keyMsg("enter"))
	if !m.Confirmed {
		t.Fatal("Confirmed = false after enter, want true")
	}
	if m.Selected != "low-cost" {
		t.Fatalf("Selected after enter = %q, want \"low-cost\"", m.Selected)
	}
	if cmd == nil {
		t.Fatal("Update(enter) returned a nil tea.Cmd, want tea.Quit")
	}
}

func TestCodexTierSelectModel_Update_EscCancelsAndQuits(t *testing.T) {
	m := NewCodexTierSelectModel()
	m, cmd := updateCodexTierModel(m, keyMsg("esc"))
	if !m.Cancelled {
		t.Fatal("Cancelled = false after esc, want true")
	}
	if m.Confirmed {
		t.Fatal("Confirmed = true after esc, want false")
	}
	if cmd == nil {
		t.Fatal("Update(esc) returned a nil tea.Cmd, want tea.Quit")
	}
}

func TestCodexTierSelectModel_Update_QCancelsAndQuits(t *testing.T) {
	m := NewCodexTierSelectModel()
	m, cmd := updateCodexTierModel(m, keyMsg("q"))
	if !m.Cancelled {
		t.Fatal("Cancelled = false after q, want true")
	}
	if cmd == nil {
		t.Fatal("Update(q) returned a nil tea.Cmd, want tea.Quit")
	}
}

func TestCodexTierSelectModel_Update_IgnoresNonKeyMessages(t *testing.T) {
	m := NewCodexTierSelectModel()
	before := m
	m, cmd := updateCodexTierModel(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	if m != before {
		t.Fatalf("Update(non-key msg) mutated state: got %+v, want unchanged %+v", m, before)
	}
	if cmd != nil {
		t.Fatalf("Update(non-key msg) returned a non-nil cmd, want nil")
	}
}

func TestCodexTierSelectModel_View_RendersAllThreeTiers(t *testing.T) {
	m := NewCodexTierSelectModel()
	view := m.View()
	for _, tier := range []string{"low-cost", "recommended", "powerful"} {
		if !strings.Contains(view, tier) {
			t.Errorf("View() missing tier row %q:\n%s", tier, view)
		}
	}
	if !strings.Contains(view, "Orquestador") {
		t.Errorf("View() missing role summary mention:\n%s", view)
	}
}

func updateCodexTierModel(m CodexTierSelectModel, msg tea.Msg) (CodexTierSelectModel, tea.Cmd) {
	updated, cmd := m.Update(msg)
	return updated.(CodexTierSelectModel), cmd
}
