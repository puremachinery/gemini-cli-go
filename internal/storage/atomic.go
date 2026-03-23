package storage

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
)

// WriteFileAtomic writes data to path by writing a temp file and renaming it into place.
func WriteFileAtomic(path string, data []byte, perm fs.FileMode) error {
	if path == "" {
		return fmt.Errorf("path is empty")
	}
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	temp, err := os.CreateTemp(dir, "."+base+".tmp-*")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	committed := false
	defer func() {
		if committed {
			return
		}
		if err := temp.Close(); err != nil {
			_ = err
		}
		if err := os.Remove(tempName); err != nil {
			_ = err
		}
	}()

	if err := temp.Chmod(perm); err != nil {
		return err
	}
	if _, err := temp.Write(data); err != nil {
		return err
	}
	if err := temp.Sync(); err != nil {
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempName, path); err != nil {
		if runtime.GOOS != "windows" {
			return err
		}
		if removeErr := os.Remove(path); removeErr != nil && !os.IsNotExist(removeErr) {
			return err
		}
		if renameErr := os.Rename(tempName, path); renameErr != nil {
			return renameErr
		}
	}
	committed = true
	return nil
}
