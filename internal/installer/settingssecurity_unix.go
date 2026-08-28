//go:build !windows

package installer

import (
	"os"
)

// Apply applies owner-only security (mode 0600) to the file at the given path.
func Apply(path string) error {
	return os.Chmod(path, 0600)
}

// OwnerOnly reports whether the file at the given path is owner-only (mode 0600).
func OwnerOnly(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return false, err
	}
	return info.Mode().Perm() == 0600, nil
}
