package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/modelconfig"
)

// InstallWizardStep enumerates the pages of `click install`'s model-selection wizard, in display
// order. Kept as an ordered int enum — mirroring AgentBuilderModel's own Step type in
// agentbuilder.go — so a future page (e.g. a Codex per-phase model/reasoning-effort picker, still
// pending a confirmed model catalog) can be added later by inserting one more constant plus one
// more case in Update/View, without restructuring this type.
type InstallWizardStep int

const (
	InstallWizardStepProfile InstallWizardStep = iota
	InstallWizardStepModel
	// InstallWizardStepDone is the terminal state reached the instant the Model step is confirmed;
	// it is never actually rendered (the program quits the same tea.Update call that sets it).
	InstallWizardStepDone
)

// installWizardStepCount is the number of interactive pages shown to the developer (excludes the
// terminal Done state), used for the "Paso X de N" chrome indicator.
const installWizardStepCount = 2

// InstallWizardModel composes ProfileSelectModel and ModelSelectModel behind a single tea.Model so
// `click install`'s profile-select-then-model-select sequence runs as ONE alt-screen program
// instead of two separate non-alt-screen programs, each leaving its final frame in the terminal
// scrollback. It owns only step/navigation orchestration: every cursor/selection/cycling rule stays
// inside the composed sub-models, delegated to unchanged, and Update/View dispatch by Step exactly
// like AgentBuilderModel's own updateX/renderX split in agentbuilder.go.
//
// Navigation contract (the actual UX fix this model exists for):
//   - Enter on a page confirms that page's current selection and advances to the next one.
//   - Esc or 'q' on the Model step (page 2) goes BACK to the Profile step (page 1), preserving
//     whatever profile was already chosen there — it does NOT cancel the wizard.
//   - Esc or 'q' on the Profile step (page 1) cancels the WHOLE wizard (Cancelled=true), matching
//     the pre-wizard behavior of cancelling out of the very first screen.
//   - Ctrl+C always hard-cancels the whole wizard immediately, regardless of the current step —
//     unlike esc/q, it is never repurposed as "back", matching universal terminal convention.
//
// Target-select is deliberately NOT a step of this wizard (see internal/cli/install.go's
// runInstallSelectTUI for why): it keeps its own single, separate, non-alt-screen program.
type InstallWizardModel struct {
	Step InstallWizardStep

	Profile ProfileSelectModel
	Model   ModelSelectModel

	Confirmed bool
	Cancelled bool
}

// NewInstallWizardModel builds the wizard seeded on initialProfile (carrying forward the C2 fix
// from profileselect.go verbatim): the Profile page's cursor starts on initialProfile, falling back
// to balanced for an empty/unrecognized name, matching NewProfileSelectModelForProfile's own rule.
func NewInstallWizardModel(initialProfile modelconfig.ProfileName) InstallWizardModel {
	return InstallWizardModel{
		Step:    InstallWizardStepProfile,
		Profile: NewProfileSelectModelForProfile(initialProfile),
	}
}

// Init satisfies tea.Model. Neither composed sub-model needs a startup command.
func (m InstallWizardModel) Init() tea.Cmd { return nil }

// Update satisfies tea.Model. Ctrl+C is intercepted here, before any per-step dispatch, so it
// always hard-cancels regardless of step — see the type doc comment.
func (m InstallWizardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok && keyMsg.Type == tea.KeyCtrlC {
		m.Cancelled = true
		return m, tea.Quit
	}

	switch m.Step {
	case InstallWizardStepProfile:
		return m.updateProfileStep(msg)
	case InstallWizardStepModel:
		return m.updateModelStep(msg)
	default:
		return m, nil
	}
}

// updateProfileStep forwards msg to the composed ProfileSelectModel unchanged, then translates its
// own Confirmed/Cancelled outcome into wizard-level navigation: Confirmed advances to the Model
// step (re-seeded from the just-picked profile); Cancelled — reachable only here, on the first
// page — cancels the whole wizard.
func (m InstallWizardModel) updateProfileStep(msg tea.Msg) (tea.Model, tea.Cmd) {
	updated, _ := m.Profile.Update(msg)
	m.Profile = updated.(ProfileSelectModel)

	if m.Profile.Cancelled {
		m.Cancelled = true
		return m, tea.Quit
	}
	if m.Profile.Confirmed {
		// Reset immediately: this flag must not still read true the next time this step's Update
		// runs (e.g. after backing out of the Model step and moving the cursor without pressing
		// enter again), or a stray keypress would silently re-trigger this same transition.
		m.Profile.Confirmed = false
		m.Step = InstallWizardStepModel
		m.Model = seedInstallWizardModelStep(m.Profile.Selected)
	}
	return m, nil
}

// updateModelStep forwards msg to the composed ModelSelectModel unchanged, then translates its own
// Confirmed/Cancelled outcome: Cancelled (esc/q) means BACK to Profile, not wizard cancellation —
// the core UX fix this model exists for. Confirmed means the whole wizard is done.
func (m InstallWizardModel) updateModelStep(msg tea.Msg) (tea.Model, tea.Cmd) {
	updated, _ := m.Model.Update(msg)
	m.Model = updated.(ModelSelectModel)

	if m.Model.Cancelled {
		m.Model = ModelSelectModel{}
		m.Profile.Confirmed = false
		m.Step = InstallWizardStepProfile
		return m, nil
	}
	if m.Model.Confirmed {
		m.Confirmed = true
		m.Step = InstallWizardStepDone
		return m, tea.Quit
	}
	return m, nil
}

// seedInstallWizardModelStep mirrors runInstallSelectTUI's pre-wizard seeding rule verbatim
// (design D4): a real built-in preset seeds the Model step from that preset's own values; "custom"
// (or re-entering after a back-and-forth) seeds it from Defaults() instead. Called fresh every time
// the Profile step is confirmed, so re-confirming after backing up always re-seeds from scratch —
// see TestInstallWizardModel_ReenteringModelStepReseedsFromNewlyConfirmedProfile.
func seedInstallWizardModelStep(profile modelconfig.ProfileName) ModelSelectModel {
	if profile == modelconfig.ProfileCustom {
		return NewModelSelectModel()
	}
	return NewModelSelectModelForProfile(profile)
}

// View satisfies tea.Model, wrapping the active sub-model's own View() with a small step-indicator
// header and a footer hint clarifying what esc/q do on THIS step (back vs. cancel-everything).
func (m InstallWizardModel) View() string {
	switch m.Step {
	case InstallWizardStepProfile:
		return installWizardChrome(1, m.Profile.View(), "enter: continuar · esc/q: cancelar · ctrl+c: cancelar")
	case InstallWizardStepModel:
		return installWizardChrome(2, m.Model.View(), "enter: continuar · esc/q: atrás · ctrl+c: cancelar")
	default:
		return ""
	}
}

// installWizardChrome renders the shared wizard frame: a "Paso X de N" indicator (styled with the
// same cyan (color 6) this package already uses for the "Step" role — see renderer.go's Step
// method — rather than inventing a new color), the active sub-model's own body verbatim, and a
// wizard-level footer hint appended below the sub-model's own (unmodified) footer line.
func installWizardChrome(step int, body, hint string) string {
	var b strings.Builder
	indicator := fmt.Sprintf("Paso %d de %d", step, installWizardStepCount)
	b.WriteString(styleRenderer.NewStyle().Foreground(lipgloss.Color("6")).Bold(true).Render(indicator))
	b.WriteString("\n\n")
	b.WriteString(body)
	if hint != "" {
		b.WriteString("\n")
		b.WriteString(styleRenderer.NewStyle().Faint(true).Render(hint))
	}
	return b.String()
}
