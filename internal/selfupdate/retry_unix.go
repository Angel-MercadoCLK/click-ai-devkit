//go:build !windows

package selfupdate

import (
	"errors"
	"syscall"
)

func isPlatformTransientFileError(err error) bool { return errors.Is(err, syscall.EBUSY) }
