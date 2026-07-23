package installer

// CodexPath resolves the optional Codex CLI through the shared injectable binary lookup seam.
func CodexPath() (path string, ok bool) {
	resolved, err := binaryLookupFactory().LookPath("codex")
	if err != nil {
		return "", false
	}
	return resolved, true
}

// CodexAvailable reports whether the optional Codex CLI is resolvable on PATH.
func CodexAvailable() bool {
	_, ok := CodexPath()
	return ok
}
