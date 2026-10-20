// Package updateflow coordinates shared, approved CLI/desktop core handoff.
package updateflow

import (
	"context"
	"errors"
	"net"
	"strconv"

	"github.com/candyflosssss/Anchor/internal/client"
	"github.com/candyflosssss/Anchor/internal/connection"
	"github.com/candyflosssss/Anchor/internal/state"
	"github.com/candyflosssss/Anchor/internal/updater"
)

// VerifyLocal does not initialize, enroll or renew any credentials.
func VerifyLocal(dir, expectedID string, port int) (state.Settings, error) {
	st, err := state.Open(dir, false)
	if err != nil {
		return state.Settings{}, err
	}
	defer st.Close()
	cfg, err := st.Settings()
	if err != nil {
		return cfg, err
	}
	if expectedID != "" && cfg.InstanceID != expectedID {
		return cfg, client.ErrIdentity
	}
	if port != 0 && (expectedID == "" || port < 1 || port > 65535 || cfg.ControlAddress != net.JoinHostPort("127.0.0.1", strconv.Itoa(port))) {
		return cfg, &connection.Error{Code: "remote_control_port"}
	}
	return cfg, nil
}

// Switch performs one normal, configuration-preserving restart, with no kill,
// mutation retry or database rollback. Caller holds updater.Lock throughout.
func Switch(ctx context.Context, dir string, service *updater.Service, r updater.Runtime, expectedID string, port int) (updater.RuntimeResult, error) {
	var out updater.RuntimeResult
	cfg, err := VerifyLocal(dir, expectedID, port)
	if err != nil {
		return out, err
	}
	st, err := state.Open(dir, false)
	if err != nil {
		return out, err
	}
	token, err := st.OwnerToken()
	st.Close()
	if err != nil {
		return out, err
	}
	cl, info, err := client.Connect(ctx, state.Context{Name: "update", Endpoint: "http://" + cfg.ControlAddress, InstanceID: cfg.InstanceID, CredentialRef: "local-owner"}, token)
	if err == nil {
		if info.Version == r.Version {
			status, e := cl.Status(ctx)
			cl.Close()
			if e != nil {
				return out, e
			}
			if status["core"] != "ready" {
				return out, &connection.Error{Code: "remote_core_not_ready"}
			}
			if e := service.ActivateRuntime(dir, r); e != nil {
				return out, e
			}
			return updater.RuntimeResult{Ready: true, Version: r.Version, InstanceID: info.InstanceID, ProcessID: info.ProcessID}, nil
		}
		cl.Close()
		newer, e := updater.IsNewerVersion(info.Version, r.Version)
		if e != nil {
			return out, e
		}
		if newer {
			return out, updater.ErrUnavailable
		}
	} else {
		var request *client.RequestError
		if !errors.As(err, &request) || request.Code != "offline" {
			return out, err
		}
	}
	result, err := connection.RestartLocalPinned(ctx, dir, cfg.InstanceID, port, func() (int, <-chan error, error) {
		mode := "serve"
		if service.Product == "desktop" {
			mode = "--core"
		}
		return updater.LaunchRuntime(r.Path, "--data-dir", dir, mode)
	})
	if err != nil {
		return out, err
	}
	if result.Instance.Version != r.Version {
		return out, errors.New("replacement core version did not match the approved update; inspect status before retrying")
	}
	if err := service.ActivateRuntime(dir, r); err != nil {
		return out, errors.New("new core is ready but its launcher could not be saved; inspect status before retrying")
	}
	return updater.RuntimeResult{Ready: true, Version: r.Version, InstanceID: result.Instance.InstanceID, ProcessID: result.Instance.ProcessID}, nil
}

func Apply(ctx context.Context, dir string, service *updater.Service, approved updater.InstallApproval, expectedID string, port int) (updater.RuntimeResult, error) {
	var out updater.RuntimeResult
	if _, err := VerifyLocal(dir, expectedID, port); err != nil {
		return out, err
	}
	lock, err := updater.Lock(dir)
	if err != nil {
		return out, err
	}
	defer lock.Close()
	r, err := service.PrepareRuntime(ctx, dir, approved)
	if err != nil {
		return out, err
	}
	return Switch(ctx, dir, service, r, expectedID, port)
}
