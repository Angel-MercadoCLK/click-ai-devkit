package selfupdate

import (
	"testing"
)

// TestSupportedArtifact_Matrix verifies that exactly the GoReleaser-supported
// platforms are accepted, mirroring the ignore: block in .goreleaser.yaml.
func TestSupportedArtifact_Matrix(t *testing.T) {
	tests := []struct {
		goos   string
		goarch string
		want   bool
	}{
		// Supported tuples (must match .goreleaser.yaml builds)
		{"windows", "amd64", true},
		{"darwin", "amd64", true},
		{"darwin", "arm64", true},
		{"linux", "amd64", true},
		{"linux", "arm64", true},

		// Explicitly ignored in .goreleaser.yaml
		{"windows", "arm64", false},

		// Other unsupported combinations
		{"freebsd", "amd64", false},
		{"linux", "386", false},
		{"", "amd64", false},
		{"windows", "", false},
		{"", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.goos+"/"+tt.goarch, func(t *testing.T) {
			got := supportedArtifact(tt.goos, tt.goarch)
			if got != tt.want {
				t.Errorf("supportedArtifact(%q, %q) = %v, want %v", tt.goos, tt.goarch, got, tt.want)
			}
		})
	}
}

// TestReleaseAssetFor_ExactNamesAndURLs verifies that ReleaseAssetFor
// produces the exact archive name, executable name, and URLs for each
// supported platform, matching the .goreleaser.yaml name_template.
func TestReleaseAssetFor_ExactNamesAndURLs(t *testing.T) {
	version := "1.2.3"
	tag := "v1.2.3"

	tests := []struct {
		goos           string
		goarch         string
		wantArchive    string
		wantExecutable string
		wantArchiveURL string
		wantExeURL     string
	}{
		{
			goos:           "windows",
			goarch:         "amd64",
			wantArchive:    "click_1.2.3_windows_amd64.zip",
			wantExecutable: "click.exe",
			wantArchiveURL: "https://github.com/Angel-MercadoCLK/click-ai-devkit/releases/download/v1.2.3/click_1.2.3_windows_amd64.zip",
			wantExeURL:     "https://github.com/Angel-MercadoCLK/click-ai-devkit/releases/download/v1.2.3/click.exe",
		},
		{
			goos:           "darwin",
			goarch:         "amd64",
			wantArchive:    "click_1.2.3_darwin_amd64.zip",
			wantExecutable: "click",
			wantArchiveURL: "https://github.com/Angel-MercadoCLK/click-ai-devkit/releases/download/v1.2.3/click_1.2.3_darwin_amd64.zip",
			wantExeURL:     "https://github.com/Angel-MercadoCLK/click-ai-devkit/releases/download/v1.2.3/click",
		},
		{
			goos:           "darwin",
			goarch:         "arm64",
			wantArchive:    "click_1.2.3_darwin_arm64.zip",
			wantExecutable: "click",
			wantArchiveURL: "https://github.com/Angel-MercadoCLK/click-ai-devkit/releases/download/v1.2.3/click_1.2.3_darwin_arm64.zip",
			wantExeURL:     "https://github.com/Angel-MercadoCLK/click-ai-devkit/releases/download/v1.2.3/click",
		},
		{
			goos:           "linux",
			goarch:         "amd64",
			wantArchive:    "click_1.2.3_linux_amd64.zip",
			wantExecutable: "click",
			wantArchiveURL: "https://github.com/Angel-MercadoCLK/click-ai-devkit/releases/download/v1.2.3/click_1.2.3_linux_amd64.zip",
			wantExeURL:     "https://github.com/Angel-MercadoCLK/click-ai-devkit/releases/download/v1.2.3/click",
		},
		{
			goos:           "linux",
			goarch:         "arm64",
			wantArchive:    "click_1.2.3_linux_arm64.zip",
			wantExecutable: "click",
			wantArchiveURL: "https://github.com/Angel-MercadoCLK/click-ai-devkit/releases/download/v1.2.3/click_1.2.3_linux_arm64.zip",
			wantExeURL:     "https://github.com/Angel-MercadoCLK/click-ai-devkit/releases/download/v1.2.3/click",
		},
	}

	for _, tt := range tests {
		t.Run(tt.goos+"/"+tt.goarch, func(t *testing.T) {
			asset, err := releaseAssetFor(tt.goos, tt.goarch, version, tag)
			if err != nil {
				t.Fatalf("releaseAssetFor(%q, %q, %q, %q) unexpected error: %v", tt.goos, tt.goarch, version, tag, err)
			}

			if asset.ArchiveName != tt.wantArchive {
				t.Errorf("ArchiveName = %q, want %q", asset.ArchiveName, tt.wantArchive)
			}
			if asset.ExecutableName != tt.wantExecutable {
				t.Errorf("ExecutableName = %q, want %q", asset.ExecutableName, tt.wantExecutable)
			}
			if asset.ArchiveURL != tt.wantArchiveURL {
				t.Errorf("ArchiveURL = %q, want %q", asset.ArchiveURL, tt.wantArchiveURL)
			}
			if asset.ExecutableURL != tt.wantExeURL {
				t.Errorf("ExecutableURL = %q, want %q", asset.ExecutableURL, tt.wantExeURL)
			}
		})
	}

	// Test that unsupported tuples return an error
	t.Run("unsupported tuple returns error", func(t *testing.T) {
		_, err := releaseAssetFor("windows", "arm64", version, tag)
		if err == nil {
			t.Error("expected error for unsupported tuple, got nil")
		}
	})
}

// TestCanSelfUpdate_PlatformGate verifies that CanSelfUpdate only returns true
// for standalone installations on supported platforms.
func TestCanSelfUpdate_PlatformGate(t *testing.T) {
	// Save original seams
	origGoos := goos
	origGoarch := goarch
	origDetect := detectInstallation
	defer func() {
		goos = origGoos
		goarch = origGoarch
		detectInstallation = origDetect
	}()

	tests := []struct {
		name          string
		install       Installation
		runtimeGoos   string
		runtimeGoarch string
		want          bool
	}{
		{
			name: "standalone with supported platform",
			install: Installation{
				Method:     InstallStandalone,
				Executable: "/usr/local/bin/click",
			},
			runtimeGoos:   "linux",
			runtimeGoarch: "amd64",
			want:          true,
		},
		{
			name: "standalone with unsupported platform",
			install: Installation{
				Method:     InstallStandalone,
				Executable: "/usr/local/bin/click",
			},
			runtimeGoos:   "freebsd",
			runtimeGoarch: "amd64",
			want:          false,
		},
		{
			name: "scoop installation",
			install: Installation{
				Method:     InstallScoop,
				Executable: "C:\\scoop\\shims\\click.exe",
				Bucket:     "https://github.com/Angel-MercadoCLK/click-ai-devkit",
			},
			runtimeGoos:   "windows",
			runtimeGoarch: "amd64",
			want:          false,
		},
		{
			name: "unknown installation",
			install: Installation{
				Method:     InstallUnknown,
				Executable: "",
			},
			runtimeGoos:   "linux",
			runtimeGoarch: "amd64",
			want:          false,
		},
		{
			name: "standalone with windows arm64 (unsupported)",
			install: Installation{
				Method:     InstallStandalone,
				Executable: "C:\\click.exe",
			},
			runtimeGoos:   "windows",
			runtimeGoarch: "arm64",
			want:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Override seams
			goos = func() string { return tt.runtimeGoos }
			goarch = func() string { return tt.runtimeGoarch }
			detectInstallation = func() Installation { return tt.install }

			got := CanSelfUpdate()
			if got != tt.want {
				t.Errorf("CanSelfUpdate() = %v, want %v", got, tt.want)
			}
		})
	}
}
