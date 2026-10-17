package desktop

import (
	"context"
	"time"

	"github.com/candyflosssss/Anchor/internal/connection"
	"github.com/candyflosssss/Anchor/internal/state"
)

// RestartCore controls only the confirmed selected instance. SSH never falls back to local.
func (b *Bridge) RestartCore(in ConfirmStop) SessionResult {
	s := b.session
	out := SessionResult{Contract: Contract}
	if !s.mu.TryLock() {
		out.Problem = &Problem{"conflict"}
		return out
	}
	defer s.mu.Unlock()
	if !in.Approved || in.InstanceID == "" {
		out.Problem = &Problem{"invalid"}
		return out
	}
	if s.updating {
		out.Problem = &Problem{"conflict"}
		return out
	}
	if s.ctx.Err() != nil {
		out.Problem = &Problem{"closed"}
		return out
	}
	if s.remote != nil {
		if s.remote.Profile.InstanceID != in.InstanceID {
			out.Problem = &Problem{"identity"}
			return out
		}
		ctx, cancel := context.WithTimeout(s.ctx, 30*time.Second)
		defer cancel()
		info, err := s.remote.RestartCore(ctx)
		if err != nil {
			out.Problem = problem(err, true)
			return out
		}
		out.Data = s.sessionInfo(info)
		return out
	}
	if s.cfg != nil && s.cfg.InstanceID != in.InstanceID {
		out.Problem = &Problem{"identity"}
		return out
	}
	st, err := state.Open(s.options.Dir, false)
	if err != nil {
		out.Problem = problem(err, false)
		return out
	}
	selected, err := st.DesktopSelection()
	cfg, settingsErr := st.Settings()
	st.Close()
	if err != nil {
		out.Problem = problem(err, false)
		return out
	}
	if selected != "@local" {
		out.Problem = &Problem{"unsupported"}
		return out
	}
	if settingsErr != nil {
		out.Problem = problem(settingsErr, false)
		return out
	}
	if cfg.InstanceID != in.InstanceID {
		out.Problem = &Problem{"identity"}
		return out
	}
	// Retain the verified target even on failure, so a subsequent Status really
	// probes it (including when a previous ordinary Stop cleared the session).
	s.cfg = &cfg
	ctx, cancel := context.WithTimeout(s.ctx, s.options.StartTimeout+10*time.Second)
	defer cancel()
	result, err := connection.RestartLocal(ctx, s.options.Dir, in.InstanceID, func() (int, <-chan error, error) { return launch(s.options.Executable, s.options.Dir) })
	if err != nil {
		out.Problem = problem(err, true)
		return out
	}
	cfg, _, err = s.readLocal()
	if err != nil {
		out.Problem = problem(err, false)
		return out
	}
	if cfg.InstanceID != in.InstanceID {
		out.Problem = &Problem{"identity"}
		return out
	}
	s.cfg = &cfg
	s.launchedPID = result.LaunchedPID
	out.Data = s.sessionInfo(result.Instance)
	return out
}
