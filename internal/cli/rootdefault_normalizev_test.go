package cli

import (
	"testing"
)

// TestNormalizeV_TwoComponentVersionPadsToThreeComponents tests R11 requirement:
// 2-component versions must be padded to 3 components (e.g., "1.2" → "v1.2.0")
func TestNormalizeV_TwoComponentVersionPadsToThreeComponents(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		// 2-component versions should be padded
		{"1.2", "v1.2.0"},
		{"0.6", "v0.6.0"},
		{"2.10", "v2.10.0"},

		// 3-component versions should keep as-is
		{"1.2.3", "v1.2.3"},
		{"0.5.11", "v0.5.11"},

		// Already has v prefix - should be normalized and padded if needed
		{"v1.2", "v1.2.0"},
		{"v1.2.3", "v1.2.3"},
		{"vv1.2", "v1.2.0"}, // Multiple v's should be stripped

		// Edge cases - non-standard forms should fall back gracefully
		{"1", "v1"},             // Single component - no padding possible
		{"1.2.3.4", "v1.2.3.4"}, // 4+ components - leave as-is
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalizeV(tt.input)
			if got != tt.expected {
				t.Errorf("normalizeV(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}
