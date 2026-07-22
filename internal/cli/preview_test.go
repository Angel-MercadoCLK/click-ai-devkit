package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/installer"
)

// --- openclaw-target-support write-step generalization (tasks 2.14-2.18) ---

// TestInstallWriteSteps_OpenClawAbsent_MatchesPreChangeSixSteps guards the "zero behavior change"
// contract: a Config with no OpenClawHome must produce the exact same 6-step list installWriteSteps
// returned before OpenClaw support existed — no new prompts, no new lines.
func TestInstallWriteSteps_OpenClawAbsent_MatchesPreChangeSixSteps(t *testing.T) {
	got := installWriteSteps(installer.Config{ClaudeHome: t.TempDir()}, false)
	want := []string{
		"Registrando plugins click-sdd, click-memory, click-review y click-skills…",
		"Instalando Engram (memoria persistente)…",
		"Registrando Context7 (documentación de librerías)…",
		"Actualizando CLAUDE.md…",
		"Registrando memory-guard…",
		"Guardando modelos por fase de click-sdd…",
	}
	if len(got) != len(want) {
		t.Fatalf("installWriteSteps() = %#v, want exactly %d steps (pre-change behavior)", got, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("installWriteSteps()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// TestInstallWriteSteps_OpenClawPresent_AppendsFourOpenClawSteps is task 2.14's RED test extended
// by PR4: from 3 to 4 OpenClaw steps (the click-owned skill manifest sync step is appended after
// the memory-guard plugin step) — when OpenClaw is detected (cfg.OpenClawHome populated), the
// write plan must list all four targets.
func TestInstallWriteSteps_OpenClawPresent_AppendsFourOpenClawSteps(t *testing.T) {
	cfg := installer.Config{ClaudeHome: t.TempDir(), OpenClawHome: t.TempDir()}
	got := installWriteSteps(cfg, false)
	if len(got) != 10 {
		t.Fatalf("installWriteSteps() = %#v, want 10 steps (6 Claude + 4 OpenClaw)", got)
	}
	for _, step := range got[6:] {
		if !strings.Contains(step, "OpenClaw") {
			t.Fatalf("installWriteSteps() trailing steps = %#v, want every OpenClaw step to mention OpenClaw", got[6:])
		}
	}
}

// TestInstallWriteSteps_CloudConfigured_AddsCloudStep is task 4.1's preview-plan RED test: when
// server+project+token are all present, the plan must list the Engram Cloud enrollment step right
// after the local Engram step.
func TestInstallWriteSteps_CloudConfigured_AddsCloudStep(t *testing.T) {
	got := installWriteSteps(installer.Config{ClaudeHome: t.TempDir()}, true)
	want := []string{
		"Registrando plugins click-sdd, click-memory, click-review y click-skills…",
		"Instalando Engram (memoria persistente)…",
		"Enrolando Engram Cloud…",
		"Registrando Context7 (documentación de librerías)…",
		"Actualizando CLAUDE.md…",
		"Registrando memory-guard…",
		"Guardando modelos por fase de click-sdd…",
	}
	if len(got) != len(want) {
		t.Fatalf("installWriteSteps() = %#v, want exactly %d steps", got, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("installWriteSteps()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// TestInstallWriteSteps_CloudNotConfigured_NoCloudStep is task 4.1's no-config preview-plan test:
// when cloud config is incomplete, installWriteSteps must not add any cloud-related line.
func TestInstallWriteSteps_CloudNotConfigured_NoCloudStep(t *testing.T) {
	got := installWriteSteps(installer.Config{ClaudeHome: t.TempDir()}, false)
	for _, step := range got {
		if strings.Contains(step, "Cloud") {
			t.Fatalf("installWriteSteps() contains cloud step when not configured: %q", step)
		}
	}
}

// TestInstallWriteSteps_OpenClawAndCloudPresent_AppendsBoth is task 4.1's combined preview-plan
// test: when both OpenClaw and Engram Cloud are configured, the cloud step appears right after
// Engram and before Context7, and the OpenClaw steps remain last.
func TestInstallWriteSteps_OpenClawAndCloudPresent_AppendsBoth(t *testing.T) {
	cfg := installer.Config{ClaudeHome: t.TempDir(), OpenClawHome: t.TempDir()}
	got := installWriteSteps(cfg, true)
	if len(got) != 11 {
		t.Fatalf("installWriteSteps() = %#v, want 11 steps (6 Claude + 1 Cloud + 4 OpenClaw)", got)
	}
	if got[2] != "Enrolando Engram Cloud…" {
		t.Fatalf("installWriteSteps()[2] = %q, want Engram Cloud step right after local Engram", got[2])
	}
	for _, step := range got[7:] {
		if !strings.Contains(step, "OpenClaw") {
			t.Fatalf("installWriteSteps() trailing steps = %#v, want every OpenClaw step to mention OpenClaw", got[7:])
		}
	}
}

// TestUpdateWriteSteps_OpenClawPresent_AppendsFourOpenClawSteps mirrors the install-side test
// above for updateWriteSteps, extended by PR4 to 4 OpenClaw steps.
func TestUpdateWriteSteps_OpenClawPresent_AppendsFourOpenClawSteps(t *testing.T) {
	cfg := installer.Config{ClaudeHome: t.TempDir(), OpenClawHome: t.TempDir()}
	got := updateWriteSteps("0.1.1", cfg)
	if len(got) != 10 {
		t.Fatalf("updateWriteSteps() = %#v, want 10 steps (6 Claude + 4 OpenClaw)", got)
	}
	for _, step := range got[6:] {
		if !strings.Contains(step, "OpenClaw") {
			t.Fatalf("updateWriteSteps() trailing steps = %#v, want every OpenClaw step to mention OpenClaw", got[6:])
		}
	}
}

// TestUpdateWriteSteps_OpenClawAbsent_MatchesPreChangeSixSteps is updateWriteSteps' zero-behavior-
// change guard, mirroring TestInstallWriteSteps_OpenClawAbsent_MatchesPreChangeSixSteps.
func TestUpdateWriteSteps_OpenClawAbsent_MatchesPreChangeSixSteps(t *testing.T) {
	got := updateWriteSteps("0.1.1", installer.Config{ClaudeHome: t.TempDir()})
	if len(got) != 6 {
		t.Fatalf("updateWriteSteps() = %#v, want exactly 6 steps when OpenClaw is absent", got)
	}
}

// TestOpenClawWriteSteps_Absent_ReturnsNil guards openClawWriteSteps' own base case directly.
func TestOpenClawWriteSteps_Absent_ReturnsNil(t *testing.T) {
	if got := openClawWriteSteps(installer.Config{}); got != nil {
		t.Fatalf("openClawWriteSteps() = %#v, want nil when OpenClawHome is empty", got)
	}
}

// TestOpenClawWriteSteps_Present_IncludesPluginAndSkillSteps is PR-C/PR4's supporting RED test:
// the third OpenClaw step must mention the memory-guard plugin install, and the fourth must mention
// the click-owned skill manifests.
func TestOpenClawWriteSteps_Present_IncludesPluginAndSkillSteps(t *testing.T) {
	got := openClawWriteSteps(installer.Config{ClaudeHome: t.TempDir(), OpenClawHome: t.TempDir()})
	if len(got) != 4 {
		t.Fatalf("openClawWriteSteps() = %#v, want exactly 4 steps", got)
	}
	if !strings.Contains(got[2], "memory-guard") {
		t.Fatalf("openClawWriteSteps()[2] = %q, want it to mention the memory-guard plugin install", got[2])
	}
	if !strings.Contains(got[3], "skills") {
		t.Fatalf("openClawWriteSteps()[3] = %q, want it to mention the OpenClaw skill manifests", got[3])
	}
}

// TestRenderWritePlan_ListsStepsInOrder is the RED test for renderWritePlan: it asserts the exact
// (golden) plain-mode output — backup location line first, then a numbered list of steps in the
// exact order given — since renderWritePlan/preview.go do not exist yet at RED time.
func TestRenderWritePlan_ListsStepsInOrder(t *testing.T) {
	cfg := installer.Config{ClaudeHome: t.TempDir()}
	var buf bytes.Buffer
	r := rendererFor(NewRootCommand(), &buf)

	renderWritePlan(&buf, r, cfg, []string{"Registrando plugins…", "Actualizando CLAUDE.md…"})

	want := "[i] Se tomará un respaldo de CLAUDE.md y settings.json en " +
		filepath.Join(cfg.BackupDir(), "latest") +
		" antes de continuar.\n" +
		"[i] Se aplicarán los siguientes cambios, en este orden:\n" +
		"  1. Registrando plugins…\n" +
		"  2. Actualizando CLAUDE.md…\n"
	if buf.String() != want {
		t.Fatalf("renderWritePlan() output = %q, want %q", buf.String(), want)
	}
}

// TestRenderWritePlan_DifferentStepsProducesDifferentOutput triangulates the test above with a
// different step count/content, so a hardcoded ("Fake It") implementation of the first test cannot
// also pass this one — forcing the real numbered-list logic.
func TestRenderWritePlan_DifferentStepsProducesDifferentOutput(t *testing.T) {
	cfg := installer.Config{ClaudeHome: t.TempDir()}
	var buf bytes.Buffer
	r := rendererFor(NewRootCommand(), &buf)

	renderWritePlan(&buf, r, cfg, []string{"Solo un paso…"})

	want := "[i] Se tomará un respaldo de CLAUDE.md y settings.json en " +
		filepath.Join(cfg.BackupDir(), "latest") +
		" antes de continuar.\n" +
		"[i] Se aplicarán los siguientes cambios, en este orden:\n" +
		"  1. Solo un paso…\n"
	if buf.String() != want {
		t.Fatalf("renderWritePlan() output = %q, want %q", buf.String(), want)
	}
}

func TestConfirmProceed_LowercaseY_ReturnsTrue(t *testing.T) {
	var out bytes.Buffer
	r := rendererFor(NewRootCommand(), &out)
	proceed, err := confirmProceed(strings.NewReader("y\n"), &out, r)
	if err != nil {
		t.Fatalf("confirmProceed() error = %v", err)
	}
	if !proceed {
		t.Fatal("confirmProceed() = false, want true for input \"y\\n\"")
	}
}

// TestConfirmProceed_Yes_ReturnsTrue triangulates the "y"-only case with the full word "yes".
func TestConfirmProceed_Yes_ReturnsTrue(t *testing.T) {
	var out bytes.Buffer
	r := rendererFor(NewRootCommand(), &out)
	proceed, err := confirmProceed(strings.NewReader("yes\n"), &out, r)
	if err != nil {
		t.Fatalf("confirmProceed() error = %v", err)
	}
	if !proceed {
		t.Fatal("confirmProceed() = false, want true for input \"yes\\n\"")
	}
}

func TestConfirmProceed_UppercaseY_ReturnsTrue(t *testing.T) {
	var out bytes.Buffer
	r := rendererFor(NewRootCommand(), &out)
	proceed, err := confirmProceed(strings.NewReader("Y\n"), &out, r)
	if err != nil {
		t.Fatalf("confirmProceed() error = %v", err)
	}
	if !proceed {
		t.Fatal("confirmProceed() = false, want true for input \"Y\\n\" (case-insensitive)")
	}
}

func TestConfirmProceed_ExplicitNo_ReturnsFalse(t *testing.T) {
	var out bytes.Buffer
	r := rendererFor(NewRootCommand(), &out)
	proceed, err := confirmProceed(strings.NewReader("n\n"), &out, r)
	if err != nil {
		t.Fatalf("confirmProceed() error = %v", err)
	}
	if proceed {
		t.Fatal("confirmProceed() = true, want false for input \"n\\n\"")
	}
}

func TestConfirmProceed_EmptyInput_ReturnsFalseNoError(t *testing.T) {
	var out bytes.Buffer
	r := rendererFor(NewRootCommand(), &out)
	proceed, err := confirmProceed(strings.NewReader(""), &out, r)
	if err != nil {
		t.Fatalf("confirmProceed() error = %v, want nil on empty/EOF input (default-deny, not a failure)", err)
	}
	if proceed {
		t.Fatal("confirmProceed() = true, want false (default-deny) on empty input")
	}
}

// TestConfirmProceed_YesWithoutTrailingNewline_ReturnsTrue proves the io.EOF branch specifically:
// bufio.Reader.ReadString returns io.EOF alongside the partial line "y" when there is no trailing
// newline, and that must still parse as a valid confirmation, not be treated as an error.
func TestConfirmProceed_YesWithoutTrailingNewline_ReturnsTrue(t *testing.T) {
	var out bytes.Buffer
	r := rendererFor(NewRootCommand(), &out)
	proceed, err := confirmProceed(strings.NewReader("y"), &out, r)
	if err != nil {
		t.Fatalf("confirmProceed() error = %v, want nil (EOF is not an error here)", err)
	}
	if !proceed {
		t.Fatal("confirmProceed() = false, want true for EOF-terminated \"y\" input")
	}
}

func TestConfirmProceed_PrintsPrompt(t *testing.T) {
	var out bytes.Buffer
	r := rendererFor(NewRootCommand(), &out)
	if _, err := confirmProceed(strings.NewReader("n\n"), &out, r); err != nil {
		t.Fatalf("confirmProceed() error = %v", err)
	}
	if !strings.Contains(out.String(), "¿Continuar?") {
		t.Fatalf("confirmProceed() output = %q, want it to contain the confirmation prompt", out.String())
	}
}

// TestConfirmAndSnapshot_NonInteractive_SkipsPlanAndTakesSnapshotImmediately covers the
// --yes/--non-interactive/non-TTY bypass scenario: no plan, no prompt, but the run-start snapshot
// still happens.
func TestConfirmAndSnapshot_NonInteractive_SkipsPlanAndTakesSnapshotImmediately(t *testing.T) {
	home := t.TempDir()
	cfg := installer.Config{ClaudeHome: home}
	cmd := NewRootCommand()
	var out bytes.Buffer
	r := rendererFor(cmd, &out)

	proceed, err := confirmAndSnapshot(cmd, &out, r, cfg, true, []string{"Paso 1…"})
	if err != nil {
		t.Fatalf("confirmAndSnapshot() error = %v", err)
	}
	if !proceed {
		t.Fatal("confirmAndSnapshot() proceed = false, want true when nonInteractive is true")
	}
	if strings.Contains(out.String(), "¿Continuar?") {
		t.Fatalf("confirmAndSnapshot() output = %q, want no confirm prompt when nonInteractive is true", out.String())
	}
	has, err := installer.HasRunSnapshot(cfg)
	if err != nil {
		t.Fatalf("HasRunSnapshot() error = %v", err)
	}
	if !has {
		t.Fatal("HasRunSnapshot() = false, want true — nonInteractive must still take the snapshot")
	}
}

// TestConfirmAndSnapshot_InteractiveConfirm_ShowsPlanAndTakesSnapshot is the interactive-TTY-default
// scenario: the plan is shown, and confirming "y" takes the snapshot.
func TestConfirmAndSnapshot_InteractiveConfirm_ShowsPlanAndTakesSnapshot(t *testing.T) {
	home := t.TempDir()
	cfg := installer.Config{ClaudeHome: home}
	cmd := NewRootCommand()
	cmd.SetIn(strings.NewReader("y\n"))
	var out bytes.Buffer
	r := rendererFor(cmd, &out)

	proceed, err := confirmAndSnapshot(cmd, &out, r, cfg, false, []string{"Paso 1…"})
	if err != nil {
		t.Fatalf("confirmAndSnapshot() error = %v", err)
	}
	if !proceed {
		t.Fatal("confirmAndSnapshot() proceed = false, want true after confirming y")
	}
	if !strings.Contains(out.String(), "Paso 1…") {
		t.Fatalf("confirmAndSnapshot() output = %q, want the plan to list the step", out.String())
	}
	has, err := installer.HasRunSnapshot(cfg)
	if err != nil {
		t.Fatalf("HasRunSnapshot() error = %v", err)
	}
	if !has {
		t.Fatal("HasRunSnapshot() = false, want true after confirming")
	}
}

// TestConfirmAndSnapshot_InteractiveDecline_NoSnapshotTaken is the decline scenario: proceed=false,
// no error, and — critically — no snapshot (the first write in the whole chain) ever happens.
func TestConfirmAndSnapshot_InteractiveDecline_NoSnapshotTaken(t *testing.T) {
	home := t.TempDir()
	cfg := installer.Config{ClaudeHome: home}
	cmd := NewRootCommand()
	cmd.SetIn(strings.NewReader("n\n"))
	var out bytes.Buffer
	r := rendererFor(cmd, &out)

	proceed, err := confirmAndSnapshot(cmd, &out, r, cfg, false, []string{"Paso 1…"})
	if err != nil {
		t.Fatalf("confirmAndSnapshot() error = %v", err)
	}
	if proceed {
		t.Fatal("confirmAndSnapshot() proceed = true, want false after declining")
	}
	has, err := installer.HasRunSnapshot(cfg)
	if err != nil {
		t.Fatalf("HasRunSnapshot() error = %v", err)
	}
	if has {
		t.Fatal("HasRunSnapshot() = true, want false — decline must result in zero writes, including no snapshot")
	}
}
