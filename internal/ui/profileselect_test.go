package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/modelconfig"
)

func TestNewProfileSelectModel_StartsOnBalanced(t *testing.T) {
	m := NewProfileSelectModel()
	if m.Selected != modelconfig.ProfileBalanced {
		t.Fatalf("initial Selected = %q, want %q", m.Selected, modelconfig.ProfileBalanced)
	}
	if m.Cursor != 0 {
		t.Fatalf("initial Cursor = %d, want 0", m.Cursor)
	}
	if m.Confirmed || m.Cancelled {
		t.Fatalf("initial state Confirmed=%v Cancelled=%v, want both false", m.Confirmed, m.Cancelled)
	}
}

func TestProfileSelectModel_Update_ArrowsMoveCursorAndWrapThroughAllProfiles(t *testing.T) {
	m := NewProfileSelectModel()

	m, _ = updateProfileModel(m, keyMsg("down"))
	if m.Selected != modelconfig.ProfileCostSaver {
		t.Fatalf("Selected after one down = %q, want %q", m.Selected, modelconfig.ProfileCostSaver)
	}

	m, _ = updateProfileModel(m, keyMsg("down"))
	if m.Selected != modelconfig.ProfileQuality {
		t.Fatalf("Selected after two downs = %q, want %q", m.Selected, modelconfig.ProfileQuality)
	}

	m, _ = updateProfileModel(m, keyMsg("down"))
	if m.Selected != modelconfig.ProfileCustom {
		t.Fatalf("Selected after three downs = %q, want %q", m.Selected, modelconfig.ProfileCustom)
	}

	// one more down wraps back to balanced.
	m, _ = updateProfileModel(m, keyMsg("down"))
	if m.Selected != modelconfig.ProfileBalanced {
		t.Fatalf("Selected after wrapping down = %q, want %q", m.Selected, modelconfig.ProfileBalanced)
	}

	// up from balanced wraps to the last row (custom).
	m, _ = updateProfileModel(m, keyMsg("up"))
	if m.Selected != modelconfig.ProfileCustom {
		t.Fatalf("Selected after wrapping up = %q, want %q", m.Selected, modelconfig.ProfileCustom)
	}
}

func TestProfileSelectModel_Update_JKMoveCursorLikeArrows(t *testing.T) {
	m := NewProfileSelectModel()
	m, _ = updateProfileModel(m, keyMsg("j"))
	if m.Selected != modelconfig.ProfileCostSaver {
		t.Fatalf("Selected after j = %q, want %q", m.Selected, modelconfig.ProfileCostSaver)
	}
	m, _ = updateProfileModel(m, keyMsg("k"))
	if m.Selected != modelconfig.ProfileBalanced {
		t.Fatalf("Selected after j,k = %q, want %q", m.Selected, modelconfig.ProfileBalanced)
	}
}

func TestProfileSelectModel_Update_EnterConfirmsAndQuits(t *testing.T) {
	m := NewProfileSelectModel()
	m, _ = updateProfileModel(m, keyMsg("down"))
	m, cmd := updateProfileModel(m, keyMsg("enter"))
	if !m.Confirmed {
		t.Fatal("Confirmed = false after enter, want true")
	}
	if m.Selected != modelconfig.ProfileCostSaver {
		t.Fatalf("Selected after enter = %q, want %q", m.Selected, modelconfig.ProfileCostSaver)
	}
	if cmd == nil {
		t.Fatal("Update(enter) returned a nil tea.Cmd, want tea.Quit")
	}
}

func TestProfileSelectModel_Update_EscCancelsAndQuits(t *testing.T) {
	m := NewProfileSelectModel()
	m, cmd := updateProfileModel(m, keyMsg("esc"))
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

func TestProfileSelectModel_Update_QCancelsAndQuits(t *testing.T) {
	m := NewProfileSelectModel()
	m, cmd := updateProfileModel(m, keyMsg("q"))
	if !m.Cancelled {
		t.Fatal("Cancelled = false after q, want true")
	}
	if cmd == nil {
		t.Fatal("Update(q) returned a nil tea.Cmd, want tea.Quit")
	}
}

func TestProfileSelectModel_Update_IgnoresNonKeyMessages(t *testing.T) {
	m := NewProfileSelectModel()
	before := m
	m, cmd := updateProfileModel(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	if m != before {
		t.Fatalf("Update(non-key msg) mutated state: got %+v, want unchanged %+v", m, before)
	}
	if cmd != nil {
		t.Fatalf("Update(non-key msg) returned a non-nil cmd, want nil")
	}
}

func TestProfileSelectModel_View_RendersAllFourProfiles(t *testing.T) {
	m := NewProfileSelectModel()
	view := m.View()
	for _, name := range []modelconfig.ProfileName{
		modelconfig.ProfileBalanced, modelconfig.ProfileCostSaver, modelconfig.ProfileQuality, modelconfig.ProfileCustom,
	} {
		if !strings.Contains(view, string(name)) {
			t.Errorf("View() missing profile row %q:\n%s", name, view)
		}
	}
}

func updateProfileModel(m ProfileSelectModel, msg tea.Msg) (ProfileSelectModel, tea.Cmd) {
	updated, cmd := m.Update(msg)
	return updated.(ProfileSelectModel), cmd
}
