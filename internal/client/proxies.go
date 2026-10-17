package client

import (
	"context"
	"github.com/candyflosssss/Anchor/internal/state"
)

func (c *Client) ProxyWorkspace(ctx context.Context) (state.ProxyWorkspace, error) {
	var out state.ProxyWorkspace
	e := c.call(ctx, "GET", "/control/v1/proxies", nil, &out)
	return out, e
}
func (c *Client) ProxyAction(ctx context.Context, in state.ProxyAction) (state.ProxyWorkspace, error) {
	var out state.ProxyWorkspace
	e := c.call(ctx, "POST", "/control/v1/proxies/action", in, &out)
	return out, e
}
