package selfupdate

import (
	"testing"
)

// TestParseManifest_ValidatesScoopMetadata is a table-driven test for manifest.json validation.
func TestParseManifest_ValidatesScoopMetadata(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		wantErr bool
	}{
		{
			name: "valid manifest",
			data: []byte(`{
				"version": "1.2.3",
				"url": "https://example.com/click.zip",
				"hash": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
			}`),
			wantErr: false,
		},
		{
			name:    "missing version",
			data:    []byte(`{"url": "https://example.com/click.zip", "hash": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}`),
			wantErr: true,
		},
		{
			name:    "empty version",
			data:    []byte(`{"version": "", "url": "https://example.com/click.zip", "hash": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}`),
			wantErr: true,
		},
		{
			name:    "non-comparable version with letter",
			data:    []byte(`{"version": "1.2.3a", "url": "https://example.com/click.zip", "hash": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}`),
			wantErr: true,
		},
		{
			name:    "non-comparable version with too many components",
			data:    []byte(`{"version": "1.2.3.4", "url": "https://example.com/click.zip", "hash": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}`),
			wantErr: true,
		},
		{
			name:    "non-comparable version with hyphen",
			data:    []byte(`{"version": "1.2.3-beta", "url": "https://example.com/click.zip", "hash": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}`),
			wantErr: true,
		},
		{
			name:    "missing url",
			data:    []byte(`{"version": "1.2.3", "hash": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}`),
			wantErr: true,
		},
		{
			name:    "empty url",
			data:    []byte(`{"version": "1.2.3", "url": "", "hash": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}`),
			wantErr: true,
		},
		{
			name:    "non-https url",
			data:    []byte(`{"version": "1.2.3", "url": "http://example.com/click.zip", "hash": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}`),
			wantErr: true,
		},
		{
			name:    "hash not exactly 64 characters",
			data:    []byte(`{"version": "1.2.3", "url": "https://example.com/click.zip", "hash": "0123456789abcdef"}`),
			wantErr: true,
		},
		{
			name:    "hash contains non-hex characters",
			data:    []byte(`{"version": "1.2.3", "url": "https://example.com/click.zip", "hash": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdefg"}`),
			wantErr: true,
		},
		{
			name:    "malformed JSON",
			data:    []byte(`{"version": "1.2.3", "url": "https://example.com/click.zip", "hash": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"`),
			wantErr: true,
		},
		{
			name:    "version field wrong type",
			data:    []byte(`{"version": 123, "url": "https://example.com/click.zip", "hash": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}`),
			wantErr: true,
		},
		{
			name:    "url field wrong type",
			data:    []byte(`{"version": "1.2.3", "url": 123, "hash": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}`),
			wantErr: true,
		},
		{
			name:    "hash field wrong type",
			data:    []byte(`{"version": "1.2.3", "url": "https://example.com/click.zip", "hash": 123}`),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := parseManifest(tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseManifest() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestParseInstall_ExtractsBucketVerbatim tests install.json parsing.
func TestParseInstall_ExtractsBucketVerbatim(t *testing.T) {
	tests := []struct {
		name         string
		data         []byte
		wantBucket   string
		wantBucketOK bool
	}{
		{
			name: "valid install.json with bucket",
			data: []byte(`{
				"bucket": "https://github.com/ScoopInstaller/Main",
				"architecture": "64bit"
			}`),
			wantBucket:   "https://github.com/ScoopInstaller/Main",
			wantBucketOK: true,
		},
		{
			name:         "empty bucket",
			data:         []byte(`{"bucket": "", "architecture": "64bit"}`),
			wantBucket:   "",
			wantBucketOK: false,
		},
		{
			name:         "whitespace-only bucket",
			data:         []byte(`{"bucket": "   ", "architecture": "64bit"}`),
			wantBucket:   "",
			wantBucketOK: false,
		},
		{
			name:         "bucket with control characters (tab)",
			data:         []byte(`{"bucket": "https://github.com\t/ScoopInstaller/Main", "architecture": "64bit"}`),
			wantBucket:   "",
			wantBucketOK: false,
		},
		{
			name:         "bucket with control characters (newline)",
			data:         []byte(`{"bucket": "https://github.com\n/ScoopInstaller/Main", "architecture": "64bit"}`),
			wantBucket:   "",
			wantBucketOK: false,
		},
		{
			name:         "bucket with control characters (carriage return)",
			data:         []byte(`{"bucket": "https://github.com\r/ScoopInstaller/Main", "architecture": "64bit"}`),
			wantBucket:   "",
			wantBucketOK: false,
		},
		{
			name:         "missing bucket",
			data:         []byte(`{"architecture": "64bit"}`),
			wantBucket:   "",
			wantBucketOK: false,
		},
		{
			name:         "missing architecture",
			data:         []byte(`{"bucket": "https://github.com/ScoopInstaller/Main"}`),
			wantBucket:   "",
			wantBucketOK: false,
		},
		{
			name:         "empty architecture",
			data:         []byte(`{"bucket": "https://github.com/ScoopInstaller/Main", "architecture": ""}`),
			wantBucket:   "",
			wantBucketOK: false,
		},
		{
			name:         "whitespace-only architecture",
			data:         []byte(`{"bucket": "https://github.com/ScoopInstaller/Main", "architecture": "   "}`),
			wantBucket:   "",
			wantBucketOK: false,
		},
		{
			name:         "malformed JSON",
			data:         []byte(`{"bucket": "https://github.com/ScoopInstaller/Main", "architecture": "64bit"`),
			wantBucket:   "",
			wantBucketOK: false,
		},
		{
			name:         "bucket field wrong type",
			data:         []byte(`{"bucket": 123, "architecture": "64bit"}`),
			wantBucket:   "",
			wantBucketOK: false,
		},
		{
			name:         "architecture field wrong type",
			data:         []byte(`{"bucket": "https://github.com/ScoopInstaller/Main", "architecture": 64}`),
			wantBucket:   "",
			wantBucketOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bucket, err := parseInstall(tt.data)
			if tt.wantBucketOK {
				if err != nil {
					t.Errorf("parseInstall() unexpected error = %v", err)
				}
				if bucket != tt.wantBucket {
					t.Errorf("parseInstall() bucket = %q, want %q", bucket, tt.wantBucket)
				}
			} else {
				if err == nil {
					t.Errorf("parseInstall() expected error, got nil")
				}
				if bucket != "" {
					t.Errorf("parseInstall() bucket = %q, want empty string on error", bucket)
				}
			}
		})
	}
}

// TestParseShimTarget_ExactlyOneAbsoluteAssignment tests shim file parsing.
func TestParseShimTarget_ExactlyOneAbsoluteAssignment(t *testing.T) {
	tests := []struct {
		name         string
		data         []byte
		wantTarget   string
		wantTargetOK bool
	}{
		{
			name:         "valid shim with absolute path",
			data:         []byte(`path = "C:/Users/test/scoop/apps/click/current/click.exe"`),
			wantTarget:   "C:/Users/test/scoop/apps/click/current/click.exe",
			wantTargetOK: true,
		},
		{
			name:         "zero assignments",
			data:         []byte(``),
			wantTarget:   "",
			wantTargetOK: false,
		},
		{
			name:         "multiple path assignments",
			data:         []byte("path = \"C:/Users/test/scoop/apps/click/current/click.exe\"\npath = \"C:/other/path/click.exe\""),
			wantTarget:   "",
			wantTargetOK: false,
		},
		{
			name:         "two conflicting assignments",
			data:         []byte("path = \"C:/first/click.exe\"\npath = \"C:/second/click.exe\""),
			wantTarget:   "",
			wantTargetOK: false,
		},
		{
			name:         "relative target",
			data:         []byte(`path = "../apps/click/current/click.exe"`),
			wantTarget:   "",
			wantTargetOK: false,
		},
		{
			name:         "malformed quoting (missing closing quote)",
			data:         []byte(`path = "C:/Users/test/scoop/apps/click/current/click.exe`),
			wantTarget:   "",
			wantTargetOK: false,
		},
		{
			name:         "malformed quoting (no quotes)",
			data:         []byte(`path = C:/Users/test/scoop/apps/click/current/click.exe`),
			wantTarget:   "",
			wantTargetOK: false,
		},
		{
			name:         "environment variable SCOOP (Windows style)",
			data:         []byte(`path = "%SCOOP%/apps/click/current/click.exe"`),
			wantTarget:   "",
			wantTargetOK: false,
		},
		{
			name:         "environment variable SCOOP (Unix style)",
			data:         []byte(`path = "$SCOOP/apps/click/current/click.exe"`),
			wantTarget:   "",
			wantTargetOK: false,
		},
		{
			name:         "wrong assignment name",
			data:         []byte(`target = "C:/Users/test/scoop/apps/click/current/click.exe"`),
			wantTarget:   "",
			wantTargetOK: false,
		},
		{
			name:         "path without equals sign",
			data:         []byte(`path "C:/Users/test/scoop/apps/click/current/click.exe"`),
			wantTarget:   "",
			wantTargetOK: false,
		},
		{
			name:         "empty path value",
			data:         []byte(`path = ""`),
			wantTarget:   "",
			wantTargetOK: false,
		},
		{
			name:         "whitespace-only path value",
			data:         []byte(`path = "   "`),
			wantTarget:   "",
			wantTargetOK: false,
		},
		{
			// A literal backslash-n inside the quoted value is not an injection vector: the value is
			// bounded by the first closing quote, so no second assignment can be smuggled in. The
			// result is just an implausible path, which the metadata probe then rejects safely.
			// Rejecting it here is what previously broke legitimate paths like C:\tools\… — see
			// TestParseShimTarget_AcceptsOrdinaryWindowsPaths.
			name:         "literal backslash-n inside the value is parsed, not treated as injection",
			data:         []byte(`path = "C:/Users/test/scoop/apps/click/current/click.exe\npath = C:/other"`),
			wantTarget:   `C:/Users/test/scoop/apps/click/current/click.exe\npath = C:/other`,
			wantTargetOK: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target, err := parseShimTarget(tt.data)
			if tt.wantTargetOK {
				if err != nil {
					t.Errorf("parseShimTarget() unexpected error = %v", err)
				}
				if target != tt.wantTarget {
					t.Errorf("parseShimTarget() target = %q, want %q", target, tt.wantTarget)
				}
			} else {
				if err == nil {
					t.Errorf("parseShimTarget() expected error, got nil")
				}
				if target != "" {
					t.Errorf("parseShimTarget() target = %q, want empty string on error", target)
				}
			}
		})
	}
}

// TestShimPathFor tests shim file path generation.
func TestShimPathFor(t *testing.T) {
	tests := []struct {
		executable string
		want       string
	}{
		{
			executable: "C:/scoop/shims/click.exe",
			want:       "C:/scoop/shims/click.shim",
		},
		{
			executable: "C:/scoop/shims/click",
			want:       "C:/scoop/shims/click.shim",
		},
		{
			executable: "/usr/local/bin/click",
			want:       "/usr/local/bin/click.shim",
		},
		{
			executable: "click.exe",
			want:       "click.shim",
		},
		{
			executable: "click",
			want:       "click.shim",
		},
	}

	for _, tt := range tests {
		t.Run(tt.executable, func(t *testing.T) {
			got := shimPathFor(tt.executable)
			if got != tt.want {
				t.Errorf("shimPathFor(%q) = %q, want %q", tt.executable, got, tt.want)
			}
		})
	}
}

// TestParseShimTarget_AcceptsOrdinaryWindowsPaths guards a real defect: an earlier version
// rejected any path containing the two-character sequences \n, \r or \t, reading them as escape
// sequences. In a shim file those are not escapes — they are a backslash followed by a letter,
// which is an ordinary part of a Windows path. C:\tools\… is a common Scoop location, and any
// user whose name begins with n, r or t was silently misclassified as a non-Scoop install.
func TestParseShimTarget_AcceptsOrdinaryWindowsPaths(t *testing.T) {
	paths := []string{
		`C:\tools\scoop\apps\click\current\click.exe`,
		`C:\Users\nacho\scoop\apps\click\current\click.exe`,
		`C:\repos\click\click.exe`,
		`C:\Users\rita\scoop\apps\click\current\click.exe`,
		`C:\Users\CLK090\scoop\apps\click\current\click.exe`,
	}

	for _, want := range paths {
		t.Run(want, func(t *testing.T) {
			got, err := parseShimTarget([]byte(`path = "` + want + `"` + "\n"))
			if err != nil {
				t.Fatalf("parseShimTarget(%q) error = %v, want it accepted", want, err)
			}
			if got != want {
				t.Errorf("parseShimTarget() = %q, want %q", got, want)
			}
		})
	}
}

// TestParseShimTarget_StillRejectsRealControlCharacters pins that dropping the escape-sequence
// check did not weaken the genuine guard.
func TestParseShimTarget_StillRejectsRealControlCharacters(t *testing.T) {
	for name, raw := range map[string]string{
		"newline":         `C:\tools\cl` + "\n" + `ick.exe`,
		"carriage return": `C:\tools\cl` + "\r" + `ick.exe`,
		"tab":             `C:\tools\cl` + "\t" + `ick.exe`,
		"null":            `C:\tools\cl` + "\x00" + `ick.exe`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parseShimTarget([]byte(`path = "` + raw + `"` + "\n")); err == nil {
				t.Errorf("parseShimTarget(%q) = nil error, want rejection", raw)
			}
		})
	}
}
