package storage

import "fmt"

type lockHandle interface {
	Unlock() error
}

// WithFileLock executes fn while holding an exclusive lock for the path.
func WithFileLock(path string, fn func() error) (err error) {
	if fn == nil {
		return nil
	}
	if path == "" {
		return fn()
	}
	lock, err := lockFile(path + ".lock")
	if err != nil {
		return err
	}
	defer func() {
		if unlockErr := lock.Unlock(); unlockErr != nil && err == nil {
			err = fmt.Errorf("failed to unlock %s: %w", path, unlockErr)
		}
	}()
	return fn()
}
