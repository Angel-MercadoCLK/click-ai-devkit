package doctor

import (
	"strings"
	"testing"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/installer"
)

func TestCheckDotStates(t *testing.T) {
	exe := `C:\Users\Ana\AppData\Local\Programs\Click Dot\Click Dot.exe`
	cases := []struct {
		name      string
		status    installer.DotStatus
		build     string
		healthy   bool
		warning   bool
		wantInfix string
	}{
		{"not installed", installer.DotStatus{Supported: true, ExePath: exe}, "1.2.3", true, true, "no instalado"},
		{"marker without exe", installer.DotStatus{Supported: true, ExePath: exe, MarkerPresent: true, Version: "1.2.3"}, "1.2.3", true, true, "falta " + exe},
		{"exe without marker", installer.DotStatus{Supported: true, ExePath: exe, ExePresent: true}, "1.2.3", true, true, "sin versión registrada"},
		{"matches", installer.DotStatus{Supported: true, ExePath: exe, ExePresent: true, MarkerPresent: true, Version: "1.2.3"}, "v1.2.3", true, false, "versión 1.2.3 instalada, coincide con click"},
		{"drift", installer.DotStatus{Supported: true, ExePath: exe, ExePresent: true, MarkerPresent: true, Version: "1.2.2"}, "1.2.3", true, true, "versión 1.2.2 instalada; click espera 1.2.3"},
		{"development build", installer.DotStatus{Supported: true, ExePath: exe, ExePresent: true, MarkerPresent: true, Version: "1.2.2"}, "dev", true, false, "build de desarrollo"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := CheckDot(tc.status, tc.build)
			if got.Name != "Click Dot" || got.Healthy != tc.healthy || got.Warning != tc.warning || !strings.Contains(got.Detail, tc.wantInfix) {
				t.Fatalf("CheckDot = %+v, want healthy=%v warning=%v detail containing %q", got, tc.healthy, tc.warning, tc.wantInfix)
			}
		})
	}
}

func TestReportHealthyIgnoresWarnings(t *testing.T) {
	report := Report{Checks: []CheckResult{{Name: "a", Healthy: true}, {Name: "Click Dot", Healthy: true, Warning: true}}}
	if !report.Healthy() {
		t.Fatal("a warning must not make the report unhealthy")
	}
}

func TestRunOmitsDotOffWindows(t *testing.T) {
	// The default manager reports GOOS of the test host; on non-Windows hosts Run must keep its
	// previous check list (no Dot entry).
	if installer.DotSupported() {
		t.Skip("Windows host")
	}
	for _, c := range Run(installer.Config{}).Checks {
		if c.Name == "Click Dot" {
			t.Fatal("Dot check must be absent off Windows")
		}
	}
}
