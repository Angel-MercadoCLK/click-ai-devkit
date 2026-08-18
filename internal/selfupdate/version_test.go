package selfupdate

import "testing"

func TestCompareVersions_ComparableOrdering(t *testing.T) {
	tests := []struct {
		name      string
		left      string
		right     string
		wantOrder int // -1 for left < right, 0 for equal, 1 for left > right
		wantComp  bool
	}{
		// Equal versions
		{
			name:      "equal basic versions",
			left:      "1.2.3",
			right:     "1.2.3",
			wantOrder: 0,
			wantComp:  true,
		},
		{
			name:      "equal with leading v both sides",
			left:      "v1.2.3",
			right:     "v1.2.3",
			wantOrder: 0,
			wantComp:  true,
		},
		{
			name:      "equal 2-component versions",
			left:      "1.2",
			right:     "1.2",
			wantOrder: 0,
			wantComp:  true,
		},

		// 2 vs 3 component equivalence (missing patch defaults to 0)
		{
			name:      "v1.2 equals 1.2.0",
			left:      "v1.2",
			right:     "1.2.0",
			wantOrder: 0,
			wantComp:  true,
		},
		{
			name:      "1.2.0 equals v1.2",
			left:      "1.2.0",
			right:     "v1.2",
			wantOrder: 0,
			wantComp:  true,
		},

		// Major version differences
		{
			name:      "major less",
			left:      "1.2.3",
			right:     "2.0.0",
			wantOrder: -1,
			wantComp:  true,
		},
		{
			name:      "major greater",
			left:      "2.0.0",
			right:     "1.2.3",
			wantOrder: 1,
			wantComp:  true,
		},
		{
			name:      "major less with leading v",
			left:      "v1.2.3",
			right:     "2.0.0",
			wantOrder: -1,
			wantComp:  true,
		},

		// Minor version differences
		{
			name:      "minor less",
			left:      "1.2.3",
			right:     "1.3.0",
			wantOrder: -1,
			wantComp:  true,
		},
		{
			name:      "minor greater",
			left:      "1.3.0",
			right:     "1.2.3",
			wantOrder: 1,
			wantComp:  true,
		},
		{
			name:      "minor greater with 2 components",
			left:      "v1.10",
			right:     "1.2.0",
			wantOrder: 1,
			wantComp:  true,
		},

		// Patch version differences
		{
			name:      "patch less",
			left:      "1.2.3",
			right:     "1.2.4",
			wantOrder: -1,
			wantComp:  true,
		},
		{
			name:      "patch greater",
			left:      "1.2.4",
			right:     "1.2.3",
			wantOrder: 1,
			wantComp:  true,
		},

		// Numeric not lexicographic (1.2 < 1.10, not 1.2 > 1.10)
		{
			name:      "numeric 1.2 vs 1.10 less",
			left:      "1.2",
			right:     "1.10",
			wantOrder: -1,
			wantComp:  true,
		},
		{
			name:      "numeric 1.10 vs 1.2 greater",
			left:      "1.10",
			right:     "1.2",
			wantOrder: 1,
			wantComp:  true,
		},
		{
			name:      "numeric with v prefix",
			left:      "v1.2.3",
			right:     "v1.10.5",
			wantOrder: -1,
			wantComp:  true,
		},

		// Mixed v and non-v
		{
			name:      "left has v, right doesn't - less",
			left:      "v1.2.3",
			right:     "1.3.0",
			wantOrder: -1,
			wantComp:  true,
		},
		{
			name:      "left has v, right doesn't - greater",
			left:      "v2.0.0",
			right:     "1.2.3",
			wantOrder: 1,
			wantComp:  true,
		},
		{
			name:      "right has v, left doesn't",
			left:      "1.2.3",
			right:     "v1.2.4",
			wantOrder: -1,
			wantComp:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			order, comparable := compareVersions(tt.left, tt.right)
			if comparable != tt.wantComp {
				t.Errorf("compareVersions(%q, %q) comparable = %v, want %v", tt.left, tt.right, comparable, tt.wantComp)
			}
			if comparable && order != tt.wantOrder {
				t.Errorf("compareVersions(%q, %q) order = %d, want %d", tt.left, tt.right, order, tt.wantOrder)
			}
		})
	}
}
func TestCompareVersions_InvalidFormsAreNotComparable(t *testing.T) {
	invalidCases := []struct {
		name  string
		left  string
		right string
	}{
		{"empty string", "", "1.2.3"},
		{"dev string", "dev", "1.2.3"},
		{"just v", "v", "1.2.3"},
		{"single component", "1", "1.2.3"},
		{"four components", "1.2.3.4", "1.2.3"},
		{"double dot", "1..2", "1.2.3"},
		{"non-numeric x", "1.x.3", "1.2.3"},
		{"space in version", "v 1.2.3", "1.2.3"},
		{"beta suffix", "1.2.3-beta", "1.2.3"},
		{"negative major", "-1.2.3", "1.2.3"},
		{"signed plus major", "+1.2.3", "1.2.3"},
		{"signed plus minor", "1.+2.3", "1.2.3"},
		{"signed plus patch", "1.2.+3", "1.2.3"},
		{"signed minus major", "-1.2.3", "1.2.3"},
		{"signed minus minor", "1.-2.3", "1.2.3"},
		{"signed minus patch", "1.2.-3", "1.2.3"},
		{"trailing space", "1.2.3 ", "1.2.3"},
		{"leading space", " 1.2.3", "1.2.3"},
		{"non-ASCII", "1.β.3", "1.2.3"},
		{"multiple v prefixes", "vv1.2.3", "1.2.3"},
		{"v in middle", "1.v.3", "1.2.3"},
	}

	for _, tt := range invalidCases {
		t.Run(tt.name, func(t *testing.T) {
			order, comparable := compareVersions(tt.left, tt.right)
			if comparable {
				t.Errorf("compareVersions(%q, %q) unexpectedly comparable, want false", tt.left, tt.right)
			}
			if order != 0 {
				t.Errorf("compareVersions(%q, %q) returned non-zero order %d for non-comparable versions", tt.left, tt.right, order)
			}
		})
	}
}
