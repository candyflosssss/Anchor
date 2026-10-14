package connection

import (
	"os/exec"
	"syscall"
)

func hideWindow(*exec.Cmd)     {}
func detachCore(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} }
