package selfupdate

import (
	"strings"
	"testing"
)

// TestExpectedChecksum_ExactEntrySelection verifies that expectedChecksum
// correctly selects and validates a single SHA-256 entry for a given filename.
func TestExpectedChecksum_ExactEntrySelection(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		filename string
		wantHash string
		wantErr  bool
	}{
		{
			name:     "valid single entry",
			content:  "a1b2c3d4e5f6" + strings.Repeat("00", 26) + "  click_1.2.3_windows_amd64.zip",
			filename: "click_1.2.3_windows_amd64.zip",
			wantHash: "a1b2c3d4e5f6" + strings.Repeat("00", 26),
			wantErr:  false,
		},
		{
			name:     "valid entry with multiple spaces",
			content:  "a1b2c3d4e5f6" + strings.Repeat("00", 26) + "    click_1.2.3_windows_amd64.zip",
			filename: "click_1.2.3_windows_amd64.zip",
			wantHash: "a1b2c3d4e5f6" + strings.Repeat("00", 26),
			wantErr:  false,
		},
		{
			name:     "valid entry with tab separator",
			content:  "a1b2c3d4e5f6" + strings.Repeat("00", 26) + "\tclick_1.2.3_windows_amd64.zip",
			filename: "click_1.2.3_windows_amd64.zip",
			wantHash: "a1b2c3d4e5f6" + strings.Repeat("00", 26),
			wantErr:  false,
		},
		{
			name:     "valid entry matches case-sensitively",
			content:  "a1b2c3d4e5f6" + strings.Repeat("00", 26) + "  click_1.2.3_windows_amd64.zip",
			filename: "CLICK_1.2.3_WINDOWS_AMD64.ZIP",
			wantHash: "",
			wantErr:  true,
		},
		{
			name:     "missing filename",
			content:  "a1b2c3d4e5f6" + strings.Repeat("00", 26) + "  click_1.2.3_linux_amd64.zip",
			filename: "click_1.2.3_windows_amd64.zip",
			wantHash: "",
			wantErr:  true,
		},
		{
			name:     "malformed line - no space",
			content:  "a1b2c3d4e5f6" + strings.Repeat("00", 26) + "click_1.2.3_windows_amd64.zip",
			filename: "click_1.2.3_windows_amd64.zip",
			wantHash: "",
			wantErr:  true,
		},
		{
			name:     "malformed line - empty hash",
			content:  "  click_1.2.3_windows_amd64.zip",
			filename: "click_1.2.3_windows_amd64.zip",
			wantHash: "",
			wantErr:  true,
		},
		{
			name:     "malformed line - empty filename",
			content:  "a1b2c3d4e5f6" + strings.Repeat("00", 26) + "  ",
			filename: "click_1.2.3_windows_amd64.zip",
			wantHash: "",
			wantErr:  true,
		},
		{
			name: "duplicate filename - error",
			content: "a1b2c3d4e5f6" + strings.Repeat("00", 26) + "  click_1.2.3_windows_amd64.zip\n" +
				"b2c3d4e5f6a1" + strings.Repeat("00", 26) + "  click_1.2.3_windows_amd64.zip",
			filename: "click_1.2.3_windows_amd64.zip",
			wantHash: "",
			wantErr:  true,
		},
		{
			name:     "hex wrong length - too short",
			content:  "abc123  click_1.2.3_windows_amd64.zip",
			filename: "click_1.2.3_windows_amd64.zip",
			wantHash: "",
			wantErr:  true,
		},
		{
			name:     "hex wrong length - too long",
			content:  "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3  click_1.2.3_windows_amd64.zip",
			filename: "click_1.2.3_windows_amd64.zip",
			wantHash: "",
			wantErr:  true,
		},
		{
			name:     "hex invalid characters",
			content:  "g1b2c3d4e5f6" + strings.Repeat("00", 26) + "  click_1.2.3_windows_amd64.zip",
			filename: "click_1.2.3_windows_amd64.zip",
			wantHash: "",
			wantErr:  true,
		},
		{
			name:     "valid 64-character hex",
			content:  strings.Repeat("a", 64) + "  click_1.2.3_windows_amd64.zip",
			filename: "click_1.2.3_windows_amd64.zip",
			wantHash: strings.Repeat("a", 64),
			wantErr:  false,
		},
		{
			name: "multiple entries - selects correct one",
			content: "a1b2c3d4e5f6" + strings.Repeat("00", 26) + "  click_1.2.3_linux_amd64.zip\n" +
				"b2c3d4e5f6a1" + strings.Repeat("00", 26) + "  click_1.2.3_windows_amd64.zip\n" +
				"c3d4e5f6a1b2" + strings.Repeat("00", 26) + "  click_1.2.3_darwin_amd64.zip",
			filename: "click_1.2.3_windows_amd64.zip",
			wantHash: "b2c3d4e5f6a1" + strings.Repeat("00", 26),
			wantErr:  false,
		},
		{
			name:     "empty content",
			content:  "",
			filename: "click_1.2.3_windows_amd64.zip",
			wantHash: "",
			wantErr:  true,
		},
		{
			name:     "whitespace only content",
			content:  "   \n  \n",
			filename: "click_1.2.3_windows_amd64.zip",
			wantHash: "",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotHash, err := expectedChecksum(strings.NewReader(tt.content), tt.filename)
			if (err != nil) != tt.wantErr {
				t.Errorf("expectedChecksum() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && gotHash != tt.wantHash {
				t.Errorf("expectedChecksum() = %q, want %q", gotHash, tt.wantHash)
			}
		})
	}
}
