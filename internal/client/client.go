// Package client is the typed connection boundary shared by CLI and future UIs.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/candyflosssss/Anchor/internal/state"
)

type Client struct {
	endpoint, token, instance string
	http                      *http.Client
}
type Instance struct {
	InstanceID     string   `json:"instance_id"`
	Version        string   `json:"version"`
	APIVersion     int      `json:"api_version"`
	Platform       string   `json:"platform"`
	User           string   `json:"user"`
	ControlAddress string   `json:"control_address"`
	GatewayAddress string   `json:"gateway_address"`
	Capabilities   []string `json:"capabilities"`
}

func Connect(ctx context.Context, c state.Context, token string) (*Client, Instance, error) {
	if err := c.Validate(); err != nil {
		return nil, Instance{}, err
	}
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.Proxy = nil
	cl := &Client{endpoint: c.Endpoint, token: token, instance: c.InstanceID, http: &http.Client{Timeout: 15 * time.Second, Transport: tr, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
	var instance Instance
	if err := cl.call(ctx, http.MethodGet, "/control/v1/instance", nil, &instance); err != nil {
		cl.Close()
		return nil, Instance{}, err
	}
	if instance.APIVersion != 1 || instance.InstanceID != c.InstanceID {
		cl.Close()
		return nil, Instance{}, errors.New("instance identity or API version mismatch; explicitly rebind the context")
	}
	return cl, instance, nil
}
func (c *Client) Close() { c.http.CloseIdleConnections() }
func (c *Client) call(ctx context.Context, method, path string, in, out any) error {
	var b []byte
	var err error
	if in != nil {
		b, err = json.Marshal(in)
		if err != nil {
			return err
		}
	}
	r, err := http.NewRequestWithContext(ctx, method, c.endpoint+path, bytes.NewReader(b))
	if err != nil {
		return err
	}
	r.Header.Set("Authorization", "Bearer "+c.token)
	r.Header.Set("X-Anchor-Instance", c.instance)
	if in != nil {
		r.Header.Set("Content-Type", "application/json")
	}
	response, err := c.http.Do(r)
	if err != nil {
		return errors.New("management endpoint unavailable; no fallback target was used")
	}
	defer response.Body.Close()
	b, err = io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil || len(b) > 1<<20 {
		return errors.New("invalid or oversized management response")
	}
	if response.StatusCode != 200 {
		// Interpret a bounded error code only; do not reflect arbitrary remote error text.
		var e struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		_ = json.Unmarshal(b, &e)
		code := e.Error.Code
		if !state.ValidID(code) {
			code = "request_failed"
		}
		return fmt.Errorf("management request failed: HTTP %d (%s); writes are not automatically retried", response.StatusCode, code)
	}
	if out != nil {
		if err = json.Unmarshal(b, out); err != nil {
			return errors.New("invalid management JSON response")
		}
	}
	return nil
}
func (c *Client) Status(ctx context.Context) (map[string]any, error) {
	var v map[string]any
	e := c.call(ctx, "GET", "/control/v1/status", nil, &v)
	return v, e
}
func (c *Client) Providers(ctx context.Context) ([]state.Provider, error) {
	var v []state.Provider
	e := c.call(ctx, "GET", "/control/v1/providers", nil, &v)
	return v, e
}
func (c *Client) Models(ctx context.Context) ([]state.Model, error) {
	var v []state.Model
	e := c.call(ctx, "GET", "/control/v1/models", nil, &v)
	return v, e
}
func (c *Client) PutProvider(ctx context.Context, p state.Provider, revision int64, secret *string) (state.Provider, error) {
	if !state.ValidID(p.ID) {
		return state.Provider{}, errors.New("invalid provider ID")
	}
	var v state.Provider
	e := c.call(ctx, "PUT", "/control/v1/providers/"+p.ID, map[string]any{"provider": p, "expected_revision": revision, "secret": secret}, &v)
	return v, e
}
func (c *Client) PutModel(ctx context.Context, m state.Model, revision int64) (state.Model, error) {
	if !state.ValidID(m.ID) {
		return state.Model{}, errors.New("invalid model ID")
	}
	var v state.Model
	e := c.call(ctx, "PUT", "/control/v1/models/"+m.ID, map[string]any{"model": m, "expected_revision": revision}, &v)
	return v, e
}
