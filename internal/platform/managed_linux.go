package platform

import (
	"errors"
	"golang.org/x/sys/unix"
	"os"
)

func checkReparse(string) error { return nil }
func checkManagedOwner(path string) error {
	var st unix.Stat_t
	if e := unix.Lstat(path, &st); e != nil {
		return e
	}
	if st.Mode&unix.S_IFMT == unix.S_IFREG && st.Nlink != 1 {
		return errors.New("hard-linked configuration files are unsupported")
	}
	if st.Uid != uint32(os.Geteuid()) || st.Mode&0022 != 0 {
		return errors.New("configuration root/file must be owned by this user and not group/world writable")
	}
	return nil
}
func PreserveManagedPermissions(src, dst string) error {
	st, e := os.Stat(src)
	if os.IsNotExist(e) {
		return SecureFile(dst)
	}
	if e != nil {
		return e
	}
	return os.Chmod(dst, st.Mode().Perm())
}
func ReplaceManagedFile(src, dst string) error { return os.Rename(src, dst) }
func SyncManagedDir(path string) error {
	f, e := os.Open(path)
	if e != nil {
		return e
	}
	defer f.Close()
	return f.Sync()
}
