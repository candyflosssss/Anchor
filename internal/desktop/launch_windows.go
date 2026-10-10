package desktop

import (
	"os/exec"
	"syscall"
)

func detach(cmd *exec.Cmd) {
	// CREATE_NEW_PROCESS_GROUP + DETACHED_PROCESS; no inherited console or window.
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x00000200 | 0x00000008}
}
