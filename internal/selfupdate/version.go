package selfupdate

import (
	"strconv"
	"strings"
)

// compareVersions compares two semantic version strings.
// Returns:
//   - order: -1 if left < right, 0 if equal, 1 if left > right
//   - comparable: true if both versions are in valid numeric form (major.minor or major.minor.patch)
//
// A single leading 'v' is stripped from both sides. Missing patch defaults to 0.
// Any non-numeric component, wrong number of components (neither 2 nor 3), or malformed
// input results in comparable=false.
func compareVersions(left, right string) (order int, comparable bool) {
	leftParts, leftOK := parseAndValidateVersion(left)
	if !leftOK {
		return 0, false
	}

	rightParts, rightOK := parseAndValidateVersion(right)
	if !rightOK {
		return 0, false
	}

	// Compare major
	if leftParts[0] < rightParts[0] {
		return -1, true
	}
	if leftParts[0] > rightParts[0] {
		return 1, true
	}

	// Compare minor
	if leftParts[1] < rightParts[1] {
		return -1, true
	}
	if leftParts[1] > rightParts[1] {
		return 1, true
	}

	// Compare patch
	if leftParts[2] < rightParts[2] {
		return -1, true
	}
	if leftParts[2] > rightParts[2] {
		return 1, true
	}

	return 0, true
}

// parseAndValidateVersion parses a version string into numeric parts.
// Returns [major, minor, patch] and true if valid, nil and false otherwise.
func parseAndValidateVersion(v string) ([3]int, bool) {
	// Strip one leading 'v'
	v = strings.TrimPrefix(v, "v")

	if v == "" {
		return [3]int{}, false
	}

	parts := strings.Split(v, ".")
	if len(parts) < 2 || len(parts) > 3 {
		return [3]int{}, false
	}

	result := [3]int{0, 0, 0}
	for i := 0; i < len(parts); i++ {
		num, err := strconv.Atoi(parts[i])
		if err != nil {
			// Non-numeric component
			return [3]int{}, false
		}
		if num < 0 {
			// Negative numbers are invalid
			return [3]int{}, false
		}
		result[i] = num
	}

	return result, true
}
