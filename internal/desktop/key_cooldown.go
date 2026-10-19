package desktop

import (
	"context"
	"github.com/candyflosssss/Anchor/internal/client"
	"time"
)

func (b *Bridge) KeyCooldownSettings() KeyCooldownSettingsResult {
	return b.keyCooldownSettingsCall(nil)
}
func (b *Bridge) SaveKeyCooldownSettings(in SaveKeyCooldownSettings) KeyCooldownSettingsResult {
	if in.Validate() != nil {
		return KeyCooldownSettingsResult{Contract: Contract, Problem: &Problem{"invalid"}}
	}
	return b.keyCooldownSettingsCall(func(ctx context.Context, cl *client.Client) (KeyCooldownSettings, error) {
		return cl.SaveKeyCooldownSettings(ctx, in)
	})
}
func (b *Bridge) keyCooldownSettingsCall(write func(context.Context, *client.Client) (KeyCooldownSettings, error)) KeyCooldownSettingsResult {
	out := KeyCooldownSettingsResult{Contract: Contract}
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
	var data KeyCooldownSettings
	if write == nil {
		data, err = cl.KeyCooldownSettings(ctx)
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
