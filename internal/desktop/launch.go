package desktop

import (
	"os"
	"os/exec"
)

// No CommandContext: cancellation/closing of a window must not kill the core.
func launch(executable, dir string) (int, <-chan error, error) {
	cmd := exec.Command(executable, "--data-dir", dir, "--core")
	detach(cmd)
	sink, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		return 0, nil, err
	}
	defer sink.Close()
	cmd.Stdin, cmd.Stdout, cmd.Stderr = sink, sink, sink
	if err := cmd.Start(); err != nil {
		return 0, nil, err
	}
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait(); close(exited) }()
	return cmd.Process.Pid, exited, nil
}

// The helper must outlive the old window; no CommandContext or inherited pipes.
func launchUpdateHelper(executable string, args []string) error {
	cmd := exec.Command(executable, args...)
	detach(cmd)
	sink, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer sink.Close()
	cmd.Stdin, cmd.Stdout, cmd.Stderr = sink, sink, sink
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
