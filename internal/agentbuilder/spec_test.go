package agentbuilder

import "testing"

func TestSDDModesReturnsStandaloneAndPhaseSupportOnly(t *testing.T) {
	got := SDDModes()
	want := []SDDMode{SDDStandalone, SDDPhaseSupport}

	if len(got) != len(want) {
		t.Fatalf("len(SDDModes()) = %d, want %d (%#v)", len(got), len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("SDDModes()[%d] = %q, want %q", i, got[i], want[i])
		}
	}

	for _, mode := range got {
		if mode == "new-phase" || mode == "new phase" || mode == "new_phase" {
			t.Fatalf("SDDModes() included forbidden New Phase mode %q", mode)
		}
	}
}
