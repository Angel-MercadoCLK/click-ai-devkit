package installer

import (
	"encoding/json"
	"sort"
	"strings"
)

// managedMarkdownProjectionHash returns the hash of the managed-content projection for markdown files.
// The projection is a tagged digest: "absent:<hash>" when no exact marker lines exist,
// "malformed:<marker-sequence>:<hash>" when markers are malformed, or "present:<hash>" when a
// well-formed managed block exists — the block's raw bytes are hashed, never returned, so the
// manifest never carries click-owned content in plaintext. The tag ensures these three cases hash
// differently.
func managedMarkdownProjectionHash(content string) string {
	lines := crlfAwareSplitLines(content)
	begin, end := findMarkers(lines)

	// Check for ANY marker occurrence (well-formed or not)
	hasBeginMarker := false
	hasEndMarker := false
	for _, line := range lines {
		if line == managedBeginMarker {
			hasBeginMarker = true
		} else if line == managedEndMarker {
			hasEndMarker = true
		}
	}

	// Case 1: no markers at all
	if !hasBeginMarker && !hasEndMarker {
		return "absent:" + CanonicalContentHash("absent")
	}

	// Case 2: malformed markers exist (well-formed check failed). The marker-token sequence is a
	// small, fixed-vocabulary descriptor ("begin-marker"/"end-marker"), never raw file content, so
	// including it directly in the tag is safe and keeps malformed distinguishable by shape.
	if begin == -1 {
		var markerTokens []string
		if hasBeginMarker {
			markerTokens = append(markerTokens, "begin-marker")
		}
		if hasEndMarker {
			markerTokens = append(markerTokens, "end-marker")
		}
		tag := "malformed:" + strings.Join(markerTokens, ":")
		return tag + ":" + CanonicalContentHash(tag)
	}

	// Case 3: well-formed managed block present. The raw block text is hashed, never returned — the
	// manifest must record a compact digest of click-owned content, not the content itself.
	block := joinWithLineEnding(lines[begin:end+1], "\n")
	return "present:" + CanonicalContentHash("present:"+block)
}

// managedSettingsProjectionHash returns the hash of the managed-content projection for settings.json.
// The projection is a tagged digest: "absent:<hash>" when no owned hook exists, or "present:<hash>"
// when owned hooks are found — the compact-serialized hooks are hashed, never returned, so the
// manifest never carries click-owned content in plaintext.
// Owned hooks are identified by matching both MemoryGuardToolMatcher and MemoryGuardCommand.
// The projection is order-independent to tolerate writeSettingsFile's key-order normalization.
//
// The projection also encodes token presence (boolean) in env.ENGRAM_CLOUD_TOKEN, never the token
// value itself (NFR-6). This ensures the manifest records whether a token was present without
// leaking its value.
func managedSettingsProjectionHash(content string) string {
	var settings map[string]any
	if err := json.Unmarshal([]byte(content), &settings); err != nil {
		// Invalid JSON - treat as absent
		projection := "absent"
		return "absent:" + CanonicalContentHash(projection)
	}

	// Check for token presence (encode boolean, never the value)
	tokenPresent := false
	if env, ok := settings["env"].(map[string]any); ok {
		if _, hasToken := env["ENGRAM_CLOUD_TOKEN"]; hasToken {
			tokenPresent = true
		}
	}

	entries := getPreToolUseEntries(settings)
	var ownedHooks []string

	for _, raw := range entries {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		matcher, _ := entry["matcher"].(string)
		if matcher != MemoryGuardToolMatcher {
			continue
		}

		hooks, _ := entry["hooks"].([]any)
		for _, hookRaw := range hooks {
			hook, ok := hookRaw.(map[string]any)
			if !ok {
				continue
			}
			if hook["type"] == "command" && hook["command"] == MemoryGuardCommand {
				// Found an owned hook - create a compact-marshaled representation
				ownedEntry := map[string]any{
					"matcher": matcher,
					"hook":    hook,
				}
				marshaled, err := json.Marshal(ownedEntry)
				if err != nil {
					continue
				}
				ownedHooks = append(ownedHooks, string(marshaled))
			}
		}
	}

	// Case: no owned hook and no token
	if len(ownedHooks) == 0 && !tokenPresent {
		return "absent:" + CanonicalContentHash("absent")
	}

	// Case: owned hooks exist and/or token present. Sorting first removes dependence on position
	// among foreign entries AND on writeSettingsFile's key-order re-normalization; only the
	// resulting hash is returned — never the owned hooks' own JSON, which is click-owned content,
	// not a descriptor.
	sort.Strings(ownedHooks)

	// Build projection: token presence (as boolean string) + owned hooks
	var projectionParts []string
	if tokenPresent {
		projectionParts = append(projectionParts, "token:true")
	} else {
		projectionParts = append(projectionParts, "token:false")
	}
	projectionParts = append(projectionParts, ownedHooks...)

	projection := "present:" + strings.Join(projectionParts, ":")
	return "present:" + CanonicalContentHash(projection)
}
