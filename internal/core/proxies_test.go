package core

import (
	"context"
	"encoding/json"
	"github.com/candyflosssss/Anchor/internal/providerapi"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/candyflosssss/Anchor/internal/egress"
	"github.com/candyflosssss/Anchor/internal/state"
)

func configureProxy(t *testing.T, f *fixture, raw string) {
	t.Helper()
	w, _ := f.store.ProxyWorkspace()
	e := f.store.MutateProxy(state.ProxyAction{Action: "put", Profile: state.ProxyProfile{ID: "edge", Name: "edge", Kind: "http"}, Secret: &raw, Approved: true, ExpectedRevision: w.Revision}, egress.Normalize)
	if e != nil {
		t.Fatal(e)
	}
	w, _ = f.store.ProxyWorkspace()
	e = f.store.MutateProxy(state.ProxyAction{Action: "bind", Binding: state.ProxyBinding{ProviderID: "p", Mode: "proxy", ProxyID: "edge"}, Approved: true, ExpectedRevision: w.Revision}, egress.Normalize)
	if e != nil {
		t.Fatal(e)
	}
}
func TestProxyGatewayAndNoDirectFailover(t *testing.T) {
	f := setup(t)
	var direct atomic.Int32
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		direct.Add(1)
		io.WriteString(w, `{"id":"direct","output":[]}`)
	}))
	defer up.Close()
	f.add(t, "p", up.URL, "synthetic-key")
	f.add(t, "fallback", up.URL, "other-key")
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer synthetic-key" {
			t.Error("lost key")
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"id":"proxied","output":[]}`)
	}))
	defer proxy.Close()
	configureProxy(t, f, proxy.URL)
	f.model(t, state.Candidate{ProviderID: "p", Model: "up"}, state.Candidate{ProviderID: "fallback", Model: "up"})
	r := request(t, "POST", f.gateway.URL+"/v1/responses", f.token, `{"model":"public","input":"hi"}`)
	if r.StatusCode != 200 || !strings.Contains(text(t, r), "proxied") || direct.Load() != 0 {
		t.Fatal(r.StatusCode, direct.Load())
	}
	proxy.Close()
	r = request(t, "POST", f.gateway.URL+"/v1/responses", f.token, `{"model":"public","input":"hi"}`)
	body := text(t, r)
	if r.StatusCode != 502 || !strings.Contains(body, "proxy_unavailable") || direct.Load() != 0 {
		t.Fatal(r.StatusCode, body, direct.Load())
	}
}
func TestProxyManagementAuthorizationAndRedaction(t *testing.T) {
	f := setup(t)
	f.add(t, "p", "http://127.0.0.1:1", "synthetic-key")
	r := request(t, "GET", f.control.URL+"/control/v1/proxies", f.token, "")
	if r.StatusCode == 200 {
		t.Fatal("gateway token accessed management")
	}
	in := state.ProxyAction{Action: "put", Profile: state.ProxyProfile{ID: "edge", Name: "test", Kind: "http"}, Approved: true, ExpectedRevision: 1}
	raw := "http://user:secret-sentinel@localhost:1234"
	in.Secret = &raw
	data, _ := json.Marshal(in)
	r = request(t, "POST", f.control.URL+"/control/v1/proxies/action", f.owner, string(data))
	if r.StatusCode == 200 {
		t.Fatal("identity not required")
	}
	cfg, _ := f.store.Settings()
	req, _ := http.NewRequest("POST", f.control.URL+"/control/v1/proxies/action", strings.NewReader(string(data)))
	req.Header.Set("Authorization", "Bearer "+f.owner)
	req.Header.Set("X-Anchor-Instance", cfg.InstanceID)
	resp, e := http.DefaultClient.Do(req)
	if e != nil {
		t.Fatal(e)
	}
	defer resp.Body.Close()
	body := text(t, resp)
	if resp.StatusCode != 200 || strings.Contains(body, "secret-sentinel") || strings.Contains(body, "localhost") {
		t.Fatal(resp.StatusCode, body)
	}
}

func TestProxyCatalogProbeAndKeyOverride(t *testing.T) {
	f := setup(t)
	var direct atomic.Int32
	var routed atomic.Int32
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		direct.Add(1)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"data":[{"id":"direct-model"}]}`)
	}))
	defer up.Close()
	f.add(t, "p", up.URL, "synthetic-key")
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		routed.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/models") {
			io.WriteString(w, `{"data":[{"id":"proxied-model"}]}`)
		} else {
			io.WriteString(w, `{"id":"ok","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"OK"}]}]}`)
		}
	}))
	defer proxy.Close()
	configureProxy(t, f, proxy.URL)
	out, e := f.core.QueryProvider(context.Background(), providerapi.Query{ProviderID: "p", ExpectedRevision: 1, Operation: "models"})
	if e != nil || out.Failure != "" || len(out.Models) != 1 || out.Models[0] != "proxied-model" {
		t.Fatal(out, e)
	}
	p, _ := f.store.Provider("p")
	probe, e := f.core.probe(context.Background(), p, "default", "test")
	if e != nil || probe.Failure != "" || routed.Load() < 2 {
		t.Fatal(probe, e, routed.Load())
	}
	w, _ := f.store.ProxyWorkspace()
	e = f.store.MutateProxy(state.ProxyAction{Action: "bind", Approved: true, ExpectedRevision: w.Revision, Binding: state.ProxyBinding{ProviderID: "p", KeyID: "default", Mode: "direct"}}, egress.Normalize)
	if e != nil {
		t.Fatal(e)
	}
	out, e = f.core.QueryProvider(context.Background(), providerapi.Query{ProviderID: "p", ExpectedRevision: 1, Operation: "models"})
	if e != nil || out.Failure != "" || len(out.Models) != 1 || out.Models[0] != "direct-model" || direct.Load() != 1 {
		t.Fatal(out, e, direct.Load())
	}
}
