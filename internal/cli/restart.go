package cli

import (
	"context"
	"errors"
	"os"
	"time"

	"github.com/candyflosssss/Anchor/internal/connection"
	"github.com/candyflosssss/Anchor/internal/state"
)

func (a App) restart(ctx context.Context, dir, target string, args []string) error {
	f := a.flags("restart")
	approve := f.Bool("approve", false, "approve selected core restart; preserves managed configuration")
	timeout := f.Duration("timeout", 30*time.Second, "total shutdown/startup deadline (1s..30s)")
	expected := f.String("expected-instance", "", "require this local instance identity before restarting (SSH safety pin)")
	port := f.Int("control-port", 0, "require this local control port before restarting (SSH safety pin)")
	if err := parse(f, args); err != nil {
		return err
	}
	if !*approve {
		return errors.New("restart requires --approve; requests are briefly interrupted")
	}
	if *timeout < time.Second || *timeout > 30*time.Second {
		return errors.New("restart --timeout must be between 1s and 30s")
	}
	if *port < 0 || *port > 65535 || (*port != 0 && *expected == "") || (*expected != "" && !state.ValidOpaqueID(*expected)) {
		return errors.New("invalid restart identity/control-port pin")
	}
	ctx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()
	if target != "" {
		if *expected != "" || *port != 0 {
			return errors.New("local restart pins cannot be used with --context")
		}
		st, err := state.Open(dir, false)
		if err != nil {
			return err
		}
		defer st.Close()
		profile, err := st.Context(target)
		if err != nil {
			return err
		}
		if profile.Transport != "ssh" {
			return errors.New("restart --context requires an SSH context")
		}
		live, err := connection.OpenWithPassword(ctx, st, profile, a.sshPassword)
		if err != nil {
			return err
		}
		defer live.Close()
		result, err := live.RestartCore(ctx)
		if err != nil {
			return err
		}
		return a.output(map[string]any{"ready": true, "instance_id": result.InstanceID, "process_id": result.ProcessID, "recovered": false})
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	result, err := connection.RestartLocalPinned(ctx, dir, *expected, *port, func() (int, <-chan error, error) { return connection.LaunchLocalCore(executable, dir) })
	if err != nil {
		return err
	}
	return a.output(map[string]any{"ready": true, "instance_id": result.Instance.InstanceID, "process_id": result.Instance.ProcessID, "recovered": result.Recovered})
}
