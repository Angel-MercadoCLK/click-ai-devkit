//go:build windows

package selfupdate

import (
	"errors"
	"syscall"
)

// ERROR_SHARING_VIOLATION (32) and ERROR_LOCK_VIOLATION (33) are transient while
// another Windows process still has an executable handle open.
func isPlatformTransientFileError(err error) bool {
	return errors.Is(err, syscall.Errno(32)) || errors.Is(err, syscall.Errno(33))
}
