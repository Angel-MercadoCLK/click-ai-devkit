package installer

// OpenClawPath resolves openclaw's absolute path via the same injectable BinaryLookup used by
// ClaudePath/GitPath/ResolveEngramBinaryPath (engram.go's binaryLookupFactory) — so tests can fake
// PATH resolution deterministically instead of depending on whether the real test machine has
// OpenClaw installed. ok=false when openclaw is not resolvable on PATH.
//
// Unlike ClaudePath/GitPath, there is no PreflightOpenClaw hard-fail counterpart: OpenClaw is an
// optional second install target, not a required dependency of click itself (openclaw-target-
// support spec's openclaw-detection capability, "OpenClaw absent" scenario) — absence is a valid,
// silent state that MUST skip all OpenClaw writes without raising an error, never block install.
func OpenClawPath() (path string, ok bool) {
	resolved, err := binaryLookupFactory().LookPath("openclaw")
	if err != nil {
		return "", false
	}
	return resolved, true
}

// OpenClawAvailable reports whether openclaw is resolvable on PATH.
func OpenClawAvailable() bool {
	_, ok := OpenClawPath()
	return ok
}
