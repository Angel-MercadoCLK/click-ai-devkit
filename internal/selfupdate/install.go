package selfupdate

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

// executablePath is a seam for testing. Defaults to os.Executable.
var executablePath = os.Executable

// evalExecutableSymlinks is a seam for testing. Defaults to filepath.EvalSymlinks.
var evalExecutableSymlinks = filepath.EvalSymlinks

// statExecutable is a seam for testing. Defaults to os.Stat.
var statExecutable = os.Stat

// readOwnershipFile is a seam for testing.
var readOwnershipFile = readBoundedFile

// readBoundedFile reads a file with a byte limit, returning an error if the file
// is larger than the limit or if reading fails. The cap exists so a malicious
// or malformed metadata file cannot force an unbounded read. Scoop metadata files
// are typically a few kilobytes; the 64 KiB cap provides safety without impacting
// legitimate use.
const maxMetadataBytes = 64 << 10 // 64 KiB

func readBoundedFile(path string, limit int64) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat file: %w", err)
	}
	if info.Size() > limit {
		return nil, errors.New("file exceeds size limit")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}
	return data, nil
}

// manifest represents the Scoop manifest.json structure we care about.
type manifest struct {
	Version string `json:"version"`
	URL     string `json:"url"`
	Hash    string `json:"hash"`
}

// install represents the Scoop install.json structure we care about.
type install struct {
	Bucket       string `json:"bucket"`
	Architecture string `json:"architecture"`
}

// parseManifest validates Scoop manifest.json content.
// Returns nil if valid, an error describing the validation failure otherwise.
func parseManifest(data []byte) error {
	var m manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return fmt.Errorf("malformed JSON: %w", err)
	}

	// Validate version: must be non-empty and comparable by our numeric comparator
	if m.Version == "" {
		return errors.New("version is empty")
	}
	if _, comparable := compareVersions(m.Version, m.Version); !comparable {
		return errors.New("version is not in comparable numeric form (major.minor or major.minor.patch)")
	}

	// Validate url: must be non-empty HTTPS
	if m.URL == "" {
		return errors.New("url is empty")
	}
	if len(m.URL) < 8 || m.URL[:8] != "https://" {
		return errors.New("url must use HTTPS")
	}

	// Validate hash: must be exactly 64 hexadecimal characters
	if len(m.Hash) != 64 {
		return fmt.Errorf("hash must be exactly 64 hexadecimal characters, got %d", len(m.Hash))
	}
	if _, err := hex.DecodeString(m.Hash); err != nil {
		return fmt.Errorf("hash must be hexadecimal: %w", err)
	}

	return nil
}

// parseInstall extracts the bucket from Scoop install.json content.
// Returns the bucket URL verbatim and nil if valid, empty string and error otherwise.
func parseInstall(data []byte) (string, error) {
	var inst install
	if err := json.Unmarshal(data, &inst); err != nil {
		return "", fmt.Errorf("malformed JSON: %w", err)
	}

	// Validate bucket: must be non-empty, not whitespace-only, and contain no control characters
	if inst.Bucket == "" || strings.TrimSpace(inst.Bucket) == "" {
		return "", errors.New("bucket is empty")
	}
	for _, r := range inst.Bucket {
		if unicode.IsControl(r) {
			return "", errors.New("bucket contains control characters")
		}
	}

	// Validate architecture: must be non-empty after trimming whitespace
	arch := inst.Architecture
	if arch == "" || strings.TrimSpace(arch) == "" {
		return "", errors.New("architecture is empty")
	}

	// Retain bucket verbatim as received from JSON (per task 2 requirements)
	return inst.Bucket, nil
}

// parseShimTarget parses a Scoop .shim file to extract the target path.
// Returns the absolute target path and nil if exactly one valid path assignment exists,
// empty string and error otherwise.
func parseShimTarget(data []byte) (string, error) {
	// Split by lines
	lines := strings.Split(string(data), "\n")

	var foundPath string
	found := false

	for _, line := range lines {
		// Trim whitespace from each line
		trimmed := strings.TrimSpace(line)

		// Skip empty lines and comments
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		// Parse "path = "...""
		if !strings.HasPrefix(trimmed, "path = ") {
			continue // Skip lines that aren't path assignments
		}

		// Already found one path - this is a second assignment
		if found {
			return "", errors.New("multiple path assignments in shim")
		}

		// Extract quoted value
		if len(trimmed) < 10 || trimmed[7] != '"' {
			return "", errors.New("path value must be quoted")
		}

		// Find closing quote (first one after the opening quote)
		rest := trimmed[8:]
		closingQuote := strings.Index(rest, `"`)
		if closingQuote == -1 {
			return "", errors.New("unclosed quote in path value")
		}

		foundPath = rest[:closingQuote]
		found = true
	}

	if !found {
		return "", errors.New("no path assignment found in shim")
	}

	// Validate the path is absolute — see isAbsoluteShimTarget for why filepath.IsAbs is wrong here.
	if !isAbsoluteShimTarget(foundPath) {
		return "", fmt.Errorf("path is not absolute: %s", foundPath)
	}

	// Check for control characters in the path (including newlines)
	for _, r := range foundPath {
		if unicode.IsControl(r) {
			return "", errors.New("path contains control characters")
		}
	}

	// Deliberately no check for literal backslash-n/r/t sequences here. The shim file is data, not
	// Go source, so those are two ordinary characters — and on Windows they are ordinary parts of a
	// path: C:\tools\…, C:\repos\…, or any user whose name starts with n, r or t would be rejected.
	// Real control characters are already rejected by the loop above, which is the correct check.

	return foundPath, nil
}

// isAbsoluteShimTarget reports whether p is an absolute path under either Windows or POSIX rules,
// independently of the host OS.
//
// Deliberately not filepath.IsAbs, which answers according to the host: a real shim always contains
// a Windows path, so on a non-Windows host filepath.IsAbs would reject every legitimate target,
// while on Windows it rejects the POSIX-style temp paths the tests point shims at. Judging the form
// by both conventions removes that coupling. This only checks the SHAPE — whether the target
// actually exists, is a regular file, and is owned by Scoop is verified afterwards by stat and the
// metadata probe, which is where the real authority belongs.
func isAbsoluteShimTarget(p string) bool {
	if strings.HasPrefix(p, "/") || strings.HasPrefix(p, `\\`) {
		return true
	}
	if len(p) < 3 {
		return false
	}
	drive := p[0]
	isLetter := (drive >= 'a' && drive <= 'z') || (drive >= 'A' && drive <= 'Z')
	return isLetter && p[1] == ':' && (p[2] == '\\' || p[2] == '/')
}

// shimPathFor returns the expected path to a .shim file for the given executable.
// The shim is always a sibling of the executable with .shim extension.
func shimPathFor(executable string) string {
	// Remove any extension and append .shim
	base := executable
	if ext := filepath.Ext(base); ext != "" {
		base = strings.TrimSuffix(base, ext)
	}
	return base + ".shim"
}

// ownershipState represents the classification of Scoop ownership.
type ownershipState int

const (
	ownershipNone    ownershipState = iota // No Scoop metadata present
	ownershipPartial                       // Some metadata present but incomplete
	ownershipInvalid                       // Metadata present but malformed
	ownershipValid                         // Complete, valid Scoop metadata
)

// probeScoopDirectory examines a directory for Scoop metadata files.
// Returns the ownership state and, if valid, the bucket URL from install.json.
func probeScoopDirectory(dir string) (ownershipState, string) {
	manifestPath := filepath.Join(dir, "manifest.json")
	installPath := filepath.Join(dir, "install.json")

	// Check which files exist
	manifestExists := false
	installExists := false

	if _, err := os.Stat(manifestPath); err == nil {
		manifestExists = true
	}
	if _, err := os.Stat(installPath); err == nil {
		installExists = true
	}

	// Neither present: not a Scoop install
	if !manifestExists && !installExists {
		return ownershipNone, ""
	}

	// Both present: validate both
	if manifestExists && installExists {
		manifestData, err := readOwnershipFile(manifestPath, maxMetadataBytes)
		if err != nil {
			return ownershipInvalid, ""
		}
		if err := parseManifest(manifestData); err != nil {
			return ownershipInvalid, ""
		}

		installData, err := readOwnershipFile(installPath, maxMetadataBytes)
		if err != nil {
			return ownershipInvalid, ""
		}
		bucket, err := parseInstall(installData)
		if err != nil {
			return ownershipInvalid, ""
		}

		return ownershipValid, bucket
	}

	// Only one present: partial
	return ownershipPartial, ""
}

// InstallMethod represents the detected installation method.
type InstallMethod int

const (
	// InstallUnknown means the installation method could not be determined.
	InstallUnknown InstallMethod = iota
	// InstallScoop means the CLI was installed via Scoop.
	InstallScoop
	// InstallStandalone means the CLI was installed as a standalone binary.
	InstallStandalone
)

// Installation represents information about how the CLI was installed.
type Installation struct {
	// Method is the detected installation method.
	Method InstallMethod
	// Executable is the absolute path to the running executable.
	Executable string
	// Bucket is the Scoop bucket URL (only populated for InstallScoop).
	Bucket string
}

// DetectInstallation determines how the CLI was installed by examining
// the running executable and its metadata. Returns an Installation struct
// with the detected method and associated information.
// Deliberately independent of the running version. How click was installed is a filesystem
// question; the version answers a different one. Dev builds are already excluded upstream — Check
// skips any non-comparable version, so the notice path (the only caller) never runs for them and
// detection is never consulted. An earlier revision gated this on reading the update cache and a
// hardcoded placeholder version, which made every machine without a cache file report Unknown even
// when Scoop plainly owned the binary.
func DetectInstallation() Installation {
	// Resolve the running executable path
	exePath := resolveRunningExecutable()
	if exePath == "" {
		return Installation{Method: InstallUnknown}
	}

	// Check for Scoop metadata in the executable's directory
	exeDir := filepath.Dir(exePath)
	state, bucket := probeScoopDirectory(exeDir)

	if state == ownershipValid {
		return Installation{
			Method:     InstallScoop,
			Executable: exePath,
			Bucket:     bucket,
		}
	}

	// If we have partial metadata (not valid, not none), it's ambiguous - return unknown
	if state == ownershipPartial || state == ownershipInvalid {
		return Installation{Method: InstallUnknown}
	}

	// Check for Scoop shim indirection
	shimPath := shimPathFor(exePath)
	shimData, err := readOwnershipFile(shimPath, maxMetadataBytes)
	if err == nil {
		shimTarget, err := parseShimTarget(shimData)
		if err == nil {
			// Validate the shim target is a regular file
			targetInfo, statErr := os.Stat(shimTarget)
			if statErr == nil && targetInfo.Mode().IsRegular() {
				// Probe the shim target's directory for Scoop metadata
				targetDir := filepath.Dir(shimTarget)
				targetState, targetBucket := probeScoopDirectory(targetDir)
				if targetState == ownershipValid {
					return Installation{
						Method:     InstallScoop,
						Executable: shimTarget,
						Bucket:     targetBucket,
					}
				}
				// If target has partial or invalid metadata, it's ambiguous
				if targetState == ownershipPartial || targetState == ownershipInvalid {
					return Installation{Method: InstallUnknown}
				}
			}
			// If shim target is not a regular file, it's ambiguous
			if statErr != nil || !targetInfo.Mode().IsRegular() {
				return Installation{Method: InstallUnknown}
			}
		}
	}

	// No Scoop metadata found: standalone
	return Installation{
		Method:     InstallStandalone,
		Executable: exePath,
		Bucket:     "",
	}
}

// resolveRunningExecutable returns the absolute path to the currently running
// executable, or an empty string if resolution fails. Resolution involves:
// 1. Getting the executable path via os.Executable
// 2. Resolving symlinks/junctions via filepath.EvalSymlinks
// 3. Verifying the resolved path is a regular file
// Any failure at any step returns empty string (unknown), not an error.
func resolveRunningExecutable() string {
	// Step 1: Get the executable path
	path, err := executablePath()
	if err != nil {
		return ""
	}

	// Step 2: Resolve symlinks and junctions
	resolved, err := evalExecutableSymlinks(path)
	if err != nil {
		return ""
	}

	// Step 3: Verify it's a regular file (not a directory)
	info, err := statExecutable(resolved)
	if err != nil {
		return ""
	}
	if !info.Mode().IsRegular() {
		return ""
	}

	// Step 4: Verify the resolved path is absolute
	if !filepath.IsAbs(resolved) {
		return ""
	}

	return resolved
}
