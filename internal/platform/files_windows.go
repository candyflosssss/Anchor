package platform

import (
	"fmt"
	"golang.org/x/sys/windows"
	"os"
)

func restrict(path string, dir bool) error {
	var token windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_QUERY, &token); err != nil {
		return err
	}
	defer token.Close()
	user, err := token.GetTokenUser()
	if err != nil {
		return err
	}
	inherit := ""
	if dir {
		inherit = "OICI"
	}
	sd, err := windows.SecurityDescriptorFromString("D:P(A;" + inherit + ";FA;;;" + user.User.Sid.String() + ")")
	if err != nil {
		return err
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	// Protected DACL: do not inherit wider permissions from AppData or custom roots.
	return windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil)
}

type Lock struct {
	file       *os.File
	overlapped windows.Overlapped
}

func AcquireLock(path string) (*Lock, error) {
	if err := SecureFile(path); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	l := &Lock{file: f}
	if err := windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &l.overlapped); err != nil {
		f.Close()
		return nil, fmt.Errorf("instance already in use: %w", err)
	}
	return l, nil
}
func (l *Lock) Close() error { return l.file.Close() }
