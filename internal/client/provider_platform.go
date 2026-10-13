package client

import (
	"context"
	"errors"

	"github.com/candyflosssss/Anchor/internal/providerapi"
	"github.com/candyflosssss/Anchor/internal/state"
)

func (c *Client) ProviderPlatform(ctx context.Context, id string) (providerapi.Settings, error) {
	if !state.ValidProviderID(id) {
		return providerapi.Settings{}, errors.New("invalid provider ID")
	}
	var out providerapi.Settings
	err := c.call(ctx, "GET", "/control/v1/providers/"+id+"/platform", nil, &out)
	return out, err
}
func (c *Client) ConfigureProviderPlatform(ctx context.Context, in providerapi.Configure) (providerapi.Settings, error) {
	if !state.ValidProviderID(in.ProviderID) || in.ExpectedRevision < 1 || in.Config.Validate() != nil {
		return providerapi.Settings{}, errors.New("invalid platform configuration")
	}
	var out providerapi.Settings
	err := c.call(ctx, "PUT", "/control/v1/providers/"+in.ProviderID+"/platform", in, &out)
	return out, err
}
func (c *Client) QueryProvider(ctx context.Context, in providerapi.Query) (providerapi.Result, error) {
	if !state.ValidProviderID(in.ProviderID) || (in.KeyID != "" && !state.ValidID(in.KeyID)) || in.Validate() != nil {
		return providerapi.Result{}, errors.New("invalid platform query")
	}
	var out providerapi.Result
	err := c.call(ctx, "POST", "/control/v1/providers/"+in.ProviderID+"/query", in, &out)
	return out, err
}
