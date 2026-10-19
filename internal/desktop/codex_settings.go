package desktop

import (
	"context"
	"github.com/candyflosssss/Anchor/internal/client"
	"time"
)

func (b *Bridge) CodexSettings() CodexSettingsResult { return b.codexSettingsCall(nil) }
func (b *Bridge) SaveCodexSettings(in SaveCodexSettings) CodexSettingsResult {
	if in.Validate() != nil {
		return CodexSettingsResult{Contract: Contract, Problem: &Problem{"invalid"}}
	}
	return b.codexSettingsCall(func(ctx context.Context, cl *client.Client) (CodexSettingsState, error) {
		return cl.SaveCodexSettings(ctx, in)
	})
}
func (b *Bridge) codexSettingsCall(write func(context.Context, *client.Client) (CodexSettingsState, error)) CodexSettingsResult {
	out := CodexSettingsResult{Contract: Contract}
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
	var data CodexSettingsState
	if write == nil {
		data, err = cl.CodexSettings(ctx)
	} else {
		data, err = write(ctx, cl)
	}
	if err != nil {
		out.Problem = problem(err, write != nil)
		return out
	}
	out.Data = &data
	return out
}
