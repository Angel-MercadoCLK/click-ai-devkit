package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/installer"
)

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
