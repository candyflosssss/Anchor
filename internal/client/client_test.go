package client

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/candyflosssss/Anchor/internal/core"
	"github.com/candyflosssss/Anchor/internal/state"
)

func connection(t *testing.T) (*Client, *state.Store, state.Context, string) {
	t.Helper()
	s, e := state.Open(t.TempDir(), true)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { s.Close() })
	cfg, e := s.Initialize("127.0.0.1:17861", "127.0.0.1:17860")
	if e != nil {
		t.Fatal(e)
	}
	token, e := s.OwnerToken()
	if e != nil {
		t.Fatal(e)
	}
	srv := core.New(s, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	t.Cleanup(srv.Close)
	web := httptest.NewServer(srv.Control())
	t.Cleanup(web.Close)
	c := state.Context{Name: "local", Endpoint: web.URL, InstanceID: cfg.InstanceID, CredentialRef: "local-owner"}
	cl, _, e := Connect(context.Background(), c, token)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(cl.Close)
	return cl, s, c, token
}
func TestTypedManagementClient(t *testing.T) {
	c, _, _, _ := connection(t)
	ctx := context.Background()
	if status, e := c.Status(ctx); e != nil || status["core"] != "ready" {
		t.Fatal(status, e)
	}
	key := "secret"
	p := state.Provider{ID: "p", BaseURL: "https://example.invalid/v1", Protocol: "responses", Enabled: true, TimeoutSeconds: 10, MaxConcurrent: 2}
	saved, e := c.PutProvider(ctx, p, 0, &key)
	if e != nil || saved.Revision != 1 || saved.Secret != "" {
		t.Fatal(saved, e)
	}
	if _, e = c.PutProvider(ctx, p, 0, nil); e == nil {
		t.Fatal("duplicate create was retried/accepted")
	}
	list, e := c.Providers(ctx)
	if e != nil || len(list) != 1 || !list[0].HasSecret {
		t.Fatal(list, e)
	}
	m := state.Model{ID: "public", Candidates: []state.Candidate{{ProviderID: "p", Model: "actual"}}}
	if _, e = c.PutModel(ctx, m, 0); e != nil {
		t.Fatal(e)
	}
	models, e := c.Models(ctx)
	if e != nil || len(models) != 1 {
		t.Fatal(models, e)
	}
}
func TestIdentityMismatchRejected(t *testing.T) {
	_, _, c, token := connection(t)
	c.InstanceID = "wrong"
	if cl, _, e := Connect(context.Background(), c, token); e == nil {
		cl.Close()
		t.Fatal("instance mismatch accepted")
	}
}
func TestManagementProxyAndRedirectPolicy(t *testing.T) {
	var proxyCalls, targetCalls int
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { proxyCalls++; w.WriteHeader(502) }))
	defer proxy.Close()
	t.Setenv("HTTP_PROXY", proxy.URL)
	t.Setenv("NO_PROXY", "")
	c, _, _, _ := connection(t)
	if _, e := c.Status(context.Background()); e != nil || proxyCalls != 0 {
		t.Fatal("ambient proxy used", e)
	}
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { targetCalls++ }))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 307) }))
	defer redirect.Close()
	ctx := state.Context{Name: "local", Endpoint: redirect.URL, InstanceID: "any", CredentialRef: "local-owner"}
	if cl, _, e := Connect(context.Background(), ctx, "secret"); e == nil {
		cl.Close()
		t.Fatal("redirect accepted")
	}
	if targetCalls != 0 {
		t.Fatal("redirect target visited")
	}
}
func TestAPIVersionAndSecretBearingErrors(t *testing.T) {
	for _, tc := range []struct {
		status int
		body   string
	}{{200, `{"instance_id":"id","api_version":2}`}, {500, `{"error":{"code":"invalid code SECRET","message":"SECRET"}}`}} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(tc.status); io.WriteString(w, tc.body) }))
		c := state.Context{Name: "local", Endpoint: srv.URL, InstanceID: "id", CredentialRef: "local-owner"}
		cl, _, e := Connect(context.Background(), c, "secret")
		srv.Close()
		if e == nil {
			cl.Close()
			t.Fatal("bad API response accepted")
		}
		if strings.Contains(e.Error(), "SECRET") {
			t.Fatal("error reflected secret")
		}
	}
}
