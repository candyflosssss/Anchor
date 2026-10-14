package desktop

import (
	"context"
	"errors"
	"time"

	"github.com/candyflosssss/Anchor/internal/connection"
	"github.com/candyflosssss/Anchor/internal/state"
)

func (b *Bridge) Connections() ConnectionProfilesResult {
	s := b.session
	s.mu.Lock()
	defer s.mu.Unlock()
	out := ConnectionProfilesResult{Contract: Contract, Data: []ConnectionProfile{{Name: "@local", Authentication: "owner", Transport: "local", Target: "本机", Active: s.remote == nil && s.cfg != nil}}}
	st, err := state.Open(s.options.Dir, false)
	if err != nil {
		out.Problem = problem(err, false)
		return out
	}
	defer st.Close()
	if cfg, err := st.Settings(); err == nil {
		out.Data[0].InstanceID = cfg.InstanceID
	}
	profiles, err := st.Contexts()
	if err != nil {
		out.Problem = problem(err, false)
		return out
	}
	for _, c := range profiles {
		if c.Transport == "ssh" {
			auth := "key"
			if c.SSHAuth == "password" {
				auth = "password"
			}
			out.Data = append(out.Data, ConnectionProfile{Authentication: auth, Name: c.Name, Transport: "ssh", Target: c.SSHTarget, InstanceID: c.InstanceID, Active: s.remote != nil && s.remote.Profile.Name == c.Name})
		}
	}
	return out
}
func (b *Bridge) ConnectSSH(in SSHConnectionInput) SessionResult {
	s := b.session
	s.mu.Lock()
	defer s.mu.Unlock()
	out := SessionResult{Contract: Contract}
	if s.ctx.Err() != nil {
		out.Problem = &Problem{"closed"}
		return out
	}
	if s.updating {
		out.Problem = &Problem{"conflict"}
		return out
	}
	st, err := state.Open(s.options.Dir, false)
	if err != nil {
		out.Problem = problem(err, false)
		return out
	}
	defer st.Close()
	if _, err = st.Context(in.Name); !errors.Is(err, state.ErrNotFound) {
		out.Problem = &Problem{"conflict"}
		return out
	}
	ctx, cancel := context.WithTimeout(s.ctx, 45*time.Second)
	defer cancel()
	live, token, err := connection.EnrollConnection(ctx, state.Context{Name: in.Name, SSHTarget: in.Target, SSHPort: in.Port, SSHIdentityFile: in.IdentityFile, RemoteExecutable: in.RemoteExecutable, RemoteControlPort: in.RemoteControlPort, TrustNewHost: in.TrustNewHost}, in.Password)
	if err != nil {
		out.Problem = problem(err, false)
		return out
	}
	profile := live.Profile
	cl, info, err := live.Connect(ctx)
	if err != nil {
		live.Close()
		out.Problem = problem(err, false)
		return out
	}
	status, err := cl.Status(ctx)
	cl.Close()
	if err != nil || status["core"] != "ready" {
		live.Close()
		if err != nil {
			out.Problem = problem(err, false)
		} else {
			out.Problem = &Problem{"offline"}
		}
		return out
	}
	if err = st.PutContextWithCredential(profile, token); err != nil {
		live.Close()
		out.Problem = problem(err, false)
		return out
	}
	if err = st.SelectDesktop(profile.Name); err != nil {
		live.Close()
		out.Problem = problem(err, false)
		return out
	}
	s.activateRemote(live)
	out.Data = s.sessionInfo(info)
	return out
}
func (b *Bridge) UseConnection(in SelectConnection) SessionResult {
	s := b.session
	if in.Name == "@local" {
		return s.startLocal()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := SessionResult{Contract: Contract}
	if s.ctx.Err() != nil {
		out.Problem = &Problem{"closed"}
		return out
	}
	if s.updating {
		out.Problem = &Problem{"conflict"}
		return out
	}
	st, err := state.Open(s.options.Dir, false)
	if err != nil {
		out.Problem = problem(err, false)
		return out
	}
	defer st.Close()
	profile, err := st.Context(in.Name)
	if err != nil || profile.Transport != "ssh" {
		out.Problem = &Problem{"invalid"}
		return out
	}
	ctx, cancel := context.WithTimeout(s.ctx, 20*time.Second)
	defer cancel()
	if profile.SSHAuth == "password" && in.Password == "" && s.remote != nil && s.remote.Profile.Name == profile.Name {
		cl, info, err := s.remote.Connect(ctx)
		if err != nil {
			out.Problem = problem(err, false)
			return out
		}
		defer cl.Close()
		out.Data = s.sessionInfo(info)
		return out
	}
	live, err := connection.OpenWithPassword(ctx, st, profile, in.Password)
	if err != nil {
		out.Problem = problem(err, false)
		return out
	}
	cl, info, err := live.Connect(ctx)
	if err != nil {
		live.Close()
		out.Problem = problem(err, false)
		return out
	}
	status, err := cl.Status(ctx)
	cl.Close()
	if err != nil || status["core"] != "ready" {
		live.Close()
		if err != nil {
			out.Problem = problem(err, false)
		} else {
			out.Problem = &Problem{"offline"}
		}
		return out
	}
	if err = st.SelectDesktop(profile.Name); err != nil {
		live.Close()
		out.Problem = problem(err, false)
		return out
	}
	s.activateRemote(live)
	out.Data = s.sessionInfo(info)
	return out
}
func (s *Session) activateRemote(live *connection.Live) {
	if s.remote != nil {
		s.remote.Close()
	}
	s.remote = live
	s.cfg = &state.Settings{InstanceID: live.Profile.InstanceID}
	s.launchedPID = 0
}
