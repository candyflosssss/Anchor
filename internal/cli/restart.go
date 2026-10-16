package cli

import (
	"context"
	"errors"
	"os"
	"time"

	"github.com/candyflosssss/Anchor/internal/connection"
)

func (a App) restart(ctx context.Context, dir string, args []string) error {
	f := a.flags("restart")
	approve := f.Bool("approve", false, "approve local core restart; preserves managed configuration")
	timeout := f.Duration("timeout", 30*time.Second, "total shutdown/startup deadline (1s..30s)")
	if err := parse(f, args); err != nil {
		return err
	}
	if !*approve {
		return errors.New("restart requires --approve; requests are briefly interrupted")
	}
	if *timeout < time.Second || *timeout > 30*time.Second {
		return errors.New("restart --timeout must be between 1s and 30s")
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()
	result, err := connection.RestartLocal(ctx, dir, "", func() (int, <-chan error, error) { return connection.LaunchLocalCore(executable, dir) })
	if err != nil {
		return err
	}
	return a.output(map[string]any{"ready": true, "instance_id": result.Instance.InstanceID, "process_id": result.Instance.ProcessID, "recovered": result.Recovered})
}
