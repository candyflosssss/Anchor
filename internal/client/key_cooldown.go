package client

import (
	"context"
	"github.com/candyflosssss/Anchor/internal/state"
)

func (c *Client) KeyCooldownSettings(ctx context.Context) (state.KeyCooldownSettings, error) {
	var out state.KeyCooldownSettings
	err := c.call(ctx, "GET", "/control/v1/settings/key-cooldown", nil, &out)
	return out, err
}
func (c *Client) SaveKeyCooldownSettings(ctx context.Context, in state.SaveKeyCooldownSettings) (state.KeyCooldownSettings, error) {
	var out state.KeyCooldownSettings
	if err := in.Validate(); err != nil {
		return out, err
	}
	err := c.call(ctx, "POST", "/control/v1/settings/key-cooldown", in, &out)
	return out, err
}
