//go:build !windows

package storage

import (
	"fmt"
	"os"
	"syscall"
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
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
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
	err := syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN)
	closeErr := l.file.Close()
	if err != nil {
		return err
	}
	return closeErr
}
