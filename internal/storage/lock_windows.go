//go:build windows

package storage

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

type fileLock struct {
	file *os.File
}

func lockFile(path string) (lockHandle, error) {
	if path == "" {
		return nil, fmt.Errorf("lock path is empty")
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	var ol windows.Overlapped
	if err := windows.LockFileEx(windows.Handle(file.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK, 0, 1, 0, &ol); err != nil {
		closeErr := file.Close()
		if closeErr != nil {
			return nil, fmt.Errorf("lock %s failed: %w (also failed to close: %v)", path, err, closeErr)
		}
		return nil, err
	}
	return &fileLock{file: file}, nil
}

func (l *fileLock) Unlock() error {
	if l == nil || l.file == nil {
		return nil
	}
	var ol windows.Overlapped
	err := windows.UnlockFileEx(windows.Handle(l.file.Fd()), 0, 1, 0, &ol)
	closeErr := l.file.Close()
	if err != nil {
		return err
	}
	return closeErr
}
