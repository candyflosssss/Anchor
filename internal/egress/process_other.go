//go:build !linux && !windows

package egress

import "os/exec"

func configureProcess(c *exec.Cmd) {}

func trackProcess(c *exec.Cmd) (func(), error) { return func() {}, nil }
