package agentbuilder

import "path/filepath"

const claudeCodeEngineID = "claude-code"

var ClaudeCode = Engine{
	ID:    claudeCodeEngineID,
	Label: "Claude Code",
	AgentsDir: func(claudeHome string) string {
		return filepath.Join(claudeHome, "agents")
	},
}

type Engine struct {
	ID        string
	Label     string
	AgentsDir func(claudeHome string) string
}

func Engines() []Engine {
	return []Engine{ClaudeCode}
}

func DefaultEngine() (Engine, bool) {
	engines := Engines()
	if len(engines) != 1 {
		return Engine{}, false
	}
	return engines[0], true
}
