package egress

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func clearProxyEnvironment(t *testing.T) {
	t.Helper()
	for _, k := range []string{"HTTP_PROXY", "http_proxy", "HTTPS_PROXY", "https_proxy", "ALL_PROXY", "all_proxy", "NO_PROXY", "no_proxy", "REQUEST_METHOD"} {
		t.Setenv(k, "")
	}
}

func TestSystemProxyRoutingSwitchAndFailClosed(t *testing.T) {
	clearProxyEnvironment(t)
	m, s := testManager(t)
	var proxied, direct atomic.Int32
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { direct.Add(1); io.WriteString(w, "direct") }))
	defer origin.Close()
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxied.Add(1)
		if !r.URL.IsAbs() {
			t.Error("not proxy absolute form")
		}
		io.WriteString(w, "system")
	}))
	defer proxy.Close()
	// Map a synthetic non-loopback hostname exclusively to the isolated origin.
	// The proxy route never resolves it or connects to the origin.
	m.base.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, strings.TrimPrefix(origin.URL, "http://"))
	}
	t.Setenv("HTTP_PROXY", proxy.URL)
	bind(t, s, "", "system")
	read := func(want string) {
		t.Helper()
		r := get(t, m, "http://upstream.example/v1")
		b, e := io.ReadAll(r.Body)
		r.Body.Close()
		if e != nil || string(b) != want {
			t.Fatalf("%s %v", b, e)
		}
	}
	read("system")
	bind(t, s, "default", "direct")
	read("direct")
	if proxied.Load() != 1 || direct.Load() != 1 {
		t.Fatal(proxied.Load(), direct.Load())
	}
	bind(t, s, "default", "inherit")
	read("system")
	// Changed environment is resolved fresh, not net/http's cached snapshot.
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	m.system.CloseIdleConnections()
	req, _ := http.NewRequestWithContext(WithRoute(context.Background(), "p", "default"), "GET", "http://upstream.example/v1", nil)
	if _, e := m.RoundTrip(req); !errors.Is(e, ErrUnavailable) {
		t.Fatal("proxy failure must not go direct", e)
	}
	t.Setenv("HTTP_PROXY", proxy.URL)
	t.Setenv("NO_PROXY", "upstream.example")
	if _, e := m.RoundTrip(req); !errors.Is(e, ErrUnavailable) {
		t.Fatal("bypass must fail closed", e)
	}
	if direct.Load() != 1 {
		t.Fatal("unexpected direct request", direct.Load())
	}
}

func TestSystemProxyMissingInvalidAndLocalFailClosed(t *testing.T) {
	clearProxyEnvironment(t)
	req, _ := http.NewRequest("GET", "https://upstream.example/v1", nil)
	for _, raw := range []string{"file:///synthetic-secret", "http://127.0.0.1:0"} {
		t.Setenv("HTTPS_PROXY", raw)
		u, e := requiredSystemProxy(req)
		if e == nil || u != nil {
			t.Fatalf("invalid proxy %q accepted", raw)
		}
	}
	t.Setenv("HTTPS_PROXY", "")
	// Inject an empty OS result at the transport boundary deterministically.
	local, _ := http.NewRequest("GET", "http://127.0.0.1/test", nil)
	if u, e := requiredSystemProxy(local); e == nil || u != nil {
		t.Fatal("local bypass became direct")
	}
}

func TestSystemHTTPSConnect(t *testing.T) {
	clearProxyEnvironment(t)
	m, s := testManager(t)
	origin := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "tls-system") }))
	defer origin.Close()
	m.system.TLSClientConfig = origin.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
	m.system.TLSClientConfig.ServerName = "example.com"
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "CONNECT" || r.Host != "upstream.example:443" {
			w.WriteHeader(400)
			return
		}
		up, e := net.Dial("tcp", strings.TrimPrefix(origin.URL, "https://"))
		if e != nil {
			w.WriteHeader(502)
			return
		}
		defer up.Close()
		conn, _, e := w.(http.Hijacker).Hijack()
		if e != nil {
			return
		}
		defer conn.Close()
		io.WriteString(conn, "HTTP/1.1 200 Connection Established\r\n\r\n")
		done := make(chan struct{})
		go func() { io.Copy(up, conn); close(done) }()
		io.Copy(conn, up)
		conn.Close()
		up.Close()
		<-done
	}))
	defer proxy.Close()
	t.Setenv("HTTPS_PROXY", proxy.URL)
	bind(t, s, "", "system")
	resp := get(t, m, "https://upstream.example/v1")
	b, e := io.ReadAll(resp.Body)
	resp.Body.Close()
	if e != nil || string(b) != "tls-system" {
		t.Fatal(string(b), e)
	}
}
