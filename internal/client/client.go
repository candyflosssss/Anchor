// Package client is the typed connection boundary shared by CLI and future UIs.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/candyflosssss/Anchor/internal/catalog"
	"github.com/candyflosssss/Anchor/internal/state"
)

type Client struct {
	endpoint, token, instance string
	http                      *http.Client
}
type Instance struct {
	ProcessID      int      `json:"process_id,omitempty"`
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
		return nil, Instance{}, ErrIdentity
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
		return &RequestError{Code: "offline", Unknown: method != http.MethodGet}
	}
	defer response.Body.Close()
	b, err = io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil || len(b) > 1<<20 {
		return &RequestError{Code: "invalid_response", Unknown: method != http.MethodGet}
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
		return &RequestError{Status: response.StatusCode, Code: code, Unknown: method != http.MethodGet && response.StatusCode >= 500}
	}
	if out != nil {
		if err = json.Unmarshal(b, out); err != nil {
			return &RequestError{Code: "invalid_response", Unknown: method != http.MethodGet}
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
	if !state.ValidProviderID(p.ID) {
		return state.Provider{}, errors.New("invalid provider ID")
	}
	var v state.Provider
	e := c.call(ctx, "PUT", "/control/v1/providers/"+p.ID, map[string]any{"provider": p, "expected_revision": revision, "secret": secret}, &v)
	return v, e
}

// DiscoverProviderModels only fetches the saved provider's directory; no writes or inference.
func (c *Client) DiscoverProviderModels(ctx context.Context, in catalog.DiscoverProviderModels) (catalog.ProviderModels, error) {
	if !state.ValidProviderID(in.ProviderID) || in.ExpectedRevision < 1 {
		return catalog.ProviderModels{}, errors.New("saved provider ID and revision are required")
	}
	var v catalog.ProviderModels
	err := c.call(ctx, "POST", "/control/v1/providers/"+in.ProviderID+"/discover-models", in, &v)
	// POST expresses an explicit external request, not a configuration write.
	var requestErr *RequestError
	if errors.As(err, &requestErr) {
		copy := *requestErr
		copy.Unknown = false
		err = &copy
	}
	return v, err
}

func (c *Client) PutModel(ctx context.Context, m state.Model, revision int64) (state.Model, error) {
	if !state.ValidID(m.ID) {
		return state.Model{}, errors.New("invalid model ID")
	}
	var v state.Model
	e := c.call(ctx, "PUT", "/control/v1/models/"+m.ID, map[string]any{"model": m, "expected_revision": revision}, &v)
	return v, e
}

func (c *Client) Agents(ctx context.Context) ([]state.Binding, error) {
	var v []state.Binding
	e := c.call(ctx, "GET", "/control/v1/agents", nil, &v)
	return v, e
}
func (c *Client) AddAgent(ctx context.Context, b state.Binding) (state.Binding, error) {
	var v state.Binding
	e := c.call(ctx, "POST", "/control/v1/agents", map[string]any{"binding": b, "approved": true}, &v)
	return v, e
}
func (c *Client) AgentQuery(ctx context.Context, id, action string) (map[string]any, error) {
	if !state.ValidID(id) || (action != "inspect" && action != "plan") {
		return nil, errors.New("invalid agent query")
	}
	var v map[string]any
	e := c.call(ctx, "GET", "/control/v1/agents/"+id+"/"+action, nil, &v)
	return v, e
}
func (c *Client) AgentAction(ctx context.Context, id, action string, revision int64, fp string) (map[string]any, error) {
	if !state.ValidID(id) || (action != "apply" && action != "restore" && action != "enable" && action != "disable") {
		return nil, errors.New("invalid agent action")
	}
	var v map[string]any
	e := c.call(ctx, "POST", "/control/v1/agents/"+id+"/"+action, map[string]any{"expected_revision": revision, "fingerprint": fp}, &v)
	return v, e
}

func (c *Client) SetAgentModels(ctx context.Context, id string, revision int64, model, scope string) (map[string]any, error) {
	if !state.ValidID(id) || !state.ValidID(model) || revision < 1 || (scope != "single" && scope != "all-compatible") {
		return nil, errors.New("invalid binding model policy")
	}
	var v map[string]any
	err := c.call(ctx, "POST", "/control/v1/agents/"+id+"/set-models", map[string]any{"expected_revision": revision, "model": model, "model_scope": scope}, &v)
	return v, err
}

// Stop requests graceful shutdown. Acceptance is not proof of completed recovery.
func (c *Client) Stop(ctx context.Context) error {
	var result struct {
		Accepted bool `json:"accepted"`
	}
	if err := c.call(ctx, "POST", "/control/v1/shutdown", struct{}{}, &result); err != nil {
		return err
	}
	if !result.Accepted {
		return &RequestError{Code: "invalid_response", Unknown: true}
	}
	return nil
}
