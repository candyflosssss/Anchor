package desktop

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/candyflosssss/Anchor/internal/client"
	"github.com/candyflosssss/Anchor/internal/connection"
	"github.com/candyflosssss/Anchor/internal/platform"
	"github.com/candyflosssss/Anchor/internal/state"
	"github.com/candyflosssss/Anchor/internal/updater"
)

// Options is host-only configuration. It is never exposed to JavaScript.
type Options struct {
	Dir, Executable, ControlAddress, GatewayAddress string
	StartTimeout                                    time.Duration
	SaveLogFile                                     func() (string, error)
	Quit                                            func()
}
type Session struct {
	mu                   sync.Mutex
	updateMu             sync.Mutex
	updateCheck          *updater.Check
	remoteUpdateCheck    *updater.Check
	remoteUpdateInstance string
	updating             bool
	ctx                  context.Context
	cancel               context.CancelFunc
	options              Options
	cfg                  *state.Settings
	remote               *connection.Live
	launchedPID          int
}

func NewSession(o Options) *Session {
	if o.ControlAddress == "" {
		o.ControlAddress = "127.0.0.1:17861"
	}
	if o.GatewayAddress == "" {
		o.GatewayAddress = "127.0.0.1:17860"
	}
	if o.StartTimeout == 0 {
		o.StartTimeout = 20 * time.Second
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Session{ctx: ctx, cancel: cancel, options: o}
}

// Close releases the host's lifetime only; it deliberately never kills the core.
func (s *Session) Close() {
	s.cancel()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.remote != nil {
		s.remote.Close()
		s.remote = nil
	}
}

// An authenticated process identity prevents a concurrently started CLI core
// from being mislabelled as our child. Missing IDs on old cores mean reused.
// This is attribution only; shutdown always needs fresh auth/identity + approval.
func (s *Session) source(info client.Instance) string {
	if s.remote != nil {
		return "remote"
	}
	if s.launchedPID > 0 && info.ProcessID == s.launchedPID {
		return "started"
	}
	return "reused"
}

func problem(err error, writing bool) *Problem {
	if err == nil {
		return nil
	}
	if errors.Is(err, client.ErrIdentity) {
		return &Problem{"identity"}
	}
	var transport *connection.Error
	if errors.As(err, &transport) {
		return &Problem{transport.Code}
	}
	var r *client.RequestError
	if errors.As(err, &r) {
		if r.Unknown {
			return &Problem{"unknown"}
		}
		switch r.Status {
		case 401, 403:
			return &Problem{"unauthorized"}
		case 409:
			switch r.Code {
			case "agent_operation_failed", "agent_model_unavailable", "agent_catalog_invalid", "agent_config_conflict", "agent_config_unsafe", "skill_unsafe", "skill_changed", "skill_link_unavailable":
				return &Problem{r.Code}
			}
			if r.Code == "referenced" {
				return &Problem{"referenced"}
			}
			return &Problem{"conflict"}
		case 400:
			if r.Code == "skill_limit" {
				return &Problem{"skill_limit"}
			}
			return &Problem{"invalid"}
		case 404:
			return &Problem{"unsupported"}
		}
		if r.Code == "offline" {
			return &Problem{"offline"}
		}
	}
	if writing {
		return &Problem{"unknown"}
	}
	return &Problem{"read_error"}
}
func pause(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(75 * time.Millisecond):
		return true
	}
}

// readLocal uses the same protected local state as CLI; credentials stay in Go.
func (s *Session) readLocal() (state.Settings, string, error) {
	st, err := state.Open(s.options.Dir, false)
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
func connect(ctx context.Context, cfg state.Settings, token string) (*client.Client, client.Instance, error) {
	return client.Connect(ctx, state.Context{Name: "desktop", CredentialRef: "local-owner", Endpoint: "http://" + cfg.ControlAddress, InstanceID: cfg.InstanceID}, token)
}
func (s *Session) connection(ctx context.Context) (*client.Client, client.Instance, error) {
	if s.remote != nil {
		return s.remote.Connect(ctx)
	}
	if s.cfg == nil {
		return nil, client.Instance{}, &client.RequestError{Code: "offline"}
	}
	cfg, token, err := s.readLocal()
	if err != nil {
		return nil, client.Instance{}, err
	}
	if cfg.InstanceID != s.cfg.InstanceID {
		return nil, client.Instance{}, client.ErrIdentity
	}
	return connect(ctx, cfg, token)
}
func (s *Session) startLocal() SessionResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := SessionResult{Contract: Contract}
	if s.updating {
		result.Problem = &Problem{"conflict"}
		return result
	}
	previous := s.remote
	// Restore the active remote if local startup fails; the old tunnel stays live.
	var oldCfg *state.Settings = s.cfg
	oldPID := s.launchedPID
	defer func() {
		if s.cfg == nil && previous != nil {
			s.cfg = oldCfg
			s.remote = previous
			s.launchedPID = oldPID
		}
	}()
	s.remote = nil
	s.cfg = nil
	s.launchedPID = 0
	if s.ctx.Err() != nil {
		result.Problem = &Problem{"closed"}
		return result
	}
	ctx, cancel := context.WithTimeout(s.ctx, s.options.StartTimeout)
	defer cancel()
	if err := platform.SecureDir(s.options.Dir); err != nil {
		result.Problem = &Problem{"startup_failed"}
		return result
	}
	var lock *platform.Lock
	for {
		var err error
		lock, err = platform.AcquireLock(filepath.Join(s.options.Dir, "desktop-start.lock"))
		if err == nil {
			break
		}
		if !pause(ctx) {
			result.Problem = &Problem{"startup_failed"}
			return result
		}
	}
	defer lock.Close()
	// Initialization is safe only under the core's own lock. A running instance
	// can be read without taking its lifetime lock; no force-unlock is performed.
	for {
		if ctx.Err() != nil {
			result.Problem = &Problem{"startup_failed"}
			return result
		}
		instanceLock, lockErr := platform.AcquireLock(filepath.Join(s.options.Dir, "instance.lock"))
		if lockErr == nil {
			_, statErr := os.Lstat(filepath.Join(s.options.Dir, "state.db"))
			create := errors.Is(statErr, os.ErrNotExist)
			st, err := state.Open(s.options.Dir, create)
			if err == nil {
				_, err = st.Initialize(s.options.ControlAddress, s.options.GatewayAddress)
				st.Close()
			}
			instanceLock.Close()
			if err != nil {
				result.Problem = &Problem{"startup_failed"}
				return result
			}
		}
		cfg, token, err := s.readLocal()
		if err != nil {
			if lockErr != nil && pause(ctx) {
				continue
			}
			result.Problem = &Problem{"startup_failed"}
			return result
		}
		attempt, done := context.WithTimeout(ctx, time.Second)
		cl, info, err := connect(attempt, cfg, token)
		if err == nil {
			status, statusErr := cl.Status(attempt)
			cl.Close()
			done()
			if statusErr == nil && status["core"] == "ready" {
				s.cfg = &cfg
				if st, e := state.Open(s.options.Dir, false); e == nil {
					_ = st.SelectDesktop("@local")
					st.Close()
				}
				result.Data = s.sessionInfo(info)
				if previous != nil {
					previous.Close()
				}
				return result
			}
			if !pause(ctx) {
				result.Problem = &Problem{"startup_failed"}
				return result
			}
			continue
		}
		done()
		p := problem(err, false)
		if p.Code != "offline" {
			result.Problem = p
			return result
		}
		if lockErr != nil {
			if !pause(ctx) {
				result.Problem = &Problem{"startup_failed"}
				return result
			}
			continue
		}
		// Release startup coordination only after the child reaches readiness. A
		// concurrent CLI serve is still protected by the core's instance lock.
		pid, exited, err := launch(s.options.Executable, s.options.Dir)
		if err != nil {
			result.Problem = &Problem{"startup_failed"}
			return result
		}
		s.launchedPID = pid
		for {
			if !pause(ctx) {
				result.Problem = &Problem{"startup_failed"}
				return result
			}
			attempt, done := context.WithTimeout(ctx, time.Second)
			cl, info, err := connect(attempt, cfg, token)
			if err == nil {
				status, e := cl.Status(attempt)
				cl.Close()
				done()
				if e == nil && status["core"] == "ready" {
					s.cfg = &cfg
					if st, e := state.Open(s.options.Dir, false); e == nil {
						_ = st.SelectDesktop("@local")
						st.Close()
					}
					result.Data = s.sessionInfo(info)
					if previous != nil {
						previous.Close()
					}
					return result
				}
			} else {
				done()
				if p := problem(err, false); p.Code != "offline" {
					result.Problem = p
					return result
				}
			}
			select {
			case <-exited:
				result.Problem = &Problem{"startup_failed"}
				return result
			default:
			}
		}
	}
}

func (s *Session) sessionInfo(info client.Instance) *SessionInfo {
	out := &SessionInfo{InstanceID: info.InstanceID, Phase: "ready", Source: s.source(info), CanStop: slices.Contains(info.Capabilities, "graceful_stop"), ProfileName: "@local", Transport: "local", Target: "本机"}
	if s.remote != nil {
		out.ProfileName = s.remote.Profile.Name
		out.Transport = "ssh"
		out.Target = s.remote.Profile.SSHTarget
	}
	return out
}
func (s *Session) start() SessionResult {
	s.mu.Lock()
	if s.remote != nil {
		name := s.remote.Profile.Name
		s.mu.Unlock()
		return NewBridge(s).UseConnection(SelectConnection{Name: name})
	}
	s.mu.Unlock()
	if st, err := state.Open(s.options.Dir, false); err == nil {
		name, e := st.DesktopSelection()
		st.Close()
		if e == nil && name != "@local" {
			return NewBridge(s).UseConnection(SelectConnection{Name: name})
		}
	}
	return s.startLocal()
}
