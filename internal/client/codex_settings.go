package client

import (
	"context"
	"github.com/candyflosssss/Anchor/internal/managed"
	"github.com/candyflosssss/Anchor/internal/state"
)

func (c *Client) CodexSettings(ctx context.Context) (managed.CodexSettingsState, error) {
	var out managed.CodexSettingsState
	err := c.call(ctx, "GET", "/control/v1/settings/codex", nil, &out)
	return out, err
}
func (c *Client) SaveCodexSettings(ctx context.Context, in state.SaveCodexSettings) (managed.CodexSettingsState, error) {
	var out managed.CodexSettingsState
	if err := in.Validate(); err != nil {
		return out, err
	}
	err := c.call(ctx, "POST", "/control/v1/settings/codex", in, &out)
	return out, err
}
