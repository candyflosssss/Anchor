package updater

import (
	"os/exec"
	"syscall"
)

func detachRuntime(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x00000200 | 0x00000008}
}
func hideRuntimeWindow(cmd *exec.Cmd)              { cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true} }
func execRuntime(path string, args []string) error { return ErrUnavailable }
