package core

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/candyflosssss/Anchor/internal/egress"
	"github.com/candyflosssss/Anchor/internal/providerapi"
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

func TestSystemProxyGatewayAndNoDirectFailover(t *testing.T) {
	f := setup(t)
	for _, key := range []string{"HTTP_PROXY", "http_proxy", "HTTPS_PROXY", "https_proxy", "ALL_PROXY", "all_proxy", "NO_PROXY", "no_proxy", "REQUEST_METHOD"} {
		t.Setenv(key, "")
	}
	var direct atomic.Int32
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		direct.Add(1)
		io.WriteString(w, `{"id":"direct","output":[]}`)
	}))
	defer up.Close()
	f.add(t, "p", "https://upstream.example/v1", "synthetic-key")
	f.add(t, "fallback", up.URL, "fallback-key")
	var proxied atomic.Int32
	origin := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxied.Add(1)
		if r.Header.Get("Authorization") != "Bearer synthetic-key" {
			t.Error("credential lost")
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"id":"system","output":[]}`)
	}))
	defer origin.Close()
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect || r.Host != "upstream.example:443" {
			t.Error("expected CONNECT to the isolated upstream")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		upstream, err := net.Dial("tcp", origin.Listener.Addr().String())
		if err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		defer upstream.Close()
		conn, buf, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		defer conn.Close()
		io.WriteString(conn, "HTTP/1.1 200 Connection Established\r\n\r\n")
		done := make(chan struct{})
		go func() { io.Copy(upstream, buf); upstream.Close(); close(done) }()
		io.Copy(conn, upstream)
		conn.Close()
		<-done
	}))
	defer proxy.Close()
	tr := origin.Client().Transport.(*http.Transport).Clone()
	tr.TLSClientConfig.ServerName = "example.com" // The isolated httptest certificate.
	tr.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		if address != proxy.Listener.Addr().String() && address != up.Listener.Addr().String() {
			direct.Add(1)
			return nil, errors.New("direct upstream dialing forbidden")
		}
		return (&net.Dialer{}).DialContext(ctx, network, address)
	}
	f.core.egress.Close()
	f.core.egress = egress.New(f.store, tr)
	f.core.upstream.Transport = f.core.egress
	defer f.core.egress.Close()
	t.Setenv("HTTPS_PROXY", proxy.URL)
	w, _ := f.store.ProxyWorkspace()
	if e := f.store.MutateProxy(state.ProxyAction{Action: "bind", Approved: true, ExpectedRevision: w.Revision, Binding: state.ProxyBinding{ProviderID: "p", Mode: "system"}}, egress.Normalize); e != nil {
		t.Fatal(e)
	}
	f.model(t, state.Candidate{ProviderID: "p", Model: "up"}, state.Candidate{ProviderID: "fallback", Model: "up"})
	r := request(t, "POST", f.gateway.URL+"/v1/responses", f.token, `{"model":"public","input":"hi"}`)
	if body := text(t, r); r.StatusCode != 200 || !strings.Contains(body, "system") || proxied.Load() != 1 || direct.Load() != 0 {
		t.Fatal(r.StatusCode, body, proxied.Load(), direct.Load())
	}
	// A NO_PROXY match must not turn an explicitly proxied inference into direct.
	t.Setenv("NO_PROXY", "upstream.example")
	r = request(t, "POST", f.gateway.URL+"/v1/responses", f.token, `{"model":"public","input":"hi"}`)
	if body := text(t, r); r.StatusCode != 502 || !strings.Contains(body, "proxy_unavailable") || direct.Load() != 0 {
		t.Fatal(r.StatusCode, body, direct.Load())
	}
	t.Setenv("NO_PROXY", "")
	proxy.Close()
	// Closing the listener does not close an already-established CONNECT tunnel.
	f.core.egress.CloseIdleConnections()
	r = request(t, "POST", f.gateway.URL+"/v1/responses", f.token, `{"model":"public","input":"hi"}`)
	if body := text(t, r); r.StatusCode != 502 || !strings.Contains(body, "proxy_unavailable") || direct.Load() != 0 {
		t.Fatal(r.StatusCode, body, direct.Load())
	}
}
