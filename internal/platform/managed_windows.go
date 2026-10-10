package platform

import (
	"errors"
	"golang.org/x/sys/windows"
	"os"
)

func checkReparse(path string) error {
	p, e := windows.UTF16PtrFromString(path)
	if e != nil {
		return e
	}
	a, e := windows.GetFileAttributes(p)
	if e != nil {
		return e
	}
	if a&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return errors.New("reparse configuration path is unsupported")
	}
	return nil
}
func checkManagedOwner(path string) error {
	sd, e := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION)
	if e != nil {
		return e
	}
	owner, _, e := sd.Owner()
	if e != nil {
		return e
	}
	var token windows.Token
	if e = windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_QUERY, &token); e != nil {
		return e
	}
	defer token.Close()
	u, e := token.GetTokenUser()
	if e != nil {
		return e
	}
	if !owner.Equals(u.User.Sid) {
		return errors.New("configuration must be owned by the current user")
	}
	return nil
}
func PreserveManagedPermissions(src, dst string) error {
	if _, e := os.Stat(src); os.IsNotExist(e) {
		return SecureFile(dst)
	}
	sd, e := windows.GetNamedSecurityInfo(src, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if e != nil {
		return e
	}
	acl, _, e := sd.DACL()
	if e != nil {
		return e
	}
	return windows.SetNamedSecurityInfo(dst, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil)
}
func ReplaceManagedFile(src, dst string) error {
	a, e := windows.UTF16PtrFromString(src)
	if e != nil {
		return e
	}
	b, e := windows.UTF16PtrFromString(dst)
	if e != nil {
		return e
	}
	return windows.MoveFileEx(a, b, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
}
func SyncManagedDir(string) error { return nil } // MoveFileEx WRITE_THROUGH supplies publication durability.
