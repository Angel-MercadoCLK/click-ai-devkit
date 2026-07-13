package cli

import (
	"bytes"
	"testing"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/installer"
	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/modelconfig"
	"github.com/spf13/cobra"
)

// TestResolveAndSaveConfiguredModels_UnchangedPreset_KeepsPresetLabel guards the C1 fix:
// configure-models must not silently erase a persisted profile label (installer.SaveModels drops
// it). Re-selecting the exact same cost-saver map must keep the "cost-saver" label.
func TestResolveAndSaveConfiguredModels_UnchangedPreset_KeepsPresetLabel(t *testing.T) {
	home := t.TempDir()
	cfg := installer.Config{ClaudeHome: home}
	costSaver := modelconfig.ResolveProfile(string(modelconfig.ProfileCostSaver)).Models
	if err := installer.SaveModelsWithProfile(cfg, modelconfig.ProfileCostSaver, costSaver); err != nil {
		t.Fatalf("SaveModelsWithProfile() error = %v", err)
	}

	cmd := NewRootCommand()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	r := rendererFor(cmd, &buf)

	selector := func(*cobra.Command) (map[modelconfig.Phase]string, bool, error) {
		return costSaver, false, nil
	}

	if err := resolveAndSaveConfiguredModels(cmd, &buf, r, cfg, selector); err != nil {
		t.Fatalf("resolveAndSaveConfiguredModels() error = %v", err)
	}

	profile, _, found, err := installer.LoadModelsWithProfile(cfg)
	if err != nil {
		t.Fatalf("LoadModelsWithProfile() error = %v", err)
	}
	if !found {
		t.Fatal("LoadModelsWithProfile() found = false, want true")
	}
	if profile != modelconfig.ProfileCostSaver {
		t.Fatalf("persisted profile = %q, want %q (unchanged preset must keep its label)", profile, modelconfig.ProfileCostSaver)
	}
}

// TestResolveAndSaveConfiguredModels_TweakedPreset_DowngradesToCustom guards the other half of the
// C1 fix: a hand-tweaked map must downgrade the label to "custom", never silently drop to
// "balanced" the way the old SaveModels(cfg, selection) call did.
func TestResolveAndSaveConfiguredModels_TweakedPreset_DowngradesToCustom(t *testing.T) {
	home := t.TempDir()
	cfg := installer.Config{ClaudeHome: home}
	costSaver := modelconfig.ResolveProfile(string(modelconfig.ProfileCostSaver)).Models
	if err := installer.SaveModelsWithProfile(cfg, modelconfig.ProfileCostSaver, costSaver); err != nil {
		t.Fatalf("SaveModelsWithProfile() error = %v", err)
	}

	tweaked := make(map[modelconfig.Phase]string, len(costSaver))
	for phase, model := range costSaver {
		tweaked[phase] = model
	}
	tweaked[modelconfig.PhaseExplore] = "opus"

	cmd := NewRootCommand()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	r := rendererFor(cmd, &buf)

	selector := func(*cobra.Command) (map[modelconfig.Phase]string, bool, error) {
		return tweaked, false, nil
	}

	if err := resolveAndSaveConfiguredModels(cmd, &buf, r, cfg, selector); err != nil {
		t.Fatalf("resolveAndSaveConfiguredModels() error = %v", err)
	}

	profile, _, found, err := installer.LoadModelsWithProfile(cfg)
	if err != nil {
		t.Fatalf("LoadModelsWithProfile() error = %v", err)
	}
	if !found {
		t.Fatal("LoadModelsWithProfile() found = false, want true")
	}
	if profile != modelconfig.ProfileCustom {
		t.Fatalf("persisted profile = %q, want %q (tweaked map must downgrade the label, not drop to balanced)", profile, modelconfig.ProfileCustom)
	}
}

// TestResolveAndSaveConfiguredModels_Cancelled_LeavesModelsUntouched guards that cancelling the
// picker never persists anything, matching runConfigureModels' pre-fix behavior for this path.
func TestResolveAndSaveConfiguredModels_Cancelled_LeavesModelsUntouched(t *testing.T) {
	home := t.TempDir()
	cfg := installer.Config{ClaudeHome: home}

	cmd := NewRootCommand()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	r := rendererFor(cmd, &buf)

	selector := func(*cobra.Command) (map[modelconfig.Phase]string, bool, error) {
		return nil, true, nil
	}

	if err := resolveAndSaveConfiguredModels(cmd, &buf, r, cfg, selector); err != nil {
		t.Fatalf("resolveAndSaveConfiguredModels() error = %v", err)
	}

	if _, _, found, err := installer.LoadModelsWithProfile(cfg); err != nil {
		t.Fatalf("LoadModelsWithProfile() error = %v", err)
	} else if found {
		t.Fatal("LoadModelsWithProfile() found = true after cancel, want false (nothing persisted)")
	}
}
