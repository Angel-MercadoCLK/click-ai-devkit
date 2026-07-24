package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/modelconfig"
)

// TestInstallWizardModel_ForwardNavigation_EnterEnterProducesSameResultAsOldTwoProgramFlow proves
// composition didn't change the final contract: pressing enter on the profile page then enter on
// the model page (with no edits) must produce exactly the profile + per-phase model map the old
// profileProgram-then-modelProgram flow (runInstallSelectTUI, pre-wizard) would have produced.
func TestInstallWizardModel_ForwardNavigation_EnterEnterProducesSameResultAsOldTwoProgramFlow(t *testing.T) {
	m := NewInstallWizardModel(modelconfig.ProfileQuality)
	if m.Step != InstallWizardStepProfile {
		t.Fatalf("initial Step = %v, want InstallWizardStepProfile", m.Step)
	}

	m, _ = updateWizardModel(m, keyMsg("enter")) // confirm profile (quality preselected)
	if m.Step != InstallWizardStepModel {
		t.Fatalf("Step after confirming profile = %v, want InstallWizardStepModel", m.Step)
	}

	wantModels := NewModelSelectModelForProfile(modelconfig.ProfileQuality).Selection

	m, cmd := updateWizardModel(m, keyMsg("enter")) // confirm model defaults
	if !m.Confirmed || m.Cancelled {
		t.Fatalf("final state = %+v, want Confirmed=true Cancelled=false", m)
	}
	if cmd == nil {
		t.Fatal("Update(enter) on the final step returned a nil tea.Cmd, want tea.Quit")
	}
	if m.Profile.Selected != modelconfig.ProfileQuality {
		t.Fatalf("Profile.Selected = %q, want %q", m.Profile.Selected, modelconfig.ProfileQuality)
	}
	for phase, model := range wantModels {
		if got := m.Model.Selection[phase]; got != model {
			t.Errorf("Model.Selection[%s] = %q, want %q", phase, got, model)
		}
	}
}

// TestInstallWizardModel_EscOnModelStep_ReturnsToProfileWithSelectionPreserved guards the core UX
// fix: esc on step 2 goes BACK to step 1 instead of cancelling the whole wizard, and the profile
// choice already made must survive the round trip untouched.
func TestInstallWizardModel_EscOnModelStep_ReturnsToProfileWithSelectionPreserved(t *testing.T) {
	m := NewInstallWizardModel(modelconfig.ProfileCostSaver)
	m, _ = updateWizardModel(m, keyMsg("down")) // move cursor from cost-saver to quality
	if m.Profile.Selected != modelconfig.ProfileQuality {
		t.Fatalf("precondition Profile.Selected = %q, want %q", m.Profile.Selected, modelconfig.ProfileQuality)
	}
	m, _ = updateWizardModel(m, keyMsg("enter")) // confirm -> Model step
	if m.Step != InstallWizardStepModel {
		t.Fatalf("Step after confirm = %v, want InstallWizardStepModel", m.Step)
	}

	m, cmd := updateWizardModel(m, keyMsg("esc")) // back to Profile
	if m.Step != InstallWizardStepProfile {
		t.Fatalf("Step after esc on step 2 = %v, want InstallWizardStepProfile", m.Step)
	}
	if m.Cancelled {
		t.Fatal("Cancelled = true after esc on step 2, want false (back, not cancel)")
	}
	if cmd != nil {
		t.Fatal("Update(esc) on step 2 returned a non-nil tea.Cmd, want nil (still running, not quitting)")
	}
	if m.Profile.Selected != modelconfig.ProfileQuality {
		t.Fatalf("Profile.Selected after back = %q, want preserved %q", m.Profile.Selected, modelconfig.ProfileQuality)
	}
}

// TestInstallWizardModel_QOnModelStep_ReturnsToProfileInsteadOfCancelling mirrors the esc case for
// 'q' — ModelSelectModel treats both identically as Cancelled, and both must mean "back" here, not
// "cancel", once past the first page.
func TestInstallWizardModel_QOnModelStep_ReturnsToProfileInsteadOfCancelling(t *testing.T) {
	m := NewInstallWizardModel(modelconfig.ProfileBalanced)
	m, _ = updateWizardModel(m, keyMsg("enter")) // -> Model step
	m, _ = updateWizardModel(m, keyMsg("q"))
	if m.Step != InstallWizardStepProfile {
		t.Fatalf("Step after q on step 2 = %v, want InstallWizardStepProfile", m.Step)
	}
	if m.Cancelled {
		t.Fatal("Cancelled = true after q on step 2, want false (back, not cancel)")
	}
}

// TestInstallWizardModel_EscOnProfileStep_CancelsWholeWizard guards that only the FIRST page's
// esc/q aborts the whole flow (Cancelled=true), matching today's cancel-means-zero-changes contract.
func TestInstallWizardModel_EscOnProfileStep_CancelsWholeWizard(t *testing.T) {
	m := NewInstallWizardModel(modelconfig.ProfileBalanced)
	m, cmd := updateWizardModel(m, keyMsg("esc"))
	if !m.Cancelled {
		t.Fatal("Cancelled = false after esc on step 1, want true")
	}
	if m.Confirmed {
		t.Fatal("Confirmed = true after esc on step 1, want false")
	}
	if cmd == nil {
		t.Fatal("Update(esc) on step 1 returned a nil tea.Cmd, want tea.Quit")
	}
}

func TestInstallWizardModel_QOnProfileStep_CancelsWholeWizard(t *testing.T) {
	m := NewInstallWizardModel(modelconfig.ProfileBalanced)
	m, cmd := updateWizardModel(m, keyMsg("q"))
	if !m.Cancelled {
		t.Fatal("Cancelled = false after q on step 1, want true")
	}
	if cmd == nil {
		t.Fatal("Update(q) on step 1 returned a nil tea.Cmd, want tea.Quit")
	}
}

// TestInstallWizardModel_CtrlCAlwaysHardCancelsRegardlessOfStep guards a deliberate, narrow
// departure from "esc/q means back on step 2+": ctrl+c stays a universal, unconditional hard-kill
// on every step, matching terminal convention, instead of being repurposed as "back" like esc/q.
func TestInstallWizardModel_CtrlCAlwaysHardCancelsRegardlessOfStep(t *testing.T) {
	m := NewInstallWizardModel(modelconfig.ProfileBalanced)
	m, _ = updateWizardModel(m, keyMsg("enter")) // -> Model step
	m, cmd := updateWizardModel(m, tea.KeyMsg{Type: tea.KeyCtrlC})
	if !m.Cancelled {
		t.Fatal("Cancelled = false after ctrl+c on step 2, want true (hard cancel)")
	}
	if cmd == nil {
		t.Fatal("Update(ctrl+c) returned a nil tea.Cmd, want tea.Quit")
	}
}

// TestInstallWizardModel_ReenteringModelStepReseedsFromNewlyConfirmedProfile documents and locks
// in the chosen re-seeding semantics: going back to Profile and confirming a (possibly different)
// profile again re-seeds the Model step from scratch, matching the old two-program flow's own
// behavior (a fresh NewModelSelectModelForProfile every time profile-select is confirmed) rather
// than trying to merge in whatever the developer had tweaked on a discarded forward attempt.
func TestInstallWizardModel_ReenteringModelStepReseedsFromNewlyConfirmedProfile(t *testing.T) {
	m := NewInstallWizardModel(modelconfig.ProfileQuality)
	m, _ = updateWizardModel(m, keyMsg("enter")) // -> Model (quality preset: explore=opus)

	explorePhase := modelconfig.Phases[0]
	if m.Model.Selection[explorePhase] != "opus" {
		t.Fatalf("precondition Model.Selection[%s] = %q, want opus (quality preset)", explorePhase, m.Model.Selection[explorePhase])
	}
	m, _ = updateWizardModel(m, keyMsg("right")) // opus -> sonnet
	m, _ = updateWizardModel(m, keyMsg("right")) // sonnet -> haiku
	if m.Model.Selection[explorePhase] != "haiku" {
		t.Fatalf("Model.Selection[%s] after two rights = %q, want haiku", explorePhase, m.Model.Selection[explorePhase])
	}

	m, _ = updateWizardModel(m, keyMsg("esc"))  // back to Profile (quality still selected)
	m, _ = updateWizardModel(m, keyMsg("down")) // move cursor to custom
	if m.Profile.Selected != modelconfig.ProfileCustom {
		t.Fatalf("precondition Profile.Selected = %q, want %q", m.Profile.Selected, modelconfig.ProfileCustom)
	}
	m, _ = updateWizardModel(m, keyMsg("enter")) // confirm custom -> Model reseeded to Defaults()

	want := modelconfig.Defaults()[explorePhase]
	if got := m.Model.Selection[explorePhase]; got != want {
		t.Fatalf("Model.Selection[%s] after re-seeding = %q, want fresh default %q (earlier tweak must not survive)", explorePhase, got, want)
	}
}

// TestInstallWizardModel_View_ContainsComposedSubModelView proves the wizard composes the real
// sub-models rather than reimplementing their rendering: its View() at each step must still
// contain the exact string the standalone sub-model's own View() would render, plus wizard chrome
// (a "Paso X de N" step indicator).
func TestInstallWizardModel_View_ContainsComposedSubModelView(t *testing.T) {
	m := NewInstallWizardModel(modelconfig.ProfileBalanced)
	profileView := m.Profile.View()
	wizardView := m.View()
	if !strings.Contains(wizardView, profileView) {
		t.Errorf("wizard View() at step 1 does not contain ProfileSelectModel's own View():\nwizard:\n%s\nsub:\n%s", wizardView, profileView)
	}
	if !strings.Contains(wizardView, "Paso 1 de 2") {
		t.Errorf("wizard View() at step 1 missing step indicator, got:\n%s", wizardView)
	}

	m, _ = updateWizardModel(m, keyMsg("enter"))
	modelView := m.Model.View()
	wizardView = m.View()
	if !strings.Contains(wizardView, modelView) {
		t.Errorf("wizard View() at step 2 does not contain ModelSelectModel's own View():\nwizard:\n%s\nsub:\n%s", wizardView, modelView)
	}
	if !strings.Contains(wizardView, "Paso 2 de 2") {
		t.Errorf("wizard View() at step 2 missing step indicator, got:\n%s", wizardView)
	}
}

func TestInstallWizardModel_Update_IgnoresNonKeyMessages(t *testing.T) {
	m := NewInstallWizardModel(modelconfig.ProfileBalanced)
	before := m
	m, cmd := updateWizardModel(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	if m.Step != before.Step || m.Confirmed != before.Confirmed || m.Cancelled != before.Cancelled {
		t.Fatalf("Update(non-key msg) mutated state: got %+v, want unchanged %+v", m, before)
	}
	if cmd != nil {
		t.Fatalf("Update(non-key msg) returned a non-nil cmd, want nil")
	}
}

func updateWizardModel(m InstallWizardModel, msg tea.Msg) (InstallWizardModel, tea.Cmd) {
	updated, cmd := m.Update(msg)
	return updated.(InstallWizardModel), cmd
}
