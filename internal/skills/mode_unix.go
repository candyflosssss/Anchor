//go:build !windows

package skills

import "os"

func executable(mode os.FileMode) bool { return mode&0111 != 0 }
func executableFlag(value bool) bool   { return value }
