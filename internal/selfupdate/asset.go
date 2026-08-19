package selfupdate

import (
	"fmt"
	"runtime"
)

// releaseDownloadBaseURL is a seam for testing.
var releaseDownloadBaseURL = "https://github.com/Angel-MercadoCLK/click-ai-devkit/releases/download/"

// ReleaseAsset describes a release's archive and executable for a specific platform.
type ReleaseAsset struct {
	// ArchiveName is the exact archive filename (e.g., "click_1.2.3_windows_amd64.zip").
	ArchiveName string
	// ExecutableName is the exact executable filename inside the archive ("click" or "click.exe").
	ExecutableName string
	// ArchiveURL is the full HTTPS URL to download the archive.
	ArchiveURL string
	// ExecutableURL is the full HTTPS URL to the executable (same base as archive).
	ExecutableURL string
}

// releaseAsset constructs a ReleaseAsset for the given tag and platform.
// version is the version string WITHOUT the 'v' prefix, while tag includes it.
func releaseAsset(tag, goos, goarch string) (ReleaseAsset, error) {
	// Strip the 'v' prefix from the tag to get the version
	version := tag
	if len(tag) > 0 && tag[0] == 'v' {
		version = tag[1:]
	}

	return releaseAssetFor(goos, goarch, version, tag)
}

// releaseAssetFor creates a ReleaseAsset for the given platform.
// version is the version WITHOUT 'v', tag is WITH 'v'.
// Returns an error if the platform tuple is not supported.
func releaseAssetFor(goos, goarch, version, tag string) (ReleaseAsset, error) {
	if !supportedArtifact(goos, goarch) {
		return ReleaseAsset{}, fmt.Errorf("unsupported platform: %s/%s", goos, goarch)
	}

	archiveName := fmt.Sprintf("click_%s_%s_%s.zip", version, goos, goarch)
	executableName := "click"
	if goos == "windows" {
		executableName = "click.exe"
	}

	baseURL := releaseDownloadBaseURL + tag + "/"
	archiveURL := baseURL + archiveName
	executableURL := baseURL + executableName

	return ReleaseAsset{
		ArchiveName:    archiveName,
		ExecutableName: executableName,
		ArchiveURL:     archiveURL,
		ExecutableURL:  executableURL,
	}, nil
}

// supportedArtifact reports whether the given platform tuple is supported.
// This mirrors the ignore: block in .goreleaser.yaml: only the tuples
// actually built are accepted.
func supportedArtifact(goos, goarch string) bool {
	switch {
	case goos == "windows" && goarch == "amd64":
		return true
	case goos == "darwin" && (goarch == "amd64" || goarch == "arm64"):
		return true
	case goos == "linux" && (goarch == "amd64" || goarch == "arm64"):
		return true
	default:
		return false
	}
}

// CanSelfUpdate reports whether the current installation can self-update.
// Only standalone installations on supported platforms can self-update.
// Scoop and unknown installations cannot (they use their own update mechanism).
func CanSelfUpdate() bool {
	install := detectInstallation()
	if install.Method != InstallStandalone {
		return false
	}

	// Check if the current runtime platform is supported
	return supportedArtifact(goos(), goarch())
}

// goos and goarch are seams for testing, defaulting to runtime.GOOS and runtime.GOARCH.
var (
	goos   = func() string { return runtime.GOOS }
	goarch = func() string { return runtime.GOARCH }
	// detectInstallation is a seam for testing, defaulting to DetectInstallation.
	detectInstallation = DetectInstallation
)
