package desktop

import (
	"context"
	"github.com/candyflosssss/Anchor/internal/state"
	"time"
)

func (b *Bridge) ModelSettings(in ModelSettingsQuery) ModelSettingsResult {
	out := ModelSettingsResult{Contract: Contract}
	if !state.ValidModelID(in.ModelID) {
		out.Problem = &Problem{"invalid"}
		return out
	}
	s := b.session
	s.mu.Lock()
	defer s.mu.Unlock()
	ctx, cancel := context.WithTimeout(s.ctx, 15*time.Second)
	defer cancel()
	cl, _, err := s.connection(ctx)
	if err != nil {
		out.Problem = problem(err, false)
		return out
	}
	defer cl.Close()
	data, err := cl.ModelSettings(ctx, in)
	if err != nil {
		out.Problem = problem(err, false)
		return out
	}
	out.Data = &data
	return out
}
func (b *Bridge) SaveModelSettings(in SaveModelSettings) ModelSettingsResult {
	out := ModelSettingsResult{Contract: Contract}
	if in.Validate() != nil {
		out.Problem = &Problem{"invalid"}
		return out
	}
	s := b.session
	s.mu.Lock()
	defer s.mu.Unlock()
	ctx, cancel := context.WithTimeout(s.ctx, 45*time.Second)
	defer cancel()
	cl, _, err := s.connection(ctx)
	if err != nil {
		out.Problem = problem(err, false)
		return out
	}
	defer cl.Close()
	data, err := cl.SaveModelSettings(ctx, in)
	if err != nil {
		out.Problem = problem(err, true)
		return out
	}
	out.Data = &data
	return out
}
