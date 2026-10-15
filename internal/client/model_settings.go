package client

import (
	"context"
	"github.com/candyflosssss/Anchor/internal/catalog"
	"github.com/candyflosssss/Anchor/internal/state"
	"time"
)

func (c *Client) ModelSettings(ctx context.Context, in catalog.ModelSettingsQuery) (catalog.ModelSettingsState, error) {
	var out catalog.ModelSettingsState
	if !state.ValidModelID(in.ModelID) {
		return out, state.ErrInvalid
	}
	err := c.call(ctx, "GET", "/control/v1/models/"+in.ModelID+"/settings", nil, &out)
	return out, err
}
func (c *Client) SaveModelSettings(ctx context.Context, in catalog.SaveModelSettings) (catalog.ModelSettingsState, error) {
	var out catalog.ModelSettingsState
	if err := in.Validate(); err != nil {
		return out, err
	}
	// A catalog snapshot can require a bounded cold fetch. Retain ordinary
	// write-unknown handling if the connection fails after the request is sent.
	httpCopy := *c.http
	httpCopy.Timeout = 45 * time.Second
	copy := *c
	copy.http = &httpCopy
	err := copy.call(ctx, "POST", "/control/v1/models/"+in.ModelID+"/settings", in, &out)
	return out, err
}
