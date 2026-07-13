package agentbuilder

import "testing"

func TestDefaultEngineReturnsClaudeCodeWhenOnlyEngineExists(t *testing.T) {
	engines := Engines()
	if len(engines) != 1 {
		t.Fatalf("len(Engines()) = %d, want 1", len(engines))
	}

	got, ok := DefaultEngine()
	if !ok {
		t.Fatal("DefaultEngine() ok = false, want true")
	}
	if got.ID != ClaudeCode.ID || got.Label != ClaudeCode.Label {
		t.Fatalf("DefaultEngine() = %#v, want ID/Label from %#v", got, ClaudeCode)
	}
	if got.AgentsDir("/tmp/claude") != ClaudeCode.AgentsDir("/tmp/claude") {
		t.Fatalf("DefaultEngine().AgentsDir() = %q, want %q", got.AgentsDir("/tmp/claude"), ClaudeCode.AgentsDir("/tmp/claude"))
	}
}
