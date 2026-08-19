package selfupdate

import (
	"errors"
	"os"
	"time"
)

const fileOperationAttempts = 7

var fileRetrySleep = time.Sleep

func retryFileOperation(operation func() error) error {
	var err error
	for attempt := 0; attempt < fileOperationAttempts; attempt++ {
		err = operation()
		if err == nil || !isTransientFileError(err) {
			return err
		}
		if attempt+1 < fileOperationAttempts {
			fileRetrySleep(50 * time.Millisecond << attempt)
		}
	}
	return err
}

func isTransientFileError(err error) bool {
	return errors.Is(err, os.ErrPermission) || errors.Is(err, os.ErrExist) || isPlatformTransientFileError(err)
}
