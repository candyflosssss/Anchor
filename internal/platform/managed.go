package platform

import (
	"errors"
	"os"
	"path/filepath"
)

// CheckManagedPath must be rerun before publication. No symlink or reparse
// component is accepted, including components above the approved root.
func CheckManagedPath(root, path string) error {
	if !filepath.IsAbs(root) || filepath.Dir(path) != root {
		return errors.New("target must be directly inside the approved absolute root")
	}
	for p := path; ; p = filepath.Dir(p) {
		st, e := os.Lstat(p)
		if e == nil {
			if st.Mode()&os.ModeSymlink != 0 {
				return errors.New("linked configuration path is unsupported")
			}
			if e = checkReparse(p); e != nil {
				return e
			}
			if p != path && !st.IsDir() {
				return errors.New("configuration ancestor is not a directory")
			}
		}
		if e != nil && !errors.Is(e, os.ErrNotExist) {
			return e
		}
		if filepath.Dir(p) == p {
			break
		}
	}
	st, e := os.Stat(root)
	if e != nil || !st.IsDir() {
		return errors.New("approved configuration root must already exist")
	}
	if e = checkManagedOwner(root); e != nil {
		return e
	}
	if st, e = os.Lstat(path); e == nil {
		if !st.Mode().IsRegular() {
			return errors.New("configuration is not a regular file")
		}
		return checkManagedOwner(path)
	} else if !errors.Is(e, os.ErrNotExist) {
		return e
	}
	return nil
}
