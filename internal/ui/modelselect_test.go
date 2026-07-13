package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/modelconfig"
)

func TestNewModelSelectModel_StartsWithDefaultsPreSelected(t *testing.T) {
	m := NewModelSelectModel()
	want := modelconfig.Defaults()
	for phase, model := range want {
		if got := m.Selection[phase]; got != model {
			t.Errorf("initial Selection[%s] = %q, want default %q", phase, got, model)
		}
	}
	if m.Cursor != 0 {
		t.Errorf("initial Cursor = %d, want 0", m.Cursor)
	}
	if m.Confirmed || m.Cancelled {
		t.Errorf("initial state Confirmed=%v Cancelled=%v, want both false", m.Confirmed, m.Cancelled)
	}
}

func TestModelSelectModel_Update_ArrowsMoveCursorAndWrap(t *testing.T) {
	m := NewModelSelectModel()

	m, _ = updateModel(m, keyMsg("down"))
	if m.Cursor != 1 {
		t.Fatalf("Cursor after one down = %d, want 1", m.Cursor)
	}

	m, _ = updateModel(m, keyMsg("up"))
	if m.Cursor != 0 {
		t.Fatalf("Cursor after down,up = %d, want 0", m.Cursor)
	}

	// up from row 0 must wrap to the last phase row.
	m, _ = updateModel(m, keyMsg("up"))
	if want := len(modelconfig.Phases) - 1; m.Cursor != want {
		t.Fatalf("Cursor after wrapping up = %d, want %d", m.Cursor, want)
	}

	// down from the last row must wrap back to 0.
	m, _ = updateModel(m, keyMsg("down"))
	if m.Cursor != 0 {
		t.Fatalf("Cursor after wrapping down = %d, want 0", m.Cursor)
	}
}

func TestModelSelectModel_Update_RightCyclesModelForCursorRowOnly(t *testing.T) {
	m := NewModelSelectModel()
	phase := modelconfig.Phases[0]
	// modelconfig.Phases[0] is "explore", whose default (per the real SDD taxonomy) is sonnet.
	if m.Selection[phase] != "sonnet" {
		t.Fatalf("precondition: Selection[%s] = %q, want sonnet", phase, m.Selection[phase])
	}
	otherPhase := modelconfig.Phases[1]
	otherBefore := m.Selection[otherPhase]

	m, _ = updateModel(m, keyMsg("right"))
	if m.Selection[phase] != "haiku" {
		t.Fatalf("Selection[%s] after one right = %q, want haiku", phase, m.Selection[phase])
	}
	if m.Selection[otherPhase] != otherBefore {
		t.Fatalf("Selection[%s] changed to %q after cycling a different row, want unchanged %q", otherPhase, m.Selection[otherPhase], otherBefore)
	}

	m, _ = updateModel(m, keyMsg("right"))
	if m.Selection[phase] != "opus" {
		t.Fatalf("Selection[%s] after two rights = %q, want opus", phase, m.Selection[phase])
	}

	// cycle wraps back to sonnet after the last option.
	m, _ = updateModel(m, keyMsg("right"))
	if m.Selection[phase] != "sonnet" {
		t.Fatalf("Selection[%s] after three rights (full cycle) = %q, want sonnet", phase, m.Selection[phase])
	}
}

func TestModelSelectModel_Update_LeftCyclesBackward(t *testing.T) {
	m := NewModelSelectModel()
	phase := modelconfig.Phases[0]

	m, _ = updateModel(m, keyMsg("left"))
	if m.Selection[phase] != "opus" {
		t.Fatalf("Selection[%s] after one left (wrap backward) = %q, want opus", phase, m.Selection[phase])
	}
}

func TestModelSelectModel_Update_EnterConfirmsAndQuits(t *testing.T) {
	m := NewModelSelectModel()
	m, cmd := updateModel(m, keyMsg("enter"))
	if !m.Confirmed {
		t.Fatal("Confirmed = false after enter, want true")
	}
	if cmd == nil {
		t.Fatal("Update(enter) returned a nil tea.Cmd, want tea.Quit")
	}
}

func TestModelSelectModel_Update_EnterWithNoEditsAcceptsAllDefaults(t *testing.T) {
	m := NewModelSelectModel()
	m, _ = updateModel(m, keyMsg("enter"))
	want := modelconfig.Defaults()
	for phase, model := range want {
		if got := m.Selection[phase]; got != model {
			t.Errorf("Selection[%s] after immediate enter = %q, want default %q", phase, got, model)
		}
	}
}

func TestModelSelectModel_Update_EscCancelsAndQuits(t *testing.T) {
	m := NewModelSelectModel()
	m, cmd := updateModel(m, keyMsg("esc"))
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

func TestModelSelectModel_Update_IgnoresNonKeyMessages(t *testing.T) {
	m := NewModelSelectModel()
	before := m
	m, cmd := updateModel(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	if m.Cursor != before.Cursor || m.Confirmed != before.Confirmed || m.Cancelled != before.Cancelled {
		t.Fatalf("Update(non-key msg) mutated state: got %+v, want unchanged %+v", m, before)
	}
	if cmd != nil {
		t.Fatalf("Update(non-key msg) returned a non-nil cmd, want nil")
	}
}

func TestModelSelectModel_View_RendersAllPhaseRows(t *testing.T) {
	m := NewModelSelectModel()
	view := m.View()
	rowCount := 0
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, " opus") || strings.Contains(line, " sonnet") || strings.Contains(line, " haiku") {
			rowCount++
		}
	}
	if rowCount != len(modelconfig.Phases) {
		t.Fatalf("View() rendered %d phase rows, want %d:\n%s", rowCount, len(modelconfig.Phases), view)
	}
	for _, model := range m.Selection {
		if !strings.Contains(view, model) {
			t.Errorf("View() missing selected model %q:\n%s", model, view)
		}
	}
}

func TestModelSelectModel_PhaseLabelsCoverEveryPhase(t *testing.T) {
	m := NewModelSelectModel()
	view := m.View()
	for _, phase := range modelconfig.Phases {
		label := phaseLabels[phase]
		if label == "" {
			t.Fatalf("phaseLabels[%q] is empty or missing", phase)
		}
		if !strings.Contains(view, label) {
			t.Fatalf("View() missing label %q for phase %q:\n%s", label, phase, view)
		}
	}
}

// TestNewModelSelectModelForProfile_SeedsFromProfilePreset guards design D4's seeding contract:
// picking a preset in the profile-select step must pre-fill this screen with that preset's own
// values, not the D25 defaults.
func TestNewModelSelectModelForProfile_SeedsFromProfilePreset(t *testing.T) {
	m := NewModelSelectModelForProfile(modelconfig.ProfileQuality)
	want := modelconfig.ResolveForProfile(string(modelconfig.ProfileQuality), nil)
	for phase, model := range want {
		if got := m.Selection[phase]; got != model {
			t.Errorf("Selection[%s] = %q, want quality preset %q", phase, got, model)
		}
	}
	if m.Cursor != 0 {
		t.Errorf("initial Cursor = %d, want 0", m.Cursor)
	}
	if m.Confirmed || m.Cancelled {
		t.Errorf("initial state Confirmed=%v Cancelled=%v, want both false", m.Confirmed, m.Cancelled)
	}
}

// TestNewModelSelectModelForProfile_Balanced_MatchesDefaults guards that seeding from "balanced"
// still matches Defaults() verbatim (design D1: balanced IS Defaults()) — the same invariant
// TestNewModelSelectModel_StartsWithDefaultsPreSelected already covers for the plain constructor.
func TestNewModelSelectModelForProfile_Balanced_MatchesDefaults(t *testing.T) {
	m := NewModelSelectModelForProfile(modelconfig.ProfileBalanced)
	want := modelconfig.Defaults()
	for phase, model := range want {
		if got := m.Selection[phase]; got != model {
			t.Errorf("Selection[%s] = %q, want default %q", phase, got, model)
		}
	}
}

func keyMsg(key string) tea.KeyMsg {
	switch key {
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "left":
		return tea.KeyMsg{Type: tea.KeyLeft}
	case "right":
		return tea.KeyMsg{Type: tea.KeyRight}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
	}
}

// updateModel calls ModelSelectModel.Update and type-asserts the result back, so tests can chain
// calls without repeating the tea.Model interface's boilerplate.
func updateModel(m ModelSelectModel, msg tea.Msg) (ModelSelectModel, tea.Cmd) {
	updated, cmd := m.Update(msg)
	return updated.(ModelSelectModel), cmd
}
