package installer

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestManagedMarkdownProjectionHash tests the managed-content projection for markdown files.
func TestManagedMarkdownProjectionHash(t *testing.T) {
	// Test 1: present + canonical block hashes equal for CRLF vs LF forms of the same logical block
	t.Run("CRLF and LF forms hash the same for present block", func(t *testing.T) {
		crlfContent := "# >>> click-ai-devkit (managed) >>>\nline1\r\nline2\r\n# <<< click-ai-devkit (managed) <<<\n"
		lfContent := "# >>> click-ai-devkit (managed) >>>\nline1\nline2\n# <<< click-ai-devkit (managed) <<<\n"

		crlfHash := managedMarkdownProjectionHash(crlfContent)
		lfHash := managedMarkdownProjectionHash(lfContent)

		if crlfHash != lfHash {
			t.Errorf("CRLF and LF forms should hash the same, got CRLF=%s LF=%s", crlfHash, lfHash)
		}
	})

	// Test 2: absent when no exact marker lines
	t.Run("absent when no markers", func(t *testing.T) {
		content := "some random text\nwith no markers\nat all\n"
		hash := managedMarkdownProjectionHash(content)

		if !strings.HasPrefix(hash, "absent:") {
			t.Errorf("Expected hash to start with 'absent:', got %s", hash)
		}

		// Prove that absent hashes to something different from an empty-block present case
		emptyBlockContent := "# >>> click-ai-devkit (managed) >>>\n# <<< click-ai-devkit (managed) <<<\n"
		emptyBlockHash := managedMarkdownProjectionHash(emptyBlockContent)
		if hash == emptyBlockHash {
			t.Errorf("absent hash should differ from empty-block present hash")
		}
	})

	// Test 3: malformed + ordered marker-token sequence when marker lines exist without a well-formed pair
	t.Run("malformed when only begin marker exists", func(t *testing.T) {
		content := "# >>> click-ai-devkit (managed) >>>\nsome content\nno end marker\n"
		hash := managedMarkdownProjectionHash(content)

		if !strings.HasPrefix(hash, "malformed:") {
			t.Errorf("Expected hash to start with 'malformed:', got %s", hash)
		}

		if !strings.Contains(hash, "begin-marker") {
			t.Errorf("malformed hash should contain 'begin-marker', got %s", hash)
		}
	})

	// Test 4: malformed when only end marker exists
	t.Run("malformed when only end marker exists", func(t *testing.T) {
		content := "some content\n# <<< click-ai-devkit (managed) <<<\nmore content\n"
		hash := managedMarkdownProjectionHash(content)

		if !strings.HasPrefix(hash, "malformed:") {
			t.Errorf("Expected hash to start with 'malformed:', got %s", hash)
		}

		if !strings.Contains(hash, "end-marker") {
			t.Errorf("malformed hash should contain 'end-marker', got %s", hash)
		}
	})

	// Test 5: malformed when end marker comes before begin marker
	t.Run("malformed when end marker precedes begin marker", func(t *testing.T) {
		content := "# <<< click-ai-devkit (managed) <<<\nsome content\n# >>> click-ai-devkit (managed) >>>\n"
		hash := managedMarkdownProjectionHash(content)

		if !strings.HasPrefix(hash, "malformed:") {
			t.Errorf("Expected hash to start with 'malformed:', got %s", hash)
		}

		// Should have both markers in the hash
		if !strings.Contains(hash, "begin-marker") || !strings.Contains(hash, "end-marker") {
			t.Errorf("malformed hash should contain both markers, got %s", hash)
		}
	})

	// Test 6: prove malformed != absent
	t.Run("malformed hash differs from absent", func(t *testing.T) {
		malformedContent := "# >>> click-ai-devkit (managed) >>>\nno end\n"
		absentContent := "no markers at all\n"

		malformedHash := managedMarkdownProjectionHash(malformedContent)
		absentHash := managedMarkdownProjectionHash(absentContent)

		if malformedHash == absentHash {
			t.Errorf("malformed hash should differ from absent hash")
		}
	})

	// Test 7: present case with actual content
	t.Run("present case with content", func(t *testing.T) {
		content := "# >>> click-ai-devkit (managed) >>>\nmanaged line 1\nmanaged line 2\n# <<< click-ai-devkit (managed) <<<\n"
		hash := managedMarkdownProjectionHash(content)

		if !strings.HasPrefix(hash, "present:") {
			t.Errorf("Expected hash to start with 'present:', got %s", hash)
		}
	})

	// Test 8: present case with content outside markers (should not be in projection)
	t.Run("present case ignores content outside markers", func(t *testing.T) {
		content := "outside before\n# >>> click-ai-devkit (managed) >>>\nmanaged content\n# <<< click-ai-devkit (managed) <<<\noutside after\n"
		hash := managedMarkdownProjectionHash(content)

		if !strings.HasPrefix(hash, "present:") {
			t.Errorf("Expected hash to start with 'present:', got %s", hash)
		}

		// The hash should NOT include "outside before" or "outside after"
		if strings.Contains(hash, "outside before") || strings.Contains(hash, "outside after") {
			t.Errorf("present projection should not include content outside markers, got %s", hash)
		}
	})
}

// TestManagedSettingsProjectionHash tests the managed-content projection for settings.json.
func TestManagedSettingsProjectionHash(t *testing.T) {
	// Test 1: an owned hook among foreign PreToolUse entries hashes the same regardless of position
	t.Run("owned hook among foreign entries is position-independent", func(t *testing.T) {
		// First version: owned hook at position 0
		settings1 := map[string]any{
			"hooks": map[string]any{
				"PreToolUse": []any{
					map[string]any{
						"matcher": MemoryGuardToolMatcher,
						"hooks": []any{
							map[string]any{
								"type":    "command",
								"command": MemoryGuardCommand,
							},
						},
					},
					map[string]any{
						"matcher": "some_other_tool",
						"hooks":   []any{},
					},
				},
			},
		}

		// Second version: owned hook at position 1
		settings2 := map[string]any{
			"hooks": map[string]any{
				"PreToolUse": []any{
					map[string]any{
						"matcher": "some_other_tool",
						"hooks":   []any{},
					},
					map[string]any{
						"matcher": MemoryGuardToolMatcher,
						"hooks": []any{
							map[string]any{
								"type":    "command",
								"command": MemoryGuardCommand,
							},
						},
					},
				},
			},
		}

		data1, _ := json.Marshal(settings1)
		data2, _ := json.Marshal(settings2)

		hash1 := managedSettingsProjectionHash(string(data1))
		hash2 := managedSettingsProjectionHash(string(data2))

		if hash1 != hash2 {
			t.Errorf("Owned hook should hash the same regardless of position, got %s vs %s", hash1, hash2)
		}
	})

	// Test 2: re-marshaling the whole settings document with different key order does not change the hash
	t.Run("different JSON key order does not change hash", func(t *testing.T) {
		// Original order
		settings1 := map[string]any{
			"hooks": map[string]any{
				"PreToolUse": []any{
					map[string]any{
						"matcher": MemoryGuardToolMatcher,
						"hooks": []any{
							map[string]any{
								"type":    "command",
								"command": MemoryGuardCommand,
							},
						},
					},
				},
			},
		}

		// Re-marshaled with potentially different key order
		data1, _ := json.Marshal(settings1)
		var parsed map[string]any
		json.Unmarshal(data1, &parsed)
		data2, _ := json.MarshalIndent(parsed, "", "  ") // Simulates writeSettingsFile behavior

		hash1 := managedSettingsProjectionHash(string(data1))
		hash2 := managedSettingsProjectionHash(string(data2))

		if hash1 != hash2 {
			t.Errorf("Re-marshaled settings should hash the same, got %s vs %s", hash1, hash2)
		}
	})

	// Test 3: no owned hook → absent
	t.Run("absent when no owned hook", func(t *testing.T) {
		settings := map[string]any{
			"hooks": map[string]any{
				"PreToolUse": []any{
					map[string]any{
						"matcher": "some_other_tool",
						"hooks": []any{
							map[string]any{
								"type":    "command",
								"command": "some_command",
							},
						},
					},
				},
			},
		}

		data, _ := json.Marshal(settings)
		hash := managedSettingsProjectionHash(string(data))

		if !strings.HasPrefix(hash, "absent:") {
			t.Errorf("Expected hash to start with 'absent:', got %s", hash)
		}
	})

	// Test 4: absent when no PreToolUse hooks exist at all
	t.Run("absent when no PreToolUse hooks", func(t *testing.T) {
		settings := map[string]any{
			"otherKey": "otherValue",
		}

		data, _ := json.Marshal(settings)
		hash := managedSettingsProjectionHash(string(data))

		if !strings.HasPrefix(hash, "absent:") {
			t.Errorf("Expected hash to start with 'absent:', got %s", hash)
		}
	})

	// Test 5: absent when settings has no hooks key
	t.Run("absent when no hooks key", func(t *testing.T) {
		settings := map[string]any{}

		data, _ := json.Marshal(settings)
		hash := managedSettingsProjectionHash(string(data))

		if !strings.HasPrefix(hash, "absent:") {
			t.Errorf("Expected hash to start with 'absent:', got %s", hash)
		}
	})

	// Test 6: multiple owned hooks (duplicates) are all included and order-independent
	t.Run("multiple owned hooks included and order-independent", func(t *testing.T) {
		// First order
		settings1 := map[string]any{
			"hooks": map[string]any{
				"PreToolUse": []any{
					map[string]any{
						"matcher": MemoryGuardToolMatcher,
						"hooks": []any{
							map[string]any{
								"type":    "command",
								"command": MemoryGuardCommand,
							},
							map[string]any{
								"type":    "command",
								"command": MemoryGuardCommand, // Duplicate
							},
						},
					},
				},
			},
		}

		// Second order (hook objects swapped)
		settings2 := map[string]any{
			"hooks": map[string]any{
				"PreToolUse": []any{
					map[string]any{
						"matcher": MemoryGuardToolMatcher,
						"hooks": []any{
							map[string]any{
								"type":    "command",
								"command": MemoryGuardCommand,
							},
							map[string]any{
								"type":    "command",
								"command": MemoryGuardCommand,
							},
						},
					},
				},
			},
		}

		data1, _ := json.Marshal(settings1)
		data2, _ := json.Marshal(settings2)

		hash1 := managedSettingsProjectionHash(string(data1))
		hash2 := managedSettingsProjectionHash(string(data2))

		if hash1 != hash2 {
			t.Errorf("Multiple owned hooks should hash the same regardless of order, got %s vs %s", hash1, hash2)
		}
	})

	// Test 7: owned hook is only included when both matcher and command match
	t.Run("only matches when both matcher and command match", func(t *testing.T) {
		// Wrong matcher
		settings1 := map[string]any{
			"hooks": map[string]any{
				"PreToolUse": []any{
					map[string]any{
						"matcher": "wrong_matcher",
						"hooks": []any{
							map[string]any{
								"type":    "command",
								"command": MemoryGuardCommand,
							},
						},
					},
				},
			},
		}

		// Wrong command
		settings2 := map[string]any{
			"hooks": map[string]any{
				"PreToolUse": []any{
					map[string]any{
						"matcher": MemoryGuardToolMatcher,
						"hooks": []any{
							map[string]any{
								"type":    "command",
								"command": "wrong_command",
							},
						},
					},
				},
			},
		}

		data1, _ := json.Marshal(settings1)
		data2, _ := json.Marshal(settings2)

		hash1 := managedSettingsProjectionHash(string(data1))
		hash2 := managedSettingsProjectionHash(string(data2))

		if !strings.HasPrefix(hash1, "absent:") || !strings.HasPrefix(hash2, "absent:") {
			t.Errorf("Both should be absent when matcher or command doesn't match, got %s and %s", hash1, hash2)
		}
	})

	// Test 8: present case with correct matcher and command
	t.Run("present case with correct matcher and command", func(t *testing.T) {
		settings := map[string]any{
			"hooks": map[string]any{
				"PreToolUse": []any{
					map[string]any{
						"matcher": MemoryGuardToolMatcher,
						"hooks": []any{
							map[string]any{
								"type":    "command",
								"command": MemoryGuardCommand,
							},
						},
					},
				},
			},
		}

		data, _ := json.Marshal(settings)
		hash := managedSettingsProjectionHash(string(data))

		if !strings.HasPrefix(hash, "present:") {
			t.Errorf("Expected hash to start with 'present:', got %s", hash)
		}
	})

	// Test 9: ignores hooks with non-command type
	t.Run("ignores non-command hooks", func(t *testing.T) {
		settings := map[string]any{
			"hooks": map[string]any{
				"PreToolUse": []any{
					map[string]any{
						"matcher": MemoryGuardToolMatcher,
						"hooks": []any{
							map[string]any{
								"type": "some_other_type",
							},
							map[string]any{
								"type":    "command",
								"command": MemoryGuardCommand,
							},
						},
					},
				},
			},
		}

		data, _ := json.Marshal(settings)
		hash := managedSettingsProjectionHash(string(data))

		// Should be present with only the command hook
		if !strings.HasPrefix(hash, "present:") {
			t.Errorf("Expected hash to start with 'present:', got %s", hash)
		}
	})

	// Test 10: token presence is encoded, never the token value (task 4.20)
	t.Run("token presence encoded, never value", func(t *testing.T) {
		// Settings with a token value
		settingsWithToken := map[string]any{
			"env": map[string]any{
				"ENGRAM_CLOUD_TOKEN":  "secret-token-value-123",
				"ENGRAM_CLOUD_SERVER": "https://engram.example.com",
			},
			"hooks": map[string]any{
				"PreToolUse": []any{},
			},
		}

		// Same settings structure but with a different token value
		settingsWithDifferentToken := map[string]any{
			"env": map[string]any{
				"ENGRAM_CLOUD_TOKEN":  "different-secret-token-456",
				"ENGRAM_CLOUD_SERVER": "https://engram.example.com",
			},
			"hooks": map[string]any{
				"PreToolUse": []any{},
			},
		}

		// Settings without a token
		settingsWithoutToken := map[string]any{
			"env": map[string]any{
				"ENGRAM_CLOUD_SERVER": "https://engram.example.com",
			},
			"hooks": map[string]any{
				"PreToolUse": []any{},
			},
		}

		dataWithToken1, _ := json.Marshal(settingsWithToken)
		dataWithToken2, _ := json.Marshal(settingsWithDifferentToken)
		dataWithoutToken, _ := json.Marshal(settingsWithoutToken)

		hashWithToken1 := managedSettingsProjectionHash(string(dataWithToken1))
		hashWithToken2 := managedSettingsProjectionHash(string(dataWithToken2))
		hashWithoutToken := managedSettingsProjectionHash(string(dataWithoutToken))

		// Both settings with token (different values) should hash the same
		// because only presence is encoded, not the value
		if hashWithToken1 != hashWithToken2 {
			t.Errorf("Settings with different token values should hash the same (only presence encoded), got %s vs %s", hashWithToken1, hashWithToken2)
		}

		// Settings with token should hash differently from settings without token
		if hashWithToken1 == hashWithoutToken {
			t.Errorf("Settings with token should hash differently from settings without token, got same hash %s", hashWithToken1)
		}

		// Verify the token value never appears in the hash
		if strings.Contains(hashWithToken1, "secret-token-value-123") {
			t.Errorf("Token value should not appear in hash, got %s", hashWithToken1)
		}
		if strings.Contains(hashWithToken2, "different-secret-token-456") {
			t.Errorf("Token value should not appear in hash, got %s", hashWithToken2)
		}
	})
}
