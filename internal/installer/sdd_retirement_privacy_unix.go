//go:build !windows

package installer

import (
	"fmt"
	"os"
)

func setRetirementPathPrivacy(path string, directory bool) error {
	mode := os.FileMode(0o600)
	if directory {
		mode = 0o700
	}
	return os.Chmod(path, mode)
}

func retirementPathPrivacySatisfied(path string, directory bool) (bool, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return false, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return false, nil
	}
	if directory && !info.IsDir() {
		return false, fmt.Errorf("retirement privacy path is not a directory")
	}
	if !directory && !info.Mode().IsRegular() {
		return false, fmt.Errorf("retirement privacy path is not a regular file")
	}
	want := os.FileMode(0o600)
	if directory {
		want = 0o700
	}
	return info.Mode().Perm() == want, nil
}
