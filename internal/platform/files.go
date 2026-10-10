// Package platform holds the OS-specific security and locking boundary.
package platform

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// SecureDir rejects links rather than silently following a new storage target.
func SecureDir(path string) error {
	path, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	for p := path; ; p = filepath.Dir(p) {
		info, err := os.Lstat(p)
		if err == nil {
			if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
				return fmt.Errorf("storage path is not a real directory: %s", p)
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if parent := filepath.Dir(p); parent == p {
			break
		}
	}
	if err := os.MkdirAll(path, 0700); err != nil {
		return err
	}
	return restrict(path, true)
}

func SecureFile(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		if err = f.Close(); err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else if !info.Mode().IsRegular() {
		return fmt.Errorf("storage file is not a regular file: %s", path)
	}
	return restrict(path, false)
}
