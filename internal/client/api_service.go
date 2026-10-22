package client

import (
	"context"
	"net/url"
	"slices"

	"github.com/candyflosssss/Anchor/internal/state"
)

type APIServiceWorkspace = state.APIServiceWorkspace

func (c *Client) APIService(ctx context.Context) (APIServiceWorkspace, error) {
	var out APIServiceWorkspace
	if !slices.Contains(c.capabilities, "api_service_v1") {
		return out, &RequestError{Status: 404, Code: "unsupported"}
	}
	err := c.call(ctx, "GET", "/control/v1/api-service", nil, &out)
	return out, err
}
func (c *Client) SaveAPIService(ctx context.Context, in state.SaveAPIServiceSettings) (APIServiceWorkspace, error) {
	var out APIServiceWorkspace
	if !slices.Contains(c.capabilities, "api_service_v1") {
		return out, &RequestError{Status: 404, Code: "unsupported"}
	}
	if err := in.Validate(); err != nil {
		return out, err
	}
	err := c.call(ctx, "POST", "/control/v1/api-service/settings", in, &out)
	return out, err
}
func (c *Client) IssueAPIKey(ctx context.Context, in state.IssueAPIKey) (state.IssuedAPIKey, error) {
	var out state.IssuedAPIKey
	if !slices.Contains(c.capabilities, "api_service_v1") {
		return out, &RequestError{Status: 404, Code: "unsupported"}
	}
	if err := in.Validate(); err != nil {
		return out, err
	}
	err := c.call(ctx, "POST", "/control/v1/api-service/keys", in, &out)
	return out, err
}
func (c *Client) RevokeAPIKey(ctx context.Context, id string) (bool, error) {
	if !slices.Contains(c.capabilities, "api_service_v1") {
		return false, &RequestError{Status: 404, Code: "unsupported"}
	}
	var out struct {
		Revoked bool `json:"revoked"`
	}
	if id == "" {
		return false, state.ErrInvalid
	}
	err := c.call(ctx, "DELETE", "/control/v1/api-service/keys/"+url.PathEscape(id), nil, &out)
	return out.Revoked, err
}
