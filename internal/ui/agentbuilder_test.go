package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/agentbuilder"
	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/modelconfig"
)

func TestAgentBuilderModel_OneEngineStartsAtDescription(t *testing.T) {
	m := NewAgentBuilderModel([]agentbuilder.Engine{agentbuilder.ClaudeCode})

	if m.Step != StepDescription {
		t.Fatalf("Step = %v, want StepDescription", m.Step)
	}
	if m.Spec.Engine.ID != agentbuilder.ClaudeCode.ID {
		t.Fatalf("Spec.Engine.ID = %q, want %q", m.Spec.Engine.ID, agentbuilder.ClaudeCode.ID)
	}
	if m.Confirmed || m.Cancelled {
		t.Fatalf("initial Confirmed=%v Cancelled=%v, want both false", m.Confirmed, m.Cancelled)
	}
}

func TestAgentBuilderModel_TwoEnginesStartsAtEngineAndConfirmsSelection(t *testing.T) {
	engines := []agentbuilder.Engine{
		{ID: "claude-code", Label: "Claude Code"},
		{ID: "future-engine", Label: "Future Engine"},
	}
	m := NewAgentBuilderModel(engines)

	if m.Step != StepEngine {
		t.Fatalf("Step = %v, want StepEngine", m.Step)
	}
	view := m.View()
	for _, label := range []string{"Claude Code", "Future Engine"} {
		if !strings.Contains(view, label) {
			t.Fatalf("View() missing engine %q:\n%s", label, view)
		}
	}

	m, _ = updateAgentBuilderModel(m, keyMsg("down"))
	m, _ = updateAgentBuilderModel(m, keyMsg("enter"))
	if m.Step != StepDescription {
		t.Fatalf("Step after engine confirm = %v, want StepDescription", m.Step)
	}
	if m.Spec.Engine.ID != "future-engine" {
		t.Fatalf("selected engine ID = %q, want future-engine", m.Spec.Engine.ID)
	}
}

func TestAgentBuilderModel_DescriptionCapturesFreeTextBeforeThemes(t *testing.T) {
	m := NewAgentBuilderModel([]agentbuilder.Engine{agentbuilder.ClaudeCode})
	if strings.Contains(m.View(), "Propósito") {
		t.Fatalf("description view rendered themed prompts too early:\n%s", m.View())
	}

	m = typeAgentBuilderText(t, m, "Review risky database migrations")
	m, _ = updateAgentBuilderModel(m, keyMsg("enter"))

	if got := m.Spec.Description; got != "Review risky database migrations" {
		t.Fatalf("Spec.Description = %q, want captured free text", got)
	}
	if m.Step != StepSDDMode {
		t.Fatalf("Step after description = %v, want StepSDDMode", m.Step)
	}
	if strings.Contains(m.View(), "Propósito") {
		t.Fatalf("SDD mode view rendered themed prompts too early:\n%s", m.View())
	}
}

func TestAgentBuilderModel_TextEntryStatesCaptureLowercaseQ(t *testing.T) {
	t.Run("description", func(t *testing.T) {
		m := NewAgentBuilderModel([]agentbuilder.Engine{agentbuilder.ClaudeCode})

		m, cmd := updateAgentBuilderModel(m, keyMsg("q"))

		if m.Cancelled || cmd != nil {
			t.Fatalf("lowercase q cancelled description input: Cancelled=%v cmd=%v", m.Cancelled, cmd)
		}
		if m.input != "q" {
			t.Fatalf("input = %q, want q", m.input)
		}
	})

	t.Run("themes", func(t *testing.T) {
		m := advanceToThemes(t, NewAgentBuilderModel([]agentbuilder.Engine{agentbuilder.ClaudeCode}))

		m, cmd := updateAgentBuilderModel(m, keyMsg("q"))

		if m.Cancelled || cmd != nil {
			t.Fatalf("lowercase q cancelled theme input: Cancelled=%v cmd=%v", m.Cancelled, cmd)
		}
		if m.input != "q" {
			t.Fatalf("input = %q, want q", m.input)
		}
	})

	t.Run("preview edit", func(t *testing.T) {
		m := completeRequiredFieldsToPreview(t, NewAgentBuilderModel([]agentbuilder.Engine{agentbuilder.ClaudeCode}))
		m, _ = updateAgentBuilderModel(m, keyMsg("down"))
		m, _ = updateAgentBuilderModel(m, keyMsg("enter"))
		before := m.input

		m, cmd := updateAgentBuilderModel(m, keyMsg("q"))

		if m.Cancelled || cmd != nil {
			t.Fatalf("lowercase q cancelled preview edit: Cancelled=%v cmd=%v", m.Cancelled, cmd)
		}
		if m.input != before+"q" {
			t.Fatalf("input suffix = %q, want %q", m.input, before+"q")
		}
	})
}

func TestAgentBuilderModel_SDDModeOffersTwoOptionsAndNoNewPhase(t *testing.T) {
	m := advancePastDescription(t, NewAgentBuilderModel([]agentbuilder.Engine{agentbuilder.ClaudeCode}))

	view := m.View()
	for _, label := range []string{"Standalone", "Phase Support"} {
		if !strings.Contains(view, label) {
			t.Fatalf("View() missing SDD mode %q:\n%s", label, view)
		}
	}
	for _, forbidden := range []string{"New Phase", "Nueva fase"} {
		if strings.Contains(view, forbidden) {
			t.Fatalf("View() exposed forbidden mode %q:\n%s", forbidden, view)
		}
	}

	m, _ = updateAgentBuilderModel(m, keyMsg("down"))
	m, _ = updateAgentBuilderModel(m, keyMsg("down"))
	m, _ = updateAgentBuilderModel(m, keyMsg("enter"))
	if m.Spec.SDDMode != agentbuilder.SDDStandalone {
		t.Fatalf("SDDMode after wrapping two options = %q, want %q", m.Spec.SDDMode, agentbuilder.SDDStandalone)
	}
	if m.Step != StepThemes {
		t.Fatalf("Step after standalone confirm = %v, want StepThemes", m.Step)
	}
}

func TestAgentBuilderModel_PhaseStepOnlyForPhaseSupportAndReadsModelconfigPhases(t *testing.T) {
	t.Run("standalone skips phase", func(t *testing.T) {
		m := advancePastDescription(t, NewAgentBuilderModel([]agentbuilder.Engine{agentbuilder.ClaudeCode}))
		m, _ = updateAgentBuilderModel(m, keyMsg("enter"))
		if m.Step != StepThemes {
			t.Fatalf("Step after standalone SDD mode = %v, want StepThemes", m.Step)
		}
		if m.Spec.Phase != "" {
			t.Fatalf("Spec.Phase = %q, want empty for standalone", m.Spec.Phase)
		}
	})

	t.Run("phase support shows read-only phase picklist", func(t *testing.T) {
		before := append([]modelconfig.Phase(nil), modelconfig.Phases...)
		m := advancePastDescription(t, NewAgentBuilderModel([]agentbuilder.Engine{agentbuilder.ClaudeCode}))
		m, _ = updateAgentBuilderModel(m, keyMsg("down"))
		m, _ = updateAgentBuilderModel(m, keyMsg("enter"))
		if m.Step != StepPhase {
			t.Fatalf("Step after phase-support SDD mode = %v, want StepPhase", m.Step)
		}
		if !strings.Contains(m.View(), string(modelconfig.Phases[0])) {
			t.Fatalf("phase view missing first modelconfig phase %q:\n%s", modelconfig.Phases[0], m.View())
		}
		m, _ = updateAgentBuilderModel(m, keyMsg("down"))
		m, _ = updateAgentBuilderModel(m, keyMsg("enter"))
		if m.Spec.Phase != modelconfig.Phases[1] {
			t.Fatalf("Spec.Phase = %q, want %q", m.Spec.Phase, modelconfig.Phases[1])
		}
		if !samePhases(before, modelconfig.Phases) {
			t.Fatalf("modelconfig.Phases mutated: got %#v, want %#v", modelconfig.Phases, before)
		}
	})
}

func TestAgentBuilderModel_ThemesPopulateSpecSequentially(t *testing.T) {
	m := advanceToThemes(t, NewAgentBuilderModel([]agentbuilder.Engine{agentbuilder.ClaudeCode}))
	answers := []string{
		"Protect production data",
		"Review SQL migrations and rollback plans",
		"Before merging schema changes",
		"Never approve destructive migrations without backup",
		"Read, Grep, Bash",
		"sonnet",
		"Direct and careful",
		"Postgres, RLS, Supabase migrations",
		"A concise risk report with blockers",
	}
	for i, answer := range answers {
		if m.Step != StepThemes {
			t.Fatalf("before answer %d, Step = %v, want StepThemes", i, m.Step)
		}
		if m.ThemeIndex != i {
			t.Fatalf("before answer %d, ThemeIndex = %d", i, m.ThemeIndex)
		}
		m = typeAgentBuilderText(t, m, answer)
		m, _ = updateAgentBuilderModel(m, keyMsg("enter"))
	}

	if m.Step != StepPreview {
		t.Fatalf("Step after 9 themed answers = %v, want StepPreview", m.Step)
	}
	want := agentbuilder.AgentSpec{
		Purpose:    answers[0],
		Tasks:      answers[1],
		Triggers:   answers[2],
		Rules:      answers[3],
		Tools:      answers[4],
		Model:      answers[5],
		Tone:       answers[6],
		Domain:     answers[7],
		GoodOutput: answers[8],
	}
	if m.Spec.Purpose != want.Purpose || m.Spec.Tasks != want.Tasks || m.Spec.Triggers != want.Triggers ||
		m.Spec.Rules != want.Rules || m.Spec.Tools != want.Tools || m.Spec.Model != want.Model ||
		m.Spec.Tone != want.Tone || m.Spec.Domain != want.Domain || m.Spec.GoodOutput != want.GoodOutput {
		t.Fatalf("theme fields not populated correctly: got %+v, want fields from %+v", m.Spec, want)
	}
	if !strings.Contains(m.PreviewContent, "Protect production data") {
		t.Fatalf("PreviewContent missing rendered purpose:\n%s", m.PreviewContent)
	}
}

func TestAgentBuilderModel_PreviewActionsAndEditUsesEditedText(t *testing.T) {
	m := completeRequiredFieldsToPreview(t, NewAgentBuilderModel([]agentbuilder.Engine{agentbuilder.ClaudeCode}))
	view := m.View()
	for _, action := range []string{"Instalar", "Editar", "Regenerar", "Volver"} {
		if !strings.Contains(view, action) {
			t.Fatalf("preview view missing action %q:\n%s", action, view)
		}
	}
	original := m.PreviewContent

	m, _ = updateAgentBuilderModel(m, keyMsg("down"))
	m, _ = updateAgentBuilderModel(m, keyMsg("enter"))
	if !m.EditingPreview {
		t.Fatal("EditingPreview = false after choosing Edit, want true")
	}
	if m.input != original {
		t.Fatalf("preview edit input = %q, want existing preview content", m.input)
	}
	m, _ = updateAgentBuilderModel(m, keyMsg("enter"))
	if m.PreviewContent != original {
		t.Fatalf("PreviewContent after bare edit enter = %q, want original", m.PreviewContent)
	}

	m, _ = updateAgentBuilderModel(m, keyMsg("enter"))
	m = clearAgentBuilderInput(t, m)
	m = typeAgentBuilderText(t, m, "edited markdown body")
	m, _ = updateAgentBuilderModel(m, keyMsg("enter"))
	if m.EditingPreview {
		t.Fatal("EditingPreview = true after committing edited preview, want false")
	}
	if m.PreviewContent != "edited markdown body" {
		t.Fatalf("PreviewContent = %q, want edited markdown body", m.PreviewContent)
	}

	m, _ = updateAgentBuilderModel(m, keyMsg("down"))
	m, _ = updateAgentBuilderModel(m, keyMsg("enter"))
	if m.PreviewContent != original {
		t.Fatalf("PreviewContent after regenerate = %q, want original generated draft", m.PreviewContent)
	}

	m, _ = updateAgentBuilderModel(m, keyMsg("down"))
	m, _ = updateAgentBuilderModel(m, keyMsg("enter"))
	if m.Step != StepThemes || m.ThemeIndex != 8 {
		t.Fatalf("Step/ThemeIndex after Back = %v/%d, want StepThemes/8", m.Step, m.ThemeIndex)
	}
}

func TestAgentBuilderModel_InvalidSpecCannotConfirm(t *testing.T) {
	t.Run("blank required metadata stays at preview", func(t *testing.T) {
		m := completeAnswersToPreview(t, advanceToThemes(t, NewAgentBuilderModel([]agentbuilder.Engine{agentbuilder.ClaudeCode})), []string{
			"Protect production data",
			"Review SQL migrations and rollback plans",
			"Before merging schema changes",
			"Never approve destructive migrations without backup",
			"",
			"sonnet",
			"Direct and careful",
			"Postgres, RLS, Supabase migrations",
			"A concise risk report with blockers",
		})

		if m.PreviewError == "" {
			t.Fatal("PreviewError empty for blank tools, want validation error")
		}
		m, cmd := updateAgentBuilderModel(m, keyMsg("enter"))
		if m.Confirmed || m.Step != StepPreview || cmd != nil {
			t.Fatalf("invalid preview install advanced: Confirmed=%v Step=%v cmd=%v", m.Confirmed, m.Step, cmd)
		}
		m, cmd = updateAgentBuilderModel(m, keyMsg("enter"))
		if m.Confirmed || cmd != nil {
			t.Fatalf("invalid preview confirmed after repeated enter: Confirmed=%v cmd=%v", m.Confirmed, cmd)
		}
	})

	t.Run("invalid generated name stays at preview", func(t *testing.T) {
		m := NewAgentBuilderModel([]agentbuilder.Engine{agentbuilder.ClaudeCode})
		m = typeAgentBuilderText(t, m, "Ñandú migration reviewer")
		m, _ = updateAgentBuilderModel(m, keyMsg("enter"))
		m, _ = updateAgentBuilderModel(m, keyMsg("enter"))
		m = completeAnswersToPreview(t, m, validAgentBuilderThemeAnswers())

		if m.PreviewError == "" {
			t.Fatal("PreviewError empty for accented generated name, want validation error")
		}
		m, cmd := updateAgentBuilderModel(m, keyMsg("enter"))
		if m.Confirmed || m.Step != StepPreview || cmd != nil {
			t.Fatalf("invalid name install advanced: Confirmed=%v Step=%v cmd=%v", m.Confirmed, m.Step, cmd)
		}
		m, cmd = updateAgentBuilderModel(m, keyMsg("enter"))
		if m.Confirmed || cmd != nil {
			t.Fatalf("invalid name confirmed after repeated enter: Confirmed=%v cmd=%v", m.Confirmed, cmd)
		}
	})

	t.Run("valid spec can confirm", func(t *testing.T) {
		m := completeRequiredFieldsToPreview(t, NewAgentBuilderModel([]agentbuilder.Engine{agentbuilder.ClaudeCode}))
		if m.PreviewError != "" {
			t.Fatalf("PreviewError = %q, want empty", m.PreviewError)
		}
		m, _ = updateAgentBuilderModel(m, keyMsg("enter"))
		m, cmd := updateAgentBuilderModel(m, keyMsg("enter"))
		if !m.Confirmed || m.Step != StepDone || cmd == nil {
			t.Fatalf("valid placement confirm failed: Confirmed=%v Step=%v cmd=%v", m.Confirmed, m.Step, cmd)
		}
	})
}

func TestAgentBuilderModel_PlacementConfirmsOnlyAfterExplicitInstallAndSupportsCancel(t *testing.T) {
	t.Run("install then personal placement confirms", func(t *testing.T) {
		m := completeRequiredFieldsToPreview(t, NewAgentBuilderModel([]agentbuilder.Engine{agentbuilder.ClaudeCode}))
		if m.Confirmed {
			t.Fatal("Confirmed = true at preview before Install, want false")
		}
		m, _ = updateAgentBuilderModel(m, keyMsg("enter"))
		if m.Step != StepPlacement {
			t.Fatalf("Step after Install action = %v, want StepPlacement", m.Step)
		}
		if m.Confirmed {
			t.Fatal("Confirmed = true before placement confirmation, want false")
		}
		m, cmd := updateAgentBuilderModel(m, keyMsg("enter"))
		if !m.Confirmed {
			t.Fatal("Confirmed = false after placement confirmation, want true")
		}
		if m.Spec.Placement != agentbuilder.PlacementPersonal {
			t.Fatalf("Placement = %q, want %q", m.Spec.Placement, agentbuilder.PlacementPersonal)
		}
		if cmd == nil {
			t.Fatal("Update(placement enter) returned nil cmd, want tea.Quit")
		}
	})

	t.Run("shareable placement and cancel", func(t *testing.T) {
		m := completeRequiredFieldsToPreview(t, NewAgentBuilderModel([]agentbuilder.Engine{agentbuilder.ClaudeCode}))
		m, _ = updateAgentBuilderModel(m, keyMsg("enter"))
		m, _ = updateAgentBuilderModel(m, keyMsg("down"))
		m, cmd := updateAgentBuilderModel(m, keyMsg("enter"))
		if !m.Confirmed {
			t.Fatal("Confirmed = false after shareable placement confirmation, want true")
		}
		if m.Spec.Placement != agentbuilder.PlacementShareable {
			t.Fatalf("Placement = %q, want %q", m.Spec.Placement, agentbuilder.PlacementShareable)
		}
		if cmd == nil {
			t.Fatal("Update(shareable enter) returned nil cmd, want tea.Quit")
		}

		cancelModel := completeRequiredFieldsToPreview(t, NewAgentBuilderModel([]agentbuilder.Engine{agentbuilder.ClaudeCode}))
		cancelModel, cmd = updateAgentBuilderModel(cancelModel, keyMsg("esc"))
		if !cancelModel.Cancelled || cancelModel.Confirmed {
			t.Fatalf("after cancel Confirmed=%v Cancelled=%v, want false/true", cancelModel.Confirmed, cancelModel.Cancelled)
		}
		if cmd == nil {
			t.Fatal("Update(esc) returned nil cmd, want tea.Quit")
		}
	})
}

func updateAgentBuilderModel(m AgentBuilderModel, msg tea.Msg) (AgentBuilderModel, tea.Cmd) {
	updated, cmd := m.Update(msg)
	return updated.(AgentBuilderModel), cmd
}

func typeAgentBuilderText(t *testing.T, m AgentBuilderModel, text string) AgentBuilderModel {
	t.Helper()
	for _, r := range text {
		m, _ = updateAgentBuilderModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	return m
}

func clearAgentBuilderInput(t *testing.T, m AgentBuilderModel) AgentBuilderModel {
	t.Helper()
	for range []rune(m.input) {
		m, _ = updateAgentBuilderModel(m, tea.KeyMsg{Type: tea.KeyBackspace})
	}
	return m
}

func advancePastDescription(t *testing.T, m AgentBuilderModel) AgentBuilderModel {
	t.Helper()
	m = typeAgentBuilderText(t, m, "Review risky database migrations")
	m, _ = updateAgentBuilderModel(m, keyMsg("enter"))
	return m
}

func advanceToThemes(t *testing.T, m AgentBuilderModel) AgentBuilderModel {
	t.Helper()
	m = advancePastDescription(t, m)
	m, _ = updateAgentBuilderModel(m, keyMsg("enter"))
	return m
}

func completeRequiredFieldsToPreview(t *testing.T, m AgentBuilderModel) AgentBuilderModel {
	t.Helper()
	return completeAnswersToPreview(t, advanceToThemes(t, m), validAgentBuilderThemeAnswers())
}

func completeAnswersToPreview(t *testing.T, m AgentBuilderModel, answers []string) AgentBuilderModel {
	t.Helper()
	for _, answer := range answers {
		m = typeAgentBuilderText(t, m, answer)
		m, _ = updateAgentBuilderModel(m, keyMsg("enter"))
	}
	return m
}

func validAgentBuilderThemeAnswers() []string {
	return []string{
		"Protect production data",
		"Review SQL migrations and rollback plans",
		"Before merging schema changes",
		"Never approve destructive migrations without backup",
		"Read, Grep, Bash",
		"sonnet",
		"Direct and careful",
		"Postgres, RLS, Supabase migrations",
		"A concise risk report with blockers",
	}
}

func samePhases(a, b []modelconfig.Phase) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
