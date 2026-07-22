package menu

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestNewModel_StartsAtFirstItem(t *testing.T) {
	m := NewModel()
	if m.Cursor != 0 {
		t.Fatalf("initial Cursor = %d, want 0", m.Cursor)
	}
	if m.Chosen != "" {
		t.Fatalf("initial Chosen = %q, want empty", m.Chosen)
	}
	if m.StatusMsg != "" {
		t.Fatalf("initial StatusMsg = %q, want empty", m.StatusMsg)
	}
}

func TestModel_Update_JKMoveCursorAndWrap(t *testing.T) {
	m := NewModel()

	m, _ = updateModel(m, keyMsg("j"))
	if m.Cursor != 1 {
		t.Fatalf("Cursor after one j = %d, want 1", m.Cursor)
	}

	m, _ = updateModel(m, keyMsg("k"))
	if m.Cursor != 0 {
		t.Fatalf("Cursor after j,k = %d, want 0", m.Cursor)
	}

	// k from row 0 must wrap to the last item.
	m, _ = updateModel(m, keyMsg("k"))
	if want := len(Items) - 1; m.Cursor != want {
		t.Fatalf("Cursor after wrapping k = %d, want %d", m.Cursor, want)
	}

	// j from the last row must wrap back to 0.
	m, _ = updateModel(m, keyMsg("j"))
	if m.Cursor != 0 {
		t.Fatalf("Cursor after wrapping j = %d, want 0", m.Cursor)
	}
}

func TestModel_Update_ArrowKeysAlsoMoveCursor(t *testing.T) {
	m := NewModel()

	m, _ = updateModel(m, tea.KeyMsg{Type: tea.KeyDown})
	if m.Cursor != 1 {
		t.Fatalf("Cursor after KeyDown = %d, want 1", m.Cursor)
	}

	m, _ = updateModel(m, tea.KeyMsg{Type: tea.KeyUp})
	if m.Cursor != 0 {
		t.Fatalf("Cursor after KeyDown,KeyUp = %d, want 0", m.Cursor)
	}
}

func TestModel_Update_EnterOnActiveItemSetsChosenAndQuits(t *testing.T) {
	idx := firstItemIndexWhere(t, func(it Item) bool { return it.Active && it.Action != ActionQuit })
	m := NewModel()
	m.Cursor = idx

	m, cmd := updateModel(m, keyMsg("enter"))
	if m.Chosen != Items[idx].Action {
		t.Fatalf("Chosen = %q, want %q", m.Chosen, Items[idx].Action)
	}
	if cmd == nil {
		t.Fatal("Update(enter) on active item returned a nil tea.Cmd, want tea.Quit")
	}
}

// TestModel_Update_EnterOnInertItemShowsStatusAndDoesNotQuit guards selectCurrent's
// inert-item branch, which every current real Items row is now Active and no longer
// exercises (this change activated "Gestionar backups" and removed the other two inert
// placeholders entirely). The dispatch mechanism itself is unchanged and must still work
// correctly if a future item is ever added inert again, so this test swaps in a synthetic
// inert item via setItemsForTest instead of relying on real Items to contain one.
func TestModel_Update_EnterOnInertItemShowsStatusAndDoesNotQuit(t *testing.T) {
	defer setItemsForTest(t, []Item{{Label: "Inert placeholder", Active: false}})()

	m := NewModel()
	m.Cursor = 0

	m, cmd := updateModel(m, keyMsg("enter"))
	if m.Chosen != "" {
		t.Fatalf("Chosen = %q after selecting an inert item, want empty (no dispatch)", m.Chosen)
	}
	if m.StatusMsg == "" {
		t.Fatal("StatusMsg is empty after selecting an inert item, want a coming-soon message")
	}
	if cmd != nil {
		t.Fatal("Update(enter) on inert item returned a non-nil tea.Cmd, want nil (stay in menu)")
	}
}

func TestAgentBuilderMenuItemIsActiveAndDispatchable(t *testing.T) {
	idx := firstItemIndexWhere(t, func(it Item) bool { return it.Label == "Crear agente propio" })
	item := Items[idx]
	if !item.Active {
		t.Fatal("Crear agente propio Active = false, want true")
	}
	if item.Action != ActionAgentBuilder {
		t.Fatalf("Crear agente propio Action = %q, want %q", item.Action, ActionAgentBuilder)
	}

	got := ActionArgs(item.Action)
	want := []string{"agent-builder"}
	if len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("ActionArgs(%q) = %v, want %v", item.Action, got, want)
	}
}

// TestManageBackupsMenuItemIsActiveAndDispatchable guards the menu-side half of the
// "Gestionar backups" activation: the item must be active, wired to ActionManageBackups,
// and ActionArgs must map that action to the exact args click's fresh root command needs
// to reach `click manage-backups`.
func TestManageBackupsMenuItemIsActiveAndDispatchable(t *testing.T) {
	idx := firstItemIndexWhere(t, func(it Item) bool { return it.Label == "Gestionar backups" })
	item := Items[idx]
	if !item.Active {
		t.Fatal("Gestionar backups Active = false, want true")
	}
	if item.Action != ActionManageBackups {
		t.Fatalf("Gestionar backups Action = %q, want %q", item.Action, ActionManageBackups)
	}

	got := ActionArgs(item.Action)
	want := []string{"manage-backups"}
	if len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("ActionArgs(%q) = %v, want %v", item.Action, got, want)
	}
}

// TestRollbackMenuItemIsActiveAndDispatchable guards the menu-side rollback wiring: the item must
// be active, wired to ActionRollback, and ActionArgs must map it to `click rollback`.
func TestRollbackMenuItemIsActiveAndDispatchable(t *testing.T) {
	idx := firstItemIndexWhere(t, func(it Item) bool { return it.Label == "Restaurar último respaldo" })
	item := Items[idx]
	if !item.Active {
		t.Fatal("Restaurar último respaldo Active = false, want true")
	}
	if item.Action != ActionRollback {
		t.Fatalf("Restaurar último respaldo Action = %q, want %q", item.Action, ActionRollback)
	}

	got := ActionArgs(item.Action)
	want := []string{"rollback"}
	if len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("ActionArgs(%q) = %v, want %v", item.Action, got, want)
	}
}

// TestItems_NoLongerContainsRemovedInertPlaceholders guards the removal of the two inert
// "coming soon" rows that were dropped entirely (not merely deactivated) by this change:
// "Presets de instalación" and "Sincronizar configuración" no longer exist in Items at all.
func TestItems_NoLongerContainsRemovedInertPlaceholders(t *testing.T) {
	removed := []string{"Presets de instalación", "Sincronizar configuración"}
	for _, label := range removed {
		for _, item := range Items {
			if item.Label == label {
				t.Fatalf("Items still contains removed placeholder %q", label)
			}
		}
	}
}

func TestModel_Update_QRuneQuits(t *testing.T) {
	m := NewModel()
	m, cmd := updateModel(m, keyMsg("q"))
	if m.Chosen != ActionQuit {
		t.Fatalf("Chosen = %q after q, want %q", m.Chosen, ActionQuit)
	}
	if cmd == nil {
		t.Fatal("Update(q) returned a nil tea.Cmd, want tea.Quit")
	}
}

func TestModel_Update_EscAndCtrlCQuit(t *testing.T) {
	for _, keyType := range []tea.KeyType{tea.KeyEsc, tea.KeyCtrlC} {
		m := NewModel()
		m, cmd := updateModel(m, tea.KeyMsg{Type: keyType})
		if m.Chosen != ActionQuit {
			t.Fatalf("Chosen = %q after %v, want %q", m.Chosen, keyType, ActionQuit)
		}
		if cmd == nil {
			t.Fatalf("Update(%v) returned a nil tea.Cmd, want tea.Quit", keyType)
		}
	}
}

func TestModel_Update_IgnoresNonKeyMessages(t *testing.T) {
	m := NewModel()
	before := m
	m, cmd := updateModel(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	if m.Cursor != before.Cursor || m.Chosen != before.Chosen || m.StatusMsg != before.StatusMsg {
		t.Fatalf("Update(non-key msg) mutated state: got %+v, want unchanged %+v", m, before)
	}
	if cmd != nil {
		t.Fatal("Update(non-key msg) returned a non-nil cmd, want nil")
	}
}

// TestModel_View_ShowsSparkLogoAndTagline guards the branded menu header: the spark logo glyph
// (✦) and the Click Seguros tagline must both render, so the menu opens with the brand mark the
// approved design calls for rather than a bare text line.
func TestModel_View_ShowsSparkLogoAndTagline(t *testing.T) {
	view := NewModel().View()
	if !strings.Contains(view, "✦") {
		t.Errorf("View() missing spark logo glyph ✦:\n%s", view)
	}
	if !strings.Contains(view, "AI Devkit · Click Seguros") {
		t.Errorf("View() missing brand tagline:\n%s", view)
	}
}

// TestModel_View_HighlightsActiveRowPointer guards that the row under the cursor renders the
// active-row pointer (▸); with the cursor at its initial row 0 the pointer must be present.
func TestModel_View_HighlightsActiveRowPointer(t *testing.T) {
	if view := NewModel().View(); !strings.Contains(view, "▸") {
		t.Errorf("View() missing active-row pointer ▸:\n%s", view)
	}
}

func TestModel_View_RendersEveryItemLabel(t *testing.T) {
	m := NewModel()
	view := m.View()
	for _, item := range Items {
		if !strings.Contains(view, item.Label) {
			t.Errorf("View() missing item label %q:\n%s", item.Label, view)
		}
	}
}

// TestModel_View_InertItemsShowComingSoonSuffix guards View's inert-row rendering, which no
// current real Items row exercises anymore (see TestModel_Update_EnterOnInertItemShowsStatusAndDoesNotQuit's
// comment) — swaps in a synthetic inert item so this rendering path stays covered.
func TestModel_View_InertItemsShowComingSoonSuffix(t *testing.T) {
	defer setItemsForTest(t, []Item{{Label: "Inert placeholder", Active: false}})()

	m := NewModel()
	view := m.View()
	lines := strings.Split(view, "\n")
	for _, item := range Items {
		if item.Active {
			continue
		}
		found := false
		for _, line := range lines {
			if strings.Contains(line, item.Label) && strings.Contains(line, "(próximamente)") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("View() line for inert item %q missing '(próximamente)' suffix:\n%s", item.Label, view)
		}
	}
}

func TestActionArgs_MapsEachActiveDispatchableAction(t *testing.T) {
	cases := map[string][]string{
		ActionInstall:         {"install"},
		ActionUpdate:          {"update"},
		ActionConfigureModels: {"configure-models"},
		ActionAgentBuilder:    {"agent-builder"},
		ActionDoctor:          {"doctor"},
		ActionUninstall:       {"uninstall"},
		ActionManageBackups:   {"manage-backups"},
		ActionRollback:        {"rollback"},
	}
	for action, want := range cases {
		got := ActionArgs(action)
		if len(got) != len(want) {
			t.Fatalf("ActionArgs(%q) = %v, want %v", action, got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("ActionArgs(%q) = %v, want %v", action, got, want)
			}
		}
	}
}

func TestActionArgs_QuitAndUnknownReturnNil(t *testing.T) {
	for _, action := range []string{ActionQuit, "", "not-a-real-action"} {
		if got := ActionArgs(action); got != nil {
			t.Errorf("ActionArgs(%q) = %v, want nil", action, got)
		}
	}
}

// setItemsForTest swaps the package-level Items with a temporary slice for the duration of a
// test, returning a restore func the caller must defer immediately. Used to exercise code paths
// (like the inert-item branch of selectCurrent/View) that no current real Items row triggers.
func setItemsForTest(t *testing.T, items []Item) func() {
	t.Helper()
	original := Items
	Items = items
	return func() { Items = original }
}

func firstItemIndexWhere(t *testing.T, pred func(Item) bool) int {
	t.Helper()
	for i, item := range Items {
		if pred(item) {
			return i
		}
	}
	t.Fatal("no item in Items matched the predicate")
	return -1
}

func keyMsg(key string) tea.KeyMsg {
	switch key {
	case "j":
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")}
	case "k":
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")}
	case "q":
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
	}
}

// updateModel calls Model.Update and type-asserts the result back, so tests can chain calls
// without repeating the tea.Model interface's boilerplate.
func updateModel(m Model, msg tea.Msg) (Model, tea.Cmd) {
	updated, cmd := m.Update(msg)
	return updated.(Model), cmd
}
