package connection

import (
	"context"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"github.com/candyflosssss/Anchor/internal/client"
	"github.com/candyflosssss/Anchor/internal/platform"
	"github.com/candyflosssss/Anchor/internal/state"
)

// EnsureLocal initializes/reuses the SSH account's Anchor and starts a detached
// core if needed. It never replaces identities or kills a port occupant.
func EnsureLocal(ctx context.Context, dir string, port int) (*state.Store, error) {
	if port < 1 || port > 65535 {
		return nil, failure("invalid")
	}
	if err := platform.SecureDir(dir); err != nil {
		return nil, err
	}
	lock, err := platform.AcquireLock(filepath.Join(dir, "remote-start.lock"))
	if err != nil {
		return nil, failure("conflict")
	}
	defer lock.Close()
	_, err = os.Lstat(filepath.Join(dir, "state.db"))
	create := errors.Is(err, os.ErrNotExist)
	if create {
		instance, err := platform.AcquireLock(filepath.Join(dir, "instance.lock"))
		if err != nil {
			return nil, err
		}
		s, err := state.Open(dir, true)
		if err == nil {
			_, err = s.Initialize(net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), "127.0.0.1:17860")
			s.Close()
		}
		instance.Close()
		if err != nil {
			return nil, err
		}
	}
	s, err := state.Open(dir, false)
	if err != nil {
		return nil, err
	}
	fail := func(err error) (*state.Store, error) { s.Close(); return nil, err }
	cfg, err := s.Settings()
	if err != nil {
		return fail(err)
	}
	if cfg.ControlAddress != net.JoinHostPort("127.0.0.1", strconv.Itoa(port)) {
		return fail(failure("remote_anchor"))
	}
	token, err := s.OwnerToken()
	if err != nil {
		return fail(err)
	}
	profile := state.Context{Name: "bootstrap", Endpoint: "http://" + cfg.ControlAddress, InstanceID: cfg.InstanceID, CredentialRef: "local-owner"}
	probe := func() error {
		attempt, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		cl, _, err := client.Connect(attempt, profile, token)
		if err == nil {
			defer cl.Close()
			status, e := cl.Status(attempt)
			if e != nil {
				return e
			}
			if status["core"] != "ready" {
				return failure("remote_anchor")
			}
		}
		return err
	}
	err = probe()
	if err == nil {
		return s, nil
	}
	var req *client.RequestError
	if !errors.As(err, &req) || req.Code != "offline" {
		return fail(err)
	}
	instance, err := platform.AcquireLock(filepath.Join(dir, "instance.lock"))
	if err != nil {
		// Another launcher may already be bringing up the same identity.
		for i := 0; i < 100; i++ {
			select {
			case <-ctx.Done():
				return fail(ctx.Err())
			case <-time.After(100 * time.Millisecond):
			}
			if probe() == nil {
				return s, nil
			}
		}
		return fail(failure("remote_anchor"))
	}
	instance.Close()
	executable, err := os.Executable()
	if err != nil {
		return fail(err)
	}
	cmd := exec.Command(executable, "--data-dir", dir, "serve")
	detachCore(cmd)
	sink, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		return fail(err)
	}
	cmd.Stdin, cmd.Stdout, cmd.Stderr = sink, sink, sink
	err = cmd.Start()
	sink.Close()
	if err != nil {
		return fail(err)
	}
	exited := make(chan struct{})
	go func() { _ = cmd.Wait(); close(exited) }()
	for i := 0; i < 150; i++ {
		select {
		case <-ctx.Done():
			return fail(ctx.Err())
		case <-exited:
			return fail(failure("remote_anchor"))
		case <-time.After(100 * time.Millisecond):
		}
		if probe() == nil {
			return s, nil
		}
	}
	return fail(failure("remote_anchor"))
}
