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

func TestAgentBuilderModel_BlankDescriptionMustBeCorrectedBeforeSDDMode(t *testing.T) {
	m := NewAgentBuilderModel([]agentbuilder.Engine{agentbuilder.ClaudeCode})
	m = typeAgentBuilderText(t, m, "   ")
	m, _ = updateAgentBuilderModel(m, keyMsg("enter"))

	if m.Step != StepDescription {
		t.Fatalf("blank description advanced to Step=%v, want StepDescription", m.Step)
	}
	if m.PreviewError == "" {
		t.Fatal("PreviewError empty after blank description, want required-field error")
	}
	if m.Spec.Description != "" || m.Spec.Name != "" {
		t.Fatalf("blank description mutated spec to Description=%q Name=%q, want both empty", m.Spec.Description, m.Spec.Name)
	}

	m = typeAgentBuilderText(t, m, "Review risky database migrations")
	m, _ = updateAgentBuilderModel(m, keyMsg("enter"))
	if m.Step != StepSDDMode || m.Spec.Description != "Review risky database migrations" || m.Spec.Name != "review-risky-database-migrations" || m.PreviewError != "" {
		t.Fatalf("corrected description state = Step %v Description %q Name %q PreviewError %q, want SDD mode/corrected name/no error", m.Step, m.Spec.Description, m.Spec.Name, m.PreviewError)
	}
}

func TestAgentBuilderModel_MultilinePastedDescriptionMustBeCorrectedBeforeSDDMode(t *testing.T) {
	m := NewAgentBuilderModel([]agentbuilder.Engine{agentbuilder.ClaudeCode})
	m = pasteAgentBuilderText(t, m, "Review risky\ndatabase migrations")
	m, _ = updateAgentBuilderModel(m, keyMsg("enter"))

	if m.Step != StepDescription {
		t.Fatalf("multiline description advanced to Step=%v, want StepDescription", m.Step)
	}
	if m.PreviewError == "" || !strings.Contains(m.View(), m.PreviewError) {
		t.Fatalf("multiline description error not visible: PreviewError=%q View=\n%s", m.PreviewError, m.View())
	}
	if m.Spec.Description != "" || m.Spec.Name != "" {
		t.Fatalf("multiline description mutated spec to Description=%q Name=%q, want both empty", m.Spec.Description, m.Spec.Name)
	}

	m = clearAgentBuilderInput(t, m)
	m = pasteAgentBuilderText(t, m, "Review risky database migrations")
	m, _ = updateAgentBuilderModel(m, keyMsg("enter"))
	if m.Step != StepSDDMode || m.Spec.Description != "Review risky database migrations" || m.Spec.Name != "review-risky-database-migrations" || m.PreviewError != "" {
		t.Fatalf("corrected description state = Step %v Description %q Name %q PreviewError %q, want SDD mode/corrected name/no error", m.Step, m.Spec.Description, m.Spec.Name, m.PreviewError)
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

func TestAgentBuilderModel_RequiredThemeFieldsMustBeCorrectedBeforePreview(t *testing.T) {
	m := advanceToThemes(t, NewAgentBuilderModel([]agentbuilder.Engine{agentbuilder.ClaudeCode}))
	for _, answer := range validAgentBuilderThemeAnswers()[:4] {
		m = typeAgentBuilderText(t, m, answer)
		m, _ = updateAgentBuilderModel(m, keyMsg("enter"))
	}
	if m.ThemeIndex != 4 {
		t.Fatalf("ThemeIndex before tools = %d, want 4", m.ThemeIndex)
	}

	m, _ = updateAgentBuilderModel(m, keyMsg("enter"))
	if m.Step != StepThemes || m.ThemeIndex != 4 {
		t.Fatalf("blank tools advanced to Step/ThemeIndex = %v/%d, want StepThemes/4", m.Step, m.ThemeIndex)
	}
	if m.PreviewError == "" {
		t.Fatal("PreviewError empty after blank tools, want required-field error")
	}
	m = typeAgentBuilderText(t, m, "Read, Grep, Bash")
	m, _ = updateAgentBuilderModel(m, keyMsg("enter"))
	if m.Step != StepThemes || m.ThemeIndex != 5 || m.Spec.Tools != "Read, Grep, Bash" {
		t.Fatalf("corrected tools state = Step %v ThemeIndex %d Tools %q, want StepThemes/5/corrected", m.Step, m.ThemeIndex, m.Spec.Tools)
	}

	m, _ = updateAgentBuilderModel(m, keyMsg("enter"))
	if m.Step != StepThemes || m.ThemeIndex != 5 {
		t.Fatalf("blank model advanced to Step/ThemeIndex = %v/%d, want StepThemes/5", m.Step, m.ThemeIndex)
	}
	m = typeAgentBuilderText(t, m, "sonnet")
	m, _ = updateAgentBuilderModel(m, keyMsg("enter"))
	for _, answer := range validAgentBuilderThemeAnswers()[6:] {
		m = typeAgentBuilderText(t, m, answer)
		m, _ = updateAgentBuilderModel(m, keyMsg("enter"))
	}

	if m.Step != StepPreview || m.PreviewError != "" {
		t.Fatalf("corrected required fields ended at Step=%v PreviewError=%q, want preview without error", m.Step, m.PreviewError)
	}
	m, _ = updateAgentBuilderModel(m, keyMsg("enter"))
	m, cmd := updateAgentBuilderModel(m, keyMsg("enter"))
	if !m.Confirmed || m.Step != StepDone || cmd == nil {
		t.Fatalf("corrected required fields did not confirm: Confirmed=%v Step=%v cmd=%v", m.Confirmed, m.Step, cmd)
	}
}

func TestAgentBuilderModel_MultilineRequiredThemeScalarsMustBeCorrectedBeforeAdvancing(t *testing.T) {
	tests := []struct {
		name       string
		themeIndex int
		invalid    string
		corrected  string
		gotField   func(agentbuilder.AgentSpec) string
	}{
		{
			name:       "tools",
			themeIndex: 4,
			invalid:    "Read, Grep\nBash",
			corrected:  "Read, Grep, Bash",
			gotField:   func(spec agentbuilder.AgentSpec) string { return spec.Tools },
		},
		{
			name:       "model",
			themeIndex: 5,
			invalid:    "sonnet\nopus",
			corrected:  "sonnet",
			gotField:   func(spec agentbuilder.AgentSpec) string { return spec.Model },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := advanceToThemes(t, NewAgentBuilderModel([]agentbuilder.Engine{agentbuilder.ClaudeCode}))
			for _, answer := range validAgentBuilderThemeAnswers()[:tt.themeIndex] {
				m = typeAgentBuilderText(t, m, answer)
				m, _ = updateAgentBuilderModel(m, keyMsg("enter"))
			}
			if m.ThemeIndex != tt.themeIndex {
				t.Fatalf("ThemeIndex before %s = %d, want %d", tt.name, m.ThemeIndex, tt.themeIndex)
			}

			m = pasteAgentBuilderText(t, m, tt.invalid)
			m, _ = updateAgentBuilderModel(m, keyMsg("enter"))
			if m.Step != StepThemes || m.ThemeIndex != tt.themeIndex {
				t.Fatalf("multiline %s advanced to Step/ThemeIndex = %v/%d, want StepThemes/%d", tt.name, m.Step, m.ThemeIndex, tt.themeIndex)
			}
			if m.PreviewError == "" || !strings.Contains(m.View(), m.PreviewError) {
				t.Fatalf("multiline %s error not visible: PreviewError=%q View=\n%s", tt.name, m.PreviewError, m.View())
			}
			if got := tt.gotField(m.Spec); got != "" {
				t.Fatalf("multiline %s mutated spec field to %q, want empty", tt.name, got)
			}

			m = clearAgentBuilderInput(t, m)
			m = pasteAgentBuilderText(t, m, tt.corrected)
			m, _ = updateAgentBuilderModel(m, keyMsg("enter"))
			if m.Step != StepThemes || m.ThemeIndex != tt.themeIndex+1 || tt.gotField(m.Spec) != tt.corrected || m.PreviewError != "" {
				t.Fatalf("corrected %s state = Step %v ThemeIndex %d Value %q PreviewError %q, want StepThemes/%d/%q/no error", tt.name, m.Step, m.ThemeIndex, tt.gotField(m.Spec), m.PreviewError, tt.themeIndex+1, tt.corrected)
			}
		})
	}
}

func TestAgentBuilderModel_InvalidGeneratedNameCanBeCorrectedBeforePreview(t *testing.T) {
	m := NewAgentBuilderModel([]agentbuilder.Engine{agentbuilder.ClaudeCode})
	m = typeAgentBuilderText(t, m, "Ñandú migration reviewer")
	m, _ = updateAgentBuilderModel(m, keyMsg("enter"))

	if m.Step != StepDescription {
		t.Fatalf("invalid generated name advanced to Step=%v, want StepDescription", m.Step)
	}
	if m.PreviewError == "" {
		t.Fatal("PreviewError empty after invalid generated name, want validation error")
	}

	m = clearAgentBuilderInput(t, m)
	m = typeAgentBuilderText(t, m, "Nandu migration reviewer")
	m, _ = updateAgentBuilderModel(m, keyMsg("enter"))
	if m.Step != StepSDDMode || m.Spec.Name != "nandu-migration-reviewer" || m.PreviewError != "" {
		t.Fatalf("corrected generated name state = Step %v Name %q PreviewError %q, want SDD mode/nandu-migration-reviewer/no error", m.Step, m.Spec.Name, m.PreviewError)
	}

	m, _ = updateAgentBuilderModel(m, keyMsg("enter"))
	m = completeAnswersToPreview(t, m, validAgentBuilderThemeAnswers())
	m, _ = updateAgentBuilderModel(m, keyMsg("enter"))
	m, cmd := updateAgentBuilderModel(m, keyMsg("enter"))
	if !m.Confirmed || m.Step != StepDone || cmd == nil {
		t.Fatalf("corrected generated name did not confirm: Confirmed=%v Step=%v cmd=%v", m.Confirmed, m.Step, cmd)
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

func TestAgentBuilderModel_EditedPreviewConfirmsAsFinalMarkdown(t *testing.T) {
	m := completeRequiredFieldsToPreview(t, NewAgentBuilderModel([]agentbuilder.Engine{agentbuilder.ClaudeCode}))
	edited := strings.Replace(m.PreviewContent, "Protect production data", "Protect production indexes", 1)

	m, _ = updateAgentBuilderModel(m, keyMsg("down"))
	m, _ = updateAgentBuilderModel(m, keyMsg("enter"))
	m = clearAgentBuilderInput(t, m)
	m = typeAgentBuilderText(t, m, edited)
	m, _ = updateAgentBuilderModel(m, keyMsg("enter"))

	m, _ = updateAgentBuilderModel(m, keyMsg("up"))
	m, _ = updateAgentBuilderModel(m, keyMsg("enter"))
	if m.Step != StepPlacement {
		t.Fatalf("edited native preview install ended at Step=%v PreviewError=%q, want StepPlacement", m.Step, m.PreviewError)
	}
	m, cmd := updateAgentBuilderModel(m, keyMsg("enter"))
	if !m.Confirmed || m.Step != StepDone || cmd == nil {
		t.Fatalf("edited native preview did not confirm: Confirmed=%v Step=%v cmd=%v", m.Confirmed, m.Step, cmd)
	}
	if m.FinalMarkdown != edited {
		t.Fatalf("FinalMarkdown = %q, want edited preview content", m.FinalMarkdown)
	}
}

func TestAgentBuilderModel_InvalidEditedPreviewCannotConfirm(t *testing.T) {
	m := completeRequiredFieldsToPreview(t, NewAgentBuilderModel([]agentbuilder.Engine{agentbuilder.ClaudeCode}))

	m, _ = updateAgentBuilderModel(m, keyMsg("down"))
	m, _ = updateAgentBuilderModel(m, keyMsg("enter"))
	m = clearAgentBuilderInput(t, m)
	m = typeAgentBuilderText(t, m, "edited markdown body")
	m, _ = updateAgentBuilderModel(m, keyMsg("enter"))

	m, _ = updateAgentBuilderModel(m, keyMsg("up"))
	m, cmd := updateAgentBuilderModel(m, keyMsg("enter"))
	if m.Confirmed || m.Step != StepPreview || cmd != nil {
		t.Fatalf("invalid edited preview advanced: Confirmed=%v Step=%v cmd=%v", m.Confirmed, m.Step, cmd)
	}
	if m.PreviewError == "" {
		t.Fatal("PreviewError empty after invalid edited preview, want validation error")
	}
	if m.FinalMarkdown != "" {
		t.Fatalf("FinalMarkdown = %q, want empty for invalid edited preview", m.FinalMarkdown)
	}
	if view := m.View(); !strings.Contains(view, m.PreviewError) {
		t.Fatalf("preview view missing PreviewError %q:\n%s", m.PreviewError, view)
	}
}

func TestAgentBuilderModel_PreviewValidationErrorIsVisible(t *testing.T) {
	t.Run("edited preview validation", func(t *testing.T) {
		m := completeRequiredFieldsToPreview(t, NewAgentBuilderModel([]agentbuilder.Engine{agentbuilder.ClaudeCode}))

		m, _ = updateAgentBuilderModel(m, keyMsg("down"))
		m, _ = updateAgentBuilderModel(m, keyMsg("enter"))
		m = clearAgentBuilderInput(t, m)
		m = typeAgentBuilderText(t, m, "edited markdown body")
		m, _ = updateAgentBuilderModel(m, keyMsg("enter"))

		if m.PreviewError == "" {
			t.Fatal("PreviewError empty after invalid edit, want validation error")
		}
		if view := m.View(); !strings.Contains(view, m.PreviewError) {
			t.Fatalf("preview view missing edited PreviewError %q:\n%s", m.PreviewError, view)
		}
	})

	t.Run("install validation", func(t *testing.T) {
		m := completeRequiredFieldsToPreview(t, NewAgentBuilderModel([]agentbuilder.Engine{agentbuilder.ClaudeCode}))
		m.Spec.Tools = ""

		m, _ = updateAgentBuilderModel(m, keyMsg("enter"))

		if m.PreviewError == "" {
			t.Fatal("PreviewError empty after invalid install, want validation error")
		}
		if view := m.View(); !strings.Contains(view, m.PreviewError) {
			t.Fatalf("preview view missing install PreviewError %q:\n%s", m.PreviewError, view)
		}
	})
}

func TestAgentBuilderModel_EditedPreviewInvalidFrontmatterDomainCannotConfirm(t *testing.T) {
	tests := []struct {
		name string
		edit func(string) string
	}{
		{
			name: "invalid slug name",
			edit: func(content string) string {
				return strings.Replace(content, `name: "review-risky-database-migrations"`, `name: "bad name"`, 1)
			},
		},
		{
			name: "nested required name is not top-level metadata",
			edit: func(content string) string {
				return strings.Replace(content, `name: "review-risky-database-migrations"`, "metadata:\n  name: \"review-risky-database-migrations\"", 1)
			},
		},
		{
			name: "duplicate required name",
			edit: func(content string) string {
				return strings.Replace(content, `description: "Review risky database migrations"`, "description: \"Review risky database migrations\"\nname: \"review-risky-database-migrations-copy\"", 1)
			},
		},
		{
			name: "multiline frontmatter scalar",
			edit: func(content string) string {
				return strings.Replace(content, `model: "sonnet"`, "model: |\n  sonnet", 1)
			},
		},
		{
			name: "comment-only unquoted frontmatter scalar",
			edit: func(content string) string {
				return strings.Replace(content, `model: "sonnet"`, `model: # sonnet`, 1)
			},
		},
		{
			name: "flow sequence frontmatter scalar",
			edit: func(content string) string {
				return strings.Replace(content, `tools: "Read, Grep, Bash"`, `tools: [Read, Grep]`, 1)
			},
		},
		{
			name: "flow map frontmatter scalar",
			edit: func(content string) string {
				return strings.Replace(content, `tools: "Read, Grep, Bash"`, `tools: {Read: true}`, 1)
			},
		},
		{
			name: "plain scalar with colon-space",
			edit: func(content string) string {
				return strings.Replace(content, `description: "Review risky database migrations"`, `description: Release helper: drafts notes`, 1)
			},
		},
		{
			name: "plain scalar with inline comment",
			edit: func(content string) string {
				return strings.Replace(content, `model: "sonnet"`, `model: sonnet # comment`, 1)
			},
		},
		{
			name: "required frontmatter scalar without separator space",
			edit: func(content string) string {
				content = strings.Replace(content, `name: "review-risky-database-migrations"`, `name:bad-name`, 1)
				return strings.Replace(content, `model: "sonnet"`, `model:sonnet`, 1)
			},
		},
		{
			name: "single quoted scalar with unescaped apostrophe",
			edit: func(content string) string {
				return strings.Replace(content, `model: "sonnet"`, `model: 'sonnet's helper'`, 1)
			},
		},
		{
			name: "implicit boolean scalar",
			edit: func(content string) string {
				return strings.Replace(content, `model: "sonnet"`, `model: true`, 1)
			},
		},
		{
			name: "implicit integer scalar",
			edit: func(content string) string {
				return strings.Replace(content, `model: "sonnet"`, `model: 123`, 1)
			},
		},
		{
			name: "implicit null scalar",
			edit: func(content string) string {
				return strings.Replace(content, `model: "sonnet"`, `model: null`, 1)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := completeRequiredFieldsToPreview(t, NewAgentBuilderModel([]agentbuilder.Engine{agentbuilder.ClaudeCode}))
			edited := tt.edit(m.PreviewContent)

			m, _ = updateAgentBuilderModel(m, keyMsg("down"))
			m, _ = updateAgentBuilderModel(m, keyMsg("enter"))
			m = clearAgentBuilderInput(t, m)
			m = typeAgentBuilderText(t, m, edited)
			m, _ = updateAgentBuilderModel(m, keyMsg("enter"))

			m, _ = updateAgentBuilderModel(m, keyMsg("up"))
			m, cmd := updateAgentBuilderModel(m, keyMsg("enter"))
			if m.Confirmed || m.Step != StepPreview || cmd != nil {
				t.Fatalf("invalid frontmatter domain advanced: Confirmed=%v Step=%v cmd=%v", m.Confirmed, m.Step, cmd)
			}
			if m.PreviewError == "" {
				t.Fatal("PreviewError empty after invalid frontmatter domain, want validation error")
			}
			if m.FinalMarkdown != "" {
				t.Fatalf("FinalMarkdown = %q, want empty for invalid frontmatter domain", m.FinalMarkdown)
			}
		})
	}
}

func TestAgentBuilderModel_EditedPreviewValidFrontmatterScalarsStillConfirm(t *testing.T) {
	tests := []struct {
		name string
		edit func(string) string
	}{
		{
			name: "quoted scalars",
			edit: func(content string) string { return content },
		},
		{
			name: "plain safe scalars",
			edit: func(content string) string {
				content = strings.Replace(content, `model: "sonnet"`, `model: sonnet`, 1)
				return strings.Replace(content, `tools: "Read, Grep, Bash"`, `tools: Read, Grep, Bash`, 1)
			},
		},
		{
			name: "single quoted scalar with doubled apostrophe",
			edit: func(content string) string {
				return strings.Replace(content, `model: "sonnet"`, `model: 'sonnet''s helper'`, 1)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := completeRequiredFieldsToPreview(t, NewAgentBuilderModel([]agentbuilder.Engine{agentbuilder.ClaudeCode}))
			edited := tt.edit(m.PreviewContent)

			m, _ = updateAgentBuilderModel(m, keyMsg("down"))
			m, _ = updateAgentBuilderModel(m, keyMsg("enter"))
			m = clearAgentBuilderInput(t, m)
			m = typeAgentBuilderText(t, m, edited)
			m, _ = updateAgentBuilderModel(m, keyMsg("enter"))

			m, _ = updateAgentBuilderModel(m, keyMsg("up"))
			m, _ = updateAgentBuilderModel(m, keyMsg("enter"))
			if m.Step != StepPlacement {
				t.Fatalf("valid frontmatter scalar install ended at Step=%v PreviewError=%q, want StepPlacement", m.Step, m.PreviewError)
			}
			m, cmd := updateAgentBuilderModel(m, keyMsg("enter"))
			if !m.Confirmed || m.Step != StepDone || cmd == nil {
				t.Fatalf("valid frontmatter scalar did not confirm: Confirmed=%v Step=%v cmd=%v", m.Confirmed, m.Step, cmd)
			}
			if m.FinalMarkdown != edited {
				t.Fatalf("FinalMarkdown = %q, want edited preview content", m.FinalMarkdown)
			}
		})
	}
}

func TestAgentBuilderModel_InvalidSpecCannotConfirm(t *testing.T) {
	t.Run("tampered blank required metadata stays at preview", func(t *testing.T) {
		m := completeRequiredFieldsToPreview(t, NewAgentBuilderModel([]agentbuilder.Engine{agentbuilder.ClaudeCode}))
		m.Spec.Tools = ""
		m.PreviewError = ""

		m, cmd := updateAgentBuilderModel(m, keyMsg("enter"))
		if m.Confirmed || m.Step != StepPreview || cmd != nil {
			t.Fatalf("invalid preview install advanced: Confirmed=%v Step=%v cmd=%v", m.Confirmed, m.Step, cmd)
		}
		if m.PreviewError == "" {
			t.Fatal("PreviewError empty for blank tools, want validation error")
		}
		m, cmd = updateAgentBuilderModel(m, keyMsg("enter"))
		if m.Confirmed || cmd != nil {
			t.Fatalf("invalid preview confirmed after repeated enter: Confirmed=%v cmd=%v", m.Confirmed, cmd)
		}
	})

	t.Run("tampered invalid generated name stays at preview", func(t *testing.T) {
		m := completeRequiredFieldsToPreview(t, NewAgentBuilderModel([]agentbuilder.Engine{agentbuilder.ClaudeCode}))
		m.Spec.Name = "ñandú-migration-reviewer"
		m.PreviewError = ""

		m, cmd := updateAgentBuilderModel(m, keyMsg("enter"))
		if m.Confirmed || m.Step != StepPreview || cmd != nil {
			t.Fatalf("invalid name install advanced: Confirmed=%v Step=%v cmd=%v", m.Confirmed, m.Step, cmd)
		}
		if m.PreviewError == "" {
			t.Fatal("PreviewError empty for accented generated name, want validation error")
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

func pasteAgentBuilderText(t *testing.T, m AgentBuilderModel, text string) AgentBuilderModel {
	t.Helper()
	m, _ = updateAgentBuilderModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(text)})
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
