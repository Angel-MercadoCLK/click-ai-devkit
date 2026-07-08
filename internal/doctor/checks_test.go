package doctor

import (
	"testing"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/installer"
)

func TestRun_BeforeInstall_ReportsUnhealthy(t *testing.T) {
	cfg := installer.Config{ClaudeHome: t.TempDir()}

	report := Run(cfg)

	if report.Healthy() {
		t.Fatal("Run() on a fresh, never-installed ClaudeHome reports healthy, want unhealthy")
	}
	if len(report.Checks) == 0 {
		t.Fatal("Run() returned zero checks")
	}
}

func TestRun_AfterInstall_ReportsHealthy(t *testing.T) {
	cfg := installer.Config{ClaudeHome: t.TempDir()}

	if err := installer.Install(cfg); err != nil {
		t.Fatalf("installer.Install() error = %v", err)
	}

	report := Run(cfg)

	if !report.Healthy() {
		t.Fatalf("Run() after Install() reports unhealthy, want healthy: %+v", report.Checks)
	}
	for _, c := range report.Checks {
		if !c.Healthy {
			t.Errorf("check %q reported unhealthy after Install(): %s", c.Name, c.Detail)
		}
	}
}

func TestRun_AfterUninstall_ReportsUnhealthy(t *testing.T) {
	cfg := installer.Config{ClaudeHome: t.TempDir()}

	if err := installer.Install(cfg); err != nil {
		t.Fatalf("installer.Install() error = %v", err)
	}
	if err := installer.Uninstall(cfg); err != nil {
		t.Fatalf("installer.Uninstall() error = %v", err)
	}

	report := Run(cfg)

	if report.Healthy() {
		t.Fatal("Run() after Uninstall() reports healthy, want unhealthy")
	}
}

func TestRun_ChecksHavePluginAndClaudeMD(t *testing.T) {
	cfg := installer.Config{ClaudeHome: t.TempDir()}
	report := Run(cfg)

	if len(report.Checks) != 5 {
		t.Fatalf("Run() returned %d checks, want 5 (click-sdd plugin, click-memory plugin, click-review plugin, CLAUDE.md, memory-guard hook)", len(report.Checks))
	}
}
