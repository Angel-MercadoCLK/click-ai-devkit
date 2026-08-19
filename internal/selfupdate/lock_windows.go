//go:build windows

package selfupdate

import (
	"fmt"
	"io"
	"syscall"
)

type targetLock struct{ handle syscall.Handle }

func (l *targetLock) Close() error { return syscall.CloseHandle(l.handle) }

func acquireTargetLock(target string) (io.Closer, error) {
	handle, err := syscall.CreateFile(syscall.StringToUTF16Ptr(target+".lock"), syscall.GENERIC_READ|syscall.GENERIC_WRITE, 0, nil, syscall.OPEN_ALWAYS, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, fmt.Errorf("acquire update lock: %w", err)
	}
	return &targetLock{handle: handle}, nil
}
