package selfupdate

import (
	"bufio"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
)

// What this checksum does and does NOT establish (spec RAD-5).
//
// checksums.txt is published unsigned, from the same origin as the archive it describes. Verifying
// an archive against it therefore establishes INTEGRITY — it catches corruption, truncation, a
// partial download, and tampering below an uncompromised TLS connection. It does NOT establish
// AUTHENTICITY: an attacker who can publish a release can publish a malicious archive together
// with a checksum that matches it perfectly, and this check would pass.
//
// TLS is the real authenticity boundary here. Genuine publisher authentication would require
// signing the artifacts, which is deliberately out of scope for this change — an accepted
// limitation for an internal tool served from the organization's own repository, recorded here so
// no future reader mistakes a matching checksum for proof of origin.
//
// expectedChecksum reads a checksums.txt file and returns the SHA-256 hash
// for the given filename. Returns an error if:
// - The file is empty or malformed
// - The filename is not found
// - The filename appears more than once
// - The hash is not exactly 64 hexadecimal characters
// - Any line is malformed
func expectedChecksum(r io.Reader, filename string) (string, error) {
	if filename == "" {
		return "", errors.New("filename is empty")
	}

	scanner := bufio.NewScanner(r)
	var foundHash string
	found := false
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse line: "<hash>  <filename>" or "<hash>\t<filename>"
		// SHA-256 checksums.txt format is: 64 hex chars, two spaces or tab, filename
		parts := strings.SplitN(line, "  ", 2)
		if len(parts) != 2 {
			// Try tab separator
			parts = strings.SplitN(line, "\t", 2)
		}
		if len(parts) != 2 {
			return "", fmt.Errorf("malformed line %d: expected '<hash>  <filename>' format", lineNum)
		}
		if len(parts) != 2 {
			return "", fmt.Errorf("malformed line %d: expected '<hash>  <filename>' format", lineNum)
		}

		hash := strings.TrimSpace(parts[0])
		entryFilename := strings.TrimSpace(parts[1])

		// Validate hash is exactly 64 hex characters
		if len(hash) != 64 {
			return "", fmt.Errorf("malformed hash on line %d: expected 64 hex characters, got %d", lineNum, len(hash))
		}
		if _, err := hex.DecodeString(hash); err != nil {
			return "", fmt.Errorf("invalid hex hash on line %d: %w", lineNum, err)
		}

		// Check if this is our target filename
		if entryFilename == filename {
			if found {
				// Duplicate filename found
				return "", fmt.Errorf("duplicate filename entry on line %d: %s", lineNum, filename)
			}
			foundHash = hash
			found = true
		}
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("read error: %w", err)
	}

	if !found {
		return "", fmt.Errorf("filename not found: %s", filename)
	}

	return foundHash, nil
}
