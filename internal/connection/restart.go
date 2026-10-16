package connection

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"time"

	"github.com/candyflosssss/Anchor/internal/client"
	"github.com/candyflosssss/Anchor/internal/platform"
	"github.com/candyflosssss/Anchor/internal/state"
)

// CoreLauncher is host-owned, never supplied by an API/JavaScript caller.
type CoreLauncher func() (pid int, exited <-chan error, err error)

type RestartResult struct {
	Instance    client.Instance `json:"instance"`
	Recovered   bool            `json:"recovered"`
	LaunchedPID int             `json:"-"`
}

// RestartLocal coordinates a single, approved local restart. The caller obtains
// approval before entry. It never initializes a directory, retries a mutation,
// falls back to ordinary Stop, or kills a process. Success means ready, not just
// acceptance of shutdown. An offline instance can be recovered under the same
// identity; an occupied/unresponsive instance is not replaced.
func RestartLocal(ctx context.Context, dir, expectedID string, launch CoreLauncher) (RestartResult, error) {
	var out RestartResult
	if launch == nil {
		return out, failure("invalid")
	}
	// Bound callers too, including clients that forget to set a deadline.
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cfg, token, err := restartState(dir)
	if err != nil {
		return out, err
	}
	if expectedID != "" && expectedID != cfg.InstanceID {
		return out, client.ErrIdentity
	}
	// Coordinate with both GUI/update startup and SSH account bootstrap. Locks
	// are nonblocking: double submission is rejected, never queued for replay.
	for _, name := range []string{"desktop-start.lock", "remote-start.lock"} {
		lock, err := platform.AcquireLock(filepath.Join(dir, name))
		if err != nil {
			return out, failure("conflict")
		}
		defer lock.Close()
	}
	probe := func() (*client.Client, client.Instance, error) {
		attempt, done := context.WithTimeout(ctx, time.Second)
		defer done()
		return client.Connect(attempt, state.Context{Name: "restart", Endpoint: "http://" + cfg.ControlAddress, InstanceID: cfg.InstanceID, CredentialRef: "local-owner"}, token)
	}
	cl, info, err := probe()
	if err == nil {
		if !slices.Contains(info.Capabilities, "restart_handoff") {
			cl.Close()
			return out, failure("unsupported")
		}
		err = cl.StopForRestart(ctx)
		cl.Close()
		if err != nil {
			return out, err
		} // especially Unknown: no replay or launch
	} else {
		var request *client.RequestError
		if !errors.As(err, &request) || request.Code != "offline" {
			return out, err
		}
		out.Recovered = true
	}
	// Accepted shutdown is not completed shutdown. Keep the startup guards until
	// the old core releases its lifetime lock and the replacement becomes ready.
	for {
		if ctx.Err() != nil {
			return out, failure("startup_failed")
		}
		lock, err := platform.AcquireLock(filepath.Join(dir, "instance.lock"))
		if err == nil {
			lock.Close()
			break
		}
		if !restartPause(ctx) {
			return out, failure("startup_failed")
		}
	}
	current, currentToken, err := restartState(dir)
	if err != nil {
		return out, err
	}
	if current != cfg || currentToken != token {
		return out, client.ErrIdentity
	}
	if ctx.Err() != nil {
		return out, failure("startup_failed")
	}
	pid, exited, err := launch()
	if err != nil {
		return out, failure("startup_failed")
	}
	out.LaunchedPID = pid
	for {
		if !restartPause(ctx) {
			return out, failure("startup_failed")
		}
		cl, info, err := probe()
		if err == nil {
			attempt, done := context.WithTimeout(ctx, time.Second)
			status, statusErr := cl.Status(attempt)
			cl.Close()
			done()
			if statusErr == nil && status["core"] == "ready" {
				out.Instance = info
				return out, nil
			}
			if statusErr != nil {
				var request *client.RequestError
				if !errors.As(statusErr, &request) || (request.Code != "offline" && request.Status != 503) {
					return out, statusErr
				}
			}
		} else {
			var request *client.RequestError
			if !errors.As(err, &request) || request.Code != "offline" {
				return out, err
			}
		}
		select {
		case <-exited:
			return out, failure("startup_failed")
		default:
		}
	}
}

func restartState(dir string) (state.Settings, string, error) {
	st, err := state.Open(dir, false)
	if err != nil {
		return state.Settings{}, "", err
	}
	defer st.Close()
	cfg, err := st.Settings()
	if err != nil {
		return cfg, "", err
	}
	token, err := st.OwnerToken()
	return cfg, token, err
}
func restartPause(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(75 * time.Millisecond):
		return true
	}
}

// LaunchLocalCore starts the CLI's detached serve mode, not a shell or service
// manager. It deliberately outlives the invoking CLI and inherits no pipes.
func LaunchLocalCore(executable, dir string) (int, <-chan error, error) {
	cmd := exec.Command(executable, "--data-dir", dir, "serve")
	detachCore(cmd)
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
