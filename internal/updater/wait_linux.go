package updater

import (
	"context"
	"errors"
	"fmt"
	"golang.org/x/sys/unix"
	"time"
)

func waitParent(ctx context.Context, pid int) error {
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	for {
		err := unix.Kill(pid, 0) // existence only; never sends a termination signal
		if errors.Is(err, unix.ESRCH) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("cannot verify update window exit")
		}
		if err := sleep(ctx); err != nil {
			return err
		}
	}
}
