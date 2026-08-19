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

	// Validate path is absolute and contains no control characters or suspicious escape-like sequences
	if !filepath.IsAbs(foundPath) {
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
