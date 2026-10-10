package platform

import (
	"fmt"
	"golang.org/x/sys/unix"
	"os"
)

func restrict(path string, dir bool) error {
	var st unix.Stat_t
	if err := unix.Lstat(path, &st); err != nil {
		return err
	}
	if st.Uid != uint32(os.Geteuid()) {
		return fmt.Errorf("storage must be owned by the current user: %s", path)
	}
	mode := os.FileMode(0600)
	if dir {
		mode = 0700
	}
	return os.Chmod(path, mode)
}

// Lock is a kernel-held lock; a crashed process cannot leave it held.
type Lock struct{ file *os.File }

func AcquireLock(path string) (*Lock, error) {
	if err := SecureFile(path); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		f.Close()
		return nil, fmt.Errorf("instance already in use: %w", err)
	}
	return &Lock{f}, nil
}
func (l *Lock) Close() error { return l.file.Close() }
