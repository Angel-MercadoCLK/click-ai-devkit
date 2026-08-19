//go:build !windows

package selfupdate

import (
	"fmt"
	"io"
	"os"
	"syscall"
)

type targetLock struct{ file *os.File }

func (l *targetLock) Close() error {
	err := syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN)
	closeErr := l.file.Close()
	if err != nil {
		return err
	}
	return closeErr
}

func acquireTargetLock(target string) (io.Closer, error) {
	file, err := os.OpenFile(target+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open update lock: %w", err)
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("acquire update lock: %w", err)
	}
	return &targetLock{file: file}, nil
}
