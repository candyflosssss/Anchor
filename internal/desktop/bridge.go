package desktop

import (
	"context"
	"slices"
	"time"

	"github.com/candyflosssss/Anchor/internal/state"
)

// Bridge is the only object bound to Wails. Host lifetime/configuration is not exported.
type Bridge struct{ session *Session }

func NewBridge(s *Session) *Bridge     { return &Bridge{session: s} }
func (b *Bridge) Start() SessionResult { return b.session.start() }

// ExitDesktop exits only this GUI host; the detached core keeps running.
// The host callback also bypasses close-to-tray for update installation.
func (b *Bridge) ExitDesktop() StopResult {
	s := b.session
	s.mu.Lock()
	defer s.mu.Unlock()
	out := StopResult{Contract: Contract}
	if s.updating {
		out.Problem = &Problem{"conflict"}
		return out
	}
	if s.options.Quit == nil {
		out.Problem = &Problem{"unsupported"}
		return out
	}
	s.options.Quit()
	out.Data = &StopInfo{Accepted: true}
	return out
}

func (b *Bridge) Status() SessionResult {
	s := b.session
	s.mu.Lock()
	defer s.mu.Unlock()
	r := SessionResult{Contract: Contract}
	ctx, cancel := context.WithTimeout(s.ctx, 3*time.Second)
	defer cancel()
	cl, info, err := s.connection(ctx)
	if err != nil {
		r.Problem = problem(err, false)
		return r
	}
	defer cl.Close()
	status, err := cl.Status(ctx)
	if err != nil {
		r.Problem = problem(err, false)
		return r
	}
	if status["core"] != "ready" {
		r.Problem = &Problem{"offline"}
		return r
	}
	r.Data = &SessionInfo{InstanceID: info.InstanceID, Phase: "ready", Source: s.source(info), CanStop: slices.Contains(info.Capabilities, "graceful_stop")}
	return r
}
func provider(p state.Provider) (Provider, bool) {
	if p.Revision < 1 || p.Revision > MaxSafeRevision || (p.Protocol != "responses" && p.Protocol != "chat-completions" && p.Protocol != "anthropic") {
		return Provider{}, false
	}
	return Provider{p.ID, p.BaseURL, p.Protocol, p.Enabled, p.TimeoutSeconds, p.MaxConcurrent, p.Revision, p.HasSecret}, true
}
func (b *Bridge) Providers() ProvidersResult {
	s := b.session
	s.mu.Lock()
	defer s.mu.Unlock()
	r := ProvidersResult{Contract: Contract, Data: []Provider{}}
	ctx, cancel := context.WithTimeout(s.ctx, 15*time.Second)
	defer cancel()
	cl, _, err := s.connection(ctx)
	if err != nil {
		r.Problem = problem(err, false)
		return r
	}
	defer cl.Close()
	rows, err := cl.Providers(ctx)
	if err != nil {
		r.Problem = problem(err, false)
		return r
	}
	for _, row := range rows {
		p, ok := provider(row)
		if !ok {
			r.Data = []Provider{}
			r.Problem = &Problem{"invalid"}
			return r
		}
		r.Data = append(r.Data, p)
	}
	return r
}
func (b *Bridge) SaveProvider(in SaveProvider) SaveResult {
	s := b.session
	s.mu.Lock()
	defer s.mu.Unlock()
	r := SaveResult{Contract: Contract}
	if in.ExpectedRevision < 0 || in.ExpectedRevision >= MaxSafeRevision || in.Provider.Revision != in.ExpectedRevision {
		r.Problem = &Problem{"invalid"}
		return r
	}
	p := state.Provider{ID: in.Provider.ID, BaseURL: in.Provider.BaseURL, Protocol: in.Provider.Protocol, Enabled: in.Provider.Enabled, TimeoutSeconds: in.Provider.TimeoutSeconds, MaxConcurrent: in.Provider.MaxConcurrent}
	if err := p.Validate(); err != nil {
		r.Problem = &Problem{"invalid"}
		return r
	}
	if in.Secret != nil {
		check := p
		check.Secret = *in.Secret
		if check.Validate() != nil {
			r.Problem = &Problem{"invalid"}
			return r
		}
	}
	ctx, cancel := context.WithTimeout(s.ctx, 15*time.Second)
	defer cancel()
	cl, _, err := s.connection(ctx)
	if err != nil {
		r.Problem = problem(err, false)
		return r
	}
	defer cl.Close()
	saved, err := cl.PutProvider(ctx, p, in.ExpectedRevision, in.Secret)
	if err != nil {
		r.Problem = problem(err, true)
		return r
	}
	result, ok := provider(saved)
	if !ok {
		r.Problem = &Problem{"unknown"}
		return r
	}
	r.Data = &result
	return r
}
func (b *Bridge) StopCore(in ConfirmStop) StopResult {
	s := b.session
	s.mu.Lock()
	defer s.mu.Unlock()
	r := StopResult{Contract: Contract}
	if !in.Approved || s.cfg == nil || in.InstanceID != s.cfg.InstanceID {
		r.Problem = &Problem{"invalid"}
		return r
	}
	ctx, cancel := context.WithTimeout(s.ctx, 15*time.Second)
	defer cancel()
	cl, info, err := s.connection(ctx)
	if err != nil {
		r.Problem = problem(err, false)
		return r
	}
	defer cl.Close()
	if !slices.Contains(info.Capabilities, "graceful_stop") {
		r.Problem = &Problem{"unsupported"}
		return r
	}
	if err := cl.Stop(ctx); err != nil {
		r.Problem = problem(err, true)
		return r
	}
	r.Data = &StopInfo{Accepted: true}
	s.cfg = nil
	return r
}

func (b *Bridge) DiscoverProviderModels(in DiscoverProviderModels) ProviderModelsResult {
	s := b.session
	s.mu.Lock()
	defer s.mu.Unlock()
	out := ProviderModelsResult{Contract: Contract}
	if !state.ValidProviderID(in.ProviderID) || in.ExpectedRevision < 1 || in.ExpectedRevision > MaxSafeRevision {
		out.Problem = &Problem{"invalid"}
		return out
	}
	ctx, cancel := context.WithTimeout(s.ctx, 15*time.Second)
	defer cancel()
	cl, _, err := s.connection(ctx)
	if err != nil {
		out.Problem = problem(err, false)
		return out
	}
	defer cl.Close()
	result, err := cl.DiscoverProviderModels(ctx, in)
	if err != nil {
		out.Problem = problem(err, false)
		return out
	}
	out.Data = &result
	return out
}

func (b *Bridge) ProviderWorkspace(in WorkspaceQuery) WorkspaceResult {
	s := b.session
	s.mu.Lock()
	defer s.mu.Unlock()
	out := WorkspaceResult{Contract: Contract}
	if !state.ValidProviderID(in.ProviderID) {
		out.Problem = &Problem{"invalid"}
		return out
	}
	ctx, cancel := context.WithTimeout(s.ctx, 15*time.Second)
	defer cancel()
	cl, _, err := s.connection(ctx)
	if err != nil {
		out.Problem = problem(err, false)
		return out
	}
	defer cl.Close()
	data, err := cl.ProviderWorkspace(ctx, in.ProviderID)
	if err != nil {
		out.Problem = problem(err, false)
		return out
	}
	out.Data = &data
	return out
}
func (b *Bridge) ProviderAction(in Action) WorkspaceResult {
	s := b.session
	s.mu.Lock()
	defer s.mu.Unlock()
	out := WorkspaceResult{Contract: Contract}
	if !state.ValidProviderID(in.ProviderID) || in.ExpectedRevision < 1 || in.ExpectedRevision >= MaxSafeRevision || in.ModelRevision < 0 || in.ModelRevision >= MaxSafeRevision {
		out.Problem = &Problem{"invalid"}
		return out
	}
	ctx, cancel := context.WithTimeout(s.ctx, 15*time.Second)
	defer cancel()
	cl, _, err := s.connection(ctx)
	if err != nil {
		out.Problem = problem(err, false)
		return out
	}
	defer cl.Close()
	data, err := cl.ProviderAction(ctx, in)
	if err != nil {
		out.Problem = problem(err, true)
		return out
	}
	out.Data = &data
	return out
}
