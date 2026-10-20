package updater

import (
	"os"
	"os/exec"
	"syscall"
)

func detachRuntime(cmd *exec.Cmd)     { cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} }
func hideRuntimeWindow(cmd *exec.Cmd) {}
func execRuntime(path string, args []string) error {
	return syscall.Exec(path, append([]string{path}, args...), os.Environ())
}
